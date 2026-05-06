package jobs

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"pulse/internal/idgen"
	"pulse/internal/ipsentinel"
	"pulse/internal/nodes"
	pgStore "pulse/internal/store/postgres"
)

// IPSentinelSettingsStore 提供 IP Sentinel 调度所需的配置读写。
type IPSentinelSettingsStore interface {
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
}

// RunIPSentinel 遍历所有活跃节点，自动执行 IP 纠偏。
// 调度器每 30 分钟调用一次，内部检查是否到了配置的间隔（默认 1 小时）。
// 每个节点使用默认配置（enable_google=true, enable_trust=true），
// 仅 region_code/region_name 从数据库读取。
// RunIPSentinel 遍历所有活跃节点，自动执行 IP 纠偏。
// force=true 时跳过间隔检查（用于手动触发）。
func RunIPSentinel(
	ctx context.Context,
	sentinelStore *pgStore.IPSentinelStore,
	dial func(string) (*nodes.Client, error),
	nodeStore nodes.Store,
	settings IPSentinelSettingsStore,
	force ...bool,
) error {
	if len(force) == 0 || !force[0] {
		// 读取间隔（小时），0 或未配置 → 默认 1 小时
		intervalHours := 1
		if v, ok := settings.GetSetting("ip_sentinel_interval_hours"); ok && v != "" {
			if n := parseInt(v); n > 0 {
				intervalHours = n
			}
		}
		// 检查上次执行时间
		if lastStr, ok := settings.GetSetting("ip_sentinel_last_run_at"); ok && lastStr != "" {
			if last, err := time.Parse(time.RFC3339, lastStr); err == nil {
				if time.Since(last) < time.Duration(intervalHours)*time.Hour {
					return nil
				}
			}
		}
	}

	allNodes, err := nodeStore.List()
	if err != nil {
		return err
	}

	active := make([]nodes.Node, 0, len(allNodes))
	for _, n := range allNodes {
		if !n.Disabled {
			active = append(active, n)
		}
	}
	if len(active) == 0 {
		return nil
	}

	_ = settings.SetSetting("ip_sentinel_last_run_at", time.Now().UTC().Format(time.RFC3339))
	log.Printf("ip-sentinel: 开始自动执行，共 %d 个节点", len(active))

	for _, n := range active {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 随机抖动 0-30 秒，避免同时打到所有节点
		jitter := time.Duration(rand.Intn(30)) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter):
		}

		client, err := dial(n.ID)
		if err != nil {
			log.Printf("ip-sentinel auto-run: dial node=%s err=%v", n.ID, err)
			continue
		}

		// 检查节点是否已有任务在跑
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		status, err := client.IPSentinelStatus(checkCtx)
		checkCancel()
		if err == nil && status.Running {
			log.Printf("ip-sentinel auto-run: node=%s 已有任务运行中，跳过", n.ID)
			continue
		}

		// 无 region 配置时跳过纠偏（避免用 (0,0) 坐标污染 Google 记录）
		cfg := buildDefaultConfig(n.ID, sentinelStore)
		if cfg.RegionCode == "" {
			log.Printf("ip-sentinel auto-run: node=%s 未配置地区，跳过纠偏", n.ID)
			continue
		}

		// 后台异步检测 IP（仅记录监控数据，不修改 region）
		go func(nodeID string, c *nodes.Client) {
			runID := idgen.NextString()
			_ = sentinelStore.InsertRun(runID, nodeID, "detect", "scheduled", "running", time.Now())
			detectCtx, detectCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer detectCancel()
			result, err := c.IPSentinelDetect(detectCtx)
			if err != nil {
				_ = sentinelStore.UpdateRun(runID, "failed", err.Error(), "", time.Now())
				return
			}
			resultJSON, _ := json.Marshal(result)
			_ = sentinelStore.UpdateRun(runID, "success", "", string(resultJSON), time.Now())
		}(n.ID, client)

		// 推送配置到节点
		pushCtx, pushCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := client.IPSentinelSetConfig(pushCtx, cfg); err != nil {
			log.Printf("ip-sentinel auto-run: node=%s 推送配置失败: %v", n.ID, err)
		}
		pushCancel()

		// 触发纠偏执行
		trigCtx, trigCancel := context.WithTimeout(ctx, 10*time.Second)
		trigErr := client.IPSentinelRun(trigCtx, "auto")
		trigCancel()
		if trigErr != nil {
			log.Printf("ip-sentinel auto-run: node=%s 触发失败: %v", n.ID, trigErr)
			continue
		}

		runID := idgen.NextString()
		_ = sentinelStore.InsertRun(runID, n.ID, "auto", "scheduled", "running", time.Now())
		go pollIPSentinelResult(runID, n.ID, client, sentinelStore)
		log.Printf("ip-sentinel auto-run: node=%s 已触发 run_id=%s", n.ID, runID)
	}

	return nil
}

