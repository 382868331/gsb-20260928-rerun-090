import type { TraceStep } from "../types";
import { describeCondition, describeEffect } from "../format";

interface Props {
  trace: TraceStep[];
}

// HistoryPanel renders the play log as an explainable, step-by-step record:
// every step shows how the player got there, the condition the server
// confirmed, and exactly which rewards changed hands.
export function HistoryPanel({ trace }: Props) {
  return (
    <section className="card history">
      <h2>剧情记录</h2>
      {trace.length === 0 ? (
        <p className="muted">还没有剧情进展。</p>
      ) : (
        <ol className="trace">
          {trace.map((step) => (
            <li key={`${step.seq}-${step.node}`} data-step={step.seq}>
              <div className="trace-head">
                <span className="trace-seq">第 {step.seq} 步</span>
                <span className="node-id">{step.node}</span>
                {step.end && <span className="tag tag-end">结局</span>}
              </div>
              {step.speaker && <div className="speaker">{step.speaker}</div>}
              {step.nodeText && <p className="node-text">{step.nodeText}</p>}
              {step.choiceId && (
                <div className="trace-choice" data-choice-id={step.choiceId}>
                  <span className="trace-choice-label">选择：</span>
                  {step.choiceText || step.choiceId}
                </div>
              )}
              {step.condition && (
                <div className="trace-condition">
                  条件（提交时由服务端重新确认）：{describeCondition(step.condition)}
                </div>
              )}
              {step.effects && step.effects.length > 0 && (
                <ul className="trace-rewards">
                  {step.effects.map((eff, i) => (
                    <li key={eff.key + "-" + i}>{describeEffect(eff)}</li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
