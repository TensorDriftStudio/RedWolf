import React, { useState, useEffect } from 'react';
import { 
  Server, 
  FileCode, 
  HardDrive, 
  Network, 
  KeyRound, 
  ChevronLeft, 
  ChevronRight
} from 'lucide-react';

export type NavSection = 'fleet' | 'templates' | 'storage' | 'network' | 'auth';

interface SidebarProps {
  currentView: 'fleet' | 'settings';
  settingsTab: 'auth' | 'network' | 'storage' | 'templates';
  onNavigate: (view: 'fleet' | 'settings', tab?: 'auth' | 'network' | 'storage' | 'templates') => void;
  nodeCount: number;
  templateCount: number;
  isConnected?: boolean;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  settingsTab,
  onNavigate,
  nodeCount,
  templateCount,
  isCollapsed,
  onToggleCollapse,
}) => {
  const [version, setVersion] = useState<string>('1.3.3');

  useEffect(() => {
    fetch('/api/version')
      .then((res) => (res.ok ? res.json() : null))
      .then((data: { version?: string } | null) => {
        if (data?.version) {
          setVersion(data.version.replace(/^v/, ''));
        }
      })
      .catch(() => {});
  }, []);
  const isItemActive = (view: 'fleet' | 'settings', tab?: 'auth' | 'network' | 'storage' | 'templates') => {
    if (view === 'fleet') {
      return currentView === 'fleet';
    }
    return currentView === 'settings' && settingsTab === tab;
  };

  const navItemClass = (active: boolean) => `
    flex items-center gap-3 px-3 py-2 text-xs rounded-sm transition-colors cursor-pointer select-none
    ${active 
      ? 'bg-[#1a202c] text-white font-semibold border-l-2 border-redwolf-primary shadow-sm' 
      : 'text-slate-400 hover:text-slate-100 hover:bg-[#131720] border-l-2 border-transparent'
    }
    ${isCollapsed ? 'justify-center px-2' : ''}
  `;

  return (
    <aside
      className={`bg-[#0f1218] border-r border-[#1e2430] flex flex-col justify-between shrink-0 transition-all duration-200 z-30 ${
        isCollapsed ? 'w-14' : 'w-60'
      }`}
    >
      {/* Brand & Rail Header */}
      <div>
        <div className="h-13 border-b border-[#1e2430] flex items-center justify-between px-3">
          <div 
            onClick={() => onNavigate('fleet')}
            className="flex items-center gap-2.5 cursor-pointer overflow-hidden"
            title="RedWolf Provisioning"
          >
            <img 
              src="/assets/redwolf-icon.svg" 
              alt="RedWolf" 
              className="h-6 w-6 shrink-0 object-contain"
            />
            {!isCollapsed && (
              <div className="flex items-baseline gap-1.5 overflow-hidden">
                <span className="font-bold tracking-tight text-white text-sm">REDWOLF</span>
                <span className="text-[10px] font-mono text-slate-500">{version}</span>
              </div>
            )}
          </div>

          <button
            type="button"
            onClick={onToggleCollapse}
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            className="p-1 rounded text-slate-500 hover:text-slate-200 hover:bg-[#181e28] transition-colors"
          >
            {isCollapsed ? <ChevronRight className="w-3.5 h-3.5" /> : <ChevronLeft className="w-3.5 h-3.5" />}
          </button>
        </div>

        {/* Navigation Items */}
        <nav className="p-2 space-y-4">
          
          {/* Section: Inventory */}
          <div>
            {!isCollapsed && (
              <div className="px-3 pb-1 text-[10px] font-semibold text-slate-500 uppercase tracking-wider">
                Inventory
              </div>
            )}
            <div className="space-y-0.5">
              <button
                type="button"
                onClick={() => onNavigate('fleet')}
                title="Servers"
                className={`w-full ${navItemClass(isItemActive('fleet'))}`}
              >
                <Server className="w-4 h-4 shrink-0 text-slate-400" />
                {!isCollapsed && (
                  <div className="flex items-center justify-between flex-1">
                    <span>Servers</span>
                    <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-[#181e28] text-slate-300 border border-[#2a3344]">
                      {nodeCount}
                    </span>
                  </div>
                )}
              </button>
            </div>
          </div>

          {/* Section: Provisioning */}
          <div>
            {!isCollapsed && (
              <div className="px-3 pb-1 text-[10px] font-semibold text-slate-500 uppercase tracking-wider">
                Provisioning
              </div>
            )}
            <div className="space-y-0.5">
              <button
                type="button"
                onClick={() => onNavigate('settings', 'templates')}
                title="Cloud-Init Templates"
                className={`w-full ${navItemClass(isItemActive('settings', 'templates'))}`}
              >
                <FileCode className="w-4 h-4 shrink-0 text-slate-400" />
                {!isCollapsed && (
                  <div className="flex items-center justify-between flex-1">
                    <span>Templates</span>
                    {templateCount > 0 && (
                      <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-[#181e28] text-slate-400 border border-[#2a3344]">
                        {templateCount}
                      </span>
                    )}
                  </div>
                )}
              </button>

              <button
                type="button"
                onClick={() => onNavigate('settings', 'storage')}
                title="OS Images & Mirror"
                className={`w-full ${navItemClass(isItemActive('settings', 'storage'))}`}
              >
                <HardDrive className="w-4 h-4 shrink-0 text-slate-400" />
                {!isCollapsed && <span>OS Images & Storage</span>}
              </button>
            </div>
          </div>

          {/* Section: Administration */}
          <div>
            {!isCollapsed && (
              <div className="px-3 pb-1 text-[10px] font-semibold text-slate-500 uppercase tracking-wider">
                Administration
              </div>
            )}
            <div className="space-y-0.5">
              <button
                type="button"
                onClick={() => onNavigate('settings', 'network')}
                title="Network & PXE"
                className={`w-full ${navItemClass(isItemActive('settings', 'network'))}`}
              >
                <Network className="w-4 h-4 shrink-0 text-slate-400" />
                {!isCollapsed && <span>Network & DHCP</span>}
              </button>

              <button
                type="button"
                onClick={() => onNavigate('settings', 'auth')}
                title="Authentication & Directories"
                className={`w-full ${navItemClass(isItemActive('settings', 'auth'))}`}
              >
                <KeyRound className="w-4 h-4 shrink-0 text-slate-400" />
                {!isCollapsed && <span>Authentication</span>}
              </button>
            </div>
          </div>

        </nav>
      </div>
    </aside>
  );
};
