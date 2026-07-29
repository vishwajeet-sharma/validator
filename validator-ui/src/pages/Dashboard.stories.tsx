import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryRouter } from 'react-router-dom';
import { Dashboard } from './Dashboard';
import { useIdeaStore } from '../store/useIdeaStore';
import { api, ApiError } from '../lib/api';
import { mockIdeas } from '../stories/mockData';

const meta = {
  title: 'Pages/Dashboard',
  component: Dashboard,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof Dashboard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Populated: Story = {
  decorators: [
    (Story) => {
      api.listIdeas = async () => mockIdeas;
      useIdeaStore.setState({ ideas: mockIdeas, loading: false, error: null });
      return <Story />;
    },
  ],
};

export const Loading: Story = {
  decorators: [
    (Story) => {
      api.listIdeas = async () => new Promise(() => {});
      useIdeaStore.setState({ ideas: [], loading: true, error: null });
      return <Story />;
    },
  ],
};

export const Empty: Story = {
  decorators: [
    (Story) => {
      api.listIdeas = async () => [];
      useIdeaStore.setState({ ideas: [], loading: false, error: null });
      return <Story />;
    },
  ],
};

export const ErrorState: Story = {
  decorators: [
    (Story) => {
      api.listIdeas = async () => {
        throw new ApiError(500, 'Failed to load ideas. Check your connection.');
      };
      useIdeaStore.setState({
        ideas: [],
        loading: false,
        error: 'Failed to load ideas. Check your connection.',
      });
      return <Story />;
    },
  ],
};
