import { ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { LayoutDashboard, PlusCircle, Moon, Sun, ValidatorLogo } from './Icons';

interface LayoutProps {
  children: ReactNode;
}

export function Layout({ children }: LayoutProps) {
  const { theme, toggleTheme } = useTheme();

  const navItems = [
    { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
    { to: '/new', icon: PlusCircle, label: 'New Idea' },
  ];

  return (
    <div className={`min-h-screen transition-colors duration-300 ${
      theme === 'dark'
        ? 'bg-zinc-950 text-zinc-100'
        : 'bg-zinc-50 text-zinc-900'
    }`}>
      {/* Top Navigation */}
      <header className={`fixed top-0 left-0 right-0 z-50 h-16 border-b backdrop-blur-md transition-colors duration-300 ${
        theme === 'dark'
          ? 'bg-zinc-950/80 border-zinc-800'
          : 'bg-white/80 border-zinc-200'
      }`}>
        <div className="h-full max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex items-center justify-between">
          <NavLink to="/" className="flex items-center gap-3 group">
            <div className={`w-9 h-9 rounded-xl flex items-center justify-center transition-all duration-300 ${
              theme === 'dark'
                ? 'bg-gradient-to-br from-emerald-500 to-teal-600'
                : 'bg-gradient-to-br from-emerald-400 to-teal-500'
            }`}>
              <ValidatorLogo className="w-5 h-5 text-white" />
            </div>
            <span className={`text-xl font-semibold tracking-tight transition-colors ${
              theme === 'dark' ? 'text-zinc-100' : 'text-zinc-900'
            }`}>
              Validator
            </span>
          </NavLink>

          <nav className="flex items-center gap-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) => `flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
                  isActive
                    ? theme === 'dark'
                      ? 'bg-zinc-800 text-zinc-100'
                      : 'bg-zinc-200 text-zinc-900'
                    : theme === 'dark'
                      ? 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/50'
                      : 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-200/50'
                }`}
              >
                <item.icon className="w-4 h-4" />
                {item.label}
              </NavLink>
            ))}
          </nav>

          <div className="flex items-center gap-3">
            <button
              onClick={toggleTheme}
              className={`p-2 rounded-lg transition-all duration-200 ${
                theme === 'dark'
                  ? 'text-zinc-400 hover:text-yellow-400 hover:bg-zinc-800'
                  : 'text-zinc-600 hover:text-amber-500 hover:bg-zinc-200'
              }`}
              aria-label="Toggle theme"
            >
              {theme === 'dark' ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
            </button>
          </div>
        </div>
      </header>

      <main className="pt-16 min-h-screen">
        {children}
      </main>
    </div>
  );
}

import { useTheme } from '../context/ThemeContext';
