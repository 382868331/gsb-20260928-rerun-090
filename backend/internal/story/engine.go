package story

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Errors returned by the runtime. Callers can inspect these with errors.Is.
var (
	ErrUnknownStory      = fmt.Errorf("unknown story")
	ErrUnknownInstance   = fmt.Errorf("unknown instance")
	ErrUnknownNode       = fmt.Errorf("unknown current node")
	ErrUnknownChoice     = fmt.Errorf("unknown choice")
	ErrConditionFailed   = fmt.Errorf("choice condition is not satisfied for this story version")
	ErrChoiceNotOnNode   = fmt.Errorf("choice is not attached to the current node")
	ErrStoryVersionDrift = fmt.Errorf("story version no longer matches the instance; pin the definition and retry")
	ErrDuplicateChoice   = fmt.Errorf("choice already applied; duplicate submission grants no reward")
	ErrBadEffect         = fmt.Errorf("invalid effect")
	ErrEndNode           = fmt.Errorf("story has ended")
	ErrMigrationBlocked  = fmt.Errorf("migration blocked: progress would be lost")
	ErrMigrationPending  = fmt.Errorf("migration recovery is pending: complete or cancel it before continuing the story")
)

// claimSep separates visit id and choice in a claim key.
const claimSep = "|"

// Reward is a single grant produced by applying a choice. Rewards are granted
// exactly once per (node, choice) pair and are recorded in the instance.
type Reward struct {
	Node     string      `json:"node"`
	ChoiceID string      `json:"choiceId"`
	Key      string      `json:"key"`
	Op       string      `json:"op"`
	Value    interface{} `json:"value,omitempty"`
	At       string      `json:"at"`
}

// AppliedChoice is the result of a successful choice submission.
type AppliedChoice struct {
	ChoiceID  string   `json:"choiceId"`
	Node      string   `json:"node"`
	Rewards   []Reward `json:"rewards"`
	View      NodeView `json:"view"`
	Duplicate bool     `json:"duplicate"`
}

// Catalog holds story definitions by id. Starting an instance pins the
// definition version (hash) so later catalogue changes cannot invalidate
// an in-progress play-through's conditions. Every definition ever registered
// is kept in the version history, so saves pinned to a superseded revision
// keep playing under their original rules until they explicitly migrate.
type Catalog struct {
	definitions map[string]*Definition
	history     map[string]*Definition // defHash -> definition snapshot
	revisions   RevisionStore          // persisted snapshots; nil means memory-only
	mu          sync.RWMutex
}

func NewCatalog(defs ...*Definition) *Catalog {
	return NewCatalogWithStore(NewMemoryRevisionStore(), defs...)
}

// NewCatalogWithStore builds a catalog whose published snapshots are
// persisted through revisions, so a restarted process can still resolve
// definitions adopted by existing saves.
func NewCatalogWithStore(revisions RevisionStore, defs ...*Definition) *Catalog {
	c := &Catalog{
		definitions: map[string]*Definition{},
		history:     map[string]*Definition{},
		revisions:   revisions,
	}
	// Rebuild the in-memory version history from snapshots saved by earlier
	// process runs. The newest snapshot for each story becomes its current
	// latest definition; ordering is by persisted version metadata below.
	if revisions != nil {
		if snapshots, err := revisions.LoadAll(); err == nil {
			latest := map[string]*Definition{}
			for _, d := range snapshots {
				hash, _ := DefHash(d)
				c.history[hash] = d
				cur := latest[d.ID]
				if cur == nil || revisionOrder(d, cur) > 0 {
					latest[d.ID] = d
				}
			}
			for id, d := range latest {
				c.definitions[id] = d
			}
		}
	}
	for _, d := range defs {
		hash, err := DefHash(d)
		if err != nil {
			continue
		}
		c.history[hash] = d
		if c.revisions != nil {
			_ = c.revisions.SaveRevision(d.ID, hash, d)
		}
		// A built-in seed never downgrades a newer revision restored from
		// snapshots written by a previous process run.
		if cur, ok := c.definitions[d.ID]; !ok || revisionOrder(d, cur) > 0 {
			c.definitions[d.ID] = d
		}
	}
	return c
}

