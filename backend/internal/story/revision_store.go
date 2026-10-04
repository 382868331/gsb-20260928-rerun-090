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

// RevisionStore persists every published story definition snapshot so that
// a process restart can rebuild the catalog's version history. A save that
// adopted a revision keeps resolving its pinned rules as long as the
// revision snapshot is retained here.
type RevisionStore interface {
	// SaveRevision persists one definition snapshot keyed by its content hash.
	SaveRevision(storyID, hash string, def *Definition) error
	// LoadRevision returns one snapshot.
	LoadRevision(storyID, hash string) (*Definition, error)
	// LoadAll returns every persisted snapshot, keyed by hash.
	LoadAll() (map[string]*Definition, error)
	// DeleteRevision removes one snapshot. Withdrawing a revision only
	// succeeds when no save or history record references it.
	DeleteRevision(storyID, hash string) error
	// HasRevision reports whether a snapshot is still retained.
	HasRevision(storyID, hash string) bool
}

// ErrRevisionNotFound is returned when a withdrawn or never-published
// revision snapshot is requested from the store.
var ErrRevisionNotFound = fmt.Errorf("revision snapshot not found")

// FileRevisionStore keeps one JSON file per revision under
// Dir/revisions/<story-id>/<hash>.json and a pointer file per story naming
// the hash of its current latest revision.
type FileRevisionStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileRevisionStore(dir string) (*FileRevisionStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileRevisionStore{Dir: dir}, nil
}

func revisionSafeID(id string) string {
	return strings.NewReplacer("..", "", "/", "_", "\\", "_").Replace(id)
}

func (s *FileRevisionStore) dirFor(storyID string) string {
	return filepath.Join(s.Dir, revisionSafeID(storyID))
}

func (s *FileRevisionStore) pathFor(storyID, hash string) string {
	return filepath.Join(s.dirFor(storyID), revisionSafeID(hash)+".json")
}

func (s *FileRevisionStore) SaveRevision(storyID, hash string, def *Definition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dirFor(storyID), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.pathFor(storyID, hash) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.pathFor(storyID, hash))
}

func (s *FileRevisionStore) LoadRevision(storyID, hash string) (*Definition, error) {
	b, err := os.ReadFile(s.pathFor(storyID, hash))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s@%s", ErrRevisionNotFound, storyID, hash)
	}
	if err != nil {
		return nil, err
	}
	var def Definition
	if err := json.Unmarshal(b, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

func (s *FileRevisionStore) HasRevision(storyID, hash string) bool {
	_, err := os.Stat(s.pathFor(storyID, hash))
	return err == nil
}

func (s *FileRevisionStore) DeleteRevision(storyID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.pathFor(storyID, hash))
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %s@%s", ErrRevisionNotFound, storyID, hash)
	}
	return err
}

func (s *FileRevisionStore) LoadAll() (map[string]*Definition, error) {
	out := map[string]*Definition{}
	storyDirs, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, sd := range storyDirs {
		if !sd.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(s.Dir, sd.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(s.Dir, sd.Name(), e.Name()))
			if err != nil {
				return nil, err
			}
			var def Definition
			if err := json.Unmarshal(b, &def); err != nil {
				return nil, err
			}
			hash, err := DefHash(&def)
			if err != nil {
				return nil, err
			}
			out[hash] = &def
		}
	}
	return out, nil
}

// MemoryRevisionStore keeps revision snapshots in memory and is used in tests.
type MemoryRevisionStore struct {
	mu   sync.Mutex
	snap map[string]map[string]*Definition // storyID -> hash -> def
}

func NewMemoryRevisionStore() *MemoryRevisionStore {
	return &MemoryRevisionStore{snap: map[string]map[string]*Definition{}}
}

func (s *MemoryRevisionStore) SaveRevision(storyID, hash string, def *Definition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap[storyID] == nil {
		s.snap[storyID] = map[string]*Definition{}
	}
	b, err := json.Marshal(def)
	if err != nil {
		return err
	}
	var cp Definition
	if err := json.Unmarshal(b, &cp); err != nil {
		return err
	}
	s.snap[storyID][hash] = &cp
	return nil
}

func (s *MemoryRevisionStore) LoadRevision(storyID, hash string) (*Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.snap[storyID][hash]; ok {
		b, _ := json.Marshal(d)
		var cp Definition
		_ = json.Unmarshal(b, &cp)
		return &cp, nil
	}
	return nil, fmt.Errorf("%w: %s@%s", ErrRevisionNotFound, storyID, hash)
}

func (s *MemoryRevisionStore) HasRevision(storyID, hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.snap[storyID][hash]
	return ok
}

func (s *MemoryRevisionStore) DeleteRevision(storyID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.snap[storyID][hash]; !ok {
		return fmt.Errorf("%w: %s@%s", ErrRevisionNotFound, storyID, hash)
	}
	delete(s.snap[storyID], hash)
	return nil
}

func (s *MemoryRevisionStore) LoadAll() (map[string]*Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]*Definition{}
	ids := make([]string, 0, len(s.snap))
	for id := range s.snap {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for hash, d := range s.snap[id] {
			b, _ := json.Marshal(d)
			var cp Definition
			_ = json.Unmarshal(b, &cp)
			out[hash] = &cp
		}
	}
	return out, nil
}
