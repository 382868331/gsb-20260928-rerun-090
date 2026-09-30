package story

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Errors returned by the engine.
var (
	ErrNodeNotFound   = errors.New("current node not found")
	ErrChoiceNotFound = errors.New("choice not found on current node")
	ErrChoiceDenied   = errors.New("choice condition is not satisfied")
	ErrStoryEnded     = errors.New("story has ended")
	ErrInvalidState   = errors.New("instance state is invalid")
)

// nowUTC is overridable in tests.
var nowUTC = func() time.Time { return time.Now().UTC() }

func timestamp() string { return nowUTC().Format(time.RFC3339Nano) }

// HashDefinition returns the stable content hash of a story definition.
func HashDefinition(def *Definition) (string, error) {
	b, err := json.Marshal(def)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateDefinition performs structural validation of a story definition.
func ValidateDefinition(def *Definition) error {
	if def == nil {
		return fmt.Errorf("definition is nil")
	}
	if def.ID == "" {
		return fmt.Errorf("story id is required")
	}
	if def.Version == "" {
		return fmt.Errorf("story version is required")
	}
	if def.Start == "" {
		return fmt.Errorf("start node is required")
	}
	if len(def.Nodes) == 0 {
		return fmt.Errorf("story has no nodes")
	}
	if _, ok := def.Nodes[def.Start]; !ok {
		return fmt.Errorf("start node %q does not exist", def.Start)
	}
	ids := map[string]bool{}
	for id, node := range def.Nodes {
		if id == "" {
			return fmt.Errorf("node id must not be empty")
		}
		if ids[id] {
			return fmt.Errorf("duplicate node id %q", id)
		}
		ids[id] = true
		switch node.Type {
		case "", "dialogue", "choice", "end":
		default:
			return fmt.Errorf("node %q: unknown type %q", id, node.Type)
		}
		choiceIDs := map[string]bool{}
		for _, ch := range node.Choices {
			if ch.ID == "" {
				return fmt.Errorf("node %q contains a choice without id", id)
			}
			if choiceIDs[ch.ID] {
				return fmt.Errorf("node %q: duplicate choice id %q", id, ch.ID)
			}
			choiceIDs[ch.ID] = true
			if ch.Condition != nil {
				if err := ValidateCondition(ch.Condition); err != nil {
					return fmt.Errorf("node %q choice %q: %w", id, ch.ID, err)
				}
			}
			for _, e := range ch.Effects {
				if e.Key == "" {
					return fmt.Errorf("node %q choice %q: effect requires a key", id, ch.ID)
				}
				switch e.Op {
				case "flag", "set", "inc":
				default:
					return fmt.Errorf("node %q choice %q: unknown effect op %q", id, ch.ID, e.Op)
				}
			}
		}
	}
	for id, node := range def.Nodes {
		if node.Type == "end" {
			continue
		}
		if len(node.Choices) == 0 && node.Type != "" {
			return fmt.Errorf("node %q has no choices and is not an end node", id)
		}
		for _, ch := range node.Choices {
			if _, ok := def.Nodes[ch.Target]; !ok {
				return fmt.Errorf("node %q choice %q: target %q does not exist", id, ch.ID, ch.Target)
			}
		}
	}
	return nil
}

// StartInstance creates a new playthrough pinned to the definition version.
func StartInstance(id string, def *Definition, facts map[string]interface{}) (*Instance, error) {
	if err := ValidateDefinition(def); err != nil {
		return nil, err
	}
	hash, err := HashDefinition(def)
	if err != nil {
		return nil, err
	}
	now := timestamp()
	inst := &Instance{
		ID:         id,
		StoryID:    def.ID,
		Version:    def.Version,
		DefHash:    hash,
		Current:    def.Start,
		Facts:      cloneFacts(facts),
		Claimed:    []string{},
		RequestIDs: map[string]RequestRecord{},
		Log:        []LogEntry{{Node: def.Start, At: now}},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return inst, nil
}

// View renders the current node with only the choices allowed by facts.
func (inst *Instance) View(def *Definition) (NodeView, error) {
	hash, err := HashDefinition(def)
	if err != nil {
		return NodeView{}, err
	}
	if hash != inst.DefHash {
		return NodeView{}, fmt.Errorf("instance pinned to %s/%s (%s), definition hash mismatch", inst.StoryID, inst.Version, inst.DefHash)
	}
	node, ok := def.Nodes[inst.Current]
	if !ok {
		return NodeView{}, fmt.Errorf("%w: %q", ErrNodeNotFound, inst.Current)
	}
	view := NodeView{Node: node, AvailableChoices: []AvailableChoice{}}
	for _, ch := range node.Choices {
		allowed := true
		if ch.Condition != nil {
			allowed, err = Evaluate(ch.Condition, inst.Facts)
			if err != nil {
				return NodeView{}, fmt.Errorf("choice %q: %w", ch.ID, err)
			}
		}
		if allowed {
			view.AvailableChoices = append(view.AvailableChoices, AvailableChoice{Choice: ch, Allowed: true})
		}
	}
	return view, nil
}

// ChoiceResult describes what happened after applying a choice.
type ChoiceResult struct {
	Node             Node              `json:"node"`
	AvailableChoices []AvailableChoice `json:"availableChoices"`
	ChoiceID         string            `json:"choiceId"`
	AppliedEffects   []Effect          `json:"appliedEffects"`
	Rewards          []string          `json:"rewards"`
	Replayed         bool              `json:"replayed"`
}

// SubmitChoice re-validates the choice against the pinned definition and current
// facts, applies effects once and advances the instance. A repeated requestID
// returns the current view without granting rewards again.
func (inst *Instance) SubmitChoice(def *Definition, choiceID, requestID string) (*ChoiceResult, error) {
	if def == nil {
		return nil, ErrInvalidState
	}
	hash, err := HashDefinition(def)
	if err != nil {
		return nil, err
	}
	if hash != inst.DefHash {
		return nil, fmt.Errorf("choice rejected: instance pinned to %s/%s, definition changed", inst.StoryID, inst.Version)
	}
	if requestID != "" && inst.RequestIDs != nil {
		if rec, ok := inst.RequestIDs[requestID]; ok {
			return inst.resultFor(def, rec.Choice, true, nil)
		}
	}
	node, ok := def.Nodes[inst.Current]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNodeNotFound, inst.Current)
	}
	if node.Type == "end" {
		return nil, ErrStoryEnded
	}
	var choice *Choice
	for i := range node.Choices {
		if node.Choices[i].ID == choiceID {
			choice = &node.Choices[i]
			break
		}
	}
	if choice == nil {
		return nil, fmt.Errorf("%w: %q on node %q", ErrChoiceNotFound, choiceID, inst.Current)
	}
	// Server-side re-confirmation against the pinned story version.
	if choice.Condition != nil {
		allowed, err := Evaluate(choice.Condition, inst.Facts)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, fmt.Errorf("%w: choice %q", ErrChoiceDenied, choice.ID)
		}
	}
	applied := applyEffects(inst.Facts, choice.Effects)
	claimKey := inst.Current + ":" + choice.ID
	if !contains(inst.Claimed, claimKey) {
		inst.Claimed = append(inst.Claimed, claimKey)
	}
	inst.Current = choice.Target
	now := timestamp()
	inst.UpdatedAt = now
	inst.Log = append(inst.Log, LogEntry{Node: choice.Target, ChoiceID: choice.ID, At: now})
	if requestID != "" {
		if inst.RequestIDs == nil {
			inst.RequestIDs = map[string]RequestRecord{}
		}
		inst.RequestIDs[requestID] = RequestRecord{Node: node.ID, Choice: choice.ID}
	}
	return inst.resultFor(def, choice.ID, false, applied)
}

