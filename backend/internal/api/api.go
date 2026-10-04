// Package api exposes the story runtime over HTTP.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"storyplatform/internal/story"
)

type Handler struct {
	catalog *story.Catalog
	store   story.Store
	defs    *story.FileDefStore
	initial map[string]interface{}
	locks   sync.Map // instance id -> *sync.Mutex
}

func NewHandler(catalog *story.Catalog, store story.Store, initial map[string]interface{}) *Handler {
	return &Handler{catalog: catalog, store: store, initial: initial}
}

// SetDefStore attaches revision persistence so adoption decisions and the
// exact adopted definitions survive process restarts.
func (h *Handler) SetDefStore(defs *story.FileDefStore) {
	h.defs = defs
}

// LoadPersistedRevisions restores every revision saved next to the saves
// into the catalog's history and reinstates each story's latest marker.
// Without this a restart would forget revisions that saves adopted and
// report version drift for saves that were perfectly valid.
func LoadPersistedRevisions(catalog *story.Catalog, defs *story.FileDefStore) error {
	revisions, latest, err := defs.LoadAll()
	if err != nil {
		return err
	}
	for _, d := range revisions {
		if _, err := catalog.RegisterRevision(d); err != nil {
			return err
		}
	}
	for storyID, hash := range latest {
		if err := catalog.RestoreLatest(storyID, hash); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stories", h.listStories)
	mux.HandleFunc("GET /api/stories/{id}", h.getStory)
	mux.HandleFunc("PUT /api/stories/{id}", h.updateStory)
	mux.HandleFunc("POST /api/instances", h.createInstance)
	mux.HandleFunc("GET /api/instances/{id}", h.getInstance)
	mux.HandleFunc("POST /api/instances/{id}/choices", h.applyChoice)
	mux.HandleFunc("POST /api/instances/{id}/forks", h.forkInstance)
	mux.HandleFunc("GET /api/instances/{id}/migration", h.previewMigration)
	mux.HandleFunc("POST /api/instances/{id}/migration", h.decideMigration)
	mux.HandleFunc("GET /api/stories/{id}/revisions", h.listRevisions)
	mux.HandleFunc("DELETE /api/stories/{id}/revisions/{hash}", h.withdrawRevision)
	mux.HandleFunc("GET /api/instances", h.listInstances)
}

type instanceResponse struct {
	Instance *story.Instance `json:"instance"`
	View     story.NodeView  `json:"view"`
	// VersionDrift is true when the author published a newer story revision
	// than the one this save is pinned to. The save keeps playing under its
	// pinned rules until it explicitly adopts the new ones.
	VersionDrift  bool   `json:"versionDrift,omitempty"`
	LatestVersion string `json:"latestVersion,omitempty"`
	LatestDefHash string `json:"latestDefHash,omitempty"`
	// MigrationState lets clients distinguish saves that never adopted a
	// revision, saves that adopted one, and saves whose adopt decision is
	// still awaiting recovery after an interrupted run.
	MigrationState string `json:"migrationState"`
}

// decorate annotates the response with rule-revision drift information.
func (h *Handler) decorate(resp *instanceResponse, inst *story.Instance) {
	resp.MigrationState = migrationState(inst, "")
	d, err := h.catalog.Get(inst.StoryID)
	if err != nil {
		return
	}
	hash, err := story.DefHash(d)
	if err != nil {
		return
	}
	resp.MigrationState = migrationState(inst, hash)
	if hash == inst.DefHash {
		return
	}
	resp.VersionDrift = true
	resp.LatestVersion = d.Version
	resp.LatestDefHash = hash
}

// Migration states reported to clients.
const (
	StateCurrent          = "current"           // playing the latest rules, never migrated
	StateNotAdopted       = "not-adopted"       // a newer revision exists; no decision yet
	StateAdopted          = "adopted"           // this save adopted the revision it plays
	StateAwaitingRecovery = "awaiting-recovery" // adopt decision saved, completion pending
)

func migrationState(inst *story.Instance, latestHash string) string {
	if inst.PendingMigration != nil {
		return StateAwaitingRecovery
	}
	if inst.MigrationAdopted != "" && inst.MigrationAdopted == inst.DefHash {
		return StateAdopted
	}
	if latestHash != "" && latestHash != inst.DefHash {
		return StateNotAdopted
	}
	return StateCurrent
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, story.ErrUnknownStory), errors.Is(err, story.ErrUnknownInstance):
		status = http.StatusNotFound
	case errors.Is(err, story.ErrUnknownChoice), errors.Is(err, story.ErrUnknownNode),
		errors.Is(err, story.ErrChoiceNotOnNode), errors.Is(err, story.ErrBadEffect),
		errors.Is(err, story.ErrEndNode):
		status = http.StatusBadRequest
	case errors.Is(err, story.ErrConditionFailed):
		status = http.StatusConflict
	case errors.Is(err, story.ErrDuplicateChoice):
		status = http.StatusConflict
	case errors.Is(err, story.ErrStoryVersionDrift):
		status = http.StatusConflict
	case errors.Is(err, story.ErrMigrationBlocked):
		status = http.StatusConflict
	case errors.Is(err, story.ErrMigrationPending):
		status = http.StatusConflict
	case errors.Is(err, story.ErrRevisionInUse):
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) listStories(w http.ResponseWriter, r *http.Request) {
	defs := h.catalog.List()
	out := make([]map[string]interface{}, 0, len(defs))
	for _, d := range defs {
		hash, _ := story.DefHash(d)
		out = append(out, map[string]interface{}{
			"id": d.ID, "title": d.Title, "version": d.Version, "defHash": hash,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stories": out})
}

func (h *Handler) getStory(w http.ResponseWriter, r *http.Request) {
	d, err := h.catalog.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// updateStory lets an author publish a revised definition. Existing saves
// stay pinned to their original revision; the previous revision remains
// playable through the catalog's version history.
func (h *Handler) updateStory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var def story.Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if def.ID == "" {
		def.ID = id
	}
	if def.ID != id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "definition id does not match path"})
		return
	}
	if err := story.ValidateDefinition(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.catalog.Update(&def); err != nil {
		writeError(w, err)
		return
	}
	if h.defs != nil {
		if _, err := h.defs.SaveDef(&def); err != nil {
			writeError(w, err)
			return
		}
	}
	hash, _ := story.DefHash(&def)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": def.ID, "title": def.Title, "version": def.Version, "defHash": hash,
	})
}

