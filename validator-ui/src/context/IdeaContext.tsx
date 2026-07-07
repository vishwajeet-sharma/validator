import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react';
import { Idea, NewIdeaForm } from '../types';
import { api, ApiError } from '../lib/api';

interface IdeaContextType {
  ideas: Idea[];
  selectedIdea: Idea | null;
  loading: boolean;
  error: string | null;
  selectIdea: (id: string | null) => void;
  addIdea: (form: NewIdeaForm) => Promise<Idea>;
  refreshIdeas: () => Promise<void>;
}

const IdeaContext = createContext<IdeaContextType | undefined>(undefined);

export function IdeaProvider({ children }: { children: ReactNode }) {
  const [ideas, setIdeas] = useState<Idea[]>([]);
  const [selectedIdea, setSelectedIdea] = useState<Idea | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refreshIdeas = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.listIdeas();
      setIdeas(data);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load ideas.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refreshIdeas();
  }, [refreshIdeas]);

  const selectIdea = useCallback((id: string | null) => {
    if (!id) {
      setSelectedIdea(null);
      return;
    }
    const idea = ideas.find((i) => i.id === id);
    setSelectedIdea(idea || null);
  }, [ideas]);

  const addIdea = useCallback(async (form: NewIdeaForm) => {
    const { idea } = await api.createIdea(form);
    // Refresh so the list (and rollup counters) reflects server state.
    await refreshIdeas();
    return idea;
  }, [refreshIdeas]);

  return (
    <IdeaContext.Provider
      value={{ ideas, selectedIdea, loading, error, selectIdea, addIdea, refreshIdeas }}
    >
      {children}
    </IdeaContext.Provider>
  );
}

export function useIdeas() {
  const context = useContext(IdeaContext);
  if (!context) {
    throw new Error('useIdeas must be used within an IdeaProvider');
  }
  return context;
}
