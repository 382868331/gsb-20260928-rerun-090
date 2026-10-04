package story

import (
	"errors"
	"strings"
	"testing"
)

// TestAdoptionPersistsAcrossCatalogRestart proves the adopted revision
// definition is saved together with the decision: a brand-new catalog
// backed by the same revision store still resolves the adopted save.
func TestAdoptionPersistsAcrossCatalogRestart(t *testing.T) {
	revs := NewMemoryRevisionStore()
	c := NewCatalogWithStore(revs, testStory())
	root := startAtGate(t, c, "persist-root")
	branch := Fork(root, "persist-branch", "分支")

	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	// The branch adopts v2; the root defers and keeps v1.
	if _, err := c.AdoptMigration(branch); err != nil {
		t.Fatal(err)
	}
	if err := c.DeferMigration(root); err != nil {
		t.Fatal(err)
	}
	if len(branch.MigrationHistory) != 1 || branch.MigrationHistory[0].ToVersion != "v2" {
		t.Fatalf("adoption history not recorded: %+v", branch.MigrationHistory)
	}
	if branch.PendingMigration != nil {
		t.Fatal("completed adoption must clear pending marker")
	}

	// Simulate a process restart: new in-memory catalog, same snapshots.
	restarted := NewCatalogWithStore(revs)
	if _, err := restarted.View(branch); err != nil {
		t.Fatalf("adopted branch must resolve after restart: %v", err)
	}
	if _, err := restarted.View(root); err != nil {
		t.Fatalf("deferred branch must resolve after restart: %v", err)
	}
	// The adopted branch enforces v2 (tightened condition); the other keeps v1.
	if _, err := restarted.Apply(branch, "pay", ""); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("adopted branch must use v2 after restart, got %v", err)
	}
	if _, err := restarted.Apply(root, "pay", "k1"); err != nil {
		t.Fatalf("deferred branch must keep v1 after restart: %v", err)
	}

	// Withdrawal: v1 and v2 are both referenced and must be retained.
	refs := &RevisionUsage{PinnedBy: []string{"persist-root"}, HistoryOf: []string{"persist-branch"}}
	v1Hash := root.DefHash
	v2Hash := branch.DefHash
	if err := restarted.WithdrawRevision("test", v1Hash, refs); !errors.Is(err, ErrRevisionInUse) {
		t.Fatalf("referenced v1 must be retained, got %v", err)
	}
	if err := restarted.WithdrawRevision("test", v2Hash, &RevisionUsage{
		PinnedBy: []string{"persist-branch"},
	}); !errors.Is(err, ErrRevisionInUse) {
		t.Fatalf("referenced v2 must be retained, got %v", err)
	}
	// The current latest revision cannot be withdrawn even with no refs.
	if err := restarted.WithdrawRevision("test", v2Hash, &RevisionUsage{}); !errors.Is(err, ErrRevisionInUse) {
		t.Fatalf("current revision cannot be withdrawn, got %v", err)
	}
}

