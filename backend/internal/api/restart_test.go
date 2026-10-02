package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// TestRestartResumeFromDisk spins up a handler backed by a file store, plays
// part of the story, then builds a completely new handler (as a restarted
// process would) against the same directory and verifies state continues.
func TestRestartResumeFromDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	start := func() (http.Handler, *story.FileStore) {
		store, err := story.NewFileStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		catalog := story.NewVersionedCatalog(seed.Story())
		h := NewHandler(catalog, store, seed.InitialFacts())
		mux := http.NewServeMux()
		h.Register(mux)
		return mux, store
	}

	h, _ := start()
	run := func(handler http.Handler, method, path, body string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	if code, body := run(h, "POST", "/api/instances", `{"id":"restart-1"}`); code != http.StatusCreated {
		t.Fatalf("create %d: %s", code, body)
	}
	run(h, "POST", "/api/instances/restart-1/choices", `{"choiceId":"chop_wood","idempotencyKey":"a1"}`)
	run(h, "POST", "/api/instances/restart-1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"a2"}`)
	run(h, "POST", "/api/instances/restart-1/forks", `{"id":"restart-1-b1","label":"存档后继续"}`)

	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatalf("expected 2 persisted files, got %d", len(files))
	}

	// Simulate a process restart with brand-new in-memory state.
	h2, _ := start()
	code, body := run(h2, "GET", "/api/instances/restart-1", "")
	if code != http.StatusOK {
		t.Fatalf("resume after restart: %d %s", code, body)
	}
	if !strings.Contains(body, `"current":"square"`) {
		t.Fatalf("resume at wrong node: %s", body)
	}
	if !strings.Contains(body, `"gold":13`) {
		t.Fatalf("facts lost across restart: %s", body)
	}
	if !strings.Contains(body, `"id":"restart-1-b1"`) {
		t.Fatalf("branch marker lost across restart: %s", body)
	}

	// The idempotency record survives too: replaying key a2 grants nothing.
	code, body = run(h2, "POST", "/api/instances/restart-1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"a2"}`)
	if code != http.StatusOK || !strings.Contains(body, `"duplicate":true`) {
		t.Fatalf("idempotent replay after restart: %d %s", code, body)
	}
	if !strings.Contains(body, `"gold":13`) {
		t.Fatalf("replayed request changed facts: %s", body)
	}

	// A fresh request without a key for an already-applied visit is rejected.
	code, _ = run(h2, "POST", "/api/instances/restart-1/choices", `{"choiceId":"deliver_wood"}`)
	if code != http.StatusConflict {
		t.Fatalf("duplicate bare resubmit after restart: %d", code)
	}

	// Continued play after resume works and reaches an ending.
	run(h2, "POST", "/api/instances/restart-1/choices", `{"choiceId":"bribe_guard","idempotencyKey":"a3"}`)
	code, body = run(h2, "POST", "/api/instances/restart-1/choices", `{"choiceId":"walk_out","idempotencyKey":"a4"}`)
	if code != http.StatusOK || !strings.Contains(body, `"current":"ending_normal"`) {
		t.Fatalf("continue to ending failed: %d %s", code, body)
	}
}
