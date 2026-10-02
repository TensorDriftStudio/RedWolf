import React, { useState } from 'react';
import { RefreshCw, Search, Radio, Settings, Server, LogOut } from 'lucide-react';
import type { User } from '../types';

interface NavbarProps {
  onRefresh: () => void;
  isRefreshing: boolean;
  searchTerm: string;
  setSearchTerm: (s: string) => void;
  isConnected: boolean;
  currentView: 'fleet' | 'settings';
  setCurrentView: (v: 'fleet' | 'settings') => void;
  user: User | null;
  onLogout: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  onRefresh,
  isRefreshing,
  searchTerm,
  setSearchTerm,
  isConnected,
  currentView,
  setCurrentView,
  user,
  onLogout,
}) => {
  const [showUserMenu, setShowUserMenu] = useState<boolean>(false);

  return (
    <header className="sticky top-0 z-40 w-full border-b border-enterprise-border bg-enterprise-header">
      <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4 sm:px-6">
        
        {/* Brand & Navigation */}
        <div className="flex items-center gap-5">
          <div className="flex items-center gap-3 cursor-pointer" onClick={() => setCurrentView('fleet')}>
            <img 
              src="/assets/redwolf-horizontal-dark.svg" 
              alt="RedWolf" 
              className="h-7 w-auto object-contain"
            />
            <span className="hidden sm:inline-block rounded-sm bg-[#1e2430] border border-enterprise-border px-1.5 py-0.5 font-mono text-[10px] text-enterprise-textMuted uppercase tracking-wider">
              GUI v1.1.0 Enterprise
            </span>
          </div>

          <div className="hidden h-4 w-px bg-enterprise-border md:block" />

          {/* Console View Navigation */}
          <nav className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setCurrentView('fleet')}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-sm transition-colors ${
                currentView === 'fleet'
                  ? 'bg-slate-800 text-white border border-slate-700'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900/50'
              }`}
            >
              <Server className="w-3.5 h-3.5" />
              <span>Fleet Inventory</span>
            </button>
            <button
              type="button"
              onClick={() => setCurrentView('settings')}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-sm transition-colors ${
                currentView === 'settings'
                  ? 'bg-slate-800 text-white border border-slate-700'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900/50'
              }`}
            >
              <Settings className="w-3.5 h-3.5" />
              <span>Settings & Auth</span>
            </button>
          </nav>
        </div>

        {/* Search, Status & Actions */}
        <div className="flex items-center gap-3">
          {/* Quick Filter Search (only on fleet view) */}
          {currentView === 'fleet' && (
            <div className="relative w-40 sm:w-56">
              <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-enterprise-textDim" />
              <input
                type="text"
                placeholder="Search serial, MAC, IP..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel py-1 pl-8 pr-3 text-xs text-white placeholder-enterprise-textDim focus:border-[#1f6feb] focus:bg-[#1b212c] focus:outline-none"
              />
            </div>
          )}

          {/* Scan Network Trigger */}
          {currentView === 'fleet' && (
            <button
              onClick={onRefresh}
              disabled={isRefreshing}
              title="Scan provisioning broadcast domain"
              className="flex items-center gap-1.5 rounded-sm border border-enterprise-border bg-enterprise-panel px-3 py-1 text-xs font-medium text-slate-200 hover:bg-enterprise-hover hover:border-slate-500 transition-colors disabled:opacity-50"
            >
              <RefreshCw className={`h-3 w-3 text-enterprise-textMuted ${isRefreshing ? 'animate-spin text-redwolf-primary' : ''}`} />
              <span className="hidden sm:inline">Scan</span>
            </button>
          )}

          {/* Real-time WebSocket Status */}
          <div 
            className={`flex items-center gap-1.5 rounded-sm border px-2 py-1 text-[11px] font-mono ${
              isConnected 
                ? 'border-[#1b3d2f] bg-[#11261d] text-[#7ee787]' 
                : 'border-[#3c1618] bg-[#221012] text-[#f85149]'
            }`}
            title={isConnected ? 'Connected to RedWolf Core live event bus' : 'Reconnecting to event bus...'}
          >
            <Radio className={`h-3 w-3 ${isConnected ? 'animate-pulse text-[#3fb950]' : 'text-[#da3633]'}`} />
            <span className="hidden lg:inline">{isConnected ? 'LIVE' : 'OFFLINE'}</span>
          </div>

          {/* User Profile & Sign Out Dropdown */}
          {user && (
            <div className="relative">
              <button
                type="button"
                onClick={() => setShowUserMenu(!showUserMenu)}
                className="flex items-center gap-2 pl-2 pr-2.5 py-1 rounded-sm border border-slate-700 bg-slate-800/80 hover:bg-slate-700/80 transition-colors text-xs text-slate-200"
              >
                <div className="w-5 h-5 rounded-full bg-red-900 border border-red-700 flex items-center justify-center text-[10px] font-bold text-red-200 uppercase">
                  {user.username.charAt(0)}
                </div>
                <span className="font-medium hidden sm:inline">{user.username}</span>
                <span className="px-1 py-0.2 rounded text-[10px] bg-slate-900 border border-slate-700 text-slate-400 font-mono">
                  {user.source === 'ACTIVE_DIRECTORY' ? 'AD' : user.source}
                </span>
              </button>

              {showUserMenu && (
                <>
                  <div
                    className="fixed inset-0 z-40"
                    onClick={() => setShowUserMenu(false)}
                  />
                  <div className="absolute right-0 mt-1.5 w-56 bg-slate-900 border border-slate-700 shadow-xl rounded-sm py-1.5 z-50 text-xs">
                    <div className="px-3 py-2 border-b border-slate-800">
                      <div className="font-semibold text-slate-200">{user.displayName || user.username}</div>
                      <div className="text-[11px] text-slate-400 truncate">{user.email || `${user.username}@redwolf.internal`}</div>
                      <div className="mt-1.5 flex items-center gap-1.5">
                        <span className="px-1.5 py-0.2 bg-red-950/80 border border-red-800 text-red-300 rounded text-[10px] font-mono">
                          {user.role}
                        </span>
                        <span className="text-[10px] text-slate-500 font-mono">
                          Auth: {user.source}
                        </span>
                      </div>
                    </div>

                    <button
                      type="button"
                      onClick={() => {
                        setShowUserMenu(false);
                        setCurrentView('settings');
                      }}
                      className="w-full text-left px-3 py-2 text-slate-300 hover:bg-slate-800 flex items-center gap-2"
                    >
                      <Settings className="w-3.5 h-3.5 text-slate-400" />
                      <span>Appliance Settings</span>
                    </button>

                    <div className="border-t border-slate-800 my-1" />

                    <button
                      type="button"
                      onClick={() => {
                        setShowUserMenu(false);
                        onLogout();
                      }}
                      className="w-full text-left px-3 py-2 text-red-400 hover:bg-red-950/40 hover:text-red-300 flex items-center gap-2"
                    >
                      <LogOut className="w-3.5 h-3.5" />
                      <span>Sign Out</span>
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </div>

      </div>
    </header>
  );
};
