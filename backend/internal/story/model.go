package story

// Op is a comparison applied to a story-state value.
type Op string

const (
	OpEq Op = "eq"
	OpNe Op = "ne"
	OpGt Op = "gt"
	OpGe Op = "ge"
	OpLt Op = "lt"
	OpLe Op = "le"
)

// Condition gates a choice. A nil Condition means the choice is the default
// (unconditional) option of its node.
type Condition struct {
	Key string `json:"key"`
	Op  Op     `json:"op"`
	Val any    `json:"val"`
}

// Effect mutates story state when a node is entered or a choice is taken.
type Effect struct {
	Key   string   `json:"key"`
	Set   any      `json:"set,omitempty"`
	Add   *float64 `json:"add,omitempty"`
	Once  bool     `json:"once,omitempty"`
	Grant string   `json:"grant,omitempty"`
}

type Node struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Enter    []Effect `json:"enter,omitempty"`
	Choices  []Choice `json:"choices,omitempty"`
	Terminal bool     `json:"terminal,omitempty"`
}

type Choice struct {
	ID      string     `json:"id"`
	Text    string     `json:"text"`
	Next    string     `json:"next"`
	When    *Condition `json:"when,omitempty"`
	Effects []Effect   `json:"effects,omitempty"`
}

// Definition is an immutable, versioned story graph.
type Definition struct {
	ID      string          `json:"id"`
	Version int             `json:"version"`
	Title   string          `json:"title"`
	Start   string          `json:"start"`
	Nodes   map[string]Node `json:"nodes"`
}
