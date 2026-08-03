import { useEffect } from 'react';
import { Link } from 'react-router-dom';
import { LayoutDashboard, Clock, AlertTriangle } from '../components/Icons';
import { useIdeaStore } from '../store/useIdeaStore';
import { useTheme } from '../context/ThemeContext';
import type { IdeaSummary } from '../types';

function statusBadge(status: string, theme: 'dark' | 'light') {
  if (status === 'PENDING_MUTATION') {
    return (
      <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full ${theme === 'dark' ? 'bg-amber-500/20 text-amber-400' : 'bg-amber-100 text-amber-700'}`}>
        <AlertTriangle className="w-3 h-3" /> review
      </span>
    );
  }
  if (status === 'UNDEPLOYED') {
    return (
      <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full ${theme === 'dark' ? 'bg-sky-500/20 text-sky-400' : 'bg-sky-100 text-sky-700'}`}>
        deploying
      </span>
    );
  }
  return (
    <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full ${theme === 'dark' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-emerald-100 text-emerald-700'}`}>
      active
    </span>
  );
}

function IdeaCard({ idea }: { idea: IdeaSummary }) {
  const { theme } = useTheme();

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    const diffMs = Date.now() - date.getTime();
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffHours / 24);
    if (diffHours < 1) return 'Just now';
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  };

  const pending = idea.proScoutStatus === 'PENDING_MUTATION' || idea.conScoutStatus === 'PENDING_MUTATION';

  return (
    <Link
      to={idea.status === 'DRAFT' ? `/refine/${idea.id}` : `/idea/${idea.id}`}
      className={`group block p-5 rounded-2xl border transition-all duration-300 hover:scale-[1.02] hover:shadow-xl ${
        theme === 'dark' ? 'bg-zinc-900 border-zinc-800 hover:border-zinc-700' : 'bg-white border-zinc-200 hover:border-zinc-300 hover:shadow-lg'
      }`}
    >
      <div className="flex items-start justify-between mb-3 gap-2">
        <h3 className={`font-semibold text-lg leading-snug line-clamp-2 transition-colors ${theme === 'dark' ? 'group-hover:text-emerald-400' : 'group-hover:text-emerald-600'}`}>
          {idea.title}
        </h3>
        {pending && (
          <AlertTriangle className={`w-4 h-4 flex-shrink-0 mt-1 ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`} />
        )}
      </div>

      <p className={`text-sm line-clamp-2 mb-4 ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'}`}>{idea.description}</p>

      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full ${
          idea.status === 'DRAFT'
            ? theme === 'dark' ? 'bg-violet-500/20 text-violet-400' : 'bg-violet-100 text-violet-600'
            : idea.status === 'INITIAL_SWEEP'
              ? theme === 'dark' ? 'bg-sky-500/20 text-sky-400' : 'bg-sky-100 text-sky-600'
              : theme === 'dark' ? 'bg-zinc-800 text-zinc-400' : 'bg-zinc-100 text-zinc-600'
        }`}>
          {idea.status === 'DRAFT' ? 'Draft' : idea.status === 'INITIAL_SWEEP' ? 'Deploying' : `Every ${idea.scoutingFrequencyDays}d`}
        </span>
        <span className="flex items-center gap-1.5 text-xs">
          <span className={theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}>Pro</span>
          {statusBadge(idea.proScoutStatus, theme)}
        </span>
        <span className="flex items-center gap-1.5 text-xs">
          <span className={theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}>Con</span>
          {statusBadge(idea.conScoutStatus, theme)}
        </span>
      </div>

      <div className={`flex items-center justify-between pt-4 border-t ${theme === 'dark' ? 'border-zinc-800' : 'border-zinc-100'}`}>
        <div className="flex items-center gap-4">
          <span className={`text-xs font-semibold ${theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'}`}>{idea.totalPros} pros</span>
          <span className={`text-xs font-semibold ${theme === 'dark' ? 'text-rose-400' : 'text-rose-600'}`}>{idea.totalCons} cons</span>
        </div>
        <div className={`flex items-center gap-1 text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>
          <Clock className="w-3.5 h-3.5" />
          {formatDate(idea.lastUpdated)}
        </div>
      </div>
    </Link>
  );
}

export function Dashboard() {
  const { ideas, loading, error, refreshIdeas } = useIdeaStore();
  const { theme } = useTheme();

  useEffect(() => {
    refreshIdeas();
  }, [refreshIdeas]);

  const totalSignals = ideas.reduce((a, i) => a + i.totalPros + i.totalCons, 0);
  const pendingCount = ideas.filter(
    (i) => i.proScoutStatus === 'PENDING_MUTATION' || i.conScoutStatus === 'PENDING_MUTATION'
  ).length;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <div className={`p-2 rounded-xl ${theme === 'dark' ? 'bg-emerald-500/20' : 'bg-emerald-100'}`}>
            <LayoutDashboard className={`w-6 h-6 ${theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'}`} />
          </div>
          <h1 className={`text-3xl font-bold tracking-tight ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>Dashboard</h1>
        </div>
        <p className={`text-lg ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'}`}>Your tracked ideas and their market scouts</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        <StatCard label="Active Ideas" value={ideas.length} theme={theme} />
        <StatCard label="Total Signals" value={totalSignals} theme={theme} />
        <StatCard label="Pending Reviews" value={pendingCount} highlight={pendingCount > 0} theme={theme} />
      </div>

      {/* Grid */}
      {loading ? (
        <div className={`p-8 rounded-2xl border text-center ${theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <div className="w-8 h-8 mx-auto mb-4 border-2 border-emerald-500/30 border-t-emerald-500 rounded-full animate-spin" />
          <p className={theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}>Loading your ideas…</p>
        </div>
      ) : error ? (
        <div className={`p-8 rounded-2xl border text-center ${theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30' : 'bg-rose-50 border-rose-200'}`}>
          <p className={`mb-4 ${theme === 'dark' ? 'text-rose-400' : 'text-rose-700'}`}>{error}</p>
          <button onClick={() => refreshIdeas()} className="px-4 py-2 rounded-lg text-sm font-medium bg-emerald-500 text-white hover:bg-emerald-600 transition-colors">Retry</button>
        </div>
      ) : ideas.length === 0 ? (
        <div className={`p-8 rounded-2xl border text-center ${theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <Clock className={`w-12 h-12 mx-auto mb-4 ${theme === 'dark' ? 'text-zinc-600' : 'text-zinc-400'}`} />
          <h2 className={`text-xl font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>No ideas tracked yet</h2>
          <p className={theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'}>Describe an idea and we'll deploy your first scouts.</p>
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

function StatCard({ label, value, highlight, theme }: { label: string; value: number; highlight?: boolean; theme: 'dark' | 'light' }) {
  return (
    <div className={`p-5 rounded-2xl border transition-all duration-300 ${
      highlight
        ? theme === 'dark' ? 'bg-amber-500/10 border-amber-500/30' : 'bg-amber-50 border-amber-200'
        : theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
    }`}>
      <p className={`text-sm font-medium mb-1 ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'}`}>{label}</p>
      <p className={`text-3xl font-bold ${highlight ? (theme === 'dark' ? 'text-amber-400' : 'text-amber-600') : (theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900')}`}>{value}</p>
    </div>
  );
}
