package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/store/postgres/sqlcgen"
)

type SettingsStore struct {
	db *pgxpool.Pool
}

func (s *SettingsStore) GetSetting(key string) (string, bool) {
	value, err := sqlcgen.New(s.db).GetSetting(context.Background(), key)
	if err != nil {
		return "", false
	}
	return value, true
}

func (s *SettingsStore) SetSetting(key, value string) error {
	err := sqlcgen.New(s.db).UpsertSetting(context.Background(), sqlcgen.UpsertSettingParams{
		Key:   key,
		Value: value,
	})
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
