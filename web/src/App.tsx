import { useState, useMemo, useEffect } from 'react';
import { Sidebar } from './components/Sidebar';
import { Navbar } from './components/Navbar';
import { StatsBar } from './components/StatsBar';
import { NodeTable } from './components/NodeTable';
import { NodeDrawer } from './components/NodeDrawer';
import { ProvisioningWizard } from './components/ProvisioningWizard';
import { SettingsView } from './components/SettingsView';
import { LoginView } from './components/LoginView';
import { useNodes } from './hooks/useNodes';
import { useAuth } from './hooks/useAuth';
import type { Vendor, NodeStatus, CloudInitTemplate } from './types';
import { getAuthHeaders } from './utils/auth';
import { Filter, CheckCircle2, AlertCircle } from 'lucide-react';

export function App() {
  const { user, isAuthenticated, isLoading: isAuthLoading, error: authError, login, logout } = useAuth();
  const { nodes, isLoading, error, isConnected, refresh, deployNode, resetNode, deleteNode } = useNodes();

  const [currentView, setCurrentView] = useState<'fleet' | 'settings'>('fleet');
  const [settingsTab, setSettingsTab] = useState<'auth' | 'network' | 'storage' | 'templates'>('auth');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedVendor, setSelectedVendor] = useState<'ALL' | Vendor>('ALL');
  const [selectedStatus, setSelectedStatus] = useState<'ALL' | NodeStatus>('ALL');
  const [subnetCidr, setSubnetCidr] = useState<string>('192.168.0.0/24');
  const [templateCount, setTemplateCount] = useState<number>(0);
  
  // Collapsible AWX-style sidebar state
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem('redwolf_sidebar_collapsed') === 'true';
    } catch {
      return false;
    }
  });

  const toggleSidebar = () => {
    setIsSidebarCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem('redwolf_sidebar_collapsed', String(next));
      } catch {
        // ignore
      }
      return next;
    });
  };

  // Drawer & Wizard state tracked by ID to keep real-time sync with WebSocket updates
  const [inspectedNodeId, setInspectedNodeId] = useState<string | null>(null);
  const [deployingNodeId, setDeployingNodeId] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  // Dynamically query subnet configuration from Core settings
  useEffect(() => {
    fetch('/api/settings', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data?.network?.subnetCidr) {
          setSubnetCidr(data.network.subnetCidr);
        }
      })
      .catch(() => {});
  }, [isAuthenticated]);

  // Fetch template count for sidebar badge
  useEffect(() => {
    fetch('/api/templates', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: CloudInitTemplate[]) => {
        if (Array.isArray(data)) {
          setTemplateCount(data.length);
        }
      })
      .catch(() => {});
  }, [isAuthenticated]);

  // Synchronously resolve live node instances from current nodes state
  const inspectedNode = useMemo(() => {
    if (!inspectedNodeId) return null;
    return nodes.find((n) => n.id === inspectedNodeId) || null;
  }, [inspectedNodeId, nodes]);

  const deployingNode = useMemo(() => {
    if (!deployingNodeId) return null;
    return nodes.find((n) => n.id === deployingNodeId) || null;
  }, [deployingNodeId, nodes]);

  // Filtered nodes
  const filteredNodes = useMemo(() => {
    return nodes.filter((node) => {
      const matchesSearch = 
        node.model.toLowerCase().includes(searchTerm.toLowerCase()) ||
        node.serialNumber.toLowerCase().includes(searchTerm.toLowerCase()) ||
        node.bmc.ip.toLowerCase().includes(searchTerm.toLowerCase()) ||
        node.nics.some(nic => nic.mac.toLowerCase().includes(searchTerm.toLowerCase()));

      const matchesVendor = selectedVendor === 'ALL' || node.vendor === selectedVendor;
      const matchesStatus = selectedStatus === 'ALL' || node.status === selectedStatus;

      return matchesSearch && matchesVendor && matchesStatus;
    });
  }, [nodes, searchTerm, selectedVendor, selectedStatus]);

  // Handle manual scan network
  const handleRefresh = async () => {
    await refresh();
    setNotification('Network scan initiated.');
    setTimeout(() => setNotification(null), 3000);
  };

  const handleNavigate = (view: 'fleet' | 'settings', tab?: 'auth' | 'network' | 'storage' | 'templates') => {
    setCurrentView(view);
    if (tab) {
      setSettingsTab(tab);
    }
  };

  // If user is not authenticated, render Login Screen
  if (!isAuthenticated) {
    return (
      <LoginView
        onLogin={login}
        isLoading={isAuthLoading}
        error={authError}
      />
    );
  }

  return (
    <div className="min-h-screen bg-[#0b0e14] text-slate-100 flex flex-col font-sans">
      
      {/* Top Header Navigation */}
      <Navbar 
        onRefresh={handleRefresh}
        isRefreshing={isLoading}
        searchTerm={searchTerm}
        setSearchTerm={setSearchTerm}
        currentView={currentView}
        settingsTab={settingsTab}
        user={user}
        onLogout={logout}
        onOpenSettings={() => handleNavigate('settings', 'auth')}
      />

      {/* AWX Two-Column Shell */}
      <div className="flex-1 flex overflow-hidden">
        
        {/* Left Column: AWX-style collapsible Sidebar */}
        <Sidebar
          currentView={currentView}
          settingsTab={settingsTab}
          onNavigate={handleNavigate}
          nodeCount={nodes.length}
          templateCount={templateCount}
          isConnected={isConnected}
          isCollapsed={isSidebarCollapsed}
          onToggleCollapse={toggleSidebar}
        />

        {/* Right Column: Main Content Canvas */}
        <main className="flex-1 overflow-y-auto min-w-0 bg-[#0b0e14] p-4 sm:p-6 space-y-4">
          
          {/* Error Banner */}
          {error && (
            <div className="flex items-center justify-between rounded-sm border border-rose-800/80 bg-rose-950/50 px-3 py-2 text-xs font-medium text-rose-300">
              <div className="flex items-center gap-2">
                <AlertCircle className="h-4 w-4 text-rose-400 shrink-0" />
                <span>{error}</span>
              </div>
            </div>
          )}

          {/* Success Banner */}
          {notification && (
            <div className="flex items-center justify-between rounded-sm border border-emerald-800/80 bg-emerald-950/50 px-3 py-2 text-xs font-medium text-emerald-300">
              <div className="flex items-center gap-2">
                <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
                <span>{notification}</span>
              </div>
              <button onClick={() => setNotification(null)} className="text-emerald-400 hover:text-white">
                ✕
              </button>
            </div>
          )}

          {/* View Switcher: Settings vs Fleet */}
          {currentView === 'settings' ? (
            <SettingsView 
              activeTab={settingsTab} 
              onTabChange={setSettingsTab}
              onBackToFleet={() => setCurrentView('fleet')} 
            />
          ) : (
            <>
              {/* Clean Section Title & Subnet */}
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between pb-3 border-b border-[#212836]">
                <div>
                  <h1 className="text-base font-bold text-white tracking-tight">
                    Servers
                  </h1>
                </div>

                <div className="flex items-center gap-2 text-xs font-mono">
                  <span className="rounded-sm border border-[#212836] bg-[#151b24] px-2.5 py-1 text-slate-300">
                    Subnet: <strong className="text-white">{subnetCidr}</strong>
                  </span>
                </div>
              </div>

              {/* Fleet Metric Cards */}
              <StatsBar nodes={nodes} isLoading={isLoading} />

              {/* Filters Toolbar */}
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-[#212836] bg-[#151b24] p-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="flex items-center gap-1 text-[11px] font-semibold text-slate-400 pr-2 border-r border-[#212836]">
                    <Filter className="h-3 w-3" />
                    Vendor:
                  </span>

                  {/* Vendor Filter Buttons */}
                  {(['ALL', 'Dell Inc.', 'Supermicro', 'ASRockRack'] as const).map((v) => (
                    <button
                      key={v}
                      onClick={() => setSelectedVendor(v)}
                      className={`rounded-sm px-2 py-0.5 text-xs font-medium transition-colors ${
                        selectedVendor === v
                          ? 'bg-redwolf-primary text-white'
                          : 'bg-[#10141d] text-slate-300 hover:bg-[#1a212d]'
                      }`}
                    >
                      {v === 'ALL' ? 'All Vendors' : v}
                    </button>
                  ))}
                </div>

                {/* Status Filter */}
                <div className="flex items-center gap-1.5">
                  <span className="text-[11px] font-semibold text-slate-400 pr-2 border-r border-[#212836]">
                    Status:
                  </span>
                  {(['ALL', 'READY_FOR_PROVISIONING', 'ACTIVE'] as const).map((st) => (
                    <button
                      key={st}
                      onClick={() => setSelectedStatus(st as NodeStatus)}
                      className={`rounded-sm px-2 py-0.5 text-xs font-medium transition-colors ${
                        selectedStatus === st
                          ? 'border border-[#1f6feb] bg-[#122033] text-[#58a6ff]'
                          : 'text-slate-400 hover:text-white'
                      }`}
                    >
                      {st === 'ALL' ? 'All' : st === 'READY_FOR_PROVISIONING' ? 'Ready to Deploy' : 'Active'}
                    </button>
                  ))}
                </div>
              </div>

              {/* Server Nodes Table */}
              <NodeTable 
                nodes={filteredNodes}
                isLoading={isLoading}
                onSelectNode={(node) => setInspectedNodeId(node.id)}
                onDeployNode={(node) => setDeployingNodeId(node.id)}
              />

              {/* Detail Inspection Drawer */}
              <NodeDrawer
                node={inspectedNode}
                onClose={() => setInspectedNodeId(null)}
                onDeploy={(node) => setDeployingNodeId(node.id)}
                onReset={async (id) => {
                  await resetNode(id);
                  setNotification('Node state reset.');
                  setTimeout(() => setNotification(null), 3000);
                }}
                onDelete={async (id) => {
                  await deleteNode(id);
                  setInspectedNodeId(null);
                  setNotification('Node removed from inventory.');
                  setTimeout(() => setNotification(null), 3000);
                }}
              />

              {/* Provisioning Wizard Modal */}
              <ProvisioningWizard
                node={deployingNode}
                onClose={() => setDeployingNodeId(null)}
                onDeploy={deployNode}
              />
            </>
          )}

        </main>
      </div>

    </div>
  );
}

export default App;
