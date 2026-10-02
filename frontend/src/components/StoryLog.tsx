import type { Instance, Reward, StoryDefinition } from "../types";
import { describeReward } from "../format";

export interface LogStep {
  nodeId: string;
  nodeText?: string;
  choiceText?: string;
  rewards: Reward[];
  at: string;
  migration?: import("../types").MigrationSummary;
}

// buildSteps replays the persisted log against the story definition so the
// record still explains itself after a reload: each entry names the node,
// the choice that led there (looked up on the previous node) and the rewards
// granted by that choice.
export function buildSteps(inst: Instance, def?: StoryDefinition): LogStep[] {
  return inst.log.map((entry, i) => {
    const node = def?.nodes[entry.node];
    const prevId = i > 0 ? inst.log[i - 1].node : undefined;
    const prev = prevId ? def?.nodes[prevId] : undefined;
    let choiceText: string | undefined;
    if (entry.choiceId) {
      const choice = prev?.choices?.find((c) => c.id === entry.choiceId);
      choiceText = choice ? choice.text : entry.choiceId;
    }
    const rewards = entry.choiceId
      ? inst.rewards.filter(
          (r) => r.choiceId === entry.choiceId && r.node === prevId,
        )
      : [];
    return {
      nodeId: entry.node,
      nodeText: node?.text,
      choiceText,
      rewards,
      at: entry.at,
      migration: entry.migration,
    };
  });
}

interface ListProps {
  inst: Instance;
  def?: StoryDefinition;
  from?: number;
  to?: number;
}

export function LogEntryList({ inst, def, from = 0, to }: ListProps) {
  const steps = buildSteps(inst, def).slice(from, to);
  if (steps.length === 0) {
    return <p className="muted">尚无记录。</p>;
  }
  return (
    <ol className="log-steps" start={from + 1}>
      {steps.map((step, i) => (
        <li key={from + "-" + i + "-" + step.nodeId}>
          <div className="log-node">
            <code>{step.nodeId}</code>
            {step.nodeText && <span className="log-node-text">{step.nodeText}</span>}
          </div>
          {step.choiceText && (
            <div className="log-choice">选择：{step.choiceText}</div>
          )}
          {step.rewards.map((r, j) => (
            <div className="log-reward" key={j}>
              奖励：{describeReward(r)}
            </div>
          ))}
          {step.migration && (
            <div className="log-migration">
              规则交界：{step.migration.fromVersion} → {step.migration.toVersion}；旧事件截止于 <code>{step.migration.boundaryNode}</code>，{step.migration.note}
            </div>
          )}
        </li>
      ))}
    </ol>
  );
}

export function StoryLog({
  instance,
  def,
}: {
  instance: Instance;
  def?: StoryDefinition;
}) {
  return (
    <section className="card">
      <h2>剧情记录</h2>
      <LogEntryList inst={instance} def={def} />
    </section>
  );
}
