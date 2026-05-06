package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/routerules"
	"pulse/internal/store/postgres/sqlcgen"
)

// RouteRuleStore 实现 routerules.Store 接口。
type RouteRuleStore struct {
	db *pgxpool.Pool
}

func (db *DB) RouteRuleStore() *RouteRuleStore {
	return &RouteRuleStore{db: db.conn}
}

func (s *RouteRuleStore) Upsert(rule routerules.RouteRule) (routerules.RouteRule, error) {
	err := sqlcgen.New(s.db).UpsertRouteRule(context.Background(), sqlcgen.UpsertRouteRuleParams{
		ID:            rule.ID,
		Name:          rule.Name,
		RuleType:      rule.RuleType,
		Patterns:      rule.Patterns,
		OutboundID:    rule.OutboundID,
		Priority:      int32(rule.Priority),
		RuleSetUrl:    rule.RuleSetURL,
		RuleSetFormat: rule.RuleSetFormat,
		NodeIds:       rule.NodeIDs,
		InboundIds:    rule.InboundIDs,
	})
	if err != nil {
		return routerules.RouteRule{}, fmt.Errorf("upsert route rule: %w", err)
	}
	return rule, nil
}

func (s *RouteRuleStore) Get(id string) (routerules.RouteRule, error) {
	row, err := sqlcgen.New(s.db).GetRouteRuleByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return routerules.RouteRule{}, routerules.ErrNotFound
	}
	if err != nil {
		return routerules.RouteRule{}, fmt.Errorf("get route rule: %w", err)
	}
	return routerules.RouteRule{
		ID: row.ID, Name: row.Name, RuleType: row.RuleType, Patterns: row.Patterns,
		OutboundID: row.OutboundID, Priority: int(row.Priority),
		RuleSetURL: row.RuleSetUrl, RuleSetFormat: row.RuleSetFormat,
		NodeIDs: row.NodeIds, InboundIDs: row.InboundIds,
	}, nil
}

func (s *RouteRuleStore) List() ([]routerules.RouteRule, error) {
	rows, err := sqlcgen.New(s.db).ListRouteRules(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list route rules: %w", err)
	}
	out := make([]routerules.RouteRule, len(rows))
	for i, r := range rows {
		out[i] = routerules.RouteRule{
			ID: r.ID, Name: r.Name, RuleType: r.RuleType, Patterns: r.Patterns,
			OutboundID: r.OutboundID, Priority: int(r.Priority),
			RuleSetURL: r.RuleSetUrl, RuleSetFormat: r.RuleSetFormat,
			NodeIDs: r.NodeIds, InboundIDs: r.InboundIds,
		}
	}
	return out, nil
}

func (s *RouteRuleStore) Delete(id string) error {
	result, err := sqlcgen.New(s.db).DeleteRouteRuleByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete route rule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return routerules.ErrNotFound
	}
	return nil
}
