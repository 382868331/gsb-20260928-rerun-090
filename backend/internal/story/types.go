package story

// Condition is a predicate over the play state facts.
// Exactly one of Fact/All/Any must be set.
type Condition struct {
	Fact  string      `json:"fact,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`
	All   []Condition `json:"all,omitempty"`
	Any   []Condition `json:"any,omitempty"`
}

type Effect struct {
	// Op is "flag" (set boolean), "set" (assign number/string) or "inc" (add number).
	Op    string      `json:"op"`
	Key   string      `json:"key"`
	Value interface{} `json:"value,omitempty"`
}

type Choice struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Target    string     `json:"target"`
	Condition *Condition `json:"condition,omitempty"`
	Effects   []Effect   `json:"effects,omitempty"`
}

type Node struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"` // dialogue | choice | end
	Speaker string   `json:"speaker,omitempty"`
	Text    string   `json:"text"`
	Choices []Choice `json:"choices,omitempty"`
}

type Definition struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	Version string          `json:"version"`
	Start   string          `json:"start"`
	Nodes   map[string]Node `json:"nodes"`
}

type Instance struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name,omitempty"`
	StoryID  string                 `json:"storyId"`
	Version  string                 `json:"version"`
	DefHash  string                 `json:"defHash"`
	ParentID string                 `json:"parentId,omitempty"`
	Current  string                 `json:"current"`
	Facts    map[string]interface{} `json:"facts"`
	Claimed  []string               `json:"claimedRewards,omitempty"`
	// RequestIDs maps a client-supplied idempotency key to the choice it applied.
	RequestIDs map[string]RequestRecord `json:"requestIds,omitempty"`
	Log        []LogEntry               `json:"log,omitempty"`
	CreatedAt  string                   `json:"createdAt"`
	UpdatedAt  string                   `json:"updatedAt"`
}

type LogEntry struct {
	Node     string `json:"node"`
	ChoiceID string `json:"choiceId,omitempty"`
	At       string `json:"at"`
}

// RequestRecord remembers where an idempotency key was consumed so a replay
// cannot advance the story a second time.
type RequestRecord struct {
	Node   string `json:"node"`
	Choice string `json:"choice"`
}

type AvailableChoice struct {
	Choice
	Allowed bool `json:"allowed"`
}

type NodeView struct {
	Node             Node              `json:"node"`
	AvailableChoices []AvailableChoice `json:"availableChoices"`
}

type ValidationReport struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}
