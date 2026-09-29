import { create } from "zustand";
import { api } from "./api";
import type {
  Definition,
  Save,
  SaveSummary,
  StorySummary,
  View,
} from "./types";

interface PlayState {
  saveId: string;
  save: Save | null;
  view: View | null;
}

interface AppState {
  stories: StorySummary[];
  definition: Definition | null;
  loadError: string | null;
  play: PlayState;
  saves: SaveSummary[];
  busy: boolean;

  refreshStories: () => Promise<void>;
  selectStory: (id: string) => Promise<void>;
  saveDefinition: (d: Definition) => Promise<void>;
  updateDraft: (d: Definition) => void;
  startPlay: (saveId: string) => Promise<void>;
  choose: (choiceId: string) => Promise<void>;
  resume: (saveId: string) => Promise<void>;
  refreshSaves: () => Promise<void>;
  deleteSave: (saveId: string) => Promise<void>;
}

export const useStore = create<AppState>((set, get) => ({
  stories: [],
  definition: null,
  loadError: null,
  play: { saveId: "autosave", save: null, view: null },
  saves: [],
  busy: false,

  refreshStories: async () => {
    const stories = await api.listStories();
    set({ stories });
  },

  selectStory: async (id) => {
    set({ busy: true, loadError: null });
    try {
      const definition = await api.getStory(id);
      set({
        definition,
        play: { saveId: "autosave", save: null, view: null },
      });
      await get().refreshSaves();
    } catch (e) {
      set({ loadError: (e as Error).message });
    } finally {
      set({ busy: false });
    }
  },

  saveDefinition: async (d) => {
    set({ busy: true, loadError: null });
    try {
      const saved = await api.putStory(d.id, d);
      set({ definition: saved });
      await get().refreshStories();
    } finally {
      set({ busy: false });
    }
  },

  updateDraft: (d) => set({ definition: d, loadError: null }),

  startPlay: async (saveId) => {
    const { definition } = get();
    if (!definition) return;
    set({ busy: true, loadError: null });
    try {
      const r = await api.start(definition.id, saveId);
      set({ play: { saveId: r.saveId, save: r.save, view: r.view } });
      await get().refreshSaves();
    } catch (e) {
      set({ loadError: (e as Error).message });
    } finally {
      set({ busy: false });
    }
  },

  choose: async (choiceId) => {
    const { definition, play } = get();
    if (!definition) return;
    set({ busy: true, loadError: null });
    try {
      const r = await api.choose(definition.id, play.saveId, choiceId);
      set({ play: { ...play, save: r.save, view: r.view } });
    } catch (e) {
      set({ loadError: (e as Error).message });
    } finally {
      set({ busy: false });
    }
  },

  resume: async (saveId) => {
    const { definition } = get();
    if (!definition) return;
    set({ busy: true, loadError: null });
    try {
      const r = await api.resume(definition.id, saveId);
      set({ play: { saveId, save: r.save, view: r.view } });
    } catch (e) {
      set({
        loadError: (e as Error).message,
        play: { saveId, save: null, view: null },
      });
    } finally {
      set({ busy: false });
    }
  },

  refreshSaves: async () => {
    try {
      set({ saves: await api.listSaves() });
    } catch {
      /* saves are optional */
    }
  },

  deleteSave: async (saveId) => {
    await api.deleteSave(saveId);
    if (get().play.saveId === saveId) {
      set({ play: { saveId, save: null, view: null } });
    }
    await get().refreshSaves();
  },
}));
