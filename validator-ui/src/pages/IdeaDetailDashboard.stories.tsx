import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { IdeaDetailDashboard } from './IdeaDetailDashboard';
import { api } from '../lib/api';
import { mockIdeaDetail } from '../stories/mockData';

const meta = {
  title: 'Pages/IdeaDetailDashboard',
  component: IdeaDetailDashboard,
  tags: ['autodocs'],
  decorators: [
    (Story) => {
      api.getIdea = async () => mockIdeaDetail;
      api.stopScout = async () => ({ status: 'stopped' });
      api.deactivateIdea = async () => ({ status: 'inactive' });
      window.confirm = () => false;
      return (
        <MemoryRouter initialEntries={['/idea/idea-1']}>
          <Routes>
            <Route path="/idea/:id" element={<Story />} />
          </Routes>
        </MemoryRouter>
      );
    },
  ],
} satisfies Meta<typeof IdeaDetailDashboard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ActiveWithPendingProposal: Story = {};

export const NotFound: Story = {
  decorators: [
    (Story) => {
      api.getIdea = async () => {
        throw new Error('not found');
      };
      return (
        <MemoryRouter initialEntries={['/idea/nonexistent']}>
          <Routes>
            <Route path="/idea/:id" element={<Story />} />
          </Routes>
        </MemoryRouter>
      );
    },
  ],
};
