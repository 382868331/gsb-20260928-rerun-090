import type { Instance, NodeView } from "./types";

export function normalizeInstance(raw: Partial<Instance> | null | undefined): Instance {
  const fallbackId = typeof raw?.id === "string" ? raw.id : "unknown";
  return {
    id: fallbackId,
    storyId: raw?.storyId ?? "mist-harbor",
    version: raw?.version ?? "unknown",
    defHash: raw?.defHash ?? "",
    current: raw?.current ?? "",
    visitId: raw?.visitId ?? "",
    rev: Number.isFinite(raw?.rev) ? Number(raw?.rev) : 0,
    facts: normalizeFacts(raw?.facts),
    claimedRewards: Array.isArray(raw?.claimedRewards) ? raw.claimedRewards : [],
    rewards: Array.isArray(raw?.rewards) ? raw.rewards : [],
    branches: raw?.branches ?? {},
    decisions: raw?.decisions ?? {},
    log: Array.isArray(raw?.log) ? raw.log : raw?.current ? [{ node: raw.current, at: raw.updatedAt ?? "" }] : [],
    parentId: raw?.parentId,
    rootId: raw?.rootId,
    label: raw?.label,
    createdAt: raw?.createdAt,
    updatedAt: raw?.updatedAt,
  };
}

function normalizeFacts(raw: Record<string, unknown> | undefined): Record<string, unknown> {
  const facts = raw && typeof raw === "object" ? raw : {};
  if (facts.gold === undefined) {
    facts.gold = 0;
  }
  return facts;
}

export function normalizeView(raw: Partial<NodeView> | null | undefined): NodeView {
  const node = raw?.node ?? {
    id: "unknown",
    type: "dialogue" as const,
    text: "剧情节点读取失败",
  };
  return {
    node,
    availableChoices: Array.isArray(raw?.availableChoices) ? raw.availableChoices : [],
  };
}
