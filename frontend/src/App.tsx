import { useCallback, useEffect, useState } from "react";
import type { Instance, MigrationReview, NodeView, StoryDefinition } from "./types";
import {
  ApiError,
  createInstance,
  adoptMigration,
  forkInstance,
  getInstance,
  getStory,
  listInstances,
  listMigrations,
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
import { normalizeInstance, normalizeView } from "./normalize";

export function App() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [currentId, setCurrentId] = useState<string | null>(null);
  const [instance, setInstance] = useState<Instance | null>(null);
  const [view, setView] = useState<NodeView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [stories, setStories] = useState<Record<string, StoryDefinition>>({});
  const [compareId, setCompareId] = useState<string | null>(null);
  const [migrations, setMigrations] = useState<MigrationReview[]>([]);
  const [migrationError, setMigrationError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const refreshList = useCallback(async () => {
    try {
      const { instances } = await listInstances();
      // An empty archive is a valid state, never an error; older backends may
      // still send null, so normalise before anything iterates the list.
      setInstances((Array.isArray(instances) ? instances : []).map(normalizeInstance));
      setListError(null);
    } catch (e) {
      setListError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const refreshMigrations = useCallback(async () => {
    try {
      const { migrations } = await listMigrations();
      setMigrations(Array.isArray(migrations) ? migrations : []);
      setMigrationError(null);
    } catch (e) {
      setMigrationError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const loadInstance = useCallback(async (id: string) => {
    const res = await getInstance(id);
    const normalized = normalizeInstance(res.instance);
    setInstance(normalized);
    setView(normalizeView(res.view));
    setCurrentId(id);
    setError(null);
    setLoadError(null);
  }, []);

  useEffect(() => {
    refreshList();
    refreshMigrations();
  }, [refreshList, refreshMigrations]);

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
      loadInstance(currentId).catch((e: unknown) => {
        setError(e instanceof Error ? e.message : String(e));
        setLoadError(e instanceof Error ? e.message : String(e));
      });
    }
  }, [currentId, loadInstance]);

  const applyResponse = useCallback((res: { instance: unknown; view: unknown }) => {
    const normalized = normalizeInstance(res.instance as Partial<Instance>);
    setInstance(normalized);
    setView(normalizeView(res.view as Partial<NodeView>));
    refreshList();
    refreshMigrations();
  }, [refreshList, refreshMigrations]);

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
  const siblings = instance
    ? instances.filter((i) => i.id !== instance.id && rootOf(i) === rootOf(instance))
    : [];
  const compareOpen = compareId !== null && siblings.length > 0;

  const handleAdopt = async (review: MigrationReview, target?: string) => {
    setBusy(true);
    setError(null);
    try {
      const res = await adoptMigration(review.instanceId, target);
      applyResponse(res);
      setCurrentId(res.instance.id);
      await refreshMigrations();
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
          {migrationError && (
            <div className="banner banner-error" role="alert">
              规则审查读取失败：{migrationError}
              <button onClick={refreshMigrations}>重试规则审查</button>
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
          {loadError && currentId && (
            <div className="banner banner-error" role="alert">
              存档读取失败：{loadError}。已保留当前选择。
              <button onClick={() => loadInstance(currentId)}>重试读取</button>
            </div>
          )}
          {notice && <div className="banner banner-notice">{notice}</div>}
          {!instance ? (
            <div className="empty">点击“开始新故事”进入《雾港抉择》。</div>
          ) : (
            <>
              <NodePanel view={view} busy={busy} onChoose={handleChoose} />
              <MigrationPanel
                reviews={migrations}
                currentId={currentId}
                busy={busy}
                onAdopt={handleAdopt}
                onRefresh={refreshMigrations}
              />
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
