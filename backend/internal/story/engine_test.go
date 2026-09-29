package story

import (
	"errors"
	"testing"
)

func choiceIDs(v *View) []string {
	ids := make([]string, 0, len(v.Choices))
	for _, c := range v.Choices {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestMutualExclusion_RichAndPoorPath(t *testing.T) {
	d := SeedStory()
	inst, v, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}

	// Rich branch: forest gives 5 gold -> at the gate only the >=5 choice shows.
	v, err = inst.Choose(d, "go_forest")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.State["gold"]; got != float64(5) {
		t.Fatalf("gold = %v, want 5", got)
	}
	v, err = inst.Choose(d, "to_gate")
	if err != nil {
		t.Fatal(err)
	}
	if ids := choiceIDs(v); len(ids) != 1 || ids[0] != "bribe_rich" {
		t.Fatalf("rich gate choices = %v, want [bribe_rich]", ids)
	}
	if _, err := inst.Choose(d, "bribe_poor"); err == nil {
		t.Fatal("runner accepted a choice the UI did not offer")
	}
	v, err = inst.Choose(d, "bribe_rich")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Terminal || v.NodeID != "city" {
		t.Fatalf("expected terminal city, got %+v", v)
	}
}

func TestMutualExclusion_PoorPath(t *testing.T) {
	d := SeedStory()
	inst, _, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Choose(d, "go_market"); err != nil {
		t.Fatal(err)
	}
	v, err := inst.Choose(d, "to_gate")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.State["gold"]; got != float64(3) {
		t.Fatalf("gold = %v, want 3", got)
	}
	if ids := choiceIDs(v); len(ids) != 1 || ids[0] != "bribe_poor" {
		t.Fatalf("poor gate choices = %v, want [bribe_poor]", ids)
	}
	if _, err := inst.Choose(d, "bribe_rich"); err == nil {
		t.Fatal("runner accepted disallowed rich choice")
	}
}

func TestConvergence_BothBranchesReachSameGate(t *testing.T) {
	d := SeedStory()
	reach := func(firstChoice string) string {
		inst, _, err := Start(d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inst.Choose(d, firstChoice); err != nil {
			t.Fatal(err)
		}
		v, err := inst.Choose(d, "to_gate")
		if err != nil {
			t.Fatal(err)
		}
		return v.NodeID
	}
	if got := reach("go_market"); got != "gate" {
		t.Fatalf("market branch converged at %q", got)
	}
	if got := reach("go_forest"); got != "gate" {
		t.Fatalf("forest branch converged at %q", got)
	}
}

func TestRevisitDoesNotReissueReward(t *testing.T) {
	d := SeedStory()
	inst, _, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	// campfire(0) -> market(+3 once) -> gate -> beggar(-1) -> market(no reissue).
	if _, err := inst.Choose(d, "go_market"); err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Choose(d, "to_gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Choose(d, "bribe_poor"); err != nil {
		t.Fatal(err)
	}
	if got := inst.State["gold"]; got != float64(2) {
		t.Fatalf("gold after beggar = %v, want 2", got)
	}
	v, err := inst.Choose(d, "to_market")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.State["gold"]; got != float64(2) {
		t.Fatalf("revisiting market reissued reward: gold = %v, want 2", got)
	}
	if v.Visits != 2 {
		t.Fatalf("market visits = %d, want 2", v.Visits)
	}
	// grant effect is also idempotent: still one item key.
	items, _ := v.State["items"].(map[string]any)
	if len(items) != 1 || items["coin_purse"] != true {
		t.Fatalf("items = %v", items)
	}
}

func TestMidStorySaveAndResume(t *testing.T) {
	d := SeedStory()
	inst, _, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Choose(d, "go_forest"); err != nil {
		t.Fatal(err)
	}
	saved := cloneJSON(inst)
	restored, err := Resume(d, saved)
	if err != nil {
		t.Fatal(err)
	}
	if restored.NodeID != "forest" || restored.State["gold"] != float64(5) {
		t.Fatalf("resume lost progress: %+v", restored)
	}
}

func TestResumeRejectsVersionMismatch(t *testing.T) {
	d := SeedStory()
	inst, _, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	newer := cloneJSON(d)
	newer.Version = 2
	_, err = Resume(newer, inst)
	var vm *VersionMismatchError
	if !errors.As(err, &vm) {
		t.Fatalf("want VersionMismatchError, got %v", err)
	}
}

func TestResumeRejectsDeletedNode(t *testing.T) {
	d := SeedStory()
	inst, _, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Choose(d, "go_forest"); err != nil {
		t.Fatal(err)
	}
	// A v2 definition that deleted the save's node is rejected first by the
	// version pin. Independently, a same-version save pointing at a missing
	// node must be reported, never silently redirected.
	updated := cloneJSON(d)
	updated.Version = 2
	delete(updated.Nodes, "forest")
	updated.Start = "campfire"
	camp := updated.Nodes["campfire"]
	camp.Choices = camp.Choices[:1]
	updated.Nodes["campfire"] = camp
	if err := ValidateDefinition(updated); err != nil {
		t.Fatal(err)
	}
	if _, err := Resume(updated, inst); err == nil {
		t.Fatal("deleted-node save must not resume against v2")
	}
	broken := cloneJSON(inst)
	broken.Current = "ghost"
	_, err = Resume(d, broken)
	var mn *MissingNodeError
	if !errors.As(err, &mn) {
		t.Fatalf("want MissingNodeError, got %v", err)
	}
}

func TestValidationRejectsOverlappingConditions(t *testing.T) {
	d := SeedStory()
	gate := d.Nodes["gate"]
	gate.Choices[0].When = &Condition{Key: "gold", Op: OpGe, Val: float64(3)}
	gate.Choices[1].When = &Condition{Key: "gold", Op: OpLt, Val: float64(5)}
	d.Nodes["gate"] = gate
	if err := ValidateDefinition(d); err == nil {
		t.Fatal("overlapping >=3 and <5 must be rejected")
	}
}

func TestValidationRejectsDifferentKeyPairs(t *testing.T) {
	d := SeedStory()
	gate := d.Nodes["gate"]
	gate.Choices[0].When = &Condition{Key: "gold", Op: OpGe, Val: float64(5)}
	gate.Choices[1].When = &Condition{Key: "fame", Op: OpGe, Val: float64(1)}
	d.Nodes["gate"] = gate
	if err := ValidateDefinition(d); err == nil {
		t.Fatal("conditions on different keys cannot be proven disjoint")
	}
}

func TestUnconditionalChoicesAlwaysAvailable(t *testing.T) {
	d := &Definition{
		ID: "d", Version: 1, Start: "a",
		Nodes: map[string]Node{
			"a": {ID: "a", Choices: []Choice{
				{ID: "yes", Text: "yes", Next: "b", When: &Condition{Key: "flag", Op: OpEq, Val: true}},
				{ID: "no", Text: "no", Next: "c"},
			}},
			"b": {ID: "b", Terminal: true},
			"c": {ID: "c", Terminal: true},
		},
	}
	if err := ValidateDefinition(d); err != nil {
		t.Fatal(err)
	}
	inst, v, err := Start(d)
	if err != nil {
		t.Fatal(err)
	}
	// flag unset -> only the unconditional choice shows.
	if ids := choiceIDs(v); len(ids) != 1 || ids[0] != "no" {
		t.Fatalf("unconditional choice hidden: %v", ids)
	}
	inst.State["flag"] = true
	v, err = inst.view(d)
	if err != nil {
		t.Fatal(err)
	}
	if ids := choiceIDs(v); len(ids) != 2 || ids[0] != "no" || ids[1] != "yes" {
		t.Fatalf("expected both choices, got %v", ids)
	}
}
