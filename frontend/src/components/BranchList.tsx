import type { Instance } from "../types";

function statusOf(i: Instance): { key: string; label: string } | null {
  if (i.pendingMigration) {
    return { key: "pending", label: "等待恢复采用" };
  }
  const last = i.migrationHistory?.[i.migrationHistory.length - 1];
  if (last && last.toHash === i.defHash) {
    return { key: "adopted", label: `已采用 ${last.toVersion}` };
  }
  return null;
}

interface Props {
  instances: Instance[];
  currentId: string | null;
  onSelect: (id: string) => void;
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
            <button
              className={"branch-item " + (currentId === root.id ? "active" : "")}
              onClick={() => onSelect(root.id)}
            >
              <span className="branch-label">
                主线{root.label ? " · " + root.label : ""}
              </span>
              {statusOf(root) && (
                <span className={"branch-status branch-status-" + statusOf(root)?.key}>
                  {statusOf(root)?.label}
                </span>
              )}
              <span className="branch-node">{root.current}</span>
            </button>
            <ul className="subbranches">
              {childrenOf(root.id).map((b) => (
                <li key={b.id}>
                  <button
                    className={"branch-item " + (currentId === b.id ? "active" : "")}
                    onClick={() => onSelect(b.id)}
                    title={"从 " + b.parentId + " 分出"}
                  >
                    <span className="branch-label">{"⎇ " + (b.label || b.id)}</span>
                    {statusOf(b) && (
                      <span className={"branch-status branch-status-" + statusOf(b)?.key}>
                        {statusOf(b)?.label}
                      </span>
                    )}
                    <span className="branch-node">{b.current}</span>
                  </button>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </section>
  );
}
