import type { Instance, Reward } from "./types";

// rewardsOf tolerates payloads from older backends that omitted the
// collection entirely; an absent list must never blank the page.
export function rewardsOf(inst: Instance): Reward[] {
  return inst.rewards ?? [];
}

export function factsEntries(inst: Instance): [string, string][] {
  const out: [string, string][] = [];
  for (const [key, value] of Object.entries(inst.facts)) {
    if (key === "items") {
      const items = value as Record<string, unknown> | undefined;
      if (items) {
        for (const [item, count] of Object.entries(items)) {
          out.push(["道具·" + item, String(count)]);
        }
      }
      continue;
    }
    out.push([key, formatValue(value)]);
  }
  return out.sort((a, b) => a[0].localeCompare(b[0]));
}

export function formatValue(v: unknown): string {
  if (v === true) return "是";
  if (v === false) return "否";
  if (typeof v === "number" && Number.isInteger(v)) return String(v);
  return String(v);
}

export function describeReward(reward: Reward): string {
  switch (reward.op) {
    case "inc":
      return reward.key + " " + (Number(reward.value) >= 0 ? "+" : "") + reward.value;
    case "item":
      return "获得道具：" + reward.key;
    case "flag":
      return "标记：" + reward.key;
    default:
      return reward.key + " = " + String(reward.value);
  }
}
