import type { Meta, StoryObj } from '@storybook/react-vite';
import { PromptApprovalDrawer } from './PromptApprovalDrawer';
import { api } from '../lib/api';
import { mockScoutWithProposal } from '../stories/mockData';

const meta = {
  title: 'Components/PromptApprovalDrawer',
  component: PromptApprovalDrawer,
  tags: ['autodocs'],
  args: {
    scout: mockScoutWithProposal,
    ideaId: 'idea-1',
    onClose: () => {},
    onResolved: () => {},
  },
  decorators: [
    (Story) => {
      api.respondProposal = async () => ({ status: 'ok' });
      return <Story />;
    },
  ],
} satisfies Meta<typeof PromptApprovalDrawer>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ConScoutProposal: Story = {};

export const ProScoutProposal: Story = {
  args: {
    scout: {
      ...mockScoutWithProposal,
      scoutType: 'PRO',
      currentPrompt:
        'Track positive sentiment, feature requests, and adoption trends for AI code review tools. Focus on developer satisfaction metrics.',
    },
  },
};
