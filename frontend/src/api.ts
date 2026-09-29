import type { Definition, Save, SaveSummary, StorySummary, View } from "./types";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    let message = res.status + " " + res.statusText;
    try {
      const body = await res.json();
      if (body.error) message = body.error;
    } catch {
      /* non-json error body */
    }
    throw new Error(message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

const storyPath = (id: string) => "/api/stories/" + encodeURIComponent(id);

export const api = {
  listStories: () => req<StorySummary[]>("/api/stories"),
  getStory: (id: string) => req<Definition>(storyPath(id)),
  putStory: (id: string, d: Definition) =>
    req<Definition>(storyPath(id), {
      method: "PUT",
      body: JSON.stringify(d),
    }),
  start: (id: string, saveId: string) =>
    req<{ saveId: string; save: Save; view: View }>(storyPath(id) + "/start", {
      method: "POST",
      body: JSON.stringify({ saveId }),
    }),
  choose: (id: string, saveId: string, choice: string) =>
    req<{ save: Save; view: View }>(storyPath(id) + "/choose", {
      method: "POST",
      body: JSON.stringify({ saveId, choice }),
    }),
  resume: (id: string, saveId: string) =>
    req<{ save: Save; view: View }>(
      storyPath(id) + "/saves/" + encodeURIComponent(saveId) + "/resume",
      { method: "POST" },
    ),
  listSaves: () => req<SaveSummary[]>("/api/saves"),
  deleteSave: (saveId: string) =>
    req<void>("/api/saves/" + encodeURIComponent(saveId), { method: "DELETE" }),
};
