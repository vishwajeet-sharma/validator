import { useState } from 'react';
import { X, Check, AlertTriangle, Sparkles } from './Icons';
import { useTheme } from '../context/ThemeContext';
import { api, ApiError } from '../lib/api';
import type { Scout } from '../types';

interface PromptApprovalDrawerProps {
  scout: Scout;
  ideaId: string;
  onClose: () => void;
  onResolved: () => void;
}

export function PromptApprovalDrawer({ scout, onClose, onResolved }: PromptApprovalDrawerProps) {
  const { theme } = useTheme();
  const proposal = scout.pendingProposal;

  const [editedText, setEditedText] = useState(proposal?.proposedPrompt ?? '');
  const [busy, setBusy] = useState<'APPROVE' | 'REJECT' | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!proposal) return null;

  const polarityLabel = scout.scoutType === 'PRO' ? 'Pro-Scout' : 'Con-Scout';

  const respond = async (action: 'APPROVE' | 'REJECT') => {
    setBusy(action);
    setError(null);
    try {
      await api.respondProposal(proposal.id, {
        action,
        edited_text: action === 'APPROVE' ? editedText : undefined,
      });
      onResolved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not submit your decision. Please try again.');
      setBusy(null);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onClose} />

      {/* Drawer */}
      <div className={`relative w-full max-w-xl h-full overflow-y-auto border-l shadow-2xl transition-transform ${
        theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
      }`}>
        {/* Header */}
        <div className={`sticky top-0 z-10 flex items-center justify-between px-6 py-4 border-b backdrop-blur-md ${
          theme === 'dark' ? 'bg-zinc-900/90 border-zinc-800' : 'bg-white/90 border-zinc-200'
        }`}>
          <div className="flex items-center gap-3">
            <div className={`p-2 rounded-lg ${theme === 'dark' ? 'bg-amber-500/20' : 'bg-amber-100'}`}>
              <AlertTriangle className={`w-5 h-5 ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`} />
            </div>
            <div>
              <h2 className={`text-lg font-bold ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>
                Search Radius Expansion
              </h2>
              <p className={`text-xs ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}`}>
                {polarityLabel} · Review the AI-proposed prompt update
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className={`p-2 rounded-lg transition-colors ${
              theme === 'dark' ? 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800' : 'text-zinc-500 hover:text-zinc-700 hover:bg-zinc-100'
            }`}
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body: side-by-side comparison */}
        <div className="px-6 py-6 space-y-6">
          {/* Active prompt */}
          <div>
            <div className="flex items-center gap-2 mb-2">
              <span className={`text-xs font-semibold uppercase tracking-wide ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}`}>
                Active Tracking Prompt
              </span>
            </div>
            <div className={`p-4 rounded-xl border text-sm leading-relaxed whitespace-pre-wrap ${
              theme === 'dark' ? 'bg-zinc-800/50 border-zinc-700 text-zinc-300' : 'bg-zinc-50 border-zinc-200 text-zinc-600'
            }`}>
              {scout.currentPrompt}
            </div>
          </div>

          {/* Proposed prompt (editable) */}
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Sparkles className={`w-3.5 h-3.5 ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`} />
              <span className={`text-xs font-semibold uppercase tracking-wide ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`}>
                AI-Proposed Expansion (editable)
              </span>
            </div>
            <textarea
              value={editedText}
              onChange={(e) => setEditedText(e.target.value)}
              rows={10}
              className={`w-full px-4 py-3 rounded-xl border text-sm leading-relaxed transition-all duration-200 focus:outline-none focus:ring-2 resize-none ${
                theme === 'dark'
                  ? 'bg-zinc-800 border-amber-500/40 text-zinc-100 focus:border-amber-500 focus:ring-amber-500/20'
                  : 'bg-amber-50/40 border-amber-300 text-zinc-900 focus:border-amber-500 focus:ring-amber-500/20'
              }`}
            />
            <p className={`mt-2 text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>
              Fine-tune the prompt before approving — add your own angles or constraints. Approving updates the live Yutori scout.
            </p>
          </div>

          {error && (
            <div className={`px-4 py-3 rounded-xl text-sm border ${
              theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' : 'bg-rose-50 border-rose-200 text-rose-700'
            }`}>
              {error}
            </div>
          )}
        </div>

        {/* Footer actions */}
        <div className={`sticky bottom-0 flex justify-end gap-3 px-6 py-4 border-t backdrop-blur-md ${
          theme === 'dark' ? 'bg-zinc-900/90 border-zinc-800' : 'bg-white/90 border-zinc-200'
        }`}>
          <button
            type="button"
            onClick={() => respond('REJECT')}
            disabled={busy !== null}
            className={`px-5 py-2.5 rounded-xl font-medium text-sm transition-all duration-200 disabled:opacity-50 ${
              theme === 'dark'
                ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700'
                : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200'
            }`}
          >
            {busy === 'REJECT' ? 'Rejecting…' : 'Reject Proposal'}
          </button>
          <button
            type="button"
            onClick={() => respond('APPROVE')}
            disabled={busy !== null || !editedText.trim()}
            className="flex items-center gap-2 px-5 py-2.5 rounded-xl font-semibold text-sm transition-all duration-200 disabled:opacity-50 bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600 shadow-lg shadow-emerald-500/25"
          >
            <Check className="w-4 h-4" />
            {busy === 'APPROVE' ? 'Approving…' : 'Approve Changes'}
          </button>
        </div>
      </div>
    </div>
  );
}
