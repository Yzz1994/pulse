package serverapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"pulse/internal/cfdomain"
	"pulse/internal/cloudflarex"
	"pulse/internal/idgen"
	"pulse/internal/nodedomain"
	"pulse/internal/nodes"
)

type nodeDomainAPI struct {
	ndStore    nodedomain.Store
	cfStore    cfdomain.Store
	nodesStore nodes.Store
}

// RegisterNodeDomainAPI 注册节点域名相关路由。
//   GET    /v1/node-domains              列出（支持 ?cf_domain_id= 过滤）
//   POST   /v1/node-domains/sync         从 CF 同步记录到本地库
//   PUT    /v1/node-domains/{id}         更新节点分配
//   DELETE /v1/node-domains/{id}         删除
func RegisterNodeDomainAPI(mux *http.ServeMux, ndStore nodedomain.Store, cfStore cfdomain.Store, nodesStore nodes.Store) {
	a := &nodeDomainAPI{ndStore: ndStore, cfStore: cfStore, nodesStore: nodesStore}
	mux.HandleFunc("/v1/node-domains", a.handleNodeDomains)
	mux.HandleFunc("/v1/node-domains/", a.handleNodeDomainRoutes)
}

// handleNodeDomains 处理 GET /v1/node-domains。
func (a *nodeDomainAPI) handleNodeDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	cfDomainID := r.URL.Query().Get("cf_domain_id")
	var (
		list []nodedomain.NodeDomain
		err  error
	)
	if cfDomainID != "" {
		list, err = a.ndStore.ListByCFDomain(cfDomainID)
	} else {
		list, err = a.ndStore.List()
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if list == nil {
		list = []nodedomain.NodeDomain{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"node_domains": list})
}

// handleNodeDomainRoutes 处理 /v1/node-domains/ 下的子路由。
func (a *nodeDomainAPI) handleNodeDomainRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/node-domains/")
	path = strings.TrimSuffix(path, "/")

	// POST /v1/node-domains/sync — 同步入口
	if path == "sync" {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		a.handleSync(w, r)
		return
	}

	// PUT/DELETE /v1/node-domains/{id}
	id := path
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "id is required"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		var body struct {
			NodeID string `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		nd, err := a.ndStore.UpdateNodeID(id, body.NodeID)
		if errors.Is(err, nodedomain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, nd)

	case http.MethodDelete:
		if err := a.ndStore.Delete(id); err != nil {
			if errors.Is(err, nodedomain.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
				return
			}
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})

	default:
		writeMethodNotAllowed(w, http.MethodPut+", "+http.MethodDelete)
	}
}

// handleSync 从 CF 同步 DNS 记录到本地节点域名表。
// POST /v1/node-domains/sync
// Body: { "cf_domain_id": "xxx", "node_id": "xxx" }  node_id 可选，留空则按 IP 自动匹配
func (a *nodeDomainAPI) handleSync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CFDomainID string `json:"cf_domain_id"`
		NodeID     string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CFDomainID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cf_domain_id is required"})
		return
	}

	domain, err := a.cfStore.GetCFDomain(body.CFDomainID)
	if err != nil {
		if errors.Is(err, cfdomain.ErrCFDomainNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "cf domain not found"})
			return
		}
		internalError(w, r, err)
		return
	}

	ctx := r.Context()
	client := cloudflarex.NewClient(domain.CFToken)

	// 拉取全部 DNS 记录
	allRecords, err := client.ListDNSRecords(ctx, domain.ZoneID, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// 获取所有节点，建立 IP → nodeID 映射（用于自动匹配）
	nodeList, err := a.nodesStore.List()
	if err != nil {
		internalError(w, r, err)
		return
	}
	nodeIPMap := buildNodeIPMap(nodeList)

	synced := make([]nodedomain.NodeDomain, 0, len(allRecords))
	for _, rec := range allRecords {
		// 只同步 A、AAAA、CNAME
		switch rec.Type {
		case "A", "AAAA", "CNAME":
		default:
			continue
		}

		assignedNodeID := body.NodeID
		if assignedNodeID == "" && (rec.Type == "A" || rec.Type == "AAAA") {
			// 按 IP 自动匹配节点
			assignedNodeID = nodeIPMap[rec.Content]
		}

		nd, err := a.ndStore.Upsert(nodedomain.NodeDomain{
			ID:         idgen.NextString(),
			NodeID:     assignedNodeID,
			CFDomainID: body.CFDomainID,
			FQDN:       rec.Name,
			RecordType: rec.Type,
			Content:    rec.Content,
			Proxied:    rec.Proxied,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
		synced = append(synced, nd)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"synced":       len(synced),
		"node_domains": synced,
	})
}

// buildNodeIPMap 将节点公网 IP → node ID 建立索引。
// 优先使用 IPOverride，否则从 BaseURL 解析主机名。
func buildNodeIPMap(nodeList []nodes.Node) map[string]string {
	m := make(map[string]string, len(nodeList))
	for _, n := range nodeList {
		ip := n.IPOverride
		if ip == "" {
			ip = extractURLHost(n.BaseURL)
		}
		if ip != "" {
			m[ip] = n.ID
		}
	}
	return m
}

// extractURLHost 从形如 https://1.2.3.4:8081 的 URL 中提取主机部分。
func extractURLHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
