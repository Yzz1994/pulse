package accesslogs

import (
	"strings"
	"sync"
	"time"
)

// MemoryStore 内存实现，用于测试。
type MemoryStore struct {
	mu      sync.Mutex
	entries []Entry
	nextID  int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Insert(entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		s.nextID++
		e.ID = s.nextID
		s.entries = append(s.entries, e)
	}
	return nil
}

func (s *MemoryStore) List(params ListParams) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.entries {
		if params.NodeID != "" && e.NodeID != params.NodeID {
			continue
		}
		if params.Username != "" && e.Username != params.Username {
			continue
		}
		if !params.Since.IsZero() && e.CreatedAt.Before(params.Since) {
			continue
		}
		if !params.Until.IsZero() && e.CreatedAt.After(params.Until) {
			continue
		}
		out = append(out, e)
	}
	// 分页
	if params.Offset > 0 {
		if params.Offset >= len(out) {
			return nil, nil
		}
		out = out[params.Offset:]
	}
	if params.Limit > 0 && len(out) > params.Limit {
		out = out[:params.Limit]
	}
	return out, nil
}

func (s *MemoryStore) ListDistinctUsers(since time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]struct{}{}
	for _, e := range s.entries {
		if !e.CreatedAt.Before(since) {
			u := e.Username
			if idx := strings.Index(u, "@"); idx >= 0 {
				u = u[:idx]
			}
			seen[u] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	return out, nil
}

func (s *MemoryStore) ListUserAnalysis(since, until time.Time) ([]UserAnalysis, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type agg struct {
		conns int64
		ips   map[string]struct{}
		last  time.Time
	}
	m := map[string]*agg{}
	for _, e := range s.entries {
		if e.CreatedAt.Before(since) || e.CreatedAt.After(until) {
			continue
		}
		user := e.Username
		if idx := len(e.Username); idx > 0 {
			for i, c := range e.Username {
				if c == '@' {
					user = e.Username[:i]
					break
				}
			}
		}
		if _, ok := m[user]; !ok {
			m[user] = &agg{ips: map[string]struct{}{}}
		}
		m[user].conns++
		m[user].ips[e.SourceIP] = struct{}{}
		if e.CreatedAt.After(m[user].last) {
			m[user].last = e.CreatedAt
		}
	}
	out := make([]UserAnalysis, 0, len(m))
	for u, a := range m {
		out = append(out, UserAnalysis{
			Username:    u,
			Connections: a.conns,
			DistinctIPs: int64(len(a.ips)),
			LastSeen:    a.last,
		})
	}
	return out, nil
}

func (s *MemoryStore) Count() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.entries)), nil
}

func (s *MemoryStore) DeleteOlderThan(t time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []Entry
	var deleted int64
	for _, e := range s.entries {
		if e.CreatedAt.Before(t) {
			deleted++
		} else {
			kept = append(kept, e)
		}
	}
	s.entries = kept
	return deleted, nil
}
