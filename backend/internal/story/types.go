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
	ID      string                 `json:"id"`
	StoryID string                 `json:"storyId"`
	Version string                 `json:"version"`
	DefHash string                 `json:"defHash"`
	Current string                 `json:"current"`
	VisitID string                 `json:"visitId"`
	Rev     int                    `json:"rev"`
	Facts   map[string]interface{} `json:"facts"`
	Claimed []string               `json:"claimedRewards"`
	// Rewards must always serialise (even as []) so clients never see an
	// undefined collection for a fresh instance.
	Rewards   []Reward              `json:"rewards"`
	Decisions map[string]IdemRecord `json:"decisions,omitempty"`
	Branches  map[string]BranchInfo `json:"branches,omitempty"`
	Log       []LogEntry            `json:"log"`
	ParentID  string                `json:"parentId,omitempty"`
	RootID    string                `json:"rootId,omitempty"`
	Label     string                `json:"label,omitempty"`
	// MigrationDeferred records the latest definition hash the author of
	// this save explicitly declined to adopt; the save keeps playing under
	// its pinned rules. Purely a marker: setting it never touches events.
	MigrationDeferred string `json:"migrationDeferred,omitempty"`
	// MigrationAdopted records the definition hash this save last adopted,
	// so clients can distinguish a save that adopted new rules from one
	// that simply never drifted.
	MigrationAdopted string `json:"migrationAdopted,omitempty"`
	// PendingMigration is set the moment an adopt decision is durably
	// saved and cleared only when the adoption completes. If the process
	// exits in between, the save reloads in an awaiting-recovery state:
	// the decision and the exact target revision are still on disk.
	PendingMigration *PendingMigration `json:"pendingMigration,omitempty"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
}

// PendingMigration is a persisted adopt decision waiting to be completed.
type PendingMigration struct {
	ToHash    string `json:"toHash"`
	ToVersion string `json:"toVersion"`
	DecidedAt string `json:"decidedAt"`
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
	// Note marks non-choice boundary entries, e.g. a rule migration, so the
	// story log can explain where old rules stop and new rules begin.
	Note string `json:"note,omitempty"`
	At   string `json:"at"`
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
