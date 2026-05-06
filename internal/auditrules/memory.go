package auditrules

import (
	"fmt"
	"sync"
)

type MemoryStore struct {
	mu    sync.Mutex
	rules []Rule
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) List() ([]Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Rule, len(s.rules))
	copy(out, s.rules)
	return out, nil
}

func (s *MemoryStore) Insert(r Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, r)
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("rule %s not found", id)
}

func (s *MemoryStore) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("rule %s not found", id)
}
