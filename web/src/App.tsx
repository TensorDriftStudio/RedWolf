import { useState, useMemo } from 'react';
import { Navbar } from './components/Navbar';
import { StatsBar } from './components/StatsBar';
import { NodeTable } from './components/NodeTable';
import { NodeDrawer } from './components/NodeDrawer';
import { ProvisioningWizard } from './components/ProvisioningWizard';
import { initialNodes } from './data/mockNodes';
import type { ServerNode, Vendor, NodeStatus } from './types';
import { Filter, CheckCircle2 } from 'lucide-react';

export function App() {
  const [nodes, setNodes] = useState<ServerNode[]>(initialNodes);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedVendor, setSelectedVendor] = useState<'ALL' | Vendor>('ALL');
  const [selectedStatus, setSelectedStatus] = useState<'ALL' | NodeStatus>('ALL');
  const [isRefreshing, setIsRefreshing] = useState(false);
  
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

  // Network scan trigger
  const handleRefresh = () => {
    setIsRefreshing(true);
    setTimeout(() => {
      setIsRefreshing(false);
      setNotification('Network scan complete: 4 nodes active on 192.168.50.0/24');
      setTimeout(() => setNotification(null), 4000);
    }, 900);
  };

  // Handle successful deployment
  const handleDeploySuccess = (nodeId: string, os: string, drive: string, ip: string) => {
    setNodes((prev) => 
      prev.map((n) => {
        if (n.id === nodeId) {
          return {
            ...n,
            status: 'ACTIVE' as NodeStatus,
            provisioningState: {
              progress: 100,
              stage: 'Active in Production',
              os: `${os} (Cloud-Init)`,
              targetDrive: drive,
              targetIp: ip,
              logs: [
                `Zstandard raw image streaming complete to ${drive}`,
                'Formatted partition as NoCloud cidata',
                'Applied Cloud-Init network-config (MAC-matched)',
                'Registered UEFI boot entry via efibootmgr',
                'Node rebooted into production successfully.'
              ]
            }
          };
        }
        return n;
      })
    );

    setDeployingNode(null);
    setNotification(`Successfully deployed ${os} on ${nodeId}! Server is now active.`);
    setTimeout(() => setNotification(null), 6000);
  };

  return (
    <div className="min-h-screen bg-[#07090e] text-slate-100 flex flex-col font-sans">
      
      {/* Top Navigation */}
      <Navbar 
        onRefresh={handleRefresh}
        isRefreshing={isRefreshing}
        searchTerm={searchTerm}
        setSearchTerm={setSearchTerm}
      />

      {/* Main Container */}
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6 lg:px-8 space-y-6">
        
        {/* Notification Banner */}
        {notification && (
          <div className="flex items-center justify-between rounded-xl border border-emerald-500/40 bg-emerald-950/40 px-4 py-3 text-xs font-semibold text-emerald-300 shadow-glow-green">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="h-4 w-4 text-emerald-400" />
              <span>{notification}</span>
            </div>
            <button onClick={() => setNotification(null)} className="text-emerald-400 hover:text-white">
              ✕
            </button>
          </div>
        )}

        {/* Dashboard Title & Overview */}
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-extrabold tracking-tight text-white sm:text-3xl">
              Server Provisioning Fleet
            </h1>
            <p className="mt-1 text-xs text-slate-400">
              Zero-touch hardware discovery, in-band BMC credential configuration, and bare-metal Cloud-Init streaming.
            </p>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <span className="rounded-lg border border-surface-border bg-surface-card px-3 py-1.5 font-mono text-slate-300">
              Provisioning VLAN: <strong className="text-white">192.168.50.0/24</strong>
            </span>
          </div>
        </div>

        {/* Fleet KPI Metric Cards */}
        <StatsBar nodes={nodes} />

        {/* Filters Bar */}
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-surface-border bg-surface-card p-3 shadow-card">
          <div className="flex flex-wrap items-center gap-2">
            <span className="flex items-center gap-1.5 text-xs font-semibold text-slate-400 pr-2 border-r border-surface-border">
              <Filter className="h-3.5 w-3.5" />
              Filter:
            </span>

            {/* Vendor Filter Buttons */}
            {(['ALL', 'Dell Inc.', 'Supermicro', 'ASRockRack'] as const).map((v) => (
              <button
                key={v}
                onClick={() => setSelectedVendor(v)}
                className={`rounded-lg px-2.5 py-1 text-xs font-medium transition-all ${
                  selectedVendor === v
                    ? 'bg-redwolf-primary text-white shadow-glow-red'
                    : 'bg-surface-panel text-slate-300 hover:bg-slate-800'
                }`}
              >
                {v === 'ALL' ? 'All Vendors' : v}
              </button>
            ))}
          </div>

          {/* Status Filter */}
          <div className="flex items-center gap-2">
            {(['ALL', 'READY_FOR_PROVISIONING', 'ACTIVE'] as const).map((st) => (
              <button
                key={st}
                onClick={() => setSelectedStatus(st as any)}
                className={`rounded-lg px-2.5 py-1 text-xs font-medium transition-all ${
                  selectedStatus === st
                    ? 'border border-slate-500 bg-slate-800 text-white'
                    : 'text-slate-400 hover:text-white'
                }`}
              >
                {st === 'ALL' ? 'All Status' : st === 'READY_FOR_PROVISIONING' ? 'Ready to Deploy' : 'Active'}
              </button>
            ))}
          </div>
        </div>

        {/* Node Matrix Table */}
        <NodeTable 
          nodes={filteredNodes}
          onSelectNode={(node) => setInspectedNode(node)}
          onDeployNode={(node) => setDeployingNode(node)}
        />

        {/* Drawer & Modal */}
        <NodeDrawer
          node={inspectedNode}
          onClose={() => setInspectedNode(null)}
          onDeploy={(node) => setDeployingNode(node)}
        />

        <ProvisioningWizard
          node={deployingNode}
          onClose={() => setDeployingNode(null)}
          onDeploySuccess={handleDeploySuccess}
        />

      </main>

      {/* Footer */}
      <footer className="border-t border-surface-border bg-surface-ground py-4 text-center text-xs text-slate-400">
        <div className="mx-auto max-w-7xl flex flex-col sm:flex-row items-center justify-between px-4">
          <div className="flex items-center gap-2">
            <span className="font-bold text-white">RedWolf</span>
            <span>— Open Source Enterprise Bare-Metal Provisioning</span>
          </div>
          <div className="mt-2 sm:mt-0 font-mono text-slate-400">
            Dell PowerEdge • Supermicro • ASRock Rack • Generic x86_64
          </div>
        </div>
      </footer>

    </div>
  );
}
export default App;
