export interface Condition {
  fact?: string;
  op?: string;
  value?: unknown;
  all?: Condition[];
  any?: Condition[];
}

export interface Effect {
  op: "flag" | "set" | "inc" | "item";
  key: string;
  value?: unknown;
}

export interface Choice {
  id: string;
  text: string;
  target: string;
  condition?: Condition;
  effects?: Effect[];
}

export interface AvailableChoice extends Choice {
  allowed: boolean;
}

export interface StoryNode {
  id: string;
  type: "dialogue" | "choice" | "end";
  speaker?: string;
  text: string;
  choices?: Choice[];
}

export interface NodeView {
  node: StoryNode;
  availableChoices: AvailableChoice[];
}

export interface StoryDefinition {
  id: string;
  title: string;
  version: string;
  start: string;
  nodes: Record<string, StoryNode>;
}

export interface Reward {
  node: string;
  choiceId: string;
  key: string;
  op: string;
  value?: unknown;
  at: string;
}

export interface BranchInfo {
  id: string;
  label: string;
  node: string;
  createdAt: string;
}

export interface Instance {
  id: string;
  storyId: string;
  version: string;
  defHash: string;
  current: string;
  visitId: string;
  rev: number;
  facts: Record<string, unknown>;
  claimedRewards: string[];
  rewards: Reward[];
  branches?: Record<string, BranchInfo>;
  parentId?: string;
  rootId?: string;
  label?: string;
  log: { node: string; choiceId?: string; at: string; migration?: MigrationSummary }[];
  decisions?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
}

export interface MigrationSummary {
  fromVersion: string;
  toVersion: string;
  boundaryNode: string;
  explicitTarget?: string;
  note: string;
}

export interface ReviewedEvent {
  node: string;
  choiceId?: string;
  summary: string;
  status: string;
}

export interface RuleChange {
  nodeId: string;
  choiceId?: string;
  kind: "condition" | "rewards" | "target" | "node" | string;
  before?: string;
  after?: string;
  impact: string;
}

export interface MigrationReview {
  instanceId: string;
  label: string;
  status: "current" | "upgradable" | "blocked" | string;
  currentVersion: string;
  currentHash: string;
  latestVersion: string;
  latestHash: string;
  boundaryNode: string;
  completedEvents: ReviewedEvent[];
  futureChanges: RuleChange[];
  missingNodes: string[];
  availableTargets: string[];
  rewardNotice: string;
}

export interface InstanceResponse {
  instance: Instance;
  view: NodeView;
}

export interface AppliedResponse extends InstanceResponse {
  applied: {
    choiceId: string;
    node: string;
    rewards: Reward[];
    duplicate: boolean;
  };
}
