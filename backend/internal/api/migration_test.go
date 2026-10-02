package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"storyplatform/internal/seed"
)

func cloneSeed(t *testing.T) map[string]interface{} {
	t.Helper()
	b, _ := json.Marshal(seed.Story())
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMigrationReviewAndPerBranchAdoption(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"old-branch"}`)
	postJSON(t, h, "/api/instances", `{"id":"stay-branch"}`)
	postJSON(t, h, "/api/instances/old-branch/choices", `{"choiceId":"chop_wood","idempotencyKey":"w1"}`)
	postJSON(t, h, "/api/instances/stay-branch/choices", `{"choiceId":"chop_wood","idempotencyKey":"s1"}`)

	updated := cloneSeed(t)
	updated["version"] = "2026-10-02.v2"
	nodes := updated["nodes"].(map[string]interface{})
	lumber := nodes["lumberyard"].(map[string]interface{})
	choices := lumber["choices"].([]interface{})
	deliver := choices[0].(map[string]interface{})
	deliver["effects"] = []map[string]interface{}{{"op": "inc", "key": "gold", "value": 20}}
	body, _ := json.Marshal(updated)
	req := httptest.NewRequest(http.MethodPut, "/api/stories/mist-harbor", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("publish revision: %d %s", w.Code, w.Body.String())
	}

	code, reviewBody := getJSON(t, h, "/api/migrations")
	if code != http.StatusOK {
		t.Fatalf("list reviews: %d", code)
	}
	migrations, _ := reviewBody["migrations"].([]interface{})
	if len(migrations) != 2 {
		t.Fatalf("both saves need review, got %#v", migrations)
	}

	// The old, already granted five gold and completed chop-wood event remain
	// untouched. Adopting only this branch uses the new future reward.
	if code, out := postJSON(t, h, "/api/instances/old-branch/migration", `{}`); code != http.StatusOK {
		t.Fatalf("adopt migration: %d %#v", code, out)
	}
	if code, out := postJSON(t, h, "/api/instances/old-branch/choices", `{"choiceId":"deliver_wood","idempotencyKey":"w2"}`); code != http.StatusOK {
		t.Fatalf("new-rule future choice: %d %#v", code, out)
	} else if g := instFacts(t, out)["gold"].(float64); g != 25 {
		t.Fatalf("new branch reward should be 5+20, got %v", g)
	}

	// The other save explicitly remains on its pinned old version.
	if code, out := postJSON(t, h, "/api/instances/stay-branch/choices", `{"choiceId":"deliver_wood","idempotencyKey":"s2"}`); code != http.StatusOK {
		t.Fatalf("old-rule continuation: %d %#v", code, out)
	} else if g := instFacts(t, out)["gold"].(float64); g != 13 {
		t.Fatalf("unadopted branch must keep 5+8 reward, got %v", g)
	}

	_, old := getJSON(t, h, "/api/instances/old-branch")
	rewards, _ := old["instance"].(map[string]interface{})["rewards"].([]interface{})
	if len(rewards) != 2 {
		t.Fatalf("migration must not replay completed rewards: %d", len(rewards))
	}
}
