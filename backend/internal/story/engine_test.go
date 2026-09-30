package story

import (
	"errors"
	"testing"
)

func fact(k string, op string, v interface{}) *Condition {
	return &Condition{Fact: k, Op: op, Value: v}
}

// testStory builds a small story covering conditional choices and rewards:
//
//	start -> gate (requires gold >= 10) -> vault (pays item + flag) -> end
//	  |-> work (gold += 8) -> gate
func testStory() *Definition {
	return &Definition{
		ID: "test", Title: "t", Version: "v1", Start: "start",
		Nodes: map[string]Node{
			"start": {ID: "start", Type: "choice", Text: "s", Choices: []Choice{
				{ID: "work", Text: "work", Target: "gate", Effects: []Effect{{Op: "inc", Key: "gold", Value: float64(8)}}},
			}},
			"gate": {ID: "gate", Type: "choice", Text: "g", Choices: []Choice{
				{ID: "pay", Text: "pay", Target: "vault",
					Condition: fact("gold", ">=", float64(10)),
					Effects: []Effect{
						{Op: "inc", Key: "gold", Value: float64(-10)},
						{Op: "flag", Key: "paid"},
						{Op: "item", Key: "key"},
					}},
				{ID: "back", Text: "back", Target: "start"},
			}},
			"vault": {ID: "vault", Type: "choice", Text: "v", Choices: []Choice{
				{ID: "finish", Text: "finish", Target: "end", Effects: []Effect{{Op: "set", Key: "done", Value: true}}},
			}},
			"end": {ID: "end", Type: "end", Text: "done"},
		},
	}
}

func goldOf(t *testing.T, inst *Instance) float64 {
	t.Helper()
	g, ok := toFloat(inst.Facts["gold"])
	if !ok {
		t.Fatalf("gold missing or non-numeric: %#v", inst.Facts["gold"])
	}
	return g
}

func TestOnlyAllowedChoicesRendered(t *testing.T) {
	c := NewCatalog(testStory())
	inst, _, err := c.Start("test", "i1", map[string]interface{}{"gold": float64(0)})
	if err != nil {
		t.Fatal(err)
	}
	inst.Current = "gate"
	view, err := c.View(inst)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.AvailableChoices) != 1 || view.AvailableChoices[0].ID != "back" {
		t.Fatalf("gate choice must be hidden when gold < 10, got %+v", view.AvailableChoices)
	}
	if _, err := c.Apply(inst, "pay", ""); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("direct submit of hidden choice must fail, got %v", err)
	}
}

func TestDuplicateSubmissionGrantsOnce(t *testing.T) {
	c := NewCatalog(testStory())
	store := NewMemoryStore()
	inst, _, _ := c.Start("test", "i2", map[string]interface{}{"gold": float64(20)})
	inst.Current = "gate"

	applied, err := c.Apply(inst, "pay", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Rewards) != 3 {
		t.Fatalf("expected 3 rewards, got %d", len(applied.Rewards))
	}
	if g := goldOf(t, inst); g != 10 {
		t.Fatalf("gold should be 10 after payment, got %v", g)
	}
	items, _ := inst.Facts["items"].(map[string]interface{})
	if items["key"] != float64(1) {
		t.Fatalf("key item expected once, got %#v", items)
	}

	// Simulate the client double-posting the same choice: replay from the
	// persisted state (as a second HTTP request would) and submit again.
	if err := store.Save(inst); err != nil {
		t.Fatal(err)
	}
	inst2, err := store.Load("i2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(inst2, "pay", ""); !errors.Is(err, ErrDuplicateChoice) {
		t.Fatalf("resubmitted pay after commit must be rejected, got %v", err)
	}

	// ...and replaying against a stale in-memory snapshot at the gate is rejected.
	stale, _, _ := c.Start("test", "i2-stale", map[string]interface{}{"gold": float64(20)})
	stale.Current = "gate"
	if _, err := c.Apply(stale, "pay", ""); err != nil {
		t.Fatal(err)
	}
	_, err = c.Apply(stale, "pay", "")
	if !errors.Is(err, ErrDuplicateChoice) {
		t.Fatalf("repeated submission must return ErrDuplicateChoice, got %v", err)
	}
	if g := goldOf(t, stale); g != 10 {
		t.Fatalf("duplicate submit changed gold: %v", g)
	}
}

