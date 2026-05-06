package serverapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"pulse/internal/enrolltokens"
	"pulse/internal/nodes"
)

// ServerURLResolver 决定写入 install/manual 命令中的控制面 URL。
// 返回值应是无尾斜杠的形如 "https://panel.example.com" 的字符串。
type ServerURLResolver func(r *http.Request) string

const (
	defaultEnrollTokenTTL = time.Hour
	maxEnrollTokenTTL     = 24 * time.Hour
	installScriptURL      = "https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh"
)

type enrollTokenRequest struct {
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

type enrollTokenResponse struct {
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expires_at"`
	InstallCommand string    `json:"install_command"`
	ManualCommand  string    `json:"manual_command"`
	ServerURL      string    `json:"server_url"`
}

// RegisterNodeEnrollTokenEndpoint 注册 POST /v1/nodes/{nodeID}/enroll-token，
// 由管理员调用以生成 pulse-node 安装期使用的一次性 enrollment token。
// 该路径必须挂在已加 admin 鉴权的 mux 上。
//
// resolveServerURL 用于决定回写到 install/manual 命令中的控制面 URL；
// 优先级：URL 查询参数 ?server=<url> > resolveServerURL(r) > 由请求 Host 推断。
func RegisterNodeEnrollTokenEndpoint(mux *http.ServeMux, nodeStore nodes.Store, tokens enrolltokens.Store, resolveServerURL ServerURLResolver) {
	mux.HandleFunc("POST /v1/nodes/{nodeID}/enroll-token", func(w http.ResponseWriter, r *http.Request) {
		nodeID := strings.TrimSpace(r.PathValue("nodeID"))
		if nodeID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
			return
		}

		if _, err := nodeStore.Get(nodeID); err != nil {
			if errors.Is(err, nodes.ErrNodeNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "node not found"})
				return
			}
			internalError(w, r, fmt.Errorf("nodes.Get(%q): %w", nodeID, err))
			return
		}

		// 请求体可选；为空时使用默认 TTL。
		var req enrollTokenRequest
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(strings.TrimSpace(string(body))) > 0 {
				if err := json.Unmarshal(body, &req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
					return
				}
			}
		}

		ttl := defaultEnrollTokenTTL
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}
		if ttl > maxEnrollTokenTTL {
			ttl = maxEnrollTokenTTL
		}

		token, err := newEnrollTokenString()
		if err != nil {
			internalError(w, r, fmt.Errorf("generate enroll token: %w", err))
			return
		}

		now := time.Now().UTC()
		expiresAt := now.Add(ttl)
		record := enrolltokens.Token{
			Token:     token,
			NodeID:    nodeID,
			ExpiresAt: expiresAt,
			CreatedAt: now,
		}
		if err := tokens.Insert(r.Context(), record); err != nil {
			internalError(w, r, fmt.Errorf("insert enroll token: %w", err))
			return
		}

		serverURL := strings.TrimSpace(r.URL.Query().Get("server"))
		if serverURL == "" && resolveServerURL != nil {
			serverURL = strings.TrimSpace(resolveServerURL(r))
		}
		if serverURL == "" {
			serverURL = inferServerURL(r)
		}
		serverURL = strings.TrimRight(serverURL, "/")

		log.Printf("node-enroll-token: issued node_id=%q ttl=%s token_prefix=%s", nodeID, ttl, tokenPrefix(token))

		writeJSON(w, http.StatusOK, enrollTokenResponse{
			Token:          token,
			ExpiresAt:      expiresAt,
			ServerURL:      serverURL,
			InstallCommand: buildInstallCommand(serverURL, nodeID, token),
			ManualCommand:  buildManualCommand(serverURL, nodeID, token),
		})
	})
}

func newEnrollTokenString() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func inferServerURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func buildInstallCommand(serverURL, nodeID, token string) string {
	return fmt.Sprintf("bash <(curl -fsSL %s) node --server=%s --node-id=%s --token=%s",
		installScriptURL, serverURL, nodeID, token)
}

func buildManualCommand(serverURL, nodeID, token string) string {
	return fmt.Sprintf("pulse-node enroll --server=%s --node-id=%s --token=%s",
		serverURL, nodeID, token)
}
