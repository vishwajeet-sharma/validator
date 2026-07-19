import type {
  CreateIdeaResponse,
  IdeaDetail,
  IdeaSummary,
  NewIdeaForm,
  ProposalResponseRequest,
} from '../types';

// Base URL for the Validator backend. In dev we leave this empty so requests are
// relative and the Vite dev-server proxy (see vite.config.ts) forwards them to
// the API surface. For production builds, set VITE_API_BASE to the backend origin.
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

export const api = {
  listIdeas: () => request<IdeaSummary[]>('/api/ideas'),

  getIdea: (id: string) => request<IdeaDetail>(`/api/ideas/${encodeURIComponent(id)}`),

  createIdea: (form: NewIdeaForm) =>
    request<CreateIdeaResponse>('/api/ideas', {
      method: 'POST',
      body: JSON.stringify(form),
    }),

  respondProposal: (proposalId: string, body: ProposalResponseRequest) =>
    request<{ status: string }>(`/api/proposals/${encodeURIComponent(proposalId)}/respond`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  stopScout: (scoutId: string) =>
    request<{ status: string }>(`/api/scouts/${encodeURIComponent(scoutId)}`, {
      method: 'DELETE',
    }),
};
