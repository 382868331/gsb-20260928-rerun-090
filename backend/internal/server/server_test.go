package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/server"
	"storyplatform/internal/store"
)

type harness struct {
	t       *testing.T
	dataDir string
	st      store.Store
	h       http.Handler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	st, err := store.NewFileStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutStory(seed.HeroV1()); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, dataDir: dataDir, st: st, h: server.New(st).Handler()}
}

func (hc *harness) do(method, path string, body interface{}) (int, map[string]interface{}) {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	hc.h.ServeHTTP(w, r)
	out := map[string]interface{}{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out
}

func (hc *harness) post(path string, body interface{}) (int, map[string]interface{}) {
	return hc.do(http.MethodPost, path, body)
}

func (hc *harness) get(path string) (int, map[string]interface{}) {
	return hc.do(http.MethodGet, path, nil)
}

func factsOf(out map[string]interface{}) map[string]interface{} {
	return out["instance"].(map[string]interface{})["facts"].(map[string]interface{})
}

func TestEndToEndRewardOnceAndResume(t *testing.T) {
	hc := newHarness(t)

	status, out := hc.post("/api/instances", map[string]interface{}{
		"storyId": "hero-road", "id": "run-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("start status=%d body=%v", status, out)
	}
	submit := func(choiceID, requestID string) (int, map[string]interface{}) {
		return hc.post("/api/instances/run-1/choices", map[string]interface{}{
			"choiceId": choiceID, "requestId": requestID,
		})
	}

	if s, _ := submit("to_gate", "r1"); s != http.StatusOK {
		t.Fatal("to_gate failed")
	}
	// The page shows only currently allowed options: broke player cannot pay.
	status, view := hc.get("/api/instances/run-1/view")
	if status != http.StatusOK {
		t.Fatal(view)
	}
	choices := view["view"].(map[string]interface{})["availableChoices"].([]interface{})
	for _, c := range choices {
		if c.(map[string]interface{})["id"] == "pay" {
			t.Fatal("pay option must not be rendered for broke player")
		}
	}
	// A forged direct POST is rejected server-side, not just hidden in the UI.
	if s, body := submit("pay", "forged"); s != http.StatusConflict {
		t.Fatalf("forged pay status=%d body=%v", s, body)
	}

	if s, _ := submit("beg", "r2"); s != http.StatusOK {
		t.Fatal("beg failed")
	}
	if s, _ := submit("back_forest", "r3"); s != http.StatusOK {
		t.Fatal("back_forest failed")
	}
	s, first := submit("take_gold", "r4")
	if s != http.StatusOK {
		t.Fatal(first)
	}
	if factsOf(first)["gold"].(float64) != 10 {
		t.Fatalf("gold=%v", factsOf(first)["gold"])
	}

	// Interrupt: reopen the store from disk exactly like a restart.
	reopened, err := store.NewFileStore(hc.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reopened.GetInstance("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Current != "forest_after" {
		t.Fatalf("current=%s", reloaded.Current)
	}
	if reloaded.Facts["gold"].(float64) != 10 {
		t.Fatalf("reward did not survive restart: %v", reloaded.Facts)
	}
	reloadedH := server.New(reopened).Handler()
	viewReq := httptest.NewRequest(http.MethodGet, "/api/instances/run-1/view", nil)
	w := httptest.NewRecorder()
	reloadedH.ServeHTTP(w, viewReq)
	if w.Code != http.StatusOK {
		t.Fatalf("resume view status=%d", w.Code)
	}

	// Duplicate POST with the SAME requestId: no second reward.
	s, dup := submit("take_gold", "r4")
	if s != http.StatusOK || dup["result"].(map[string]interface{})["replayed"] != true {
		t.Fatalf("dup status=%d body=%v", s, dup)
	}
	s, check := hc.get("/api/instances/run-1")
	if factsOf(check)["gold"].(float64) != 10 {
		t.Fatalf("gold after duplicate = %v, want 10", factsOf(check)["gold"])
	}

	if s, _ := submit("back_gate", "r5"); s != http.StatusOK {
		t.Fatal("back_gate failed")
	}
	if s, body := submit("pay", "r6"); s != http.StatusOK {
		t.Fatalf("pay failed after earning gold: %v", body)
	}
	s, end := hc.get("/api/instances/run-1")
	inst := end["instance"].(map[string]interface{})
	if inst["current"] != "city" {
		t.Fatalf("current=%v", inst["current"])
	}
	if factsOf(end)["gold"].(float64) != 0 {
		t.Fatalf("gold=%v", factsOf(end)["gold"])
	}
}

func TestForkBranchesAreIndependentOverHTTP(t *testing.T) {
	hc := newHarness(t)

	hc.post("/api/instances", map[string]interface{}{
		"storyId": "hero-road", "id": "main", "facts": map[string]interface{}{"gold": 10.0},
	})
	hc.post("/api/instances/main/choices", map[string]interface{}{"choiceId": "to_gate", "requestId": "m1"})

	s, forkOut := hc.post("/api/instances/main/fork", map[string]interface{}{"id": "rehearsal", "name": "A"})
	if s != http.StatusCreated {
		t.Fatal(forkOut)
	}

	hc.post("/api/instances/main/choices", map[string]interface{}{"choiceId": "pay", "requestId": "m2"})
	hc.post("/api/instances/rehearsal/choices", map[string]interface{}{"choiceId": "beg", "requestId": "f1"})

	_, mainNow := hc.get("/api/instances/main")
	_, forkNow := hc.get("/api/instances/rehearsal")
	if factsOf(mainNow)["enteredCity"] != true {
		t.Fatal("main flag missing")
	}
	if factsOf(forkNow)["enteredCity"] != nil {
		t.Fatal("rehearsal inherited main flag")
	}
	if factsOf(forkNow)["gold"].(float64) != 10 {
		t.Fatal("rehearsal purse was mutated by main")
	}
	if forkNow["instance"].(map[string]interface{})["current"] != "gate_rejected" {
		t.Fatal("rehearsal did not advance independently")
	}
}

func TestOldSavePinnedAfterVersionPublish(t *testing.T) {
	hc := newHarness(t)

	_, out := hc.post("/api/instances", map[string]interface{}{
		"storyId": "hero-road", "id": "oldsave", "facts": map[string]interface{}{"gold": 15.0},
	})
	inst := out["instance"].(map[string]interface{})
	if inst["version"] != "1" {
		t.Fatalf("version=%v", inst["version"])
	}
	// Publish v2 (25 gold toll).
	if s, _ := hc.post("/api/stories/hero-road/versions", seed.HeroV2()); s != http.StatusOK {
		t.Fatal("publish v2 failed")
	}
	hc.post("/api/instances/oldsave/choices", map[string]interface{}{"choiceId": "to_gate", "requestId": "o1"})
	_, view := hc.get("/api/instances/oldsave/view")
	choices := view["view"].(map[string]interface{})["availableChoices"].([]interface{})
	sawPay := false
	for _, c := range choices {
		if c.(map[string]interface{})["id"] == "pay" {
			sawPay = true
		}
	}
	if !sawPay {
		t.Fatal("pinned v1 save must still offer the 10-gold toll")
	}
	if s, body := hc.post("/api/instances/oldsave/choices", map[string]interface{}{"choiceId": "pay", "requestId": "o2"}); s != http.StatusOK {
		t.Fatalf("resume pay: %v", body)
	}
}
