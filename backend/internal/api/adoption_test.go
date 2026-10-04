package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

// newFileServer builds a handler backed by file stores, like the real
// server, and returns a restart func that rebuilds everything from disk.
func newFileServer(t *testing.T, dir string) (http.Handler, func() http.Handler) {
	t.Helper()
	start := func() http.Handler {
		store, err := story.NewFileStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		defs, err := story.NewFileDefStore(filepath.Join(dir, "definitions"))
		if err != nil {
			t.Fatal(err)
		}
		catalog := story.NewCatalog(seed.Story())
		if err := LoadPersistedRevisions(catalog, defs); err != nil {
			t.Fatal(err)
		}
		h := NewHandler(catalog, store, seed.InitialFacts())
		h.SetDefStore(defs)
		mux := http.NewServeMux()
		h.Register(mux)
		return mux
	}
	return start(), start
}

// reviseSeedWith returns a modified seed definition as JSON.
func reviseSeedWith(t *testing.T, version string, mutate func(d *story.Definition)) string {
	t.Helper()
	def := seed.Story()
	def.Version = version
	mutate(def)
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func goldReward(n float64) func(d *story.Definition) {
	return func(d *story.Definition) {
		sq := d.Nodes["square"]
		for i := range sq.Choices {
			if sq.Choices[i].ID == "chop_wood" {
				sq.Choices[i].Effects = []story.Effect{{Op: "inc", Key: "gold", Value: n}}
			}
		}
		d.Nodes["square"] = sq
	}
}

// Two branches of the same save adopt different revisions; after a full
// process restart each branch still finds the rules it actually adopted.
func TestBranchesAdoptDifferentVersionsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	h, restart := newFileServer(t, filepath.Join(dir, "data"))

	postJSON(t, h, "/api/instances", `{"id":"root"}`)
	postJSON(t, h, "/api/instances/root/forks", `{"id":"b1","label":"分支一"}`)
	postJSON(t, h, "/api/instances/root/forks", `{"id":"b2","label":"分支二"}`)

	// Publish v2; branch b1 adopts it.
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v2", goldReward(7)))
	if code, body := postJSON(t, h, "/api/instances/b1/migration", `{"action":"adopt"}`); code != http.StatusOK {
		t.Fatalf("b1 adopt v2: %d %v", code, body)
	}
	// Publish v3; branch b2 adopts that instead.
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v3", goldReward(11)))
	if code, body := postJSON(t, h, "/api/instances/b2/migration", `{"action":"adopt"}`); code != http.StatusOK {
		t.Fatalf("b2 adopt v3: %d %v", code, body)
	}

	// Restart: adopted revisions must be found again for both branches.
	h2 := restart()
	for id, want := range map[string]string{"b1": "v2", "b2": "v3"} {
		code, body := getJSON(t, h2, "/api/instances/"+id)
		if code != http.StatusOK {
			t.Fatalf("%s after restart: %d %v", id, code, body)
		}
		inst := body["instance"].(map[string]interface{})
		if inst["version"] != want {
			t.Fatalf("%s must stay on %s, got %v", id, want, inst["version"])
		}
		if body["migrationState"] != StateAdopted {
			t.Fatalf("%s state = %v, want adopted", id, body["migrationState"])
		}
	}

	// Each branch keeps playing under its own adopted reward rules.
	_, body := postJSON(t, h2, "/api/instances/b1/choices", `{"choiceId":"chop_wood","idempotencyKey":"k-b1"}`)
	if got := instFacts(t, body)["gold"].(float64); got != 7 {
		t.Fatalf("b1 must use v2 reward 7, got %v", got)
	}
	_, body = postJSON(t, h2, "/api/instances/b2/choices", `{"choiceId":"chop_wood","idempotencyKey":"k-b2"}`)
	if got := instFacts(t, body)["gold"].(float64); got != 11 {
		t.Fatalf("b2 must use v3 reward 11, got %v", got)
	}
}