// revisionOrder ranks built-in seeds below published snapshots and otherwise
// treats snapshots as equal so a restored snapshot wins on startup.
func revisionOrder(a, b *Definition) int {
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	return strings.Compare(a.Version, b.Version)
}

func (c *Catalog) Get(id string) (*Definition, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.definitions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStory, id)
	}
	return d, nil
}

func (c *Catalog) List() []*Definition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Definition, 0, len(c.definitions))
	for _, d := range c.definitions {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Update registers a (possibly revised) definition as the story's latest
// version. The previous revision stays addressable by its hash so pinned
// instances are unaffected.
func (c *Catalog) Update(d *Definition) error {
	if d == nil || d.ID == "" {
		return fmt.Errorf("definition missing id")
	}
	hash, err := DefHash(d)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.definitions[d.ID] = d
	c.history[hash] = d
	if c.revisions != nil {
		if err := c.revisions.SaveRevision(d.ID, hash, d); err != nil {
			return err
		}
	}
	return nil
}

// ErrRevisionInUse is returned when withdrawing a revision that a save is
// pinned to, that a pending adoption targets, or that a completed adoption
// history references.
var ErrRevisionInUse = fmt.Errorf("revision is in use and must be retained")

// RevisionUsage explains why a revision cannot be withdrawn.
type RevisionUsage struct {
	PinnedBy  []string `json:"pinnedBy"`
	PendingBy []string `json:"pendingBy"`
	HistoryOf []string `json:"historyOf"`
}

// WithdrawRevision removes a non-current revision snapshot when nothing
// references it. refs supplies the instance usage scan; a referenced
// revision is retained and the call returns the specific reasons. The
// current latest revision cannot be withdrawn this way either — publish a
// replacement instead.
func (c *Catalog) WithdrawRevision(storyID, hash string, refs *RevisionUsage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.history[hash]
	if !ok || d.ID != storyID {
		return fmt.Errorf("%w: %s@%s", ErrUnknownStory, storyID, hash)
	}
	if latest, ok := c.definitions[storyID]; ok {
		if h, _ := DefHash(latest); h == hash {
			return fmt.Errorf("%w: revision %s is the current latest version of %s; publish a replacement instead",
				ErrRevisionInUse, d.Version, storyID)
		}
	}
	if refs != nil && (len(refs.PinnedBy)+len(refs.PendingBy)+len(refs.HistoryOf) > 0) {
		return fmt.Errorf("%w: %s is referenced (pinned=%v pending=%v history=%v) and is retained so saves stay replayable",
			ErrRevisionInUse, d.Version, refs.PinnedBy, refs.PendingBy, refs.HistoryOf)
	}
	delete(c.history, hash)
	if c.revisions != nil {
		if err := c.revisions.DeleteRevision(storyID, hash); err != nil {
			return err
		}
	}
	return nil
}

// RevisionSnapshots returns the retained version hashes for a story with
// their version labels, oldest first.
func (c *Catalog) RevisionSnapshots(storyID string) []RevisionSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []RevisionSummary{}
	for hash, d := range c.history {
		if d.ID != storyID {
			continue
		}
		current := false
		if latest, ok := c.definitions[storyID]; ok {
			h, _ := DefHash(latest)
			current = h == hash
		}
		out = append(out, RevisionSummary{Version: d.Version, Hash: hash, Current: current})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// RevisionSummary describes one retained revision of a story.
type RevisionSummary struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
	Current bool   `json:"current"`
}

// DefinitionForHash returns the retained snapshot for a hash regardless of
// whether it is the latest.
func (c *Catalog) DefinitionForHash(hash string) (*Definition, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if d, ok := c.history[hash]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownStory, hash)
}

// LatestHash returns the hash of the story's current latest definition.
func (c *Catalog) LatestHash(storyID string) (string, error) {
	d, err := c.Get(storyID)
	if err != nil {
		return "", err
	}
	return DefHash(d)
}