// buildDefaultConfig 构建默认配置：enable_google/trust=true，region 从 DB 读取。
func buildDefaultConfig(nodeID string, store *pgStore.IPSentinelStore) ipsentinel.Config {
	cfg := ipsentinel.Config{
		EnableGoogle:   true,
		EnableTrust:    true,
		LangParams:     "hl=en&gl=US",
		ValidURLSuffix: "com",
		Keywords:       []string{},
		WhiteURLs:      []string{},
	}

	if stored, err := store.GetConfig(nodeID); err == nil && stored != nil && stored.RegionCode != "" {
		// 用地区模板填充所有与地区相关的字段
		tmpl := ipsentinel.SuggestConfigByCountry(stored.RegionCode)
		cfg.RegionCode = stored.RegionCode
		cfg.RegionName = stored.RegionName
		cfg.BaseLat = tmpl.BaseLat
		cfg.BaseLon = tmpl.BaseLon
		cfg.LangParams = tmpl.LangParams
		cfg.ValidURLSuffix = tmpl.ValidURLSuffix
		if len(tmpl.Keywords) > 0 {
			cfg.Keywords = tmpl.Keywords
		}
		if len(tmpl.WhiteURLs) > 0 {
			cfg.WhiteURLs = tmpl.WhiteURLs
		}
	}

	return cfg
}

// pollIPSentinelResult 后台轮询节点直到任务完成，更新 DB 记录。
func pollIPSentinelResult(runID, nodeID string, client *nodes.Client, store *pgStore.IPSentinelStore) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		status, err := client.IPSentinelStatus(ctx)
		cancel()
		if err != nil {
			continue
		}
		if !status.Running && status.Last != nil && status.Last.TaskType == "auto" {
			last := status.Last
			output := strings.Join(last.Output, "\n")
			var resultJSON string
			if b, err := json.Marshal(last); err == nil {
				resultJSON = string(b)
			}
			_ = store.UpdateRun(runID, last.Status, output, resultJSON, last.FinishedAt)
			return
		}
		if !status.Running {
			// 节点返回的最后任务不是本次触发的 auto，结果不可信
			_ = store.UpdateRun(runID, "failed", "节点返回结果与触发任务不匹配", "", time.Now())
			return
		}
	}
	_ = store.UpdateRun(runID, "timeout", "等待节点响应超时", "", time.Now())
}

// IPSentinelIntervalHours 读取当前配置的间隔小时数（默认 1）。
func IPSentinelIntervalHours(settings IPSentinelSettingsStore) int {
	if v, ok := settings.GetSetting("ip_sentinel_interval_hours"); ok && v != "" {
		if n := parseInt(v); n > 0 {
			return n
		}
	}
	return 1
}

// IPSentinelLastRunAt 读取上次执行时间（nil 表示未执行过）。
func IPSentinelLastRunAt(settings IPSentinelSettingsStore) *time.Time {
	if v, ok := settings.GetSetting("ip_sentinel_last_run_at"); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t
		}
	}
	return nil
}

// IPSentinelNextRunAt 预计下次执行时间。
func IPSentinelNextRunAt(settings IPSentinelSettingsStore) *time.Time {
	last := IPSentinelLastRunAt(settings)
	if last == nil {
		return nil
	}
	next := last.Add(time.Duration(IPSentinelIntervalHours(settings)) * time.Hour)
	return &next
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// MarshalIPSentinel 是一个辅助类型，用于 JSON 序列化 ipsentinel.RunResult。
var _ = ipsentinel.RunResult{}