// A revision that deletes future nodes can be adopted; a revision that
// deletes the current node is refused and progress stays put.
func TestAdoptWithDeletedFutureNodes(t *testing.T) {
	h, _, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"del1"}`)

	// Revision removing a node the save has not reached yet: adoptable.
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v2-no-market", func(d *story.Definition) {
		delete(d.Nodes, "market")
		sq := d.Nodes["square"]
		kept := sq.Choices[:0]
		for _, ch := range sq.Choices {
			if ch.Target != "market" {
				kept = append(kept, ch)
			}
		}
		sq.Choices = kept
		d.Nodes["square"] = sq
	}))
	if code, body := postJSON(t, h, "/api/instances/del1/migration", `{"action":"adopt"}`); code != http.StatusOK {
		t.Fatalf("adopt with future node removed: %d %v", code, body)
	}

	// Revision removing the save's current node: refused, save stays put.
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v3-no-square", func(d *story.Definition) {
		delete(d.Nodes, "square")
		d.Start = "lumberyard"
		for id, n := range d.Nodes {
			for i := range n.Choices {
				if n.Choices[i].Target == "square" {
					n.Choices[i].Target = "lumberyard"
				}
			}
			d.Nodes[id] = n
		}
	}))
	code, body := postJSON(t, h, "/api/instances/del1/migration", `{"action":"adopt"}`)
	if code != http.StatusConflict {
		t.Fatalf("adopt removing current node must be 409, got %d %v", code, body)
	}
	code, body = getJSON(t, h, "/api/instances/del1")
	if code != http.StatusOK {
		t.Fatalf("save must still load after refused adoption: %d", code)
	}
	inst := body["instance"].(map[string]interface{})
	if inst["version"] != "v2-no-market" || inst["current"] != "square" {
		t.Fatalf("progress must stay on v2-no-market@square, got %v@%v", inst["version"], inst["current"])
	}
	if inst["pendingMigration"] != nil {
		t.Fatal("refused adoption must not leave a pending decision")
	}
}

// An adopt decision interrupted mid-flight reloads as awaiting-recovery:
// facts and the pending decision are shown, choices are blocked, and
// repeated recovery never re-grants rewards or appends extra log entries.
func TestInterruptedAdoptionRecoveryIsIdempotent(t *testing.T) {
	h, catalog, store := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"rec1"}`)
	postJSON(t, h, "/api/instances/rec1/choices", `{"choiceId":"chop_wood","idempotencyKey":"rk1"}`)
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v2", goldReward(7)))

	// Simulate a crash right after the decision was saved: begin the
	// adoption and persist, but never complete it.
	inst, err := store.Load("rec1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BeginAdoption(inst); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(inst); err != nil {
		t.Fatal(err)
	}

	// The save reports awaiting-recovery with the pending decision visible.
	code, body := getJSON(t, h, "/api/instances/rec1")
	if code != http.StatusOK {
		t.Fatalf("load awaiting recovery: %d", code)
	}
	if body["migrationState"] != StateAwaitingRecovery {
		t.Fatalf("state = %v, want awaiting-recovery", body["migrationState"])
	}
	instMap := body["instance"].(map[string]interface{})
	pending, _ := instMap["pendingMigration"].(map[string]interface{})
	if pending["toVersion"] != "v2" {
		t.Fatalf("pending decision missing: %v", instMap["pendingMigration"])
	}
	if instFacts(t, body)["gold"].(float64) != 5 {
		t.Fatal("facts must be intact while awaiting recovery")
	}

	// Choices are refused until the decision is completed.
	if code, _ := postJSON(t, h, "/api/instances/rec1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"rk2"}`); code != http.StatusConflict {
		t.Fatalf("choice during recovery must be 409, got %d", code)
	}

	// Recover, then recover again: identical outcome, no double effects.
	for i := 0; i < 2; i++ {
		code, body := postJSON(t, h, "/api/instances/rec1/migration", `{"action":"adopt"}`)
		if code != http.StatusOK {
			t.Fatalf("recovery %d: %d %v", i, code, body)
		}
		instMap := body["instance"].(map[string]interface{})
		if instMap["version"] != "v2" {
			t.Fatalf("recovery %d must land on v2, got %v", i, instMap["version"])
		}
		if instMap["pendingMigration"] != nil {
			t.Fatalf("recovery %d must clear the pending decision", i)
		}
		entries, _ := instMap["log"].([]interface{})
		notes := 0
		for _, e := range entries {
			if n, _ := e.(map[string]interface{})["note"].(string); strings.Contains(n, "规则迁移") {
				notes++
			}
		}
		if notes != 1 {
			t.Fatalf("recovery %d appended duplicate boundary notes: %d", i, notes)
		}
		if got := instFacts(t, body)["gold"].(float64); got != 5 {
			t.Fatalf("recovery %d changed facts, gold=%v", i, got)
		}
		if got := len(instMap["rewards"].([]interface{})); got != 1 {
			t.Fatalf("recovery %d duplicated rewards: %d", i, got)
		}
	}

	// Play continues under the adopted rules.
	code, body = postJSON(t, h, "/api/instances/rec1/choices", `{"choiceId":"deliver_wood","idempotencyKey":"rk3"}`)
	if code != http.StatusOK {
		t.Fatalf("continue after recovery: %d %v", code, body)
	}
	if got := instFacts(t, body)["gold"].(float64); got != 13 {
		t.Fatalf("post-recovery reward wrong, gold=%v", got)
	}
}

