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

// StoredRevision is one published revision of a story definition. Revisions
// are persisted alongside the saves so a restart can restore every rule
// version any save has pinned or adopted — an adopted revision is part of
// the save's adoption decision and must survive restarts with it.
type StoredRevision struct {
	StoryID     string      `json:"storyId"`
	Hash        string      `json:"hash"`
	Version     string      `json:"version"`
	Definition  *Definition `json:"definition"`
	PublishedAt string      `json:"publishedAt"`
	// Latest marks the revision authors currently publish under. Only one
	// revision per story carries it; saves pinned to other revisions keep
	// playing under their own rules.
	Latest bool `json:"latest"`
}

// DefinitionStore persists published revisions so the catalogue's version
// history survives process restarts.
type DefinitionStore interface {
	SaveRevision(rev StoredRevision) error
	Revisions() ([]StoredRevision, error)
	DeleteRevision(hash string) error
}

// FileDefinitionStore keeps one JSON file per revision under Dir.
type FileDefinitionStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileDefinitionStore(dir string) (*FileDefinitionStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileDefinitionStore{Dir: dir}, nil
}

func (s *FileDefinitionStore) path(hash string) string {
	safe := strings.NewReplacer("..", "", "/", "_", "\\", "_").Replace(hash)
	return filepath.Join(s.Dir, safe+".json")
}

func (s *FileDefinitionStore) SaveRevision(rev StoredRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(rev.Hash) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(rev.Hash))
}

func (s *FileDefinitionStore) Revisions() ([]StoredRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]StoredRevision, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rev StoredRevision
		if err := json.Unmarshal(b, &rev); err != nil {
			return nil, fmt.Errorf("revision file %s: %w", e.Name(), err)
		}
		out = append(out, rev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt < out[j].PublishedAt })
	return out, nil
}

func (s *FileDefinitionStore) DeleteRevision(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(hash))
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrUnknownRevision, hash)
	}
	return err
}

// MemoryDefinitionStore keeps revisions in memory and is used in tests.
type MemoryDefinitionStore struct {
	mu sync.Mutex
	m  map[string]StoredRevision
}

func NewMemoryDefinitionStore() *MemoryDefinitionStore {
	return &MemoryDefinitionStore{m: map[string]StoredRevision{}}
}

func (s *MemoryDefinitionStore) SaveRevision(rev StoredRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[rev.Hash] = rev
	return nil
}

func (s *MemoryDefinitionStore) Revisions() ([]StoredRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoredRevision, 0, len(s.m))
	for _, rev := range s.m {
		out = append(out, rev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt < out[j].PublishedAt })
	return out, nil
}

func (s *MemoryDefinitionStore) DeleteRevision(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[hash]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownRevision, hash)
	}
	delete(s.m, hash)
	return nil
}
