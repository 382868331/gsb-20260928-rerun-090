import type { Instance } from "../types";
import { describeReward } from "../format";

export function RewardLog({ instance }: { instance: Instance }) {
  return (
    <section className="card">
      <h2>发放记录</h2>
      {instance.rewards.length === 0 ? (
        <p className="muted">尚无奖励。</p>
      ) : (
        <ol className="rewards">
          {instance.rewards.map((r, i) => (
            <li key={r.at + "-" + r.choiceId + "-" + i}>
              <span className="reward-choice">{r.choiceId}</span>
              <span>{describeReward(r)}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
