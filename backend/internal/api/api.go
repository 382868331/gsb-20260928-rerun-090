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
	initial map[string]interface{}
	locks   sync.Map // instance id -> *sync.Mutex
}

func NewHandler(catalog *story.Catalog, store story.Store, initial map[string]interface{}) *Handler {
	return &Handler{catalog: catalog, store: store, initial: initial}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stories", h.listStories)
	mux.HandleFunc("GET /api/stories/{id}", h.getStory)
	mux.HandleFunc("PUT /api/stories/{id}", h.updateStory)
	mux.HandleFunc("GET /api/stories/{id}/revisions", h.listRevisions)
	mux.HandleFunc("DELETE /api/stories/{id}/revisions/{hash}", h.withdrawRevision)
	mux.HandleFunc("POST /api/instances", h.createInstance)
	mux.HandleFunc("GET /api/instances/{id}", h.getInstance)
	mux.HandleFunc("POST /api/instances/{id}/choices", h.applyChoice)
	mux.HandleFunc("POST /api/instances/{id}/forks", h.forkInstance)
	mux.HandleFunc("GET /api/instances/{id}/migration", h.previewMigration)
	mux.HandleFunc("POST /api/instances/{id}/migration", h.decideMigration)
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
	// MigrationStatus is current (up to date), available (new revision
	// offered, not adopted), pending (a begin recorded, completion
	// interrupted) or blocked (latest revision removed the current node).
	MigrationStatus string                 `json:"migrationStatus,omitempty"`
	Pending         *story.PendingAdoption `json:"pendingMigration,omitempty"`
}

// decorate annotates the response with rule-revision drift information.
func (h *Handler) decorate(resp *instanceResponse, inst *story.Instance) {
	if inst.PendingMigration != nil {
		resp.MigrationStatus = "pending"
		resp.Pending = inst.PendingMigration
		resp.LatestVersion = inst.PendingMigration.ToVersion
		resp.LatestDefHash = inst.PendingMigration.ToHash
		return
	}
	d, err := h.catalog.Get(inst.StoryID)
	if err != nil {
		return
	}
	hash, err := story.DefHash(d)
	if err != nil {
		return
	}
	if hash == inst.DefHash {
		resp.MigrationStatus = "current"
		return
	}
	resp.VersionDrift = true
	resp.LatestVersion = d.Version
	resp.LatestDefHash = hash
	// If adopting the offered revision would strand the save, mark blocked so
	// the UI can explain rather than offer a failing button.
	if impact, perr := h.catalog.PreviewMigration(inst); perr == nil {
		if impact.CurrentNodeMissing {
			resp.MigrationStatus = "blocked"
		} else {
			resp.MigrationStatus = "available"
		}
	} else {
		resp.MigrationStatus = "available"
	}
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
	case errors.Is(err, story.ErrAdoptionPending):
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

func (h *Handler) listRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.catalog.Get(id); err != nil {
		// A story that only has withdrawn/historical snapshots still lists
		// them when at least one retained revision exists for the id.
		writeError(w, err)
		return
	}
	refs := h.scanRevisionUsage(id)
	resp := make([]map[string]interface{}, 0)
	for _, rev := range h.catalog.RevisionSnapshots(id) {
		item := map[string]interface{}{
			"version": rev.Version, "hash": rev.Hash, "current": rev.Current,
			"retained": true,
		}
		reasons := refs[rev.Hash]
		if len(reasons) > 0 {
			item["withdrawable"] = false
			item["reasons"] = reasons
		} else if !rev.Current {
			item["withdrawable"] = true
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revisions": resp})
}

// scanRevisionUsage walks every save once and records why each revision hash
// of the story must be retained: a live pin, a pending adoption, or a
// completed adoption history entry.
func (h *Handler) scanRevisionUsage(storyID string) map[string][]string {
	out := map[string][]string{}
	insts, err := h.store.List()
	if err != nil {
		return out
	}
	add := func(hash, reason, id string) {
		if hash == "" {
			return
		}
		for _, existing := range out[hash] {
			if existing == reason+":"+id {
				return
			}
		}
		out[hash] = append(out[hash], reason+":"+id)
	}
	for _, inst := range insts {
		if inst.StoryID != storyID {
			continue
		}
		add(inst.DefHash, "存档采用", inst.ID)
		if inst.PendingMigration != nil {
			add(inst.PendingMigration.ToHash, "等待恢复的采用", inst.ID)
			add(inst.PendingMigration.FromHash, "等待恢复的存档基线", inst.ID)
		}
		for _, rec := range inst.MigrationHistory {
			add(rec.ToHash, "历史采用记录", inst.ID)
			add(rec.FromHash, "历史采用记录", inst.ID)
		}
	}
	return out
}

func (h *Handler) withdrawRevision(w http.ResponseWriter, r *http.Request) {
	storyID := r.PathValue("id")
	hash := r.PathValue("hash")
	usage := h.scanRevisionUsage(storyID)
	refs := &story.RevisionUsage{}
	for _, reason := range usage[hash] {
		id := reason
		if i := strings.LastIndex(reason, ":"); i >= 0 {
			id = reason[i+1:]
		}
		switch {
		case strings.HasPrefix(reason, "存档采用"):
			refs.PinnedBy = appendUnique(refs.PinnedBy, id)
		case strings.HasPrefix(reason, "等待恢复的采用"):
			refs.PendingBy = appendUnique(refs.PendingBy, id)
		case strings.HasPrefix(reason, "等待恢复的存档基线"):
			refs.PinnedBy = appendUnique(refs.PinnedBy, id)
		case strings.HasPrefix(reason, "历史采用记录"):
			refs.HistoryOf = appendUnique(refs.HistoryOf, id)
		}
	}
	if err := h.catalog.WithdrawRevision(storyID, hash, refs); err != nil {
		status := http.StatusConflict
		if errors.Is(err, story.ErrUnknownStory) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]interface{}{
			"error":    err.Error(),
			"retained": true,
			"pinnedBy": refs.PinnedBy, "pendingBy": refs.PendingBy, "historyOf": refs.HistoryOf,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"withdrawn": hash})
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
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
	Action string `json:"action"` // adopt | defer | cancel
	// Phase is begin (record the decision but keep old rules until resume),
	// complete (finish a begun adoption) or complete-and-commit (default:
	// begin + complete in one request).
	Phase string `json:"phase,omitempty"`
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
		switch req.Phase {
		case "", "complete-and-commit":
			impact, err = h.catalog.AdoptMigration(inst)
		case "begin":
			impact, err = h.catalog.BeginAdoption(inst)
		case "complete":
			var applied bool
			impact, applied, err = h.catalog.CompleteAdoption(inst)
			if err == nil && impact != nil && impact.AlreadyAdopted {
				_ = applied
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phase must be begin, complete or complete-and-commit"})
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
	case "defer":
		if err := h.catalog.DeferMigration(inst); err != nil {
			writeError(w, err)
			return
		}
		impact, _ = h.catalog.PreviewMigration(inst)
	case "cancel":
		if err := h.catalog.CancelAdoption(inst); err != nil {
			writeError(w, err)
			return
		}
		impact, _ = h.catalog.PreviewMigration(inst)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be adopt, complete, defer or cancel"})
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
