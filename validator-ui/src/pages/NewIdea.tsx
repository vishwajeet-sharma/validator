import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PlusCircle, X, Check, MessageSquare, Youtube, Newspaper, InstagramIcon, Plus, Globe, Trash2 } from '../components/Icons';
import { useTheme } from '../context/ThemeContext';
import { useIdeas } from '../context/IdeaContext';
import { Platform, CustomChannel } from '../types';
import { ApiError } from '../lib/api';

const platformOptions: { value: Platform; label: string; icon: React.ElementType }[] = [
  { value: 'reddit', label: 'Reddit', icon: MessageSquare },
  { value: 'youtube', label: 'YouTube', icon: Youtube },
  { value: 'social', label: 'Social Media', icon: InstagramIcon },
  { value: 'news', label: 'News', icon: Newspaper },
];

export function NewIdea() {
  const { theme } = useTheme();
  const { addIdea } = useIdeas();
  const navigate = useNavigate();

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [keywordInput, setKeywordInput] = useState('');
  const [keywords, setKeywords] = useState<string[]>([]);
  const [frequencyDays, setFrequencyDays] = useState(7);
  const [channels, setChannels] = useState<Platform[]>(['reddit', 'youtube']);
  const [customChannels, setCustomChannels] = useState<CustomChannel[]>([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Custom channel modal state
  const [showCustomModal, setShowCustomModal] = useState(false);
  const [customUrl, setCustomUrl] = useState('');
  const [customLabel, setCustomLabel] = useState('');

  const addKeyword = () => {
    const trimmed = keywordInput.trim();
    if (trimmed && !keywords.includes(trimmed)) {
      setKeywords([...keywords, trimmed]);
      setKeywordInput('');
    }
  };

  const removeKeyword = (keyword: string) => {
    setKeywords(keywords.filter((k) => k !== keyword));
  };

  const handleKeywordKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addKeyword();
    }
  };

  const toggleChannel = (channel: Platform) => {
    if (channels.includes(channel)) {
      if (channels.length > 1) {
        setChannels(channels.filter((c) => c !== channel));
      }
    } else {
      setChannels([...channels, channel]);
    }
  };

  const addCustomChannel = () => {
    if (!customUrl.trim()) return;

    const newChannel: CustomChannel = {
      id: `custom-${Date.now()}`,
      url: customUrl.trim(),
      label: customLabel.trim() || new URL(customUrl.trim()).hostname || 'Custom Source',
    };

    setCustomChannels([...customChannels, newChannel]);
    setCustomUrl('');
    setCustomLabel('');
    setShowCustomModal(false);
  };

  const removeCustomChannel = (id: string) => {
    setCustomChannels(customChannels.filter((c) => c.id !== id));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim() || !description.trim() || keywords.length === 0) return;

    setIsSubmitting(true);
    setSubmitError(null);

    try {
      const created = await addIdea({
        title: title.trim(),
        description: description.trim(),
        keywords,
        scoutingFrequencyDays: frequencyDays,
        channels,
        customChannels,
      });
      navigate(`/idea/${created.id}`);
    } catch (err) {
      setSubmitError(
        err instanceof ApiError
          ? err.message
          : 'Something went wrong while creating your scout. Please try again.'
      );
      setIsSubmitting(false);
    }
  };

  const isValid = title.trim() && description.trim() && keywords.length > 0 && channels.length > 0;

  return (
    <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <div className={`p-2 rounded-xl ${
            theme === 'dark' ? 'bg-teal-500/20' : 'bg-teal-100'
          }`}>
            <PlusCircle className={`w-6 h-6 ${
              theme === 'dark' ? 'text-teal-400' : 'text-teal-600'
            }`} />
          </div>
          <h1 className={`text-3xl font-bold tracking-tight ${
            theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
          }`}>
            New Idea
          </h1>
        </div>
        <p className={`text-lg ${
          theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
        }`}>
          Set up a new market research scout for your idea
        </p>
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Title */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Idea Title
          </label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="e.g., SaaS for Remote Team Collaboration"
            className={`w-full px-4 py-3 rounded-xl border text-lg transition-all duration-200 focus:outline-none focus:ring-2 ${
              theme === 'dark'
                ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-emerald-500 focus:ring-emerald-500/20'
                : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:ring-emerald-500/20'
            }`}
          />
        </div>

        {/* Description */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Core Description
          </label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Describe your idea in a few sentences. What problem does it solve? Who is it for?"
            rows={4}
            className={`w-full px-4 py-3 rounded-xl border text-base transition-all duration-200 focus:outline-none focus:ring-2 resize-none ${
              theme === 'dark'
                ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-emerald-500 focus:ring-emerald-500/20'
                : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:ring-emerald-500/20'
            }`}
          />
        </div>

        {/* Keywords */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Target Audience / Keywords
          </label>
          <p className={`text-sm mb-3 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
          }`}>
            Add keywords that define your target audience and market
          </p>

          <div className="flex flex-wrap gap-2 mb-3">
            {keywords.map((keyword) => (
              <span
                key={keyword}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm font-medium transition-all duration-200 ${
                  theme === 'dark'
                    ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                    : 'bg-emerald-100 text-emerald-700 border border-emerald-200'
                }`}
              >
                {keyword}
                <button
                  type="button"
                  onClick={() => removeKeyword(keyword)}
                  className={`hover:scale-110 transition-transform ${
                    theme === 'dark' ? 'text-emerald-300' : 'text-emerald-500'
                  }`}
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </span>
            ))}
          </div>

          <div className="flex gap-2">
            <input
              type="text"
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              onKeyDown={handleKeywordKeyDown}
              placeholder="Type a keyword and press Enter"
              className={`flex-1 px-4 py-2.5 rounded-xl border text-sm transition-all duration-200 focus:outline-none focus:ring-2 ${
                theme === 'dark'
                  ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-emerald-500 focus:ring-emerald-500/20'
                  : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:ring-emerald-500/20'
              }`}
            />
            <button
              type="button"
              onClick={addKeyword}
              className={`px-4 py-2.5 rounded-xl font-medium text-sm transition-all duration-200 ${
                theme === 'dark'
                  ? 'bg-zinc-700 text-zinc-300 hover:bg-zinc-600'
                  : 'bg-zinc-200 text-zinc-700 hover:bg-zinc-300'
              }`}
            >
              Add
            </button>
          </div>
        </div>

        {/* Scouting Frequency Slider */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-2 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Scouting Frequency
          </label>
          <p className={`text-sm mb-4 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
          }`}>
            How often should validators run scouts for this idea?
          </p>

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <span className={`text-sm ${
                theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
              }`}>
                Every {frequencyDays} {frequencyDays === 1 ? 'day' : 'days'}
              </span>
              <span className={`text-2xl font-bold ${
                theme === 'dark' ? 'text-emerald-400' : 'text-emerald-600'
              }`}>
                {frequencyDays}d
              </span>
            </div>

            <div className="relative">
              <input
                type="range"
                min="1"
                max="30"
                value={frequencyDays}
                onChange={(e) => setFrequencyDays(parseInt(e.target.value))}
                className={`w-full h-2 rounded-full appearance-none cursor-pointer ${
                  theme === 'dark'
                    ? 'bg-zinc-700'
                    : 'bg-zinc-200'
                }`}
                style={{
                  background: theme === 'dark'
                    ? `linear-gradient(to right, #10b981 0%, #10b981 ${((frequencyDays - 1) / 29) * 100}%, #3f3f46 ${((frequencyDays - 1) / 29) * 100}%, #3f3f46 100%)`
                    : `linear-gradient(to right, #10b981 0%, #10b981 ${((frequencyDays - 1) / 29) * 100}%, #e4e4e7 ${((frequencyDays - 1) / 29) * 100}%, #e4e4e7 100%)`,
                }}
              />
              {/* Tick marks */}
              <div className="flex justify-between mt-2">
                <span className={`text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>1d</span>
                <span className={`text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>7d</span>
                <span className={`text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>14d</span>
                <span className={`text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>21d</span>
                <span className={`text-xs ${theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'}`}>30d</span>
              </div>
            </div>
          </div>
        </div>

        {/* Scout Channels */}
        <div className={`p-6 rounded-2xl border transition-all duration-300 ${
          theme === 'dark'
            ? 'bg-zinc-900 border-zinc-800'
            : 'bg-white border-zinc-200'
        }`}>
          <label className={`block text-sm font-semibold mb-3 ${
            theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
          }`}>
            Scout Channels
          </label>
          <p className={`text-sm mb-4 ${
            theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
          }`}>
            Select platforms where Validator will search for market signals
          </p>

          {/* Standard Channels */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
            {platformOptions.map((option) => {
              const Icon = option.icon;
              const isSelected = channels.includes(option.value);
              return (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => toggleChannel(option.value)}
                  className={`relative flex flex-col items-center gap-2 p-4 rounded-xl border transition-all duration-200 ${
                    isSelected
                      ? theme === 'dark'
                        ? 'bg-emerald-500/20 border-emerald-500/50 text-emerald-400'
                        : 'bg-emerald-50 border-emerald-300 text-emerald-700'
                      : theme === 'dark'
                        ? 'bg-zinc-800 border-zinc-700 text-zinc-400 hover:border-zinc-600'
                        : 'bg-zinc-50 border-zinc-200 text-zinc-600 hover:border-zinc-300'
                  }`}
                >
                  <Icon className="w-6 h-6" />
                  <span className="text-sm font-medium">{option.label}</span>
                  {isSelected && (
                    <div className={`absolute top-2 right-2 w-5 h-5 rounded-full flex items-center justify-center ${
                      theme === 'dark' ? 'bg-emerald-500' : 'bg-emerald-500'
                    }`}>
                      <Check className="w-3 h-3 text-white" />
                    </div>
                  )}
                </button>
              );
            })}
          </div>

          {/* Custom Channels */}
          {customChannels.length > 0 && (
            <div className="mb-4">
              <p className={`text-sm font-medium mb-2 ${
                theme === 'dark' ? 'text-zinc-300' : 'text-zinc-600'
              }`}>
                Custom Sources
              </p>
              <div className="space-y-2">
                {customChannels.map((channel) => (
                  <div
                    key={channel.id}
                    className={`flex items-center gap-3 p-3 rounded-xl border transition-all duration-200 ${
                      theme === 'dark'
                        ? 'bg-zinc-800 border-zinc-700'
                        : 'bg-zinc-50 border-zinc-200'
                    }`}
                  >
                    <div className={`p-2 rounded-lg ${
                      theme === 'dark' ? 'bg-violet-500/20' : 'bg-violet-100'
                    }`}>
                      <Globe className={`w-4 h-4 ${
                        theme === 'dark' ? 'text-violet-400' : 'text-violet-600'
                      }`} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className={`text-sm font-medium truncate ${
                        theme === 'dark' ? 'text-zinc-200' : 'text-zinc-700'
                      }`}>
                        {channel.label}
                      </p>
                      <p className={`text-xs truncate ${
                        theme === 'dark' ? 'text-zinc-500' : 'text-zinc-400'
                      }`}>
                        {channel.url}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => removeCustomChannel(channel.id)}
                      className={`p-2 rounded-lg transition-colors ${
                        theme === 'dark'
                          ? 'text-zinc-500 hover:text-rose-400 hover:bg-zinc-700'
                          : 'text-zinc-400 hover:text-rose-500 hover:bg-zinc-200'
                      }`}
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Add Custom Channel Button */}
          <button
            type="button"
            onClick={() => setShowCustomModal(true)}
            className={`flex items-center gap-2 px-4 py-3 rounded-xl border-2 border-dashed transition-all duration-200 w-full justify-center ${
              theme === 'dark'
                ? 'border-zinc-700 text-zinc-400 hover:border-violet-500 hover:text-violet-400 hover:bg-violet-500/5'
                : 'border-zinc-300 text-zinc-500 hover:border-violet-400 hover:text-violet-600 hover:bg-violet-50'
            }`}
          >
            <Plus className="w-5 h-5" />
            <span className="font-medium">Add Custom Source</span>
          </button>
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
              theme === 'dark'
                ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700'
                : 'bg-zinc-200 text-zinc-600 hover:bg-zinc-300'
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
                Creating...
              </>
            ) : (
              <>
                <PlusCircle className="w-4 h-4" />
                Create Scout
              </>
            )}
          </button>
        </div>
      </form>

      {/* Custom Channel Modal */}
      {showCustomModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            className="absolute inset-0 bg-black/50 backdrop-blur-sm"
            onClick={() => setShowCustomModal(false)}
          />
          <div className={`relative w-full max-w-md p-6 rounded-2xl border shadow-2xl ${
            theme === 'dark'
              ? 'bg-zinc-900 border-zinc-700'
              : 'bg-white border-zinc-200'
          }`}>
            <div className="flex items-center justify-between mb-4">
              <h3 className={`text-lg font-semibold ${
                theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
              }`}>
                Add Custom Source
              </h3>
              <button
                type="button"
                onClick={() => setShowCustomModal(false)}
                className={`p-2 rounded-lg transition-colors ${
                  theme === 'dark'
                    ? 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'
                    : 'text-zinc-500 hover:text-zinc-700 hover:bg-zinc-100'
                }`}
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <p className={`text-sm mb-4 ${
              theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
            }`}>
              Add a custom URL to scout, such as a competitor's website, industry blog, or forum.
            </p>

            <div className="space-y-4">
              <div>
                <label className={`block text-sm font-medium mb-1.5 ${
                  theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
                }`}>
                  URL
                </label>
                <input
                  type="url"
                  value={customUrl}
                  onChange={(e) => setCustomUrl(e.target.value)}
                  placeholder="https://competitor.com or https://blog.example.com"
                  className={`w-full px-4 py-2.5 rounded-xl border text-sm transition-all duration-200 focus:outline-none focus:ring-2 ${
                    theme === 'dark'
                      ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-violet-500 focus:ring-violet-500/20'
                      : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-violet-500 focus:ring-violet-500/20'
                  }`}
                />
              </div>

              <div>
                <label className={`block text-sm font-medium mb-1.5 ${
                  theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
                }`}>
                  Label (optional)
                </label>
                <input
                  type="text"
                  value={customLabel}
                  onChange={(e) => setCustomLabel(e.target.value)}
                  placeholder="e.g., Competitor XYZ, Industry News Blog"
                  className={`w-full px-4 py-2.5 rounded-xl border text-sm transition-all duration-200 focus:outline-none focus:ring-2 ${
                    theme === 'dark'
                      ? 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder-zinc-500 focus:border-violet-500 focus:ring-violet-500/20'
                      : 'bg-zinc-50 border-zinc-300 text-zinc-900 placeholder-zinc-400 focus:border-violet-500 focus:ring-violet-500/20'
                  }`}
                />
              </div>
            </div>

            <div className="flex justify-end gap-3 mt-6">
              <button
                type="button"
                onClick={() => setShowCustomModal(false)}
                className={`px-4 py-2 rounded-xl font-medium text-sm transition-colors ${
                  theme === 'dark'
                    ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700'
                    : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200'
                }`}
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={addCustomChannel}
                disabled={!customUrl.trim()}
                className={`flex items-center gap-2 px-4 py-2 rounded-xl font-medium text-sm transition-all duration-200 ${
                  customUrl.trim()
                    ? 'bg-violet-500 text-white hover:bg-violet-600'
                    : theme === 'dark'
                      ? 'bg-zinc-800 text-zinc-500 cursor-not-allowed'
                      : 'bg-zinc-200 text-zinc-400 cursor-not-allowed'
                }`}
              >
                <Plus className="w-4 h-4" />
                Add Source
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
