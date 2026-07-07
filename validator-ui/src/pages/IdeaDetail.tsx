import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useTheme } from '../context/ThemeContext';
import { Timeline } from '../components/Timeline';
import { FindingCard } from '../components/FindingCard';
import { CopyToLLM } from '../components/CopyToLLM';
import { Lightbulb, TrendingUp, Clock, ArrowLeft, AlertTriangle, CheckCircle2, Globe } from '../components/Icons';
import { Idea, ScoutCycle } from '../types';
import { api } from '../lib/api';

export function IdeaDetail() {
  const { id } = useParams<{ id: string }>();
  const { theme } = useTheme();
  const [idea, setIdea] = useState<Idea | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedCycleId, setSelectedCycleId] = useState<string | null>(null);
  const [selectedCycle, setSelectedCycle] = useState<ScoutCycle | null>(null);

  useEffect(() => {
    if (!id) {
      setIdea(null);
      setLoading(false);
      return;
    }
    let cancelled = false;

    const loadIdea = async () => {
      try {
        const data = await api.getIdea(id);
        if (cancelled) return;
        setIdea(data);
        if (data.cycles.length > 0) {
          const sortedCycles = [...data.cycles].sort((a, b) => b.day - a.day);
          const latestCycle = sortedCycles[0];
          setSelectedCycleId(latestCycle.id);
          setSelectedCycle(latestCycle);
        }
      } catch {
        if (!cancelled) setIdea(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    setLoading(true);
    loadIdea();

    // Poll lightly so the first scout run (and subsequent cycles) surface
    // automatically while the page is open.
    const interval = setInterval(loadIdea, 15000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [id]);

  useEffect(() => {
    if (idea && selectedCycleId) {
      const cycle = idea.cycles.find((c) => c.id === selectedCycleId);
      setSelectedCycle(cycle || null);
    }
  }, [idea, selectedCycleId]);

  if (loading) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
        }`}>
          <div className="w-8 h-8 mx-auto mb-4 border-2 border-emerald-500/30 border-t-emerald-500 rounded-full animate-spin" />
          <p className={theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}>Loading idea…</p>
        </div>
      </div>
    );
  }

  if (!idea) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <AlertTriangle className={`w-12 h-12 mx-auto mb-4 ${
            theme === 'dark' ? 'text-zinc-600' : 'text-zinc-400'
          }`} />
          <h2 className={`text-xl font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Idea Not Found
          </h2>
          <p className={`mb-4 ${
            theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
          }`}>
            The idea you're looking for doesn't exist.
          </p>
          <Link
            to="/"
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium bg-emerald-500 text-white hover:bg-emerald-600 transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            Back to Dashboard
          </Link>
        </div>
      </div>
    );
  }

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const cycle = selectedCycle || idea.cycles[idea.cycles.length - 1];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-6">
        <Link
          to="/"
          className={`inline-flex items-center gap-1.5 text-sm font-medium mb-4 transition-colors ${
            theme === 'dark'
              ? 'text-zinc-400 hover:text-zinc-200'
              : 'text-zinc-500 hover:text-zinc-700'
          }`}
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Dashboard
        </Link>

        <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-4">
          <div className="flex items-start gap-4">
            <div className={`p-3 rounded-xl ${
              theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'
            }`}>
              <Lightbulb className={`w-7 h-7 ${
                theme === 'dark' ? 'text-teal-400' : 'text-teal-600'
              }`} />
            </div>
            <div>
              <h1 className={`text-2xl sm:text-3xl font-bold tracking-tight mb-2 ${
                theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
              }`}>
                {idea.title}
              </h1>
              <p className={`text-base max-w-2xl ${
                theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
              }`}>
                {idea.description}
              </p>
            </div>
          </div>

          <div className={`flex items-center gap-2 px-4 py-2 rounded-xl ${
            idea.status === 'expanded'
              ? theme === 'dark'
                ? 'bg-emerald-500/10 border border-emerald-500/30'
                : 'bg-emerald-50 border border-emerald-200'
              : theme === 'dark'
                ? 'bg-zinc-800 border border-zinc-700'
                : 'bg-zinc-100 border border-zinc-200'
          }`}>
            {idea.status === 'expanded' ? (
              <TrendingUp className={`w-4 h-4 ${
                theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
              }`} />
            ) : (
              <CheckCircle2 className={`w-4 h-4 ${
                theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
              }`} />
            )}
            <span className={`text-sm font-medium ${
              idea.status === 'expanded'
                ? theme === 'dark' ? 'text-emerald-400' : 'text-emerald-700'
                : theme === 'dark' ? 'text-zinc-300' : 'text-zinc-600'
            }`}>
              {idea.status === 'expanded' ? 'Expanded' : 'Stable'}
            </span>
          </div>
        </div>

        {/* Keywords & Status */}
        <div className={`mt-4 p-4 rounded-xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <div className="flex flex-wrap items-center gap-3 mb-3">
            <span className={`text-sm font-semibold ${
              theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
            }`}>
              Tracked Keywords:
            </span>
            <div className="flex flex-wrap gap-1.5">
              {idea.keywords.map((keyword) => (
                <span
                  key={keyword}
                  className={`text-xs px-2.5 py-1 rounded-full font-medium ${
                    theme === 'dark'
                      ? 'bg-teal-500/20 text-teal-400'
                      : 'bg-teal-100 text-teal-700'
                  }`}
                >
                  {keyword}
                </span>
              ))}
            </div>
          </div>

          {/* Channels & Custom Sources */}
          <div className="flex flex-wrap items-center gap-3 mb-3">
            <span className={`text-sm font-semibold ${
              theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
            }`}>
              Scout Channels:
            </span>
            <div className="flex flex-wrap gap-1.5">
              {idea.channels.map((channel) => (
                <span
                  key={channel}
                  className={`text-xs px-2.5 py-1 rounded-full font-medium ${
                    theme === 'dark'
                      ? 'bg-zinc-800 text-zinc-300'
                      : 'bg-zinc-100 text-zinc-600'
                  }`}
                >
                  {channel}
                </span>
              ))}
            </div>
          </div>

          {/* Custom Channels */}
          {idea.customChannels && idea.customChannels.length > 0 && (
            <div className="flex flex-wrap items-center gap-3 mb-3">
              <span className={`text-sm font-semibold ${
                theme === 'dark' ? 'text-violet-400' : 'text-violet-600'
              }`}>
                Custom Sources:
              </span>
              <div className="flex flex-wrap gap-1.5">
                {idea.customChannels.map((channel) => (
                  <a
                    key={channel.id}
                    href={channel.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full font-medium transition-colors ${
                      theme === 'dark'
                        ? 'bg-violet-500/20 text-violet-400 hover:bg-violet-500/30'
                        : 'bg-violet-100 text-violet-600 hover:bg-violet-200'
                    }`}
                  >
                    <Globe className="w-3 h-3" />
                    {channel.label}
                  </a>
                ))}
              </div>
            </div>
          )}

          <div className={`flex items-start gap-2 text-sm ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
          }`}>
            <span className="font-medium">Scouting Frequency:</span>
            <span>Every {idea.scoutingFrequencyDays} {idea.scoutingFrequencyDays === 1 ? 'day' : 'days'}</span>
          </div>

          <div className={`flex items-start gap-2 text-sm mt-2 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
          }`}>
            <span className="font-medium">Search Radius Status:</span>
            <span>{idea.statusMessage}</span>
          </div>
        </div>

        {/* Last updated */}
        <div className={`flex items-center gap-2 mt-3 text-sm ${
          theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
        }`}>
          <Clock className="w-4 h-4" />
          <span>Last scout run: {formatDate(idea.lastUpdated)}</span>
        </div>
      </div>

      {/* Timeline */}
      {idea.cycles.length > 0 && (
        <div className="mb-6">
          <Timeline
            cycles={idea.cycles}
            selectedCycleId={selectedCycleId || ''}
            onSelectCycle={setSelectedCycleId}
          />
        </div>
      )}

      {/* Copy to LLM */}
      <div className="mb-6">
        <CopyToLLM idea={idea} selectedCycle={cycle} />
      </div>

      {/* Split View */}
      {cycle && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Pros Column */}
          <div className={`p-5 rounded-2xl border-2 transition-all duration-300 ${
            theme === 'dark'
              ? 'bg-emerald-500/5 border-emerald-500/30'
              : 'bg-emerald-50/50 border-emerald-200'
          }`}>
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <div className={`w-3 h-3 rounded-full ${
                  theme === 'dark' ? 'bg-emerald-500' : 'bg-emerald-500'
                }`} />
                <h2 className={`text-lg font-bold ${
                  theme === 'dark' ? 'text-emerald-400' : 'text-emerald-700'
                }`}>
                  Pro-Scout Findings
                </h2>
              </div>
              <span className={`text-sm font-semibold px-3 py-1 rounded-full ${
                theme === 'dark'
                  ? 'bg-emerald-500/20 text-emerald-400'
                  : 'bg-emerald-100 text-emerald-700'
              }`}>
                {cycle.pros.length} signals
              </span>
            </div>

            {cycle.pros.length === 0 ? (
              <div className={`p-6 text-center rounded-xl border ${
                theme === 'dark'
                  ? 'bg-zinc-900/50 border-zinc-800'
                  : 'bg-white border-zinc-200'
              }`}>
                <p className={`text-sm ${
                  theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
                }`}>
                  No positive signals found in this cycle
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {cycle.pros.map((pro) => (
                  <FindingCard key={pro.id} finding={pro} type="pro" />
                ))}
              </div>
            )}
          </div>

          {/* Cons Column */}
          <div className={`p-5 rounded-2xl border-2 transition-all duration-300 ${
            theme === 'dark'
              ? 'bg-rose-500/5 border-rose-500/30'
              : 'bg-rose-50/50 border-rose-200'
          }`}>
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <div className={`w-3 h-3 rounded-full ${
                  theme === 'dark' ? 'bg-rose-500' : 'bg-rose-500'
                }`} />
                <h2 className={`text-lg font-bold ${
                  theme === 'dark' ? 'text-rose-400' : 'text-rose-700'
                }`}>
                  Con-Scout Findings
                </h2>
              </div>
              <span className={`text-sm font-semibold px-3 py-1 rounded-full ${
                theme === 'dark'
                  ? 'bg-rose-500/20 text-rose-400'
                  : 'bg-rose-100 text-rose-700'
              }`}>
                {cycle.cons.length} signals
              </span>
            </div>

            {cycle.cons.length === 0 ? (
              <div className={`p-6 text-center rounded-xl border ${
                theme === 'dark'
                  ? 'bg-zinc-900/50 border-zinc-800'
                  : 'bg-white border-zinc-200'
              }`}>
                <p className={`text-sm ${
                  theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
                }`}>
                  No negative signals found in this cycle
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {cycle.cons.map((con) => (
                  <FindingCard key={con.id} finding={con} type="con" />
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* No cycles yet */}
      {idea.cycles.length === 0 && (
        <div className={`p-8 rounded-2xl border text-center ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <Clock className={`w-12 h-12 mx-auto mb-4 ${
            theme === 'dark' ? 'text-zinc-600' : 'text-zinc-400'
          }`} />
          <h2 className={`text-xl font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Waiting for First Scout Run
          </h2>
          <p className={`${
            theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
          }`}>
            The initial market research scout is pending. Check back soon!
          </p>
        </div>
      )}
    </div>
  );
}
