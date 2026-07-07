import { Link } from 'react-router-dom';
import { LayoutDashboard, TrendingUp, Clock, MessageSquare, Youtube, Newspaper, InstagramIcon, Globe } from '../components/Icons';
import { useIdeas } from '../context/IdeaContext';
import { useTheme } from '../context/ThemeContext';
import { Platform, Idea } from '../types';

const platformIcons: Record<Platform, React.ElementType> = {
  reddit: MessageSquare,
  youtube: Youtube,
  news: Newspaper,
  social: InstagramIcon,
  custom: Globe,
};

function IdeaCard({ idea }: { idea: Idea }) {
  const { theme } = useTheme();

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffHours / 24);

    if (diffHours < 1) return 'Just now';
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  };

  const totalCustomChannels = idea.customChannels?.length || 0;

  return (
    <Link
      to={`/idea/${idea.id}`}
      className={`group block p-5 rounded-2xl border transition-all duration-300 hover:scale-[1.02] hover:shadow-xl ${
        theme === 'dark'
          ? 'bg-zinc-900 border-zinc-800 hover:border-zinc-700 hover:bg-zinc-900/80'
          : 'bg-white border-zinc-200 hover:border-zinc-300 hover:shadow-lg'
      }`}
    >
      <div className="flex items-start justify-between mb-3">
        <h3 className={`font-semibold text-lg leading-snug line-clamp-2 transition-colors ${
          theme === 'dark' ? 'group-hover:text-emerald-400' : 'group-hover:text-emerald-600'
        }`}>
          {idea.title}
        </h3>
      </div>

      <p className={`text-sm line-clamp-2 mb-4 ${
        theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
      }`}>
        {idea.description}
      </p>

      <div className="flex flex-wrap gap-1.5 mb-4">
        {idea.keywords.slice(0, 3).map((keyword) => (
          <span
            key={keyword}
            className={`text-xs px-2 py-1 rounded-md transition-colors ${
              theme === 'dark'
                ? 'bg-zinc-800 text-zinc-400'
                : 'bg-zinc-100 text-zinc-600'
            }`}
          >
            {keyword}
          </span>
        ))}
        {idea.keywords.length > 3 && (
          <span className={`text-xs px-2 py-1 ${
            theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'
          }`}>
            +{idea.keywords.length - 3}
          </span>
        )}
      </div>

      <div className="flex items-center gap-3 mb-4">
        <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full ${
          idea.status === 'expanded'
            ? theme === 'dark'
              ? 'bg-emerald-500/20 text-emerald-400'
              : 'bg-emerald-100 text-emerald-600'
            : theme === 'dark'
              ? 'bg-zinc-800 text-zinc-400'
              : 'bg-zinc-100 text-zinc-600'
        }`}>
          {idea.status === 'expanded' && <TrendingUp className="w-3 h-3" />}
          Every {idea.scoutingFrequencyDays} {idea.scoutingFrequencyDays === 1 ? 'day' : 'days'}
        </span>

        <div className="flex items-center gap-1">
          {idea.channels.map((channel) => {
            const Icon = platformIcons[channel];
            return (
              <Icon
                key={channel}
                className={`w-4 h-4 ${
                  theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'
                }`}
              />
            );
          })}
          {totalCustomChannels > 0 && (
            <span className={`text-xs font-medium px-1.5 ${
              theme === 'dark' ? 'text-violet-400' : 'text-violet-600'
            }`}>
              +{totalCustomChannels}
            </span>
          )}
        </div>
      </div>

      <div className={`flex items-center justify-between pt-4 border-t ${
        theme === 'dark' ? 'border-zinc-800' : 'border-zinc-100'
      }`}>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-1.5">
            <span className={`text-xs font-semibold ${
              theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
            }`}>
              {idea.totalPros} pros
            </span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className={`text-xs font-semibold ${
              theme === 'dark' ? 'text-rose-400' : 'text-rose-600'
            }`}>
              {idea.totalCons} cons
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {idea.newSignalsToday > 0 && (
            <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full animate-pulse ${
              theme === 'dark'
                ? 'bg-blue-500/20 text-blue-400'
                : 'bg-blue-100 text-blue-600'
            }`}>
              +{idea.newSignalsToday} new
            </span>
          )}
          <div className={`flex items-center gap-1 text-xs ${
            theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'
          }`}>
            <Clock className="w-3.5 h-3.5" />
            {formatDate(idea.lastUpdated)}
          </div>
        </div>
      </div>
    </Link>
  );
}

export function Dashboard() {
  const { ideas, loading, error, refreshIdeas } = useIdeas();
  const { theme } = useTheme();

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <div className={`p-2 rounded-xl ${
            theme === 'dark' ? 'bg-emerald-500/20' : 'bg-emerald-100'
          }`}>
            <LayoutDashboard className={`w-6 h-6 ${
              theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
            }`} />
          </div>
          <h1 className={`text-3xl font-bold tracking-tight ${
            theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
          }`}>
            Dashboard
          </h1>
        </div>
        <p className={`text-lg ${
          theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
        }`}>
          Your tracked ideas and their market signals
        </p>
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        <div className={`p-5 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <p className={`text-sm font-medium mb-1 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
          }`}>
            Active Ideas
          </p>
          <p className={`text-3xl font-bold ${
            theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
          }`}>
            {ideas.length}
          </p>
        </div>

        <div className={`p-5 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <p className={`text-sm font-medium mb-1 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
          }`}>
            Total Signals
          </p>
          <p className={`text-3xl font-bold ${
            theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
          }`}>
            {ideas.reduce((acc, i) => acc + i.totalPros + i.totalCons, 0)}
          </p>
        </div>

        <div className={`p-5 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <p className={`text-sm font-medium mb-1 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
          }`}>
            Signals Today
          </p>
          <p className={`text-3xl font-bold ${
            theme === 'dark' ? 'text-blue-400' : 'text-blue-600'
          }`}>
            +{ideas.reduce((acc, i) => acc + i.newSignalsToday, 0)}
          </p>
        </div>
      </div>

      {/* Ideas Grid */}
      {loading ? (
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
        }`}>
          <div className="w-8 h-8 mx-auto mb-4 border-2 border-emerald-500/30 border-t-emerald-500 rounded-full animate-spin" />
          <p className={theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}>Loading your ideas…</p>
        </div>
      ) : error ? (
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30' : 'bg-rose-50 border-rose-200'
        }`}>
          <p className={`mb-4 ${theme === 'dark' ? 'text-rose-400' : 'text-rose-700'}`}>{error}</p>
          <button
            onClick={() => refreshIdeas()}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-emerald-500 text-white hover:bg-emerald-600 transition-colors"
          >
            Retry
          </button>
        </div>
      ) : ideas.length === 0 ? (
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
        }`}>
          <Clock className={`w-12 h-12 mx-auto mb-4 ${theme === 'dark' ? 'text-zinc-600' : 'text-zinc-400'}`} />
          <h2 className={`text-xl font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>
            No ideas tracked yet
          </h2>
          <p className={theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'}>
            Create your first scout to start tracking market signals.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-5">
          {ideas.map((idea) => (
            <IdeaCard key={idea.id} idea={idea} />
          ))}
        </div>
      )}
    </div>
  );
}
