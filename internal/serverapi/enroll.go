package serverapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"pulse/internal/cert"
	"pulse/internal/enrolltokens"
)

// 包级原子计数器：节点入网（enroll）成功/失败次数。
// 用于 /v1/system/nodehub/metrics 的可观测性合并输出。
var (
	enrollSuccessTotal atomic.Uint64
	enrollFailureTotal atomic.Uint64
)

// EnrollMetrics 返回 enroll 端点的累计计数。
// 合并到 nodehub Snapshot JSON 输出中（避免引入第二个端点）。
func EnrollMetrics() (success, failure uint64) {
	return enrollSuccessTotal.Load(), enrollFailureTotal.Load()
}

type enrollRequest struct {
	NodeID string `json:"node_id"`
	CSRPem string `json:"csr_pem"`
	Token  string `json:"enroll_token"`
}

type enrollResponse struct {
	CertPEM   string `json:"cert_pem"`
	CACertPEM string `json:"ca_cert_pem"`
	GRPCURL   string `json:"grpc_url"`
}

const enrollCertTTL = 365 * 24 * time.Hour

// RegisterEnrollEndpoint 注册 POST /v1/node-enroll 端点。
// 该端点不走 admin 鉴权：节点首次接入时还没有客户端证书，token 本身即凭据。
func RegisterEnrollEndpoint(mux *http.ServeMux, ca *cert.NodeCA, tokens enrolltokens.Store, grpcURL string) {
	mux.HandleFunc("POST /v1/node-enroll", func(w http.ResponseWriter, r *http.Request) {
		var req enrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("node-enroll: decode body: %v", err)
			enrollFailureTotal.Add(1)
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
			return
		}
		if req.NodeID == "" || req.CSRPem == "" || req.Token == "" {
			log.Printf("node-enroll: missing fields (node_id_present=%v csr_present=%v token_present=%v)",
				req.NodeID != "", req.CSRPem != "", req.Token != "")
			enrollFailureTotal.Add(1)
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id, csr_pem and enroll_token are required"})
			return
		}

		consumed, err := tokens.Consume(r.Context(), req.Token)
		if err != nil {
			switch {
			case errors.Is(err, enrolltokens.ErrNotFound),
				errors.Is(err, enrolltokens.ErrAlreadyConsumed),
				errors.Is(err, enrolltokens.ErrExpired):
				log.Printf("node-enroll: token rejected for node_id=%q: %v", req.NodeID, err)
				enrollFailureTotal.Add(1)
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired token"})
			default:
				log.Printf("node-enroll: consume token error for node_id=%q: %v", req.NodeID, err)
				enrollFailureTotal.Add(1)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
			}
			return
		}

		if consumed.NodeID != req.NodeID {
			log.Printf("node-enroll: node_id mismatch (request=%q token-bound=%q)", req.NodeID, consumed.NodeID)
			enrollFailureTotal.Add(1)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired token"})
			return
		}

		certPEM, err := ca.SignCSR([]byte(req.CSRPem), req.NodeID, enrollCertTTL)
		if err != nil {
			log.Printf("node-enroll: sign CSR for node_id=%q: %v", req.NodeID, err)
			enrollFailureTotal.Add(1)
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid csr"})
			return
		}

		log.Printf("node-enroll: issued certificate node_id=%q token_prefix=%s", req.NodeID, tokenPrefix(req.Token))
		enrollSuccessTotal.Add(1)

		writeJSON(w, http.StatusOK, enrollResponse{
			CertPEM:   string(certPEM),
			CACertPEM: string(ca.CertPEM()),
			GRPCURL:   grpcURL,
		})
	})
}

func tokenPrefix(t string) string {
	t = strings.TrimSpace(t)
	if len(t) <= 8 {
		return t
	}
	return t[:8]
}
