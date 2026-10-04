import type { Instance } from "../types";

interface Props {
  instances: Instance[];
  currentId: string | null;
  onSelect: (id: string) => void;
  latestHashes: Record<string, string>;
}

// migrationBadge distinguishes branches that have not adopted a published
// revision, branches that adopted the rules they play, and branches whose
// adopt decision is still waiting to be recovered.
export function migrationBadge(
  i: Instance,
  latestHashes: Record<string, string>,
): { text: string; className: string } | null {
  if (i.pendingMigration) {
    return { text: "等待恢复", className: "tag tag-recovery" };
  }
  if (i.migrationAdopted && i.migrationAdopted === i.defHash) {
    return { text: "已采用", className: "tag tag-adopted" };
  }
  const latest = latestHashes[i.storyId];
  if (latest && latest !== i.defHash) {
    return { text: "未采用", className: "tag tag-pending" };
  }
  return null;
}

export function BranchList({ instances, currentId, onSelect, latestHashes }: Props) {
  const roots = instances.filter((i) => !i.parentId);
  const childrenOf = (rootId: string) =>
    instances.filter((i) => (i.rootId || i.parentId) === rootId);
  const renderItem = (i: Instance, label: string, title?: string) => {
    const badge = migrationBadge(i, latestHashes);
    return (
      <button
        className={"branch-item " + (currentId === i.id ? "active" : "")}
        onClick={() => onSelect(i.id)}
        title={title}
      >
        <span className="branch-label">{label}</span>
        <span className="branch-node">{i.current}</span>
        {badge && <span className={badge.className}>{badge.text}</span>}
      </button>
    );
  };
  return (
    <section className="card branch-list">
      <h2>存档与分支</h2>
      {instances.length === 0 && <p className="muted">还没有存档。</p>}
      <ul>
        {roots.map((root) => (
          <li key={root.id}>
            {renderItem(root, "主线" + (root.label ? " · " + root.label : ""))}
            <ul className="subbranches">
              {childrenOf(root.id).map((b) => (
                <li key={b.id}>{renderItem(b, "⎇ " + (b.label || b.id), "从 " + b.parentId + " 分出")}</li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </section>
  );
}
