package serverapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"pulse/internal/inbounds"
	"pulse/internal/jobs"
	"pulse/internal/nodes"
	"pulse/internal/outbounds"
	"pulse/internal/users"
)

type systemAPI struct {
	users         users.Store
	nodes         nodes.Store
	inboundStore  inbounds.InboundStore
	outboundStore outbounds.Store
	base          *API
	applyOpts     jobs.ApplyOptions
}

// RegisterSystemAPIWithInbounds 注册 system API（含 inboundStore，用于流量同步）。
func RegisterSystemAPIWithInbounds(mux *http.ServeMux, usersStore users.Store, nodesStore nodes.Store, ibStore inbounds.InboundStore, applyOpts jobs.ApplyOptions) {
	base := New(nodesStore)
	api := &systemAPI{
		users:         usersStore,
		nodes:         nodesStore,
		inboundStore:  ibStore,
		outboundStore: nil, // 调用方可通过 RegisterSystemAPIWithInboundsAndOutbounds 传入
		base:          base,
		applyOpts:     applyOpts,
	}
	mux.HandleFunc("/v1/system/sync-usage", api.handleSyncUsage)
}

func (a *systemAPI) handleSyncUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if a.inboundStore == nil {
		internalError(w, r, errors.New("inbound store not configured"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	result, err := jobs.SyncUsage(ctx, a.users, a.nodes, a.inboundStore, a.base.Dial, a.applyOpts, a.outboundStore)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
