package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/idgen"
	"pulse/internal/nodes"
	"pulse/internal/store/postgres/sqlcgen"
)

type NodeStore struct {
	db *pgxpool.Pool
}

func (s *NodeStore) Upsert(node nodes.Node) (nodes.Node, error) {
	var expireAt *string
	if node.ExpireAt != nil {
		sv := node.ExpireAt.UTC().Format(time.RFC3339)
		expireAt = &sv
	}
	disabled := int32(0)
	if node.Disabled {
		disabled = 1
	}
	err := sqlcgen.New(s.db).UpsertNode(context.Background(), sqlcgen.UpsertNodeParams{
		ID:           node.ID,
		Name:         node.Name,
		BaseUrl:      node.BaseURL,
		ExpireAt:     expireAt,
		PanelUrl:     node.PanelURL,
		Remark:       node.Remark,
		IpOverride:   node.IPOverride,
		Disabled:     disabled,
		AcmeEmail:    node.ACMEEmail,
		PanelDomain:  node.PanelDomain,
		ExtraProxies: node.ExtraProxies,
		HttpsPort:    int32(node.HTTPSPort),
		IsLanding:    node.IsLanding,
	})
	if err != nil {
		return nodes.Node{}, fmt.Errorf("upsert node: %w", err)
	}
	return s.Get(node.ID)
}

func (s *NodeStore) Delete(id string) error {
	result, err := sqlcgen.New(s.db).DeleteNodeByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nodes.ErrNodeNotFound
	}
	return nil
}

func (s *NodeStore) Get(id string) (nodes.Node, error) {
	row, err := sqlcgen.New(s.db).GetNodeByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nodes.Node{}, nodes.ErrNodeNotFound
	}
	if err != nil {
		return nodes.Node{}, fmt.Errorf("get node: %w", err)
	}
	return toNodeFromGet(row), nil
}

func (s *NodeStore) List() ([]nodes.Node, error) {
	rows, err := sqlcgen.New(s.db).ListNodes(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	out := make([]nodes.Node, len(rows))
	for i, r := range rows {
		out[i] = toNodeFromList(r)
	}
	return out, nil
}


func (s *NodeStore) AddTraffic(nodeID string, upload, download int64) error {
	err := sqlcgen.New(s.db).AddNodeTraffic(context.Background(), sqlcgen.AddNodeTrafficParams{
		UploadBytes:   upload,
		DownloadBytes: download,
		ID:            nodeID,
	})
	if err != nil {
		return fmt.Errorf("add node traffic: %w", err)
	}
	return nil
}

func (s *NodeStore) AddNodeDailyUsage(nodeID, date string, upload, download int64) error {
	err := sqlcgen.New(s.db).UpsertNodeDailyUsage(context.Background(), sqlcgen.UpsertNodeDailyUsageParams{
		NodeID:        nodeID,
		Date:          date,
		UploadBytes:   upload,
		DownloadBytes: download,
	})
	if err != nil {
		return fmt.Errorf("add node daily usage: %w", err)
	}
	return nil
}

func (s *NodeStore) ListNodeDailyUsage(days int) ([]nodes.NodeDailyUsage, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := sqlcgen.New(s.db).ListNodeDailyUsage(context.Background(), cutoff)
	if err != nil {
		return nil, fmt.Errorf("list node daily usage: %w", err)
	}
	out := make([]nodes.NodeDailyUsage, len(rows))
	for i, r := range rows {
		out[i] = nodes.NodeDailyUsage{
			NodeID:        r.NodeID,
			Date:          r.Date,
			UploadBytes:   r.UploadBytes,
			DownloadBytes: r.DownloadBytes,
		}
	}
	return out, nil
}

func (s *NodeStore) ListNodeDailyUsageRange(nodeID, since, until string) ([]nodes.NodeDailyUsage, error) {
	rows, err := sqlcgen.New(s.db).ListNodeDailyUsageRange(context.Background(), sqlcgen.ListNodeDailyUsageRangeParams{
		NodeID: nodeID,
		Date:   since,
		Date_2: until,
	})
	if err != nil {
		return nil, fmt.Errorf("list node daily usage range: %w", err)
	}
	out := make([]nodes.NodeDailyUsage, len(rows))
	for i, r := range rows {
		out[i] = nodes.NodeDailyUsage{
			NodeID:        nodeID,
			Date:          r.Date,
			UploadBytes:   r.UploadBytes,
			DownloadBytes: r.DownloadBytes,
		}
	}
	return out, nil
}

func (s *NodeStore) CleanupOldDailyUsage(retainDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retainDays).Format("2006-01-02")
	if err := sqlcgen.New(s.db).DeleteOldNodeDailyUsage(context.Background(), cutoff); err != nil {
		return fmt.Errorf("cleanup old daily usage: %w", err)
	}
	return nil
}

