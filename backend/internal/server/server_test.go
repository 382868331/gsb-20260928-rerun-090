package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"storyplatform/internal/story"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	store, err := story.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDefinition(story.SeedStory()); err != nil {
		t.Fatal(err)
	}
	return New(store)
}

func post(t *testing.T, h http.Handler, path string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	recw := httptest.NewRecorder()
	h.ServeHTTP(recw, req)
	var out map[string]any
	if recw.Body.Len() > 0 {
		_ = json.Unmarshal(recw.Body.Bytes(), &out)
	}
	return recw.Code, out
}

func TestUIAllowedChoicesMatchRunner(t *testing.T) {
	h := newTestServer(t)

	status, resp := post(t, h, "/api/stories/crossroads/start", map[string]string{"saveId": "s1"})
	if status != http.StatusOK {
		t.Fatalf("start status %d: %v", status, resp)
	}
	status, _ = post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s1", "choice": "go_market"})
	if status != http.StatusOK {
		t.Fatalf("choose market: %d", status)
	}
	status, _ = post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s1", "choice": "to_gate"})
	if status != http.StatusOK {
		t.Fatalf("choose gate: %d", status)
	}

	// Market gold = 3: the view offers bribe_poor only.
	status, view := post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s1", "choice": "bribe_poor"})
	if status != http.StatusOK {
		t.Fatalf("bribe_poor: %d %v", status, view)
	}

	// Restart a second save directly at the gate state is not possible; instead
	// verify the rich path rejects the poor choice with 400.
	post(t, h, "/api/stories/crossroads/start", map[string]string{"saveId": "s2"})
	post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s2", "choice": "go_forest"})
	post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s2", "choice": "to_gate"})
	status, resp = post(t, h, "/api/stories/crossroads/choose", map[string]string{"saveId": "s2", "choice": "bribe_poor"})
	if status != http.StatusBadRequest {
		t.Fatalf("disallowed choice status = %d, want 400: %v", status, resp)
	}
}

func TestResumeVersionMismatchIsConflict(t *testing.T) {
	store, _ := story.NewStore(t.TempDir())
	_ = store.PutDefinition(story.SeedStory())
	h := New(store)
	post(t, h, "/api/stories/crossroads/start", map[string]string{"saveId": "s1"})
	v2 := story.SeedStory()
	v2.Version = 2
	if err := store.PutDefinition(v2); err != nil {
		t.Fatal(err)
	}
	status, resp := post(t, h, "/api/stories/crossroads/saves/s1/resume", nil)
	if status != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409: %v", status, resp)
	}
}
