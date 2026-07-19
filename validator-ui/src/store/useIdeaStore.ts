import { create } from 'zustand';
import type { IdeaSummary, NewIdeaForm } from '../types';
import { api, ApiError } from '../lib/api';

// useIdeaStore is the Zustand state engine for the dashboard list and idea
// onboarding. The detail board fetches its own idea (with polling) since it
// needs the heavy scout/signal payload.
interface IdeaStoreState {
  ideas: IdeaSummary[];
  loading: boolean;
  error: string | null;
  refreshIdeas: () => Promise<void>;
  createIdea: (form: NewIdeaForm) => Promise<string>;
}

export const useIdeaStore = create<IdeaStoreState>((set, get) => ({
  ideas: [],
  loading: true,
  error: null,

  refreshIdeas: async () => {
    try {
      set({ loading: true, error: null });
      const data = await api.listIdeas();
      set({ ideas: data, loading: false });
    } catch (err) {
      set({
        error: err instanceof ApiError ? err.message : 'Failed to load ideas.',
        loading: false,
      });
    }
  },

  createIdea: async (form: NewIdeaForm) => {
    const res = await api.createIdea(form);
    // Refresh so the dashboard reflects the new idea immediately.
    await get().refreshIdeas();
    return res.id;
  },
}));
