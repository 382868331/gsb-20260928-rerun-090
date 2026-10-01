package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// A fresh data directory must produce [] rather than null so an empty
// archive is rendered as an empty state, not an exception.
func TestEmptyListEncodesArray(t *testing.T) {
	store, err := story.NewFileStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(story.NewCatalog(seed.Story()), store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["instances"]) != "[]" {
		t.Fatalf("empty archive must encode as [], got %s", body["instances"])
	}
}

// A real read failure (unreadable data dir) stays a 500 so the client can
// keep its retry entry; it must never be presented as an empty archive.
func TestListRealFailurePropagates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	store, err := story.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(story.NewCatalog(seed.Story()), store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt save must be a real failure, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCompareBranchesHTTP(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"r"}`)
	postJSON(t, h, "/api/instances/r/choices", `{"choiceId":"chop_wood","idempotencyKey":"c1"}`)
	postJSON(t, h, "/api/instances/r/forks", `{"id":"r-a","label":"领工钱"}`)

	// Branch A takes the wage; the root keeps waiting at the lumberyard.
	postJSON(t, h, "/api/instances/r-a/choices", `{"choiceId":"deliver_wood","idempotencyKey":"ca"}`)

	status, body := getJSON(t, h, "/api/instances/r/compare/r-a")
	if status != http.StatusOK {
		t.Fatalf("compare: %d %v", status, body)
	}
	if body["forkNode"].(string) != "lumberyard" {
		t.Fatalf("fork node wrong: %v", body["forkNode"])
	}
	prefix, _ := body["prefix"].([]interface{})
	if len(prefix) != 2 {
		t.Fatalf("shared prefix must cover square+lumberyard, got %d", len(prefix))
	}
	baseSteps, _ := body["baseSteps"].([]interface{})
	otherSteps, _ := body["otherSteps"].([]interface{})
	if len(baseSteps) != 0 {
		t.Fatalf("root has no divergent steps, got %d", len(baseSteps))
	}
	if len(otherSteps) != 1 {
		t.Fatalf("branch must diverge by one step, got %d", len(otherSteps))
	}
	baseFacts, _ := body["baseFacts"].(map[string]interface{})
	otherFacts, _ := body["otherFacts"].(map[string]interface{})
	if baseFacts["gold"].(float64) != 5 || otherFacts["gold"].(float64) != 13 {
		t.Fatalf("branch state leaked across comparison: %v / %v", baseFacts["gold"], otherFacts["gold"])
	}

	// Switching back to the root and continuing still grants the wage once
	// and does not see the branch's gold.
	status, body = postJSON(t, h, "/api/instances/r/choices", `{"choiceId":"deliver_wood","idempotencyKey":"c2"}`)
	if status != http.StatusOK {
		t.Fatalf("continue on root after comparison: %d %v", status, body)
	}
	if g := instFacts(t, body)["gold"].(float64); g != 13 {
		t.Fatalf("root wage must land on the root only, gold=%v", g)
	}
	// The branch keeps its own already-granted reward.
	_, branch := getJSON(t, h, "/api/instances/r-a")
	bf, _ := branch["instance"].(map[string]interface{})["facts"].(map[string]interface{})
	if bf["gold"].(float64) != 13 {
		t.Fatalf("branch reward history changed: %v", bf["gold"])
	}

	// Comparing unrelated or identical instances is a client error.
	if code, _ := getJSON(t, h, "/api/instances/r/compare/r"); code != http.StatusBadRequest {
		t.Fatalf("self compare expected 400, got %d", code)
	}
}

func TestTraceExplainsSteps(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"tr"}`)
	_, body := postJSON(t, h, "/api/instances/tr/choices", `{"choiceId":"chop_wood","idempotencyKey":"t1"}`)
	steps, _ := body["trace"].([]interface{})
	if len(steps) != 2 {
		t.Fatalf("trace needs arrival + choice steps, got %d", len(steps))
	}
	last := steps[1].(map[string]interface{})
	if last["choiceId"].(string) != "chop_wood" {
		t.Fatalf("trace missing choice: %v", last)
	}
	if last["choiceText"].(string) == "" {
		t.Fatalf("trace must explain the choice text: %v", last)
	}
	rewards, _ := last["rewards"].([]interface{})
	if len(rewards) != 1 {
		t.Fatalf("trace step missing reward: %v", rewards)
	}
}

// Fork racing with a choice on the same parent must not lose the choice
// commit: the fork holds the instance lock across marker save.
func TestForkConcurrentWithChoice(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"race"}`)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		postJSON(t, h, "/api/instances/race/choices", `{"choiceId":"chop_wood","idempotencyKey":"r1"}`)
	}()
	go func() {
		defer wg.Done()
		postJSON(t, h, "/api/instances/race/forks", `{"id":"race-b1","label":"并发分支"}`)
	}()
	wg.Wait()

	_, body := getJSON(t, h, "/api/instances/race")
	inst := body["instance"].(map[string]interface{})
	branches, _ := inst["branches"].(map[string]interface{})
	if _, ok := branches["race-b1"]; !ok {
		t.Fatalf("fork marker lost: %v", inst["branches"])
	}
	facts, _ := inst["facts"].(map[string]interface{})
	if facts["gold"].(float64) != 5 {
		t.Fatalf("choice commit lost to fork save: gold=%v", facts["gold"])
	}
}
