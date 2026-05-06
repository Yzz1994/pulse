package cert

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type NodeCA struct {
	cert    *x509.Certificate
	key     crypto.Signer
	certPEM []byte
}

func LoadOrCreateNodeCA(certPath, keyPath string) (*NodeCA, error) {
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("cert path and key path are required")
	}

	certInfo, certErr := os.Stat(certPath)
	keyInfo, keyErr := os.Stat(keyPath)
	bothExist := certErr == nil && keyErr == nil && !certInfo.IsDir() && !keyInfo.IsDir()

	if bothExist {
		ca, err := loadNodeCA(certPath, keyPath)
		if err == nil {
			return ca, nil
		}
		return nil, err
	}
	if (certErr == nil) != (keyErr == nil) {
		return nil, fmt.Errorf("cert file and key file must exist together")
	}
	if certErr != nil && !os.IsNotExist(certErr) {
		return nil, fmt.Errorf("stat cert file: %w", certErr)
	}
	if keyErr != nil && !os.IsNotExist(keyErr) {
		return nil, fmt.Errorf("stat key file: %w", keyErr)
	}

	return createNodeCA(certPath, keyPath)
}

func loadNodeCA(certPath, keyPath string) (*NodeCA, error) {
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read cert file: %w", err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	certBlock, _ := pem.Decode(certBytes)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("decode CA cert PEM: invalid block")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	now := time.Now()
	if now.Before(caCert.NotBefore) || now.After(caCert.NotAfter) {
		return nil, fmt.Errorf("CA cert expired or not yet valid: notBefore=%s notAfter=%s", caCert.NotBefore, caCert.NotAfter)
	}

	keyBlock, _ := pem.Decode(keyBytes)
	if keyBlock == nil {
		return nil, fmt.Errorf("decode CA key PEM: invalid block")
	}
	signer, err := parsePrivateKey(keyBlock)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}

	return &NodeCA{cert: caCert, key: signer, certPEM: certBytes}, nil
}

func parsePrivateKey(block *pem.Block) (crypto.Signer, error) {
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return k, nil
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not a signer")
		}
		return s, nil
	case "EC PRIVATE KEY":
		k, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return k, nil
	default:
		return nil, fmt.Errorf("unsupported key PEM type: %s", block.Type)
	}
}

func createNodeCA(certPath, keyPath string) (*NodeCA, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "pulse-node-ca",
			Organization: []string{"pulse"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse newly created CA cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return nil, fmt.Errorf("create cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write CA cert file: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write CA key file: %w", err)
	}

	return &NodeCA{cert: caCert, key: priv, certPEM: certPEM}, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return n, nil
}

// SignCSR signs the given PEM-encoded CSR producing a node client certificate
// whose CommonName is forced to nodeID. The CSR's self-reported subject is not
// trusted; only its public key and signature are used.
func (ca *NodeCA) SignCSR(csrPEM []byte, nodeID string, ttl time.Duration) ([]byte, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl must be positive")
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("decode CSR PEM: invalid block")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("verify CSR signature: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	uri := &url.URL{Scheme: "pulse-node", Host: nodeID}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   nodeID,
			Organization: []string{"pulse-node"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		URIs:                  []*url.URL{uri},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// CertPEM returns the CA's own certificate in PEM form. Safe to share with
// nodes (used as RootCA for verifying the server when establishing mTLS).
func (ca *NodeCA) CertPEM() []byte {
	out := make([]byte, len(ca.certPEM))
	copy(out, ca.certPEM)
	return out
}

// ClientCAPool returns a CertPool containing only this CA's certificate,
// suitable for use as tls.Config.ClientCAs on the gRPC server.
func (ca *NodeCA) ClientCAPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

// IssueServerCert issues a TLS server certificate signed by this CA, using a
// freshly generated RSA-2048 key pair owned by the returned tls.Certificate.
// The certificate's CommonName is set to cn and its SAN list is populated from
// sans (DNS names and/or IP literals; IPs are detected via net.ParseIP).
//
// This is intended for the nodehub gRPC server's TLS identity: nodes already
// trust the NodeCA (delivered via enroll response), so they will accept any
// server cert chained to it without extra trust configuration.
func (ca *NodeCA) IssueServerCert(cn string, sans []string, ttl time.Duration) (tls.Certificate, error) {
	if cn == "" {
		return tls.Certificate{}, fmt.Errorf("common name is required")
	}
	if ttl <= 0 {
		return tls.Certificate{}, fmt.Errorf("ttl must be positive")
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}

	dnsNames := make([]string, 0, len(sans))
	ipAddrs := make([]net.IP, 0)
	for _, s := range sans {
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, s)
		}
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"pulse"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &priv.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("sign server certificate: %w", err)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse issued server cert: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
		Leaf:        leaf,
	}, nil
}
