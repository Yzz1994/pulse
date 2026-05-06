package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/enrolltokens"
)

// setupEnrollTokenStore 在有 PULSE_TEST_DATABASE_URL 环境变量时返回连接好的 Store。
// 否则跳过当前测试。
func setupEnrollTokenStore(t *testing.T) (*EnrollTokenStore, func()) {
	t.Helper()
	dsn := os.Getenv("PULSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PULSE_TEST_DATABASE_URL not set; skipping postgres integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	db := &DB{conn: pool}
	if err := db.init(); err != nil {
		pool.Close()
		t.Fatalf("init schema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM enroll_tokens`); err != nil {
		pool.Close()
		t.Fatalf("clean enroll_tokens: %v", err)
	}
	return db.EnrollTokenStore(), func() { pool.Close() }
}

func TestPGEnrollTokenStore_FullFlow(t *testing.T) {
	store, cleanup := setupEnrollTokenStore(t)
	defer cleanup()
	ctx := context.Background()

	tok := enrolltokens.Token{
		Token:     "tk-flow",
		NodeID:    "node1",
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if err := store.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := store.Get(ctx, "tk-flow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.NodeID != "node1" {
		t.Fatalf("node id = %q", got.NodeID)
	}
	if got.ConsumedAt != nil {
		t.Fatalf("consumed_at should be nil, got %v", got.ConsumedAt)
	}
	consumed, err := store.Consume(ctx, "tk-flow")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatalf("consumed_at should be set after Consume")
	}
}

func TestPGEnrollTokenStore_Expired(t *testing.T) {
	store, cleanup := setupEnrollTokenStore(t)
	defer cleanup()
	ctx := context.Background()
	tok := enrolltokens.Token{
		Token:     "tk-expired",
		NodeID:    "node",
		ExpiresAt: time.Now().Add(-time.Minute).UTC(),
	}
	if err := store.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := store.Consume(ctx, "tk-expired"); !errors.Is(err, enrolltokens.ErrExpired) {
		t.Fatalf("Consume expired: want ErrExpired, got %v", err)
	}
}

func TestPGEnrollTokenStore_AlreadyConsumed(t *testing.T) {
	store, cleanup := setupEnrollTokenStore(t)
	defer cleanup()
	ctx := context.Background()
	tok := enrolltokens.Token{
		Token:     "tk-twice",
		NodeID:    "node",
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if err := store.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := store.Consume(ctx, "tk-twice"); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if _, err := store.Consume(ctx, "tk-twice"); !errors.Is(err, enrolltokens.ErrAlreadyConsumed) {
		t.Fatalf("second Consume: want ErrAlreadyConsumed, got %v", err)
	}
}

func TestPGEnrollTokenStore_NotFound(t *testing.T) {
	store, cleanup := setupEnrollTokenStore(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, enrolltokens.ErrNotFound) {
		t.Fatalf("Get missing: want ErrNotFound, got %v", err)
	}
	if _, err := store.Consume(ctx, "missing"); !errors.Is(err, enrolltokens.ErrNotFound) {
		t.Fatalf("Consume missing: want ErrNotFound, got %v", err)
	}
}

func TestPGEnrollTokenStore_CleanupExpired(t *testing.T) {
	store, cleanup := setupEnrollTokenStore(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()
	_ = store.Insert(ctx, enrolltokens.Token{Token: "c-old1", NodeID: "n", ExpiresAt: now.Add(-2 * time.Hour)})
	_ = store.Insert(ctx, enrolltokens.Token{Token: "c-old2", NodeID: "n", ExpiresAt: now.Add(-time.Hour)})
	_ = store.Insert(ctx, enrolltokens.Token{Token: "c-fresh", NodeID: "n", ExpiresAt: now.Add(time.Hour)})
	n, err := store.CleanupExpired(ctx, now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	if _, err := store.Get(ctx, "c-fresh"); err != nil {
		t.Fatalf("fresh missing: %v", err)
	}
}