type createRequest struct {
	StoryID string `json:"storyId"`
	ID      string `json:"id"`
}

func (h *Handler) createInstance(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.StoryID == "" {
		if defs := h.catalog.List(); len(defs) == 1 {
			req.StoryID = defs[0].ID
		}
	}
	id := req.ID
	if id == "" {
		id = "inst-" + newID()
	}
	inst, _, err := h.catalog.Start(req.StoryID, id, h.initial)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.store.Save(inst); err != nil {
		writeError(w, err)
		return
	}
	view, err := h.catalog.View(inst)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, instanceResponse{Instance: inst, View: view})
}

func (h *Handler) listInstances(w http.ResponseWriter, r *http.Request) {
	insts, err := h.store.List()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instances": insts})
}

func (h *Handler) getInstance(w http.ResponseWriter, r *http.Request) {
	inst, err := h.store.Load(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := h.catalog.View(inst)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := instanceResponse{Instance: inst, View: view}
	h.decorate(&resp, inst)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) previewMigration(w http.ResponseWriter, r *http.Request) {
	inst, err := h.store.Load(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	impact, err := h.catalog.PreviewMigration(inst)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"migration": impact})
}

type migrationRequest struct {
	Action string `json:"action"` // adopt | defer
}

// decideMigration records the author's per-save decision about a revised
// story definition. "adopt" re-pins this save only; "defer" keeps the old
// rules. Both are idempotent and neither rewrites happened events.
func (h *Handler) decideMigration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	unlock := h.lockInstance(id)
	defer unlock()
	inst, err := h.store.Load(id)
	if err != nil {
		writeError(w, err)
		return
	}
	var req migrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	var impact *story.MigrationImpact
	switch req.Action {
	case "adopt":
		if inst.PendingMigration != nil {
			// Recovery: the decision was already saved; completing it again
			// is idempotent and never re-grants rewards or rewrites events.
			impact, err = h.catalog.CompleteAdoption(inst)
			if err != nil {
				writeError(w, err)
				return
			}
		} else {
			impact, err = h.catalog.BeginAdoption(inst)
			if err != nil {
				writeError(w, err)
				return
			}
			if !impact.UpToDate {
				// Persist the decision together with the exact revision it
				// points to before applying anything, so a restart can
				// always find the rules this save decided to adopt.
				if h.defs != nil {
					if def, ok := h.catalog.RevisionByHash(impact.ToHash); ok {
						if _, err := h.defs.KeepDef(def); err != nil {
							writeError(w, err)
							return
						}
					}
				}
				if err := h.store.Save(inst); err != nil {
					writeError(w, err)
					return
				}
				if _, err := h.catalog.CompleteAdoption(inst); err != nil {
					writeError(w, err)
					return
				}
			}
		}
	case "defer":
		if inst.PendingMigration != nil {
			writeError(w, story.ErrMigrationPending)
			return
		}
		if err := h.catalog.DeferMigration(inst); err != nil {
			writeError(w, err)
			return
		}
		impact, _ = h.catalog.PreviewMigration(inst)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be adopt or defer"})
		return
	}
	if err := h.store.Save(inst); err != nil {
		writeError(w, err)
		return
	}
	view, err := h.catalog.View(inst)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := instanceResponse{Instance: inst, View: view}
	h.decorate(&resp, inst)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"migration":    impact,
		"instance":     resp.Instance,
		"view":         resp.View,
		"versionDrift": resp.VersionDrift,
	})
}

