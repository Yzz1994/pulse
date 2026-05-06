package serverapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"pulse/internal/idgen"
	"pulse/internal/plans"
)

type planAPI struct {
	store plans.Store
}

func RegisterPlansAPI(mux *http.ServeMux, store plans.Store) {
	a := &planAPI{store: store}
	mux.HandleFunc("/v1/plans", a.handlePlans)
	mux.HandleFunc("/v1/plans/", a.handlePlanRoutes)
}

func (a *planAPI) handlePlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.store.ListPlans()
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"plans": items})
	case http.MethodPost:
		var req plans.Plan
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		if req.ID == "" {
			req.ID = idgen.NextString()
		}
		plan, err := a.store.UpsertPlan(req)
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, plan)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (a *planAPI) handlePlanRoutes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/plans/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "plan id is required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		plan, err := a.store.GetPlan(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "plan not found"})
			return
		}
		writeJSON(w, http.StatusOK, plan)
	case http.MethodPut:
		existing, err := a.store.GetPlan(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "plan not found"})
			return
		}
		var req plans.Plan
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
			return
		}
		req.ID = existing.ID
		plan, err := a.store.UpsertPlan(req)
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	case http.MethodDelete:
		if err := a.store.DeletePlan(id); err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}
