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
  migrationDeferred?: string;
  migrationHistory?: {
    fromVersion: string;
    toVersion: string;
    fromHash: string;
    toHash: string;
    at: string;
  }[];
  pendingMigration?: PendingAdoption;
  log: { node: string; choiceId?: string; note?: string; at: string }[];
  createdAt?: string;
  updatedAt?: string;
}

export interface PendingAdoption {
  storyId: string;
  fromVersion: string;
  toVersion: string;
  fromHash: string;
  toHash: string;
  at: string;
}

export type MigrationStatus = "current" | "available" | "pending" | "blocked";

export interface InstanceResponse {
  instance: Instance;
  view: NodeView;
  versionDrift?: boolean;
  latestVersion?: string;
  latestDefHash?: string;
  migrationStatus?: MigrationStatus;
  pendingMigration?: PendingAdoption;
}

export interface EventImpact {
  node: string;
  choiceId?: string;
  status: "unchanged" | "revised" | "removed";
  detail?: string;
}

export interface ChoiceImpact {
  choiceId: string;
  text: string;
  change:
    | "added"
    | "removed"
    | "condition-changed"
    | "reward-changed"
    | "target-changed"
    | "unchanged";
}

export interface MigrationImpact {
  storyId: string;
  fromVersion: string;
  toVersion: string;
  fromHash: string;
  toHash: string;
  upToDate: boolean;
  currentNode: string;
  currentNodeMissing: boolean;
  completed: EventImpact[];
  future: ChoiceImpact[];
  pending?: boolean;
  alreadyAdopted?: boolean;
}

export interface MigrationResponse extends InstanceResponse {
  migration: MigrationImpact;
}

export interface AppliedResponse extends InstanceResponse {
  applied: {
    choiceId: string;
    node: string;
    rewards: Reward[];
    duplicate: boolean;
  };
}