// DefHash is a stable hash of the definition content used as a version pin.
func DefHash(d *Definition) (string, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16]), nil
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func newVisitID() string {
	b := make([]byte, 6)
	_, _ = crand.Read(b)
	return "v-" + hex.EncodeToString(b)
}

// Start creates a new instance pinned to the catalogue's current version.
func (c *Catalog) Start(storyID, instanceID string, initial map[string]interface{}) (*Instance, *Definition, error) {
	d, err := c.Get(storyID)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateDefinition(d); err != nil {
		return nil, nil, err
	}
	hash, err := DefHash(d)
	if err != nil {
		return nil, nil, err
	}
	facts := map[string]interface{}{}
	for k, v := range initial {
		facts[k] = v
	}
	now := nowUTC()
	inst := &Instance{
		ID:        instanceID,
		StoryID:   storyID,
		Version:   d.Version,
		DefHash:   hash,
		Current:   d.Start,
		VisitID:   newVisitID(),
		Rev:       1,
		Facts:     facts,
		Claimed:   []string{},
		Rewards:   []Reward{},
		Branches:  map[string]BranchInfo{},
		Decisions: map[string]IdemRecord{},
		Log:       []LogEntry{{Node: d.Start, At: now}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	return inst, d, nil
}

// pinnedDef resolves the exact definition an instance is pinned to. Any
// revision ever registered with this catalog remains playable; drift is
// only reported when the pinned revision is unknown (e.g. the save was
// created against a different catalogue).
func (c *Catalog) pinnedDef(inst *Instance) (*Definition, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if d, ok := c.history[inst.DefHash]; ok {
		return d, nil
	}
	latest := "none"
	if d, ok := c.definitions[inst.StoryID]; ok {
		if hash, err := DefHash(d); err == nil {
			latest = hash
		}
	}
	return nil, fmt.Errorf("%w: story %s is now %s, instance pinned at %s", ErrStoryVersionDrift, inst.StoryID, latest, inst.DefHash)
}

// View renders the current node and only the choices whose conditions hold
// against the pinned definition.
func (c *Catalog) View(inst *Instance) (NodeView, error) {
	d, err := c.pinnedDef(inst)
	if err != nil {
		return NodeView{}, err
	}
	return viewFor(d, inst)
}

func viewFor(d *Definition, inst *Instance) (NodeView, error) {
	node, ok := d.Nodes[inst.Current]
	if !ok {
		return NodeView{}, fmt.Errorf("%w: %s", ErrUnknownNode, inst.Current)
	}
	view := NodeView{Node: node, AvailableChoices: []AvailableChoice{}}
	for _, ch := range node.Choices {
		allowed := true
		if ch.Condition != nil {
			ok, err := Evaluate(ch.Condition, inst.Facts)
			if err != nil {
				return NodeView{}, err
			}
			allowed = ok
		}
		if allowed {
			view.AvailableChoices = append(view.AvailableChoices, AvailableChoice{Choice: ch, Allowed: true})
		}
	}
	return view, nil
}

// Apply re-validates the choice against the pinned story version server-side
// and grants its rewards exactly once per node visit.
//
// idemKey is an optional client-generated token: retried requests carrying
// the same token return the cached outcome and never grant twice. A bare
// re-submission of the same choice while still on the same visit returns
// ErrDuplicateChoice. Reaching the same node again on a later visit is a new
// decision and is allowed.
func (c *Catalog) Apply(inst *Instance, choiceID, idemKey string) (*AppliedChoice, error) {
	d, err := c.pinnedDef(inst)
	if err != nil {
		return nil, err
	}
	// A retried request carrying an idempotency key replays the original
	// outcome even if play has since moved to another node.
	if idemKey != "" {
		if rec, ok := inst.Decisions[idemKey]; ok && rec.ChoiceID == choiceID {
			view, viewErr := viewFor(d, inst)
			if viewErr != nil {
				return nil, viewErr
			}
			return &AppliedChoice{
				ChoiceID: rec.ChoiceID, Node: rec.FromNode, Rewards: rec.Rewards,
				View: view, Duplicate: true,
			}, nil
		}
	}
	if inst.PendingMigration != nil {
		return nil, ErrMigrationPending
	}
	node, ok := d.Nodes[inst.Current]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, inst.Current)
	}
	var choice *Choice
	for i := range node.Choices {
		if node.Choices[i].ID == choiceID {
			choice = &node.Choices[i]
			break
		}
	}
	if choice == nil {
		// A resubmission of the choice that just moved us here (e.g. a
		// double-clicked button whose first request already committed) must
		// be reported as a duplicate, not as a different kind of failure:
		// either way no rewards may be granted again.
		if len(inst.Log) > 0 {
			last := inst.Log[len(inst.Log)-1]
			if last.ChoiceID == choiceID && last.Node == inst.Current {
				return nil, fmt.Errorf("%w: %s@%s", ErrDuplicateChoice, choiceID, last.Node)
			}
		}
		for _, n := range d.Nodes {
			for i := range n.Choices {
				if n.Choices[i].ID == choiceID {
					return nil, fmt.Errorf("%w: %s", ErrChoiceNotOnNode, choiceID)
				}
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrUnknownChoice, choiceID)
	}

	claimKey := inst.VisitID + claimSep + choice.ID
	for _, k := range inst.Claimed {
		if k == claimKey {
			return nil, fmt.Errorf("%w: %s@%s", ErrDuplicateChoice, choice.ID, node.ID)
		}
	}

	// Re-confirm the condition at submit time against the pinned definition,
	// not against any previously rendered view.
	if choice.Condition != nil {
		ok, err := Evaluate(choice.Condition, inst.Facts)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: choice %s", ErrConditionFailed, choice.ID)
		}
	}

	rewards := make([]Reward, 0, len(choice.Effects))
	now := nowUTC()
	for _, eff := range choice.Effects {
		if err := applyEffect(inst.Facts, eff); err != nil {
			return nil, err
		}
		rewards = append(rewards, Reward{
			Node:     node.ID,
			ChoiceID: choice.ID,
			Key:      eff.Key,
			Op:       eff.Op,
			Value:    eff.Value,
			At:       now,
		})
	}
	inst.Claimed = append(inst.Claimed, claimKey)
	inst.Rewards = append(inst.Rewards, rewards...)
	if idemKey != "" {
		if inst.Decisions == nil {
			inst.Decisions = map[string]IdemRecord{}
		}
		inst.Decisions[idemKey] = IdemRecord{
			VisitID: inst.VisitID, ChoiceID: choice.ID, FromNode: node.ID, Rewards: rewards, At: now,
		}
	}
	inst.Current = choice.Target
	if _, ok := d.Nodes[inst.Current]; !ok {
		return nil, fmt.Errorf("choice %s targets missing node %s", choice.ID, choice.Target)
	}
	inst.VisitID = newVisitID()
	inst.Rev++
	inst.Log = append(inst.Log, LogEntry{Node: inst.Current, ChoiceID: choice.ID, At: now})
	inst.UpdatedAt = now

	view, err := viewFor(d, inst)
	if err != nil {
		return nil, err
	}
	return &AppliedChoice{ChoiceID: choice.ID, Node: node.ID, Rewards: rewards, View: view}, nil
}

