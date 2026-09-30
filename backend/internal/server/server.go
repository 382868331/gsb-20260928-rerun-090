// Package server exposes the story engine over HTTP. Every state-changing rule
// (allowed options, condition re-check, reward-once, version pinning) is
// enforced here, not in the frontend.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"storyplatform/internal/store"
	"storyplatform/internal/story"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func New(st store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/stories", s.listStories)
	s.mux.HandleFunc("POST /api/stories", s.putStory)
	s.mux.HandleFunc("GET /api/stories/{id}", s.getStory)
	s.mux.HandleFunc("POST /api/stories/{id}/versions", s.putStory)

	s.mux.HandleFunc("GET /api/instances", s.listInstances)
	s.mux.HandleFunc("POST /api/instances", s.startInstance)
	s.mux.HandleFunc("GET /api/instances/{id}", s.getInstance)
	s.mux.HandleFunc("DELETE /api/instances/{id}", s.deleteInstance)
	s.mux.HandleFunc("GET /api/instances/{id}/view", s.viewInstance)
	s.mux.HandleFunc("POST /api/instances/{id}/choices", s.submitChoice)
	s.mux.HandleFunc("POST /api/instances/{id}/fork", s.forkInstance)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *Server) listStories(w http.ResponseWriter, r *http.Request) {
	defs, err := s.store.ListStories()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stories": defs})
}

func (s *Server) putStory(w http.ResponseWriter, r *http.Request) {
	var def story.Definition
	if !decode(w, r, &def) {
		return
	}
	if pathID := r.PathValue("id"); pathID != "" && def.ID == "" {
		def.ID = pathID
	}
	if err := s.store.PutStory(&def); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, _ := story.HashDefinition(&def)
	writeJSON(w, http.StatusOK, map[string]interface{}{"definition": &def, "hash": hash})
}

func (s *Server) getStory(w http.ResponseWriter, r *http.Request) {
	def, err := s.store.GetStory(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	hash, _ := story.HashDefinition(def)
	writeJSON(w, http.StatusOK, map[string]interface{}{"definition": def, "hash": hash})
}

type startRequest struct {
	StoryID string                 `json:"storyId"`
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Facts   map[string]interface{} `json:"facts"`
}

type instanceResponse struct {
	Instance *story.Instance `json:"instance"`
	View     *story.NodeView `json:"view,omitempty"`
}

func (s *Server) startInstance(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if !decode(w, r, &req) {
		return
	}
	if req.StoryID == "" {
		writeErr(w, http.StatusBadRequest, "storyId is required")
		return
	}
	def, err := s.store.GetStory(req.StoryID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	id := req.ID
	if id == "" {
		id = newID("inst")
	}
	if existing, _ := s.store.GetInstance(id); existing != nil {
		writeErr(w, http.StatusConflict, "instance id already exists: "+id)
		return
	}
	inst, err := story.StartInstance(id, def, req.Facts)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inst.Name = req.Name
	if err := s.store.SaveInstance(inst); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, _ := inst.View(def)
	writeJSON(w, http.StatusCreated, instanceResponse{Instance: inst, View: &view})
}

func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListInstances()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instances": list})
}

func (s *Server) loadInstance(w http.ResponseWriter, id string) (*story.Instance, *story.Definition) {
	inst, err := s.store.GetInstance(id)
	if err != nil {
		code := http.StatusNotFound
		if !errors.Is(err, store.ErrNotFound) {
			code = http.StatusInternalServerError
		}
		writeErr(w, code, err.Error())
		return nil, nil
	}
	def, err := s.store.DefinitionFor(inst)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pinned story version missing: "+err.Error())
		return nil, nil
	}
	return inst, def
}

func (s *Server) getInstance(w http.ResponseWriter, r *http.Request) {
	inst, def := s.loadInstance(w, r.PathValue("id"))
	if inst == nil {
		return
	}
	view, _ := inst.View(def)
	writeJSON(w, http.StatusOK, instanceResponse{Instance: inst, View: &view})
}

func (s *Server) deleteInstance(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInstance(r.PathValue("id")); err != nil {
		code := http.StatusNotFound
		if !errors.Is(err, store.ErrNotFound) {
			code = http.StatusInternalServerError
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) viewInstance(w http.ResponseWriter, r *http.Request) {
	inst, def := s.loadInstance(w, r.PathValue("id"))
	if inst == nil {
		return
	}
	view, err := inst.View(def)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instance": inst, "view": view})
}

type choiceRequest struct {
	ChoiceID  string `json:"choiceId"`
	RequestID string `json:"requestId"`
}

type forkRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) submitChoice(w http.ResponseWriter, r *http.Request) {
	var req choiceRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ChoiceID) == "" {
		writeErr(w, http.StatusBadRequest, "choiceId is required")
		return
	}
	inst, def := s.loadInstance(w, r.PathValue("id"))
	if inst == nil {
		return
	}
	result, err := inst.SubmitChoice(def, req.ChoiceID, req.RequestID)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, story.ErrChoiceDenied), errors.Is(err, story.ErrStoryEnded), errors.Is(err, story.ErrNodeNotFound):
			status = http.StatusConflict
		}
		writeErr(w, status, err.Error())
		return
	}
	// Persist before answering so a crash never loses the reward/advancement.
	if err := s.store.SaveInstance(inst); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instance": inst, "result": result})
}

func (s *Server) forkInstance(w http.ResponseWriter, r *http.Request) {
	var req forkRequest
	if !decode(w, r, &req) {
		return
	}
	parent, _ := s.loadInstance(w, r.PathValue("id"))
	if parent == nil {
		return
	}
	id := req.ID
	if id == "" {
		id = newID("fork")
	}
	if existing, _ := s.store.GetInstance(id); existing != nil {
		writeErr(w, http.StatusConflict, "instance id already exists: "+id)
		return
	}
	fork := parent.Fork(id, req.Name)
	if err := s.store.SaveInstance(fork); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"instance": fork, "parentId": parent.ID})
}
