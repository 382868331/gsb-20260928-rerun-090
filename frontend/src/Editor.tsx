import { useState } from "react";
import { useStore } from "./store";
import type { Choice, Definition, Effect, Op, StoryNode } from "./types";

const OPS: Op[] = ["eq", "ne", "gt", "ge", "lt", "le"];

export function Editor() {
  const { definition, updateDraft, saveDefinition, busy } = useStore();
  const [selected, setSelected] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  if (!definition) return null;

  const nodeIds = Object.keys(definition.nodes);
  const selectedId = selected && definition.nodes[selected] ? selected : definition.start;

  const patchDefinition = (next: Definition) => {
    updateDraft(next);
    setFeedback(null);
  };

  const updateNode = (id: string, patch: Partial<StoryNode>) => {
    const next = structuredClone(definition);
    next.nodes[id] = { ...next.nodes[id], ...patch };
    patchDefinition(next);
  };

  const addNode = () => {
    const id = prompt("新节点 ID（英文标识）", "node_" + (nodeIds.length + 1));
    if (!id || definition.nodes[id]) return;
    const node: StoryNode = {
      id,
      title: "新节点",
      text: "",
      choices: [],
      terminal: false,
    };
    const next = structuredClone(definition);
    next.nodes[id] = node;
    patchDefinition(next);
    setSelected(id);
  };

  const deleteNode = (id: string) => {
    const next = structuredClone(definition);
    delete next.nodes[id];
    for (const n of Object.values(next.nodes)) {
      n.choices = (n.choices ?? []).filter((c) => c.next !== id);
    }
    if (next.start === id) next.start = Object.keys(next.nodes)[0] ?? "";
    patchDefinition(next);
  };

  const publish = async () => {
    try {
      const toPublish = structuredClone(definition);
      toPublish.version = definition.version + 1;
      await saveDefinition(toPublish);
      setFeedback("已发布 v" + toPublish.version + "。旧版本存档将无法继续。");
    } catch (e) {
      setFeedback("发布被拒绝：" + (e as Error).message);
    }
  };

  const node = definition.nodes[selectedId];

  return (
    <section className="editor">
      <header className="panel-head">
        <div>
          <h2>分支编辑</h2>
          <p className="muted">
            {definition.title} · 当前发布版本 v{definition.version}（草稿基于此版本）
          </p>
        </div>
        <div className="row">
          <button onClick={addNode}>新增节点</button>
          <button className="primary" disabled={busy} onClick={publish}>
            校验并发布新版本
          </button>
        </div>
      </header>
      {feedback ? <div className="feedback">{feedback}</div> : null}
      <div className="editor-body">
        <ul className="node-list">
          {nodeIds.map((id) => (
            <li key={id}>
              <button
                className={id === selectedId ? "node-item active" : "node-item"}
                onClick={() => setSelected(id)}
              >
                <span className="node-id">{id}</span>
                <span className="muted">{definition.nodes[id].title}</span>
                {definition.start === id ? <span className="tag">起点</span> : null}
                {definition.nodes[id].terminal ? (
                  <span className="tag end">终局</span>
                ) : null}
              </button>
              {id !== definition.start ? (
                <button
                  className="danger"
                  title="删除节点（引用它的分支会一并移除）"
                  onClick={() => deleteNode(id)}
                >
                  删
                </button>
              ) : null}
            </li>
          ))}
        </ul>
        {node ? <NodeForm key={node.id} node={node} definition={definition} updateNode={updateNode} /> : (
          <p className="muted">没有节点了。</p>
        )}
      </div>
    </section>
  );
}

