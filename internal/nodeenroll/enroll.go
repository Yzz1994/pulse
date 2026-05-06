// Package nodeenroll implements the node-side enrollment flow against
// the control plane's POST /v1/node-enroll endpoint.
//
// The flow:
//  1. generate an RSA 4096 keypair locally,
//  2. build a CSR with CommonName = NodeID,
//  3. POST {csr_pem, node_id, enroll_token} to the control plane,
//  4. persist the returned node certificate, private key and CA bundle.
package nodeenroll

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Request describes the inputs for an enrollment run.
type Request struct {
	ServerURL  string
	NodeID     string
	Token      string
	OutDir     string
	HTTPClient *http.Client
	// Insecure skips server TLS verification. Required for the very first
	// enrollment because the node has no CA bundle yet.
	// TODO: replace with fingerprint pinning (--server-fingerprint).
	Insecure bool
}

// Result describes the outputs of a successful enrollment.
type Result struct {
	CertPath string
	KeyPath  string
	CAPath   string
	GRPCURL  string
}

type enrollRequestBody struct {
	NodeID string `json:"node_id"`
	CSRPem string `json:"csr_pem"`
	Token  string `json:"enroll_token"`
}

type enrollResponseBody struct {
	CertPEM   string `json:"cert_pem"`
	CACertPEM string `json:"ca_cert_pem"`
	GRPCURL   string `json:"grpc_url"`
}

type errorResponseBody struct {
	Error string `json:"error"`
}

// Run executes the enrollment flow. On success it writes node_cert.pem,
// node_key.pem and node_ca.pem under req.OutDir.
func Run(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.ServerURL) == "" {
		return Result{}, fmt.Errorf("server URL is required")
	}
	if strings.TrimSpace(req.NodeID) == "" {
		return Result{}, fmt.Errorf("node id is required")
	}
	if strings.TrimSpace(req.Token) == "" {
		return Result{}, fmt.Errorf("enroll token is required")
	}
	if strings.TrimSpace(req.OutDir) == "" {
		return Result{}, fmt.Errorf("output directory is required")
	}

	if err := os.MkdirAll(req.OutDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return Result{}, fmt.Errorf("generate RSA key: %w", err)
	}

	csrTemplate := &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: req.NodeID},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, priv)
	if err != nil {
		return Result{}, fmt.Errorf("create CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return Result{}, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	client := req.HTTPClient
	if client == nil {
		// TODO: pin server certificate via --server-fingerprint instead of
		// trusting any TLS cert when Insecure is true.
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: req.Insecure}, // #nosec G402
		}
		client = &http.Client{Timeout: 30 * time.Second, Transport: tr}
	}

	body, err := json.Marshal(enrollRequestBody{
		NodeID: req.NodeID,
		CSRPem: string(csrPEM),
		Token:  req.Token,
	})
	if err != nil {
		return Result{}, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimRight(req.ServerURL, "/") + "/v1/node-enroll"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("post enroll request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := extractErrorMessage(respBytes)
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return Result{}, fmt.Errorf("server rejected enrollment (400): %s", msg)
		case http.StatusUnauthorized:
			return Result{}, fmt.Errorf("server rejected enrollment (401): %s", msg)
		default:
			return Result{}, fmt.Errorf("enroll failed: status=%d body=%s", resp.StatusCode, msg)
		}
	}

	var parsed enrollResponseBody
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return Result{}, fmt.Errorf("decode response: %w", err)
	}
	if err := validatePEM(parsed.CertPEM, "CERTIFICATE"); err != nil {
		return Result{}, fmt.Errorf("server cert: %w", err)
	}
	if err := validatePEM(parsed.CACertPEM, "CERTIFICATE"); err != nil {
		return Result{}, fmt.Errorf("server CA: %w", err)
	}

	certPath := filepath.Join(req.OutDir, "node_cert.pem")
	keyPath := filepath.Join(req.OutDir, "node_key.pem")
	caPath := filepath.Join(req.OutDir, "node_ca.pem")

	if err := os.WriteFile(certPath, []byte(parsed.CertPEM), 0o644); err != nil {
		return Result{}, fmt.Errorf("write node_cert.pem: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return Result{}, fmt.Errorf("write node_key.pem: %w", err)
	}
	if err := os.WriteFile(caPath, []byte(parsed.CACertPEM), 0o644); err != nil {
		return Result{}, fmt.Errorf("write node_ca.pem: %w", err)
	}

	return Result{
		CertPath: certPath,
		KeyPath:  keyPath,
		CAPath:   caPath,
		GRPCURL:  parsed.GRPCURL,
	}, nil
}

func validatePEM(s, wantType string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("empty PEM")
	}
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return fmt.Errorf("invalid PEM block")
	}
	if block.Type != wantType {
		return fmt.Errorf("unexpected PEM type %q, want %q", block.Type, wantType)
	}
	return nil
}

func extractErrorMessage(body []byte) string {
	var er errorResponseBody
	if err := json.Unmarshal(body, &er); err == nil && er.Error != "" {
		return er.Error
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty body)"
	}
	return s
}
