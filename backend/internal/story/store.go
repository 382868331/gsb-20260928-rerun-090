package story

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var ErrNotFound = errors.New("not found")

// Store keeps the latest definition of each story and named saves.
type Store struct {
	mu    sync.RWMutex
	dir   string
	defs  map[string]*Definition
	saves map[string]*Instance
}

func NewStore(dir string) (*Store, error) {
	s := &Store{dir: dir, defs: map[string]*Definition{}, saves: map[string]*Instance{}}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := s.loadFromDisk(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// PutDefinition validates and publishes a definition. When replacing a story
// the version must be exactly current+1, which pins existing saves to an
// older version and makes them fail resume instead of silently jumping.
func (s *Store) PutDefinition(d *Definition) error {
	if err := ValidateDefinition(d); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.defs[d.ID]; ok && d.Version != cur.Version+1 {
		return fmt.Errorf("story %q version must be %d, got %d", d.ID, cur.Version+1, d.Version)
	}
	clone := cloneJSON(d)
	s.defs[d.ID] = clone
	return s.persistLocked()
}

func (s *Store) GetDefinition(id string) (*Definition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.defs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneJSON(d), nil
}

func (s *Store) ListStories() []DefinitionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DefinitionSummary, 0, len(s.defs))
	for _, d := range s.defs {
		out = append(out, DefinitionSummary{ID: d.ID, Title: d.Title, Version: d.Version})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type DefinitionSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
}

func (s *Store) SaveGame(id string, in *Instance) error {
	if id == "" {
		return fmt.Errorf("save id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves[id] = cloneJSON(in)
	return s.persistLocked()
}

func (s *Store) LoadGame(id string) (*Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	in, ok := s.saves[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneJSON(in), nil
}

func (s *Store) DeleteGame(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.saves, id)
}

func (s *Store) ListSaves() []SaveSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SaveSummary, 0, len(s.saves))
	for id, in := range s.saves {
		out = append(out, SaveSummary{ID: id, StoryID: in.StoryID, Version: in.Version, NodeID: in.Current, Over: in.Over})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type SaveSummary struct {
	ID      string `json:"id"`
	StoryID string `json:"storyId"`
	Version int    `json:"version"`
	NodeID  string `json:"nodeId"`
	Over    bool   `json:"over"`
}

type snapshot struct {
	Definitions map[string]*Definition `json:"definitions"`
	Saves       map[string]*Instance   `json:"saves"`
}

func (s *Store) loadFromDisk() error {
	b, err := os.ReadFile(filepath.Join(s.dir, "data.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	if snap.Definitions != nil {
		s.defs = snap.Definitions
	}
	if snap.Saves != nil {
		s.saves = snap.Saves
	}
	return nil
}

func (s *Store) persistLocked() error {
	if s.dir == "" {
		return nil
	}
	snap := snapshot{Definitions: s.defs, Saves: s.saves}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "data.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "data.json"))
}

func cloneJSON[T any](v *T) *T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return &out
}
