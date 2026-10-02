package story

import (
	"errors"
	"testing"
)

func TestBlockedMigrationRequiresExplicitTarget(t *testing.T) {
	old := testStory()
	catalog := NewVersionedCatalog(old)
	inst, _, err := catalog.Start("test", "blocked", map[string]interface{}{"gold": float64(20)})
	if err != nil {
		t.Fatal(err)
	}
	inst.Current = "vault"

	updated := testStory()
	gate := updated.Nodes["gate"]
	gate.Choices[0].Target = "end"
	updated.Nodes["gate"] = gate
	delete(updated.Nodes, "vault")
	if err := catalog.Add(updated); err != nil {
		t.Fatal(err)
	}
	review, err := catalog.ReviewMigration(inst)
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "blocked" || len(review.MissingNodes) != 1 {
		t.Fatalf("missing current node must block: %+v", review)
	}
	if err := catalog.AdoptLatest(inst, ""); !errors.Is(err, ErrMigrationRequiresChoice) {
		t.Fatalf("silent jump forbidden, got %v", err)
	}
	if inst.Current != "vault" || inst.DefHash == review.LatestHash {
		t.Fatal("failed migration must leave save pinned to old version")
	}
	if err := catalog.AdoptLatest(inst, "gate"); err != nil {
		t.Fatal(err)
	}
	if inst.Current != "gate" || inst.Migration == nil || inst.Migration.ExplicitTarget != "gate" {
		t.Fatalf("explicit continuation not recorded: %+v", inst.Migration)
	}
	if len(inst.Rewards) != 0 {
		t.Fatal("migration must not grant rewards")
	}
}
