import type { Condition, Effect, Instance, Reward } from "./types";

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

export function describeEffect(eff: Effect): string {
  switch (eff.op) {
    case "inc":
      return eff.key + " " + (Number(eff.value) >= 0 ? "+" : "") + String(eff.value);
    case "item":
      return "获得道具：" + eff.key;
    case "flag":
      return "标记：" + eff.key;
    default:
      return eff.key + " = " + String(eff.value);
  }
}

export function describeCondition(cond: Condition): string {
  if (cond.all) return cond.all.map(describeCondition).join(" 且 ");
  if (cond.any) return cond.any.map(describeCondition).join(" 或 ");
  const fact = cond.fact ?? "";
  switch (cond.op) {
    case "exists": return `${fact} 已存在`;
    case "missing": return `${fact} 尚未获得`;
    case "==": return `${fact} == ${String(cond.value)}`;
    case "!=": return `${fact} != ${String(cond.value)}`;
    default: return `${fact} ${cond.op} ${String(cond.value)}`;
  }
}
