package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/ixdomain"
	"pulse/internal/store/postgres/sqlcgen"
)

// IXDomainStore 实现 ixdomain.Store 接口。
type IXDomainStore struct {
	db *pgxpool.Pool
}

func (s *IXDomainStore) UpsertIXDomain(d ixdomain.IXDomain) (ixdomain.IXDomain, error) {
	err := sqlcgen.New(s.db).UpsertIXDomain(context.Background(), sqlcgen.UpsertIXDomainParams{
		ID:     d.ID,
		Name:   d.Name,
		Domain: d.Domain,
		Remark: d.Remark,
	})
	if err != nil {
		return ixdomain.IXDomain{}, fmt.Errorf("upsert ix domain: %w", err)
	}
	return d, nil
}

func (s *IXDomainStore) GetIXDomain(id string) (ixdomain.IXDomain, error) {
	row, err := sqlcgen.New(s.db).GetIXDomainByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ixdomain.IXDomain{}, ixdomain.ErrIXDomainNotFound
	}
	if err != nil {
		return ixdomain.IXDomain{}, fmt.Errorf("get ix domain: %w", err)
	}
	return ixdomain.IXDomain{ID: row.ID, Name: row.Name, Domain: row.Domain, Remark: row.Remark}, nil
}

func (s *IXDomainStore) ListIXDomains() ([]ixdomain.IXDomain, error) {
	rows, err := sqlcgen.New(s.db).ListIXDomains(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list ix domains: %w", err)
	}
	out := make([]ixdomain.IXDomain, len(rows))
	for i, r := range rows {
		out[i] = ixdomain.IXDomain{ID: r.ID, Name: r.Name, Domain: r.Domain, Remark: r.Remark}
	}
	return out, nil
}

func (s *IXDomainStore) DeleteIXDomain(id string) error {
	res, err := sqlcgen.New(s.db).DeleteIXDomainByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete ix domain: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ixdomain.ErrIXDomainNotFound
	}
	return nil
}
