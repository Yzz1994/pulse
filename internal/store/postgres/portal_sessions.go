package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PortalSessionStore struct {
	db *pgxpool.Pool
}

func (s *PortalSessionStore) Create(token, userID string, expiresAt time.Time) error {
	_, err := s.db.Exec(context.Background(),
		`INSERT INTO portal_sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expiresAt,
	)
	return err
}

func (s *PortalSessionStore) GetUserID(token string) (string, bool) {
	var userID string
	err := s.db.QueryRow(context.Background(),
		`SELECT user_id FROM portal_sessions WHERE token = $1 AND expires_at > NOW()`, token,
	).Scan(&userID)
	if err != nil {
		return "", false
	}
	return userID, true
}

func (s *PortalSessionStore) Delete(token string) error {
	_, err := s.db.Exec(context.Background(),
		`DELETE FROM portal_sessions WHERE token = $1`, token)
	return err
}

func (s *PortalSessionStore) DeleteByUserID(userID string) error {
	_, err := s.db.Exec(context.Background(),
		`DELETE FROM portal_sessions WHERE user_id = $1`, userID)
	return err
}
