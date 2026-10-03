import { useCallback, useEffect, useState } from "react";
import type {
  AppliedResponse,
  Instance,
  InstanceResponse,
  MigrationImpact,
  MigrationState,
  NodeView,
  StoryDefinition,
} from "./types";
import {
  ApiError,
  createInstance,
  decideMigration,
  forkInstance,
  getInstance,
  getMigration,
  getStory,
  listInstances,
  newIdempotencyKey,
  submitChoice,
} from "./api";
import { FactsPanel } from "./components/FactsPanel";
import { NodePanel } from "./components/NodePanel";
import { BranchList } from "./components/BranchList";
import { RewardLog } from "./components/RewardLog";
import { StoryLog } from "./components/StoryLog";
import { ComparePanel } from "./components/ComparePanel";
import { MigrationPanel } from "./components/MigrationPanel";

export function App() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [currentId, setCurrentId] = useState<string | null>(null);
  const [instance, setInstance] = useState<Instance | null>(null);
  const [view, setView] = useState<NodeView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [stories, setStories] = useState<Record<string, StoryDefinition>>({});
  const [compareId, setCompareId] = useState<string | null>(null);
  const [drift, setDrift] = useState<{ latestVersion: string; latestDefHash: string } | null>(null);
  const [migration, setMigration] = useState<MigrationImpact | null>(null);
  const [migrationState, setMigrationState] = useState<MigrationState | null>(null);
  const [recovery, setRecovery] = useState(false);

  const refreshList = useCallback(async () => {
    try {
      const { instances } = await listInstances();
      // An empty archive is a valid state, never an error; older backends may
      // still send null, so normalise before anything iterates the list.
      setInstances(Array.isArray(instances) ? instances : []);
      setListError(null);
    } catch (e) {
      setListError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  // A genuine read failure keeps the user's selection and offers a retry;
  // it never falls back to silently starting another story.
  const loadInstance = useCallback(async (id: string) => {
    try {
      const res = await getInstance(id);
      setInstance(res.instance);
      setView(res.view);
      setCurrentId(id);
      setError(null);
      setLoadError(null);
      setMigration(null);
      setMigrationState(null);
      setRecovery(false);
      setDrift(
        res.versionDrift && res.latestDefHash
          ? { latestVersion: res.latestVersion ?? "", latestDefHash: res.latestDefHash }
          : null,
      );
    } catch (e) {
      setInstance(null);
      setView(null);
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    refreshList();
  }, [refreshList]);

  useEffect(() => {
    if (!instance) return;
    const storyId = instance.storyId;
    if (stories[storyId]) return;
    getStory(storyId)
      .then((def) => setStories((prev) => ({ ...prev, [storyId]: def })))
      .catch(() => undefined);
  }, [instance, stories]);

  useEffect(() => {
    if (currentId) {
      loadInstance(currentId);
    }
  }, [currentId, loadInstance]);

  const applyResponse = useCallback((res: AppliedResponse | InstanceResponse) => {
    setInstance(res.instance);
    setView(res.view);
    setMigration(null);
    setMigrationState(null);
    setRecovery(false);
    setDrift(
      res.versionDrift && res.latestDefHash
        ? { latestVersion: res.latestVersion ?? "", latestDefHash: res.latestDefHash }
        : null,
    );
    refreshList();
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

  const rootOf = (i: Instance) => i.rootId || i.id;

  const handleReviewMigration = async () => {
    if (!currentId || busy) return;
    setBusy(true);
    setError(null);
    try {
      const { migration, state, recovery } = await getMigration(currentId);
      setMigration(migration);
      setMigrationState(state ?? null);
      setRecovery(recovery === true);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleMigrationDecision = async (action: "adopt" | "defer" | "recover") => {
    if (!currentId || busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const res = await decideMigration(currentId, action);
      applyResponse(res);
      setNotice(
        action === "recover"
          ? "已恢复中断的采用：此前事件保持原样，此后选择按新规则求值。"
          : action === "adopt"
          ? "已采用新规则：此前事件保持原样，此后选择按新规则求值。"
          : "已保留原版本，已发生事件不受影响；可随时重新评估。",
      );
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setError(`迁移被拒绝：${e.message}`);
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const siblings = instance
    ? instances.filter((i) => i.id !== instance.id && rootOf(i) === rootOf(instance))
    : [];
  const compareOpen = compareId !== null && siblings.length > 0;

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
          <button
            onClick={() => setCompareId(compareOpen ? null : siblings[0]?.id ?? null)}
            disabled={!instance || siblings.length === 0}
            title="对比同一选择点分出的不同剧情分支"
          >
            比较分支
          </button>
        </div>
      </header>
      <main className="layout">
        <aside className="sidebar">
          {listError && (
            <div className="banner banner-error" role="alert">
              存档列表读取失败：{listError}
              <button onClick={refreshList}>重试</button>
            </div>
          )}
          <BranchList
            instances={instances}
            currentId={currentId}
            onSelect={setCurrentId}
          />
        </aside>
        <section className="stage">
          {error && <div className="banner banner-error" role="alert">{error}</div>}
          {notice && <div className="banner banner-notice">{notice}</div>}
          {loadError && (
            <div className="banner banner-error" role="alert">
              存档读取失败：{loadError}
              <button onClick={() => currentId && loadInstance(currentId)}>重试</button>
            </div>
          )}
          {instance && drift && (
            <div className="banner banner-notice">
              剧情规则已有新版本 {drift.latestVersion}，当前存档仍按原版本 {instance.version} 进行。
              <button onClick={handleReviewMigration} disabled={busy}>
                查看影响
              </button>
            </div>
          )}
          {instance?.migration?.status === "adopting" && (
            <div className="banner banner-notice" data-testid="recovery-banner">
              该分支的规则采用（{instance.migration.fromVersion} → {instance.migration.toVersion}）
              曾中断，需要核对恢复。
              <button onClick={handleReviewMigration} disabled={busy}>
                核对并恢复
              </button>
            </div>
          )}
          {migration && (
            <MigrationPanel
              impact={migration}
              state={migrationState}
              recovery={recovery}
              busy={busy}
              onAdopt={() => handleMigrationDecision("adopt")}
              onDefer={() => handleMigrationDecision("defer")}
              onRecover={() => handleMigrationDecision("recover")}
              onClose={() => setMigration(null)}
            />
          )}
          {!instance && loadError ? null : !instance ? (
            <div className="empty">点击“开始新故事”进入《雾港抉择》。</div>
          ) : (
            <>
              <NodePanel view={view} busy={busy} onChoose={handleChoose} />
              <div className="panels">
                <FactsPanel instance={instance} />
                <RewardLog instance={instance} />
                <StoryLog instance={instance} def={stories[instance.storyId]} />
              </div>
              {compareOpen && (
                <ComparePanel
                  base={instance}
                  others={siblings}
                  compareId={compareId}
                  defs={stories}
                  onSelect={setCompareId}
                  onClose={() => setCompareId(null)}
                />
              )}
            </>
          )}
        </section>
      </main>
    </div>
  );
}
