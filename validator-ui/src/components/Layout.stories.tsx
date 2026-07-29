import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryRouter } from 'react-router-dom';
import { Layout } from './Layout';

const meta = {
  title: 'Components/Layout',
  component: Layout,
  tags: ['autodocs'],
  args: {
    children: (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <h1 className="text-3xl font-bold tracking-tight text-zinc-100 mb-2">
          Sample Page Content
        </h1>
        <p className="text-zinc-400">
          This is the main content area wrapped by the Layout component — top nav bar
          with logo, navigation links, and theme toggle.
        </p>
      </div>
    ),
  },
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof Layout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
