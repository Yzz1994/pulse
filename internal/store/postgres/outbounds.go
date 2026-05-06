package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/outbounds"
	"pulse/internal/store/postgres/sqlcgen"
)

// OutboundStore 实现 outbounds.Store 接口。
type OutboundStore struct {
	db *pgxpool.Pool
}

func (db *DB) OutboundStore() *OutboundStore {
	return &OutboundStore{db: db.conn}
}

func (s *OutboundStore) Upsert(ob outbounds.Outbound) (outbounds.Outbound, error) {
	err := sqlcgen.New(s.db).UpsertOutbound(context.Background(), sqlcgen.UpsertOutboundParams{
		ID:          ob.ID,
		Name:        ob.Name,
		Protocol:    ob.Protocol,
		Server:      ob.Server,
		Username:    ob.Username,
		Password:    ob.Password,
		Method:      ob.Method,
		Uuid:        ob.UUID,
		Sni:         ob.SNI,
		PublicKey:   ob.PublicKey,
		ShortID:     ob.ShortID,
		Fingerprint: ob.Fingerprint,
		Flow:        ob.Flow,
	})
	if err != nil {
		return outbounds.Outbound{}, fmt.Errorf("upsert outbound: %w", err)
	}
	return ob, nil
}

func (s *OutboundStore) Get(id string) (outbounds.Outbound, error) {
	row, err := sqlcgen.New(s.db).GetOutboundByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbounds.Outbound{}, outbounds.ErrOutboundNotFound
	}
	if err != nil {
		return outbounds.Outbound{}, fmt.Errorf("get outbound: %w", err)
	}
	return toOutbound(row), nil
}

func (s *OutboundStore) List() ([]outbounds.Outbound, error) {
	rows, err := sqlcgen.New(s.db).ListOutbounds(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list outbounds: %w", err)
	}
	out := make([]outbounds.Outbound, len(rows))
	for i, r := range rows {
		out[i] = toOutbound(r)
	}
	return out, nil
}

func (s *OutboundStore) Delete(id string) error {
	result, err := sqlcgen.New(s.db).DeleteOutboundByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete outbound: %w", err)
	}
	if result.RowsAffected() == 0 {
		return outbounds.ErrOutboundNotFound
	}
	return nil
}

func toOutbound(r sqlcgen.Outbound) outbounds.Outbound {
	return outbounds.Outbound{
		ID:          r.ID,
		Name:        r.Name,
		Protocol:    r.Protocol,
		Server:      r.Server,
		Username:    r.Username,
		Password:    r.Password,
		Method:      r.Method,
		UUID:        r.Uuid,
		SNI:         r.Sni,
		PublicKey:   r.PublicKey,
		ShortID:     r.ShortID,
		Fingerprint: r.Fingerprint,
		Flow:        r.Flow,
	}
}
