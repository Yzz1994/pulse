package nodes

import (
	"sort"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	nodes map[string]Node
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes: make(map[string]Node),
	}
}

func (s *MemoryStore) Upsert(node Node) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	return node, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[id]; !ok {
		return ErrNodeNotFound
	}
	delete(s.nodes, id)
	return nil
}

func (s *MemoryStore) Get(id string) (Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[id]
	if !ok {
		return Node{}, ErrNodeNotFound
	}
	return node, nil
}

func (s *MemoryStore) AddTraffic(nodeID string, upload, download int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.nodes[nodeID]
	if !ok {
		return ErrNodeNotFound
	}
	node.UploadBytes += upload
	node.DownloadBytes += download
	s.nodes[nodeID] = node
	return nil
}


func (s *MemoryStore) AddNodeDailyUsage(nodeID, date string, upload, download int64) error {
	return nil // 内存 store 仅用于测试，不持久化日统计
}

func (s *MemoryStore) ListNodeDailyUsage(days int) ([]NodeDailyUsage, error) {
	return nil, nil
}

func (s *MemoryStore) ListNodeDailyUsageRange(nodeID, since, until string) ([]NodeDailyUsage, error) {
	return nil, nil
}

func (s *MemoryStore) CleanupOldDailyUsage(retainDays int) error {
	return nil // 内存 store 不持久化日统计，无需清理
}

func (s *MemoryStore) UpsertNodeCheckResults(nodeID string, results []CheckResult) error {
	return nil // 内存 store 仅用于测试，不持久化检测结果
}

func (s *MemoryStore) ListAllNodeCheckResults() (map[string][]CheckResult, error) {
	return nil, nil
}

func (s *MemoryStore) UpsertNodeSpeedTest(nodeID string, result SpeedTestResult) error {
	return nil
}

func (s *MemoryStore) ListAllNodeSpeedTests() (map[string]SpeedTestResult, error) {
	return nil, nil
}

func (s *MemoryStore) RecordNodeUptime(nodeID string, online, running bool) error {
	return nil
}

func (s *MemoryStore) ListNodeUptimeSummary(days int) (map[string]UptimeSummary, error) {
	return nil, nil
}

func (s *MemoryStore) CleanupOldNodeUptime(retainDays int) error {
	return nil
}

func (s *MemoryStore) ListNodeUptimeBars(maxDays int) (map[string]UptimeBarsResult, error) {
	return nil, nil
}

func (s *MemoryStore) SaveTracerouteSnapshot(snapshot TracerouteSnapshot) error { return nil }
func (s *MemoryStore) ListNodeTracerouteSnapshots(nodeID string, limit int) ([]TracerouteSnapshot, error) {
	return nil, nil
}
func (s *MemoryStore) ListLatestTracerouteSnapshots() (map[string][]TracerouteSnapshot, error) {
	return nil, nil
}
func (s *MemoryStore) DeleteTracerouteSnapshot(id string) error { return nil }
func (s *MemoryStore) SaveLatencySamples(samples []LatencySample) error  { return nil }
func (s *MemoryStore) QueryLatencySamples(nodeIDs []string, from, to time.Time) ([]LatencySample, error) {
	return nil, nil
}
func (s *MemoryStore) CleanupOldLatencySamples(before time.Time) error { return nil }

func (s *MemoryStore) List() ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}
