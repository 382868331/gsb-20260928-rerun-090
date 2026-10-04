import type {
  AppliedResponse,
  InstanceResponse,
  MigrationImpact,
  MigrationResponse,
  StoryDefinition,
} from "./types";

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

export function getStory(id: string): Promise<StoryDefinition> {
  return request(`/api/stories/${encodeURIComponent(id)}`);
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

export function getMigration(id: string): Promise<{ migration: MigrationImpact }> {
  return request(`/api/instances/${encodeURIComponent(id)}/migration`);
}

export function decideMigration(
  id: string,
	action: "adopt" | "defer" | "cancel",
	phase?: "begin" | "complete" | "complete-and-commit",
): Promise<MigrationResponse> {
  return request(`/api/instances/${encodeURIComponent(id)}/migration`, {
    method: "POST",
		body: JSON.stringify({ action, phase }),
  });
}

export function withdrawRevision(
  storyId: string,
  hash: string,
): Promise<{ withdrawn?: string; error?: string; retained?: boolean; pinnedBy?: string[]; pendingBy?: string[]; historyOf?: string[] }> {
  return request(`/api/stories/${encodeURIComponent(storyId)}/revisions/${encodeURIComponent(hash)}`, {
    method: "DELETE",
  });
}

export interface RevisionSummary {
  version: string;
  hash: string;
  current: boolean;
  withdrawable?: boolean;
  retained?: boolean;
  reasons?: string[];
}

export function listRevisions(storyId: string): Promise<{ revisions: RevisionSummary[] }> {
  return request(`/api/stories/${encodeURIComponent(storyId)}/revisions`);
}
