export type Op = "eq" | "ne" | "gt" | "ge" | "lt" | "le";

export interface Condition {
  key: string;
  op: Op;
  val: unknown;
}

export interface Effect {
  key: string;
  set?: unknown;
  add?: number;
  once?: boolean;
  grant?: string;
}

export interface Choice {
  id: string;
  text: string;
  next: string;
  when?: Condition | null;
  effects?: Effect[];
}

export interface StoryNode {
  id: string;
  title: string;
  text: string;
  enter?: Effect[];
  choices?: Choice[];
  terminal?: boolean;
}

export interface Definition {
  id: string;
  version: number;
  title: string;
  start: string;
  nodes: Record<string, StoryNode>;
}

export interface AvailableChoice {
  id: string;
  text: string;
  next: string;
}

export interface View {
  nodeId: string;
  title: string;
  text: string;
  terminal: boolean;
  choices: AvailableChoice[];
  state: Record<string, unknown>;
  visits: number;
}

export interface Save {
  storyId: string;
  version: number;
  current: string;
  state: Record<string, unknown>;
  appliedOnce: Record<string, boolean>;
  visited: Record<string, number>;
  over: boolean;
}

export interface SaveSummary {
  id: string;
  storyId: string;
  version: number;
  nodeId: string;
  over: boolean;
}

export interface StorySummary {
  id: string;
  title: string;
  version: number;
}
