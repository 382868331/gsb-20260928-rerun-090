package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store is the persistence boundary for play instances. Implementations must
// be safe for concurrent use; the file store serialises writes with a mutex
// so a crash during save cannot duplicate rewards.
type Store interface {
	Save(inst *Instance) error
	Load(id string) (*Instance, error)
	Delete(id string) error
	List() ([]*Instance, error)
}

// FileStore persists one instance per JSON file under Dir.
type FileStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileStore{Dir: dir}, nil
}

func safeName(id string) string {
	id = strings.NewReplacer("..", "", "/", "_", "\\", "_").Replace(id)
	return id + ".json"
}

func (s *FileStore) path(id string) string {
	return filepath.Join(s.Dir, safeName(id))
}

func (s *FileStore) Save(inst *Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(inst.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(inst.ID))
}

func (s *FileStore) Load(id string) (*Instance, error) {
	b, err := os.ReadFile(s.path(id))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrUnknownInstance, id)
	}
	if err != nil {
		return nil, err
	}
	var inst Instance
	if err := json.Unmarshal(b, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *FileStore) Delete(id string) error {
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrUnknownInstance, id)
	}
	return err
}

func (s *FileStore) List() ([]*Instance, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]*Instance, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var inst Instance
		if err := json.Unmarshal(b, &inst); err != nil {
			return nil, err
		}
		out = append(out, &inst)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// MemoryStore keeps instances in memory and is used in tests.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string]*Instance
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: map[string]*Instance{}}
}

func (s *MemoryStore) Save(inst *Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	var cp Instance
	if err := json.Unmarshal(b, &cp); err != nil {
		return err
	}
	s.m[inst.ID] = &cp
	return nil
}

func (s *MemoryStore) Load(id string) (*Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.m[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownInstance, id)
	}
	b, _ := json.Marshal(inst)
	var cp Instance
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownInstance, id)
	}
	delete(s.m, id)
	return nil
}

func (s *MemoryStore) List() ([]*Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.m))
	for id := range s.m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Instance, 0, len(ids))
	for _, id := range ids {
		b, _ := json.Marshal(s.m[id])
		var cp Instance
		_ = json.Unmarshal(b, &cp)
		out = append(out, &cp)
	}
	return out, nil
}
