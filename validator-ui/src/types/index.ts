// Domain types mirroring the backend DTOs (internal/api/dto.go). Both sides
// use camelCase so the JSON contract is 1:1.

export type ScoutType = 'PRO' | 'CON';
export type ScoutStatus = 'ACTIVE' | 'PENDING_MUTATION' | 'UNDEPLOYED' | 'STOPPED';
export type IdeaStatus = 'DRAFT' | 'INITIAL_SWEEP' | 'ACTIVE' | 'INACTIVE';
export type ProposalStatus = 'PENDING' | 'APPROVED' | 'REJECTED';

export interface Proposal {
  id: string;
  proposedPrompt: string;
  status: ProposalStatus;
  createdAt: string;
}

export interface Scout {
  id: string;
  scoutType: ScoutType;
  status: ScoutStatus;
  currentPrompt: string;
  pendingProposal?: Proposal;
}

export interface Finding {
  id: string;
  polarity: ScoutType;
  platform: string;
  quote: string;
  reason: string;
  sourceUrl: string;
  sourceTitle: string;
  createdAt: string;
}

export interface IdeaSummary {
  id: string;
  title: string;
  description: string;
  scoutingFrequencyDays: number;
  status: IdeaStatus;
  totalPros: number;
  totalCons: number;
  proScoutStatus: ScoutStatus;
  conScoutStatus: ScoutStatus;
  createdAt: string;
  lastUpdated: string;
}

export interface IdeaDetail extends IdeaSummary {
  scouts: Scout[];
  recentPros: Finding[];
  recentCons: Finding[];
  refinedPrompt: string;
}

export interface NewIdeaForm {
  description: string;
  scoutingFrequencyDays: number;
}

export interface CreateIdeaResponse {
  id: string;
  status: string;
  workflowId: string;
}

export type ProposalAction = 'APPROVE' | 'REJECT';

export interface ProposalResponseRequest {
  action: ProposalAction;
  edited_text?: string;
}

export interface ChatMessage {
  role: string;
  content: string;
  messageType?: string;
  timestamp: string;
}

export interface Conversation {
  id: string;
  title: string;
  status: string;
  conversation: ChatMessage[];
  refinedPrompt: string;
}

export interface ChatResponse {
  message: ChatMessage;
  prompt?: string;
  status: string;
}
