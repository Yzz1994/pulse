package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/ipsentinel"
	"pulse/internal/store/postgres/sqlcgen"
)

// IPSentinelStore IP Sentinel 数据访问层。
type IPSentinelStore struct {
	db *pgxpool.Pool
}

// IPSentinelStore 返回 IPSentinelStore 实例。
func (db *DB) IPSentinelStore() *IPSentinelStore {
	return &IPSentinelStore{db: db.conn}
}

// UpsertConfig 创建或更新节点的 IP Sentinel 配置。
func (s *IPSentinelStore) UpsertConfig(cfg ipsentinel.Config, nodeID string) error {
	if cfg.WhiteURLs == nil {
		cfg.WhiteURLs = []string{}
	}
	if cfg.Keywords == nil {
		cfg.Keywords = []string{}
	}
	whiteURLs, _ := json.Marshal(cfg.WhiteURLs)
	keywords, _ := json.Marshal(cfg.Keywords)

	return sqlcgen.New(s.db).UpsertIPSentinelConfig(context.Background(), sqlcgen.UpsertIPSentinelConfigParams{
		NodeID:         nodeID,
		RegionCode:     cfg.RegionCode,
		RegionName:     cfg.RegionName,
		BaseLat:        cfg.BaseLat,
		BaseLon:        cfg.BaseLon,
		LangParams:     cfg.LangParams,
		ValidUrlSuffix: cfg.ValidURLSuffix,
		EnableGoogle:   cfg.EnableGoogle,
		EnableTrust:    cfg.EnableTrust,
		WhiteUrls:      string(whiteURLs),
		Keywords:       string(keywords),
		UpdatedAt:      time.Now().UTC(),
	})
}

// GetConfig 获取节点的 IP Sentinel 配置，不存在时返回 nil, nil。
func (s *IPSentinelStore) GetConfig(nodeID string) (*ipsentinel.Config, error) {
	row, err := sqlcgen.New(s.db).GetIPSentinelConfig(context.Background(), nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rowToConfig(row), nil
}

// InsertRun 插入一条任务执行记录，并保留每个节点最近 5 条。
func (s *IPSentinelStore) InsertRun(id, nodeID, taskType, triggeredBy, status string, startedAt time.Time) error {
	ctx := context.Background()
	if err := sqlcgen.New(s.db).InsertIPSentinelRun(ctx, sqlcgen.InsertIPSentinelRunParams{
		ID:          id,
		NodeID:      nodeID,
		TaskType:    taskType,
		TriggeredBy: triggeredBy,
		Status:      status,
		Output:      "",
		Result:      "",
		StartedAt:   startedAt,
	}); err != nil {
		return err
	}
	// 只保留该节点最近 5 条记录
	_, _ = s.db.Exec(ctx,
		`DELETE FROM ip_sentinel_runs WHERE node_id = $1 AND id NOT IN (
			SELECT id FROM ip_sentinel_runs WHERE node_id = $1 ORDER BY started_at DESC LIMIT 5
		)`, nodeID)
	return nil
}

// UpdateRun 更新任务执行结果。
func (s *IPSentinelStore) UpdateRun(id, status, output, result string, finishedAt time.Time) error {
	return sqlcgen.New(s.db).UpdateIPSentinelRun(context.Background(), sqlcgen.UpdateIPSentinelRunParams{
		ID:     id,
		Status: status,
		Output: output,
		Result: result,
		FinishedAt: pgtype.Timestamptz{
			Time:  finishedAt,
			Valid: true,
		},
	})
}

// ListRuns 列出节点最近 20 条任务记录。
func (s *IPSentinelStore) ListRuns(nodeID string) ([]sqlcgen.IpSentinelRun, error) {
	return sqlcgen.New(s.db).ListIPSentinelRuns(context.Background(), nodeID)
}

// rowToConfig 将 sqlcgen 行转换为 ipsentinel.Config。
func rowToConfig(r sqlcgen.IpSentinelConfig) *ipsentinel.Config {
	cfg := &ipsentinel.Config{
		RegionCode:     r.RegionCode,
		RegionName:     r.RegionName,
		BaseLat:        r.BaseLat,
		BaseLon:        r.BaseLon,
		LangParams:     r.LangParams,
		ValidURLSuffix: r.ValidUrlSuffix,
		EnableGoogle:   r.EnableGoogle,
		EnableTrust:    r.EnableTrust,
	}

	// 解析 JSON 数组字段
	if r.WhiteUrls != "" && r.WhiteUrls != "null" {
		_ = json.Unmarshal([]byte(r.WhiteUrls), &cfg.WhiteURLs)
	}
	if r.Keywords != "" && r.Keywords != "null" {
		_ = json.Unmarshal([]byte(r.Keywords), &cfg.Keywords)
	}

	// 确保 slice 不为 nil
	if cfg.WhiteURLs == nil {
		cfg.WhiteURLs = []string{}
	}
	if cfg.Keywords == nil {
		cfg.Keywords = []string{}
	}

	// 默认值补全
	if cfg.LangParams == "" {
		cfg.LangParams = "hl=en&gl=US"
	}
	if cfg.ValidURLSuffix == "" {
		cfg.ValidURLSuffix = "com"
	}

	return cfg
}

// RunToMap 将 IpSentinelRun 转换为可序列化的 map。
func RunToMap(r sqlcgen.IpSentinelRun) map[string]any {
	m := map[string]any{
		"id":           r.ID,
		"node_id":      r.NodeID,
		"task_type":    r.TaskType,
		"triggered_by": r.TriggeredBy,
		"status":       r.Status,
		"output":       splitLines(r.Output),
		"started_at":   r.StartedAt,
	}
	if r.FinishedAt.Valid {
		m["finished_at"] = r.FinishedAt.Time
	}
	// result 字段若为有效 JSON 则展开，否则作为字符串
	if r.Result != "" {
		var raw json.RawMessage
		if json.Unmarshal([]byte(r.Result), &raw) == nil {
			m["result"] = raw
		} else {
			m["result"] = r.Result
		}
	}
	return m
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
