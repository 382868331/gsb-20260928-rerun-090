import { useCallback, useEffect, useState } from "react";
import { api, newRequestId } from "./api";
import type { Instance, NodeView } from "./types";

interface Notice {
  kind: "info" | "error";
  text: string;
}

function FactsTable({ facts }: { facts: Record<string, unknown> }) {
  const keys = Object.keys(facts).sort();
  if (keys.length === 0) {
    return <p className="muted">（暂无标记 / 道具）</p>;
  }
  return (
    <ul className="facts">
      {keys.map((k) => (
        <li key={k}>
          <code>{k}</code> = <code>{String(facts[k])}</code>
        </li>
      ))}
    </ul>
  );
}

export default function App() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [currentId, setCurrentId] = useState<string>("");
  const [instance, setInstance] = useState<Instance | null>(null);
  const [view, setView] = useState<NodeView | null>(null);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [busy, setBusy] = useState(false);
  const [initialGold, setInitialGold] = useState(0);

  const refreshList = useCallback(async () => {
    const res = await api.listInstances();
    setInstances(res.instances);
    return res.instances;
  }, []);

  const openInstance = useCallback(
    async (id: string) => {
      const res = await api.view(id);
      setInstance(res.instance);
      setView(res.view);
      setCurrentId(id);
      setNotice(null);
    },
    [],
  );

  useEffect(() => {
    refreshList()
      .then((list) => {
        if (list.length > 0) {
          void openInstance(list[0].id);
        }
      })
      .catch((e: Error) => setNotice({ kind: "error", text: e.message }));
  }, [refreshList, openInstance]);

  const startGame = async () => {
    setBusy(true);
    try {
      const facts: Record<string, number> = {};
      if (initialGold > 0) {
        facts.gold = initialGold;
      }
      const res = await api.start("hero-road", facts);
      await refreshList();
      await openInstance(res.instance.id);
    } catch (e) {
      setNotice({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  const choose = async (choiceId: string) => {
    if (!instance || busy) return;
    setBusy(true);
    // One idempotency key per button press; the server dedupes replays.
    const requestId = newRequestId();
    try {
      const res = await api.choose(instance.id, choiceId, requestId);
      setInstance(res.instance);
      setView({ node: res.result.node, availableChoices: res.result.availableChoices });
      const rewardCount = res.result.rewards.length;
      setNotice({
        kind: "info",
        text: res.result.replayed
          ? "重复提交已识别：未重复发放奖励。"
          : rewardCount > 0
            ? "已发放奖励：" + res.result.rewards.join(", ")
            : "已记录选择。",
      });
    } catch (e) {
      // The server remains authoritative: re-sync in case facts changed.
      setNotice({ kind: "error", text: (e as Error).message });
      try {
        const res = await api.view(instance.id);
        setInstance(res.instance);
        setView(res.view);
      } catch {
        // ignore refresh error
      }
    } finally {
      setBusy(false);
    }
  };

  const forkBranch = async () => {
    if (!instance || busy) return;
    setBusy(true);
    try {
      const name = "rehearsal-" + (instances.length + 1);
      const res = await api.fork(instance.id, name);
      await refreshList();
      await openInstance(res.instance.id);
      setNotice({ kind: "info", text: "已在选择前建立试演分支：" + res.instance.id });
    } catch (e) {
      setNotice({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  const ended = view?.node.type === "end";

  return (
    <div className="app">
      <header>
        <h1>游戏内容编排平台</h1>
        <div className="toolbar">
          <label>
            初始金币：
            <input
              type="number"
              min={0}
              value={initialGold}
              onChange={(e) => setInitialGold(Number(e.target.value) || 0)}
            />
          </label>
          <button onClick={startGame} disabled={busy}>
            开始新存档
          </button>
          <button onClick={forkBranch} disabled={busy || !instance || ended}>
            从当前位置建立试演分支
          </button>
        </div>
      </header>

      <main>
        <aside className="saves">
          <h2>存档 / 分支</h2>
          <ul>
            {instances.map((inst) => (
              <li key={inst.id}>
                <button
                  className={inst.id === currentId ? "active" : ""}
                  onClick={() => void openInstance(inst.id)}
                >
                  {inst.name || inst.id}
                  <small>
                    {" "}
                    v{inst.version}
                    {inst.parentId ? " (分支)" : ""}
                  </small>
                </button>
              </li>
            ))}
          </ul>
        </aside>

        <section className="stage">
          {notice && <div className={"notice " + notice.kind}>{notice.text}</div>}
          {!instance && <p className="muted">还没有存档，点击“开始新存档”。</p>}
          {instance && view && (
            <>
              <div className="meta">
                <span>存档：{instance.name || instance.id}</span>
                <span>剧情版本：{instance.version}</span>
                {instance.parentId && <span>派生自：{instance.parentId}</span>}
              </div>
              <article className="node">
                {view.node.speaker && <h3>{view.node.speaker}</h3>}
                <p>{view.node.text}</p>
                {ended ? (
                  <p className="ended">— 剧情结束 —</p>
                ) : (
                  <div className="choices">
                    {/* Only server-allowed choices are ever rendered. */}
                    {view.availableChoices.map((ch) => (
                      <button
                        key={ch.id}
                        disabled={busy}
                        onClick={() => void choose(ch.id)}
                      >
                        {ch.text}
                      </button>
                    ))}
                    {view.availableChoices.length === 0 && (
                      <p className="muted">当前没有可选项。</p>
                    )}
                  </div>
                )}
              </article>
              <div className="state">
                <h3>标记 / 道具</h3>
                <FactsTable facts={instance.facts} />
              </div>
            </>
          )}
        </section>
      </main>
    </div>
  );
}
