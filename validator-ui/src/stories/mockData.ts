import type { IdeaSummary, IdeaDetail, Scout, Finding } from '../types';

export const mockProFindings: Finding[] = [
  {
    id: 'f1',
    polarity: 'PRO',
    platform: 'reddit',
    quote:
      "This tool replaced our entire QA review process. We catch 80% of bugs before they hit staging now.",
    reason:
      'Users consistently report significant time savings and quality improvements in code review workflows.',
    sourceUrl: 'https://reddit.com/r/devops/comments/abc123',
    sourceTitle: 'r/devops — Real world experience',
    createdAt: new Date(Date.now() - 2 * 3600000).toISOString(),
  },
  {
    id: 'f2',
    polarity: 'PRO',
    platform: 'news',
    quote:
      'The automated PR review market is projected to grow 40% YoY as engineering teams prioritise shipping velocity.',
    reason:
      'Market growth signals strong demand trajectory and validates the timing of this product.',
    sourceUrl: 'https://techcrunch.com/2024/ai-pr-review',
    sourceTitle: 'TechCrunch — Engineering Tools 2024',
    createdAt: new Date(Date.now() - 8 * 3600000).toISOString(),
  },
  {
    id: 'f3',
    polarity: 'PRO',
    platform: 'youtube',
    quote:
      "I demoed five different AI code review tools and this one had the lowest false-positive rate by far.",
    reason:
      'Low false-positive rate is a key differentiator that drives retention and word-of-mouth referrals.',
    sourceUrl: 'https://youtube.com/watch?v=abc',
    sourceTitle: 'DevTools Review Channel',
    createdAt: new Date(Date.now() - 26 * 3600000).toISOString(),
  },
];

export const mockConFindings: Finding[] = [
  {
    id: 'f4',
    polarity: 'CON',
    platform: 'social',
    quote:
      "The pricing is insane for small teams. $49/user/month when we only need it for 2 PRs a week?",
    reason:
      'Pricing friction among small teams indicates a potential churn risk and limits the addressable market.',
    sourceUrl: 'https://twitter.com/dev/status/123',
    sourceTitle: '@indiedev — Thread',
    createdAt: new Date(Date.now() - 3 * 3600000).toISOString(),
  },
  {
    id: 'f5',
    polarity: 'CON',
    platform: 'reddit',
    quote:
      'GitHub Copilot is adding native PR reviews for free. Hard to justify a separate tool when the platform bundles it.',
    reason:
      'Platform bundling by GitHub/Microsoft poses a existential competitive threat to standalone tools.',
    sourceUrl: 'https://reddit.com/r/programming/comments/xyz',
    sourceTitle: 'r/programming — Discussion',
    createdAt: new Date(Date.now() - 12 * 3600000).toISOString(),
  },
];

export const mockScouts: Scout[] = [
  {
    id: 'scout-pro-1',
    scoutType: 'PRO',
    status: 'ACTIVE',
    currentPrompt:
      'Track discussions, reviews, and mentions about AI-powered code review tools. Focus on teams using automated PR reviews, their satisfaction levels, and feature requests.',
  },
  {
    id: 'scout-con-1',
    scoutType: 'CON',
    status: 'PENDING_MUTATION',
    currentPrompt:
      'Monitor criticism, complaints, and competitive threats to AI code review tools. Track pricing objections, competitor launches, and negative reviews.',
    pendingProposal: {
      id: 'prop-1',
      proposedPrompt:
        'Expand competitive monitoring to include GitHub Copilot native features, CodeRabbit, and Cursor. Track developer sentiment on bundled vs standalone review tools. Monitor enterprise procurement decisions and vendor consolidation trends.',
      status: 'PENDING',
      createdAt: new Date(Date.now() - 1 * 3600000).toISOString(),
    },
  },
];

export const mockIdeas: IdeaSummary[] = [
  {
    id: 'idea-1',
    title: 'AI-Powered Code Review for Engineering Teams',
    description:
      'A SaaS that automatically reviews pull requests using LLMs, catching bugs and suggesting improvements before merge.',
    scoutingFrequencyDays: 3,
    status: 'ACTIVE',
    totalPros: 15,
    totalCons: 8,
    proScoutStatus: 'ACTIVE',
    conScoutStatus: 'PENDING_MUTATION',
    createdAt: new Date(Date.now() - 14 * 86400000).toISOString(),
    lastUpdated: new Date(Date.now() - 2 * 3600000).toISOString(),
  },
  {
    id: 'idea-2',
    title: 'No-Code Analytics Dashboard for E-commerce',
    description:
      'A drag-and-drop analytics platform that lets non-technical store owners build custom dashboards without SQL.',
    scoutingFrequencyDays: 7,
    status: 'INITIAL_SWEEP',
    totalPros: 0,
    totalCons: 0,
    proScoutStatus: 'UNDEPLOYED',
    conScoutStatus: 'UNDEPLOYED',
    createdAt: new Date(Date.now() - 1 * 3600000).toISOString(),
    lastUpdated: new Date(Date.now() - 30 * 60000).toISOString(),
  },
  {
    id: 'idea-3',
    title: 'Voice-First CRM for Field Sales Teams',
    description:
      'A mobile CRM optimised for voice input so sales reps can log calls and update pipeline hands-free while driving.',
    scoutingFrequencyDays: 7,
    status: 'ACTIVE',
    totalPros: 23,
    totalCons: 5,
    proScoutStatus: 'ACTIVE',
    conScoutStatus: 'ACTIVE',
    createdAt: new Date(Date.now() - 30 * 86400000).toISOString(),
    lastUpdated: new Date(Date.now() - 5 * 86400000).toISOString(),
  },
  {
    id: 'idea-4',
    title: 'Blockchain-Based Customer Loyalty Platform',
    description:
      'A decentralised loyalty points system that lets customers trade rewards across participating brands.',
    scoutingFrequencyDays: 14,
    status: 'ACTIVE',
    totalPros: 4,
    totalCons: 19,
    proScoutStatus: 'ACTIVE',
    conScoutStatus: 'ACTIVE',
    createdAt: new Date(Date.now() - 45 * 86400000).toISOString(),
    lastUpdated: new Date(Date.now() - 10 * 86400000).toISOString(),
  },
];

export const mockIdeaDetail: IdeaDetail = {
  ...mockIdeas[0],
  scouts: mockScouts,
  recentPros: mockProFindings,
  recentCons: mockConFindings,
};

export const mockScoutWithProposal: Scout = mockScouts[1];
