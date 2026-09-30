import { useCallback, useEffect, useState } from "react";
import type { AppliedResponse, Instance, NodeView } from "./types";
import {
  ApiError,
  createInstance,
  forkInstance,
  getInstance,
  listInstances,
  newIdempotencyKey,
  submitChoice,
} from "./api";
import { FactsPanel } from "./components/FactsPanel";
import { NodePanel } from "./components/NodePanel";
import { BranchList } from "./components/BranchList";
import { RewardLog } from "./components/RewardLog";

export function App() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [currentId, setCurrentId] = useState<string | null>(null);
  const [instance, setInstance] = useState<Instance | null>(null);
  const [view, setView] = useState<NodeView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const refreshList = useCallback(async () => {
    const { instances } = await listInstances();
    setInstances(instances);
  }, []);

  const loadInstance = useCallback(async (id: string) => {
    const res = await getInstance(id);
    setInstance(res.instance);
    setView(res.view);
    setCurrentId(id);
    setError(null);
  }, []);

  useEffect(() => {
    refreshList().catch(() => undefined);
  }, [refreshList]);

  useEffect(() => {
    if (currentId) {
      loadInstance(currentId).catch((e: unknown) => {
        setError(e instanceof Error ? e.message : String(e));
      });
    }
  }, [currentId, loadInstance]);

  const applyResponse = useCallback((res: AppliedResponse | { instance: Instance; view: NodeView }) => {
    setInstance(res.instance);
    setView(res.view);
    refreshList().catch(() => undefined);
  }, [refreshList]);

  const handleStart = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await createInstance();
      applyResponse(res);
      setCurrentId(res.instance.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleChoose = async (choiceId: string) => {
    if (!currentId || busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    // A fresh key per click: retries (same key) replay, genuine second
    // clicks on the same visit are rejected server-side.
    const key = newIdempotencyKey();
    try {
      const res = await submitChoice(currentId, choiceId, key);
      applyResponse(res);
      if (res.applied.duplicate) {
        setNotice("该提交已处理过，奖励未重复发放。");
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setError(`提交被拒绝：${e.message}`);
        await loadInstance(currentId);
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const handleFork = async () => {
    if (!currentId || busy) return;
    setBusy(true);
    setError(null);
    try {
      const label = window.prompt("给这次试演分支起个名字", `试演 @ ${instance?.current ?? ""}`);
      if (label === null) {
        return;
      }
      const res = await forkInstance(currentId, label);
      applyResponse(res);
      setCurrentId(res.instance.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app">
      <header className="topbar">
        <h1>游戏内容编排平台</h1>
        <div className="topbar-actions">
          <button onClick={handleStart} disabled={busy}>
            开始新故事
          </button>
          <button onClick={handleFork} disabled={busy || !instance} title="在本次选择之前建立独立试演分支">
            建立试演分支
          </button>
        </div>
      </header>
      <main className="layout">
        <aside className="sidebar">
          <BranchList
            instances={instances}
            currentId={currentId}
            onSelect={setCurrentId}
          />
        </aside>
        <section className="stage">
          {error && <div className="banner banner-error" role="alert">{error}</div>}
          {notice && <div className="banner banner-notice">{notice}</div>}
          {!instance ? (
            <div className="empty">点击“开始新故事”进入《雾港抉择》。</div>
          ) : (
            <>
              <NodePanel view={view} busy={busy} onChoose={handleChoose} />
              <div className="panels">
                <FactsPanel instance={instance} />
                <RewardLog instance={instance} />
              </div>
            </>
          )}
        </section>
      </main>
    </div>
  );
}
