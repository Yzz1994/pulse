package serverapi

import (
	"net/http"
	"time"

	"pulse/internal/accesslogs"
)

func (a *API) handleAuditAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.accessLogStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": []any{}})
		return
	}
	q := r.URL.Query()
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now()
	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	if s := q.Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			until = t
		}
	}
	result, err := a.accessLogStore.ListUserAnalysis(since, until)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if result == nil {
		result = []accesslogs.UserAnalysis{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

func (a *API) handleAuditCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.accessLogStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0})
		return
	}
	n, err := a.accessLogStore.Count()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

func (a *API) handleAuditUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.accessLogStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": []string{}})
		return
	}
	users, err := a.accessLogStore.ListDistinctUsers(time.Now().Add(-24 * time.Hour))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if users == nil {
		users = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *API) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if a.accessLogStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "audit log store not configured"})
		return
	}

	q := r.URL.Query()
	params := accesslogs.ListParams{
		NodeID:   q.Get("node_id"),
		Username: q.Get("username"),
		Limit:    500,
	}

	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			params.Since = t
		}
	}
	if s := q.Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			params.Until = t
		}
	}

	entries, err := a.accessLogStore.List(params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []accesslogs.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
