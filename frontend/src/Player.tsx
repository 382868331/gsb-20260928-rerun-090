import { useState } from "react";
import { useStore } from "./store";

export function Player() {
  const {
    definition,
    play,
    saves,
    busy,
    loadError,
    startPlay,
    choose,
    resume,
    deleteSave,
  } = useStore();
  const [saveId, setSaveId] = useState("autosave");
  if (!definition) return null;

  const v = play.view;
  const storySaves = saves.filter((s) => s.storyId === definition.id);

  return (
    <section className="player">
      <header className="panel-head">
        <div>
          <h2>试玩</h2>
          <p className="muted">
            存档绑定 {definition.id} v{definition.version}
          </p>
        </div>
        <div className="row">
          <input
            value={saveId}
            onChange={(e) => setSaveId(e.target.value)}
            placeholder="存档名"
          />
          <button
            className="primary"
            disabled={busy || !saveId}
            onClick={() => startPlay(saveId)}
          >
            新开一局
          </button>
        </div>
      </header>

      {loadError ? (
        <div className="feedback error">
          {loadError}
          <span className="muted"> 运行器已阻止继续，避免跳转到错误节点。</span>
        </div>
      ) : null}

      {!v ? (
        <div className="empty">
          <p>选择存档名并“新开一局”，或从下方中途存档继续。</p>
          {storySaves.length ? (
            <ul className="save-list">
              {storySaves.map((s) => (
                <li key={s.id} className="save-row">
                  <button onClick={() => resume(s.id)} disabled={busy}>
                    继续 {s.id}
                  </button>
                  <span className="muted">
                    v{s.version} · 节点 {s.nodeId} · {s.over ? "已结束" : "进行中"}
                  </span>
                  {s.version !== definition.version ? (
                    <span className="tag warn">版本不匹配</span>
                  ) : null}
                  <button className="danger" onClick={() => deleteSave(s.id)}>
                    删除
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : (
        <div className="play-body">
          <article className="dialogue">
            <h3>{v.title}</h3>
            <p>{v.text}</p>
            <p className="muted small">
              节点 {v.nodeId} · 本节点第 {v.visits} 次到达
            </p>
            <div className="choices">
              {v.choices.length === 0 && !v.terminal ? (
                <p className="muted">当前没有允许的选项（所有条件均不满足）。</p>
              ) : null}
              {/* 只渲染运行器实际允许的选项，页面与引擎保持一致 */}
              {v.choices.map((c) => (
                <button
                  key={c.id}
                  className="choice"
                  disabled={busy}
                  onClick={() => choose(c.id)}
                >
                  {c.text} <span className="muted small">→ {c.next}</span>
                </button>
              ))}
              {v.terminal ? <div className="tag end">剧情结束</div> : null}
            </div>
          </article>
          <aside className="state">
            <h3>剧情状态</h3>
            <pre>{JSON.stringify(v.state, null, 2)}</pre>
            <h3>存档信息</h3>
            <pre>
{JSON.stringify(
                {
                  saveId: play.saveId,
                  version: play.save?.version,
                  current: play.save?.current,
                  appliedOnce: play.save?.appliedOnce,
                  visited: play.save?.visited,
                },
                null,
                2,
              )}
            </pre>
            {storySaves.length ? (
              <>
                <h3>其他存档</h3>
                <ul className="save-list compact">
                  {storySaves
                    .filter((s) => s.id !== play.saveId)
                    .map((s) => (
                      <li key={s.id} className="save-row">
                        <button onClick={() => resume(s.id)} disabled={busy}>
                          {s.id}
                        </button>
                        <span className="muted small">
                          v{s.version} @ {s.nodeId}
                        </span>
                      </li>
                    ))}
                </ul>
              </>
            ) : null}
          </aside>
        </div>
      )}
    </section>
  );
}
