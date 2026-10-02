import { useState, useMemo } from 'react';
import { Navbar } from './components/Navbar';
import { StatsBar } from './components/StatsBar';
import { NodeTable } from './components/NodeTable';
import { NodeDrawer } from './components/NodeDrawer';
import { ProvisioningWizard } from './components/ProvisioningWizard';
import { SettingsView } from './components/SettingsView';
import { LoginView } from './components/LoginView';
import { useNodes } from './hooks/useNodes';
import { useAuth } from './hooks/useAuth';
import type { ServerNode, Vendor, NodeStatus } from './types';
import { Filter, CheckCircle2, AlertCircle } from 'lucide-react';

export function App() {
  const { user, isAuthenticated, isLoading: isAuthLoading, error: authError, login, logout } = useAuth();
  const { nodes, isLoading, error, isConnected, refresh, deployNode, resetNode, deleteNode } = useNodes();

  const [currentView, setCurrentView] = useState<'fleet' | 'settings'>('fleet');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedVendor, setSelectedVendor] = useState<'ALL' | Vendor>('ALL');
  const [selectedStatus, setSelectedStatus] = useState<'ALL' | NodeStatus>('ALL');
  
  // Drawer & Wizard state
  const [inspectedNode, setInspectedNode] = useState<ServerNode | null>(null);
  const [deployingNode, setDeployingNode] = useState<ServerNode | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

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
    setNotification('Network scan dispatched to RedWolf Core daemon.');
    setTimeout(() => setNotification(null), 4000);
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
    <div className="min-h-screen bg-enterprise-base text-slate-100 flex flex-col font-sans">
      
      {/* Top Header Navigation */}
      <Navbar 
        onRefresh={handleRefresh}
        isRefreshing={isLoading}
        searchTerm={searchTerm}
        setSearchTerm={setSearchTerm}
        isConnected={isConnected}
        currentView={currentView}
        setCurrentView={setCurrentView}
        user={user}
        onLogout={logout}
      />

      {/* Main Enterprise Container */}
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-5 sm:px-6 space-y-4">
        
        {/* Error Banner */}
        {error && (
          <div className="flex items-center justify-between rounded-sm border border-status-errorBorder bg-status-errorBg px-3 py-2 text-xs font-medium text-status-errorText">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 text-[#f85149]" />
              <span>{error}</span>
            </div>
          </div>
        )}

        {/* Success Banner */}
        {notification && (
          <div className="flex items-center justify-between rounded-sm border border-status-activeBorder bg-status-activeBg px-3 py-2 text-xs font-medium text-status-activeText">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="h-4 w-4 text-[#3fb950]" />
              <span>{notification}</span>
            </div>
            <button onClick={() => setNotification(null)} className="text-[#3fb950] hover:text-white">
              ✕
            </button>
          </div>
        )}

        {/* View Switcher: Settings vs Fleet */}
        {currentView === 'settings' ? (
          <SettingsView onBackToFleet={() => setCurrentView('fleet')} />
        ) : (
          <>
            {/* Dashboard Title & Overview */}
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border-b border-enterprise-border pb-3">
              <div>
                <h1 className="text-lg font-bold text-white tracking-tight flex items-center gap-2">
                  Server Provisioning Fleet
                </h1>
                <p className="text-xs text-enterprise-textMuted">
                  Autonomous hardware discovery, in-band BMC credential configuration, and sparse raw Cloud-Init streaming.
                </p>
              </div>

              <div className="flex items-center gap-2 text-xs font-mono">
                <span className="rounded-sm border border-enterprise-border bg-enterprise-panel px-2.5 py-1 text-slate-300">
                  VLAN: <strong className="text-white">192.168.50.0/24</strong>
                </span>
              </div>
            </div>

            {/* Fleet Metric Cards */}
            <StatsBar nodes={nodes} />

            {/* Filters Toolbar (AWX / PatternFly style) */}
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-enterprise-border bg-enterprise-panel p-2">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="flex items-center gap-1 text-[11px] font-semibold text-enterprise-textMuted pr-2 border-r border-enterprise-border">
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
                        : 'bg-[#13171f] text-slate-300 hover:bg-enterprise-hover'
                    }`}
                  >
                    {v === 'ALL' ? 'All Vendors' : v}
                  </button>
                ))}
              </div>

              {/* Status Filter */}
              <div className="flex items-center gap-1.5">
                <span className="text-[11px] font-semibold text-enterprise-textMuted pr-2 border-r border-enterprise-border">
                  Status:
                </span>
                {(['ALL', 'READY_FOR_PROVISIONING', 'ACTIVE'] as const).map((st) => (
                  <button
                    key={st}
                    onClick={() => setSelectedStatus(st as NodeStatus)}
                    className={`rounded-sm px-2 py-0.5 text-xs font-medium transition-colors ${
                      selectedStatus === st
                        ? 'border border-[#1f6feb] bg-[#122033] text-[#58a6ff]'
                        : 'text-enterprise-textMuted hover:text-white'
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
              onSelectNode={(node) => setInspectedNode(node)}
              onDeployNode={(node) => setDeployingNode(node)}
            />

            {/* Detail Inspection Drawer */}
            <NodeDrawer
              node={inspectedNode}
              onClose={() => setInspectedNode(null)}
              onDeploy={(node) => setDeployingNode(node)}
              onReset={async (id) => {
                await resetNode(id);
                setNotification('Node state reset to Ready for Provisioning.');
                setTimeout(() => setNotification(null), 4000);
              }}
              onDelete={async (id) => {
                await deleteNode(id);
                setInspectedNode(null);
                setNotification('Node decommissioned and removed from inventory.');
                setTimeout(() => setNotification(null), 4000);
              }}
            />

            {/* Provisioning Wizard Modal */}
            <ProvisioningWizard
              node={deployingNode}
              onClose={() => setDeployingNode(null)}
              onDeploy={deployNode}
            />
          </>
        )}

      </main>

      {/* Enterprise Footer */}
      <footer className="border-t border-enterprise-border bg-enterprise-header py-3 text-xs text-enterprise-textDim">
        <div className="mx-auto max-w-7xl flex flex-col sm:flex-row items-center justify-between px-4 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="font-bold text-white">RedWolf GUI</span>
            <span className="font-mono text-[11px] text-red-400 font-semibold px-1.5 py-0.5 rounded bg-red-950/60 border border-red-800/80">v1.1.0 Enterprise</span>
            <span className="hidden sm:inline">— Multi-Vendor Zero-Touch Bare-Metal Automation</span>
          </div>
          <div className="mt-1 sm:mt-0 font-mono text-[11px] text-enterprise-textDim">
            Dell PowerEdge • Supermicro • ASRock Rack • Generic x86_64
          </div>
        </div>
      </footer>

    </div>
  );
}
export default App;
