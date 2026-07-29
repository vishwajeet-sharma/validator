import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryRouter } from 'react-router-dom';
import { NewIdeaForm } from './NewIdeaForm';
import { api } from '../lib/api';

const meta = {
  title: 'Pages/NewIdeaForm',
  component: NewIdeaForm,
  tags: ['autodocs'],
  decorators: [
    (Story) => {
      api.createIdea = async () => ({ id: 'new-idea', status: 'created', workflowId: 'wf-1' });
      return (
        <MemoryRouter>
          <Story />
        </MemoryRouter>
      );
    },
  ],
} satisfies Meta<typeof NewIdeaForm>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
