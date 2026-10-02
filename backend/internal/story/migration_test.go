package story

import (
	"errors"
	"strings"
	"testing"
)

// revisedStory returns a new revision of testStory with a tightened
// condition, a changed reward and a removed choice.
func revisedStory() *Definition {
	d := testStory()
	d.Version = "v2"
	gate := d.Nodes["gate"]
	gate.Choices = []Choice{
		{ID: "pay", Text: "pay", Target: "vault",
			Condition: fact("gold", ">=", float64(20)),
			Effects: []Effect{
				{Op: "inc", Key: "gold", Value: float64(-20)},
				{Op: "flag", Key: "paid"},
				{Op: "item", Key: "key"},
			}},
	}
	d.Nodes["gate"] = gate
	return d
}

func startAtGate(t *testing.T, c *Catalog, id string) *Instance {
	t.Helper()
	// 5 gold + 8 from work = 13: enough for the old pay condition (>= 10)
	// but not for the revised one (>= 20).
	inst, _, err := c.Start("test", id, map[string]interface{}{"gold": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(inst, "work", ""); err != nil {
		t.Fatal(err)
	}
	return inst
}

func TestMigrationPreviewClassifiesEventsAndChoices(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "m1")

	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	impact, err := c.PreviewMigration(inst)
	if err != nil {
		t.Fatal(err)
	}
	if impact.UpToDate {
		t.Fatal("expected drift to be reported")
	}
	if impact.FromVersion != "v1" || impact.ToVersion != "v2" {
		t.Fatalf("wrong versions: %+v", impact)
	}
	// The completed "work" event is untouched by the revision.
	if len(impact.Completed) != 1 || impact.Completed[0].Status != EventUnchanged {
		t.Fatalf("completed events wrong: %+v", impact.Completed)
	}
	// At the gate: pay changed condition+reward, back was removed.
	changes := map[string]string{}
	for _, ci := range impact.Future {
		changes[ci.ChoiceID] = ci.Change
	}
	if changes["pay"] != ChoiceConditionChanged {
		t.Fatalf("pay should be condition-changed, got %v", changes["pay"])
	}
	if changes["back"] != ChoiceRemoved {
		t.Fatalf("back should be removed, got %v", changes["back"])
	}
}

func TestMigrationAdoptRepinsAndIsIdempotent(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "m2")
	rewardsBefore := len(inst.Rewards)
	claimedBefore := len(inst.Claimed)
	logBefore := len(inst.Log)

	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdoptMigration(inst); err != nil {
		t.Fatal(err)
	}
	if inst.Version != "v2" {
		t.Fatalf("expected re-pin to v2, got %s", inst.Version)
	}
	// Already-granted rewards and claims are untouched.
	if len(inst.Rewards) != rewardsBefore || len(inst.Claimed) != claimedBefore {
		t.Fatal("adoption must not re-grant or rewrite claimed rewards")
	}
	// The boundary is explained in the log exactly once.
	if len(inst.Log) != logBefore+1 || inst.Log[len(inst.Log)-1].Note == "" {
		t.Fatalf("migration boundary note missing: %+v", inst.Log)
	}
	if _, err := c.AdoptMigration(inst); err != nil {
		t.Fatal(err)
	}
	if len(inst.Log) != logBefore+1 {
		t.Fatal("repeated adoption must be a no-op")
	}

	// Under the new rules the tightened condition is enforced.
	if _, err := c.Apply(inst, "pay", ""); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("new rules must apply after adoption, got %v", err)
	}
}

func TestMigrationBlockedWhenCurrentNodeRemoved(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "m3")

	revised := revisedStory()
	delete(revised.Nodes, "gate")
	if err := c.Update(revised); err != nil {
		t.Fatal(err)
	}
	impact, err := c.PreviewMigration(inst)
	if err != nil {
		t.Fatal(err)
	}
	if !impact.CurrentNodeMissing {
		t.Fatal("missing current node must be flagged")
	}
	if _, err := c.AdoptMigration(inst); !errors.Is(err, ErrMigrationBlocked) {
		t.Fatalf("adoption must be blocked, got %v", err)
	}
	// No silent jump: position and pin are unchanged.
	if inst.Current != "gate" || inst.Version != "v1" {
		t.Fatalf("progress silently moved: current=%s version=%s", inst.Current, inst.Version)
	}
}

func TestDeferMigrationKeepsEventsAndOldRules(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "m4")
	logBefore := len(inst.Log)

	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	if err := c.DeferMigration(inst); err != nil {
		t.Fatal(err)
	}
	if inst.MigrationDeferred == "" {
		t.Fatal("defer marker missing")
	}
	if len(inst.Log) != logBefore {
		t.Fatal("defer must not alter happened events")
	}
	// The save keeps playing under its pinned old rules.
	if _, err := c.Apply(inst, "pay", ""); err != nil {
		t.Fatalf("old rules must still apply after defer: %v", err)
	}
	if inst.Facts["paid"] != true {
		t.Fatal("old-rule choice must still grant its rewards")
	}
}

func TestSiblingsMigrateIndependently(t *testing.T) {
	c := NewCatalog(testStory())
	root := startAtGate(t, c, "root")
	branch := Fork(root, "root-b1", "试演")

	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdoptMigration(branch); err != nil {
		t.Fatal(err)
	}
	// The sibling that did not adopt still uses the old condition.
	if _, err := c.Apply(root, "pay", ""); err != nil {
		t.Fatalf("unmigrated branch must keep old rules: %v", err)
	}
	// The adopted branch enforces the new condition.
	if _, err := c.Apply(branch, "pay", ""); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("migrated branch must use new rules, got %v", err)
	}
	if !strings.Contains(branch.Log[len(branch.Log)-1].Note, "规则迁移") {
		t.Fatal("migration boundary note missing on adopted branch")
	}
}
