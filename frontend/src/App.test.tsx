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
});
