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
	ID        string                 `json:"id"`
	StoryID   string                 `json:"storyId"`
	Version   string                 `json:"version"`
	DefHash   string                 `json:"defHash"`
	Current   string                 `json:"current"`
	VisitID   string                 `json:"visitId"`
	Rev       int                    `json:"rev"`
	Facts     map[string]interface{} `json:"facts"`
	Claimed   []string               `json:"claimedRewards"`
	Rewards   []Reward               `json:"rewards,omitempty"`
	Decisions map[string]IdemRecord  `json:"decisions,omitempty"`
	Branches  map[string]BranchInfo  `json:"branches,omitempty"`
	Log       []LogEntry             `json:"log"`
	ParentID  string                 `json:"parentId,omitempty"`
	RootID    string                 `json:"rootId,omitempty"`
	Label     string                 `json:"label,omitempty"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
}

// BranchInfo records a rehearsal branch created from a parent instance.
type BranchInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Node      string `json:"node"`
	CreatedAt string `json:"createdAt"`
}

// IdemRecord caches a choice submission so a retried HTTP request with the
// same idempotency key returns the original result without re-granting.
type IdemRecord struct {
	VisitID  string   `json:"visitId"`
	ChoiceID string   `json:"choiceId"`
	FromNode string   `json:"fromNode"`
	Rewards  []Reward `json:"rewards"`
	At       string   `json:"at"`
}

type LogEntry struct {
	Node     string `json:"node"`
	ChoiceID string `json:"choiceId,omitempty"`
	At       string `json:"at"`
	// FromNode, ChoiceText and reward snapshot make the record self
	// explaining after restarts and version changes. Older saves simply
	// leave these empty and Trace rebuilds them from the pinned definition.
	FromNode   string   `json:"fromNode,omitempty"`
	ChoiceText string   `json:"choiceText,omitempty"`
	Effects    []Effect `json:"effects,omitempty"`
}

// TraceStep is one explainable step of a play-through: arriving at a node
// and, when applicable, the choice that led there with the condition that
// was confirmed server-side and the rewards it granted.
type TraceStep struct {
	Seq        int        `json:"seq"`
	Node       string     `json:"node"`
	NodeText   string     `json:"nodeText"`
	Speaker    string     `json:"speaker,omitempty"`
	At         string     `json:"at"`
	FromNode   string     `json:"fromNode,omitempty"`
	ChoiceID   string     `json:"choiceId,omitempty"`
	ChoiceText string     `json:"choiceText,omitempty"`
	Condition  *Condition `json:"condition,omitempty"`
	Effects    []Effect   `json:"effects,omitempty"`
	Rewards    []Reward   `json:"rewards,omitempty"`
	End        bool       `json:"end,omitempty"`
}

// BranchComparison explains how two play-throughs diverged. Prefix steps are
// shared history and must be treated as read-only; each side keeps its own
// conditions, rewards and later progress.
type BranchComparison struct {
	BaseID       string                 `json:"baseId"`
	OtherID      string                 `json:"otherId"`
	ForkNode     string                 `json:"forkNode"`
	ForkLabel    string                 `json:"forkLabel,omitempty"`
	Prefix       []TraceStep            `json:"prefix"`
	BaseSteps    []TraceStep            `json:"baseSteps"`
	OtherSteps   []TraceStep            `json:"otherSteps"`
	BaseFacts    map[string]interface{} `json:"baseFacts"`
	OtherFacts   map[string]interface{} `json:"otherFacts"`
	BaseRewards  []Reward               `json:"baseRewards"`
	OtherRewards []Reward               `json:"otherRewards"`
	BaseCurrent  string                 `json:"baseCurrent"`
	OtherCurrent string                 `json:"otherCurrent"`
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