func applyEffect(facts map[string]interface{}, eff Effect) error {
	switch eff.Op {
	case "flag":
		facts[eff.Key] = true
	case "set":
		if eff.Value == nil {
			return fmt.Errorf("%w: set requires a value", ErrBadEffect)
		}
		facts[eff.Key] = eff.Value
	case "inc":
		cur, curOK := toFloat(facts[eff.Key])
		delta, deltaOK := toFloat(eff.Value)
		if !deltaOK {
			return fmt.Errorf("%w: inc requires a numeric value", ErrBadEffect)
		}
		if !curOK {
			cur = 0
		}
		facts[eff.Key] = cur + delta
	case "item":
		items, _ := facts["items"].(map[string]interface{})
		if items == nil {
			items = map[string]interface{}{}
		}
		count, _ := toFloat(items[eff.Key])
		items[eff.Key] = count + 1
		facts["items"] = items
	default:
		return fmt.Errorf("%w: unknown op %q", ErrBadEffect, eff.Op)
	}
	return nil
}

// Fork snapshots an instance before a choice is made. The rehearsal branch is
// fully independent: facts, claim records and rewards never leak back.
func Fork(parent *Instance, branchID, label string) *Instance {
	now := nowUTC()
	if parent.Branches == nil {
		parent.Branches = map[string]BranchInfo{}
	}
	parent.Branches[branchID] = BranchInfo{ID: branchID, Label: label, Node: parent.Current, CreatedAt: now}
	parent.UpdatedAt = now
	forked := &Instance{
		ID:                branchID,
		StoryID:           parent.StoryID,
		Version:           parent.Version,
		DefHash:           parent.DefHash,
		Current:           parent.Current,
		Facts:             cloneFacts(parent.Facts),
		Claimed:           append([]string{}, parent.Claimed...),
		Rewards:           append([]Reward{}, parent.Rewards...),
		Branches:          map[string]BranchInfo{},
		VisitID:           parent.VisitID,
		Rev:               parent.Rev,
		Decisions:         cloneDecisions(parent.Decisions),
		Log:               append([]LogEntry{}, parent.Log...),
		ParentID:          parent.ID,
		RootID:            parent.RootID,
		Label:             label,
		MigrationDeferred: parent.MigrationDeferred,
		MigrationHistory:  append([]MigrationRecord{}, parent.MigrationHistory...),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if parent.PendingMigration != nil {
		pending := *parent.PendingMigration
		forked.PendingMigration = &pending
	}
	if forked.RootID == "" {
		forked.RootID = parent.ID
	}
	return forked
}

func cloneDecisions(in map[string]IdemRecord) map[string]IdemRecord {
	if in == nil {
		return map[string]IdemRecord{}
	}
	out := make(map[string]IdemRecord, len(in))
	for k, v := range in {
		v.Rewards = append([]Reward{}, v.Rewards...)
		out[k] = v
	}
	return out
}

func cloneFacts(in map[string]interface{}) map[string]interface{} {
	b, err := json.Marshal(in)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

// ValidateDefinition checks structural integrity of a story definition.
func ValidateDefinition(d *Definition) error {
	if d == nil {
		return fmt.Errorf("nil definition")
	}
	if d.ID == "" {
		return fmt.Errorf("definition missing id")
	}
	if _, ok := d.Nodes[d.Start]; !ok {
		return fmt.Errorf("start node %q does not exist", d.Start)
	}
	ids := map[string]bool{}
	for id, n := range d.Nodes {
		if n.ID == "" {
			n.ID = id
		}
		if ids[id] {
			return fmt.Errorf("duplicate node id %s", id)
		}
		ids[id] = true
		switch n.Type {
		case "dialogue", "choice", "end":
		default:
			return fmt.Errorf("node %s has unknown type %s", id, n.Type)
		}
	}
	for id, n := range d.Nodes {
		seen := map[string]bool{}
		for _, ch := range n.Choices {
			if ch.ID == "" {
				return fmt.Errorf("node %s has a choice without id", id)
			}
			if seen[ch.ID] {
				return fmt.Errorf("node %s has duplicate choice id %s", id, ch.ID)
			}
			seen[ch.ID] = true
			if ch.Condition != nil {
				if err := ValidateCondition(ch.Condition); err != nil {
					return fmt.Errorf("node %s choice %s: %w", id, ch.ID, err)
				}
			}
			for _, eff := range ch.Effects {
				if eff.Key == "" {
					return fmt.Errorf("node %s choice %s: effect missing key", id, ch.ID)
				}
			}
			if _, ok := d.Nodes[ch.Target]; !ok {
				return fmt.Errorf("node %s choice %s targets missing node %s", id, ch.ID, ch.Target)
			}
		}
	}
	return nil
}
