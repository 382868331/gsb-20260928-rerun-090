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

// FileDefStore persists story definition revisions so adoption decisions
// survive restarts: every revision a save adopted (or is about to adopt) is
// written next to the saves and reloaded into the catalog's history on boot.
type FileDefStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileDefStore(dir string) (*FileDefStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileDefStore{Dir: dir}, nil
}

func (s *FileDefStore) path(hash string) string {
	return filepath.Join(s.Dir, safeName("rev-"+hash))
}

func (s *FileDefStore) latestPath(storyID string) string {
	return filepath.Join(s.Dir, safeName("latest-"+storyID))
}

// SaveDef writes a revision snapshot keyed by its content hash and marks it
// as the story's latest published revision.
func (s *FileDefStore) SaveDef(d *Definition) (string, error) {
	hash, err := DefHash(d)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := s.path(hash) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, s.path(hash)); err != nil {
		return "", err
	}
	ltmp := s.latestPath(d.ID) + ".tmp"
	if err := os.WriteFile(ltmp, []byte(hash), 0o644); err != nil {
		return "", err
	}
	return hash, os.Rename(ltmp, s.latestPath(d.ID))
}

// KeepDef persists a revision snapshot without touching the latest marker.
// Used when a save adopts an older revision: the definition the save now
// depends on must survive restarts exactly as decided.
func (s *FileDefStore) KeepDef(d *Definition) (string, error) {
	hash, err := DefHash(d)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := s.path(hash) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	return hash, os.Rename(tmp, s.path(hash))
}

// DeleteDef removes a withdrawn revision snapshot. The latest marker is
// never deleted here; the catalog refuses to withdraw in-use revisions.
func (s *FileDefStore) DeleteDef(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(hash))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadAll reads every persisted revision plus each story's latest marker.
func (s *FileDefStore) LoadAll() (defs []*Definition, latest map[string]string, err error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, nil, err
	}
	latest = map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, nil, err
		}
		if strings.HasPrefix(e.Name(), "latest-") {
			storyID := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "latest-"), ".json")
			latest[storyID] = strings.TrimSpace(string(b))
			continue
		}
		var d Definition
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, nil, err
		}
		defs = append(defs, &d)
	}
	return defs, latest, nil
}

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
