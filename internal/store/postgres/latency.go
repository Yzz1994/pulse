package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"pulse/internal/nodes"
)

// SaveLatencySamples 批量插入延迟采样，使用 SendBatch 减少往返次数。
func (s *NodeStore) SaveLatencySamples(samples []nodes.LatencySample) error {
	if len(samples) == 0 {
		return nil
	}
	ctx := context.Background()
	batch := &pgx.Batch{}
	for _, sm := range samples {
		batch.Queue(
			`INSERT INTO node_latency_samples (node_id, isp, rtt_ms, sampled_at) VALUES ($1,$2,$3,$4)`,
			sm.NodeID, sm.ISP, sm.RttMs, sm.SampledAt,
		)
	}
	br := s.db.SendBatch(ctx, batch)
	defer br.Close()
	for range samples {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("save latency sample: %w", err)
		}
	}
	return nil
}

// QueryLatencySamples 查询时间范围内的采样，按 sampled_at 升序。
func (s *NodeStore) QueryLatencySamples(nodeIDs []string, from, to time.Time) ([]nodes.LatencySample, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	ctx := context.Background()
	rows, err := s.db.Query(ctx,
		`SELECT node_id, isp, rtt_ms, sampled_at FROM node_latency_samples
		 WHERE sampled_at >= $1 AND sampled_at <= $2
		   AND node_id = ANY($3)
		 ORDER BY sampled_at ASC`,
		from, to, nodeIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query latency samples: %w", err)
	}
	defer rows.Close()

	var out []nodes.LatencySample
	for rows.Next() {
		var sm nodes.LatencySample
		if err := rows.Scan(&sm.NodeID, &sm.ISP, &sm.RttMs, &sm.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// CleanupOldLatencySamples 删除 before 之前的采样。
func (s *NodeStore) CleanupOldLatencySamples(before time.Time) error {
	_, err := s.db.Exec(context.Background(),
		`DELETE FROM node_latency_samples WHERE sampled_at < $1`, before)
	return err
}
