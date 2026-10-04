// These tests drive the UI against a stateful mock of the HTTP API. The mock
// enforces conditions and duplicate rules server-side: the component never
// decides availability itself, so the assertions prove state is correct
// rather than merely that buttons are disabled.
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { App } from "./App";
import type { Choice, Instance, NodeView, StoryNode } from "./types";

interface State {
  instances: Record<string, Instance>;
}

const state: State = { instances: {} };

let idCounter = 0;

function evalAtom(cond: { fact?: string; op?: string; value?: unknown }, facts: Record<string, unknown>): boolean {
  const actual = facts[cond.fact as string];
  switch (cond.op) {
    case ">=":
      return Number(actual) >= Number(cond.value);
    case "==":
      return actual === cond.value;
    case "missing":
      return !(cond.fact as string in facts);
    default:
      return false;
  }
}

const mockStory = {
  nodes: {
    square: {
      id: "square",
      type: "choice" as StoryNode["type"],
      text: "广场",
      choices: [
        { id: "chop_wood", text: "去干活", target: "lumberyard", effects: [{ op: "inc" as const, key: "gold", value: 5 }] },
        {
          id: "bribe_guard",
          text: "贿赂卫兵",
          target: "ending_normal",
          condition: { fact: "gold", op: ">=", value: 10 },
          effects: [{ op: "inc" as const, key: "gold", value: -10 }],
        },
      ] as Choice[],
    },
    lumberyard: {
      id: "lumberyard",
      type: "choice" as StoryNode["type"],
      text: "伐木场",
      choices: [{ id: "deliver_wood", text: "领工钱", target: "square", effects: [{ op: "inc" as const, key: "gold", value: 8 }] }] as Choice[],
    },
    ending_normal: { id: "ending_normal", type: "end" as StoryNode["type"], text: "结局", choices: [] as Choice[] },
  } satisfies Record<string, StoryNode>,
};

function viewFor(inst: Instance): NodeView {
  const node = mockStory.nodes[inst.current as keyof typeof mockStory.nodes];
  const availableChoices = (node.choices ?? [])
    .filter((ch) => {
      if (!ch.condition) return true;
      const c = ch.condition;
      if (c.all) return c.all.every((a) => evalAtom(a, inst.facts));
      return evalAtom(c, inst.facts);
    })
    .map((ch) => ({ ...ch, allowed: true }));
  return { node, availableChoices };
}

