package sniproxy

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

// genSelfSigned 生成自签证书，供真实 TLS 握手使用。
func genSelfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		DNSNames:              []string{"example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// captureClientHello 启动一个 TCP server，把客户端发来的前 N 字节存下来供测试。
// 返回 server 监听地址和一个 chan，该 chan 在连接建立后发送捕获的字节。
func captureClientHello(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })

	out := make(chan []byte, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 8192)
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _ := c.Read(buf)
		out <- buf[:n]
	}()
	return l.Addr().String(), out
}

// clientHelloWithSNI 真实发起一次 TLS 握手（会失败，因为 server 不响应），
// 目的只是让客户端把 ClientHello 写到 wire 上，我们捕获它。
func clientHelloWithSNI(t *testing.T, sni string) []byte {
	t.Helper()
	addr, ch := captureClientHello(t)
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
		_ = tlsConn.HandshakeContext(timeoutCtx(t))
	}()
	select {
	case data := <-ch:
		return data
	case <-time.After(3 * time.Second):
		t.Fatal("timed out capturing ClientHello")
		return nil
	}
}

func timeoutCtx(t *testing.T) contextLike {
	return contextLike{t: t, deadline: time.Now().Add(2 * time.Second)}
}

// contextLike 是最小 context 实现，避免引入 context 包仅为此测试。
type contextLike struct {
	t        *testing.T
	deadline time.Time
}

func (c contextLike) Deadline() (time.Time, bool) { return c.deadline, true }
func (c contextLike) Done() <-chan struct{}       { return nil }
func (c contextLike) Err() error                  { return nil }
func (c contextLike) Value(any) any               { return nil }

func TestPeekSNI_Valid(t *testing.T) {
	tests := []string{
		"example.com",
		"cdn-ad5d.wam.qzz.io",
		"a.b.c.d.very.long.subdomain.example.org",
	}
	for _, want := range tests {
		t.Run(want, func(t *testing.T) {
			hello := clientHelloWithSNI(t, want)
			got, peeked, err := PeekSNI(bytes.NewReader(hello))
			if err != nil {
				t.Fatalf("PeekSNI: %v", err)
			}
			if got != want {
				t.Errorf("SNI = %q, want %q", got, want)
			}
			if !bytes.Equal(peeked, hello[:len(peeked)]) {
				t.Error("peeked bytes do not match input prefix")
			}
		})
	}
}

func TestPeekSNI_NotTLS(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\n\r\n")
	_, _, err := PeekSNI(bytes.NewReader(data))
	if !errors.Is(err, ErrNotTLS) {
		t.Errorf("err = %v, want ErrNotTLS", err)
	}
}

func TestPeekSNI_Truncated(t *testing.T) {
	// 只给 3 字节，连 record header 都读不全
	_, _, err := PeekSNI(bytes.NewReader([]byte{0x16, 0x03, 0x01}))
	if err == nil {
		t.Error("expected error on truncated input")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Logf("got %v (acceptable if it wraps EOF)", err)
	}
}

func TestPeekSNI_NoSNI(t *testing.T) {
	// 真实握手但不设 ServerName，ClientHello 里不应有 SNI 扩展
	addr, ch := captureClientHello(t)
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
		_ = tlsConn.HandshakeContext(timeoutCtx(t))
	}()
	var hello []byte
	select {
	case hello = <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	_, _, err := PeekSNI(bytes.NewReader(hello))
	if !errors.Is(err, ErrNoSNI) {
		t.Errorf("err = %v, want ErrNoSNI", err)
	}
}

func TestPeekSNI_InvalidRecordLength(t *testing.T) {
	// type=0x16 handshake，但 length 写成 0xFFFF（超过 maxClientHelloLen）
	data := []byte{0x16, 0x03, 0x01, 0xFF, 0xFF}
	_, _, err := PeekSNI(bytes.NewReader(data))
	if err == nil {
		t.Error("expected error on oversized record")
	}
}
