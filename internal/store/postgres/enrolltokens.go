package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/enrolltokens"
	"pulse/internal/store/postgres/sqlcgen"
)

// EnrollTokenStore 实现 enrolltokens.Store 接口。
type EnrollTokenStore struct {
	db *pgxpool.Pool
}

// EnrollTokenStore 返回 enroll_tokens 表的 Store。
func (db *DB) EnrollTokenStore() *EnrollTokenStore {
	return &EnrollTokenStore{db: db.conn}
}

func (s *EnrollTokenStore) Insert(ctx context.Context, t enrolltokens.Token) error {
	return sqlcgen.New(s.db).InsertEnrollToken(ctx, sqlcgen.InsertEnrollTokenParams{
		Token:     t.Token,
		NodeID:    t.NodeID,
		ExpiresAt: t.ExpiresAt,
	})
}

func (s *EnrollTokenStore) Get(ctx context.Context, token string) (enrolltokens.Token, error) {
	row, err := sqlcgen.New(s.db).GetEnrollToken(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return enrolltokens.Token{}, enrolltokens.ErrNotFound
	}
	if err != nil {
		return enrolltokens.Token{}, err
	}
	return toEnrollToken(row), nil
}

func (s *EnrollTokenStore) Consume(ctx context.Context, token string) (enrolltokens.Token, error) {
	q := sqlcgen.New(s.db)
	res, err := q.ConsumeEnrollToken(ctx, token)
	if err != nil {
		return enrolltokens.Token{}, err
	}
	if res.RowsAffected() == 1 {
		row, err := q.GetEnrollToken(ctx, token)
		if err != nil {
			return enrolltokens.Token{}, err
		}
		return toEnrollToken(row), nil
	}
	// 0 rows affected: 找出原因
	row, err := q.GetEnrollToken(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return enrolltokens.Token{}, enrolltokens.ErrNotFound
	}
	if err != nil {
		return enrolltokens.Token{}, err
	}
	if row.ConsumedAt.Valid {
		return enrolltokens.Token{}, enrolltokens.ErrAlreadyConsumed
	}
	return enrolltokens.Token{}, enrolltokens.ErrExpired
}

func (s *EnrollTokenStore) CleanupExpired(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := sqlcgen.New(s.db).CleanupExpiredEnrollTokens(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}

func toEnrollToken(r sqlcgen.EnrollToken) enrolltokens.Token {
	t := enrolltokens.Token{
		Token:     r.Token,
		NodeID:    r.NodeID,
		ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt,
	}
	if r.ConsumedAt.Valid {
		v := r.ConsumedAt.Time
		t.ConsumedAt = &v
	}
	return t
}
