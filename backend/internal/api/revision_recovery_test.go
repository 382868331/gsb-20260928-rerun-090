package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// startPersisted mirrors cmd/server: a file-backed instance store plus a
// file-backed definition store, so rebuilding the handler simulates a full
// process restart.
func startPersisted(t *testing.T, dir string) http.Handler {
	t.Helper()
	store, err := story.NewFileStore(filepath.Join(dir, "instances"))
	if err != nil {
		t.Fatal(err)
	}
	defStore, err := story.NewFileDefinitionStore(filepath.Join(dir, "definitions"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := story.NewCatalog()
	catalog.AttachStore(defStore)
	revs, err := defStore.Revisions()
	if err != nil {
		t.Fatal(err)
	}
	for _, rev := range revs {
		if err := catalog.RestoreRevision(rev); err != nil {
			t.Fatal(err)
		}
	}
	if len(catalog.List()) == 0 {
		if err := catalog.Update(seed.Story()); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(catalog, store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func run(t *testing.T, h http.Handler, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]interface{}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s %s: %v (%s)", method, path, err, w.Body.String())
		}
	}
	return w.Code, out
}

func mustOK(t *testing.T, h http.Handler, method, path, body string) map[string]interface{} {
	t.Helper()
	code, out := run(t, h, method, path, body)
	if code >= 300 {
		t.Fatalf("%s %s: %d %v", method, path, code, out)
	}
	return out
}

// reviseSeedNodeRemoved returns a revision whose square node no longer
// offers chop_wood and whose lumberyard node is gone entirely.
func reviseSeedRemoveNode(t *testing.T) string {
	t.Helper()
	def := seed.Story()
	def.Version = "2026-10-03.v3"
	delete(def.Nodes, "lumberyard")
	sq := def.Nodes["square"]
	kept := sq.Choices[:0]
	for _, ch := range sq.Choices {
		if ch.Target != "lumberyard" {
			kept = append(kept, ch)
		}
	}
	sq.Choices = kept
	def.Nodes["square"] = sq
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A save that adopted a revised rule set must resume on exactly those rules
// after the service restarts — not fall back to the initial rules, not fail
// with a version mismatch.
func TestAdoptedRevisionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	h := startPersisted(t, dir)

	mustOK(t, h, "POST", "/api/instances", `{"id":"a1"}`)
	mustOK(t, h, "POST", "/api/instances/a1/choices", `{"choiceId":"chop_wood","idempotencyKey":"k1"}`)
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeed(t))
	mustOK(t, h, "POST", "/api/instances/a1/migration", `{"action":"adopt"}`)

	// Restart: brand-new in-memory catalogue over the same directories.
	h2 := startPersisted(t, dir)
	code, body := run(t, h2, "GET", "/api/instances/a1", "")
	if code != http.StatusOK {
		t.Fatalf("adopted save must resume after restart, got %d %v", code, body)
	}
	inst := body["instance"].(map[string]interface{})
	if inst["version"] != "2026-10-02.v2" {
		t.Fatalf("save must stay on the adopted revision, got %v", inst["version"])
	}
	if body["versionDrift"] == true {
		t.Fatal("no drift expected right after adopting the latest revision")
	}
	// Already-claimed rewards stay as they were granted under the old rules.
	facts := inst["facts"].(map[string]interface{})
	if facts["gold"].(float64) != 5 {
		t.Fatalf("rewards granted under old rules must be kept, gold=%v", facts["gold"])
	}
	// The migration boundary is still explained after the restart.
	logEntries := inst["log"].([]interface{})
	last := logEntries[len(logEntries)-1].(map[string]interface{})
	if !strings.Contains(last["note"].(string), "规则迁移") {
		t.Fatalf("migration boundary note lost after restart: %v", last)
	}
	// Play continues under the adopted rules.
	code, body = run(t, h2, "POST", "/api/instances/a1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"k2"}`)
	if code != http.StatusOK {
		t.Fatalf("continue after restart: %d %v", code, body)
	}
}

// Two branches adopt the same revision at different times; one defers.
// Across restarts each keeps its own version.
func TestBranchesKeepOwnVersionsAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	h := startPersisted(t, dir)

	mustOK(t, h, "POST", "/api/instances", `{"id":"b1"}`)
	mustOK(t, h, "POST", "/api/instances/b1/choices", `{"choiceId":"chop_wood","idempotencyKey":"f1"}`)
	mustOK(t, h, "POST", "/api/instances/b1/forks", `{"id":"b1-adopter","label":"采用新规则"}`)
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeed(t))
	mustOK(t, h, "POST", "/api/instances/b1-adopter/migration", `{"action":"adopt"}`)
	mustOK(t, h, "POST", "/api/instances/b1/migration", `{"action":"defer"}`)

	for round := 0; round < 2; round++ {
		h = startPersisted(t, dir)
		_, adopter := run(t, h, "GET", "/api/instances/b1-adopter", "")
		_, keeper := run(t, h, "GET", "/api/instances/b1", "")
		if got := adopter["instance"].(map[string]interface{})["version"]; got != "2026-10-02.v2" {
			t.Fatalf("round %d: adopter must be on v2, got %v", round, got)
		}
		if got := keeper["instance"].(map[string]interface{})["version"]; got != "2026-09-30.v1" {
			t.Fatalf("round %d: keeper must stay on v1, got %v", round, got)
		}
		if keeper["versionDrift"] != true {
			t.Fatalf("round %d: keeper should still see the pending revision", round)
		}
	}
}