func TestConditionRecheckedAfterFactChange(t *testing.T) {
	c := NewCatalog(testStory())
	inst, _, _ := c.Start("test", "i3", map[string]interface{}{"gold": float64(12)})
	inst.Current = "gate"

	// Another path of play spends the gold before this request arrives.
	inst.Facts["gold"] = float64(4)
	if _, err := c.Apply(inst, "pay", ""); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("condition must be re-confirmed at submit time, got %v", err)
	}
	if _, ok := inst.Facts["paid"]; ok {
		t.Fatal("failed conditional choice must not grant rewards")
	}
	if inst.Current != "gate" {
		t.Fatalf("node must be unchanged on failed submit, got %s", inst.Current)
	}
}

func TestResumeAfterReward(t *testing.T) {
	c := NewCatalog(testStory())
	store := NewMemoryStore()
	inst, _, _ := c.Start("test", "i4", map[string]interface{}{"gold": float64(20)})
	inst.Current = "gate"
	if _, err := c.Apply(inst, "pay", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(inst); err != nil {
		t.Fatal(err)
	}

	// Interruption: a brand new request reloads the instance from storage.
	resumed, err := store.Load("i4")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Current != "vault" {
		t.Fatalf("resume should land on vault, got %s", resumed.Current)
	}
	view, err := c.View(resumed)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.AvailableChoices) != 1 || view.AvailableChoices[0].ID != "finish" {
		t.Fatalf("resumed view wrong: %+v", view.AvailableChoices)
	}
	if len(resumed.Rewards) != 3 {
		t.Fatalf("reward history must survive resume, got %d", len(resumed.Rewards))
	}
}

func TestForkIsolation(t *testing.T) {
	c := NewCatalog(testStory())
	inst, _, _ := c.Start("test", "root", map[string]interface{}{"gold": float64(20)})
	inst.Current = "gate"

	branch := Fork(inst, "rehearsal-1", "买钥匙试试")
	if _, err := c.Apply(branch, "pay", ""); err != nil {
		t.Fatal(err)
	}
	if g := goldOf(t, branch); g != 10 {
		t.Fatalf("branch gold should be 10, got %v", g)
	}

	// Parent state must be untouched.
	if g := goldOf(t, inst); g != 20 {
		t.Fatalf("branch rewards leaked into parent: gold=%v", g)
	}
	if inst.Current != "gate" {
		t.Fatalf("parent node moved: %s", inst.Current)
	}
	if _, ok := inst.Facts["paid"]; ok {
		t.Fatal("branch flag leaked into parent facts")
	}
	if len(inst.Branches) != 1 || inst.Branches["rehearsal-1"].ID != "rehearsal-1" {
		t.Fatalf("branch marker missing on parent: %+v", inst.Branches)
	}

	// Item maps must not be shared either.
	branch.Facts["items"].(map[string]interface{})["key"] = float64(5)
	if _, ok := inst.Facts["items"]; ok {
		t.Fatal("item map is shared between branch and parent")
	}

	// The same choice can still be taken independently in the parent.
	if _, err := c.Apply(inst, "pay", ""); err != nil {
		t.Fatalf("parent play should be independent: %v", err)
	}
}

func TestVersionDriftRejected(t *testing.T) {
	d := testStory()
	c := NewCatalog(d)
	inst, _, _ := c.Start("test", "i5", map[string]interface{}{"gold": float64(20)})

	// Author edits the story after the instance started.
	d2 := testStory()
	d2.Nodes["gate"] = d2.Nodes["gate"] // force a structural edit below
	n := d2.Nodes["gate"]
	n.Choices[0].Condition = fact("gold", ">=", float64(99))
	d2.Nodes["gate"] = n
	c2 := NewCatalog(d2)
	if _, err := c2.View(inst); !errors.Is(err, ErrStoryVersionDrift) {
		t.Fatalf("changed story version must be rejected, got %v", err)
	}
}

func TestSeedStoryFullChain(t *testing.T) {
	c := NewCatalog(testStory())
	inst, _, err := c.Start("test", "i6", map[string]interface{}{"gold": float64(0)})
	if err != nil {
		t.Fatal(err)
	}
	steps := []string{"work", "back", "work", "pay", "finish"}
	for _, ch := range steps {
		view, err := c.View(inst)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ac := range view.AvailableChoices {
			if ac.ID == ch {
				found = true
			}
		}
		if !found {
			t.Fatalf("step %s not available at %s: %+v", ch, inst.Current, view.AvailableChoices)
		}
		if _, err := c.Apply(inst, ch, ""); err != nil {
			t.Fatalf("step %s: %v", ch, err)
		}
	}
	if inst.Current != "end" {
		t.Fatalf("expected end, got %s", inst.Current)
	}
	if done, _ := inst.Facts["done"].(bool); !done {
		t.Fatal("final effect missing")
	}
}
