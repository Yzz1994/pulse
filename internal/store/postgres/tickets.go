package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/store/postgres/sqlcgen"
	"pulse/internal/tickets"
)

// TicketStore PostgreSQL 实现。
type TicketStore struct {
	db *pgxpool.Pool
}

func (s *TicketStore) CreateTicket(t *tickets.Ticket) error {
	now := time.Now().UTC()
	err := sqlcgen.New(s.db).InsertTicket(context.Background(), sqlcgen.InsertTicketParams{
		ID:        t.ID,
		UserID:    t.UserID,
		Username:  t.Username,
		Title:     t.Title,
		Status:    string(t.Status),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

func (s *TicketStore) GetTicket(id string) (*tickets.Ticket, error) {
	row, err := sqlcgen.New(s.db).GetTicketByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("ticket %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	return toTicket(row), nil
}

func (s *TicketStore) ListTickets() ([]tickets.Ticket, error) {
	rows, err := sqlcgen.New(s.db).ListTickets(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Ticket, len(rows))
	for i, r := range rows {
		out[i] = *toTicket(r)
	}
	return out, nil
}

func (s *TicketStore) ListTicketsByUser(userID string) ([]tickets.Ticket, error) {
	rows, err := sqlcgen.New(s.db).ListTicketsByUser(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Ticket, len(rows))
	for i, r := range rows {
		out[i] = *toTicket(r)
	}
	return out, nil
}

func (s *TicketStore) UpdateTicketStatus(id string, status tickets.TicketStatus) error {
	return sqlcgen.New(s.db).UpdateTicketStatus(context.Background(), sqlcgen.UpdateTicketStatusParams{
		Status:    string(status),
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
}

func (s *TicketStore) AddMessage(m *tickets.Message) error {
	now := time.Now().UTC()
	ctx := context.Background()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlcgen.New(tx)
	if err := q.InsertTicketMessage(ctx, sqlcgen.InsertTicketMessageParams{
		ID:        m.ID,
		TicketID:  m.TicketID,
		Content:   m.Content,
		IsAdmin:   m.IsAdmin,
		CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("add ticket message: %w", err)
	}
	if err := q.UpdateTicketUpdatedAt(ctx, sqlcgen.UpdateTicketUpdatedAtParams{
		UpdatedAt: now,
		ID:        m.TicketID,
	}); err != nil {
		return fmt.Errorf("update ticket updated_at: %w", err)
	}
	m.CreatedAt = now
	return tx.Commit(ctx)
}

func (s *TicketStore) ListMessages(ticketID string) ([]tickets.Message, error) {
	rows, err := sqlcgen.New(s.db).ListTicketMessages(context.Background(), ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Message, len(rows))
	for i, r := range rows {
		out[i] = tickets.Message{
			ID:        r.ID,
			TicketID:  r.TicketID,
			Content:   r.Content,
			IsAdmin:   r.IsAdmin,
			CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (s *TicketStore) AddImage(img *tickets.Image) error {
	now := time.Now().UTC()
	err := sqlcgen.New(s.db).InsertTicketImage(context.Background(), sqlcgen.InsertTicketImageParams{
		ID:         img.ID,
		TicketID:   img.TicketID,
		Filename:   img.Filename,
		StoredName: img.StoredName,
		Size:       img.Size,
		CreatedAt:  now,
	})
	if err != nil {
		return fmt.Errorf("add ticket image: %w", err)
	}
	img.CreatedAt = now
	return nil
}

func (s *TicketStore) ListImages(ticketID string) ([]tickets.Image, error) {
	rows, err := sqlcgen.New(s.db).ListTicketImages(context.Background(), ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Image, len(rows))
	for i, r := range rows {
		out[i] = tickets.Image{
			ID:         r.ID,
			TicketID:   r.TicketID,
			Filename:   r.Filename,
			StoredName: r.StoredName,
			Size:       r.Size,
			CreatedAt:  r.CreatedAt,
		}
	}
	return out, nil
}

// ─── 类型转换辅助 ─────────────────────────────────────────────────────────────

func toTicket(r sqlcgen.Ticket) *tickets.Ticket {
	return &tickets.Ticket{
		ID:        r.ID,
		UserID:    r.UserID,
		Username:  r.Username,
		Title:     r.Title,
		Status:    tickets.TicketStatus(r.Status),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
