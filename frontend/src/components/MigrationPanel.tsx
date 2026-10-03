import type { MigrationImpact, MigrationState } from "../types";

interface Props {
  impact: MigrationImpact;
  state?: MigrationState | null;
  recovery?: boolean;
  busy: boolean;
  onAdopt: () => void;
  onDefer: () => void;
  onRecover: () => void;
  onClose: () => void;
}

const EVENT_STATUS: Record<string, string> = {
  unchanged: "保持原样",
  revised: "规则已修订 · 事件保持原样",
  removed: "节点已移除 · 事件保持原样",
};

const CHOICE_CHANGE: Record<string, string> = {
  added: "新增选项",
  removed: "新规则已移除",
  "condition-changed": "条件变化",
  "reward-changed": "奖励变化",
  "target-changed": "后续节点变化",
  unchanged: "不变",
};

// MigrationPanel is the per-save review shown before a branch adopts a
// revised story definition: which completed events stay as they are and
// which future choices would change.
export function MigrationPanel({ impact, state, recovery, busy, onAdopt, onDefer, onRecover, onClose }: Props) {
  return (
    <section className="card migration" data-testid="migration-panel">
      <div className="migration-head">
        <h2>规则修订影响审查</h2>
        <button onClick={onClose}>关闭</button>
      </div>
      {recovery && state && (
        <div className="banner banner-notice" data-testid="migration-recovery">
          上次采用 <code>{state.fromVersion}</code> → <code>{state.toVersion}</code>
          的过程中断。下方展示实际保留下来的规则、采用记录与已发生事件；
          核对后可继续完成采用，或保留原版本。重复恢复不会重复发放奖励。
        </div>
      )}
      <p className="muted">
        版本 <code>{impact.fromVersion}</code> → <code>{impact.toVersion}</code>
        ，仅影响当前存档；其他分支继续原版本。
      </p>

      <h3>已完成事件（不会被改写）</h3>
      {impact.completed.length === 0 ? (
        <p className="muted">尚无已完成的选择。</p>
      ) : (
        <ul className="migration-events">
          {impact.completed.map((ev, i) => (
            <li key={ev.node + "-" + (ev.choiceId ?? "") + "-" + i}>
              <code>{ev.node}</code>
              {ev.choiceId && <span> · {ev.choiceId}</span>}
              <span className={"tag tag-" + ev.status}>{EVENT_STATUS[ev.status] ?? ev.status}</span>
              {ev.detail && <div className="muted">{ev.detail}</div>}
            </li>
          ))}
        </ul>
      )}

      <h3>未来选择（当前节点 {impact.currentNode}）</h3>
      {impact.currentNodeMissing ? (
        <p className="banner banner-error">
          当前节点在新规则中已不存在。进度不会静默跳回默认位置，因此无法自动迁移；
          请保留原版本，或等待作者恢复该节点。
        </p>
      ) : impact.future.every((f) => f.change === "unchanged") ? (
        <p className="muted">当前节点的可选分支没有变化。</p>
      ) : (
        <ul className="migration-future">
          {impact.future
            .filter((f) => f.change !== "unchanged")
            .map((f) => (
              <li key={f.choiceId}>
                <span>{f.text}</span> <code>{f.choiceId}</code>
                <span className="tag">{CHOICE_CHANGE[f.change] ?? f.change}</span>
              </li>
            ))}
        </ul>
      )}

      <p className="muted">
        采用后：已领取的奖励不会重新发放；尚未采用的修改可随时再评估，重复确认或撤回都不会改变已发生事件。
      </p>
      <div className="migration-actions">
        {recovery ? (
          <button onClick={onRecover} disabled={busy || impact.currentNodeMissing}>
            核对无误，完成恢复采用
          </button>
        ) : (
          <button onClick={onAdopt} disabled={busy || impact.currentNodeMissing}>
            采用新规则（仅此分支）
          </button>
        )}
        <button onClick={onDefer} disabled={busy}>
          暂不采用，保留原版本
        </button>
      </div>
    </section>
  );
}
