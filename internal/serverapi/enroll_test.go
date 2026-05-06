package serverapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pulse/internal/cert"
	"pulse/internal/enrolltokens"
)

func newTestCA(t *testing.T) *cert.NodeCA {
	t.Helper()
	dir := t.TempDir()
	ca, err := cert.LoadOrCreateNodeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("create test CA: %v", err)
	}
	return ca
}

func newTestCSR(t *testing.T, cn string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	tpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}
	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, key)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func doEnroll(t *testing.T, mux *http.ServeMux, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	switch v := body.(type) {
	case string:
		r = httptest.NewRequest(http.MethodPost, "/v1/node-enroll", strings.NewReader(v))
	default:
		buf, _ := json.Marshal(v)
		r = httptest.NewRequest(http.MethodPost, "/v1/node-enroll", bytes.NewReader(buf))
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestEnrollSuccess(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	ctx := context.Background()
	tok := enrolltokens.Token{Token: "tk-ok-1234567890abcdef", NodeID: "node-1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Insert(ctx, tok); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "https://example.com:8082")

	w := doEnroll(t, mux, enrollRequest{NodeID: "node-1", CSRPem: newTestCSR(t, "node-1"), Token: tok.Token})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.GRPCURL != "https://example.com:8082" {
		t.Errorf("grpc_url=%q", resp.GRPCURL)
	}
	if resp.CACertPEM == "" {
		t.Error("missing ca_cert_pem")
	}

	// Verify the issued cert chains to the CA.
	block, _ := pem.Decode([]byte(resp.CertPEM))
	if block == nil {
		t.Fatal("decode cert_pem failed")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     ca.ClientCAPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("verify cert: %v", err)
	}
	if leaf.Subject.CommonName != "node-1" {
		t.Errorf("CN=%q", leaf.Subject.CommonName)
	}
}

func TestEnrollTokenNotFound(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "https://x")

	w := doEnroll(t, mux, enrollRequest{NodeID: "n", CSRPem: newTestCSR(t, "n"), Token: "missing"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEnrollTokenAlreadyConsumed(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	ctx := context.Background()
	tok := enrolltokens.Token{Token: "tk-used", NodeID: "n", ExpiresAt: time.Now().Add(time.Hour)}
	_ = store.Insert(ctx, tok)
	if _, err := store.Consume(ctx, tok.Token); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "")
	w := doEnroll(t, mux, enrollRequest{NodeID: "n", CSRPem: newTestCSR(t, "n"), Token: tok.Token})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEnrollTokenExpired(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	ctx := context.Background()
	tok := enrolltokens.Token{Token: "tk-exp", NodeID: "n", ExpiresAt: time.Now().Add(-time.Minute)}
	_ = store.Insert(ctx, tok)

	// Sanity: confirm Consume reports expired.
	if _, err := store.Consume(ctx, tok.Token); !errors.Is(err, enrolltokens.ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
	// Re-insert (Consume above doesn't delete) - actually Consume on expired doesn't mark consumed in memory store.
	// But to keep this test self-contained, build a fresh store:
	store = enrolltokens.NewMemoryStore()
	_ = store.Insert(ctx, tok)

	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "")
	w := doEnroll(t, mux, enrollRequest{NodeID: "n", CSRPem: newTestCSR(t, "n"), Token: tok.Token})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEnrollNodeIDMismatch(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	ctx := context.Background()
	tok := enrolltokens.Token{Token: "tk-mm", NodeID: "node-A", ExpiresAt: time.Now().Add(time.Hour)}
	_ = store.Insert(ctx, tok)

	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "")
	w := doEnroll(t, mux, enrollRequest{NodeID: "node-B", CSRPem: newTestCSR(t, "node-B"), Token: tok.Token})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestEnrollInvalidCSR(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	ctx := context.Background()
	tok := enrolltokens.Token{Token: "tk-bad-csr", NodeID: "n", ExpiresAt: time.Now().Add(time.Hour)}
	_ = store.Insert(ctx, tok)

	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "")
	w := doEnroll(t, mux, enrollRequest{NodeID: "n", CSRPem: "not a csr", Token: tok.Token})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEnrollMissingFields(t *testing.T) {
	ca := newTestCA(t)
	store := enrolltokens.NewMemoryStore()
	mux := http.NewServeMux()
	RegisterEnrollEndpoint(mux, ca, store, "")

	cases := []enrollRequest{
		{NodeID: "", CSRPem: "x", Token: "y"},
		{NodeID: "n", CSRPem: "", Token: "y"},
		{NodeID: "n", CSRPem: "x", Token: ""},
	}
	for i, c := range cases {
		w := doEnroll(t, mux, c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d: status=%d", i, w.Code)
		}
	}

	// Invalid JSON body
	w := doEnroll(t, mux, "{not json")
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid json: status=%d", w.Code)
	}
}
