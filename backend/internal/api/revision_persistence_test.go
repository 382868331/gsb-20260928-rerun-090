package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// reviseSeedV2 changes the chop_wood reward to 7.
func reviseSeedV2(t *testing.T) string {
	t.Helper()
	def := seed.Story()
	def.Version = "2026-10-02.v2"
	sq := def.Nodes["square"]
	for i := range sq.Choices {
		if sq.Choices[i].ID == "chop_wood" {
			sq.Choices[i].Effects = []story.Effect{{Op: "inc", Key: "gold", Value: float64(7)}}
		}
	}
	def.Nodes["square"] = sq
	return marshalDef(t, def)
}

func marshalDef(t *testing.T, def interface{}) string {
	t.Helper()
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// reviseSeedV3 removes a future choice (go_market) from square and changes
// reward again, exercising the "future node/choice removed" review.
func reviseSeedV3(t *testing.T) string {
	t.Helper()
	def := seed.Story()
	def.Version = "2026-10-03.v3"
	sq := def.Nodes["square"]
	kept := sq.Choices[:0]
	for _, ch := range sq.Choices {
		if ch.ID == "go_market" {
			continue
		}
		kept = append(kept, ch)
	}
	sq.Choices = kept
	def.Nodes["square"] = sq
	return marshalDef(t, def)
}

func startFileServer(t *testing.T, dir string) http.Handler {
	t.Helper()
	store, err := story.NewFileStore(filepath.Join(dir, "saves"))
	if err != nil {
		t.Fatal(err)
	}
	revs, err := story.NewFileRevisionStore(filepath.Join(dir, "revisions"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := story.NewCatalogWithStore(revs, seed.Story())
	h := NewHandler(catalog, store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func req(h http.Handler, method, path, body string) (int, string) {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// TestAdoptionSurvivesRestartAndRepeatedResume is the end-to-end version of
// the bug report: an adopted save must reopen after restart, a deferred
// sibling keeps its old rules, and a duplicated resume never re-awards or
// re-advances.
func TestAdoptionSurvivesRestartAndRepeatedResume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	h := startFileServer(t, dir)

	if code, body := req(h, "POST", "/api/instances", `{"id":"a"}`); code != http.StatusCreated {
		t.Fatalf("create a: %d %s", code, body)
	}
	if code, body := req(h, "POST", "/api/instances", `{"id":"b"}`); code != http.StatusCreated {
		t.Fatalf("create b: %d %s", code, body)
	}
	if code, body := req(h, "POST", "/api/instances/a/choices", `{"choiceId":"chop_wood","idempotencyKey":"a1"}`); code != http.StatusOK {
		t.Fatalf("a chop: %d %s", code, body)
	}
	if code, body := req(h, "POST", "/api/instances/b/choices", `{"choiceId":"chop_wood","idempotencyKey":"b1"}`); code != http.StatusOK {
		t.Fatalf("b chop: %d %s", code, body)
	}
	if code, body := req(h, "PUT", "/api/stories/mist-harbor", reviseSeedV2(t)); code != http.StatusOK {
		t.Fatalf("publish v2: %d %s", code, body)
	}

	// Branch a begins adoption, then the process exits before completion.
	if code, body := req(h, "POST", "/api/instances/a/migration", `{"action":"adopt","phase":"begin"}`); code != http.StatusOK {
		t.Fatalf("begin: %d %s", code, body)
	}
	// Branch b defers and keeps v1.
	if code, body := req(h, "POST", "/api/instances/b/migration", `{"action":"defer"}`); code != http.StatusOK {
		t.Fatalf("defer: %d %s", code, body)
	}

	// Restart: a new catalog rebuilt from persisted revision snapshots.
	h2 := startFileServer(t, dir)

	// Both saves open; a used to fail with version mismatch.
	code, body := req(h2, "GET", "/api/instances/a", "")
	if code != http.StatusOK {
		t.Fatalf("adopted-pending save must reopen after restart: %d %s", code, body)
	}
	if !strings.Contains(body, `"migrationStatus":"pending"`) {
		t.Fatalf("restarted save must be reported pending: %s", body)
	}
	if !strings.Contains(body, "2026-10-02.v2") {
		t.Fatalf("pending decision (target version) must be visible: %s", body)
	}
	code, body = req(h2, "GET", "/api/instances/b", "")
	if code != http.StatusOK {
		t.Fatalf("deferred save must reopen: %d %s", code, body)
	}
	if !strings.Contains(body, `"version":"2026-09-30.v1"`) {
		t.Fatalf("deferred branch keeps v1: %s", body)
	}

	// Resume completion for a; repeated resumes must be idempotent.
	code, body = req(h2, "POST", "/api/instances/a/migration", `{"action":"adopt","phase":"complete"}`)
	if code != http.StatusOK {
		t.Fatalf("complete: %d %s", code, body)
	}
	if !strings.Contains(body, `"version":"2026-10-02.v2"`) {
		t.Fatalf("a must be on v2 after completion: %s", body)
	}
	if !strings.Contains(body, `"gold":5`) {
		t.Fatalf("completion must not re-grant prior rewards: %s", body)
	}
	firstLog := strings.Count(body, "规则迁移")
	if firstLog != 1 {
		t.Fatalf("exactly one boundary note expected, got %d: %s", firstLog, body)
	}
	// Duplicated resume request arrives.
	code, body = req(h2, "POST", "/api/instances/a/migration", `{"action":"adopt","phase":"complete"}`)
	if code != http.StatusOK {
		t.Fatalf("duplicate complete: %d %s", code, body)
	}
	if strings.Count(body, "规则迁移") != 1 {
		t.Fatalf("duplicate resume appended a boundary note: %s", body)
	}
	if !strings.Contains(body, `"gold":5`) {
		t.Fatalf("duplicate resume changed rewards: %s", body)
	}
	if !strings.Contains(body, `"alreadyAdopted":true`) {
		t.Fatalf("duplicate resume must be marked alreadyAdopted: %s", body)
	}

	// The two branches play under different versions: a's new choices follow
	// v2 (chop_wood +7), b keeps v1 (+5).
	code, body = req(h2, "POST", "/api/instances/a/choices", `{"choiceId":"deliver_wood","idempotencyKey":"a2"}`)
	if code != http.StatusOK || !strings.Contains(body, `"gold":13`) {
		t.Fatalf("a deliver: %d %s", code, body)
	}
	code, body = req(h2, "POST", "/api/instances/b/choices", `{"choiceId":"deliver_wood","idempotencyKey":"b2"}`)
	if code != http.StatusOK || !strings.Contains(body, `"gold":13`) {
		t.Fatalf("b continues v1: %d %s", code, body)
	}
	// Both earned the v1-era +5 then +8; the versions diverge on the next
	// chop: adopted a gets the revised +7, deferred b keeps the old +5.
	code, body = req(h2, "POST", "/api/instances/a/choices", `{"choiceId":"chop_wood","idempotencyKey":"a3"}`)
	if code != http.StatusOK || !strings.Contains(body, `"gold":20`) {
		t.Fatalf("a must chop under v2 (+7): %d %s", code, body)
	}
	code, body = req(h2, "POST", "/api/instances/b/choices", `{"choiceId":"chop_wood","idempotencyKey":"b3"}`)
	if code != http.StatusOK || !strings.Contains(body, `"gold":18`) {
		t.Fatalf("b must chop under v1 (+5): %d %s", code, body)
	}

	// Snapshot files exist on disk for v1 and v2.
	files, _ := os.ReadDir(filepath.Join(dir, "revisions", "mist-harbor"))
	if len(files) < 2 {
		t.Fatalf("expected persisted v1+v2 snapshots, got %d", len(files))
	}
}

// TestPendingAdoptionLocksStoryProgress is the end-to-end state boundary:
// once an adoption decision is written but not completed, a restart shows
// the waiting state with the existing facts and decision while every new
// story choice is rejected. Only completion or cancellation unlocks play;
// repeated recovery inputs never change rewards, position or records.
func TestPendingAdoptionLocksStoryProgress(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	h := startFileServer(t, dir)

	if code, body := req(h, "POST", "/api/instances", `{"id":"p"}`); code != http.StatusCreated {
		t.Fatalf("create p: %d %s", code, body)
	}
	if code, body := req(h, "POST", "/api/instances/p/choices", `{"choiceId":"chop_wood","idempotencyKey":"p1"}`); code != http.StatusOK {
		t.Fatalf("chop: %d %s", code, body)
	}
	if code, body := req(h, "PUT", "/api/stories/mist-harbor", reviseSeedV2(t)); code != http.StatusOK {
		t.Fatalf("publish v2: %d %s", code, body)
	}
	if code, body := req(h, "POST", "/api/instances/p/migration", `{"action":"adopt","phase":"begin"}`); code != http.StatusOK {
		t.Fatalf("begin: %d %s", code, body)
	}

	// Restart with the written but unfinished decision.
	h2 := startFileServer(t, dir)
	code, body := req(h2, "GET", "/api/instances/p", "")
	if code != http.StatusOK || !strings.Contains(body, `"migrationStatus":"pending"`) {
		t.Fatalf("save must reopen as pending: %d %s", code, body)
	}
	if !strings.Contains(body, `"version":"2026-09-30.v1"`) {
		t.Fatalf("pending save keeps and shows the old rules: %s", body)
	}
	if !strings.Contains(body, `"gold":5`) {
		t.Fatalf("existing facts must remain visible: %s", body)
	}

	snapshot := body

	// Delivering a choice while recovery is pending is refused and changes
	// nothing server-side, even with a fresh idempotency key.
	code, body = req(h2, "POST", "/api/instances/p/choices", `{"choiceId":"deliver_wood","idempotencyKey":"p-deliver"}`)
	if code != http.StatusConflict {
		t.Fatalf("choice during pending must be 409, got %d %s", code, body)
	}
	code, body = req(h2, "GET", "/api/instances/p", "")
	if code != http.StatusOK {
		t.Fatalf("reload: %d %s", code, body)
	}
	var fresh map[string]interface{}
	if err := json.Unmarshal([]byte(snapshot), &fresh); err != nil {
		t.Fatal(err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		t.Fatal(err)
	}
	before := fresh["instance"].(map[string]interface{})
	after := saved["instance"].(map[string]interface{})
	if before["current"] != after["current"] || fmt.Sprint(after["rewards"]) != fmt.Sprint(before["rewards"]) ||
		fmt.Sprint(after["facts"]) != fmt.Sprint(before["facts"]) || fmt.Sprint(after["log"]) != fmt.Sprint(before["log"]) ||
		fmt.Sprint(after["migrationHistory"]) != fmt.Sprint(before["migrationHistory"]) {
		t.Fatalf("rejected choice changed state: before=%v after=%v", before, after)
	}
	if before["updatedAt"] != after["updatedAt"] {
		t.Fatal("rejected choice must not touch the save")
	}

	// Repeated begin is idempotent: same recorded decision, no rewrite.
	code, body = req(h2, "POST", "/api/instances/p/migration", `{"action":"adopt","phase":"begin"}`)
	if code != http.StatusOK {
		t.Fatalf("repeated begin: %d %s", code, body)
	}
	code, body = req(h2, "GET", "/api/instances/p", "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprint(before["updatedAt"])) {
		t.Fatalf("repeated begin must not rewrite the decision timestamp: %s", body)
	}
	// Defer is refused while a decision waits for recovery.
	if code, _ = req(h2, "POST", "/api/instances/p/migration", `{"action":"defer"}`); code != http.StatusConflict {
		t.Fatalf("defer during pending must be 409, got %d", code)
	}

	// Completion once, then repeated: exactly one record and boundary note.
	code, body = req(h2, "POST", "/api/instances/p/migration", `{"action":"adopt","phase":"complete"}`)
	if code != http.StatusOK {
		t.Fatalf("complete: %d %s", code, body)
	}
	if strings.Count(body, "规则迁移") != 1 {
		t.Fatalf("one boundary note after completion: %s", body)
	}
	code, body = req(h2, "POST", "/api/instances/p/migration", `{"action":"adopt","phase":"complete"}`)
	if code != http.StatusOK || strings.Count(body, "规则迁移") != 1 {
		t.Fatalf("duplicate completion changes records: %d %s", code, body)
	}

	// Another save exercises cancel: after cancelling, play resumes under
	// the original rules and the revision can be adopted again afterwards.
	h3 := startFileServer(t, dir)
	if code, body := req(h3, "POST", "/api/instances", `{"id":"q"}`); code != http.StatusCreated {
		t.Fatalf("create q: %d %s", code, body)
	}
	if code, body := req(h3, "POST", "/api/instances/q/choices", `{"choiceId":"chop_wood","idempotencyKey":"q1"}`); code != http.StatusOK {
		t.Fatalf("q chop: %d %s", code, body)
	}
	// Publish a fresh offered revision after q is pinned to the older one.
	if code, body := req(h3, "PUT", "/api/stories/mist-harbor", reviseSeedV3(t)); code != http.StatusOK {
		t.Fatalf("publish v3: %d %s", code, body)
	}
	if code, body := req(h3, "POST", "/api/instances/q/migration", `{"action":"adopt","phase":"begin"}`); code != http.StatusOK {
		t.Fatalf("q begin: %d %s", code, body)
	}
	if code, body := req(h3, "POST", "/api/instances/q/choices", `{"choiceId":"deliver_wood","idempotencyKey":"q-blocked"}`); code != http.StatusConflict {
		t.Fatalf("q choice must be blocked pending, got %d %s", code, body)
	}
	if code, body := req(h3, "POST", "/api/instances/q/migration", `{"action":"cancel"}`); code != http.StatusOK {
		t.Fatalf("q cancel: %d %s", code, body)
	}
	code, body = req(h3, "POST", "/api/instances/q/choices", `{"choiceId":"deliver_wood","idempotencyKey":"q2"}`)
	if code != http.StatusOK {
		t.Fatalf("q plays old rules after cancel: %d %s", code, body)
	}
	// After cancel the save may adopt again; completing lands on v2 once.
	if code, body := req(h3, "POST", "/api/instances/q/migration", `{"action":"adopt"}`); code != http.StatusOK {
		t.Fatalf("q re-adopt: %d %s", code, body)
	} else if strings.Count(body, "规则迁移") != 1 {
		t.Fatalf("re-adoption appends exactly one note: %s", body)
	}
}

// TestRemovedFutureChoiceAndWithdraw covers the impact report for a removed
// future choice and the withdraw rule: unused revisions withdraw, referenced
// ones are retained with reasons.
func TestRemovedFutureChoiceAndWithdraw(t *testing.T) {
	h, catalog, _ := newTestServer(t)
	if code, _ := postJSON(t, h, "/api/instances", `{"id":"w"}`); code != http.StatusCreated {
		t.Fatal("create")
	}
	if code, body := putJSON(t, h, "/api/stories/mist-harbor", reviseSeedV2(t)); code != http.StatusOK {
		t.Fatalf("v2: %d %v", code, body)
	}
	v2Rev := catalog.RevisionSnapshots("mist-harbor")
	var v2Hash string
	for _, rev := range v2Rev {
		if rev.Version == "2026-10-02.v2" {
			v2Hash = rev.Hash
		}
	}
	if v2Hash == "" {
		t.Fatal("v2 hash missing")
	}
	// Publish v3 removing a future choice.
	if code, body := putJSON(t, h, "/api/stories/mist-harbor", reviseSeedV3(t)); code != http.StatusOK {
		t.Fatalf("v3: %d %v", code, body)
	}
	code, body := getJSON(t, h, "/api/instances/w/migration")
	if code != http.StatusOK {
		t.Fatalf("preview: %d", code)
	}
	future := body["migration"].(map[string]interface{})["future"].([]interface{})
	foundRemoved := false
	for _, f := range future {
		m := f.(map[string]interface{})
		if m["choiceId"] == "go_market" && m["change"] == "removed" {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Fatalf("removed future choice must be listed: %v", future)
	}

	// v2 is unused by any save (w is still on v1, current is v3): withdraw.
	code, raw := doJSONRaw(t, h, http.MethodDelete, "/api/stories/mist-harbor/revisions/"+v2Hash, "")
	if code != http.StatusOK {
		t.Fatalf("unused v2 should withdraw: %d %s", code, raw)
	}

	// v1 is pinned by save w: withdraw is refused with the reason.
	code, raw = doJSONRaw(t, h, http.MethodDelete, "/api/stories/mist-harbor/revisions/"+v1HashSeed(t), "")
	if code != http.StatusConflict || !strings.Contains(raw, "retained") || !strings.Contains(raw, "w") {
		t.Fatalf("pinned revision retained with reasons: %d %s", code, raw)
	}
}

func v1HashSeed(t *testing.T) string {
	t.Helper()
	h, _ := story.DefHash(seed.Story())
	return h
}

func doJSONRaw(t *testing.T, h http.Handler, method, path, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}
