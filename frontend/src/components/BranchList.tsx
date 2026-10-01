import type { Instance } from "../types";

interface Props {
  instances: Instance[];
  currentId: string | null;
  onSelect: (id: string) => void;
  onCompare: (id: string) => void;
  pendingCompareId?: string | null;
  listError?: string | null;
  onRetryList?: () => void;
}

export function BranchList({ instances, currentId, onSelect, onCompare, pendingCompareId, listError, onRetryList }: Props) {
  const roots = instances.filter((i) => !i.parentId);
  const childrenOf = (rootId: string) =>
    instances.filter((i) => (i.rootId || i.parentId) === rootId);
  return (
    <section className="card branch-list">
      <h2>存档与分支</h2>
      {listError && (
        <div className="banner banner-error" role="alert">
          存档列表读取失败：{listError}
          {onRetryList && (
            <button className="retry" onClick={onRetryList}>重试</button>
          )}
        </div>
      )}
      {instances.length === 0 && <p className="muted">还没有存档。</p>}
      {pendingCompareId && <p className="muted">已选择 {pendingCompareId}，再点另一条存档的「比较」。</p>}
      <ul>
        {roots.map((root) => (
          <li key={root.id}>
            <div className="branch-row">
              <button
                className={"branch-item " + (currentId === root.id ? "active" : "")}
                onClick={() => onSelect(root.id)}
              >
                <span className="branch-label">
                  主线{root.label ? " · " + root.label : ""}
                </span>
                <span className="branch-node">{root.current}</span>
              </button>
              <button
                className="compare-btn"
                onClick={() => onCompare(root.id)}
                aria-label={`比较存档 ${root.id}`}
              >
                {pendingCompareId === root.id ? "取消" : "比较"}
              </button>
            </div>
            <ul className="subbranches">
              {childrenOf(root.id).map((b) => (
                <li key={b.id}>
                  <div className="branch-row">
                    <button
                      className={"branch-item " + (currentId === b.id ? "active" : "")}
                      onClick={() => onSelect(b.id)}
                      title={"从 " + b.parentId + " 分出"}
                    >
                      <span className="branch-label">{"⎇ " + (b.label || b.id)}</span>
                      <span className="branch-node">{b.current}</span>
                    </button>
                    <button
                      className="compare-btn"
                      onClick={() => onCompare(b.id)}
                      aria-label={`比较分支 ${b.label || b.id}`}
                    >
                      {pendingCompareId === b.id ? "取消" : "比较"}
                    </button>
                  </div>
                  <button
                    className="compare-btn"
                    onClick={() => onCompare(b.id)}
                    aria-label={`比较分支 ${b.label || b.id}`}
                  >
                    {pendingCompareId === b.id ? "取消" : "比较"}
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