// Unused revisions can be withdrawn; revisions referenced by saves or
// history markers are kept and the refusal says why.
func TestWithdrawRevision(t *testing.T) {
	h, catalog, _ := newTestServer(t)
	postJSON(t, h, "/api/instances", `{"id":"wd1"}`)

	// v2 is published but never adopted; v3 becomes the latest.
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v2", goldReward(7)))
	v2Def, _ := catalog.Get("mist-harbor")
	v2Hash, _ := story.DefHash(v2Def)
	putJSON(t, h, "/api/stories/mist-harbor", reviseSeedWith(t, "v3", goldReward(9)))

	// The seed v1 revision is pinned by the save: withdrawal refused.
	seedHash := ""
	for _, rev := range catalog.Revisions("mist-harbor") {
		if rev.Version == "2026-09-30.v1" {
			seedHash = rev.Hash
		}
	}
	if seedHash == "" {
		t.Fatal("seed revision missing from history")
	}
	code, body := doJSON(t, h, http.MethodDelete, "/api/stories/mist-harbor/revisions/"+seedHash, "")
	if code != http.StatusConflict {
		t.Fatalf("withdrawing a pinned revision must be 409, got %d", code)
	}
	if !strings.Contains(body["error"].(string), "wd1") {
		t.Fatalf("refusal must name the referencing save: %v", body)
	}

	// The latest published revision cannot be withdrawn either.
	v3Def, _ := catalog.Get("mist-harbor")
	v3Hash, _ := story.DefHash(v3Def)
	if code, _ := doJSON(t, h, http.MethodDelete, "/api/stories/mist-harbor/revisions/"+v3Hash, ""); code != http.StatusConflict {
		t.Fatalf("withdrawing the latest revision must be 409, got %d", code)
	}

	// v2 is unreferenced: withdrawal succeeds and it disappears.
	code, _ = doJSON(t, h, http.MethodDelete, "/api/stories/mist-harbor/revisions/"+v2Hash, "")
	if code != http.StatusOK {
		t.Fatalf("withdraw unused revision: %d", code)
	}
	if _, ok := catalog.RevisionByHash(v2Hash); ok {
		t.Fatal("withdrawn revision must leave the history")
	}

	// The save pinned to v1 is unaffected and still opens.
	if code, _ := getJSON(t, h, "/api/instances/wd1"); code != http.StatusOK {
		t.Fatalf("save must still open after unrelated withdrawal: %d", code)
	}
}
