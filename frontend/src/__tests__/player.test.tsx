import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { App } from "../App";
import type { Definition, View } from "../types";

const definition: Definition = {
  id: "d",
  version: 1,
  title: "测试剧",
  start: "gate",
  nodes: {
    gate: {
      id: "gate",
      title: "门",
      text: "gold=5",
      choices: [
        {
          id: "rich",
          text: "富人通道",
          next: "city",
          when: { key: "gold", op: "ge", val: 5 },
        },
        {
          id: "poor",
          text: "穷人通道",
          next: "alley",
          when: { key: "gold", op: "lt", val: 5 },
        },
      ],
    },
    city: { id: "city", title: "城", text: "完", terminal: true },
    alley: { id: "alley", title: "巷子", text: "绕路", choices: [] },
  },
};

const gateView: View = {
  nodeId: "gate",
  title: "门",
  text: "gold=5",
  terminal: false,
  choices: [{ id: "rich", text: "富人通道", next: "city" }],
  state: { gold: 5 },
  visits: 1,
};

const endView: View = {
  nodeId: "city",
  title: "城",
  text: "完",
  terminal: true,
  choices: [],
  state: { gold: 5 },
  visits: 1,
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

describe("Player choice consistency", () => {
  beforeEach(() => {
    const postBodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        if (method === "GET" && url.endsWith("/api/stories")) {
          return json([{ id: "d", title: "测试剧", version: 1 }]);
        }
        if (method === "GET" && url.endsWith("/api/stories/d")) return json(definition);
        if (method === "GET" && url.endsWith("/api/saves")) return json([]);
        if (method === "POST" && url.endsWith("/start")) {
          return json({
            saveId: "autosave",
            save: { storyId: "d", version: 1, current: "gate" },
            view: gateView,
          });
        }
        if (method === "POST" && url.endsWith("/choose")) {
          postBodies.push(JSON.parse(String(init?.body ?? "{}")));
          const body = postBodies.at(-1) as { choice: string };
          if (body.choice !== "rich") return json({ error: "choice not allowed" }, 400);
          return json({
            save: { storyId: "d", version: 1, current: "city" },
            view: endView,
          });
        }
        return json({ error: "unmapped " + method + " " + url }, 404);
      }),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it("shows only runner-allowed choices and posts that exact id", async () => {
    render(<App />);
    const start = await screen.findByRole("button", { name: /新开一局/ });
    fireEvent.click(start);

    await waitFor(() => expect(screen.getByText("富人通道")).toBeTruthy());
    expect(screen.queryByText("穷人通道")).toBeNull();

    fireEvent.click(screen.getByText(/富人通道/));
    await waitFor(() => expect(screen.getByText("剧情结束")).toBeTruthy());
  });
});
