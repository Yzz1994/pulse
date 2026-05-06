package serverapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pulse/internal/geoip"
	"pulse/internal/idgen"
	"pulse/internal/ipsentinel"
	"pulse/internal/jobs"
	"pulse/internal/nodes"
	pgStore "pulse/internal/store/postgres"
)

// RegisterIPSentinelAPI 注册 IP Sentinel 控制面路由。
// geoDB 可为 nil，nil 时 detect 降级为调用节点查询外部 API。
func RegisterIPSentinelAPI(mux *http.ServeMux, sentinelStore *pgStore.IPSentinelStore, api *API, geoDB *geoip.DB, nodeStore nodes.Store) {
	// GET /v1/ip-sentinel/schedule — 查询自动执行配置
	mux.HandleFunc("GET /v1/ip-sentinel/schedule", func(w http.ResponseWriter, r *http.Request) {
		intervalHours := jobs.IPSentinelIntervalHours(api.settings)
		lastRunAt := jobs.IPSentinelLastRunAt(api.settings)
		nextRunAt := jobs.IPSentinelNextRunAt(api.settings)
		writeJSON(w, http.StatusOK, map[string]any{
			"interval_hours": intervalHours,
			"last_run_at":    lastRunAt,
			"next_run_at":    nextRunAt,
		})
	})

	// PUT /v1/ip-sentinel/schedule — 更新自动执行间隔
	mux.HandleFunc("PUT /v1/ip-sentinel/schedule", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IntervalHours int `json:"interval_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IntervalHours < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "interval_hours 必须 >= 1"})
			return
		}
		if err := api.settings.SetSetting("ip_sentinel_interval_hours", strconv.Itoa(body.IntervalHours)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "interval_hours": body.IntervalHours})
	})
	// POST /v1/ip-sentinel/run-all — 立即对所有节点执行一次（忽略间隔检查）
	mux.HandleFunc("POST /v1/ip-sentinel/run-all", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := jobs.RunIPSentinel(context.Background(), sentinelStore, api.Dial, nodeStore, api.settings, true); err != nil {
				log.Printf("ip-sentinel run-all: %v", err)
			}
		}()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// GET /v1/nodes/{id}/ip-sentinel/config — 获取节点 region 配置
	mux.HandleFunc("GET /v1/nodes/{id}/ip-sentinel/config", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		cfg, err := sentinelStore.GetConfig(nodeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if cfg == nil {
			cfg = &ipsentinel.Config{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"region_code": cfg.RegionCode,
			"region_name": cfg.RegionName,
		})
	})

	// PUT /v1/nodes/{id}/ip-sentinel/config — 更新 region（仅 region_code / region_name）
	mux.HandleFunc("PUT /v1/nodes/{id}/ip-sentinel/config", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		var body struct {
			RegionCode string `json:"region_code"`
			RegionName string `json:"region_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}

		cfg := ipsentinel.Config{
			RegionCode: body.RegionCode,
			RegionName: body.RegionName,
		}
		if err := sentinelStore.UpsertConfig(cfg, nodeID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// POST /v1/nodes/{id}/ip-sentinel/detect — 同步 IP 检测
	mux.HandleFunc("POST /v1/nodes/{id}/ip-sentinel/detect", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		runID := idgen.NextString()
		startedAt := time.Now()

		if geoDB != nil && geoDB.Ready() {
			node, err := api.store.Get(nodeID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
				return
			}
			host := node.IPOverride
			if host == "" {
				if u, err := url.Parse(node.BaseURL); err == nil {
					host = u.Hostname()
				}
			}
			if host != "" {
				info, err := geoDB.LookupHost(host)
				if err == nil {
					result := ipsentinel.DetectResult{
						IP:          info.IP,
						Country:     info.CountryName,
						CountryCode: info.CountryCode,
						RegionName:  info.Region,
						City:        info.City,
						ISP:         info.ASNOrg,
						Org:         info.ASNOrg,
						Lat:         info.Lat,
						Lon:         info.Lon,
						Timezone:    info.Timezone,
						DetectedAt:  time.Now().UTC(),
					}
					if b, _ := json.Marshal(result); len(b) > 0 {
						_ = sentinelStore.InsertRun(runID, nodeID, "detect", "manual", "success", startedAt)
						_ = sentinelStore.UpdateRun(runID, "success", "", string(b), time.Now())
					}
					writeJSON(w, http.StatusOK, result)
					return
				}
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		client, err := api.clientFor(nodeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
			return
		}
		result, err := client.IPSentinelDetect(ctx)
		status := "success"
		resultJSON := ""
		if err != nil {
			status = "failed"
			if b, _ := json.Marshal(map[string]string{"error": err.Error()}); len(b) > 0 {
				resultJSON = string(b)
			}
		} else {
			if b, jerr := json.Marshal(result); jerr == nil {
				resultJSON = string(b)
			}
		}
		_ = sentinelStore.InsertRun(runID, nodeID, "detect", "manual", status, startedAt)
		_ = sentinelStore.UpdateRun(runID, status, "", resultJSON, time.Now())
		if err != nil {
			log.Printf("ip-sentinel detect node=%s err=%v", nodeID, err)
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// POST /v1/nodes/{id}/ip-sentinel/run — 手动触发执行
	mux.HandleFunc("POST /v1/nodes/{id}/ip-sentinel/run", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		client, err := api.clientFor(nodeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
			return
		}

		runID := idgen.NextString()
		startedAt := time.Now()
		_ = sentinelStore.InsertRun(runID, nodeID, "auto", "manual", "running", startedAt)

		triggerCtx, triggerCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if triggerErr := client.IPSentinelRun(triggerCtx, "auto"); triggerErr != nil {
			triggerCancel()
			_ = sentinelStore.UpdateRun(runID, "failed", triggerErr.Error(), "", time.Now())
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": triggerErr.Error()})
			return
		}
		triggerCancel()

		go func() {
			deadline := time.Now().Add(5 * time.Minute)
			for time.Now().Before(deadline) {
				time.Sleep(5 * time.Second)
				pollCtx, pollCancel := context.WithTimeout(context.Background(), 10*time.Second)
				status, pollErr := client.IPSentinelStatus(pollCtx)
				pollCancel()
				if pollErr != nil {
					continue
				}
				if !status.Running && status.Last != nil && status.Last.TaskType == "auto" {
					last := status.Last
					outputStr := strings.Join(last.Output, "\n")
					resultJSON := ""
					if b, jerr := json.Marshal(last); jerr == nil {
						resultJSON = string(b)
					}
					_ = sentinelStore.UpdateRun(runID, last.Status, outputStr, resultJSON, last.FinishedAt)
					return
				}
				if !status.Running {
					_ = sentinelStore.UpdateRun(runID, "success", "", "", time.Now())
					return
				}
			}
			_ = sentinelStore.UpdateRun(runID, "timeout", "等待节点响应超时", "", time.Now())
		}()

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID})
	})

	// GET /v1/nodes/{id}/ip-sentinel/runs — 历史记录
	mux.HandleFunc("GET /v1/nodes/{id}/ip-sentinel/runs", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		rows, err := sentinelStore.ListRuns(nodeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		runs := make([]map[string]any, len(rows))
		for i, row := range rows {
			runs[i] = pgStore.RunToMap(row)
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	})
}
