package serverapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pulse/internal/auditrules"
	"pulse/internal/idgen"
)

func (a *API) handleAuditRuleByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/audit/rules/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
		return
	}
	a.handleAuditRule(w, r, id)
}

func (a *API) handleAuditRules(w http.ResponseWriter, r *http.Request) {
	if a.auditRuleStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "audit rule store not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules, err := a.auditRuleStore.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if rules == nil {
			rules = []auditrules.Rule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
	case http.MethodPost:
		var req struct {
			Type  auditrules.RuleType `json:"type"`
			Value string              `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		req.Value = strings.TrimSpace(req.Value)
		if req.Value == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "value is required"})
			return
		}
		if req.Type != auditrules.RuleTypeDomainKeyword && req.Type != auditrules.RuleTypePort && req.Type != auditrules.RuleTypeIP {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid type"})
			return
		}
		rule := auditrules.Rule{
			ID:        idgen.NextString(),
			Type:      req.Type,
			Value:     req.Value,
			Enabled:   true,
			CreatedAt: time.Now(),
		}
		if err := a.auditRuleStore.Insert(rule); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rule)
	default:
		writeMethodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (a *API) handleAuditRule(w http.ResponseWriter, r *http.Request, id string) {
	if a.auditRuleStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "audit rule store not configured"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := a.auditRuleStore.Delete(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodPatch:
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if err := a.auditRuleStore.SetEnabled(id, req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeMethodNotAllowed(w, http.MethodDelete+", "+http.MethodPatch)
	}
}