func (inst *Instance) resultFor(def *Definition, choiceID string, replayed bool, applied []Effect) (*ChoiceResult, error) {
	view, err := inst.View(def)
	if err != nil {
		return nil, err
	}
	res := &ChoiceResult{
		Node:             view.Node,
		AvailableChoices: view.AvailableChoices,
		ChoiceID:         choiceID,
		Replayed:         replayed,
	}
	if len(applied) > 0 {
		res.AppliedEffects = applied
		res.Rewards = rewardKeys(applied)
	}
	return res, nil
}

// Fork creates an independent rehearsal copy branched before the next choice.
// Facts, reward claims, idempotency keys and log never leak back to the parent.
func (inst *Instance) Fork(newID, name string) *Instance {
	return &Instance{
		ID:         newID,
		Name:       name,
		StoryID:    inst.StoryID,
		Version:    inst.Version,
		DefHash:    inst.DefHash,
		ParentID:   inst.ID,
		Current:    inst.Current,
		Facts:      cloneFacts(inst.Facts),
		Claimed:    append([]string(nil), inst.Claimed...),
		RequestIDs: cloneRequestIDs(inst.RequestIDs),
		Log:        append([]LogEntry(nil), inst.Log...),
		CreatedAt:  timestamp(),
		UpdatedAt:  timestamp(),
	}
}

func applyEffects(facts map[string]interface{}, effects []Effect) []Effect {
	applied := make([]Effect, 0, len(effects))
	for _, e := range effects {
		switch e.Op {
		case "flag":
			facts[e.Key] = true
		case "set":
			facts[e.Key] = e.Value
		case "inc":
			cur, _ := facts[e.Key].(float64)
			inc, ok := toFloat(e.Value)
			if !ok {
				inc = 0
			}
			facts[e.Key] = cur + inc
		}
		applied = append(applied, e)
	}
	return applied
}

func rewardKeys(effects []Effect) []string {
	out := make([]string, 0, len(effects))
	for _, e := range effects {
		out = append(out, e.Op+":"+e.Key)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func cloneFacts(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	b, _ := json.Marshal(in)
	out := map[string]interface{}{}
	_ = json.Unmarshal(b, &out)
	return out
}

func cloneRequestIDs(in map[string]RequestRecord) map[string]RequestRecord {
	if in == nil {
		return map[string]RequestRecord{}
	}
	out := make(map[string]RequestRecord, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
