import type { Instance, StoryDefinition } from "../types";
import { LogEntryList } from "./StoryLog";

interface Props {
  base: Instance;
  others: Instance[];
  compareId: string | null;
  defs: Record<string, StoryDefinition>;
  onSelect: (id: string) => void;
  onClose: () => void;
}

function branchName(inst: Instance): string {
  return inst.parentId ? "⎇ " + (inst.label || inst.id) : "主线 " + inst.id;
}

// commonPrefix counts log entries identical in both branches; everything
// before the fork point is shared history and must stay read-only.
function commonPrefix(a: Instance, b: Instance): number {
  let n = 0;
  while (
    n < a.log.length &&
    n < b.log.length &&
    a.log[n].node === b.log[n].node &&
    a.log[n].choiceId === b.log[n].choiceId
  ) {
    n += 1;
  }
  return n;
}

export function ComparePanel({ base, others, compareId, defs, onSelect, onClose }: Props) {
  const other = others.find((i) => i.id === compareId) ?? others[0];
  if (!other) return null;
  const prefix = commonPrefix(base, other);
  const baseDef = defs[base.storyId];
  const otherDef = defs[other.storyId];
  return (
    <section className="card compare">
      <div className="compare-head">
        <h2>分支比较</h2>
        <label>
          对比
          <select value={other.id} onChange={(e) => onSelect(e.target.value)}>
            {others.map((i) => (
              <option key={i.id} value={i.id}>
                {branchName(i)}
              </option>
            ))}
          </select>
        </label>
        <button onClick={onClose}>关闭</button>
      </div>
      <div className="compare-shared">
        <h3>共同前缀（只读）</h3>
        <LogEntryList inst={base} def={baseDef} to={prefix} />
      </div>
      <div className="compare-cols">
        <div className="compare-col">
          <h3>{branchName(base)}</h3>
          <p className="muted">
            当前节点 <code>{base.current}</code> · 已领奖励 {base.rewards.length} 项
          </p>
          <LogEntryList inst={base} def={baseDef} from={prefix} />
        </div>
        <div className="compare-col">
          <h3>{branchName(other)}</h3>
          <p className="muted">
            当前节点 <code>{other.current}</code> · 已领奖励 {other.rewards.length} 项
          </p>
          <LogEntryList inst={other} def={otherDef} from={prefix} />
        </div>
      </div>
    </section>
  );
}
