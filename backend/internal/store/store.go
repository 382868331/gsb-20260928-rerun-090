// Package store persists story definitions and play instances as files so a
// playthrough can continue after the process restarts.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"storyplatform/internal/story"
)

var ErrNotFound = errors.New("not found")

// Store is the persistence interface used by the HTTP layer.
type Store interface {
	PutStory(def *story.Definition) error
	GetStory(id string) (*story.Definition, error)
	GetStoryVersion(id, version string) (*story.Definition, error)
	GetStoryByHash(hash string) (*story.Definition, error)
	ListStories() ([]*story.Definition, error)

	SaveInstance(inst *story.Instance) error
	GetInstance(id string) (*story.Instance, error)
	ListInstances() ([]*story.Instance, error)
	DeleteInstance(id string) error

	// DefinitionFor resolves the exact definition an instance is pinned to.
	DefinitionFor(inst *story.Instance) (*story.Definition, error)
}

// FileStore stores each record as an individual JSON file with atomic renames.
type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	for _, dir := range []string{filepath.Join(root, "stories"), filepath.Join(root, "instances")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) storyDir(id string) string {
	return filepath.Join(s.root, "stories", id)
}

func (s *FileStore) instancePath(id string) string {
	return filepath.Join(s.root, "instances", id+".json")
}

func writeJSONAtomic(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, v interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(b, v)
}

// storyRecord is the on-disk envelope for one version of a story.
type storyRecord struct {
	Definition *story.Definition
	Hash       string
}

func (s *FileStore) PutStory(def *story.Definition) error {
	if err := story.ValidateDefinition(def); err != nil {
		return err
	}
	hash, err := story.HashDefinition(def)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := storyRecord{Definition: def, Hash: hash}
	versioned := filepath.Join(s.storyDir(def.ID), "v-"+def.Version+".json")
	if err := writeJSONAtomic(versioned, rec); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(s.storyDir(def.ID), "current.json"), rec)
}

func readRecord(path string) (*story.Definition, string, error) {
	var rec storyRecord
	if err := readJSONFile(path, &rec); err != nil {
		return nil, "", err
	}
	return rec.Definition, rec.Hash, nil
}

func (s *FileStore) GetStory(id string) (*story.Definition, error) {
	def, _, err := readRecord(filepath.Join(s.storyDir(id), "current.json"))
	return def, err
}

func (s *FileStore) GetStoryVersion(id, version string) (*story.Definition, error) {
	def, _, err := readRecord(filepath.Join(s.storyDir(id), "v-"+version+".json"))
	return def, err
}

func (s *FileStore) GetStoryByHash(hash string) (*story.Definition, error) {
	root := filepath.Join(s.root, "stories")
	storyDirs, _ := os.ReadDir(root)
	for _, sd := range storyDirs {
		if !sd.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, sd.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			def, h, err := readRecord(filepath.Join(root, sd.Name(), f.Name()))
			if err == nil && h == hash {
				return def, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: story hash %s", ErrNotFound, hash)
}

func (s *FileStore) ListStories() ([]*story.Definition, error) {
	root := filepath.Join(s.root, "stories")
	dirs, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []*story.Definition
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		def, err := s.GetStory(d.Name())
		if err == nil {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *FileStore) SaveInstance(inst *story.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONAtomic(s.instancePath(inst.ID), inst)
}

func (s *FileStore) GetInstance(id string) (*story.Instance, error) {
	var inst story.Instance
	if err := readJSONFile(s.instancePath(id), &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *FileStore) ListInstances() ([]*story.Instance, error) {
	dir := filepath.Join(s.root, "instances")
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []*story.Instance
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		var inst story.Instance
		if err := readJSONFile(filepath.Join(dir, f.Name()), &inst); err == nil {
			out = append(out, &inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *FileStore) DeleteInstance(id string) error {
	err := os.Remove(s.instancePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// DefinitionFor resolves the pinned version of a story, even if a newer
// "current" version has been published since the instance started.
func (s *FileStore) DefinitionFor(inst *story.Instance) (*story.Definition, error) {
	def, err := s.GetStoryVersion(inst.StoryID, inst.Version)
	if err == nil {
		hash, _ := story.HashDefinition(def)
		if hash == inst.DefHash {
			return def, nil
		}
	}
	return s.GetStoryByHash(inst.DefHash)
}
