package enrolltokens

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryStore_InsertGetConsume(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	tok := Token{Token: "tk1", NodeID: "n1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := s.Get(ctx, "tk1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.NodeID != "n1" {
		t.Fatalf("node id = %q", got.NodeID)
	}
	consumed, err := s.Consume(ctx, "tk1")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatalf("consumed_at not set")
	}
}

func TestMemoryStore_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing: want ErrNotFound, got %v", err)
	}
	if _, err := s.Consume(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Consume missing: want ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Expired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	tok := Token{Token: "tk", NodeID: "n", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := s.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := s.Consume(ctx, "tk"); !errors.Is(err, ErrExpired) {
		t.Fatalf("Consume expired: want ErrExpired, got %v", err)
	}
}

func TestMemoryStore_AlreadyConsumed(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	tok := Token{Token: "tk", NodeID: "n", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := s.Consume(ctx, "tk"); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if _, err := s.Consume(ctx, "tk"); !errors.Is(err, ErrAlreadyConsumed) {
		t.Fatalf("second Consume: want ErrAlreadyConsumed, got %v", err)
	}
}

func TestMemoryStore_CleanupExpired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()
	_ = s.Insert(ctx, Token{Token: "old1", ExpiresAt: now.Add(-2 * time.Hour)})
	_ = s.Insert(ctx, Token{Token: "old2", ExpiresAt: now.Add(-time.Hour)})
	_ = s.Insert(ctx, Token{Token: "fresh", ExpiresAt: now.Add(time.Hour)})
	n, err := s.CleanupExpired(ctx, now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	if _, err := s.Get(ctx, "fresh"); err != nil {
		t.Fatalf("fresh should still exist: %v", err)
	}
}

func TestMemoryStore_ConcurrentConsumeOnce(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	tok := Token{Token: "race", NodeID: "n", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	const N = 100
	var wg sync.WaitGroup
	var success int32
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.Consume(ctx, "race"); err == nil {
				atomic.AddInt32(&success, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := atomic.LoadInt32(&success); got != 1 {
		t.Fatalf("concurrent Consume: %d successes, want exactly 1", got)
	}
}
