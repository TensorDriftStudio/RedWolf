import React from 'react';
import { Activity, ShieldCheck, RefreshCw, Search } from 'lucide-react';

interface NavbarProps {
  onRefresh: () => void;
  isRefreshing: boolean;
  searchTerm: string;
  setSearchTerm: (s: string) => void;
}

export const Navbar: React.FC<NavbarProps> = ({ onRefresh, isRefreshing, searchTerm, setSearchTerm }) => {
  return (
    <header className="sticky top-0 z-40 w-full border-b border-surface-border bg-surface-ground/90 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
        
        {/* Brand / Logo */}
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-3">
            <img 
              src="/assets/redwolf-horizontal-dark.svg" 
              alt="RedWolf" 
              className="h-9 w-auto object-contain"
            />
          </div>

          <div className="hidden h-5 w-px bg-surface-border md:block" />

          {/* Network Health Indicators */}
          <div className="hidden items-center gap-4 text-xs lg:flex">
            <div className="flex items-center gap-1.5 rounded-full bg-surface-card px-2.5 py-1 text-slate-300 border border-surface-borderMuted">
              <span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
              <span>DHCP / TFTP: <strong className="text-white font-mono">192.168.50.0/24</strong></span>
            </div>
            <div className="flex items-center gap-1.5 rounded-full bg-surface-card px-2.5 py-1 text-slate-300 border border-surface-borderMuted">
              <Activity className="h-3.5 w-3.5 text-redwolf-primary" />
              <span>Discovery Engine: <strong className="text-white">Active (KCS/PXE)</strong></span>
            </div>
          </div>
        </div>

        {/* Search & Actions */}
        <div className="flex items-center gap-3">
          {/* Quick Search */}
          <div className="relative w-48 sm:w-64">
            <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              type="text"
              placeholder="Search model, MAC, IP..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full rounded-lg border border-surface-border bg-surface-card py-1.5 pl-9 pr-3 text-xs text-white placeholder-slate-400 focus:border-redwolf-primary focus:outline-none focus:ring-1 focus:ring-redwolf-primary"
            />
          </div>

          {/* Refresh Button */}
          <button
            onClick={onRefresh}
            title="Scan & Refresh Provisioning Subnet"
            className="flex items-center gap-1.5 rounded-lg border border-surface-border bg-surface-card px-3 py-1.5 text-xs font-semibold text-slate-200 transition-all hover:border-slate-600 hover:bg-surface-panel hover:text-white"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${isRefreshing ? 'animate-spin text-redwolf-primary' : 'text-slate-400'}`} />
            <span className="hidden sm:inline">Scan Network</span>
          </button>

          {/* Console Badge */}
          <div className="flex items-center gap-2 rounded-lg border border-redwolf-primary/30 bg-redwolf-primary/10 px-3 py-1.5 text-xs font-medium text-red-300">
            <ShieldCheck className="h-3.5 w-3.5 text-redwolf-primary" />
            <span className="hidden md:inline font-mono">v0.9.4-alpha</span>
          </div>
        </div>

      </div>
    </header>
  );
};
