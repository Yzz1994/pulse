package serverapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"pulse/internal/idgen"
	"pulse/internal/users"
)

type setupHandler struct {
	users              users.Store
	invalidateSessions func() error // 改密后使所有管理员 session 失效
}

// RegisterSetupAPI 注册 setup 相关公开端点（不需要管理员认证）。
func RegisterSetupAPI(mux *http.ServeMux, us users.Store) {
	h := &setupHandler{users: us}
	mux.HandleFunc("GET /v1/auth/setup-status", h.handleSetupStatus)
	mux.HandleFunc("POST /v1/auth/setup", h.handleSetup)
}

// RegisterAuthCredentialsAPI 注册改密端点（需要已认证，注册到 protectedV1 上）。
// invalidateSessions 在改密成功后被调用以清除所有旧 session。
func RegisterAuthCredentialsAPI(mux *http.ServeMux, us users.Store, invalidateSessions func() error) {
	h := &setupHandler{users: us, invalidateSessions: invalidateSessions}
	mux.HandleFunc("PUT /v1/auth/credentials", h.handleChangeCredentials)
}

// handleSetupStatus 返回系统是否需要初始化（尚无管理员）。
func (h *setupHandler) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	_, err := h.users.GetAdminUser()
	needsSetup := errors.Is(err, users.ErrUserNotFound)
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": needsSetup})
}

// handleSetup 创建第一个管理员用户（无需认证，已有管理员则拒绝）。
func (h *setupHandler) handleSetup(w http.ResponseWriter, r *http.Request) {
	// 已有 admin 则拒绝
	if _, err := h.users.GetAdminUser(); err == nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin already exists"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}
	if len(req.Password) > 72 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password too long"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "hash failed"})
		return
	}
	// 生成 sub_token
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token generation failed"})
		return
	}
	subToken := hex.EncodeToString(buf)

	user := users.User{
		ID:                     idgen.NextString(),
		Username:               req.Username,
		Status:                 users.StatusActive,
		DataLimitResetStrategy: users.ResetStrategyNoReset,
		SubToken:               subToken,
		UUID:                   randomUUID(),
		Secret:                 randomToken(16),
	}
	if _, err := h.users.UpsertUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := h.users.SetPassword(user.ID, string(hash)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := h.users.SetIsAdmin(user.ID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleChangeCredentials 修改管理员用户名和/或密码（需要提供旧密码验证）。
func (h *setupHandler) handleChangeCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	admin, err := h.users.GetAdminUser()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "admin not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.OldPassword)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid current password"})
		return
	}
	changed := false
	if req.NewPassword != "" {
		if len(req.NewPassword) > 72 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password too long"})
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "hash failed"})
			return
		}
		if err := h.users.SetPassword(admin.ID, string(hash)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		changed = true
	}
	if req.Username != "" {
		req.Username = strings.TrimSpace(req.Username)
		if err := h.users.UpdateUsername(admin.ID, req.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		changed = true
	}
	if changed && h.invalidateSessions != nil {
		_ = h.invalidateSessions()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