function NodeForm({
  node,
  definition,
  updateNode,
}: {
  node: StoryNode;
  definition: Definition;
  updateNode: (id: string, patch: Partial<StoryNode>) => void;
}) {
  const otherIds = Object.keys(definition.nodes).filter((id) => id !== node.id);

  const updateChoice = (idx: number, patch: Partial<Choice>) => {
    const choices = structuredClone(node.choices ?? []);
    choices[idx] = { ...choices[idx], ...patch };
    updateNode(node.id, { choices });
  };

  const addChoice = () => {
    const choices = structuredClone(node.choices ?? []);
    choices.push({
      id: "choice_" + (choices.length + 1),
      text: "新选项",
      next: otherIds[0] ?? node.id,
      when: null,
    });
    updateNode(node.id, { choices });
  };

  const removeChoice = (idx: number) => {
    const choices = (node.choices ?? []).filter((_, i) => i !== idx);
    updateNode(node.id, { choices });
  };

  const updateEnter = (effects: Effect[]) => updateNode(node.id, { enter: effects });

  return (
    <div className="node-form">
      <label>
        节点 ID
        <input
          value={node.id}
          readOnly
          title="节点 ID 不可修改；删除重建即可"
        />
      </label>
      <label>
        标题
        <input
          value={node.title}
          onChange={(e) => updateNode(node.id, { title: e.target.value })}
        />
      </label>
      <label>
        正文
        <textarea
          rows={3}
          value={node.text}
          onChange={(e) => updateNode(node.id, { text: e.target.value })}
        />
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={!!node.terminal}
          onChange={(e) => updateNode(node.id, { terminal: e.target.checked })}
        />
        终局节点（不能配置选项）
      </label>
      <StartPicker definition={definition} node={node} />

      <h3>进入时效果（奖励）</h3>
      <EffectsEditor effects={node.enter ?? []} onChange={updateEnter} />

      {!node.terminal ? (
        <>
          <h3>选项分支</h3>
          <p className="muted small">
            条件选项必须两两互斥，发布时由后端校验；无条件选项始终可见。
          </p>
          {(node.choices ?? []).map((choice, idx) => (
            <div key={idx} className="choice-card">
              <div className="row">
                <label>
                  选项 ID
                  <input
                    value={choice.id}
                    onChange={(e) => updateChoice(idx, { id: e.target.value })}
                  />
                </label>
                <label>
                  文案
                  <input
                    value={choice.text}
                    onChange={(e) => updateChoice(idx, { text: e.target.value })}
                  />
                </label>
                <label>
                  跳转节点
                  <select
                    value={choice.next}
                    onChange={(e) => updateChoice(idx, { next: e.target.value })}
                  >
                    {Object.keys(definition.nodes).map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                  </select>
                </label>
                <button className="danger" onClick={() => removeChoice(idx)}>
                  删除
                </button>
              </div>
              <ConditionEditor
                when={choice.when ?? null}
                onChange={(when) => updateChoice(idx, { when })}
              />
              <EffectsEditor
                effects={choice.effects ?? []}
                onChange={(effects) => updateChoice(idx, { effects })}
                title="选择时效果"
              />
            </div>
          ))}
          <button onClick={addChoice}>新增选项</button>
        </>
      ) : null}
    </div>
  );
}

function StartPicker({ definition, node }: { definition: Definition; node: StoryNode }) {
  const { updateDraft } = useStore();
  return (
    <label>
      剧情起点
      <select
        value={definition.start}
        onChange={(e) => {
          const next = structuredClone(definition);
          next.start = e.target.value;
          updateDraft(next);
        }}
      >
        {Object.keys(definition.nodes).map((id) => (
          <option key={id} value={id}>
            {id}
          </option>
        ))}
      </select>
      <span className="muted small"> 当前选中：{node.id}</span>
    </label>
  );
}

function ConditionEditor({
  when,
  onChange,
}: {
  when: Choice["when"];
  onChange: (w: Choice["when"]) => void;
}) {
  return (
    <div className="condition">
      <label className="check">
        <input
          type="checkbox"
          checked={!!when}
          onChange={(e) =>
            onChange(e.target.checked ? { key: "gold", op: "ge", val: 0 } : null)
          }
        />
        带条件
      </label>
      {when ? (
        <div className="row">
          <input
            placeholder="状态键"
            value={when.key}
            onChange={(e) => onChange({ ...when, key: e.target.value })}
          />
          <select
            value={when.op}
            onChange={(e) => onChange({ ...when, op: e.target.value as Op })}
          >
            {OPS.map((op) => (
              <option key={op} value={op}>
                {op}
              </option>
            ))}
          </select>
          <input
            placeholder="比较值"
            value={String(when.val ?? "")}
            onChange={(e) => onChange({ ...when, val: parseValue(e.target.value) })}
          />
        </div>
      ) : null}
    </div>
  );
}

function EffectsEditor({
  effects,
  onChange,
  title = "效果列表",
}: {
  effects: Effect[];
  onChange: (effects: Effect[]) => void;
  title?: string;
}) {
  const update = (idx: number, patch: Partial<Effect>) => {
    const next = structuredClone(effects);
    next[idx] = { ...next[idx], ...patch };
    onChange(next);
  };
  return (
    <div className="effects">
      <div className="muted small">{title}</div>
      {effects.map((eff, idx) => (
        <div className="row effect-row" key={idx}>
          <input
            placeholder="键"
            value={eff.key}
            onChange={(e) => update(idx, { key: e.target.value })}
          />
          <select
            value={eff.grant ? "grant" : eff.add != null ? "add" : "set"}
            onChange={(e) => {
              const kind = e.target.value;
              const base: Effect = { key: eff.key };
              if (kind === "set") base.set = 0;
              if (kind === "add") base.add = 0;
              if (kind === "grant") base.grant = "";
              onChange(effects.map((x, i) => (i === idx ? { ...base, once: x.once } : x)));
            }}
          >
            <option value="set">set 赋值</option>
            <option value="add">add 增减</option>
            <option value="grant">grant 授予</option>
          </select>
          {eff.set !== undefined ? (
            <input
              placeholder="值"
              value={String(eff.set)}
              onChange={(e) => update(idx, { set: parseValue(e.target.value) })}
            />
          ) : null}
          {eff.add !== undefined ? (
            <input
              type="number"
              value={eff.add}
              onChange={(e) => update(idx, { add: Number(e.target.value) })}
            />
          ) : null}
          {eff.grant !== undefined ? (
            <input
              placeholder="授予物 id"
              value={eff.grant}
              onChange={(e) => update(idx, { grant: e.target.value })}
            />
          ) : null}
          <label className="check">
            <input
              type="checkbox"
              checked={!!eff.once}
              onChange={(e) => update(idx, { once: e.target.checked })}
            />
            仅一次
          </label>
          <button className="danger" onClick={() => onChange(effects.filter((_, i) => i !== idx))}>
            删
          </button>
        </div>
      ))}
      <button onClick={() => onChange([...effects, { key: "new_key", set: 0 }])}>
        新增效果
      </button>
    </div>
  );
}

function parseValue(raw: string): unknown {
  if (raw === "true") return true;
  if (raw === "false") return false;
  const n = Number(raw);
  return raw !== "" && !Number.isNaN(n) ? n : raw;
}
