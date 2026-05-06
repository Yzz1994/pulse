package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/cfdomain"
	"pulse/internal/store/postgres/sqlcgen"
)

// CFDomainStore 实现 cfdomain.Store 接口。
type CFDomainStore struct {
	db *pgxpool.Pool
}

func (s *CFDomainStore) UpsertCFDomain(domain cfdomain.CFDomain) (cfdomain.CFDomain, error) {
	err := sqlcgen.New(s.db).UpsertCFDomain(context.Background(), sqlcgen.UpsertCFDomainParams{
		ID:       domain.ID,
		CfToken:  domain.CFToken,
		ZoneID:   domain.ZoneID,
		ZoneName: domain.ZoneName,
		Remark:   &domain.Remark,
	})
	if err != nil {
		return cfdomain.CFDomain{}, fmt.Errorf("upsert cf domain: %w", err)
	}
	return domain, nil
}

func (s *CFDomainStore) GetCFDomain(id string) (cfdomain.CFDomain, error) {
	row, err := sqlcgen.New(s.db).GetCFDomainByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return cfdomain.CFDomain{}, cfdomain.ErrCFDomainNotFound
	}
	if err != nil {
		return cfdomain.CFDomain{}, fmt.Errorf("get cf domain: %w", err)
	}
	return cfdomain.CFDomain{ID: row.ID, CFToken: row.CfToken, ZoneID: row.ZoneID, ZoneName: row.ZoneName, Remark: row.Remark}, nil
}

func (s *CFDomainStore) ListCFDomains() ([]cfdomain.CFDomain, error) {
	rows, err := sqlcgen.New(s.db).ListCFDomains(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list cf domains: %w", err)
	}
	out := make([]cfdomain.CFDomain, len(rows))
	for i, r := range rows {
		out[i] = cfdomain.CFDomain{ID: r.ID, CFToken: r.CfToken, ZoneID: r.ZoneID, ZoneName: r.ZoneName, Remark: r.Remark}
	}
	return out, nil
}

func (s *CFDomainStore) DeleteCFDomain(id string) error {
	res, err := sqlcgen.New(s.db).DeleteCFDomainByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete cf domain: %w", err)
	}
	if res.RowsAffected() == 0 {
		return cfdomain.ErrCFDomainNotFound
	}
	return nil
}