// An adoption interrupted mid-way reloads as a recoverable state: the
// preview shows what was kept plus the pending adoption record, recovery
// completes it, and repeating recovery neither re-grants rewards nor adds a
// second migration boundary.
func TestInterruptedAdoptionRecoversIdempotently(t *testing.T) {
	dir := t.TempDir()
	h := startPersisted(t, dir)

	mustOK(t, h, "POST", "/api/instances", `{"id":"c1"}`)
	mustOK(t, h, "POST", "/api/instances/c1/choices", `{"choiceId":"chop_wood","idempotencyKey":"g1"}`)
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeed(t))

	// Simulate an interrupted adoption: record the decision, persist, then
	// "crash" before completion by rebuilding the handler.
	store, _ := story.NewFileStore(filepath.Join(dir, "instances"))
	defStore, _ := story.NewFileDefinitionStore(filepath.Join(dir, "definitions"))
	catalog := story.NewCatalog()
	revs, _ := defStore.Revisions()
	for _, rev := range revs {
		_ = catalog.RestoreRevision(rev)
	}
	inst, err := store.Load("c1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BeginAdoption(inst); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(inst); err != nil {
		t.Fatal(err)
	}

	h2 := startPersisted(t, dir)
	// The save still loads under its pinned old rules — no vague read
	// failure — and the migration preview reports the recovery state.
	code, body := run(t, h2, "GET", "/api/instances/c1", "")
	if code != http.StatusOK {
		t.Fatalf("interrupted save must still load, got %d %v", code, body)
	}
	_, mig := run(t, h2, "GET", "/api/instances/c1/migration", "")
	if mig["recovery"] != true {
		t.Fatalf("expected recovery state, got %v", mig)
	}
	state := mig["state"].(map[string]interface{})
	if state["status"] != "adopting" || state["toVersion"] != "2026-10-02.v2" {
		t.Fatalf("pending adoption record missing: %v", state)
	}

	// Recovery completes the adoption exactly once.
	_, rec1 := run(t, h2, "POST", "/api/instances/c1/migration", `{"action":"recover"}`)
	inst1 := rec1["instance"].(map[string]interface{})
	if inst1["version"] != "2026-10-02.v2" {
		t.Fatalf("recovery must complete the adoption, got %v", inst1["version"])
	}
	logLen := len(inst1["log"].([]interface{}))
	rewards := len(inst1["rewards"].([]interface{}))

	// Repeating recovery is an idempotent no-op: no new boundary, no new
	// rewards.
	code, _ = run(t, h2, "POST", "/api/instances/c1/migration", `{"action":"recover"}`)
	if code != http.StatusOK {
		t.Fatalf("repeat recover should be an idempotent no-op, got %d", code)
	}
	_, again := run(t, h2, "GET", "/api/instances/c1", "")
	inst2 := again["instance"].(map[string]interface{})
	if len(inst2["log"].([]interface{})) != logLen {
		t.Fatal("repeated recovery must not add a second migration boundary")
	}
	if len(inst2["rewards"].([]interface{})) != rewards {
		t.Fatal("repeated recovery must not re-grant rewards")
	}
	// Facts keep the old-rule reward (5 gold), not the revised 7.
	if inst2["facts"].(map[string]interface{})["gold"].(float64) != 5 {
		t.Fatal("already-claimed rewards must stay as granted")
	}
}

