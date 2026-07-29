import type { Meta, StoryObj } from '@storybook/react-vite';
import { FindingCard } from './FindingCard';
import { Finding } from '../types';

const proReddit: Finding = {
  id: '1',
  polarity: 'PRO',
  platform: 'reddit',
  quote:
    "After three months of using this for our team's project management, the automation features alone have saved us 10+ hours per week. The learning curve is minimal.",
  reason:
    'Users consistently praise the time-saving automation, indicating strong product-market fit for productivity-focused teams.',
  sourceUrl: 'https://reddit.com/r/productivity/comments/example',
  sourceTitle: 'r/productivity discussion',
  createdAt: '2024-01-15T10:30:00Z',
};

const conYouTube: Finding = {
  id: '2',
  polarity: 'CON',
  platform: 'youtube',
  quote:
    "The pricing model is way too aggressive for small teams. At $50/month per user, it's nearly impossible to justify when there are free alternatives.",
  reason:
    'Price sensitivity is a recurring concern, particularly among smaller teams who may churn to free competitors.',
  sourceUrl: 'https://youtube.com/watch?v=example',
  sourceTitle: 'Tech Review Channel',
  createdAt: '2024-01-14T15:00:00Z',
};

const proNews: Finding = {
  id: '3',
  polarity: 'PRO',
  platform: 'news',
  quote:
    'This tool was featured in our annual roundup as one of the most innovative products of the year, praised for its intuitive design.',
  reason:
    'Media coverage validates market awareness and credibility, driving organic discovery.',
  sourceUrl: 'https://example.com/article',
  sourceTitle: 'TechCrunch',
  createdAt: '2024-01-13T08:00:00Z',
};

const conSocial: Finding = {
  id: '4',
  polarity: 'CON',
  platform: 'social',
  quote:
    "Honestly the customer support is non-existent. Been waiting 2 weeks for a response and still nothing.",
  reason:
    'Poor customer experience risks negative word-of-mouth and high churn rates.',
  sourceUrl: 'https://instagram.com/p/example',
  sourceTitle: '@userreview',
  createdAt: '2024-01-12T18:45:00Z',
};

const meta = {
  title: 'Components/FindingCard',
  component: FindingCard,
  tags: ['autodocs'],
  args: {
    finding: proReddit,
    type: 'pro' as const,
  },
  render: (args) => (
    <div className="max-w-md">
      <FindingCard {...args} />
    </div>
  ),
} satisfies Meta<typeof FindingCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ProReddit: Story = {};

export const ProNews: Story = {
  args: { finding: proNews },
};

export const ConYouTube: Story = {
  args: { finding: conYouTube, type: 'con' as const },
};

export const ConSocial: Story = {
  args: { finding: conSocial, type: 'con' as const },
};
