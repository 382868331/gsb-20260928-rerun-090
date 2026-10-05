package story

import (
	"encoding/json"
	"fmt"
)

// Change classifications used in migration impact reports.
const (
	EventUnchanged = "unchanged" // new rules describe this past event identically
	EventRevised   = "revised"   // new rules changed this choice; the happened event stays as-is
	EventRemoved   = "removed"   // new rules removed the node/choice; the happened event stays as-is

	ChoiceAdded            = "added"
	ChoiceRemoved          = "removed"
	ChoiceConditionChanged = "condition-changed"
	ChoiceRewardChanged    = "reward-changed"
	ChoiceTargetChanged    = "target-changed"
	ChoiceUnchanged        = "unchanged"
)

// EventImpact describes how one already-played log entry relates to the new
// rules. Completed events are never rewritten; the status only explains the
// boundary between old and new rules.
type EventImpact struct {
	Node     string `json:"node"`
	ChoiceID string `json:"choiceId,omitempty"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

// ChoiceImpact describes one difference in the choices offered at the
// instance's current node between the pinned and the latest definition.
type ChoiceImpact struct {
	ChoiceID string `json:"choiceId"`
	Text     string `json:"text"`
	Change   string `json:"change"`
}

// MigrationImpact is the review an author-facing client shows before a save
// adopts a revised story definition.
type MigrationImpact struct {
	StoryID            string         `json:"storyId"`
	FromVersion        string         `json:"fromVersion"`
	ToVersion          string         `json:"toVersion"`
	FromHash           string         `json:"fromHash"`
	ToHash             string         `json:"toHash"`
	UpToDate           bool           `json:"upToDate"`
	CurrentNode        string         `json:"currentNode"`
	CurrentNodeMissing bool           `json:"currentNodeMissing"`
	Completed          []EventImpact  `json:"completed"`
	Future             []ChoiceImpact `json:"future"`
	// Pending is true when a durable "adoption in progress" decision was
	// saved but never completed (e.g. the process exited mid-adoption). The
	// save is still playing under FromHash rules until completion resumes.
	Pending bool `json:"pending,omitempty"`
	// AlreadyAdopted is true when a repeated completion arrives after the
	// adoption already finished; no rewards or story position change again.
	AlreadyAdopted bool `json:"alreadyAdopted,omitempty"`
}

func jsonEqual(a, b interface{}) bool {
	ba, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ba) == string(bb)
}

func findChoice(d *Definition, nodeID, choiceID string) *Choice {
	n, ok := d.Nodes[nodeID]
	if !ok {
		return nil
	}
	for i := range n.Choices {
		if n.Choices[i].ID == choiceID {
			return &n.Choices[i]
		}
	}
	return nil
}

// choiceDiff classifies the difference between two revisions of a choice.
func choiceDiff(oldCh, newCh *Choice) string {
	if !jsonEqual(oldCh.Condition, newCh.Condition) {
		return ChoiceConditionChanged
	}
	if !jsonEqual(oldCh.Effects, newCh.Effects) {
		return ChoiceRewardChanged
	}
	if oldCh.Target != newCh.Target {
		return ChoiceTargetChanged
	}
	return ChoiceUnchanged
}

// PreviewMigration compares the instance's pinned definition with the
// story's latest revision without changing anything.
func (c *Catalog) PreviewMigration(inst *Instance) (*MigrationImpact, error) {
	oldDef, err := c.pinnedDef(inst)
	if err != nil {
		return nil, err
	}
	latest, err := c.Get(inst.StoryID)
	if err != nil {
		return nil, err
	}
	toHash, err := DefHash(latest)
	if err != nil {
		return nil, err
	}
	impact := &MigrationImpact{
		StoryID:     inst.StoryID,
		FromVersion: oldDef.Version,
		ToVersion:   latest.Version,
		FromHash:    inst.DefHash,
		ToHash:      toHash,
		CurrentNode: inst.Current,
		Completed:   []EventImpact{},
		Future:      []ChoiceImpact{},
	}
	if toHash == inst.DefHash {
		impact.UpToDate = true
		impact.Pending = inst.PendingMigration != nil
		return impact, nil
	}
	if inst.PendingMigration != nil && inst.PendingMigration.ToHash == toHash {
		impact.Pending = true
	}

	// Completed events: replay the persisted log and classify each taken
	// choice against the new rules. Happened events always stay as-is.
	for i, entry := range inst.Log {
		if entry.ChoiceID == "" || i == 0 {
			continue
		}
		prevNode := inst.Log[i-1].Node
		ev := EventImpact{Node: prevNode, ChoiceID: entry.ChoiceID, Status: EventUnchanged}
		if _, ok := latest.Nodes[prevNode]; !ok {
			ev.Status = EventRemoved
			ev.Detail = "新规则已移除节点 " + prevNode + "；已发生事件保持原样"
		} else {
			oldCh := findChoice(oldDef, prevNode, entry.ChoiceID)
			newCh := findChoice(latest, prevNode, entry.ChoiceID)
			switch {
			case newCh == nil:
				ev.Status = EventRemoved
				ev.Detail = "新规则已移除该选项；已发生事件保持原样"
			case oldCh != nil:
				if diff := choiceDiff(oldCh, newCh); diff != ChoiceUnchanged {
					ev.Status = EventRevised
					ev.Detail = "新规则修订了该选项（" + diff + "）；已发生事件保持原样"
				}
			}
		}
		impact.Completed = append(impact.Completed, ev)
	}

	// Future choices at the current node.
	if _, ok := latest.Nodes[inst.Current]; !ok {
		impact.CurrentNodeMissing = true
	} else {
		oldNode := oldDef.Nodes[inst.Current]
		newNode := latest.Nodes[inst.Current]
		seen := map[string]bool{}
		for _, ch := range newNode.Choices {
			seen[ch.ID] = true
			oldCh := findChoice(oldDef, inst.Current, ch.ID)
			if oldCh == nil {
				impact.Future = append(impact.Future, ChoiceImpact{ChoiceID: ch.ID, Text: ch.Text, Change: ChoiceAdded})
				continue
			}
			impact.Future = append(impact.Future, ChoiceImpact{ChoiceID: ch.ID, Text: ch.Text, Change: choiceDiff(oldCh, &ch)})
		}
		for _, ch := range oldNode.Choices {
			if !seen[ch.ID] {
				impact.Future = append(impact.Future, ChoiceImpact{ChoiceID: ch.ID, Text: ch.Text, Change: ChoiceRemoved})
			}
		}
	}
	return impact, nil
}

// BeginAdoption reviews the revision and durably records the decision to
// adopt it without touching the pin, rewards, facts or position. The caller
// must persist the instance afterwards. A crash after begin leaves a
// resumable "waiting for recovery" save that still plays under the old
// rules. Begin refuses revisions that would strand the current node.
func (c *Catalog) BeginAdoption(inst *Instance) (*MigrationImpact, error) {
	impact, err := c.PreviewMigration(inst)
	if err != nil {
		return nil, err
	}
	// A retried begin (client believed the first request failed) must not
	// rewrite the recorded decision or its timestamp. A pending decision for
	// a different revision must be completed or cancelled first.
	if inst.PendingMigration != nil {
		if inst.PendingMigration.ToHash != impact.ToHash {
			return impact, fmt.Errorf("%w: an adoption decision %s -> %s is already waiting for recovery; complete or cancel it before adopting another revision",
				ErrMigrationBlocked, inst.PendingMigration.FromVersion, inst.PendingMigration.ToVersion)
		}
		impact.Pending = true
		return impact, nil
	}
	if impact.UpToDate {
		return impact, nil
	}
	if impact.CurrentNodeMissing {
		return impact, fmt.Errorf("%w: current node %s no longer exists in version %s; the save stays on its pinned rules",
			ErrMigrationBlocked, inst.Current, impact.ToVersion)
	}
	inst.PendingMigration = &PendingAdoption{
		StoryID:     inst.StoryID,
		FromVersion: impact.FromVersion,
		ToVersion:   impact.ToVersion,
		FromHash:    impact.FromHash,
		ToHash:      impact.ToHash,
		At:          nowUTC(),
	}
	inst.MigrationDeferred = ""
	inst.UpdatedAt = nowUTC()
	impact.Pending = true
	return impact, nil
}

// CompleteAdoption finishes a previously begun adoption. It is safe against
// duplicated resume requests: once completed, later calls change nothing
// (no second boundary log entry, no reward replay, no story advance) and
// report AlreadyAdopted. The boolean result is true only when this call
// actually performed the re-pin.
func (c *Catalog) CompleteAdoption(inst *Instance) (*MigrationImpact, bool, error) {
	pending := inst.PendingMigration
	// Idempotent replay: the adoption already completed in an earlier
	// request that the client believed had failed.
	if pending == nil {
		impact, err := c.PreviewMigration(inst)
		if err != nil {
			return nil, false, err
		}
		if impact.UpToDate && hasAdoptedTo(inst, impact.ToHash) {
			impact.AlreadyAdopted = true
		}
		return impact, false, nil
	}
	if inst.DefHash != pending.FromHash {
		// The save moved under different rules while a decision was pending
		// (it should keep old rules, so this indicates a newer completed
		// migration); drop the stale decision instead of forcing the pin.
		if inst.DefHash == pending.ToHash && hasAdoptedTo(inst, pending.ToHash) {
			inst.PendingMigration = nil
			impact, err := c.PreviewMigration(inst)
			if err != nil {
				return nil, false, err
			}
			impact.AlreadyAdopted = true
			return impact, false, nil
		}
		inst.PendingMigration = nil
		return nil, false, fmt.Errorf("%w: save moved from %s while adoption to %s was pending",
			ErrMigrationBlocked, pending.FromHash, pending.ToHash)
	}
	// The target snapshot must still be resolvable; persistence keeps it
	// alongside the decision, so a restarted process can find it.
	toDef, err := c.DefinitionForHash(pending.ToHash)
	if err != nil {
		return nil, false, err
	}
	if _, ok := toDef.Nodes[inst.Current]; !ok {
		return nil, false, fmt.Errorf("%w: current node %s no longer exists in version %s; the save stays on its pinned rules",
			ErrMigrationBlocked, inst.Current, pending.ToVersion)
	}
	now := nowUTC()
	inst.DefHash = pending.ToHash
	inst.Version = pending.ToVersion
	inst.MigrationDeferred = ""
	inst.PendingMigration = nil
	inst.UpdatedAt = now
	inst.MigrationHistory = append(inst.MigrationHistory, MigrationRecord{
		FromVersion: pending.FromVersion,
		ToVersion:   pending.ToVersion,
		FromHash:    pending.FromHash,
		ToHash:      pending.ToHash,
		At:          now,
	})
	inst.Log = append(inst.Log, LogEntry{
		Node: inst.Current,
		Note: fmt.Sprintf("规则迁移：%s → %s。此前事件按旧规则保持不变，此后选择按新规则求值；已领取的奖励不重新发放。",
			pending.FromVersion, pending.ToVersion),
		At: now,
	})
	impact, err := c.PreviewMigration(inst)
	if err != nil {
		return nil, true, err
	}
	return impact, true, nil
}

// CancelAdoption abandons a pending adoption decision. Happened events and
// the pinned rules are untouched.
func (c *Catalog) CancelAdoption(inst *Instance) error {
	if inst.PendingMigration == nil {
		return nil
	}
	inst.PendingMigration = nil
	inst.UpdatedAt = nowUTC()
	return nil
}

func hasAdoptedTo(inst *Instance, hash string) bool {
	for _, rec := range inst.MigrationHistory {
		if rec.ToHash == hash {
			return true
		}
	}
	return false
}

// AdoptMigration performs the full begin + complete sequence in one call for
// callers that do not need a separate resume step. Claimed rewards and facts
// are untouched, so already-granted rewards never re-fire.
func (c *Catalog) AdoptMigration(inst *Instance) (*MigrationImpact, error) {
	impact, err := c.BeginAdoption(inst)
	if err != nil {
		return nil, err
	}
	if impact.UpToDate {
		return impact, nil
	}
	impact, _, err = c.CompleteAdoption(inst)
	return impact, err
}

// DeferMigration records that the author of this save declined the latest
// revision for now. It never alters happened events and is idempotent.
func (c *Catalog) DeferMigration(inst *Instance) error {
	if inst.PendingMigration != nil {
		return fmt.Errorf("%w: an adoption decision %s -> %s waits for recovery; complete or cancel it before declining",
			ErrMigrationBlocked, inst.PendingMigration.FromVersion, inst.PendingMigration.ToVersion)
	}
	hash, err := c.LatestHash(inst.StoryID)
	if err != nil {
		return err
	}
	inst.MigrationDeferred = hash
	inst.UpdatedAt = nowUTC()
	return nil
}
