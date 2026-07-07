export type Platform = 'reddit' | 'youtube' | 'social' | 'news' | 'custom';

export type ScoutingFrequency = '3days' | '7days' | '14days';

export interface CustomChannel {
  id: string;
  url: string;
  label: string;
}

export interface Finding {
  id: string;
  platform: Platform;
  quote: string;
  reason: string;
  sourceUrl: string;
  sourceTitle: string;
}

export interface ScoutCycle {
  id: string;
  day: number;
  label: string;
  date: string;
  pros: Finding[];
  cons: Finding[];
}

export interface Idea {
  id: string;
  title: string;
  description: string;
  keywords: string[];
  scoutingFrequencyDays: number;
  channels: Platform[];
  customChannels: CustomChannel[];
  createdAt: string;
  lastUpdated: string;
  totalPros: number;
  totalCons: number;
  newSignalsToday: number;
  status: IdeaStatus;
  statusMessage: string;
  cycles: ScoutCycle[];
}

export type IdeaStatus = 'stable' | 'expanded' | 'pending';

export interface NewIdeaForm {
  title: string;
  description: string;
  keywords: string[];
  scoutingFrequencyDays: number;
  channels: Platform[];
  customChannels: CustomChannel[];
}
