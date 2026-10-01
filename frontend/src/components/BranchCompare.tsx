import type { BranchComparison, TraceStep } from "../types";
import { describeEffect } from "../format";

interface Props {
  report: BranchComparison;
  onClose: () => void;
  onOpen: (id: string) => void;
}

function StepColumn({ title, steps, current }: { title: string; steps: TraceStep[]; current: string }) {
  return (
    <div className="compare-column">
      <h4>{title}</h4>
      <p className="muted">当前节点：{current}</p>
      {steps.length === 0 ? (
        <p className="muted">该分支尚无独立进展。</p>
      ) : (
        <ol className="compare-steps">
          {steps.map((step) => (
            <li key={`${step.seq}-${step.node}`}>
              <span className="node-id">{step.node}</span>
              {step.choiceText && <span>→ {step.choiceText}</span>}
              {step.effects && step.effects.length > 0 && (
                <ul>
                  {step.effects.map((eff, i) => (
                    <li key={eff.key + "-" + i}>{describeEffect(eff)}</li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

export function BranchCompare({ report, onClose, onOpen }: Props) {
  return (
    <section className="card compare" data-testid="branch-compare">
      <div className="compare-head">
        <h3>分支比较 · 分叉于 {report.forkNode}</h3>
        <button onClick={onClose}>关闭比较</button>
      </div>
      <div className="compare-prefix">
        <h4>共同前缀（只读）</h4>
        {report.prefix.length === 0 ? (
          <p className="muted">无共同步骤。</p>
        ) : (
          <ol className="compare-prefix-steps">
            {report.prefix.map((step) => (
              <li key={`${step.seq}-${step.node}`} className="readonly-step">
                <span className="node-id">{step.node}</span>
                {step.choiceText && <span>→ {step.choiceText}</span>}
              </li>
            ))}
          </ol>
        )}
      </div>
      <div className="compare-columns">
        <StepColumn title={`当前分支 ${report.baseId}`} steps={report.baseSteps} current={report.baseCurrent} />
        <StepColumn title={`对比分支 ${report.otherId}${report.forkLabel ? " · " + report.forkLabel : ""}`} steps={report.otherSteps} current={report.otherCurrent} />
      </div>
      <div className="compare-actions">
        <button onClick={() => onOpen(report.baseId)}>回到当前分支</button>
        <button onClick={() => onOpen(report.otherId)}>切到对比分支继续</button>
      </div>
    </section>
  );
}
