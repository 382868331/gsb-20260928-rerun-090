import type { NodeView } from "../types";

interface Props {
  view: NodeView | null;
  busy: boolean;
  locked?: boolean;
  onChoose: (choiceId: string) => void;
}

export function NodePanel({ view, busy, locked = false, onChoose }: Props) {
  if (!view) return null;
  const { node } = view;
  const ended = node.type === "end";
  return (
    <article className="node">
      <div className="node-meta">
        <span className="node-id">{node.id}</span>
        {ended && <span className="tag tag-end">结局</span>}
      </div>
      {node.speaker && <div className="speaker">{node.speaker}</div>}
      <p className="node-text">{node.text}</p>
      {!ended && (
        <div className="choices">
          {locked && <div className="no-choice">等待规则采用恢复完成或取消，剧情暂不能推进。</div>}
          {view.availableChoices.length === 0 && (
            <div className="no-choice">当前没有满足条件的选项。</div>
          )}
          {view.availableChoices.map((choice) => (
            <button
              key={choice.id}
              className="choice"
              disabled={busy || locked}
              aria-disabled={locked || undefined}
              onClick={() => onChoose(choice.id)}
              data-choice-id={choice.id}
            >
              {choice.text}
            </button>
          ))}
        </div>
      )}
    </article>
  );
}
