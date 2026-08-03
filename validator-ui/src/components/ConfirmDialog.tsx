import { AlertTriangle, X } from './Icons';
import { useTheme } from '../context/ThemeContext';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const { theme } = useTheme();
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onCancel} />
      <div className={`relative w-full max-w-md rounded-2xl border shadow-2xl ${
        theme === 'dark' ? 'bg-zinc-900 border-zinc-800' : 'bg-white border-zinc-200'
      }`}>
        <div className="flex items-start gap-4 p-6">
          <div className={`p-2.5 rounded-xl flex-shrink-0 ${
            danger
              ? theme === 'dark' ? 'bg-rose-500/15' : 'bg-rose-100'
              : theme === 'dark' ? 'bg-amber-500/15' : 'bg-amber-100'
          }`}>
            <AlertTriangle className={`w-5 h-5 ${
              danger
                ? theme === 'dark' ? 'text-rose-400' : 'text-rose-600'
                : theme === 'dark' ? 'text-amber-400' : 'text-amber-600'
            }`} />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className={`text-base font-semibold mb-1 ${
              theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
            }`}>{title}</h3>
            <p className={`text-sm leading-relaxed ${
              theme === 'dark' ? 'text-zinc-400' : 'text-zinc-600'
            }`}>{message}</p>
          </div>
          <button
            onClick={onCancel}
            className={`p-1.5 rounded-lg flex-shrink-0 transition-colors ${
              theme === 'dark' ? 'text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800' : 'text-zinc-400 hover:text-zinc-600 hover:bg-zinc-100'
            }`}>
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className={`flex justify-end gap-3 px-6 py-4 border-t ${
          theme === 'dark' ? 'border-zinc-800' : 'border-zinc-100'
        }`}>
          <button
            onClick={onCancel}
            className={`px-4 py-2 rounded-xl text-sm font-medium transition-all duration-200 ${
              theme === 'dark' ? 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700' : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200'
            }`}>
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            className={`px-4 py-2 rounded-xl text-sm font-semibold transition-all duration-200 ${
              danger
                ? 'bg-rose-500 text-white hover:bg-rose-600 shadow-lg shadow-rose-500/25'
                : 'bg-gradient-to-r from-emerald-500 to-teal-500 text-white hover:from-emerald-600 hover:to-teal-600 shadow-lg shadow-emerald-500/25'
            }`}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
