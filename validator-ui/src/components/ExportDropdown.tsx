import { useState, useRef, useEffect } from 'react';
import { ChevronDown, Copy, Check, ExternalLink } from './Icons';
import { useTheme } from '../context/ThemeContext';
import type { IdeaDetail } from '../types';

interface ExportDropdownProps {
  idea: IdeaDetail;
}

export function ExportDropdown({ idea }: ExportDropdownProps) {
  const { theme } = useTheme();
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const buildMarkdown = () => {
    let md = `# ${idea.title}\n\n`;
    md += `## Description\n${idea.description}\n\n`;
    if (idea.refinedPrompt) {
      md += `## Research Brief\n${idea.refinedPrompt}\n\n`;
    }
    md += `## Pro Signals (${idea.totalPros})\n`;
    if (idea.recentPros.length === 0) {
      md += '_No signals yet._\n\n';
    } else {
      for (const f of idea.recentPros) {
        md += `### ${f.platform} - ${f.sourceTitle}\n`;
        md += `> "${f.quote}"\n`;
        if (f.reason) md += `**Why it matters:** ${f.reason}\n`;
        md += `[Source](${f.sourceUrl})\n\n`;
      }
    }
    md += `## Con Signals (${idea.totalCons})\n`;
    if (idea.recentCons.length === 0) {
      md += '_No signals yet._\n\n';
    } else {
      for (const f of idea.recentCons) {
        md += `### ${f.platform} - ${f.sourceTitle}\n`;
        md += `> "${f.quote}"\n`;
        if (f.reason) md += `**Why it matters:** ${f.reason}\n`;
        md += `[Source](${f.sourceUrl})\n\n`;
      }
    }
    return md;
  };

  const copyAndOpen = async (label: string, url?: string) => {
    const md = buildMarkdown();
    try {
      await navigator.clipboard.writeText(md);
      setCopied(label);
      setTimeout(() => setCopied(null), 2000);
    } catch {
      // clipboard not available
    }
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer');
    }
    setOpen(false);
  };

  const btnClass = `text-xs font-medium px-3 py-2 rounded-xl border transition-colors flex items-center gap-1.5 ${
    theme === 'dark'
      ? 'text-zinc-400 border-zinc-700 hover:text-emerald-300 hover:border-emerald-500/40 hover:bg-emerald-500/10'
      : 'text-zinc-500 border-zinc-300 hover:text-emerald-600 hover:border-emerald-300 hover:bg-emerald-50'
  }`;

  const itemClass = `w-full flex items-center gap-2 px-4 py-2.5 text-left text-sm transition-colors ${
    theme === 'dark' ? 'text-zinc-300 hover:bg-zinc-800' : 'text-zinc-600 hover:bg-zinc-100'
  }`;

  return (
    <div ref={ref} className="relative">
      <button type="button" onClick={() => setOpen(!open)} className={btnClass}>
        {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <ExternalLink className="w-3.5 h-3.5" />}
        {copied ? 'Copied!' : 'Export'}
        <ChevronDown className="w-3 h-3" />
      </button>
      {open && (
        <div className={`absolute right-0 top-full mt-1 w-56 rounded-xl border shadow-xl z-20 py-1 ${
          theme === 'dark' ? 'bg-zinc-900 border-zinc-700' : 'bg-white border-zinc-200'
        }`}>
          <button className={itemClass} onClick={() => copyAndOpen('markdown')}>
            <Copy className="w-4 h-4" /> Copy as Markdown
          </button>
          <div className={`my-1 border-t ${theme === 'dark' ? 'border-zinc-800' : 'border-zinc-100'}`} />
          <button className={itemClass} onClick={() => copyAndOpen('chatgpt', 'https://chat.openai.com')}>
            <ExternalLink className="w-4 h-4" /> Open in ChatGPT
          </button>
          <button className={itemClass} onClick={() => copyAndOpen('claude', 'https://claude.ai')}>
            <ExternalLink className="w-4 h-4" /> Open in Claude
          </button>
          <button className={itemClass} onClick={() => copyAndOpen('gemini', 'https://gemini.google.com')}>
            <ExternalLink className="w-4 h-4" /> Open in Gemini
          </button>
        </div>
      )}
    </div>
  );
}
