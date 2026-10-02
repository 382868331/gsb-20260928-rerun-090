import type { Instance } from "../types";
import { describeReward, rewardsOf } from "../format";

export function RewardLog({ instance }: { instance: Instance }) {
  const rewards = rewardsOf(instance);
  return (
    <section className="card">
      <h2>发放记录</h2>
      {rewards.length === 0 ? (
        <p className="muted">尚无奖励。</p>
      ) : (
        <ol className="rewards">
          {rewards.map((r, i) => (
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