function newInstance(id: string, parent?: Instance): Instance {
  const base: Instance = parent
    ? (JSON.parse(JSON.stringify(parent)) as Instance)
    : {
        id,
        storyId: "t",
        version: "v1",
        defHash: "h",
        current: "square",
        visitId: "v1",
        rev: 1,
        facts: { gold: 0 },
        claimedRewards: [],
        rewards: [],
        branches: {},
        log: [{ node: "square", at: "t0" }],
        createdAt: "t0",
        updatedAt: "t0",
      };
  if (parent) {
    base.id = id;
    base.parentId = parent.id;
    base.rootId = parent.rootId || parent.id;
    base.label = "试演";
  }
  return base;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function handle(input: string | URL | Request, init?: RequestInit): Response {
  const url = typeof input === "string" ? input : input.toString();
  const method = init?.method ?? "GET";
  const body = init?.body ? JSON.parse(String(init.body)) : {};
  const parts = url.split("/");

  if (url === "/api/instances" && method === "GET") {
    return jsonResponse({ instances: Object.values(state.instances) });
  }
  if (parts[2] === "stories" && parts.length === 4 && method === "GET") {
    return jsonResponse({
      id: "t",
      title: "测试剧情",
      version: "v1",
      start: "square",
      nodes: mockStory.nodes,
    });
  }
  if (url === "/api/instances" && method === "POST") {
    idCounter += 1;
    const id = body.id || "inst-" + idCounter;
    const inst = newInstance(id);
    state.instances[id] = inst;
    return jsonResponse({ instance: inst, view: viewFor(inst) }, 201);
  }
  if (parts[4] === "forks" && method === "POST") {
    const current = state.instances[parts[3]];
    idCounter += 1;
    const branch = newInstance("branch-" + idCounter, current);
    branch.label = body.label || "试演";
    current.branches = current.branches || {};
    current.branches[branch.id] = {
      id: branch.id,
      label: branch.label as string,
      node: current.current,
      createdAt: "t",
    };
    state.instances[branch.id] = branch;
    return jsonResponse({ instance: branch, view: viewFor(branch) }, 201);
  }
  if (parts[4] === "choices" && method === "POST") {
    const inst = state.instances[parts[3]];
    const node = mockStory.nodes[inst.current as keyof typeof mockStory.nodes];
    const choice = node.choices?.find((c) => c.id === body.choiceId);
    if (!choice) {
      const last = inst.log[inst.log.length - 1];
      if (last && last.choiceId === body.choiceId) {
        return jsonResponse({ error: "duplicate" }, 409);
      }
      return jsonResponse({ error: "unknown choice" }, 400);
    }
    if (choice.condition) {
      const c = choice.condition;
      const ok = c.all ? c.all.every((a) => evalAtom(a, inst.facts)) : evalAtom(c, inst.facts);
      if (!ok) {
        return jsonResponse({ error: "condition failed" }, 409);
      }
    }
    for (const eff of choice.effects ?? []) {
      if (eff.op === "inc") {
        inst.facts[eff.key] = Number(inst.facts[eff.key]) + Number(eff.value);
      }
    }
    inst.rewards.push({
      node: inst.current,
      choiceId: choice.id,
      key: "gold",
      op: "inc",
      value: choice.effects && choice.effects[0] ? choice.effects[0].value : undefined,
      at: "t",
    });
    inst.current = choice.target;
    inst.visitId = "v" + (inst.rev + 1);
    inst.rev += 1;
    inst.log.push({ node: inst.current, choiceId: choice.id, at: "t" });
    return jsonResponse({
      applied: { choiceId: choice.id, node: node.id, rewards: inst.rewards.slice(-1), duplicate: false },
      instance: inst,
      view: viewFor(inst),
    });
  }
  if (parts.length === 4 && parts[2] === "instances" && method === "GET") {
    const inst = state.instances[parts[3]];
    return jsonResponse({ instance: inst, view: viewFor(inst) });
  }
  return jsonResponse({ error: "not found" }, 404);
}

beforeEach(() => {
  state.instances = {};
  idCounter = 0;
  vi.stubGlobal("fetch", vi.fn(handle as unknown as typeof fetch));
});

describe("story play UI", () => {
  it("renders only currently allowed choices and unlocks after facts change", async () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));

    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    expect(screen.queryByText("贿赂卫兵")).toBeNull();

    fireEvent.click(screen.getByText("去干活"));
    await waitFor(() => expect(screen.getByText("领工钱")).toBeTruthy());
    fireEvent.click(screen.getByText("领工钱"));

    await waitFor(() => expect(screen.getByText("贿赂卫兵")).toBeTruthy());
    expect(screen.getByText("gold +5")).toBeTruthy();
  });

  it("rewards once: a stale duplicate request is rejected by the server", async () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    fireEvent.click(screen.getByText("去干活"));
    await waitFor(() => expect(screen.getByText("领工钱")).toBeTruthy());
    fireEvent.click(screen.getByText("领工钱"));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());

    const instId = Object.keys(state.instances)[0];
    const res = handle("/api/instances/" + instId + "/choices", {
      method: "POST",
      body: JSON.stringify({ choiceId: "deliver_wood", idempotencyKey: "x" }),
    } as RequestInit);
    expect(res.status).toBe(409);
    expect(Number(state.instances[instId].facts.gold)).toBe(13);
  });

  it("keeps branch facts and rewards isolated", async () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    fireEvent.click(screen.getByText("去干活"));
    await waitFor(() => expect(screen.getByText("领工钱")).toBeTruthy());

    vi.spyOn(window, "prompt").mockReturnValue("试演：领工钱");
    fireEvent.click(screen.getByRole("button", { name: "建立试演分支" }));

    await waitFor(() => expect(screen.getAllByText(/试演：领工钱/).length).toBeGreaterThan(0));
    fireEvent.click(screen.getByText("领工钱"));
    await waitFor(() =>
      expect(within(document.querySelector(".facts") as HTMLElement).getByText("13")).toBeTruthy(),
    );

    fireEvent.click(screen.getAllByText(/主线/)[0]);
    await waitFor(() => {
      const facts = document.querySelector(".facts") as HTMLElement;
      expect(within(facts).getByText("5")).toBeTruthy();
    });
    expect(screen.getByText("领工钱")).toBeTruthy();
  });

  it("renders the empty archive list as a normal state, not a crash", async () => {
    render(<App />);
    await waitFor(() => expect(screen.getByText("还没有存档。")).toBeTruthy());
    expect(screen.getByRole("button", { name: "开始新故事" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
  });

  it("keeps a retry entry when the archive list genuinely fails to load", async () => {
    let failList = true;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        if (failList && url === "/api/instances" && method === "GET") {
          return Promise.resolve(jsonResponse({ error: "boom" }, 500));
        }
        return Promise.resolve(handle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    await waitFor(() => expect(screen.getByText(/存档列表读取失败/)).toBeTruthy());
    failList = false;
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByText("还没有存档。")).toBeTruthy());
    expect(screen.queryByText(/存档列表读取失败/)).toBeNull();
  });

  it("compares branches from one choice point with a read-only shared prefix", async () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    fireEvent.click(screen.getByText("去干活"));
    await waitFor(() => expect(screen.getByText("领工钱")).toBeTruthy());

    vi.spyOn(window, "prompt").mockReturnValue("试演：对比");
    fireEvent.click(screen.getByRole("button", { name: "建立试演分支" }));
    await waitFor(() => expect(screen.getAllByText(/试演：对比/).length).toBeGreaterThan(0));

    fireEvent.click(screen.getByText("领工钱"));
    await waitFor(() =>
      expect(within(document.querySelector(".facts") as HTMLElement).getByText("13")).toBeTruthy(),
    );

    fireEvent.click(screen.getByRole("button", { name: "比较分支" }));
    await waitFor(() => expect(screen.getByText("共同前缀（只读）")).toBeTruthy());
    const shared = document.querySelector(".compare-shared") as HTMLElement;
    expect(within(shared).getByText(/去干活/)).toBeTruthy();
    expect(shared.textContent).not.toContain("领工钱");
    const cols = document.querySelectorAll(".compare-col");
    expect(cols.length).toBe(2);
    expect(within(cols[0] as HTMLElement).getByText(/试演：对比/)).toBeTruthy();
    expect(within(cols[0] as HTMLElement).getByText(/领工钱/)).toBeTruthy();
    expect((cols[1] as HTMLElement).textContent).not.toContain("领工钱");
  });
});

