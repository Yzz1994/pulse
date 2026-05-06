package nodeenroll

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pulse/internal/cert"
)

func newTestCA(t *testing.T) *cert.NodeCA {
	t.Helper()
	dir := t.TempDir()
	ca, err := cert.LoadOrCreateNodeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	return ca
}

type captured struct {
	NodeID string
	Token  string
	CSRPem string
}

func newMockServer(t *testing.T, ca *cert.NodeCA, status int, errMsg string, badCert bool, cap *captured) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc("/v1/node-enroll", func(w http.ResponseWriter, r *http.Request) {
		var body enrollRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if cap != nil {
			cap.NodeID = body.NodeID
			cap.Token = body.Token
			cap.CSRPem = body.CSRPem
		}
		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": errMsg})
			return
		}
		certPEM, err := ca.SignCSR([]byte(body.CSRPem), body.NodeID, time.Hour)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respCert := string(certPEM)
		if badCert {
			respCert = "not a pem"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(enrollResponseBody{
			CertPEM:   respCert,
			CACertPEM: string(ca.CertPEM()),
			GRPCURL:   "https://grpc.example.com:9443",
		})
	})
	return httptest.NewTLSServer(handler)
}

func insecureClient() *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
}

func TestRun_Success(t *testing.T) {
	ca := newTestCA(t)
	cap := &captured{}
	srv := newMockServer(t, ca, http.StatusOK, "", false, cap)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "nested", "etc")
	res, err := Run(context.Background(), Request{
		ServerURL:  srv.URL,
		NodeID:     "node-abc",
		Token:      "tok-123",
		OutDir:     out,
		HTTPClient: insecureClient(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.GRPCURL != "https://grpc.example.com:9443" {
		t.Errorf("GRPCURL = %q", res.GRPCURL)
	}
	if cap.NodeID != "node-abc" || cap.Token != "tok-123" {
		t.Errorf("captured = %+v", cap)
	}
	if !strings.Contains(cap.CSRPem, "CERTIFICATE REQUEST") {
		t.Errorf("CSR not propagated: %q", cap.CSRPem)
	}

	// Verify file existence + permissions (skip mode check on windows).
	checks := []struct {
		path string
		perm os.FileMode
	}{
		{res.CertPath, 0o644},
		{res.KeyPath, 0o600},
		{res.CAPath, 0o644},
	}
	for _, c := range checks {
		st, err := os.Stat(c.path)
		if err != nil {
			t.Fatalf("stat %s: %v", c.path, err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != c.perm {
			t.Errorf("%s perm = %o want %o", c.path, st.Mode().Perm(), c.perm)
		}
	}

	// OutDir should have been created.
	if _, err := os.Stat(out); err != nil {
		t.Errorf("out dir not created: %v", err)
	}
}

func TestRun_Unauthorized(t *testing.T) {
	ca := newTestCA(t)
	srv := newMockServer(t, ca, http.StatusUnauthorized, "invalid or expired token", false, nil)
	defer srv.Close()

	out := t.TempDir()
	_, err := Run(context.Background(), Request{
		ServerURL:  srv.URL,
		NodeID:     "node-abc",
		Token:      "bad",
		OutDir:     out,
		HTTPClient: insecureClient(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid or expired token") {
		t.Errorf("error = %v", err)
	}
	// Files must not be written.
	for _, f := range []string{"node_cert.pem", "node_key.pem", "node_ca.pem"} {
		if _, err := os.Stat(filepath.Join(out, f)); err == nil {
			t.Errorf("%s should not exist", f)
		}
	}
}

func TestRun_BadCertPEM(t *testing.T) {
	ca := newTestCA(t)
	srv := newMockServer(t, ca, http.StatusOK, "", true, nil)
	defer srv.Close()

	out := t.TempDir()
	_, err := Run(context.Background(), Request{
		ServerURL:  srv.URL,
		NodeID:     "node-abc",
		Token:      "tok",
		OutDir:     out,
		HTTPClient: insecureClient(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "PEM") {
		t.Errorf("error = %v", err)
	}
	for _, f := range []string{"node_cert.pem", "node_key.pem", "node_ca.pem"} {
		if _, err := os.Stat(filepath.Join(out, f)); err == nil {
			t.Errorf("%s should not exist on failure", f)
		}
	}
}

func TestRun_MissingFields(t *testing.T) {
	cases := []Request{
		{ServerURL: "", NodeID: "n", Token: "t", OutDir: "."},
		{ServerURL: "https://x", NodeID: "", Token: "t", OutDir: "."},
		{ServerURL: "https://x", NodeID: "n", Token: "", OutDir: "."},
		{ServerURL: "https://x", NodeID: "n", Token: "t", OutDir: ""},
	}
	for i, c := range cases {
		if _, err := Run(context.Background(), c); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
