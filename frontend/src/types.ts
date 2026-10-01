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
  log: LogEntry[];
  createdAt?: string;
  updatedAt?: string;
}

export interface LogEntry {
  node: string;
  choiceId?: string;
  at: string;
  fromNode?: string;
  choiceText?: string;
  effects?: Effect[];
}

export interface TraceStep {
  seq: number;
  node: string;
  nodeText: string;
  speaker?: string;
  at: string;
  fromNode?: string;
  choiceId?: string;
  choiceText?: string;
  condition?: Condition;
  effects?: Effect[];
  rewards?: Reward[];
  end?: boolean;
}

export interface BranchComparison {
  baseId: string;
  otherId: string;
  forkNode: string;
  forkLabel?: string;
  prefix: TraceStep[];
  baseSteps: TraceStep[];
  otherSteps: TraceStep[];
  baseFacts: Record<string, unknown>;
  otherFacts: Record<string, unknown>;
  baseRewards: Reward[];
  otherRewards: Reward[];
  baseCurrent: string;
  otherCurrent: string;
}

export interface InstanceResponse {
  instance: Instance;
  view: NodeView;
  trace?: TraceStep[];
}

export interface AppliedResponse extends InstanceResponse {
  applied: {
    choiceId: string;
    node: string;
    rewards: Reward[];
    duplicate: boolean;
  };
}
