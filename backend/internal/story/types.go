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
	Migration *MigrationRecord       `json:"migration,omitempty"`
	ParentID  string                 `json:"parentId,omitempty"`
	RootID    string                 `json:"rootId,omitempty"`
	Label     string                 `json:"label,omitempty"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
}

type MigrationRecord struct {
	FromHash       string `json:"fromHash"`
	ToHash         string `json:"toHash"`
	FromVersion    string `json:"fromVersion"`
	ToVersion      string `json:"toVersion"`
	BoundaryNode   string `json:"boundaryNode"`
	ContinuedAt    string `json:"continuedAt"`
	ExplicitTarget string `json:"explicitTarget,omitempty"`
	Note           string `json:"note,omitempty"`
}

type MigrationReview struct {
	InstanceID       string          `json:"instanceId"`
	Label            string          `json:"label"`
	Status           string          `json:"status"` // current | upgradable | blocked
	CurrentVersion   string          `json:"currentVersion"`
	CurrentHash      string          `json:"currentHash"`
	LatestVersion    string          `json:"latestVersion"`
	LatestHash       string          `json:"latestHash"`
	BoundaryNode     string          `json:"boundaryNode"`
	CompletedEvents  []ReviewedEvent `json:"completedEvents"`
	FutureChanges    []RuleChange    `json:"futureChanges"`
	MissingNodes     []string        `json:"missingNodes"`
	AvailableTargets []string        `json:"availableTargets"`
	RewardNotice     string          `json:"rewardNotice"`
}

type ReviewedEvent struct {
	Node     string `json:"node"`
	ChoiceID string `json:"choiceId,omitempty"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
}

type RuleChange struct {
	NodeID   string `json:"nodeId"`
	ChoiceID string `json:"choiceId,omitempty"`
	Kind     string `json:"kind"` // condition | rewards | target | node
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Impact   string `json:"impact"`
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
	Node      string            `json:"node"`
	ChoiceID  string            `json:"choiceId,omitempty"`
	At        string            `json:"at"`
	Migration *MigrationSummary `json:"migration,omitempty"`
}

type MigrationSummary struct {
	FromVersion    string `json:"fromVersion"`
	ToVersion      string `json:"toVersion"`
	BoundaryNode   string `json:"boundaryNode"`
	ExplicitTarget string `json:"explicitTarget,omitempty"`
	Note           string `json:"note"`
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
