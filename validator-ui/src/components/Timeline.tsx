import { ScoutCycle } from '../types';
import { useTheme } from '../context/ThemeContext';
import { Clock, CheckCircle2 } from '../components/Icons';

interface TimelineProps {
  cycles: ScoutCycle[];
  selectedCycleId: string;
  onSelectCycle: (cycleId: string) => void;
}

export function Timeline({ cycles, selectedCycleId, onSelectCycle }: TimelineProps) {
  const { theme } = useTheme();
  const sortedCycles = [...cycles].sort((a, b) => a.day - b.day);

  return (
    <div className={`p-5 rounded-2xl border transition-all duration-300 ${
      theme === 'dark'
        ? 'bg-zinc-900 border-zinc-800'
        : 'bg-white border-zinc-200'
    }`}>
      <div className="flex items-center gap-2 mb-4">
        <Clock className={`w-4 h-4 ${
          theme === 'dark' ? 'text-zinc-400' : 'text-zinc-500'
        }`} />
        <span className={`text-sm font-semibold ${
          theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
        }`}>
          Scout Timeline
        </span>
      </div>

      <div className="flex overflow-x-auto pb-2 -mx-2 px-2 gap-2">
        {sortedCycles.map((cycle, index) => {
          const isSelected = selectedCycleId === cycle.id;

          return (
            <button
              key={cycle.id}
              onClick={() => onSelectCycle(cycle.id)}
              className={`relative flex-shrink-0 group px-4 py-3 rounded-xl transition-all duration-200 ${
                isSelected
                  ? theme === 'dark'
                    ? 'bg-gradient-to-br from-emerald-500/30 to-teal-500/20 border border-emerald-500/50'
                    : 'bg-gradient-to-br from-emerald-50 to-teal-50 border border-emerald-300'
                  : theme === 'dark'
                    ? 'bg-zinc-800 border border-zinc-700 hover:border-zinc-600'
                    : 'bg-zinc-50 border border-zinc-200 hover:border-zinc-300'
              }`}
            >
              <div className="relative z-10">
                <div className="flex items-center gap-2 mb-1">
                  {isSelected && (
                    <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                  )}
                  <span className={`text-sm font-semibold ${
                    isSelected
                      ? theme === 'dark' ? 'text-emerald-400' : 'text-emerald-700'
                      : theme === 'dark' ? 'text-zinc-300' : 'text-zinc-700'
                  }`}>
                    Day {cycle.day}
                  </span>
                </div>
                <p className={`text-xs ${
                  isSelected
                    ? theme === 'dark' ? 'text-emerald-300' : 'text-emerald-600'
                    : theme === 'dark' ? 'text-zinc-500' : 'text-zinc-500'
                }`}>
                  {cycle.label}
                </p>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`text-xs font-medium px-1.5 py-0.5 rounded ${
                    theme === 'dark' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-emerald-100 text-emerald-600'
                  }`}>
                    +{cycle.pros.length}
                  </span>
                  <span className={`text-xs font-medium px-1.5 py-0.5 rounded ${
                    theme === 'dark' ? 'bg-rose-500/20 text-rose-400' : 'bg-rose-100 text-rose-600'
                  }`}>
                    -{cycle.cons.length}
                  </span>
                </div>
              </div>

              {/* Connection line */}
              {index < sortedCycles.length - 1 && (
                <div className={`absolute top-1/2 -right-2 w-4 h-0.5 ${
                  theme === 'dark' ? 'bg-zinc-700' : 'bg-zinc-200'
                }`} />
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
