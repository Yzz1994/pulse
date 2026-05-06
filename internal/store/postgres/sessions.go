package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/store/postgres/sqlcgen"
)

// SessionStore 将登录 session 持久化到 PostgreSQL。
type SessionStore struct {
	db *pgxpool.Pool
}

func (s *SessionStore) Create(token, username string) error {
	return sqlcgen.New(s.db).UpsertSession(context.Background(), sqlcgen.UpsertSessionParams{
		Token:     token,
		Username:  username,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *SessionStore) GetUsername(token string) (string, bool) {
	username, err := sqlcgen.New(s.db).GetSessionUsername(context.Background(), token)
	if err != nil {
		return "", false
	}
	return username, true
}

func (s *SessionStore) Delete(token string) error {
	return sqlcgen.New(s.db).DeleteSession(context.Background(), token)
}
