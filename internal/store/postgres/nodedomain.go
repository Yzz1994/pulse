package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/nodedomain"
	"pulse/internal/store/postgres/sqlcgen"
)

// NodeDomainStore 实现 nodedomain.Store 接口。
type NodeDomainStore struct {
	db *pgxpool.Pool
}

func fromRow(r sqlcgen.NodeDomain) nodedomain.NodeDomain {
	return nodedomain.NodeDomain{
		ID:         r.ID,
		NodeID:     r.NodeID,
		CFDomainID: r.CfDomainID,
		FQDN:       r.Fqdn,
		RecordType: r.RecordType,
		Content:    r.Content,
		Proxied:    r.Proxied,
		SyncedAt:   r.SyncedAt,
	}
}

func (s *NodeDomainStore) Upsert(nd nodedomain.NodeDomain) (nodedomain.NodeDomain, error) {
	row, err := sqlcgen.New(s.db).UpsertNodeDomain(context.Background(), sqlcgen.UpsertNodeDomainParams{
		ID:         nd.ID,
		NodeID:     nd.NodeID,
		CfDomainID: nd.CFDomainID,
		Fqdn:       nd.FQDN,
		RecordType: nd.RecordType,
		Content:    nd.Content,
		Proxied:    nd.Proxied,
	})
	if err != nil {
		return nodedomain.NodeDomain{}, fmt.Errorf("upsert node domain: %w", err)
	}
	return fromRow(row), nil
}

func (s *NodeDomainStore) List() ([]nodedomain.NodeDomain, error) {
	rows, err := sqlcgen.New(s.db).ListNodeDomains(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list node domains: %w", err)
	}
	out := make([]nodedomain.NodeDomain, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

func (s *NodeDomainStore) ListByCFDomain(cfDomainID string) ([]nodedomain.NodeDomain, error) {
	rows, err := sqlcgen.New(s.db).ListNodeDomainsByCFDomain(context.Background(), cfDomainID)
	if err != nil {
		return nil, fmt.Errorf("list node domains by cf domain: %w", err)
	}
	out := make([]nodedomain.NodeDomain, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

func (s *NodeDomainStore) UpdateNodeID(id, nodeID string) (nodedomain.NodeDomain, error) {
	row, err := sqlcgen.New(s.db).UpdateNodeDomainNode(context.Background(), sqlcgen.UpdateNodeDomainNodeParams{
		ID:     id,
		NodeID: nodeID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nodedomain.NodeDomain{}, nodedomain.ErrNotFound
	}
	if err != nil {
		return nodedomain.NodeDomain{}, fmt.Errorf("update node domain node: %w", err)
	}
	return fromRow(row), nil
}

func (s *NodeDomainStore) Delete(id string) error {
	res, err := sqlcgen.New(s.db).DeleteNodeDomain(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete node domain: %w", err)
	}
	if res.RowsAffected() == 0 {
		return nodedomain.ErrNotFound
	}
	return nil
}
