import type { ChoiceResult, Instance, NodeView } from "./types";

export interface InstanceResponse {
  instance: Instance;
  view?: NodeView;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = (body as { error?: string }).error ?? "HTTP " + res.status;
    throw new Error(msg);
  }
  return body as T;
}

export const api = {
  listInstances: () => request<{ instances: Instance[] }>("/api/instances"),
  start: (storyId: string, facts: Record<string, number>) =>
    request<InstanceResponse>("/api/instances", {
      method: "POST",
      body: JSON.stringify({ storyId, facts }),
    }),
  get: (id: string) => request<InstanceResponse>("/api/instances/" + id),
  view: (id: string) =>
    request<{ instance: Instance; view: NodeView }>("/api/instances/" + id + "/view"),
  choose: (id: string, choiceId: string, requestId: string) =>
    request<{ instance: Instance; result: ChoiceResult }>(
      "/api/instances/" + id + "/choices",
      {
        method: "POST",
        body: JSON.stringify({ choiceId, requestId }),
      },
    ),
  fork: (id: string, name: string) =>
    request<{ instance: Instance; parentId: string }>(
      "/api/instances/" + id + "/fork",
      { method: "POST", body: JSON.stringify({ name }) },
    ),
};

export function newRequestId(): string {
  const cryptoObj = globalThis.crypto;
  if (cryptoObj && "randomUUID" in cryptoObj) {
    return cryptoObj.randomUUID();
  }
  return "req_" + Date.now() + "_" + Math.random().toString(36).slice(2);
}