// TestInterruptedAdoptionResumesExactlyOnce covers the begin/complete
// sequence: after begin the save still plays old rules and is marked
// pending; after restart the pending decision is found; repeating the
// completion neither re-appends the boundary log nor replays rewards.
func TestInterruptedAdoptionResumesExactlyOnce(t *testing.T) {
	revs := NewMemoryRevisionStore()
	c := NewCatalogWithStore(revs, testStory())
	inst := startAtGate(t, c, "pending-1")
	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}

	impact, err := c.BeginAdoption(inst)
	if err != nil {
		t.Fatal(err)
	}
	if !impact.Pending || inst.PendingMigration == nil {
		t.Fatal("begin must record a durable pending decision")
	}
	// After begin the save is still on the old rules and position.
	if inst.DefHash != impact.FromHash || inst.Version != "v1" {
		t.Fatalf("begin must not re-pin: %s %s", inst.DefHash, inst.Version)
	}
	if _, err := c.Apply(inst, "pay", "before-complete"); err != nil {
		t.Fatalf("pending save keeps playing under old rules: %v", err)
	}
	if inst.Facts["paid"] != true {
		t.Fatal("old-rule play during pending state must grant normally")
	}
	rewardsBefore := len(inst.Rewards)

	// Restart with the decision persisted on the instance.
	c2 := NewCatalogWithStore(revs)
	pending := inst.PendingMigration
	if pending.ToVersion != "v2" || pending.FromVersion != "v1" {
		t.Fatalf("pending decision lost: %+v", pending)
	}
	_, applied, err := c2.CompleteAdoption(inst)
	if err != nil {
		t.Fatal(err)
	}
	if !applied || inst.Version != "v2" {
		t.Fatalf("completion must re-pin once: applied=%v version=%s", applied, inst.Version)
	}
	if len(inst.Rewards) != rewardsBefore {
		t.Fatal("completion must not replay rewards")
	}
	if len(inst.MigrationHistory) != 1 || inst.PendingMigration != nil {
		t.Fatal("completion records history and clears pending")
	}
	logLen := len(inst.Log)

	// The resume request is duplicated (client retried after timeout).
	impact3, appliedAgain, err := c2.CompleteAdoption(inst)
	if err != nil {
		t.Fatal(err)
	}
	if appliedAgain {
		t.Fatal("duplicate resume must not apply again")
	}
	if !impact3.AlreadyAdopted {
		t.Fatal("duplicate resume must be reported as already adopted")
	}
	if len(inst.Log) != logLen {
		t.Fatal("duplicate resume must not append another boundary note")
	}
	if len(inst.MigrationHistory) != 1 {
		t.Fatal("duplicate resume must not add history")
	}

	// Cancelling a pending adoption leaves old rules and events untouched.
	cV1 := NewCatalog(testStory())
	other := startAtGate(t, cV1, "pending-2")
	if err := cV1.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BeginAdoption(other); err != nil {
		t.Fatal(err)
	}
	if err := c.CancelAdoption(other); err != nil {
		t.Fatal(err)
	}
	if other.PendingMigration != nil || other.Version != "v1" {
		t.Fatal("cancel must clear pending and keep old rules")
	}
}

// TestUnreferencedRevisionWithdraws verifies a revision that no save uses
// can be withdrawn, after which old saves pinning elsewhere keep working.
func TestUnreferencedRevisionWithdraws(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "w1")
	v1 := inst.DefHash
	r2 := revisedStory()
	r2.Version = "v2"
	if err := c.Update(r2); err != nil {
		t.Fatal(err)
	}
	r3 := revisedStory()
	r3.Version = "v3"
	g := r3.Nodes["gate"]
	g.Text = "stricter gate"
	r3.Nodes["gate"] = g
	if err := c.Update(r3); err != nil {
		t.Fatal(err)
	}
	v2, ok := LatestHashOf(c, "test", "v2")
	if !ok {
		t.Fatal("v2 snapshot missing")
	}
	// Nothing references v2, and v3 is current: v2 withdraws.
	if err := c.WithdrawRevision("test", v2, &RevisionUsage{}); err != nil {
		t.Fatalf("unreferenced non-current revision should withdraw: %v", err)
	}
	if _, err := c.DefinitionForHash(v2); !errors.Is(err, ErrUnknownStory) {
		t.Fatalf("v2 should be gone, got %v", err)
	}
	// The v1 save and the current v3 revision still resolve.
	if _, err := c.View(inst); err != nil {
		t.Fatalf("v1 save must keep working: %v", err)
	}
	// No save adopted v2 or v3; v1 is pinned so it stays.
	if err := c.WithdrawRevision("test", v1, &RevisionUsage{PinnedBy: []string{"w1"}}); !errors.Is(err, ErrRevisionInUse) {
		t.Fatalf("pinned revision retained, got %v", err)
	}
}

// LatestHashOf is a test helper resolving a revision hash by version label.
func LatestHashOf(c *Catalog, storyID, version string) (string, bool) {
	for _, rev := range c.RevisionSnapshots(storyID) {
		if rev.Version == version {
			return rev.Hash, true
		}
	}
	return "", false
}

func TestWithdrawErrorMessageMentionsReferences(t *testing.T) {
	c := NewCatalog(testStory())
	inst := startAtGate(t, c, "w2")
	if err := c.Update(revisedStory()); err != nil {
		t.Fatal(err)
	}
	err := c.WithdrawRevision("test", inst.DefHash, &RevisionUsage{PinnedBy: []string{"w2"}})
	if !errors.Is(err, ErrRevisionInUse) || !strings.Contains(err.Error(), "w2") {
		t.Fatalf("error must explain retention reasons, got %v", err)
	}
}
