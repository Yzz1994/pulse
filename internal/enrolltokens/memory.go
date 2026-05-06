package enrolltokens

import (
	"context"
	"sync"
	"time"
)

// MemoryStore 是用于测试的内存实现。
type MemoryStore struct {
	mu     sync.RWMutex
	tokens map[string]Token
	now    func() time.Time
}

// NewMemoryStore 创建内存 Store。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tokens: make(map[string]Token),
		now:    time.Now,
	}
}

func (s *MemoryStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *MemoryStore) Insert(_ context.Context, t Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = s.clock()
	}
	s.tokens[t.Token] = t
	return nil
}

func (s *MemoryStore) Get(_ context.Context, token string) (Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[token]
	if !ok {
		return Token{}, ErrNotFound
	}
	return cloneToken(t), nil
}

func (s *MemoryStore) Consume(_ context.Context, token string) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[token]
	if !ok {
		return Token{}, ErrNotFound
	}
	if t.ConsumedAt != nil {
		return Token{}, ErrAlreadyConsumed
	}
	now := s.clock()
	if !t.ExpiresAt.After(now) {
		return Token{}, ErrExpired
	}
	consumed := now
	t.ConsumedAt = &consumed
	s.tokens[token] = t
	return cloneToken(t), nil
}

func (s *MemoryStore) CleanupExpired(_ context.Context, cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for k, v := range s.tokens {
		if v.ExpiresAt.Before(cutoff) {
			delete(s.tokens, k)
			n++
		}
	}
	return n, nil
}

func cloneToken(t Token) Token {
	if t.ConsumedAt != nil {
		c := *t.ConsumedAt
		t.ConsumedAt = &c
	}
	return t
}
