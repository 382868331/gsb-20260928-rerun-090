import type { NodeView } from "../types";

interface Props {
  view: NodeView | null;
  busy: boolean;
  onChoose: (choiceId: string) => void;
}

export function NodePanel({ view, busy, onChoose }: Props) {
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
          {view.availableChoices.length === 0 && (
            <div className="no-choice">当前没有满足条件的选项。</div>
          )}
          {view.availableChoices.map((choice) => (
            <button
              key={choice.id}
              className="choice"
              disabled={busy}
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
