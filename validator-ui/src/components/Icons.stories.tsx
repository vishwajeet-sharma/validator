import type { Meta, StoryObj } from '@storybook/react-vite';
import { ValidatorLogo, InstagramIcon, TwitterIcon } from './Icons';

const meta = {
  title: 'Components/Icons',
  tags: ['autodocs'],
} satisfies Meta;

export default meta;
type Story = StoryObj;

export const CustomSVGs: Story = {
  render: () => (
    <div className="flex flex-wrap gap-10 p-8">
      <div className="flex flex-col items-center gap-3">
        <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 flex items-center justify-center">
          <ValidatorLogo className="w-9 h-9 text-white" />
        </div>
        <span className="text-xs text-zinc-400">ValidatorLogo</span>
      </div>
      <div className="flex flex-col items-center gap-3">
        <div className="w-16 h-16 rounded-2xl bg-pink-500/10 flex items-center justify-center">
          <InstagramIcon className="w-8 h-8 text-pink-400" />
        </div>
        <span className="text-xs text-zinc-400">InstagramIcon</span>
      </div>
      <div className="flex flex-col items-center gap-3">
        <div className="w-16 h-16 rounded-2xl bg-sky-500/10 flex items-center justify-center">
          <TwitterIcon className="w-8 h-8 text-sky-400" />
        </div>
        <span className="text-xs text-zinc-400">TwitterIcon</span>
      </div>
    </div>
  ),
};