type choiceRequest struct {
	ChoiceID       string `json:"choiceId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (h *Handler) lockInstance(id string) func() {
	m, _ := h.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (h *Handler) applyChoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	unlock := h.lockInstance(id)
	defer unlock()
	inst, err := h.store.Load(id)
	if err != nil {
		writeError(w, err)
		return
	}
	var req choiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.ChoiceID = strings.TrimSpace(req.ChoiceID)
	if req.ChoiceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "choiceId is required"})
		return
	}
	if inst.PendingMigration != nil {
		// The save is awaiting recovery of an interrupted adoption: the
		// client must review the facts and the pending decision first.
		writeError(w, story.ErrMigrationPending)
		return
	}
	applied, err := h.catalog.Apply(inst, req.ChoiceID, req.IdempotencyKey)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.store.Save(inst); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"applied": applied, "instance": inst, "view": applied.View,
	})
}

type forkRequest struct {
	Label string `json:"label"`
	ID    string `json:"id"`
}

// revisionView describes one registered revision and why it must be kept.
type revisionView struct {
	Hash    string `json:"hash"`
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
	InUse   bool   `json:"inUse"`
	Reason  string `json:"reason,omitempty"`
}

// revisionUseReasons explains every reference that keeps a revision alive.
func (h *Handler) revisionUseReasons(hash string) []string {
	reasons := []string{}
	insts, err := h.store.List()
	if err != nil {
		return append(reasons, "无法检查存档引用")
	}
	for _, inst := range insts {
		if inst.DefHash == hash {
			reasons = append(reasons, fmt.Sprintf("存档 %s 正按此版本进行", inst.ID))
		}
		if inst.PendingMigration != nil && inst.PendingMigration.ToHash == hash {
			reasons = append(reasons, fmt.Sprintf("存档 %s 的采用决定待恢复", inst.ID))
		}
		if inst.MigrationDeferred == hash {
			reasons = append(reasons, fmt.Sprintf("存档 %s 标记了暂缓此版本", inst.ID))
		}
	}
	return reasons
}

func (h *Handler) listRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.catalog.Get(id); err != nil {
		writeError(w, err)
		return
	}
	out := []revisionView{}
	for _, rev := range h.catalog.Revisions(id) {
		view := revisionView{Hash: rev.Hash, Version: rev.Version, Latest: rev.Latest}
		reasons := h.revisionUseReasons(rev.Hash)
		if rev.Latest {
			reasons = append([]string{"当前发布的最新版本"}, reasons...)
		}
		if len(reasons) > 0 {
			view.InUse = true
			view.Reason = strings.Join(reasons, "；")
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revisions": out})
}

// withdrawRevision removes a revision no save references. Revisions pinned
// by saves, pending recovery, deferred markers, or the latest published
// revision are kept and the refusal explains why.
func (h *Handler) withdrawRevision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hash := r.PathValue("hash")
	if _, err := h.catalog.Get(id); err != nil {
		writeError(w, err)
		return
	}
	if reasons := h.revisionUseReasons(hash); len(reasons) > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("revision %s 仍被引用，须保留：%s", hash, strings.Join(reasons, "；")),
		})
		return
	}
	if err := h.catalog.RemoveRevision(hash); err != nil {
		writeError(w, err)
		return
	}
	if h.defs != nil {
		if err := h.defs.DeleteDef(hash); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"withdrawn": hash})
}

func (h *Handler) forkInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	unlock := h.lockInstance(id)
	parent, err := h.store.Load(id)
	if err != nil {
		unlock()
		writeError(w, err)
		return
	}
	unlock()
	var req forkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	branchID := req.ID
	if branchID == "" {
		branchID = parent.ID + "-branch-" + newID()[:6]
	}
	label := req.Label
	if label == "" {
		label = fmt.Sprintf("试演 @ %s", parent.Current)
	}
	forked := story.Fork(parent, branchID, label)
	if err := h.store.Save(parent); err != nil {
		writeError(w, err)
		return
	}
	if err := h.store.Save(forked); err != nil {
		writeError(w, err)
		return
	}
	view, err := h.catalog.View(forked)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, instanceResponse{Instance: forked, View: view})
}
