import { Finding } from '../types';
import { useTheme } from '../context/ThemeContext';
import { ExternalLink, MessageSquare, Youtube, Newspaper, InstagramIcon, Globe } from '../components/Icons';

interface FindingCardProps {
  finding: Finding;
  type: 'pro' | 'con';
}

const platformIcons: Record<string, React.ElementType> = {
  reddit: MessageSquare,
  youtube: Youtube,
  news: Newspaper,
  social: InstagramIcon,
  custom: Globe,
};

const platformColors = {
  reddit: {
    dark: 'text-orange-400 bg-orange-500/10',
    light: 'text-orange-600 bg-orange-100',
  },
  youtube: {
    dark: 'text-red-400 bg-red-500/10',
    light: 'text-red-600 bg-red-100',
  },
  news: {
    dark: 'text-blue-400 bg-blue-500/10',
    light: 'text-blue-600 bg-blue-100',
  },
  social: {
    dark: 'text-pink-400 bg-pink-500/10',
    light: 'text-pink-600 bg-pink-100',
  },
  custom: {
    dark: 'text-violet-400 bg-violet-500/10',
    light: 'text-violet-600 bg-violet-100',
  },
};

const platformNames: Record<string, string> = {
  reddit: 'Reddit',
  youtube: 'YouTube',
  news: 'News Article',
  social: 'Social Media',
  custom: 'Custom Source',
};

export function FindingCard({ finding, type }: FindingCardProps) {
  const { theme } = useTheme();
  const PlatformIcon = platformIcons[finding.platform] || MessageSquare;
  const colors = platformColors[finding.platform] || platformColors.social;
  const isPro = type === 'pro';

  const accentColors = {
    pro: {
      border: theme === 'dark' ? 'border-emerald-500/30' : 'border-emerald-300',
      badge: theme === 'dark' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-emerald-100 text-emerald-700',
      quote: theme === 'dark' ? 'text-emerald-100' : 'text-zinc-800',
      hover: theme === 'dark' ? 'hover:border-emerald-500/50' : 'hover:border-emerald-400',
    },
    con: {
      border: theme === 'dark' ? 'border-rose-500/30' : 'border-rose-300',
      badge: theme === 'dark' ? 'bg-rose-500/10 text-rose-400' : 'bg-rose-100 text-rose-700',
      quote: theme === 'dark' ? 'text-rose-100' : 'text-zinc-800',
      hover: theme === 'dark' ? 'hover:border-rose-500/50' : 'hover:border-rose-400',
    },
  };

  const style = accentColors[type];

  return (
    <div className={`group p-4 rounded-xl border transition-all duration-300 ${
      theme === 'dark'
        ? `bg-zinc-900/50 ${style.border} ${style.hover}`
        : `bg-white ${style.border} ${style.hover}`
    }`}>
      <div className="flex items-start gap-3 mb-3">
        <div className={`p-2 rounded-lg ${
          theme === 'dark' ? colors.dark : colors.light
        }`}>
          <PlatformIcon className="w-4 h-4" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className={`text-xs font-semibold uppercase tracking-wide ${
              isPro
                ? theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
                : theme === 'dark' ? 'text-rose-400' : 'text-rose-600'
            }`}>
              {isPro ? 'PRO' : 'CON'}
            </span>
            <span className={`text-xs ${
              theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'
            }`}>
              via {platformNames[finding.platform]}
            </span>
          </div>
        </div>
      </div>

      <blockquote className={`text-sm leading-relaxed mb-3 ${
        isPro ? style.quote : style.quote
      }`}>
        "{finding.quote}"
      </blockquote>

      <div className={`flex items-start gap-2 mb-3 p-2 rounded-lg ${
        theme === 'dark' ? 'bg-zinc-800/50' : 'bg-zinc-50'
      }`}>
        <span className={`text-xs font-medium ${
          isPro
            ? theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
            : theme === 'dark' ? 'text-rose-400' : 'text-rose-600'
        }`}>
          Why it{isPro ? "'s a Pro" : "'s a Con"}:
        </span>
        <span className={`text-xs ${
          theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
        }`}>
          {finding.reason}
        </span>
      </div>

      <a
        href={finding.sourceUrl}
        target="_blank"
        rel="noopener noreferrer"
        className={`inline-flex items-center gap-1.5 text-xs font-medium transition-all duration-200 ${
          isPro
            ? theme === 'dark'
              ? 'text-emerald-400 hover:text-emerald-300'
              : 'text-emerald-600 hover:text-emerald-500'
            : theme === 'dark'
              ? 'text-rose-400 hover:text-rose-300'
              : 'text-rose-600 hover:text-rose-500'
        }`}
      >
        <ExternalLink className="w-3.5 h-3.5" />
        {finding.sourceTitle}
      </a>
    </div>
  );
}
