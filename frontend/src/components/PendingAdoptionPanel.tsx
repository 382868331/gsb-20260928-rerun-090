import type { Instance, PendingAdoption } from "../types";

interface Props {
  pending: PendingAdoption;
  instance: Instance;
  busy: boolean;
  onResume: () => void;
  onCancel: () => void;
}

// PendingAdoptionPanel is shown after an interrupted adoption (including a
// restart). The save is still presented under the old rules; this panel
// lays out the current facts and the recorded decision, and story choices
// stay locked until the user finishes or cancels recovery. Resuming is
// idempotent server-side, so a repeated click or retried request can never
// re-grant rewards or advance the story twice.
export function PendingAdoptionPanel({ pending, instance, busy, onResume, onCancel }: Props) {
  return (
    <section className="card migration" data-testid="pending-adoption-panel">
      <h2>采用被中断 · 等待恢复</h2>
      <p className="muted">
        该分支记录了一次尚未完成的规则采用决定。当前仍按旧版本
        <code> {pending.fromVersion} </code>
        进行；完成后才会切换到
        <code> {pending.toVersion}</code>。
      </p>

      <h3>当前事实（采用决定保存时的存档）</h3>
      <ul className="pending-facts" data-testid="pending-facts">
        <li>当前节点：<code>{instance.current}</code></li>
        {Object.entries(instance.facts).map(([key, value]) => (
          <li key={key}>
            <code>{key}</code> = {String(value)}
          </li>
        ))}
        <li>已领取奖励：{instance.rewards?.length ?? 0} 条（保持原样，不会重发）</li>
      </ul>

      <h3>待恢复的决定</h3>
      <ul className="pending-decision" data-testid="pending-decision">
        <li>从 <code>{pending.fromVersion}</code> 采用到 <code>{pending.toVersion}</code></li>
        <li>目标版本哈希：<code>{pending.toHash.slice(0, 12)}…</code></li>
        <li>决定记录于 {new Date(pending.at).toLocaleString()}</li>
      </ul>

      <div className="migration-actions">
        <button onClick={onResume} disabled={busy} data-testid="resume-adoption">
          确认事实无误，继续完成采用
        </button>
        <button onClick={onCancel} disabled={busy}>
          放弃本次采用，继续旧版本
        </button>
      </div>
    </section>
  );
}
