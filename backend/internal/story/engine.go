package story

import "fmt"

// Instance is a single playthrough pinned to a definition version.
type Instance struct {
	StoryID     string          `json:"storyId"`
	Version     int             `json:"version"`
	Current     string          `json:"current"`
	State       map[string]any  `json:"state"`
	AppliedOnce map[string]bool `json:"appliedOnce"`
	Visited     map[string]int  `json:"visited"`
	Over        bool            `json:"over"`
}

// AvailableChoice is a choice the runner currently permits the player to take,
// with its post-condition effects already resolved.
type AvailableChoice struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Next string `json:"next"`
}

// View is what the UI shows at one point in the playthrough. The UI must offer
// exactly AvailableChoices and nothing else.
type View struct {
	NodeID   string            `json:"nodeId"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	Terminal bool              `json:"terminal"`
	Choices  []AvailableChoice `json:"choices"`
	State    map[string]any    `json:"state"`
	Visits   int               `json:"visits"`
}

// Start creates an instance and enters the definition's start node.
func Start(d *Definition) (*Instance, *View, error) {
	if err := ValidateDefinition(d); err != nil {
		return nil, nil, err
	}
	in := &Instance{
		StoryID:     d.ID,
		Version:     d.Version,
		Current:     d.Start,
		State:       map[string]any{},
		AppliedOnce: map[string]bool{},
		Visited:     map[string]int{},
	}
	if err := in.enter(d, d.Start); err != nil {
		return nil, nil, err
	}
	v, err := in.view(d)
	return in, v, err
}

// Resume rehydrates a save against the current definition. It refuses to
// continue when the save is bound to a different version or references a node
// that no longer exists, instead of silently redirecting the player.
func Resume(d *Definition, save *Instance) (*View, error) {
	if err := ValidateDefinition(d); err != nil {
		return nil, err
	}
	if save == nil {
		return nil, fmt.Errorf("missing save")
	}
	if save.StoryID != d.ID {
		return nil, fmt.Errorf("save belongs to story %q, expected %q", save.StoryID, d.ID)
	}
	if save.Version != d.Version {
		return nil, &VersionMismatchError{Saved: save.Version, Current: d.Version}
	}
	if _, ok := d.Nodes[save.Current]; !ok {
		return nil, &MissingNodeError{NodeID: save.Current}
	}
	if save.State == nil {
		save.State = map[string]any{}
	}
	if save.AppliedOnce == nil {
		save.AppliedOnce = map[string]bool{}
	}
	if save.Visited == nil {
		save.Visited = map[string]int{}
	}
	for id := range save.Visited {
		if _, ok := d.Nodes[id]; !ok {
			return nil, &MissingNodeError{NodeID: id}
		}
	}
	return save.view(d)
}

// VersionMismatchError means a save is pinned to another story version.
type VersionMismatchError struct{ Saved, Current int }

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("save version %d is incompatible with current story version %d; start a new playthrough", e.Saved, e.Current)
}

// MissingNodeError means a save references a node that was deleted.
type MissingNodeError struct{ NodeID string }

func (e *MissingNodeError) Error() string {
	return fmt.Sprintf("save references node %q which no longer exists in story version", e.NodeID)
}

func (in *Instance) enter(d *Definition, id string) error {
	n, ok := d.Nodes[id]
	if !ok {
		return &MissingNodeError{NodeID: id}
	}
	in.Current = id
	in.Visited[id]++
	if err := in.applyEffects(n.Enter, "enter:"+id); err != nil {
		return err
	}
	return nil
}

// AvailableChoices evaluates every condition against current state and returns
// exactly the choices the player may take. A gated choice that errors during
// evaluation is treated as unavailable (the definition is still rejected at
// validation when the shape is wrong, so this is a defensive path).
func (in *Instance) AvailableChoices(d *Definition) ([]AvailableChoice, error) {
	if err := in.checkBound(d); err != nil {
		return nil, err
	}
	n := d.Nodes[in.Current]
	out := make([]AvailableChoice, 0, len(n.Choices))
	unconditional := make([]AvailableChoice, 0, len(n.Choices))
	gatedMatches := make([]AvailableChoice, 0, len(n.Choices))
	for i := range n.Choices {
		ch := &n.Choices[i]
		if ch.When == nil {
			unconditional = append(unconditional, AvailableChoice{ID: ch.ID, Text: ch.Text, Next: ch.Next})
			continue
		}
		ok, err := ch.When.Eval(in.State)
		if err != nil {
			return nil, fmt.Errorf("choice %q: %w", ch.ID, err)
		}
		if ok {
			gatedMatches = append(gatedMatches, AvailableChoice{ID: ch.ID, Text: ch.Text, Next: ch.Next})
		}
	}
	// Unconditional choices are always offered. Gated choices are validated to
	// be pairwise mutually exclusive, so at most one can match.
	out = append(out, unconditional...)
	out = append(out, gatedMatches...)
	return out, nil
}

// Choose applies the choice's effects and enters its target node.
func (in *Instance) Choose(d *Definition, choiceID string) (*View, error) {
	if err := in.checkBound(d); err != nil {
		return nil, err
	}
	if in.Over {
		return nil, fmt.Errorf("playthrough is over")
	}
	n := d.Nodes[in.Current]
	allowed, err := in.AvailableChoices(d)
	if err != nil {
		return nil, err
	}
	var picked *Choice
	for i := range n.Choices {
		if n.Choices[i].ID == choiceID {
			picked = &n.Choices[i]
		}
	}
	if picked == nil {
		return nil, fmt.Errorf("choice %q does not exist on node %q", choiceID, in.Current)
	}
	permitted := false
	for _, c := range allowed {
		if c.ID == choiceID {
			permitted = true
		}
	}
	if !permitted {
		return nil, fmt.Errorf("choice %q is not currently allowed", choiceID)
	}
	if err := in.applyEffects(picked.Effects, "choice:"+picked.ID); err != nil {
		return nil, err
	}
	if err := in.enter(d, picked.Next); err != nil {
		return nil, err
	}
	return in.view(d)
}

func (in *Instance) view(d *Definition) (*View, error) {
	if err := in.checkBound(d); err != nil {
		return nil, err
	}
	n := d.Nodes[in.Current]
	choices, err := in.AvailableChoices(d)
	if err != nil {
		return nil, err
	}
	in.Over = n.Terminal
	stateCopy := make(map[string]any, len(in.State))
	for k, v := range in.State {
		stateCopy[k] = v
	}
	return &View{
		NodeID:   n.ID,
		Title:    n.Title,
		Text:     n.Text,
		Terminal: n.Terminal,
		Choices:  choices,
		State:    stateCopy,
		Visits:   in.Visited[n.ID],
	}, nil
}

func (in *Instance) checkBound(d *Definition) error {
	if in.StoryID != d.ID || in.Version != d.Version {
		return &VersionMismatchError{Saved: in.Version, Current: d.Version}
	}
	if _, ok := d.Nodes[in.Current]; !ok {
		return &MissingNodeError{NodeID: in.Current}
	}
	return nil
}

func (in *Instance) applyEffects(effects []Effect, source string) error {
	for i, e := range effects {
		key := source + "#" + fmt.Sprint(i) + ":" + e.Key
		if e.Once {
			if in.AppliedOnce[key] {
				continue
			}
			in.AppliedOnce[key] = true
		}
		switch {
		case e.Set != nil:
			in.State[e.Key] = e.Set
		case e.Add != nil:
			cur, _ := in.State[e.Key].(float64)
			in.State[e.Key] = cur + *e.Add
		case e.Grant != "":
			owned, _ := in.State[e.Key].(map[string]any)
			if owned == nil {
				owned = map[string]any{}
				in.State[e.Key] = owned
			}
			if _, exists := owned[e.Grant]; exists {
				continue
			}
			owned[e.Grant] = true
		}
	}
	return nil
}
