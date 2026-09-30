package story_test

import (
	"testing"

	"storyplatform/internal/seed"
	"storyplatform/internal/story"
)

func choiceIDs(view story.NodeView) map[string]bool {
	out := map[string]bool{}
	for _, c := range view.AvailableChoices {
		out[c.ID] = true
	}
	return out
}

func TestOnlyAllowedChoicesAreShown(t *testing.T) {
	def := seed.HeroV1()
	inst, err := story.StartInstance("t1", def, nil)
	if err != nil {
		t.Fatal(err)
	}
	// start -> forest
	if _, err := inst.SubmitChoice(def, "to_forest", "r1"); err != nil {
		t.Fatal(err)
	}
	// take gold once: 0 -> 10
	res, err := inst.SubmitChoice(def, "take_gold", "r2")
	if err != nil {
		t.Fatal(err)
	}
	if got := inst.Facts["gold"].(float64); got != 10 {
		t.Fatalf("gold = %v, want 10", got)
	}
	// forest_after -> gate: pay is now allowed with exactly 10.
	if _, err := inst.SubmitChoice(def, "back_gate", "r3"); err != nil {
		t.Fatal(err)
	}
	view, err := inst.View(def)
	if err != nil {
		t.Fatal(err)
	}
	ids := choiceIDs(view)
	if !ids["pay"] {
		t.Fatalf("pay should be allowed with 10 gold, view=%v", ids)
	}

	// Condition change: after paying, gate is gone; negative re-confirmation check:
	// fresh instance with 0 gold must not see pay.
	poor, _ := story.StartInstance("poor", def, nil)
	if _, err := poor.SubmitChoice(def, "to_gate", "x1"); err != nil {
		t.Fatal(err)
	}
	poorView, _ := poor.View(def)
	if choiceIDs(poorView)["pay"] {
		t.Fatal("pay must be hidden when gold < 10")
	}
	if _, err := poor.SubmitChoice(def, "pay", "x2"); err == nil {
		t.Fatal("server must reject pay even if client submits it directly")
	}
	_ = res
}

func TestRepeatedSubmissionGrantsRewardOnce(t *testing.T) {
	def := seed.HeroV1()
	inst, _ := story.StartInstance("rep", def, nil)
	if _, err := inst.SubmitChoice(def, "to_forest", "req-1"); err != nil {
		t.Fatal(err)
	}
	first, err := inst.SubmitChoice(def, "take_gold", "req-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rewards) != 2 {
		t.Fatalf("first submit rewards=%v", first.Rewards)
	}
	// Replay with the same idempotency key must not apply effects again.
	replay, err := inst.SubmitChoice(def, "take_gold", "req-2")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed {
		t.Fatal("expected replay marker on duplicate request")
	}
	if len(replay.Rewards) != 0 {
		t.Fatalf("replay must grant no rewards, got %v", replay.Rewards)
	}
	if got := inst.Facts["gold"].(float64); got != 10 {
		t.Fatalf("gold after replay = %v, want 10", got)
	}
	if len(inst.Claimed) != 2 {
		t.Fatalf("claimed=%v", inst.Claimed)
	}
}

func TestForksDoNotPolluteEachOther(t *testing.T) {
	def := seed.HeroV1()
	parent, _ := story.StartInstance("parent", def, map[string]interface{}{"gold": float64(10)})
	if _, err := parent.SubmitChoice(def, "to_gate", "p1"); err != nil {
		t.Fatal(err)
	}
	// Fork BEFORE the pay decision.
	fork := parent.Fork("fork-a", "rehearsal A")
	if fork.ParentID != "parent" {
		t.Fatal("fork must record parent")
	}
	// Parent pays and enters the city.
	if _, err := parent.SubmitChoice(def, "pay", "p2"); err != nil {
		t.Fatal(err)
	}
	if parent.Current != "city" {
		t.Fatalf("parent current=%s", parent.Current)
	}
	if got := parent.Facts["gold"].(float64); got != 0 {
		t.Fatalf("parent gold=%v", got)
	}
	// Fork still stands at the gate with the original purse.
	if fork.Current != "gate" {
		t.Fatalf("fork current=%s, want gate", fork.Current)
	}
	if got := fork.Facts["gold"].(float64); got != 10 {
		t.Fatalf("fork gold leaked: %v", got)
	}
	if fork.Facts["enteredCity"] != nil {
		t.Fatal("fork must not inherit parent flag after forking")
	}
	// Mutating the fork (beg -> forest -> take gold) must not touch the parent.
	if _, err := fork.SubmitChoice(def, "beg", "f1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fork.SubmitChoice(def, "back_forest", "f2"); err != nil {
		t.Fatal(err)
	}
	if _, err := fork.SubmitChoice(def, "take_gold", "f3"); err != nil {
		t.Fatal(err)
	}
	if got := fork.Facts["gold"].(float64); got != 20 {
		t.Fatalf("fork gold=%v, want 20", got)
	}
	if parent.Facts["tookGold"] != nil {
		t.Fatal("parent was polluted by fork flag")
	}
	if len(fork.Claimed) != len(parent.Claimed)+2 {
		t.Fatalf("claims leaked: parent=%v fork=%v", parent.Claimed, fork.Claimed)
	}
}

func TestOldSaveContinuesOnPinnedVersion(t *testing.T) {
	v1 := seed.HeroV1()
	v2 := seed.HeroV2()
	inst, _ := story.StartInstance("old", v1, map[string]interface{}{"gold": float64(15)})
	// Play to the gate under v1.
	if _, err := inst.SubmitChoice(v1, "to_gate", "o1"); err != nil {
		t.Fatal(err)
	}
	// A new version is published: 25-gold toll. Old save must still use v1.
	view, err := inst.View(v1)
	if err != nil {
		t.Fatal(err)
	}
	if !choiceIDs(view)["pay"] {
		t.Fatal("15 gold passes the v1 toll (10), option must remain")
	}
	if _, err := inst.View(v2); err == nil {
		t.Fatal("view against v2 must fail the hash check instead of silently migrating")
	}
	if _, err := inst.SubmitChoice(v2, "pay", "o2"); err == nil {
		t.Fatal("submit against v2 must be rejected")
	}
	if _, err := inst.SubmitChoice(v1, "pay", "o2"); err != nil {
		t.Fatalf("continue under pinned v1: %v", err)
	}
	if inst.Current != "city" {
		t.Fatalf("current=%s", inst.Current)
	}
	if got := inst.Facts["gold"].(float64); got != 5 {
		t.Fatalf("gold=%v, want 5", got)
	}
}
