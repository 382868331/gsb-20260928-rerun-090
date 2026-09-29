import { useEffect, useState } from "react";
import { useStore } from "./store";
import { Editor } from "./Editor";
import { Player } from "./Player";

type Tab = "play" | "edit";

export function App() {
  const { stories, definition, loadError, refreshStories, selectStory } = useStore();
  const [tab, setTab] = useState<Tab>("play");

  useEffect(() => {
    refreshStories();
  }, [refreshStories]);

  useEffect(() => {
    if (!definition && stories.length > 0) {
      selectStory(stories[0].id);
    }
  }, [stories, definition, selectStory]);

  return (
    <div className="app">
      <header className="app-head">
        <h1>游戏内容编排平台</h1>
        <span className="muted">条件对话 · 剧情进度 · 版本化存档（本轮不含任务与角色关系）</span>
      </header>
      <div className="toolbar">
        <label>
          剧情：
          <select
            value={definition?.id ?? ""}
            onChange={(e) => selectStory(e.target.value)}
          >
            {stories.map((s) => (
              <option key={s.id} value={s.id}>
                {s.title}（v{s.version}）
              </option>
            ))}
          </select>
        </label>
        <nav>
          <button className={tab === "play" ? "tab active" : "tab"} onClick={() => setTab("play")}>
            试玩
          </button>
          <button className={tab === "edit" ? "tab active" : "tab"} onClick={() => setTab("edit")}>
            编辑分支
          </button>
        </nav>
      </div>
      {loadError && tab === "play" ? null : null}
      {definition ? (
        tab === "play" ? <Player /> : <Editor />
      ) : (
        <p className="muted">{loadError ?? "加载中…"}</p>
      )}
    </div>
  );
}
