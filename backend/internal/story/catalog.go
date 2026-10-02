package story

import (
	"encoding/json"
	"fmt"
	"sort"
)

// VersionedCatalog keeps every definition observed at runtime. Old saves are
// therefore replayable after an author publishes a revision; migration to the
// newest version is always an explicit per-instance decision.
type VersionedCatalog struct {
	byID   map[string]map[string]*Definition
	latest map[string]*Definition
}

func NewVersionedCatalog(defs ...*Definition) *VersionedCatalog {
	c := &VersionedCatalog{
		byID:   map[string]map[string]*Definition{},
		latest: map[string]*Definition{},
	}
	for _, d := range defs {
		c.Add(d)
	}
	return c
}

func (c *VersionedCatalog) Add(def *Definition) error {
	if def == nil {
		return fmt.Errorf("nil definition")
	}
	if err := ValidateDefinition(def); err != nil {
		return err
	}
	hash, err := DefHash(def)
	if err != nil {
		return err
	}
	if c.byID[def.ID] == nil {
		c.byID[def.ID] = map[string]*Definition{}
	}
	clone := cloneDefinition(def)
	c.byID[def.ID][hash] = clone
	c.latest[def.ID] = clone
	return nil
}

func (c *VersionedCatalog) Get(id string) (*Definition, error) {
	d, ok := c.latest[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStory, id)
	}
	return d, nil
}

func (c *VersionedCatalog) GetVersion(id, hash string) (*Definition, error) {
	versions, ok := c.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStory, id)
	}
	d, ok := versions[hash]
	if !ok {
		return nil, fmt.Errorf("%w: %s version %.12s", ErrStoryVersionDrift, id, hash)
	}
	return d, nil
}

func (c *VersionedCatalog) LatestHash(id string) (string, error) {
	d, err := c.Get(id)
	if err != nil {
		return "", err
	}
	return DefHash(d)
}

func (c *VersionedCatalog) List() []*Definition {
	out := make([]*Definition, 0, len(c.latest))
	for _, d := range c.latest {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *VersionedCatalog) Start(storyID, instanceID string, initial map[string]interface{}) (*Instance, *Definition, error) {
	d, err := c.Get(storyID)
	if err != nil {
		return nil, nil, err
	}
	return startWithDefinition(d, storyID, instanceID, initial)
}

func (c *VersionedCatalog) View(inst *Instance) (NodeView, error) {
	d, err := c.GetVersion(inst.StoryID, inst.DefHash)
	if err != nil {
		return NodeView{}, err
	}
	return viewFor(d, inst)
}

func (c *VersionedCatalog) Apply(inst *Instance, choiceID, idemKey string) (*AppliedChoice, error) {
	d, err := c.GetVersion(inst.StoryID, inst.DefHash)
	if err != nil {
		return nil, err
	}
	return applyWithDefinition(d, inst, choiceID, idemKey)
}

func (c *VersionedCatalog) ReviewMigration(inst *Instance) (*MigrationReview, error) {
	old, err := c.GetVersion(inst.StoryID, inst.DefHash)
	if err != nil {
		return nil, err
	}
	new, err := c.Get(inst.StoryID)
	if err != nil {
		return nil, err
	}
	newHash, _ := DefHash(new)
	review := &MigrationReview{
		InstanceID:     inst.ID,
		Label:          instanceLabel(inst),
		CurrentVersion: inst.Version,
		CurrentHash:    inst.DefHash,
		LatestVersion:  new.Version,
		LatestHash:     newHash,
		BoundaryNode:   inst.Current,
		RewardNotice:   "已领取的奖励和已完成事件保持原样，采用新规则不会重新发放。",
	}
	for i, entry := range inst.Log {
		summary := nodeTitle(old, entry.Node)
		if entry.ChoiceID != "" {
			summary = choiceTitle(old, previousNodeAt(inst.Log, i), entry.ChoiceID)
		}
		review.CompletedEvents = append(review.CompletedEvents, ReviewedEvent{
			Node: entry.Node, ChoiceID: entry.ChoiceID, Summary: summary, Status: "unchanged",
		})
	}
	if newHash == inst.DefHash {
		review.Status = "current"
		return review, nil
	}
	if inst.Migration != nil &&
		inst.Migration.ToHash == inst.DefHash &&
		newHash == inst.Migration.FromHash {
		review.Status = "current"
		return review, nil
	}
	review.Status = "upgradable"
	review.FutureChanges = CompareRules(old, new, inst)
	if _, ok := new.Nodes[inst.Current]; !ok {
		review.Status = "blocked"
		review.MissingNodes = append(review.MissingNodes, inst.Current)
		review.AvailableTargets = ReachableStarts(new)
	}
	return review, nil
}

func (c *VersionedCatalog) AdoptLatest(inst *Instance, explicitTarget string) error {
	review, err := c.ReviewMigration(inst)
	if err != nil {
		return err
	}
	if review.Status == "current" {
		return nil
	}
	new, err := c.Get(inst.StoryID)
	if err != nil {
		return err
	}
	target := inst.Current
	note := "从当前节点开始采用新规则。"
	if review.Status == "blocked" {
		if explicitTarget == "" {
			return fmt.Errorf("%w: current node %s was removed; choose an explicit continuation node", ErrMigrationRequiresChoice, inst.Current)
		}
		if _, ok := new.Nodes[explicitTarget]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownNode, explicitTarget)
		}
		target = explicitTarget
		note = "原当前节点已移除，按作者确认的接续节点继续，未静默跳转。"
	}
	oldHash, oldVersion := inst.DefHash, inst.Version
	inst.DefHash = review.LatestHash
	inst.Version = review.LatestVersion
	inst.Current = target
	inst.VisitID = newVisitID()
	inst.Migration = &MigrationRecord{
		FromHash: oldHash, ToHash: review.LatestHash,
		FromVersion: oldVersion, ToVersion: review.LatestVersion,
		BoundaryNode: review.BoundaryNode, ContinuedAt: nowUTC(),
		ExplicitTarget: explicitTarget, Note: note,
	}
	inst.Log = append(inst.Log, LogEntry{Node: target, At: inst.Migration.ContinuedAt, Migration: &MigrationSummary{
		FromVersion: oldVersion, ToVersion: review.LatestVersion, BoundaryNode: review.BoundaryNode,
		ExplicitTarget: explicitTarget, Note: note,
	}})
	inst.UpdatedAt = nowUTC()
	return nil
}

func cloneDefinition(in *Definition) *Definition {
	b, _ := json.Marshal(in)
	var out Definition
	_ = json.Unmarshal(b, &out)
	return &out
}

func instanceLabel(inst *Instance) string {
	if inst.Label != "" {
		return inst.Label
	}
	return inst.ID
}

func nodeTitle(d *Definition, id string) string {
	if n, ok := d.Nodes[id]; ok && n.Text != "" {
		return id + "：" + n.Text
	}
	return id
}

func choiceTitle(d *Definition, nodeID, choiceID string) string {
	if nodeID == "" {
		return choiceID
	}
	n, ok := d.Nodes[nodeID]
	if !ok {
		return choiceID
	}
	for _, ch := range n.Choices {
		if ch.ID == choiceID {
			return nodeID + " / " + ch.Text
		}
	}
	return choiceID
}

func previousNodeAt(log []LogEntry, index int) string {
	if index > 0 && index < len(log) {
		return log[index-1].Node
	}
	return ""
}