// When the current node was removed by the revision, adoption is refused
// with a specific reason and the save keeps its old rules; the pending
// revision stays available for later recovery.
func TestAdoptionBlockedWhenNodeRemoved(t *testing.T) {
	dir := t.TempDir()
	h := startPersisted(t, dir)

	mustOK(t, h, "POST", "/api/instances", `{"id":"d1"}`)
	mustOK(t, h, "POST", "/api/instances/d1/choices", `{"choiceId":"chop_wood","idempotencyKey":"h1"}`)
	// Now standing on lumberyard, which the revision removes.
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeedRemoveNode(t))

	code, body := run(t, h, "POST", "/api/instances/d1/migration", `{"action":"adopt"}`)
	if code != http.StatusConflict {
		t.Fatalf("adoption onto a removed node must be refused, got %d", code)
	}
	if !strings.Contains(body["error"].(string), "lumberyard") {
		t.Fatalf("refusal must name the blocked node: %v", body["error"])
	}
	// The save still plays under its pinned rules after a restart.
	h2 := startPersisted(t, dir)
	code, body = run(t, h2, "GET", "/api/instances/d1", "")
	if code != http.StatusOK {
		t.Fatalf("blocked save must keep its old rules, got %d %v", code, body)
	}
	_, mig := run(t, h2, "GET", "/api/instances/d1/migration", "")
	impact := mig["migration"].(map[string]interface{})
	if impact["currentNodeMissing"] != true {
		t.Fatalf("preview must explain why progress cannot continue: %v", impact)
	}
}

// Authors can withdraw a revision nobody adopted; revisions still referenced
// by saves stay readable so old branches keep their basis.
func TestWithdrawRevision(t *testing.T) {
	dir := t.TempDir()
	h := startPersisted(t, dir)

	mustOK(t, h, "POST", "/api/instances", `{"id":"e1"}`)
	v2 := mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeed(t))
	v2hash := v2["defHash"].(string)
	v3 := mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeedRemoveNode(t))
	v3hash := v3["defHash"].(string)

	// The latest revision cannot be withdrawn.
	code, _ := run(t, h, "DELETE", "/api/stories/mist-harbor/revisions/"+v3hash, "")
	if code != http.StatusConflict {
		t.Fatalf("withdrawing the latest revision must be refused, got %d", code)
	}
	// An unadopted, superseded revision can be withdrawn.
	code, _ = run(t, h, "DELETE", "/api/stories/mist-harbor/revisions/"+v2hash, "")
	if code != http.StatusOK {
		t.Fatalf("withdrawing an unadopted revision: %d", code)
	}
	_, list := run(t, h, "GET", "/api/stories/mist-harbor/revisions", "")
	for _, rev := range list["revisions"].([]interface{}) {
		if rev.(map[string]interface{})["hash"] == v2hash {
			t.Fatal("withdrawn revision must no longer be listed")
		}
	}

	// A revision a save adopted cannot be withdrawn and stays readable
	// after restart.
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeed(t))
	mustOK(t, h, "POST", "/api/instances/e1/migration", `{"action":"adopt"}`)
	mustOK(t, h, "PUT", "/api/stories/mist-harbor", reviseSeedRemoveNode(t))
	code, body := run(t, h, "DELETE", "/api/stories/mist-harbor/revisions/"+v2hash, "")
	if code != http.StatusConflict {
		t.Fatalf("withdrawing an adopted revision must be refused, got %d %v", code, body)
	}
	h2 := startPersisted(t, dir)
	code, body = run(t, h2, "GET", "/api/instances/e1", "")
	if code != http.StatusOK {
		t.Fatalf("save on adopted revision must survive restart, got %d %v", code, body)
	}
	if body["instance"].(map[string]interface{})["defHash"] != v2hash {
		t.Fatalf("save must stay pinned to the adopted revision: %v", body["instance"])
	}
}
