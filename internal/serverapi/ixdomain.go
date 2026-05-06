package serverapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pulse/internal/idgen"
	"pulse/internal/ixdomain"
)

type ixDomainAPI struct {
	store ixdomain.Store
}

// RegisterIXDomainAPI 注册 IX 中转域名管理相关的 API 路由。
func RegisterIXDomainAPI(mux *http.ServeMux, store ixdomain.Store) {
	a := &ixDomainAPI{store: store}
	mux.HandleFunc("/v1/ix-domains", a.handleIXDomains)
	mux.HandleFunc("/v1/ix-domains/", a.handleIXDomainByID)
}

// handleIXDomains 处理 GET（列表）和 POST（新增）。
func (a *ixDomainAPI) handleIXDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		domains, err := a.store.ListIXDomains()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
			return
		}
		if domains == nil {
			domains = []ixdomain.IXDomain{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ix_domains": domains})

	case http.MethodPost:
		var d ixdomain.IXDomain
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		if d.Name == "" || d.Domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and domain are required"})
			return
		}
		if d.ID == "" {
			d.ID = idgen.NextString()
		}
		saved, err := a.store.UpsertIXDomain(d)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusCreated, saved)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleIXDomainByID 处理 /v1/ix-domains/{id} 的 PUT 和 DELETE。
func (a *ixDomainAPI) handleIXDomainByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/ix-domains/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing id"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		var d ixdomain.IXDomain
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		d.ID = id
		if d.Name == "" || d.Domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and domain are required"})
			return
		}
		saved, err := a.store.UpsertIXDomain(d)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, saved)

	case http.MethodDelete:
		err := a.store.DeleteIXDomain(id)
		if errors.Is(err, ixdomain.ErrIXDomainNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}
