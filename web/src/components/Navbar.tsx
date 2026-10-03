import React, { useState } from 'react';
import { RefreshCw, Search, LogOut, ChevronRight } from 'lucide-react';
import type { User } from '../types';

interface NavbarProps {
  onRefresh: () => void;
  isRefreshing: boolean;
  searchTerm: string;
  setSearchTerm: (s: string) => void;
  currentView: 'fleet' | 'settings';
  settingsTab: 'auth' | 'network' | 'storage' | 'templates';
  user: User | null;
  onLogout: () => void;
  onOpenSettings: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  onRefresh,
  isRefreshing,
  searchTerm,
  setSearchTerm,
  currentView,
  settingsTab,
  user,
  onLogout,
  onOpenSettings,
}) => {
  const [showUserMenu, setShowUserMenu] = useState<boolean>(false);

  // Derive human-readable breadcrumb trail
  const getBreadcrumb = () => {
    if (currentView === 'fleet') {
      return (
        <div className="flex items-center gap-1.5 text-xs text-slate-400">
          <span>Inventory</span>
          <ChevronRight className="w-3 h-3 text-slate-600" />
          <span className="font-semibold text-white">Servers</span>
        </div>
      );
    }
    const tabLabels: Record<string, string> = {
      auth: 'Authentication',
      network: 'Network & DHCP',
      storage: 'OS Images & Storage',
      templates: 'Cloud-Init Templates',
    };
    return (
      <div className="flex items-center gap-1.5 text-xs text-slate-400">
        <span>Administration</span>
        <ChevronRight className="w-3 h-3 text-slate-600" />
        <span className="font-semibold text-white">{tabLabels[settingsTab] || 'Settings'}</span>
      </div>
    );
  };

  return (
    <header className="h-13 border-b border-[#1e2430] bg-[#11141c] sticky top-0 z-20 flex items-center justify-between px-4 sm:px-6">
      
      {/* Left: Breadcrumbs */}
      <div className="flex items-center gap-3">
        {getBreadcrumb()}
      </div>

      {/* Right: Quick Search, Scan & User Account */}
      <div className="flex items-center gap-3">
        
        {/* Quick Filter Search (fleet view) */}
        {currentView === 'fleet' && (
          <div className="relative w-48 sm:w-64">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            <input
              type="text"
              placeholder="Search serial, MAC, IP, model..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full rounded-sm border border-[#232b3b] bg-[#161b24] py-1 pl-8 pr-3 text-xs text-white placeholder-slate-500 focus:border-redwolf-primary focus:bg-[#181f2b] focus:outline-none transition-colors"
            />
          </div>
        )}

        {/* Scan Network Trigger */}
        {currentView === 'fleet' && (
          <button
            onClick={onRefresh}
            disabled={isRefreshing}
            title="Scan provisioning subnet for network boot beacons"
            className="flex items-center gap-1.5 rounded-sm border border-[#283244] bg-[#161c26] px-2.5 py-1 text-xs font-medium text-slate-200 hover:bg-[#1d2533] hover:border-slate-500 transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`h-3 w-3 text-slate-400 ${isRefreshing ? 'animate-spin text-redwolf-primary' : ''}`} />
            <span className="hidden sm:inline">Scan Network</span>
          </button>
        )}

        {/* User Profile Menu */}
        {user && (
          <div className="relative">
            <button
              type="button"
              onClick={() => setShowUserMenu(!showUserMenu)}
              className="flex items-center gap-2 pl-2 pr-2.5 py-1 rounded-sm border border-[#232b3b] bg-[#161b24] hover:bg-[#1c2330] transition-colors text-xs text-slate-200"
            >
              <div className="w-5 h-5 rounded-full bg-red-950 border border-red-800 flex items-center justify-center text-[10px] font-bold text-red-200 uppercase">
                {user.username.charAt(0)}
              </div>
              <span className="font-medium hidden sm:inline">{user.username}</span>
              <span className="px-1 py-0.2 rounded text-[9px] bg-[#0c0e14] border border-[#232b3b] text-slate-400 font-mono">
                {user.source === 'ACTIVE_DIRECTORY' ? 'AD' : user.source}
              </span>
            </button>

            {showUserMenu && (
              <>
                <div
                  className="fixed inset-0 z-40"
                  onClick={() => setShowUserMenu(false)}
                />
                <div className="absolute right-0 mt-1.5 w-56 bg-[#131720] border border-[#242c3d] shadow-xl rounded-sm py-1 z-50 text-xs">
                  <div className="px-3 py-2 border-b border-[#1f2635]">
                    <div className="font-semibold text-slate-200">{user.displayName || user.username}</div>
                    <div className="text-[11px] text-slate-400 truncate">{user.email || `${user.username}@redwolf.internal`}</div>
                    <div className="mt-1 flex items-center gap-1.5">
                      <span className="px-1.5 py-0.2 bg-red-950/80 border border-red-800 text-red-300 rounded text-[9px] font-mono">
                        {user.role}
                      </span>
                    </div>
                  </div>

                  <button
                    type="button"
                    onClick={() => {
                      setShowUserMenu(false);
                      onOpenSettings();
                    }}
                    className="w-full text-left px-3 py-1.5 text-slate-300 hover:bg-[#1a212e] flex items-center gap-2 transition-colors"
                  >
                    <span>Settings</span>
                  </button>

                  <div className="border-t border-[#1f2635] my-1" />

                  <button
                    type="button"
                    onClick={() => {
                      setShowUserMenu(false);
                      onLogout();
                    }}
                    className="w-full text-left px-3 py-1.5 text-red-400 hover:bg-red-950/40 hover:text-red-300 flex items-center gap-2 transition-colors"
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
    </header>
  );
};
