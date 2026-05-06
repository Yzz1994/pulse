package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/announcements"
	"pulse/internal/store/postgres/sqlcgen"
)

// AnnouncementStore PostgreSQL 实现。
type AnnouncementStore struct {
	db *pgxpool.Pool
}

func (s *AnnouncementStore) Create(a *announcements.Announcement) error {
	now := time.Now().UTC()
	return sqlcgen.New(s.db).InsertAnnouncement(context.Background(), sqlcgen.InsertAnnouncementParams{
		ID:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Enabled:   a.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *AnnouncementStore) Update(a *announcements.Announcement) error {
	return sqlcgen.New(s.db).UpdateAnnouncement(context.Background(), sqlcgen.UpdateAnnouncementParams{
		Title:     a.Title,
		Content:   a.Content,
		Enabled:   a.Enabled,
		UpdatedAt: time.Now().UTC(),
		ID:        a.ID,
	})
}

func (s *AnnouncementStore) Delete(id string) error {
	return sqlcgen.New(s.db).DeleteAnnouncementByID(context.Background(), id)
}

func (s *AnnouncementStore) Get(id string) (*announcements.Announcement, error) {
	row, err := sqlcgen.New(s.db).GetAnnouncementByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	return toAnnouncement(row), nil
}

func (s *AnnouncementStore) List() ([]announcements.Announcement, error) {
	rows, err := sqlcgen.New(s.db).ListAnnouncements(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]announcements.Announcement, len(rows))
	for i, r := range rows {
		out[i] = *toAnnouncement(r)
	}
	return out, nil
}

func (s *AnnouncementStore) GetActive() (*announcements.Announcement, error) {
	row, err := sqlcgen.New(s.db).GetActiveAnnouncement(context.Background())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toAnnouncement(row), nil
}

func (s *AnnouncementStore) SetActive(id string) error {
	ctx := context.Background()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC()
	q := sqlcgen.New(tx)
	if err := q.DisableAllAnnouncements(ctx, now); err != nil {
		return err
	}
	if err := q.EnableAnnouncement(ctx, sqlcgen.EnableAnnouncementParams{
		UpdatedAt: now,
		ID:        id,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AnnouncementStore) Disable(id string) error {
	return sqlcgen.New(s.db).DisableAnnouncement(context.Background(), sqlcgen.DisableAnnouncementParams{
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
}

// ─── 类型转换辅助 ─────────────────────────────────────────────────────────────

func toAnnouncement(r sqlcgen.Announcement) *announcements.Announcement {
	return &announcements.Announcement{
		ID:        r.ID,
		Title:     r.Title,
		Content:   r.Content,
		Enabled:   r.Enabled,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
