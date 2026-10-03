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
		return impact, nil
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

// BeginAdoption records this branch's decision to adopt the story's latest
// revision. The decision is persisted (by the caller) before CompleteAdoption
// runs, so if the process stops in between the save reloads in the
// "adopting" state and can be recovered instead of silently losing which
// rules it had chosen.
func (c *Catalog) BeginAdoption(inst *Instance) (*MigrationImpact, error) {
	impact, err := c.PreviewMigration(inst)
	if err != nil {
		return nil, err
	}
	if impact.UpToDate {
		return impact, nil
	}
	if impact.CurrentNodeMissing {
		return impact, fmt.Errorf("%w: current node %s no longer exists in version %s; the save stays on its pinned rules",
			ErrMigrationBlocked, inst.Current, impact.ToVersion)
	}
	inst.Migration = &MigrationState{
		Status:      MigrationAdopting,
		FromHash:    impact.FromHash,
		ToHash:      impact.ToHash,
		FromVersion: impact.FromVersion,
		ToVersion:   impact.ToVersion,
		UpdatedAt:   nowUTC(),
	}
	return impact, nil
}

// CompleteAdoption finishes a pending adoption recorded by BeginAdoption.
// It is idempotent: if the save already sits on the target revision no
// second migration boundary is logged and no reward is touched. Claimed
// rewards and facts are never rewritten, so already-granted rewards never
// re-fire.
func (c *Catalog) CompleteAdoption(inst *Instance) error {
	m := inst.Migration
	if m == nil {
		return fmt.Errorf("%w: instance %s", ErrNoPendingRecovery, inst.ID)
	}
	if m.Status == MigrationAdopted {
		// Already completed: repeating recovery must not add a second
		// migration boundary or touch rewards.
		return nil
	}
	if m.Status != MigrationAdopting {
		return fmt.Errorf("%w: instance %s", ErrNoPendingRecovery, inst.ID)
	}
	target, ok := c.revisionByHash(m.ToHash)
	if !ok {
		return fmt.Errorf("%w: adopted revision %s is no longer published; the save stays on its pinned rules",
			ErrStoryVersionDrift, m.ToHash)
	}
	now := nowUTC()
	if inst.DefHash != m.ToHash {
		inst.Log = append(inst.Log, LogEntry{
			Node: inst.Current,
			Note: fmt.Sprintf("规则迁移：%s → %s。此前事件按旧规则保持不变，此后选择按新规则求值；已领取的奖励不重新发放。",
				m.FromVersion, m.ToVersion),
			At: now,
		})
		inst.DefHash = m.ToHash
		inst.Version = target.Version
	}
	m.Status = MigrationAdopted
	m.UpdatedAt = now
	inst.MigrationDeferred = ""
	inst.UpdatedAt = now
	return nil
}

// AbortAdoption cancels an interrupted adoption: the branch stays on its
// pinned rules and the pending record is cleared. Happened events and
// rewards are untouched.
func (c *Catalog) AbortAdoption(inst *Instance) error {
	m := inst.Migration
	if m == nil || m.Status != MigrationAdopting {
		return fmt.Errorf("%w: instance %s", ErrNoPendingRecovery, inst.ID)
	}
	inst.Migration = nil
	inst.UpdatedAt = nowUTC()
	return nil
}

// NeedsRecovery reports whether this save has an interrupted adoption that
// must be reviewed and either completed or aborted.
func NeedsRecovery(inst *Instance) bool {
	return inst.Migration != nil && inst.Migration.Status == MigrationAdopting
}

func (c *Catalog) revisionByHash(hash string) (*Definition, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.history[hash]
	return d, ok
}

// AdoptMigration re-pins the instance to the story's latest definition in a
// single step. It is idempotent: adopting when already current changes
// nothing. Adoption refuses to strand progress — if the current node no
// longer exists the save keeps its old rules instead of silently jumping to
// a default position.
func (c *Catalog) AdoptMigration(inst *Instance) (*MigrationImpact, error) {
	if NeedsRecovery(inst) {
		if err := c.CompleteAdoption(inst); err != nil {
			return nil, err
		}
		return c.PreviewMigration(inst)
	}
	impact, err := c.BeginAdoption(inst)
	if err != nil {
		return impact, err
	}
	if impact.UpToDate {
		return impact, nil
	}
	if err := c.CompleteAdoption(inst); err != nil {
		return impact, err
	}
	return impact, nil
}

// DeferMigration records that the author of this save declined the latest
// revision for now. It never alters happened events and is idempotent.
func (c *Catalog) DeferMigration(inst *Instance) error {
	hash, err := c.LatestHash(inst.StoryID)
	if err != nil {
		return err
	}
	inst.MigrationDeferred = hash
	inst.UpdatedAt = nowUTC()
	return nil
}
