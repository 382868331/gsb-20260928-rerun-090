import { useCallback, useEffect, useState } from "react";
import type { RevisionInfo } from "../types";
import { listRevisions, withdrawRevision } from "../api";

interface Props {
  storyId: string;
  onChanged?: () => void;
}

// RevisionList shows every registered rules revision. Unused revisions can
// be withdrawn; revisions still referenced by saves or history markers are
// kept and the reason is shown next to them.
export function RevisionList({ storyId, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const [revisions, setRevisions] = useState<RevisionInfo[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const { revisions } = await listRevisions(storyId);
      setRevisions(Array.isArray(revisions) ? revisions : []);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [storyId]);

  useEffect(() => {
    if (open) {
      refresh();
    }
  }, [open, refresh]);

  const handleWithdraw = async (hash: string) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await withdrawRevision(storyId, hash);
      await refresh();
      onChanged?.();
    } catch (e) {
      // Referenced revisions are refused with an explanation; keep the list.
      setError(e instanceof Error ? e.message : String(e));
      await refresh();
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="card revisions" data-testid="revision-list">
      <div className="migration-head">
        <h2>规则修订版本</h2>
        <button onClick={() => setOpen(!open)}>{open ? "收起" : "展开"}</button>
      </div>
      {open && (
        <>
          {error && (
            <div className="banner banner-error" role="alert">
              {error}
              <button onClick={refresh}>重试</button>
            </div>
          )}
          <ul className="revision-items">
            {revisions.map((rev) => (
              <li key={rev.hash}>
                <code>{rev.version}</code>
                <span className="muted"> {rev.hash.slice(0, 8)}</span>
                {rev.latest && <span className="tag">最新发布</span>}
                {rev.inUse ? (
                  <span className="muted">保留中：{rev.reason}</span>
                ) : (
                  <button onClick={() => handleWithdraw(rev.hash)} disabled={busy}>
                    撤回
                  </button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
