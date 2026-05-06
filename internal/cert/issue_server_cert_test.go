package cert

import (
	"crypto/x509"
	"path/filepath"
	"testing"
	"time"
)

func TestNodeCA_IssueServerCert(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}

	tlsCert, err := ca.IssueServerCert("pulse-grpc-server", []string{"localhost", "127.0.0.1"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueServerCert: %v", err)
	}
	if tlsCert.Leaf == nil {
		t.Fatal("Leaf is nil")
	}
	if tlsCert.PrivateKey == nil {
		t.Fatal("PrivateKey is nil")
	}
	if got := tlsCert.Leaf.Subject.CommonName; got != "pulse-grpc-server" {
		t.Fatalf("CN = %q, want pulse-grpc-server", got)
	}
	if len(tlsCert.Leaf.DNSNames) != 1 || tlsCert.Leaf.DNSNames[0] != "localhost" {
		t.Fatalf("DNSNames = %v", tlsCert.Leaf.DNSNames)
	}
	if len(tlsCert.Leaf.IPAddresses) != 1 || tlsCert.Leaf.IPAddresses[0].String() != "127.0.0.1" {
		t.Fatalf("IPAddresses = %v", tlsCert.Leaf.IPAddresses)
	}
	// 验证由 CA 签发
	pool := ca.ClientCAPool() // CA cert pool（用作 root 也可，因是自签）
	_, err = tlsCert.Leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		t.Fatalf("verify against CA: %v", err)
	}
}

func TestNodeCA_IssueServerCert_Validations(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateNodeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	if _, err := ca.IssueServerCert("", []string{"localhost"}, time.Hour); err == nil {
		t.Fatal("expected error for empty CN")
	}
	if _, err := ca.IssueServerCert("x", nil, 0); err == nil {
		t.Fatal("expected error for zero ttl")
	}
}
