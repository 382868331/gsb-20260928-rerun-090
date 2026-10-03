import type { Instance } from "../types";

interface Props {
  instances: Instance[];
  currentId: string | null;
  onSelect: (id: string) => void;
}

// migrationTag distinguishes, per branch: not yet adopted, adopted, and
// interrupted adoption awaiting recovery.
function migrationTag(i: Instance): { text: string; cls: string } | null {
  if (i.migration?.status === "adopting") {
    return { text: "采用中断 · 待恢复", cls: "tag-recovery" };
  }
  if (i.migration?.status === "adopted") {
    return { text: "已采用新规则", cls: "tag-adopted" };
  }
  if (i.migrationDeferred) {
    return { text: "保留原版本", cls: "tag-deferred" };
  }
  return null;
}

function BranchButton({ inst, currentId, onSelect, label, title }: {
  inst: Instance;
  currentId: string | null;
  onSelect: (id: string) => void;
  label: string;
  title?: string;
}) {
  const tag = migrationTag(inst);
  return (
    <button
      className={"branch-item " + (currentId === inst.id ? "active" : "")}
      onClick={() => onSelect(inst.id)}
      title={title}
    >
      <span className="branch-label">{label}</span>
      <span className="branch-node">{inst.current}</span>
      {tag && <span className={"tag " + tag.cls}>{tag.text}</span>}
    </button>
  );
}

export function BranchList({ instances, currentId, onSelect }: Props) {
  const roots = instances.filter((i) => !i.parentId);
  const childrenOf = (rootId: string) =>
    instances.filter((i) => (i.rootId || i.parentId) === rootId);
  return (
    <section className="card branch-list">
      <h2>存档与分支</h2>
      {instances.length === 0 && <p className="muted">还没有存档。</p>}
      <ul>
        {roots.map((root) => (
          <li key={root.id}>
            <BranchButton
              inst={root}
              currentId={currentId}
              onSelect={onSelect}
              label={"主线" + (root.label ? " · " + root.label : "")}
            />
            <ul className="subbranches">
              {childrenOf(root.id).map((b) => (
                <li key={b.id}>
                  <BranchButton
                    inst={b}
                    currentId={currentId}
                    onSelect={onSelect}
                    label={"⎇ " + (b.label || b.id)}
                    title={"从 " + b.parentId + " 分出"}
                  />
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </section>
  );
}
