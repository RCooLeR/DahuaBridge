package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

type ProbeStore struct {
	mu       sync.RWMutex
	results  map[string]probeEntry
	revision uint64
	dirty    bool
}

type probeEntry struct {
	Result    *dahua.ProbeResult `json:"result"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Snapshot struct {
	Version int                   `json:"version"`
	SavedAt time.Time             `json:"saved_at"`
	Results map[string]probeEntry `json:"results"`
}

type Stats struct {
	DeviceCount   int       `json:"device_count"`
	LastUpdatedAt time.Time `json:"last_updated_at,omitempty"`
}

func NewProbeStore() *ProbeStore {
	return &ProbeStore{
		results: make(map[string]probeEntry),
	}
}

func (s *ProbeStore) Set(id string, result *dahua.ProbeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[id] = probeEntry{
		Result:    cloneProbeResult(result),
		UpdatedAt: time.Now().UTC(),
	}
	s.markDirty()
}

func (s *ProbeStore) Get(id string) (*dahua.ProbeResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.results[id]
	if !ok {
		return nil, false
	}
	return cloneProbeResult(entry.Result), true
}

func (s *ProbeStore) Update(id string, mutate func(*dahua.ProbeResult)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.results[id]
	if !ok || entry.Result == nil {
		return false
	}

	mutate(entry.Result)
	entry.UpdatedAt = time.Now().UTC()
	s.results[id] = entry
	s.markDirty()
	return true
}

func (s *ProbeStore) List() []*dahua.ProbeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.results))
	for key := range s.results {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]*dahua.ProbeResult, 0, len(keys))
	for _, key := range keys {
		items = append(items, cloneProbeResult(s.results[key].Result))
	}
	return items
}

func (s *ProbeStore) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := Stats{
		DeviceCount: len(s.results),
	}
	for _, entry := range s.results {
		if entry.UpdatedAt.After(stats.LastUpdatedAt) {
			stats.LastUpdatedAt = entry.UpdatedAt
		}
	}
	return stats
}

func (s *ProbeStore) SaveFile(path string) error {
	snapshot, revision, dirty := s.snapshot()
	if !dirty {
		return nil
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revision == revision {
		s.dirty = false
	}
	return nil
}

func (s *ProbeStore) LoadFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.results = make(map[string]probeEntry, len(snapshot.Results))
	for id, entry := range snapshot.Results {
		s.results[id] = probeEntry{
			Result:    cloneProbeResult(entry.Result),
			UpdatedAt: entry.UpdatedAt,
		}
	}
	s.dirty = false
	return true, nil
}

func (s *ProbeStore) snapshot() (Snapshot, uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string]probeEntry, len(s.results))
	for id, entry := range s.results {
		results[id] = probeEntry{
			Result:    cloneProbeResult(entry.Result),
			UpdatedAt: entry.UpdatedAt,
		}
	}

	return Snapshot{
		Version: 1,
		SavedAt: time.Now().UTC(),
		Results: results,
	}, s.revision, s.dirty
}

func (s *ProbeStore) markDirty() {
	s.revision++
	s.dirty = true
}

func cloneProbeResult(input *dahua.ProbeResult) *dahua.ProbeResult {
	if input == nil {
		return nil
	}

	out := &dahua.ProbeResult{
		Root:     input.Root,
		Children: make([]dahua.Device, len(input.Children)),
		States:   make(map[string]dahua.DeviceState, len(input.States)),
		Raw:      make(map[string]string, len(input.Raw)),
	}

	out.Root.Attributes = cloneStringMap(input.Root.Attributes)
	for i, child := range input.Children {
		out.Children[i] = child
		out.Children[i].Attributes = cloneStringMap(child.Attributes)
	}
	for key, value := range input.States {
		out.States[key] = dahua.DeviceState{
			Available: value.Available,
			Info:      cloneAnyMap(value.Info),
		}
	}
	for key, value := range input.Raw {
		out.Raw[key] = value
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnySlice(input []any) []any {
	if len(input) == 0 {
		return nil
	}
	out := make([]any, len(input))
	for i, value := range input {
		out[i] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneAnyMap(v)
	case []any:
		return cloneAnySlice(v)
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	default:
		return v
	}
}