describe("resilience and rule revision review", () => {
  it("does not white-screen when a fresh instance omits the rewards collection", async () => {
    const baseHandle = handle;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        if (url === "/api/instances" && method === "POST") {
          const res = baseHandle(url, init);
          return res.json().then((body: { instance: Instance; view: NodeView }) => {
            const b = JSON.parse(JSON.stringify(body)) as { instance: Partial<Instance>; view: NodeView };
            delete b.instance.rewards; // older backends omitted empty collections
            return jsonResponse(b, 201);
          });
        }
        return Promise.resolve(baseHandle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    expect(screen.getByText("尚无奖励。")).toBeTruthy();
    expect(screen.getByText("剧情记录")).toBeTruthy();
  });

  it("keeps the selection and offers retry when a save genuinely fails to load", async () => {
    let failGet = true;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        if (failGet && url.startsWith("/api/instances/") && method === "GET") {
          return Promise.resolve(jsonResponse({ error: "disk gone" }, 500));
        }
        return Promise.resolve(handle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText(/存档读取失败/)).toBeTruthy());
    // The failed save stays selected; no new story is started behind the scenes.
    expect(Object.keys(state.instances).length).toBe(1);
    failGet = false;
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByText("去干活")).toBeTruthy());
    expect(screen.queryByText(/存档读取失败/)).toBeNull();
    expect(Object.keys(state.instances).length).toBe(1);
  });

  it("reviews a rule revision and adopts it for this branch only", async () => {
    const impact = {
      storyId: "t",
      fromVersion: "v1",
      toVersion: "v2",
      fromHash: "h",
      toHash: "h2",
      upToDate: false,
      currentNode: "square",
      currentNodeMissing: false,
      completed: [
        { node: "square", choiceId: "chop_wood", status: "revised", detail: "新规则修订了该选项" },
      ],
      future: [{ choiceId: "bribe_guard", text: "贿赂卫兵", change: "condition-changed" }],
    };
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const parts = url.split("/");
        if (parts[4] === "migration" && method === "GET") {
          return Promise.resolve(jsonResponse({ migration: impact }));
        }
        if (parts[4] === "migration" && method === "POST") {
          const inst = state.instances[parts[3]];
          inst.version = "v2";
          inst.defHash = "h2";
          inst.log.push({ node: inst.current, note: "规则迁移：v1 → v2", at: "t9" });
          return Promise.resolve(jsonResponse({ migration: { ...impact, upToDate: true }, instance: inst, view: viewFor(inst), versionDrift: false }));
        }
        if (url.startsWith("/api/instances/") && method === "GET" && parts.length === 4) {
          const inst = state.instances[parts[3]];
          const drift = inst.defHash !== "h2";
          return Promise.resolve(
            jsonResponse({
              instance: inst,
              view: viewFor(inst),
              ...(drift ? { versionDrift: true, latestVersion: "v2", latestDefHash: "h2" } : {}),
            }),
          );
        }
        return Promise.resolve(handle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "开始新故事" }));
    await waitFor(() => expect(screen.getByText(/剧情规则已有新版本/)).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "查看影响" }));
    await waitFor(() => expect(screen.getByText("规则修订影响审查")).toBeTruthy());
    expect(screen.getByText(/规则已修订 · 事件保持原样/)).toBeTruthy();
    expect(screen.getByText("条件变化")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /采用新规则/ }));
    await waitFor(() => expect(screen.getByText(/已采用新规则/)).toBeTruthy());
    expect(screen.queryByText(/剧情规则已有新版本/)).toBeNull();
    // The boundary is explained in the story log.
    expect(screen.getByText(/规则迁移：v1 → v2/)).toBeTruthy();
  });

  it("shows facts and the pending decision before resuming an interrupted adoption", async () => {
    const inst = newInstance("rec-1");
    inst.facts = { gold: 5 };
    inst.pendingMigration = { toHash: "h2", toVersion: "v2", decidedAt: "t8" };
    state.instances["rec-1"] = inst;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const parts = url.split("/");
        if (parts[4] === "migration" && method === "POST") {
          const cur = state.instances[parts[3]];
          cur.pendingMigration = undefined;
          cur.version = "v2";
          cur.defHash = "h2";
          cur.migrationAdopted = "h2";
          cur.log.push({ node: cur.current, note: "规则迁移：v1 → v2", at: "t9" });
          return Promise.resolve(
            jsonResponse({ migration: { upToDate: true }, instance: cur, view: viewFor(cur) }),
          );
        }
        return Promise.resolve(handle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    await waitFor(() => expect(screen.getByText("等待恢复")).toBeTruthy());
    fireEvent.click(screen.getByText("主线"));
    await waitFor(() => expect(screen.getByTestId("recovery-banner")).toBeTruthy());
    // Current facts and the pending decision are shown before continuing.
    expect(screen.getByTestId("recovery-banner").textContent).toContain("v2");
    expect(screen.getByTestId("recovery-banner").textContent).toContain("gold=5");

    fireEvent.click(screen.getByRole("button", { name: "恢复并完成采用" }));
    await waitFor(() => expect(screen.queryByTestId("recovery-banner")).toBeNull());
    expect(state.instances["rec-1"].pendingMigration).toBeUndefined();
    expect(screen.getByText(/规则迁移：v1 → v2/)).toBeTruthy();
    await waitFor(() => expect(screen.getByText("已采用")).toBeTruthy());
  });

  it("distinguishes not-adopted, adopted and awaiting-recovery branches", async () => {
    const adopted = newInstance("a-1");
    adopted.migrationAdopted = "h";
    const plain = newInstance("n-1");
    plain.defHash = "old";
    const pending = newInstance("p-1");
    pending.pendingMigration = { toHash: "h2", toVersion: "v2", decidedAt: "t" };
    state.instances = { "a-1": adopted, "n-1": plain, "p-1": pending };
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown, init?: RequestInit) => {
        const url = String(input);
        if (url === "/api/stories") {
          return Promise.resolve(
            jsonResponse({ stories: [{ id: "t", title: "t", version: "v2", defHash: "h" }] }),
          );
        }
        return Promise.resolve(handle(input as string, init));
      }) as unknown as typeof fetch,
    );
    render(<App />);
    await waitFor(() => expect(screen.getByText("已采用")).toBeTruthy());
    expect(screen.getByText("未采用")).toBeTruthy();
    expect(screen.getByText("等待恢复")).toBeTruthy();
  });
});
