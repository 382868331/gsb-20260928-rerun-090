export interface Condition {
  fact?: string;
  op?: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value?: any;
  all?: Condition[];
  any?: Condition[];
}

export interface Effect {
  op: "flag" | "set" | "inc";
  key: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value?: any;
}

export interface Choice {
  id: string;
  text: string;
  target: string;
  condition?: Condition;
  effects?: Effect[];
}

export interface Node {
  id: string;
  type: string;
  speaker?: string;
  text: string;
  choices?: Choice[];
}

export interface AvailableChoice extends Choice {
  allowed: boolean;
}

export interface NodeView {
  node: Node;
  availableChoices: AvailableChoice[];
}

export interface Instance {
  id: string;
  name?: string;
  storyId: string;
  version: string;
  defHash: string;
  parentId?: string;
  current: string;
  facts: Record<string, unknown>;
  claimedRewards?: string[];
  requestIds?: Record<string, { node: string; choice: string }>;
  log?: { node: string; choiceId?: string; at: string }[];
  createdAt: string;
  updatedAt: string;
}

export interface ChoiceResult {
  node: Node;
  availableChoices: AvailableChoice[];
  choiceId: string;
  appliedEffects: Effect[];
  rewards: string[];
  replayed: boolean;
}
