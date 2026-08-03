import { useState, useEffect, useCallback } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useTheme } from '../context/ThemeContext';
import { FindingCard } from '../components/FindingCard';
import { PromptApprovalDrawer } from '../components/PromptApprovalDrawer';
import { ExportDropdown } from '../components/ExportDropdown';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { Lightbulb, ArrowLeft, Clock, AlertTriangle, TrendingUp, Sparkles } from '../components/Icons';
import { api } from '../lib/api';
import type { IdeaDetail, Scout } from '../types';

export function IdeaDetailDashboard() {
  const { id } = useParams<{ id: string }>();
  const { theme } = useTheme();
  const [idea, setIdea] = useState<IdeaDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [drawerScout, setDrawerScout] = useState<Scout | null>(null);
  const [deactivating, setDeactivating] = useState(false);
  const [confirmDeactivate, setConfirmDeactivate] = useState(false);
  const [stopTarget, setStopTarget] = useState<Scout | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (!id) {
      setIdea(null);
      setLoading(false);
      return;
    }
    try {
      const data = await api.getIdea(id);
      setIdea(data);
      // Keep the drawer in sync with the freshest scout state.
      setDrawerScout((prev) => {
        if (!prev) return prev;
        const updated = data.scouts.find((s) => s.id === prev.id);
        return updated ?? null;
      });
    } catch {
      setIdea(null);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    setLoading(true);
    reload();
    const interval = setInterval(reload, 15000);
    return () => clearInterval(interval);
  }, [reload]);

  if (loading) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className={`p-8 rounded-2xl border text-center ${theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <div className="w-8 h-8 mx-auto mb-4 border-2 border-emerald-500/30 border-t-emerald-500 rounded-full animate-spin" />
          <p className={theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}>Loading idea…</p>
        </div>
      </div>
    );
  }

  if (!idea) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className={`p-8 rounded-2xl border text-center ${theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <AlertTriangle className={`w-12 h-12 mx-auto mb-4 ${theme === 'dark' ? 'text-zinc-600' : 'text-zinc-400'}`} />
          <h2 className={`text-xl font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>Idea Not Found</h2>
          <Link to="/" className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium bg-emerald-500 text-white hover:bg-emerald-600 transition-colors">
            <ArrowLeft className="w-4 h-4" /> Back to Dashboard
          </Link>
        </div>
      </div>
    );
  }

  const proScout = idea.scouts.find((s) => s.scoutType === 'PRO');
  const conScout = idea.scouts.find((s) => s.scoutType === 'CON');
  const isInitialSweep = idea.status === 'INITIAL_SWEEP';
  const isInactive = idea.status === 'INACTIVE';

  const handleDeactivate = async () => {
    setConfirmDeactivate(false);
    setDeactivating(true);
    setActionError(null);
    try {
      await api.deactivateIdea(idea.id);
      await reload();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to deactivate idea');
    } finally {
      setDeactivating(false);
    }
  };

  const handleStopConfirm = async () => {
    if (!stopTarget) return;
    setDeactivating(true);
    setActionError(null);
    try {
      await api.stopScout(stopTarget.id);
      setStopTarget(null);
      await reload();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to stop scout');
      setStopTarget(null);
    } finally {
      setDeactivating(false);
    }
  };

  const formatDate = (s: string) =>
    new Date(s).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-6">
        <Link to="/" className={`inline-flex items-center gap-1.5 text-sm font-medium mb-4 transition-colors ${theme === 'dark' ? 'text-zinc-400 hover:text-zinc-200' : 'text-zinc-500 hover:text-zinc-700'}`}>
          <ArrowLeft className="w-4 h-4" /> Back to Dashboard
        </Link>
        <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-4">
          <div className="flex items-start gap-4">
            <div className={`p-3 rounded-xl ${theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'}`}>
              <Lightbulb className={`w-7 h-7 ${theme === 'dark' ? 'text-teal-400' : 'text-teal-600'}`} />
            </div>
            <div>
              <h1 className={`text-2xl sm:text-3xl font-bold tracking-tight mb-2 ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>
                {idea.title}
              </h1>
              <p className={`text-base max-w-2xl ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'}`}>{idea.description}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <div className={`flex items-center gap-2 px-4 py-2 rounded-xl border ${
              isInactive
                ? (theme === 'dark' ? 'bg-zinc-800 border-zinc-700' : 'bg-zinc-100 border-zinc-300')
                : isInitialSweep
                  ? (theme === 'dark' ? 'bg-sky-500/10 border-sky-500/30' : 'bg-sky-50 border-sky-200')
                  : (theme === 'dark' ? 'bg-emerald-500/10 border-emerald-500/30' : 'bg-emerald-50 border-emerald-200')
            }`}>
              <span className={`text-sm font-medium ${
                isInactive
                  ? (theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500')
                  : isInitialSweep
                    ? (theme === 'dark' ? 'text-sky-400' : 'text-sky-700')
                    : (theme === 'dark' ? 'text-emerald-400' : 'text-emerald-700')
              }`}>
                {isInactive ? 'Inactive' : isInitialSweep ? 'Deploying Scouts…' : 'Active'}
              </span>
            </div>
            {/* Export dropdown */}
            <ExportDropdown idea={idea} />
            {/* Deactivate control: only while the idea is live. */}
            {!isInactive && (
              <button
                type="button"
                onClick={() => setConfirmDeactivate(true)}
                disabled={deactivating}
                className={`text-xs font-medium px-3 py-2 rounded-xl border transition-colors disabled:opacity-50 ${
                  theme === 'dark'
                    ? 'text-zinc-400 border-zinc-700 hover:text-rose-300 hover:border-rose-500/40 hover:bg-rose-500/10'
                    : 'text-zinc-500 border-zinc-300 hover:text-rose-600 hover:border-rose-300 hover:bg-rose-50'
                }`}
                title="Deactivate this idea: stop both scouts and mark it inactive"
              >
                {deactivating ? 'Deactivating…' : 'Deactivate'}
              </button>
            )}
          </div>
        </div>

        <div className={`flex items-center gap-2 mt-3 text-sm ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'}`}>
          <Clock className="w-4 h-4" />
          <span>Scouts run every {idea.scoutingFrequencyDays} {idea.scoutingFrequencyDays === 1 ? 'day' : 'days'} · last updated {formatDate(idea.lastUpdated)}</span>
        </div>
      </div>

      {/* Initial sweep state */}
      {isInitialSweep && (
        <div className={`p-8 rounded-2xl border text-center mb-6 ${theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <div className="w-8 h-8 mx-auto mb-4 border-2 border-emerald-500/30 border-t-emerald-500 rounded-full animate-spin" />
          <h2 className={`text-lg font-semibold mb-1 ${theme === 'dark' ? 'text-zinc-200' : 'text-zinc-700'}`}>Running Day 0 Research</h2>
          <p className={`text-sm ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'}`}>
            We're researching the market and deploying your Pro &amp; Con scouts. This takes a few minutes.
          </p>
        </div>
      )}

      {/* Split Board */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ScoutColumn
          title="Pro-Scout Findings"
          polarity="pro"
          scout={proScout}
          findings={idea.recentPros}
          total={idea.totalPros}
          onReview={() => proScout && setDrawerScout(proScout)}
          onStopRequest={() => proScout && setStopTarget(proScout)}
          stopping={!!stopTarget && stopTarget.id === proScout?.id && deactivating}
          theme={theme}
        />
        <ScoutColumn
          title="Con-Scout Findings"
          polarity="con"
          scout={conScout}
          findings={idea.recentCons}
          total={idea.totalCons}
          onReview={() => conScout && setDrawerScout(conScout)}
          onStopRequest={() => conScout && setStopTarget(conScout)}
          stopping={!!stopTarget && stopTarget.id === conScout?.id && deactivating}
          theme={theme}
        />
      </div>

      {/* Approval Drawer */}
      {drawerScout && drawerScout.pendingProposal && (
        <PromptApprovalDrawer
          scout={drawerScout}
          ideaId={idea.id}
          onClose={() => setDrawerScout(null)}
          onResolved={() => setDrawerScout(null)}
        />
      )}

      {/* Confirm Dialogs */}
      <ConfirmDialog
        open={confirmDeactivate}
        title={`Deactivate "${idea.title}"?`}
        message="Both scouts will be stopped, pending proposals rejected, and the idea marked inactive. Existing findings are kept. This cannot be undone."
        confirmLabel="Deactivate"
        danger
        onConfirm={handleDeactivate}
        onCancel={() => setConfirmDeactivate(false)}
      />
      <ConfirmDialog
        open={!!stopTarget}
        title={`Stop ${stopTarget?.scoutType === 'PRO' ? 'Pro' : 'Con'} Scout?`}
        message="This scout will be permanently stopped. Existing findings are kept. This cannot be undone."
        confirmLabel="Stop Scout"
        danger
        onConfirm={handleStopConfirm}
        onCancel={() => setStopTarget(null)}
      />

      {/* Error Toast */}
      {actionError && (
        <div className={`fixed bottom-4 right-4 z-50 max-w-md px-4 py-3 rounded-xl border text-sm shadow-xl ${
          theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' : 'bg-rose-50 border-rose-200 text-rose-700'
        }`}>
          {actionError}
          <button onClick={() => setActionError(null)} className="ml-3 underline">Dismiss</button>
        </div>
      )}
    </div>
  );
}

interface ScoutColumnProps {
  title: string;
  polarity: 'pro' | 'con';
  scout?: Scout;
  findings: IdeaDetail['recentPros'];
  total: number;
  onReview: () => void;
  onStopRequest: () => void;
  stopping?: boolean;
  theme: 'dark' | 'light';
}

function ScoutColumn({ title, polarity, scout, findings, total, onReview, onStopRequest, stopping, theme }: ScoutColumnProps) {
  const isPro = polarity === 'pro';
  const pendingMutation = scout?.status === 'PENDING_MUTATION' && scout?.pendingProposal;
  const undeployed = !scout || scout.status === 'UNDEPLOYED';
  const stopped = scout?.status === 'STOPPED';

  const accent = isPro
    ? { ring: theme === 'dark' ? 'border-emerald-500/30' : 'border-emerald-200', dot: 'bg-emerald-500', text: theme === 'dark' ? 'text-emerald-400' : 'text-emerald-700', soft: theme === 'dark' ? 'bg-emerald-500/5' : 'bg-emerald-50/50' }
    : { ring: theme === 'dark' ? 'border-rose-500/30' : 'border-rose-200', dot: 'bg-rose-500', text: theme === 'dark' ? 'text-rose-400' : 'text-rose-700', soft: theme === 'dark' ? 'bg-rose-500/5' : 'bg-rose-50/50' };

  return (
    <div className={`p-5 rounded-2xl border-2 transition-all duration-300 ${accent.ring} ${accent.soft}`}>
      {/* Column header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <div className={`w-3 h-3 rounded-full ${stopped ? 'bg-zinc-400' : accent.dot}`} />
          <h2 className={`text-lg font-bold ${accent.text}`}>{title}</h2>
        </div>
        <div className="flex items-center gap-2">
          <span className={`text-sm font-semibold px-3 py-1 rounded-full ${theme === 'dark' ? 'bg-zinc-800 text-zinc-300' : 'bg-zinc-100 text-zinc-600'}`}>
            {total} total
          </span>
        </div>
      </div>

      {/* Scout status row */}
      <div className={`flex items-center justify-between gap-2 mb-4 text-xs ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}`}>
        <div className="flex items-center gap-2">
          <TrendingUp className={`w-3.5 h-3.5 ${undeployed ? 'animate-pulse' : ''}`} />
          <span>
            {undeployed
              ? 'Scout deploying…'
              : stopped
                ? 'Scout stopped — no longer running'
                : `Scout ${scout?.status === 'PENDING_MUTATION' ? 'awaiting review' : 'active'}`}
          </span>
        </div>
        {/* Stop control: only when the scout is live and not already stopped. */}
        {!undeployed && !stopped && scout && (
          <button
            type="button"
            onClick={onStopRequest}
            disabled={stopping}
            className={`text-xs font-medium px-2.5 py-1 rounded-md transition-colors disabled:opacity-50 ${
              theme === 'dark'
                ? 'text-zinc-400 hover:text-rose-300 hover:bg-rose-500/10'
                : 'text-zinc-500 hover:text-rose-600 hover:bg-rose-50'
            }`}
            title="Stop this scout permanently"
          >
            {stopping ? 'Stopping…' : 'Stop scout'}
          </button>
        )}
      </div>

      {/* Pending-mutation banner */}
      {pendingMutation && (
        <button
          type="button"
          onClick={onReview}
          className={`w-full flex items-center gap-3 p-3 mb-4 rounded-xl border text-left transition-all duration-200 ${
            theme === 'dark'
              ? 'bg-amber-500/10 border-amber-500/40 hover:bg-amber-500/20'
              : 'bg-amber-50 border-amber-300 hover:bg-amber-100'
          }`}
        >
          <AlertTriangle className={`w-5 h-5 flex-shrink-0 ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`} />
          <div className="flex-1 min-w-0">
            <p className={`text-sm font-semibold ${theme === 'dark' ? 'text-amber-300' : 'text-amber-700'}`}>
              AI proposed a search-radius expansion
            </p>
            <p className={`text-xs ${theme === 'dark' ? 'text-amber-400/70' : 'text-amber-600'}`}>
              Review and approve or reject the updated prompt →
            </p>
          </div>
          <Sparkles className={`w-4 h-4 flex-shrink-0 ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`} />
        </button>
      )}

      {/* Findings */}
      {findings.length === 0 ? (
        <div className={`p-6 text-center rounded-xl border ${theme === 'dark' ? 'bg-zinc-900/50 border-zinc-800' : 'bg-white border-zinc-200'}`}>
          <p className={`text-sm ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'}`}>
            {undeployed ? 'Scout will deliver findings after its first run.' : 'No signals found yet. Findings appear after the next scout run.'}
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {findings.map((f) => (
            <FindingCard key={f.id} finding={f} type={isPro ? 'pro' : 'con'} />
          ))}
        </div>
      )}
    </div>
  );
}
