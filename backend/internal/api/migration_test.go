package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// reviseSeed returns the seed story with a revised reward and version.
func reviseSeed(t *testing.T) string {
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
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func putJSON(t *testing.T, h http.Handler, path, body string) (int, map[string]interface{}) {
	t.Helper()
	return doJSON(t, h, http.MethodPut, path, body)
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out
}

func TestRuleRevisionReviewAndAdoptOverHTTP(t *testing.T) {
	h, _, _ := newTestServer(t)

	status, _ := postJSON(t, h, "/api/instances", `{"id":"rev1"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d", status)
	}
	postJSON(t, h, "/api/instances/rev1/choices", `{"choiceId":"chop_wood","idempotencyKey":"r1"}`)

	// Author publishes a revised definition.
	status, body := putJSON(t, h, "/api/stories/mist-harbor", reviseSeed(t))
	if status != http.StatusOK {
		t.Fatalf("update story: %d %v", status, body)
	}

	// The existing save reports drift but keeps playing under old rules.
	status, body = getJSON(t, h, "/api/instances/rev1")
	if status != http.StatusOK {
		t.Fatalf("get after revision: %d", status)
	}
	if body["versionDrift"] != true {
		t.Fatalf("expected versionDrift flag: %v", body)
	}
	inst := body["instance"].(map[string]interface{})
	if inst["version"] != "2026-09-30.v1" {
		t.Fatalf("save must stay pinned to old version, got %v", inst["version"])
	}

	// Impact review: the completed chop_wood event is flagged as revised but
	// kept as-is; future choices at lumberyard are listed.
	status, body = getJSON(t, h, "/api/instances/rev1/migration")
	if status != http.StatusOK {
		t.Fatalf("preview: %d", status)
	}
	mig := body["migration"].(map[string]interface{})
	if mig["upToDate"] != false {
		t.Fatal("preview must report drift")
	}
	completed, _ := mig["completed"].([]interface{})
	if len(completed) != 1 {
		t.Fatalf("expected 1 completed event, got %v", completed)
	}
	ev := completed[0].(map[string]interface{})
	if ev["status"] != "revised" {
		t.Fatalf("chop_wood reward changed, expected revised, got %v", ev)
	}

	// Adopt the new rules for this save only.
	status, body = postJSON(t, h, "/api/instances/rev1/migration", `{"action":"adopt"}`)
	if status != http.StatusOK {
		t.Fatalf("adopt: %d %v", status, body)
	}
	inst = body["instance"].(map[string]interface{})
	if inst["version"] != "2026-10-02.v2" {
		t.Fatalf("expected re-pin to v2, got %v", inst["version"])
	}
	// Already-claimed rewards are not re-granted: gold stays 5.
	facts := inst["facts"].(map[string]interface{})
	if facts["gold"].(float64) != 5 {
		t.Fatalf("adoption must not re-grant rewards, gold=%v", facts["gold"])
	}
	// The boundary is explained in the persisted log.
	logEntries, _ := inst["log"].([]interface{})
	last := logEntries[len(logEntries)-1].(map[string]interface{})
	if !strings.Contains(last["note"].(string), "规则迁移") {
		t.Fatalf("migration boundary note missing: %v", last)
	}

	// Repeating the adoption is a no-op.
	status, body = postJSON(t, h, "/api/instances/rev1/migration", `{"action":"adopt"}`)
	if status != http.StatusOK {
		t.Fatalf("repeat adopt: %d", status)
	}
	inst = body["instance"].(map[string]interface{})
	if got, _ := inst["log"].([]interface{}); len(got) != len(logEntries) {
		t.Fatal("repeated adoption must not append another boundary note")
	}
	if body["versionDrift"] != false {
		t.Fatal("drift must be cleared after adoption")
	}
}

func TestMigrationDeferKeepsOldRulesOverHTTP(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"rev2"}`)
	if status, body := putJSON(t, h, "/api/stories/mist-harbor", reviseSeed(t)); status != http.StatusOK {
		t.Fatalf("update story: %d %v", status, body)
	}

	status, body := postJSON(t, h, "/api/instances/rev2/migration", `{"action":"defer"}`)
	if status != http.StatusOK {
		t.Fatalf("defer: %d %v", status, body)
	}
	inst := body["instance"].(map[string]interface{})
	if inst["migrationDeferred"] == nil || inst["migrationDeferred"] == "" {
		t.Fatal("defer marker missing")
	}

	// The deferred save still plays under the old reward rules.
	status, body = postJSON(t, h, "/api/instances/rev2/choices", `{"choiceId":"chop_wood","idempotencyKey":"d1"}`)
	if status != http.StatusOK {
		t.Fatalf("old-rule choice: %d %v", status, body)
	}
	if got := instFacts(t, body)["gold"].(float64); got != 5 {
		t.Fatalf("deferred save must use old reward, gold=%v", got)
	}

	// A brand-new save uses the revised rules.
	postJSON(t, h, "/api/instances", `{"id":"rev3"}`)
	_, body = postJSON(t, h, "/api/instances/rev3/choices", `{"choiceId":"chop_wood","idempotencyKey":"d2"}`)
	if got := instFacts(t, body)["gold"].(float64); got != 7 {
		t.Fatalf("new save must use revised reward, gold=%v", got)
	}
}
