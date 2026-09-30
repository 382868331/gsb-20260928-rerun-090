import type { AppliedResponse, InstanceResponse } from "./types";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const text = await res.text();
  let body: unknown = null;
  if (text) {
    body = JSON.parse(text);
  }
  if (!res.ok) {
    const message =
      (body as { error?: string } | null)?.error ?? `HTTP ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

export function createInstance(id?: string): Promise<InstanceResponse> {
  return request("/api/instances", {
    method: "POST",
    body: JSON.stringify(id ? { id } : {}),
  });
}

export function getInstance(id: string): Promise<InstanceResponse> {
  return request(`/api/instances/${encodeURIComponent(id)}`);
}

export function listInstances(): Promise<{ instances: import("./types").Instance[] }> {
  return request("/api/instances");
}

let keyCounter = 0;

export function newIdempotencyKey(): string {
  keyCounter += 1;
  return `key-${Date.now()}-${keyCounter}-${Math.random().toString(36).slice(2, 8)}`;
}

export function submitChoice(
  id: string,
  choiceId: string,
  idempotencyKey: string,
): Promise<AppliedResponse> {
  return request(`/api/instances/${encodeURIComponent(id)}/choices`, {
    method: "POST",
    body: JSON.stringify({ choiceId, idempotencyKey }),
  });
}

export function forkInstance(
  id: string,
  label: string,
): Promise<InstanceResponse> {
  return request(`/api/instances/${encodeURIComponent(id)}/forks`, {
    method: "POST",
    body: JSON.stringify({ label }),
  });
}
