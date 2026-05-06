package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/auditrules"
)

type AuditRuleStore struct {
	db *pgxpool.Pool
}

func (s *AuditRuleStore) List() ([]auditrules.Rule, error) {
	ctx := context.Background()
	rows, err := s.db.Query(ctx, `SELECT id,type,value,enabled,created_at FROM audit_rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list audit rules: %w", err)
	}
	defer rows.Close()
	var out []auditrules.Rule
	for rows.Next() {
		var r auditrules.Rule
		if err := rows.Scan(&r.ID, &r.Type, &r.Value, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AuditRuleStore) Insert(r auditrules.Rule) error {
	ctx := context.Background()
	_, err := s.db.Exec(ctx,
		`INSERT INTO audit_rules (id,type,value,enabled,created_at) VALUES ($1,$2,$3,$4,$5)`,
		r.ID, r.Type, r.Value, r.Enabled, r.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert audit rule: %w", err)
	}
	return nil
}

func (s *AuditRuleStore) Delete(id string) error {
	ctx := context.Background()
	_, err := s.db.Exec(ctx, `DELETE FROM audit_rules WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete audit rule: %w", err)
	}
	return nil
}

func (s *AuditRuleStore) SetEnabled(id string, enabled bool) error {
	ctx := context.Background()
	_, err := s.db.Exec(ctx, `UPDATE audit_rules SET enabled=$1 WHERE id=$2`, enabled, id)
	if err != nil {
		return fmt.Errorf("set audit rule enabled: %w", err)
	}
	return nil
}
