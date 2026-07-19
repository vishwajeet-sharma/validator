import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PlusCircle, Sparkles } from '../components/Icons';
import { useTheme } from '../context/ThemeContext';
import { useIdeaStore } from '../store/useIdeaStore';
import { ApiError } from '../lib/api';

const intervalOptions: { label: string; days: number }[] = [
  { label: 'Daily', days: 1 },
  { label: '3 Days', days: 3 },
  { label: '7 Days', days: 7 },
  { label: '14 Days', days: 14 },
];

export function NewIdeaForm() {
  const { theme } = useTheme();
  const createIdea = useIdeaStore((s) => s.createIdea);
  const navigate = useNavigate();

  const [description, setDescription] = useState('');
  const [frequencyDays, setFrequencyDays] = useState(7);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const isValid = description.trim().length >= 12;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isValid || isSubmitting) return;

    setIsSubmitting(true);
    setSubmitError(null);

    try {
      const id = await createIdea({
        description: description.trim(),
        scoutingFrequencyDays: frequencyDays,
      });
      navigate(`/idea/${id}`);
    } catch (err) {
      setSubmitError(
        err instanceof ApiError
          ? err.message
          : 'Something went wrong while setting up your scout. Please try again.'
      );
      setIsSubmitting(false);
    }
  };

  return (
    <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <div className={`p-2 rounded-xl ${theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'}`}>
            <PlusCircle className={`w-6 h-6 ${theme === 'dark' ? 'text-teal-400' : 'text-teal-600'}`} />
          </div>
          <h1 className={`text-3xl font-bold tracking-tight ${theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'}`}>
            New Idea
          </h1>
        </div>
        <p className={`text-lg ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'}`}>
          Describe your idea. We'll research the market and deploy two scouts — one tracking demand, one tracking threats.
        </p>
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Description */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>
            Your Idea
          </label>
          <p className={`text-sm mb-3 ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}`}>
            What problem does it solve? Who is it for? The richer the description, the sharper the scouts.
          </p>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="e.g. A SaaS that automatically reviews pull requests using LLMs, catching bugs and suggesting improvements before merge. Aimed at mid-size engineering teams who want to ship faster without sacrificing quality."
            rows={7}
            className={`w-full px-4 py-3 rounded-xl border text-base transition-all duration-200 focus:outline-none focus:ring-2 resize-none ${
              theme === 'dark'
                ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-emerald-500 focus:ring-emerald-500/20'
                : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:ring-emerald-500/20'
            }`}
          />
          <div className={`mt-2 text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>
            {description.trim().length} characters
          </div>
        </div>

        {/* Scouting Frequency */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'}`}>
            Scouting Frequency
          </label>
          <p className={`text-sm mb-4 ${theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'}`}>
            How often should each scout refresh its market sweep?
          </p>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {intervalOptions.map((opt) => {
              const isSelected = frequencyDays === opt.days;
              return (
                <button
                  key={opt.days}
                  type="button"
                  onClick={() => setFrequencyDays(opt.days)}
                  className={`px-4 py-3 rounded-xl border text-sm font-semibold transition-all duration-200 ${
                    isSelected
                      ? 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white border-transparent shadow-lg shadow-emerald-500/25'
                      : theme === 'dark'
                        ? 'bg-zinc-800 border-zinc-700 text-zinc-300 hover:border-zinc-600'
                        : 'bg-zinc-50 border-zinc-200 text-zinc-600 hover:border-zinc-300'
                  }`}
                >
                  {opt.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* Submit */}
        {submitError && (
          <div className={`px-4 py-3 rounded-xl text-sm border ${
            theme === 'dark'
              ? 'bg-rose-500/10 border-rose-500/30 text-rose-400'
              : 'bg-rose-50 border-rose-200 text-rose-700'
          }`}>
            {submitError}
          </div>
        )}
        <div className="flex justify-end gap-3 pt-4">
          <button
            type="button"
            onClick={() => navigate('/')}
            className={`px-6 py-3 rounded-xl font-medium text-sm transition-all duration-200 ${
              theme === 'dark' ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700' : 'bg-zinc-200 text-zinc-600 hover:bg-zinc-300'
            }`}
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!isValid || isSubmitting}
            className={`flex items-center gap-2 px-6 py-3 rounded-xl font-semibold text-sm transition-all duration-200 ${
              isValid && !isSubmitting
                ? 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600 shadow-lg shadow-emerald-500/25'
                : theme === 'dark'
                  ? 'bg-zinc-800 text-zinc-500 cursor-not-allowed'
                  : 'bg-zinc-200 text-zinc-400 cursor-not-allowed'
            }`}
          >
            {isSubmitting ? (
              <>
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Deploying Scouts…
              </>
            ) : (
              <>
                <Sparkles className="w-4 h-4" />
                Deploy Scouts
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}
