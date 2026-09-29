package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"storyplatform/internal/story"
)

type Server struct {
	store *story.Store
	mux   *http.ServeMux
}

func New(store *story.Store) *Server {
	s := &Server{store: store, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/stories", s.listStories)
	s.mux.HandleFunc("GET /api/stories/{id}", s.getStory)
	s.mux.HandleFunc("PUT /api/stories/{id}", s.putStory)
	s.mux.HandleFunc("POST /api/stories/{id}/start", s.start)
	s.mux.HandleFunc("POST /api/stories/{id}/choose", s.choose)
	s.mux.HandleFunc("GET /api/saves", s.listSaves)
	s.mux.HandleFunc("GET /api/saves/{saveId}", s.getSave)
	s.mux.HandleFunc("DELETE /api/saves/{saveId}", s.deleteSave)
	s.mux.HandleFunc("POST /api/stories/{id}/saves/{saveId}/resume", s.resume)
}

type startResponse struct {
	SaveID string          `json:"saveId"`
	Save   *story.Instance `json:"save"`
	View   *story.View     `json:"view"`
}

type chooseRequest struct {
	SaveID string `json:"saveId"`
	Choice string `json:"choice"`
}

type saveRequest struct {
	SaveID string          `json:"saveId"`
	Save   *story.Instance `json:"save"`
}

func (s *Server) listStories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.ListStories())
}

func (s *Server) getStory(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDefinition(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) putStory(w http.ResponseWriter, r *http.Request) {
	var d story.Definition
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeMsg(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ID = r.PathValue("id")
	if err := s.store.PutDefinition(&d); err != nil {
		writeValidation(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDefinition(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		SaveID string `json:"saveId"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeMsg(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	inst, v, err := story.Start(d)
	if err != nil {
		writeValidation(w, err)
		return
	}
	saveID := req.SaveID
	if saveID == "" {
		saveID = "autosave"
	}
	if err := s.store.SaveGame(saveID, inst); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, startResponse{SaveID: saveID, Save: inst, View: v})
}

func (s *Server) choose(w http.ResponseWriter, r *http.Request) {
	var req chooseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMsg(w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := s.store.GetDefinition(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	inst, err := s.store.LoadGame(req.SaveID)
	if err != nil {
		writeErr(w, err)
		return
	}
	v, err := inst.Choose(d, req.Choice)
	if err != nil {
		status := http.StatusBadRequest
		var vm *story.VersionMismatchError
		var mn *story.MissingNodeError
		if errors.As(err, &vm) || errors.As(err, &mn) {
			status = http.StatusConflict
		}
		writeMsg(w, status, err.Error())
		return
	}
	if err := s.store.SaveGame(req.SaveID, inst); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Save *story.Instance `json:"save"`
		View *story.View     `json:"view"`
	}{inst, v})
}

func (s *Server) listSaves(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.ListSaves())
}

func (s *Server) getSave(w http.ResponseWriter, r *http.Request) {
	in, err := s.store.LoadGame(r.PathValue("saveId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) deleteSave(w http.ResponseWriter, r *http.Request) {
	s.store.DeleteGame(r.PathValue("saveId"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDefinition(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	inst, err := s.store.LoadGame(r.PathValue("saveId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	v, err := story.Resume(d, inst)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "version") || strings.Contains(err.Error(), "no longer exists") {
			status = http.StatusConflict
		}
		writeMsg(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Save *story.Instance `json:"save"`
		View *story.View     `json:"view"`
	}{inst, v})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeMsg(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeValidation(w http.ResponseWriter, err error) {
	writeMsg(w, http.StatusUnprocessableEntity, err.Error())
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, story.ErrNotFound) {
		writeMsg(w, http.StatusNotFound, err.Error())
		return
	}
	writeMsg(w, http.StatusInternalServerError, err.Error())
}
