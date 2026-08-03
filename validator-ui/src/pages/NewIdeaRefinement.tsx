import { useState, useRef, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { PlusCircle, Sparkles, Send, ArrowLeft, Edit3, Check, X, Lightbulb } from '../components/Icons';
import { useTheme } from '../context/ThemeContext';
import { api, ApiError } from '../lib/api';
import type { ChatMessage } from '../types';

export function NewIdeaRefinement() {
  const { id: existingId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { theme } = useTheme();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const [phase, setPhase] = useState<'input' | 'chat'>('input');
  const [ideaId, setIdeaId] = useState<string | null>(existingId ?? null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [refinedPrompt, setRefinedPrompt] = useState('');
  const [frequency, setFrequency] = useState(7);
  const [input, setInput] = useState('');
  const [initialDesc, setInitialDesc] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [editedPrompt, setEditedPrompt] = useState('');
  const [starting, setStarting] = useState(false);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    if (phase === 'chat') scrollToBottom();
  }, [messages, phase, scrollToBottom]);

  // Load existing conversation if navigating to /refine/:id
  useEffect(() => {
    if (!existingId) return;
    (async () => {
      try {
        setBusy(true);
        const conv = await api.getConversation(existingId);
        setIdeaId(conv.id);
        setMessages(conv.conversation);
        setRefinedPrompt(conv.refinedPrompt);
        setPhase('chat');
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Failed to load conversation');
      } finally {
        setBusy(false);
      }
    })();
  }, [existingId]);

  const handleInitialSubmit = async () => {
    const desc = initialDesc.trim();
    if (desc.length < 12 || busy) return;

    setBusy(true);
    setError(null);
    try {
      const conv = await api.createIdea({ description: desc, scoutingFrequencyDays: frequency });
      setIdeaId(conv.id);
      setMessages(conv.conversation);
      setRefinedPrompt(conv.refinedPrompt);
      setPhase('chat');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to start conversation');
    } finally {
      setBusy(false);
    }
  };

  const handleChat = async () => {
    const msg = input.trim();
    if (!msg || !ideaId || busy) return;

    const userMsg: ChatMessage = { role: 'user', content: msg, timestamp: new Date().toISOString() };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setBusy(true);
    setError(null);

    try {
      const res = await api.chat(ideaId, msg);
      setMessages((prev) => [...prev, res.message]);
      if (res.prompt) setRefinedPrompt(res.prompt);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to get response');
      setMessages((prev) => prev.slice(0, -1));
      setInput(msg);
    } finally {
      setBusy(false);
    }
  };

  const handleSavePrompt = async () => {
    if (!ideaId) return;
    try {
      await api.updatePrompt(ideaId, editedPrompt);
      setRefinedPrompt(editedPrompt);
      setEditing(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update prompt');
    }
  };

  const handleStartResearch = async () => {
    if (!ideaId) return;
    setStarting(true);
    setError(null);
    try {
      if (editing && editedPrompt.trim()) {
        await api.updatePrompt(ideaId, editedPrompt);
      }
      await api.startResearch(ideaId);
      navigate(`/idea/${ideaId}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to start research');
      setStarting(false);
    }
  };

  const cardClass = theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200';
  const textMuted = theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600';
  const inputClass = theme === 'dark'
    ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-emerald-500'
    : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-emerald-500';

  // --- Input Phase ---
  if (phase === 'input') {
    return (
      <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className={`p-2 rounded-xl ${theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'}`}>
              <PlusCircle className={`w-6 h-6 ${theme === 'dark' ? 'text-teal-400' : 'text-teal-600'}`} />
            </div>
            <h1 className={`text-3xl font-bold tracking-tight ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>
              New Idea
            </h1>
          </div>
          <p className={`text-lg ${textMuted}`}>
            Describe your idea. Our AI analyst will ask clarifying questions to build a research brief.
          </p>
        </div>

        <div className={`p-6 rounded-2xl border ${cardClass}`}>
          <label className={`block text-sm font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>
            Your Idea
          </label>
          <textarea
            value={initialDesc}
            onChange={(e) => setInitialDesc(e.target.value)}
            placeholder="e.g. A SaaS that automatically reviews pull requests using LLMs, catching bugs and suggesting improvements before merge..."
            rows={6}
            className={`w-full px-4 py-3 rounded-xl border text-base transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-emerald-500/20 resize-none ${inputClass}`}
          />
          <div className={`mt-2 text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>
            {initialDesc.trim().length} characters (minimum 12)
          </div>
        </div>

        {/* Frequency Slider */}
        <div className={`mt-6 p-6 rounded-2xl border ${cardClass}`}>
          <label className={`block text-sm font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>
            Scouting Frequency
          </label>
          <p className={`text-sm mb-4 ${textMuted}`}>How often should scouts refresh their market sweep?</p>
          <div className="flex items-center gap-4">
            <input
              type="range"
              min={1}
              max={30}
              value={frequency}
              onChange={(e) => setFrequency(Number(e.target.value))}
              className="flex-1 accent-emerald-500"
            />
            <span className={`text-sm font-semibold min-w-[80px] text-right ${
              theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
            }`}>
              {frequency === 1 ? 'Every day' : `Every ${frequency} days`}
            </span>
          </div>
        </div>

        {error && (
          <div className={`mt-4 px-4 py-3 rounded-xl text-sm border ${
            theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' : 'bg-rose-50 border-rose-200 text-rose-700'
          }`}>{error}</div>
        )}

        <div className="flex justify-end gap-3 pt-6">
          <button
            onClick={() => navigate('/')}
            className={`px-6 py-3 rounded-xl font-medium text-sm transition-all duration-200 ${
              theme === 'dark' ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700' : 'bg-zinc-200 text-zinc-600 hover:bg-zinc-300'
            }`}>
            Cancel
          </button>
          <button
            onClick={handleInitialSubmit}
            disabled={initialDesc.trim().length < 12 || busy}
            className={`flex items-center gap-2 px-6 py-3 rounded-xl font-semibold text-sm transition-all duration-200 ${
              initialDesc.trim().length >= 12 && !busy
                ? 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600 shadow-lg shadow-emerald-500/25'
                : theme === 'dark' ? 'bg-zinc-800 text-zinc-500 cursor-not-allowed' : 'bg-zinc-200 text-zinc-400 cursor-not-allowed'
            }`}>
            {busy ? (
              <>
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Analyzing...
              </>
            ) : (
              <>
                <Sparkles className="w-4 h-4" />
                Start Analysis
              </>
            )}
          </button>
        </div>
      </div>
    );
  }

  // --- Chat Phase ---
  const hasPrompt = refinedPrompt.length > 0;

  return (
    <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-8 flex flex-col" style={{ minHeight: 'calc(100vh - 4rem)' }}>
      {/* Header */}
      <div className="mb-6">
        <button
          onClick={() => navigate('/')}
          className={`inline-flex items-center gap-1.5 text-sm font-medium mb-4 transition-colors ${textMuted} hover:text-emerald-400`}>
          <ArrowLeft className="w-4 h-4" /> Back
        </button>
        <div className="flex items-center gap-3">
          <div className={`p-2 rounded-xl ${theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'}`}>
            <Lightbulb className={`w-5 h-5 ${theme === 'dark' ? 'text-teal-400' : 'text-teal-600'}`} />
          </div>
          <div>
            <h1 className={`text-xl font-bold ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>
              Idea Refinement
            </h1>
            <p className={`text-xs ${textMuted}`}>Chat with our AI analyst to sharpen your research brief</p>
          </div>
        </div>
      </div>

      {/* Chat Messages */}
      <div className={`flex-1 space-y-4 mb-6 overflow-y-auto p-4 rounded-2xl border ${cardClass}`} style={{ maxHeight: 'calc(100vh - 22rem)' }}>
        {messages.map((msg, i) => {
          const isUser = msg.role === 'user';
          return (
            <div key={i} className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
              <div className={`max-w-[80%] rounded-2xl px-4 py-3 ${
                isUser
                  ? 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white'
                  : msg.messageType === 'prompt'
                    ? theme === 'dark' ? 'bg-amber-500/10 border border-amber-500/30 text-zinc-100' : 'bg-amber-50 border border-amber-200 text-zinc-900'
                    : theme === 'dark' ? 'bg-zinc-800 text-zinc-100' : 'bg-zinc-100 text-zinc-900'
              }`}>
                {!isUser && msg.messageType !== 'prompt' && (
                  <div className="flex items-center gap-1.5 mb-1">
                    <Sparkles className={`w-3 h-3 ${theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'}`} />
                    <span className={`text-xs font-semibold ${theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'}`}>AI Analyst</span>
                  </div>
                )}
                {!isUser && msg.messageType === 'prompt' && (
                  <div className="flex items-center gap-1.5 mb-2">
                    <Sparkles className="w-3 h-3 text-amber-400" />
                    <span className="text-xs font-semibold text-amber-400">Research Brief Ready</span>
                  </div>
                )}
                <div className={`text-sm leading-relaxed whitespace-pre-wrap ${msg.messageType === 'prompt' ? 'font-mono text-xs' : ''}`}>
                  {msg.content}
                </div>
              </div>
            </div>
          );
        })}
        {busy && (
          <div className="flex justify-start">
            <div className={`rounded-2xl px-4 py-3 ${theme === 'dark' ? 'bg-zinc-800' : 'bg-zinc-100'}`}>
              <div className="flex gap-1.5">
                <div className={`w-2 h-2 rounded-full animate-bounce ${theme === 'dark' ? 'bg-zinc-500' : 'bg-zinc-400'}`} style={{ animationDelay: '0ms' }} />
                <div className={`w-2 h-2 rounded-full animate-bounce ${theme === 'dark' ? 'bg-zinc-500' : 'bg-zinc-400'}`} style={{ animationDelay: '150ms' }} />
                <div className={`w-2 h-2 rounded-full animate-bounce ${theme === 'dark' ? 'bg-zinc-500' : 'bg-zinc-400'}`} style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Prompt Display + Actions */}
      {hasPrompt && (
        <div className={`mb-6 p-5 rounded-2xl border ${
          theme === 'dark' ? 'bg-amber-500/5 border-amber-500/30' : 'bg-amber-50/50 border-amber-200'
        }`}>
          <div className="flex items-center justify-between mb-3">
            <span className={`text-sm font-semibold ${theme === 'dark' ? 'text-amber-400' : 'text-amber-600'}`}>
              Research Brief
            </span>
            <div className="flex items-center gap-2">
              {editing ? (
                <>
                  <button onClick={() => { setEditing(false); setEditedPrompt(''); }}
                    className={`text-xs px-3 py-1.5 rounded-lg ${theme === 'dark' ? 'bg-zinc-800 text-zinc-400 hover:text-zinc-200' : 'bg-zinc-200 text-zinc-500 hover:text-zinc-700'}`}>
                    <X className="w-3.5 h-3.5 inline mr-1" />Cancel
                  </button>
                  <button onClick={handleSavePrompt}
                    className="text-xs px-3 py-1.5 rounded-lg bg-emerald-500 text-white hover:bg-emerald-600">
                    <Check className="w-3.5 h-3.5 inline mr-1" />Save
                  </button>
                </>
              ) : (
                <button onClick={() => { setEditing(true); setEditedPrompt(refinedPrompt); }}
                  className={`text-xs px-3 py-1.5 rounded-lg ${theme === 'dark' ? 'bg-zinc-800 text-zinc-400 hover:text-emerald-400' : 'bg-zinc-200 text-zinc-500 hover:text-emerald-600'}`}>
                  <Edit3 className="w-3.5 h-3.5 inline mr-1" />Edit
                </button>
              )}
            </div>
          </div>
          {editing ? (
            <textarea
              value={editedPrompt}
              onChange={(e) => setEditedPrompt(e.target.value)}
              rows={14}
              className={`w-full px-3 py-2 rounded-xl border text-xs font-mono leading-relaxed resize-none focus:outline-none focus:ring-2 focus:ring-emerald-500/20 ${inputClass}`}
            />
          ) : (
            <pre className={`text-xs leading-relaxed font-mono whitespace-pre-wrap ${
              theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
            }`}>{refinedPrompt}</pre>
          )}

          {/* Frequency Slider */}
          <div className="mt-4 pt-4 border-t border-amber-500/20">
            <div className="flex items-center gap-4">
              <span className={`text-xs font-medium ${textMuted}`}>Scouting every</span>
              <input type="range" min={1} max={30} value={frequency}
                onChange={(e) => setFrequency(Number(e.target.value))}
                className="flex-1 accent-emerald-500" />
              <span className={`text-xs font-semibold min-w-[60px] text-right ${
                theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
              }`}>{frequency === 1 ? '1 day' : `${frequency} days`}</span>
            </div>
          </div>

          {/* Start Research */}
          <button
            onClick={handleStartResearch}
            disabled={starting}
            className="mt-4 w-full flex items-center justify-center gap-2 px-6 py-3 rounded-xl font-semibold text-sm transition-all duration-200 bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600 shadow-lg shadow-emerald-500/25 disabled:opacity-50">
            {starting ? (
              <>
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Deploying Scouts...
              </>
            ) : (
              <>
                <Sparkles className="w-4 h-4" />
                Start Research
              </>
            )}
          </button>
          <p className={`mt-2 text-center text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>
            Or keep chatting below to refine the brief further
          </p>
        </div>
      )}

      {error && (
        <div className={`mb-4 px-4 py-3 rounded-xl text-sm border ${
          theme === 'dark' ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' : 'bg-rose-50 border-rose-200 text-rose-700'
        }`}>{error}</div>
      )}

      {/* Chat Input */}
      <div className="flex gap-3">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleChat(); } }}
          placeholder="Ask a question or add details..."
          disabled={busy}
          className={`flex-1 px-4 py-3 rounded-xl border text-sm focus:outline-none focus:ring-2 focus:ring-emerald-500/20 ${inputClass}`}
        />
        <button
          onClick={handleChat}
          disabled={!input.trim() || busy}
          className={`flex items-center justify-center px-5 py-3 rounded-xl font-medium text-sm transition-all duration-200 ${
            input.trim() && !busy
              ? 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600'
              : theme === 'dark' ? 'bg-zinc-800 text-zinc-600' : 'bg-zinc-200 text-zinc-400'
          }`}>
          <Send className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}