func (s *NodeStore) UpsertNodeSpeedTest(nodeID string, result nodes.SpeedTestResult) error {
	err := sqlcgen.New(s.db).UpsertNodeSpeedTest(context.Background(), sqlcgen.UpsertNodeSpeedTestParams{
		NodeID:   nodeID,
		DownBps:  result.DownBps,
		UpBps:    result.UpBps,
		TestedAt: result.TestedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("upsert node speedtest: %w", err)
	}
	return nil
}

func (s *NodeStore) ListAllNodeSpeedTests() (map[string]nodes.SpeedTestResult, error) {
	rows, err := sqlcgen.New(s.db).ListAllNodeSpeedTests(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list node speedtests: %w", err)
	}
	result := make(map[string]nodes.SpeedTestResult, len(rows))
	for _, r := range rows {
		sr := nodes.SpeedTestResult{
			DownBps: r.DownBps,
			UpBps:   r.UpBps,
		}
		sr.TestedAt, _ = time.Parse(time.RFC3339, r.TestedAt)
		result[r.NodeID] = sr
	}
	return result, nil
}

func (s *NodeStore) UpsertNodeCheckResults(nodeID string, results []nodes.CheckResult) error {
	ctx := context.Background()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlcgen.New(tx)
	if err := q.DeleteNodeCheckResults(ctx, nodeID); err != nil {
		return fmt.Errorf("clear node check results: %w", err)
	}
	for _, r := range results {
		checkType := r.CheckType
		if checkType == "" {
			checkType = "direct"
		}
		unlocked := int32(0)
		if r.Unlocked {
			unlocked = 1
		}
		if err := q.InsertNodeCheckResult(ctx, sqlcgen.InsertNodeCheckResultParams{
			NodeID:    nodeID,
			Service:   r.Service,
			CheckType: checkType,
			Unlocked:  unlocked,
			Region:    r.Region,
			Note:      r.Note,
			CheckedAt: r.CheckedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return fmt.Errorf("insert node check result: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *NodeStore) ListAllNodeCheckResults() (map[string][]nodes.CheckResult, error) {
	rows, err := sqlcgen.New(s.db).ListAllNodeCheckResults(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list node check results: %w", err)
	}
	result := make(map[string][]nodes.CheckResult)
	for _, r := range rows {
		cr := nodes.CheckResult{
			Service:   r.Service,
			CheckType: r.CheckType,
			Unlocked:  r.Unlocked != 0,
			Region:    r.Region,
			Note:      r.Note,
		}
		cr.CheckedAt, _ = time.Parse(time.RFC3339, r.CheckedAt)
		result[r.NodeID] = append(result[r.NodeID], cr)
	}
	return result, nil
}

func (s *NodeStore) RecordNodeUptime(nodeID string, online, running bool) error {
	o, r := int32(0), int32(0)
	if online {
		o = 1
	}
	if running {
		r = 1
	}
	err := sqlcgen.New(s.db).InsertNodeUptime(context.Background(), sqlcgen.InsertNodeUptimeParams{
		NodeID:    nodeID,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Online:    o,
		Running:   r,
	})
	if err != nil {
		return fmt.Errorf("record node uptime: %w", err)
	}
	return nil
}

func (s *NodeStore) ListNodeUptimeSummary(days int) (map[string]nodes.UptimeSummary, error) {
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -days).Format(time.RFC3339)
	rows, err := sqlcgen.New(s.db).ListNodeUptimeSummary(context.Background(), cutoff)
	if err != nil {
		return nil, fmt.Errorf("list node uptime summary: %w", err)
	}
	result := make(map[string]nodes.UptimeSummary, len(rows))
	for _, r := range rows {
		sum := nodes.UptimeSummary{
			TotalChecks:   int(r.TotalChecks),
			OnlineChecks:  int(r.OnlineSum),
			RunningChecks: int(r.RunningSum),
		}
		if sum.TotalChecks > 0 {
			sum.OnlinePct = sum.OnlineChecks * 100 / sum.TotalChecks
			sum.RunningPct = sum.RunningChecks * 100 / sum.TotalChecks
		}
		if t, err := time.Parse(time.RFC3339, r.FirstAt); err == nil {
			mins := int(now.Sub(t).Minutes())
			switch {
			case mins < 60:
				// 不足 1h，不设 Label
			case mins < 1440:
				sum.Label = fmt.Sprintf("%dh", mins/60)
			default:
				sum.Label = fmt.Sprintf("%dd", mins/1440)
			}
		}
		result[r.NodeID] = sum
	}
	return result, nil
}

func (s *NodeStore) CleanupOldNodeUptime(retainDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retainDays).Format(time.RFC3339)
	if err := sqlcgen.New(s.db).DeleteOldNodeUptime(context.Background(), cutoff); err != nil {
		return fmt.Errorf("cleanup node uptime log: %w", err)
	}
	return nil
}

func (s *NodeStore) ListNodeUptimeBars(maxDays int) (map[string]nodes.UptimeBarsResult, error) {
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -maxDays).Format(time.RFC3339)
	granularity := "hour"

	rows, err := sqlcgen.New(s.db).ListNodeUptimeBars(context.Background(), cutoff)
	if err != nil {
		return nil, fmt.Errorf("list node uptime bars: %w", err)
	}

	type rowData struct {
		period    string
		total     int
		onlineSum int
	}
	nodeRows := make(map[string][]rowData)
	for _, r := range rows {
		nodeRows[r.NodeID] = append(nodeRows[r.NodeID], rowData{
			period:    r.Period,
			total:     int(r.Total),
			onlineSum: int(r.OnlineSum),
		})
	}

	result := make(map[string]nodes.UptimeBarsResult)
	for nodeID, rs := range nodeRows {
		slots := buildTimeSlots(now, maxDays)
		rowMap := make(map[string]rowData, len(rs))
		for _, r := range rs {
			rowMap[r.period] = r
		}
		var bars []nodes.UptimeBar
		var totalChecks, totalOnline int
		for _, slot := range slots {
			r, ok := rowMap[slot.key]
			pct := -1
			if ok && r.total > 0 {
				pct = r.onlineSum * 100 / r.total
				totalChecks += r.total
				totalOnline += r.onlineSum
			}
			bars = append(bars, nodes.UptimeBar{Label: slot.label, OnlinePct: pct})
		}
		overallPct := 0
		if totalChecks > 0 {
			overallPct = totalOnline * 100 / totalChecks
		}
		result[nodeID] = nodes.UptimeBarsResult{
			Bars:        bars,
			Granularity: granularity,
			OverallPct:  overallPct,
		}
	}
	return result, nil
}

// SaveTracerouteSnapshot 保存一条路由追踪快照，自动生成 ID。
func (s *NodeStore) SaveTracerouteSnapshot(snapshot nodes.TracerouteSnapshot) error {
	snapshot.ID = idgen.NextString()
	err := sqlcgen.New(s.db).InsertTracerouteSnapshot(context.Background(), sqlcgen.InsertTracerouteSnapshotParams{
		ID:        snapshot.ID,
		NodeID:    snapshot.NodeID,
		Direction: snapshot.Direction,
		Target:    snapshot.Target,
		Hops:      snapshot.Hops,
		Quality:   snapshot.Quality,
		CreatedAt: snapshot.CreatedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("save traceroute snapshot: %w", err)
	}
	return nil
}

// ListNodeTracerouteSnapshots 返回节点最近 limit 条快照，按时间降序。
func (s *NodeStore) ListNodeTracerouteSnapshots(nodeID string, limit int) ([]nodes.TracerouteSnapshot, error) {
	rows, err := sqlcgen.New(s.db).ListNodeTracerouteSnapshots(context.Background(), sqlcgen.ListNodeTracerouteSnapshotsParams{
		NodeID: nodeID,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list node traceroute snapshots: %w", err)
	}
	return toTracerouteSnapshots(rows), nil
}

// ListLatestTracerouteSnapshots 返回所有节点的最新快照，按 nodeID 分组。
func (s *NodeStore) ListLatestTracerouteSnapshots() (map[string][]nodes.TracerouteSnapshot, error) {
	rows, err := sqlcgen.New(s.db).ListLatestTracerouteSnapshots(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list latest traceroute snapshots: %w", err)
	}
	result := make(map[string][]nodes.TracerouteSnapshot)
	for _, snap := range toTracerouteSnapshots(rows) {
		result[snap.NodeID] = append(result[snap.NodeID], snap)
	}
	return result, nil
}

func (s *NodeStore) DeleteTracerouteSnapshot(id string) error {
	if err := sqlcgen.New(s.db).DeleteTracerouteSnapshot(context.Background(), id); err != nil {
		return fmt.Errorf("delete traceroute snapshot: %w", err)
	}
	return nil
}

// ─── 类型转换辅助 ─────────────────────────────────────────────────────────────

func toNodeFromGet(r sqlcgen.GetNodeByIDRow) nodes.Node {
	n := nodes.Node{
		ID:            r.ID,
		Name:          r.Name,
		BaseURL:       r.BaseUrl,
		UploadBytes:   r.UploadBytes,
		DownloadBytes: r.DownloadBytes,
		ACMEEmail:     r.AcmeEmail,
		PanelDomain:   r.PanelDomain,
		ExtraProxies:  r.ExtraProxies,
		HTTPSPort:     int(r.HttpsPort),
		PanelURL:      r.PanelUrl,
		Remark:        r.Remark,
		IPOverride:    r.IpOverride,
		Disabled:      r.Disabled != 0,
		IsLanding:     r.IsLanding,
	}
	n.ExpireAt = parseExpireAt(r.ExpireAt)
	return n
}

func toNodeFromList(r sqlcgen.ListNodesRow) nodes.Node {
	n := nodes.Node{
		ID:            r.ID,
		Name:          r.Name,
		BaseURL:       r.BaseUrl,
		UploadBytes:   r.UploadBytes,
		DownloadBytes: r.DownloadBytes,
		ACMEEmail:     r.AcmeEmail,
		PanelDomain:   r.PanelDomain,
		ExtraProxies:  r.ExtraProxies,
		HTTPSPort:     int(r.HttpsPort),
		PanelURL:      r.PanelUrl,
		Remark:        r.Remark,
		IPOverride:    r.IpOverride,
		Disabled:      r.Disabled != 0,
		IsLanding:     r.IsLanding,
	}
	n.ExpireAt = parseExpireAt(r.ExpireAt)
	return n
}

func toTracerouteSnapshots(rows []sqlcgen.TracerouteSnapshot) []nodes.TracerouteSnapshot {
	out := make([]nodes.TracerouteSnapshot, len(rows))
	for i, r := range rows {
		snap := nodes.TracerouteSnapshot{
			ID:        r.ID,
			NodeID:    r.NodeID,
			Direction: r.Direction,
			Target:    r.Target,
			Hops:      r.Hops,
			Quality:   r.Quality,
		}
		snap.CreatedAt, _ = time.Parse(time.RFC3339, r.CreatedAt)
		out[i] = snap
	}
	return out
}

func parseExpireAt(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}

type timeSlot struct {
	key   string
	label string
}

func buildTimeSlots(now time.Time, maxDays int) []timeSlot {
	total := maxDays * 24
	slots := make([]timeSlot, total)
	for i := 0; i < total; i++ {
		t := now.Add(-time.Duration(total-1-i) * time.Hour).Truncate(time.Hour)
		slots[i] = timeSlot{
			key:   t.Format("2006-01-02 15"),
			label: t.Format(time.RFC3339),
		}
	}
	return slots
}
