package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

func newTestServer(t *testing.T) (http.Handler, *story.Catalog, story.Store) {
	t.Helper()
	catalog := story.NewCatalog(seed.Story())
	store := story.NewMemoryStore()
	h := NewHandler(catalog, store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, catalog, store
}

func postJSON(t *testing.T, h http.Handler, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out
}

func getJSON(t *testing.T, h http.Handler, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func instFacts(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	i, ok := body["instance"].(map[string]interface{})
	if !ok {
		t.Fatalf("no instance in response: %#v", body)
	}
	f, _ := i["facts"].(map[string]interface{})
	return f
}

func choiceIDs(body map[string]interface{}) []string {
	v, _ := body["view"].(map[string]interface{})
	list, _ := v["availableChoices"].([]interface{})
	var ids []string
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func TestListInstancesEmptyMemoryStore(t *testing.T) {
	h, _, _ := newTestServer(t)
	code, body := getJSON(t, h, "/api/instances")
	if code != http.StatusOK {
		t.Fatalf("list instances: got %d", code)
	}
	if _, ok := body["instances"].([]interface{}); !ok {
		t.Fatalf("empty store must return an empty JSON array, got %#v", body["instances"])
	}
}

func TestListInstancesEmptyFileStore(t *testing.T) {
	catalog := story.NewCatalog(seed.Story())
	store, err := story.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(catalog, store, seed.InitialFacts())
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"instances":[]`)) {
		t.Fatalf("empty file store must serialise as [], got %s", w.Body.String())
	}
}

func TestFreshInstanceSerialisesEmptyRewards(t *testing.T) {
	h, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(`{"id":"fresh"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}
	// A fresh save must send rewards:[] — clients crash on an undefined
	// collection when the field is omitted.
	if !bytes.Contains(w.Body.Bytes(), []byte(`"rewards":[]`)) {
		t.Fatalf("fresh instance must include an empty rewards array: %s", w.Body.String())
	}
}

func TestFullPlayHTTP(t *testing.T) {
	h, _, _ := newTestServer(t)

	status, body := postJSON(t, h, "/api/instances", `{"id":"p1"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, body)
	}
	ids := choiceIDs(body)
	if len(ids) != 2 || ids[0] != "chop_wood" {
		// bribe_guard must be hidden with gold 0
		t.Fatalf("initial choices wrong: %v", ids)
	}

	// Hidden choice rejected server-side.
	status, body = postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"bribe_guard"}`)
	if status != http.StatusConflict {
		t.Fatalf("hidden choice expected 409, got %d", status)
	}

	// Earn 5 then earn 8 -> 13 gold.
	postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"chop_wood","idempotencyKey":"k1"}`)
	postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"k2"}`)

	// Replay k1 with the same idempotency key after we have moved on:
	// must not grant again.
	status, body = postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"chop_wood","idempotencyKey":"k1"}`)
	if status != http.StatusOK {
		t.Fatalf("idem replay expected 200, got %d %v", status, body)
	}
	facts := instFacts(t, body)
	if facts["gold"].(float64) != 13 {
		t.Fatalf("idem replay re-granted reward: gold=%v", facts["gold"])
	}

	// Bare double-submit of the current choice (a second HTTP request after
	// the first committed) is rejected and grants nothing.
	status, _ = postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"deliver_wood"}`)
	if status != http.StatusConflict {
		t.Fatalf("duplicate resubmit expected 409, got %d", status)
	}

	// Interruption: fresh GET resumes at lumberyard with full history.
	status, body = getJSON(t, h, "/api/instances/p1")
	if status != http.StatusOK {
		t.Fatalf("resume: %d", status)
	}
	ids = choiceIDs(body)
	if i2 := body["instance"].(map[string]interface{}); i2["current"] != "square" {
		t.Fatalf("expected resume at square, got %v", i2["current"])
	}
	i := body["instance"].(map[string]interface{})
	rewards, _ := i["rewards"].([]interface{})
	if len(rewards) != 2 {
		t.Fatalf("reward history wrong after resume: %d", len(rewards))
	}

	// Finish the run: deliver -> bribe -> walk out.
	status, body = postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"bribe_guard","idempotencyKey":"k5"}`)
	if status != http.StatusOK {
		t.Fatalf("bribe expected 200 with 13 gold, got %d %v", status, body)
	}
	facts = instFacts(t, body)
	if facts["gold"].(float64) != 3 {
		t.Fatalf("gold after bribe expected 3, got %v", facts["gold"])
	}
	postJSON(t, h, "/api/instances/p1/choices", `{"choiceId":"walk_out","idempotencyKey":"k6"}`)
	status, body = getJSON(t, h, "/api/instances/p1")
	i = body["instance"].(map[string]interface{})
	if i["current"] != "ending_normal" {
		t.Fatalf("expected end node, got %v", i["current"])
	}
}

func TestForkHTTPIIsolation(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"root"}`)
	postJSON(t, h, "/api/instances/root/choices", `{"choiceId":"chop_wood","idempotencyKey":"a"}`)

	status, body := postJSON(t, h, "/api/instances/root/forks", `{"id":"b1","label":"试演：拿工资"}`)
	if status != http.StatusCreated {
		t.Fatalf("fork: %d %v", status, body)
	}

	// Branch collects the 8 gold.
	status, body = postJSON(t, h, "/api/instances/b1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"ab1"}`)
	if status != http.StatusOK {
		t.Fatalf("branch deliver: %d %v", status, body)
	}
	if g := instFacts(t, body)["gold"].(float64); g != 13 {
		t.Fatalf("branch gold expected 13, got %v", g)
	}

	// Parent is still at lumberyard with 5 gold and no 8-gold reward.
	_, parent := getJSON(t, h, "/api/instances/root")
	p := parent["instance"].(map[string]interface{})
	if p["current"] != "lumberyard" {
		t.Fatalf("parent node changed: %v", p["current"])
	}
	pf, _ := p["facts"].(map[string]interface{})
	if pf["gold"].(float64) != 5 {
		t.Fatalf("branch reward leaked into parent: %v", pf["gold"])
	}
	branches, _ := p["branches"].(map[string]interface{})
	if _, ok := branches["b1"]; !ok {
		t.Fatalf("fork marker missing: %v", branches)
	}

	// Branches can fork again independently.
	status, _ = postJSON(t, h, "/api/instances/b1/forks", `{"id":"b2"}`)
	if status != http.StatusCreated {
		t.Fatalf("nested fork: %d", status)
	}
}

func TestConcurrentDoubleSubmit(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"c1"}`)
	postJSON(t, h, "/api/instances/c1/choices", `{"choiceId":"chop_wood","idempotencyKey":"first"}`)

	var wg sync.WaitGroup
	statuses := make([]int, 8)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/instances/c1/choices",
				bytes.NewReader([]byte(`{"choiceId":"deliver_wood"}`)))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			statuses[i] = w.Code
		}(i)
	}
	wg.Wait()
	ok, dup := 0, 0
	for _, s := range statuses {
		if s == http.StatusOK {
			ok++
		}
		if s == http.StatusConflict {
			dup++
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one concurrent submit must succeed, got %d success", ok)
	}
	if dup != 7 {
		t.Fatalf("other concurrent submits must be duplicates, got %d", dup)
	}
	_, body := getJSON(t, h, "/api/instances/c1")
	facts := instFacts(t, body)
	if facts["gold"].(float64) != 13 {
		t.Fatalf("concurrent double submit granted extra reward: %v", facts["gold"])
	}
}
