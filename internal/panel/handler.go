package panel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"pulse/internal/auth"
	"pulse/internal/geoip"
	"pulse/internal/idgen"
	"pulse/internal/inbounds"
	"pulse/internal/jobs"
	"pulse/internal/nodes"
	"pulse/internal/orders"
	"pulse/internal/outbounds"
	"pulse/internal/plans"
	"pulse/internal/users"
)

const cookieName = "pulse_token"

// Handler 面板 HTTP 处理器，持有所有依赖。
type Handler struct {
	auth          *auth.Manager
	discourse     *auth.DiscourseConfig
	userStore     users.Store
	nodeStore     nodes.Store
	ibStore       inbounds.InboundStore
	outboundStore outbounds.Store
	dial          jobs.NodeDialer
	applyOpts     jobs.ApplyOptions
	planStore     plans.Store
	orderStore    orders.Store
	geoDB         *geoip.DB
}

// New 创建 Handler 实例。
func New(
	authMgr *auth.Manager,
	userStore users.Store,
	nodeStore nodes.Store,
	ibStore inbounds.InboundStore,
	outboundStore outbounds.Store,
	dial jobs.NodeDialer,
	applyOpts jobs.ApplyOptions,
	discourse *auth.DiscourseConfig,
	geoDB *geoip.DB,
) *Handler {
	return &Handler{
		auth:          authMgr,
		discourse:     discourse,
		userStore:     userStore,
		nodeStore:     nodeStore,
		ibStore:       ibStore,
		outboundStore: outboundStore,
		dial:          dial,
		applyOpts:     applyOpts,
		geoDB:         geoDB,
	}
}

func (h *Handler) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/stat", h.statJSON)
	mux.HandleFunc("GET /v1/stat/stream", h.statStream)
	mux.HandleFunc("GET /auth/discourse", h.discourseRedirect)
	mux.HandleFunc("GET /auth/discourse/callback", h.discourseCallback)
	mux.HandleFunc("POST /api/me/reset-token", h.apiResetToken)
}

func (h *Handler) SetPaymentStores(ps plans.Store, os orders.Store) {
	h.planStore = ps
	h.orderStore = os
}

// ─── 辅助函数 ────────────────────────────────────────────────────────────────


