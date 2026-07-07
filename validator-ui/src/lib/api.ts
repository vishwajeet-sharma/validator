import type { Idea, NewIdeaForm } from '../types';

// Base URL for the Validator backend. In dev we leave this empty so requests are
// relative and the Vite dev-server proxy (see vite.config.ts) forwards them to
// :8000. For production builds, set VITE_API_BASE to the backend origin.
const BASE = (import.meta.env.VITE_API_BASE ?? '').replace(/\/$/, '');

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(BASE + path, {
      headers: { 'Content-Type': 'application/json' },
      ...init,
    });
  } catch {
    throw new ApiError(0, `Unable to reach the Validator backend at ${BASE || '(relative)'}. Is it running?`);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      // fall through with parsed = null
    }
  }

  if (!res.ok) {
    let message = `Request failed with status ${res.status}`;
    if (
      parsed &&
      typeof parsed === 'object' &&
      'error' in parsed &&
      typeof (parsed as { error: unknown }).error === 'string'
    ) {
      message = (parsed as { error: string }).error;
    }
    throw new ApiError(res.status, message);
  }

  return parsed as T;
}

export interface CreateIdeaResponse {
  idea: Idea;
  workflow_id: string;
  invocation_id?: string;
}

export const api = {
  listIdeas: () => request<Idea[]>('/api/ideas'),

  getIdea: (id: string) => request<Idea>(`/api/ideas/${encodeURIComponent(id)}`),

  createIdea: (form: NewIdeaForm) =>
    request<CreateIdeaResponse>('/api/ideas', {
      method: 'POST',
      body: JSON.stringify(form),
    }),

  getPayload: (id: string) =>
    request<{ payload: string }>(`/api/ideas/${encodeURIComponent(id)}/payload`),
};
