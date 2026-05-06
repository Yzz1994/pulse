package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustGenCSR(t *testing.T, cn string) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, priv)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), priv
}

func TestLoadOrCreateNodeCA_CreateThenReuse(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "sub", "node_ca_cert.pem")
	keyPath := filepath.Join(dir, "sub", "node_ca_key.pem")

	ca1, err := LoadOrCreateNodeCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if ca1.cert.Subject.CommonName != "pulse-node-ca" {
		t.Fatalf("CN = %q, want pulse-node-ca", ca1.cert.Subject.CommonName)
	}
	if !ca1.cert.IsCA || !ca1.cert.BasicConstraintsValid {
		t.Fatalf("CA flags wrong")
	}
	if ca1.cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatalf("missing KeyUsageCertSign")
	}
	if ca1.cert.KeyUsage&x509.KeyUsageCRLSign == 0 {
		t.Fatalf("missing KeyUsageCRLSign")
	}
	if rsaKey, ok := ca1.key.(*rsa.PrivateKey); !ok {
		t.Fatalf("key type %T, want *rsa.PrivateKey", ca1.key)
	} else if rsaKey.N.BitLen() != 4096 {
		t.Fatalf("key bits = %d, want 4096", rsaKey.N.BitLen())
	}

	ca2, err := LoadOrCreateNodeCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !ca1.cert.Equal(ca2.cert) {
		t.Fatalf("reload returned different cert")
	}
}

func TestLoadOrCreateNodeCA_PartialFilesError(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "node_ca_cert.pem")
	keyPath := filepath.Join(dir, "node_ca_key.pem")

	if _, err := LoadOrCreateNodeCA(certPath, keyPath); err != nil {
		t.Fatalf("create: %v", err)
	}
	// remove cert only
	if err := os.Remove(certPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := LoadOrCreateNodeCA(certPath, keyPath); err == nil {
		t.Fatalf("expected error when only key exists")
	}
}

func TestSignCSR_Success(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}

	csrPEM, _ := mustGenCSR(t, "attacker-claims-this-cn")
	const nodeID = "node-abc"

	certPEM, err := ca.SignCSR(csrPEM, nodeID, time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("decode signed cert")
	}
	signed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse signed: %v", err)
	}
	if signed.Subject.CommonName != nodeID {
		t.Fatalf("CN = %q, want %q (CSR CN must be overridden)", signed.Subject.CommonName, nodeID)
	}
	if signed.Issuer.CommonName != ca.cert.Subject.CommonName {
		t.Fatalf("issuer CN = %q, want %q", signed.Issuer.CommonName, ca.cert.Subject.CommonName)
	}
	if len(signed.URIs) != 1 || signed.URIs[0].String() != "pulse-node://"+nodeID {
		t.Fatalf("URIs = %v, want pulse-node://%s", signed.URIs, nodeID)
	}
	foundClientAuth := false
	for _, eku := range signed.ExtKeyUsage {
		if eku == x509.ExtKeyUsageClientAuth {
			foundClientAuth = true
		}
	}
	if !foundClientAuth {
		t.Fatalf("missing ClientAuth EKU")
	}
	if signed.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Fatalf("missing DigitalSignature KU")
	}
	if signed.IsCA {
		t.Fatalf("signed cert must not be CA")
	}
	if !signed.NotBefore.Before(time.Now()) {
		t.Fatalf("NotBefore = %s should be in the past", signed.NotBefore)
	}
	if signed.NotAfter.Sub(time.Now()) > time.Hour+time.Minute {
		t.Fatalf("NotAfter %s too far", signed.NotAfter)
	}
}

func TestSignCSR_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	if _, err := ca.SignCSR([]byte("not a pem"), "node-1", time.Hour); err == nil {
		t.Fatalf("expected error for bad PEM")
	}
}

func TestSignCSR_BadSignature(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}

	csrPEM, _ := mustGenCSR(t, "node-1")
	block, _ := pem.Decode(csrPEM)
	tampered := make([]byte, len(block.Bytes))
	copy(tampered, block.Bytes)
	// flip a byte in the signature region (last bytes of the DER)
	tampered[len(tampered)-1] ^= 0xFF
	tamperedPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: tampered})

	_, err = ca.SignCSR(tamperedPEM, "node-1", time.Hour)
	if err == nil {
		t.Fatalf("expected signature verification failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "csr") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignCSR_VerifiesAgainstClientCAPool(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	csrPEM, _ := mustGenCSR(t, "ignored")
	signedPEM, err := ca.SignCSR(csrPEM, "node-xyz", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	block, _ := pem.Decode(signedPEM)
	signed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	pool := ca.ClientCAPool()

	// Simulate what tls.Server does for client auth verification.
	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if _, err := signed.Verify(opts); err != nil {
		t.Fatalf("verify against ClientCAPool: %v", err)
	}

	// Sanity: a cert from a different CA must not verify.
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rogue"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	otherDER, err := x509.CreateCertificate(rand.Reader, otherTmpl, otherTmpl, &other.PublicKey, other)
	if err != nil {
		t.Fatalf("rogue cert: %v", err)
	}
	rogue, _ := x509.ParseCertificate(otherDER)
	if _, err := rogue.Verify(opts); err == nil {
		t.Fatalf("rogue cert verified against pool — pool not isolated")
	}

	// Make sure the pool is suitable for tls.Config (compile-time use only).
	_ = &tls.Config{ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
}

func TestCertPEM_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	a := ca.CertPEM()
	if len(a) == 0 {
		t.Fatalf("empty cert pem")
	}
	a[0] ^= 0xFF
	b := ca.CertPEM()
	if a[0] == b[0] {
		t.Fatalf("CertPEM returned shared buffer")
	}
}