func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateSSPassword(method string) string {
	size := 32
	if method == "2022-blake3-aes-128-gcm" {
		size = 16
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func panelRandomUUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("pulse-%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func panelRandomToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("pulse-secret-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}

// ─── 公开 API ─────────────────────────────────────────────────────────────────

type statCheck struct {
	Service   string `json:"service"`
	Unlocked  bool   `json:"unlocked"`
	Region    string `json:"region,omitempty"`
	Note      string `json:"note,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
}
type statSpeed struct {
	DownBps  int64  `json:"down_bps"`
	UpBps    int64  `json:"up_bps"`
	TestedAt string `json:"tested_at"`
}
type statUptimeBar struct {
	Label     string `json:"label"`
	OnlinePct int    `json:"online_pct"`
}
type statNodeGeo struct {
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	City        string `json:"city,omitempty"`
	ASNOrg      string `json:"asn_org,omitempty"`
}

type statTracerouteSnapshot struct {
	Direction string `json:"direction"`
	Target    string `json:"target"`
	Hops      string `json:"hops"`
	Quality   string `json:"quality"`
	CreatedAt string `json:"created_at"`
}

type statLatencySample struct {
	ISP       string `json:"isp"`
	RttMs     *int   `json:"rtt_ms"`
	SampledAt string `json:"sampled_at"`
}

type statNode struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Geo           *statNodeGeo             `json:"geo,omitempty"`
	DirectChecks  []statCheck              `json:"direct_checks"`
	ProxiedChecks []statCheck              `json:"proxied_checks"`
	SpeedTest     *statSpeed               `json:"speed_test,omitempty"`
	UptimeBars    []statUptimeBar          `json:"uptime_bars,omitempty"`
	Granularity   string                   `json:"granularity,omitempty"`
	OverallPct    int                      `json:"overall_pct"`
	HasData       bool                     `json:"has_data"`
	CheckedAt     string                   `json:"checked_at,omitempty"`
	UnlockedCount int                      `json:"unlocked_count"`
	TotalCount    int                      `json:"total_count"`
	UnlockPct     int                      `json:"unlock_pct"`
	Traceroutes   []statTracerouteSnapshot `json:"traceroutes,omitempty"`
	Latency       []statLatencySample      `json:"latency,omitempty"`
}
type statPayload struct {
	Nodes        []statNode `json:"nodes"`
	NodeCount    int        `json:"node_count"`
	UnlockRate   int        `json:"unlock_rate"`
	AvgUptimePct int        `json:"avg_uptime_pct"`
	ServiceCount int        `json:"service_count"`
	UpdatedAt    string     `json:"updated_at"`
}

func (h *Handler) buildStatPayload() (*statPayload, error) {
	allNodes, err := h.nodeStore.List()
	if err != nil {
		return nil, err
	}
	// 监控数据只包含启用的节点
	nodeList := make([]nodes.Node, 0, len(allNodes))
	for _, n := range allNodes {
		if !n.Disabled {
			nodeList = append(nodeList, n)
		}
	}
	checkMap, _ := h.nodeStore.ListAllNodeCheckResults()
	speedMap, _ := h.nodeStore.ListAllNodeSpeedTests()
	uptimeBarsMap, _ := h.nodeStore.ListNodeUptimeBars(3)
	tracerouteMap, _ := h.nodeStore.ListLatestTracerouteSnapshots()

	// 拉取过去 1 小时的延迟采样，按 node_id 分组
	nodeIDs := make([]string, 0, len(nodeList))
	for _, n := range nodeList {
		nodeIDs = append(nodeIDs, n.ID)
	}
	latencyMap := make(map[string][]statLatencySample)
	latencyNow := time.Now().UTC()
	if samples, err := h.nodeStore.QueryLatencySamples(nodeIDs, latencyNow.Add(-1*time.Hour), latencyNow); err == nil {
		for _, s := range samples {
			var rtt *int
			if s.RttMs != nil {
				v := *s.RttMs
				rtt = &v
			}
			latencyMap[s.NodeID] = append(latencyMap[s.NodeID], statLatencySample{
				ISP:       s.ISP,
				RttMs:     rtt,
				SampledAt: s.SampledAt.Format(time.RFC3339),
			})
		}
	}

	toChecks := func(crs []nodes.CheckResult) []statCheck {
		out := make([]statCheck, 0, len(crs))
		for _, cr := range crs {
			jc := statCheck{Service: cr.Service, Unlocked: cr.Unlocked, Region: cr.Region, Note: cr.Note}
			if !cr.CheckedAt.IsZero() {
				jc.CheckedAt = cr.CheckedAt.Format(time.RFC3339)
			}
			out = append(out, jc)
		}
		return out
	}

	// 预构建节点 IP → GeoIP 信息映射（DB 未就绪时静默跳过）
	geoMap := make(map[string]*statNodeGeo, len(nodeList))
	if h.geoDB != nil && h.geoDB.Ready() {
		for _, n := range nodeList {
			host := strings.TrimSpace(n.IPOverride)
			if host == "" {
				// 从 base_url 提取 hostname
				if idx := strings.Index(n.BaseURL, "://"); idx >= 0 {
					rest := n.BaseURL[idx+3:]
					if end := strings.IndexAny(rest, ":/"); end >= 0 {
						host = rest[:end]
					} else {
						host = rest
					}
				}
			}
			if host == "" {
				continue
			}
			if info, err := h.geoDB.LookupHost(host); err == nil {
				geoMap[n.ID] = &statNodeGeo{
					CountryCode: info.CountryCode,
					CountryName: info.CountryName,
					City:        info.City,
					ASNOrg:      info.ASNOrg,
				}
			}
		}
	}

	var totalUnlocked, totalServices, maxServices int
	result := make([]statNode, 0, len(nodeList))
	for _, n := range nodeList {
		direct, proxied := splitCheckResults(checkMap[n.ID])
		jn := statNode{
			ID:            n.ID,
			Name:          n.Name,
			Geo:           geoMap[n.ID],
			DirectChecks:  toChecks(direct),
			ProxiedChecks: toChecks(proxied),
			HasData:       len(direct) > 0,
			TotalCount:    len(direct),
		}
		if bars := uptimeBarsMap[n.ID]; len(bars.Bars) > 0 {
			jn.Granularity = bars.Granularity
			jn.OverallPct = bars.OverallPct
			jBars := make([]statUptimeBar, 0, len(bars.Bars))
			for _, b := range bars.Bars {
				jBars = append(jBars, statUptimeBar{Label: b.Label, OnlinePct: b.OnlinePct})
			}
			jn.UptimeBars = jBars
		}
		if st, ok := speedMap[n.ID]; ok {
			jn.SpeedTest = &statSpeed{DownBps: st.DownBps, UpBps: st.UpBps, TestedAt: st.TestedAt.Format(time.RFC3339)}
		}
		if ls := latencyMap[n.ID]; len(ls) > 0 {
			jn.Latency = ls
		}
		if snaps := tracerouteMap[n.ID]; len(snaps) > 0 {
			for _, s := range snaps {
				jn.Traceroutes = append(jn.Traceroutes, statTracerouteSnapshot{
					Direction: s.Direction,
					Target:    s.Target,
					Hops:      s.Hops,
					Quality:   s.Quality,
					CreatedAt: s.CreatedAt.Format(time.RFC3339),
				})
			}
		}
		for _, cr := range direct {
			if cr.Unlocked {
				jn.UnlockedCount++
			}
			if jn.CheckedAt == "" && !cr.CheckedAt.IsZero() {
				jn.CheckedAt = cr.CheckedAt.Format(time.RFC3339)
			}
		}
		if jn.TotalCount > 0 {
			jn.UnlockPct = jn.UnlockedCount * 100 / jn.TotalCount
		}
		if len(direct) > maxServices {
			maxServices = len(direct)
		}
		totalUnlocked += jn.UnlockedCount
		totalServices += jn.TotalCount
		result = append(result, jn)
	}

	avgUnlockRate := 0
	if totalServices > 0 {
		avgUnlockRate = totalUnlocked * 100 / totalServices
	}
	avgUptimePct := -1
	var uptimeSum, uptimeCount int
	for _, e := range result {
		if e.OverallPct > 0 || len(e.UptimeBars) > 0 {
			uptimeSum += e.OverallPct
			uptimeCount++
		}
	}
	if uptimeCount > 0 {
		avgUptimePct = uptimeSum / uptimeCount
	}

	return &statPayload{
		Nodes:        result,
		NodeCount:    len(nodeList),
		UnlockRate:   avgUnlockRate,
		AvgUptimePct: avgUptimePct,
		ServiceCount: maxServices,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (h *Handler) statJSON(w http.ResponseWriter, r *http.Request) {
	payload, err := h.buildStatPayload()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

// statStream 是 /v1/stat/stream 的 SSE 处理器（无需认证）。
// 连接时立即推送 event:init（完整 stat 数据），之后每 2s 推送 event:metrics（实时网速），
// 每 60s 重推 event:init（刷新 uptime bars 等慢变数据）。
func (h *Handler) statStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendInit := func() {
		payload, err := h.buildStatPayload()
		if err != nil {
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
		flusher.Flush()
	}

	type nodeMetric struct {
		NodeID        string `json:"node_id"`
		UploadSpeed   int64  `json:"upload_speed"`
		DownloadSpeed int64  `json:"download_speed"`
		Connections   int    `json:"connections"`
		Running       bool   `json:"running"`
	}

	sendMetrics := func() {
		nodeList, err := h.nodeStore.List()
		if err != nil {
			return
		}
		results := make([]nodeMetric, 0, len(nodeList))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, n := range nodeList {
			wg.Add(1)
			go func(n nodes.Node) {
				defer wg.Done()
				client, err := h.dial(n.ID)
				if err != nil {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				stats, err := client.Usage(ctx, false)
				if err != nil {
					return
				}
				mu.Lock()
				results = append(results, nodeMetric{
					NodeID:        n.ID,
					UploadSpeed:   stats.UploadSpeed,
					DownloadSpeed: stats.DownloadSpeed,
					Connections:   stats.Connections,
					Running:       stats.Running,
				})
				mu.Unlock()
			}(n)
		}
		wg.Wait()
		data, err := json.Marshal(results)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// 首次立即推送完整数据
	sendInit()

	metricsTicker := time.NewTicker(2 * time.Second)
	initTicker := time.NewTicker(60 * time.Second)
	defer metricsTicker.Stop()
	defer initTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-metricsTicker.C:
			sendMetrics()
		case <-initTicker.C:
			sendInit()
		}
	}
}

func (h *Handler) discourseRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.discourse.Enabled() {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.URL.Query().Get("spa") == "1" {
		http.SetCookie(w, &http.Cookie{
			Name:     "discourse_spa",
			Value:    "1",
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	returnURL := fmt.Sprintf("%s://%s/auth/discourse/callback", scheme, r.Host)
	redirectURL, err := h.discourse.BuildRedirectURL(returnURL)
	if err != nil {
		http.Error(w, "SSO 初始化失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) discourseCallback(w http.ResponseWriter, r *http.Request) {
	if !h.discourse.Enabled() {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	rawSSO := r.URL.Query().Get("sso")
	sig := r.URL.Query().Get("sig")
	username, err := h.discourse.ParseCallback(rawSSO, sig)
	if err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}
	if !h.discourse.IsAllowed(username) {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}
	token, err := h.auth.CreateSession(username)
	if err != nil {
		http.Error(w, "创建 session 失败", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token)
	if cookie, cErr := r.Cookie("discourse_spa"); cErr == nil && cookie.Value == "1" {
		http.SetCookie(w, &http.Cookie{Name: "discourse_spa", Value: "", Path: "/", MaxAge: -1})
		http.Redirect(w, r, "/login#discourse_token="+token, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// ─── 节点管理 ─────────────────────────────────────────────────────────────────

func splitCheckResults(all []nodes.CheckResult) (direct, proxied []nodes.CheckResult) {
	for _, cr := range all {
		if cr.CheckType == "proxied" {
			proxied = append(proxied, cr)
		} else {
			direct = append(direct, cr)
		}
	}
	return
}

func (h *Handler) ApplyNodes(nodeIDs []string) {
	for _, nodeID := range nodeIDs {
		go func(id string) {
			client, err := h.dial(id)
			if err != nil {
				log.Printf("applyNodes: dial %s: %v", id, err)
				return
			}
			nodeInbounds, err := h.ibStore.ListInboundsByNode(id)
			if err != nil {
				log.Printf("applyNodes: list inbounds %s: %v", id, err)
				return
			}
			userAccesses, err := h.userStore.ListUserInboundsByNode(id)
			if err != nil {
				log.Printf("applyNodes: list user accesses %s: %v", id, err)
				return
			}
			userIDs := collectUserIDs(userAccesses)
			userMap, err := h.userStore.GetUsersByIDs(userIDs)
			if err != nil {
				log.Printf("applyNodes: get users %s: %v", id, err)
				return
			}
			n, _ := h.nodeStore.Get(id)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, _, err := jobs.ApplyNodeUsers(ctx, client, nodeInbounds, userAccesses, userMap, h.ibStore, h.outboundStore, h.applyOpts, n); err != nil {
				log.Printf("applyNodes: apply %s: %v", id, err)
			}
		}(nodeID)
	}
}

// SyncUserInbounds reconciles a user's inbound assignments. Exported for payment webhook use.
func (h *Handler) SyncUserInbounds(userID string, selectedInboundIDs []string) ([]string, error) {
	wantedInbounds := make(map[string]inbounds.Inbound)
	for _, ibID := range selectedInboundIDs {
		ib, err := h.ibStore.GetInbound(ibID)
		if err != nil {
			continue
		}
		wantedInbounds[ibID] = ib
	}
	existing, err := h.userStore.ListDirectUserInboundsByUser(userID)
	if err != nil {
		return nil, err
	}
	existingByInbound := make(map[string]users.UserInbound, len(existing))
	for _, acc := range existing {
		existingByInbound[acc.InboundID] = acc
	}
	changedNodeIDs := make(map[string]struct{})
	for ibID, ib := range wantedInbounds {
		if _, ok := existingByInbound[ibID]; !ok {
			secret := panelRandomToken(12)
			if ib.Protocol == "shadowsocks" && strings.HasPrefix(ib.Method, "2022-") {
				secret = generateSSPassword(ib.Method)
			}
			acc := users.UserInbound{
				ID:        idgen.NextString(),
				UserID:    userID,
				InboundID: ibID,
				NodeID:    ib.NodeID,
				UUID:      panelRandomUUID(),
				Secret:    secret,
			}
			if _, err := h.userStore.UpsertUserInbound(acc); err != nil {
				return nil, err
			}
			changedNodeIDs[ib.NodeID] = struct{}{}
		}
	}
	for ibID, acc := range existingByInbound {
		if _, wanted := wantedInbounds[ibID]; !wanted {
			if err := h.userStore.DeleteUserInbound(acc.ID); err != nil {
				return nil, err
			}
			changedNodeIDs[acc.NodeID] = struct{}{}
		}
	}
	affected := make([]string, 0, len(changedNodeIDs))
	for id := range changedNodeIDs {
		affected = append(affected, id)
	}
	return affected, nil
}

func collectUserIDs(accesses []users.UserInbound) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, acc := range accesses {
		if _, ok := seen[acc.UserID]; !ok {
			seen[acc.UserID] = struct{}{}
			out = append(out, acc.UserID)
		}
	}
	return out
}

// ─── 用户自助门户 API ─────────────────────────────────────────────────────────

// subURL 根据请求上下文构造完整的订阅链接。
func subURL(r *http.Request, token string) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return scheme + "://" + host + "/sub/" + token
}


func (h *Handler) apiResetToken(w http.ResponseWriter, r *http.Request) {
	jsonErr := func(status int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"error":%q}`, msg)
	}
	token := r.URL.Query().Get("token")
	user, err := h.userStore.GetUserBySubToken(token)
	if err != nil {
		jsonErr(http.StatusNotFound, "user not found")
		return
	}
	user.SubToken = panelRandomToken(16)
	user.UUID = panelRandomUUID()
	user.Secret = panelRandomToken(16)
	if _, err := h.userStore.UpsertUser(user); err != nil {
		jsonErr(http.StatusInternalServerError, "failed to reset token")
		return
	}
	accesses, err := h.userStore.ListUserInboundsByUser(user.ID)
	if err != nil {
		jsonErr(http.StatusInternalServerError, "failed to list user inbounds")
		return
	}
	affectedNodeIDs := make(map[string]struct{})
	for _, acc := range accesses {
		ib, err := h.ibStore.GetInbound(acc.InboundID)
		if err != nil {
			continue
		}
		secret := panelRandomToken(12)
		if ib.Protocol == "shadowsocks" && strings.HasPrefix(ib.Method, "2022-") {
			secret = generateSSPassword(ib.Method)
		}
		acc.UUID = panelRandomUUID()
		acc.Secret = secret
		if _, err := h.userStore.UpsertUserInbound(acc); err != nil {
			jsonErr(http.StatusInternalServerError, "failed to reset inbound credentials")
			return
		}
		affectedNodeIDs[acc.NodeID] = struct{}{}
	}
	affected := make([]string, 0, len(affectedNodeIDs))
	for id := range affectedNodeIDs {
		affected = append(affected, id)
	}
	h.ApplyNodes(affected)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":   user.SubToken,
		"sub_url": subURL(r, user.SubToken),
	})
}
