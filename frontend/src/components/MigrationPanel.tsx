import { useState } from "react";
import type { MigrationReview, RuleChange } from "../types";

interface Props {
  reviews: MigrationReview[];
  currentId: string | null;
  busy?: boolean;
  onAdopt: (review: MigrationReview, target?: string) => void;
  onRefresh: () => void;
}

const kindLabel: Record<string, string> = {
  condition: "条件修订",
  rewards: "奖励修订",
  target: "后续节点修订",
  node: "节点移除",
};

function ChangeRow({ change }: { change: RuleChange }) {
  return (
    <li>
      <strong>{kindLabel[change.kind] ?? change.kind}</strong>
      <code>{change.nodeId}{change.choiceId ? ` / ${change.choiceId}` : ""}</code>
      <p>{change.impact}</p>
      {(change.before || change.after) && (
        <p className="muted">
          {change.before ? `旧：${change.before}` : "旧版无此项"} → {change.after || "新版无此项"}
        </p>
      )}
    </li>
  );
}

export function MigrationPanel({ reviews, currentId, busy, onAdopt, onRefresh }: Props) {
  const [targets, setTargets] = useState<Record<string, string>>({});
  const relevant = reviews.filter((r) => r.status === "upgradable" || r.status === "blocked");
  if (relevant.length === 0) return null;

  return (
    <section className="card migration-panel">
      <div className="compare-head">
        <h2>剧情规则修订审查</h2>
        <button onClick={onRefresh} disabled={busy}>重新审查</button>
      </div>
      {relevant.map((review) => (
        <article className={"migration-review " + (currentId === review.instanceId ? "active" : "")} key={review.instanceId}>
          <h3>{review.label} · {review.currentVersion} → {review.latestVersion}</h3>
          <p>交界节点：<code>{review.boundaryNode}</code>。此节点之前的所有已完成事件与奖励原样保留，新规则只影响之后的选择。</p>
          <details open={currentId === review.instanceId}>
            <summary>已完成事件（保持原样，{review.completedEvents.length} 项）</summary>
            <ol className="completed-events">
              {review.completedEvents.map((event, i) => (
                <li key={`${event.node}-${event.choiceId ?? "node"}-${i}`}>
                  <span>{event.summary}</span><em>原样</em>
                </li>
              ))}
            </ol>
          </details>
          <details open={currentId === review.instanceId}>
            <summary>受到影响的未来选择（{review.futureChanges.length} 项）</summary>
            {review.futureChanges.length === 0 ? (
              <p className="muted">没有可达选择受到影响。</p>
            ) : (
              <ol className="rule-changes">{review.futureChanges.map((change, i) => <ChangeRow key={`${change.nodeId}-${change.choiceId ?? change.kind}-${i}`} change={change} />)}</ol>
            )}
          </details>
          <p className="reward-notice">{review.rewardNotice}</p>
          {review.status === "blocked" && (
            <div className="target-choice">
              <label>
                原节点已移除，请明确选择新规则下的接续节点：
                <select
                  aria-label={`${review.label} 的接续节点`}
                  value={targets[review.instanceId] ?? ""}
                  onChange={(e) => setTargets((prev) => ({ ...prev, [review.instanceId]: e.target.value }))}
                >
                  <option value="">请选择接续节点</option>
                  {review.availableTargets.map((target) => <option key={target} value={target}>{target}</option>)}
                </select>
              </label>
            </div>
          )}
          <div className="migration-actions">
            <button
              disabled={busy || (review.status === "blocked" && !targets[review.instanceId])}
              onClick={() => onAdopt(review, targets[review.instanceId])}
            >
              仅为此分支采用新规则
            </button>
            <span className="muted">不采用时继续旧版本；重复审查或撤回未采用修改都不会改变已发生事件。</span>
          </div>
        </article>
      ))}
    </section>
  );
}
