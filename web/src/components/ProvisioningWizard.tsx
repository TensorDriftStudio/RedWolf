import React, { useState } from 'react';
import type { ServerNode } from '../types';
import { VendorBadge } from './Badges';
import { 
  X, Check, ArrowRight, ArrowLeft, HardDrive, Play, Loader2
} from 'lucide-react';

interface ProvisioningWizardProps {
  node: ServerNode | null;
  onClose: () => void;
  onDeploySuccess: (nodeId: string, os: string, drive: string, ip: string) => void;
}

export const ProvisioningWizard: React.FC<ProvisioningWizardProps> = ({ node, onClose, onDeploySuccess }) => {
  if (!node) return null;

  const [step, setStep] = useState<1 | 2 | 3 | 4 | 5>(1);

  // Form State
  const [selectedOS, setSelectedOS] = useState<'AlmaLinux 8' | 'AlmaLinux 9' | 'AlmaLinux 10' | 'Debian 12' | 'Debian 13'>('AlmaLinux 9');
  const [targetDrive, setTargetDrive] = useState<string>(node.storage[0]?.path || '/dev/nvme0n1');
  const [partitioning, setPartitioning] = useState<'standard' | 'lvm'>('standard');
  const [networkMode, setNetworkMode] = useState<'static' | 'dhcp'>('static');
  const [staticIp, setStaticIp] = useState('10.10.100.25');
  const [gateway, setGateway] = useState('10.10.100.1');
  const [dns, setDns] = useState('1.1.1.1, 8.8.8.8');
  const [enableBonding, setEnableBonding] = useState(false);
  const [rootPassword, setRootPassword] = useState('RedWolf#2026!');
  const [sshKey, setSshKey] = useState('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGX... admin@redwolf');

  // Execution Simulation State
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployProgress, setDeployProgress] = useState(0);
  const [currentStage, setCurrentStage] = useState('');
  const [liveLogs, setLiveLogs] = useState<string[]>([]);

  // Distributions list
  const distros = [
    {
      id: 'AlmaLinux 9',
      name: 'AlmaLinux 9.4',
      badge: 'Enterprise EL9',
      tag: 'Recommended',
      kernel: 'Linux 5.14+',
      color: 'border-orange-500/50 bg-orange-950/20 text-orange-300'
    },
    {
      id: 'AlmaLinux 8',
      name: 'AlmaLinux 8.10',
      badge: 'Enterprise EL8',
      kernel: 'Linux 4.18+',
      color: 'border-amber-500/50 bg-amber-950/20 text-amber-300'
    },
    {
      id: 'AlmaLinux 10',
      name: 'AlmaLinux 10 (Beta)',
      badge: 'Enterprise EL10',
      kernel: 'Linux 6.x',
      color: 'border-red-500/50 bg-red-950/20 text-red-300'
    },
    {
      id: 'Debian 12',
      name: 'Debian 12 (Bookworm)',
      badge: 'Debian Stable',
      kernel: 'Linux 6.1 LTS',
      color: 'border-rose-500/50 bg-rose-950/20 text-rose-300'
    },
    {
      id: 'Debian 13',
      name: 'Debian 13 (Trixie)',
      badge: 'Debian Testing',
      kernel: 'Linux 6.6+',
      color: 'border-fuchsia-500/50 bg-fuchsia-950/20 text-fuchsia-300'
    },
  ];

  // Start deployment simulation
  const handleStartDeployment = () => {
    setIsDeploying(true);
    setDeployProgress(5);
    setCurrentStage('Streaming compressed OS image via zstd to target disk...');
    setLiveLogs([`[00:01] Connected to RedWolf Core asset repository...`]);

    const stages = [
      { p: 25, stage: `Streaming ${selectedOS} raw image to ${targetDrive} via Zstandard...`, log: `[00:04] Streamed 1.4 GB / 3.1 GB (bmaptool acceleration active)` },
      { p: 55, stage: `Writing raw partitions and rereading partition table...`, log: `[00:12] partx: partition table refreshed on ${targetDrive}` },
      { p: 75, stage: `Formatting NoCloud 'cidata' filesystem on ${targetDrive}p3...`, log: `[00:18] mkfs.vfat: formatted label 'cidata' (Cloud-Init NoCloud source)` },
      { p: 90, stage: `Injecting MAC-matched network-config & user-data...`, log: `[00:24] Wrote user-data (root password SHA-512 crypt & authorized keys)` },
      { p: 98, stage: `Registering NVRAM UEFI boot entry (efibootmgr)...`, log: `[00:29] efibootmgr: Boot0001 created as primary boot device` },
      { p: 100, stage: `Rebooting node into ${selectedOS}...`, log: `[00:32] Node signaled to reboot. Cloud-Init taking over bare-metal host.` },
    ];

    stages.forEach((item, index) => {
      setTimeout(() => {
        setDeployProgress(item.p);
        setCurrentStage(item.stage);
        setLiveLogs((prev) => [...prev, item.log]);

        if (item.p === 100) {
          setTimeout(() => {
            onDeploySuccess(node.id, selectedOS, targetDrive, networkMode === 'static' ? staticIp : 'DHCP');
          }, 1200);
        }
      }, (index + 1) * 1100);
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-md p-4 animate-fade-in">
      <div className="w-full max-w-3xl rounded-2xl border border-surface-border bg-surface-card shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
        
        {/* Modal Header */}
        <div className="border-b border-surface-border bg-surface-panel/80 p-5 flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="text-xs font-bold uppercase tracking-wider text-redwolf-primary">Zero-Touch Provisioning</span>
              <span className="text-slate-600">•</span>
              <VendorBadge vendor={node.vendor} />
            </div>
            <h2 className="text-base font-bold text-white flex items-center gap-2">
              Deploying {node.model} 
              <span className="font-mono text-xs font-normal text-slate-400">({node.serialNumber})</span>
            </h2>
          </div>
          {!isDeploying && (
            <button onClick={onClose} className="rounded-lg p-1.5 text-slate-400 hover:bg-surface-card hover:text-white transition-all">
              <X className="h-5 w-5" />
            </button>
          )}
        </div>

        {/* Stepper Progress */}
        {!isDeploying && (
          <div className="border-b border-surface-border/60 bg-surface-panel/30 px-6 py-3">
            <div className="flex items-center justify-between text-xs">
              {[
                { s: 1, label: 'Operating System' },
                { s: 2, label: 'Target Storage' },
                { s: 3, label: 'Network Setup' },
                { s: 4, label: 'Credentials' },
                { s: 5, label: 'Deploy & Stream' },
              ].map((item) => (
                <div key={item.s} className="flex items-center gap-2">
                  <div className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold ${
                    step === item.s 
                      ? 'bg-redwolf-primary text-white shadow-glow-red' 
                      : step > item.s 
                        ? 'bg-emerald-950 text-emerald-400 border border-emerald-700' 
                        : 'bg-slate-800 text-slate-500'
                  }`}>
                    {step > item.s ? <Check className="h-3.5 w-3.5" /> : item.s}
                  </div>
                  <span className={`hidden sm:inline font-medium ${step === item.s ? 'text-white' : 'text-slate-400'}`}>
                    {item.label}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Modal Body */}
        <div className="flex-1 overflow-y-auto p-6 text-xs">
          
          {/* STEP 1: OS Selection */}
          {step === 1 && !isDeploying && (
            <div className="space-y-4">
              <div className="text-sm font-semibold text-white">Select Target Operating System</div>
              <p className="text-slate-400">RedWolf streams official compressed cloud raw images directly to the target storage drive with Cloud-Init pre-configured.</p>
              
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-4">
                {distros.map((d) => (
                  <div
                    key={d.id}
                    onClick={() => setSelectedOS(d.id as any)}
                    className={`cursor-pointer rounded-xl border p-4 transition-all ${
                      selectedOS === d.id 
                        ? 'border-redwolf-primary bg-redwolf-primary/10 shadow-glow-red' 
                        : 'border-surface-border bg-surface-panel/50 hover:border-slate-600'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-bold text-white">{d.name}</span>
                      {d.tag && (
                        <span className="rounded bg-emerald-950 px-2 py-0.5 text-[10px] font-semibold text-emerald-300 border border-emerald-800">
                          {d.tag}
                        </span>
                      )}
                    </div>
                    <div className="mt-2 flex items-center gap-2 text-[11px] text-slate-400 font-mono">
                      <span>{d.badge}</span>
                      <span>•</span>
                      <span>{d.kernel}</span>
                    </div>
                    <div className="mt-2 text-[10px] text-slate-500 font-mono">
                      Image: {d.id.toLowerCase().replace(' ', '-')}.raw.zstd
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* STEP 2: Storage Target */}
          {step === 2 && !isDeploying && (
            <div className="space-y-4">
              <div className="text-sm font-semibold text-white">Select Target Storage Device</div>
              <p className="text-slate-400">Deterministic drive selection prevents overwriting secondary data disks. The boot image and NoCloud partition will be written here.</p>

              <div className="space-y-2.5 mt-3">
                {node.storage.map((disk) => (
                  <div
                    key={disk.path}
                    onClick={() => setTargetDrive(disk.path)}
                    className={`cursor-pointer rounded-xl border p-3.5 flex items-center justify-between transition-all ${
                      targetDrive === disk.path 
                        ? 'border-redwolf-primary bg-redwolf-primary/10 shadow-glow-red' 
                        : 'border-surface-border bg-surface-panel/50 hover:border-slate-600'
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <HardDrive className={`h-5 w-5 ${disk.type === 'NVMe' ? 'text-sky-400' : 'text-slate-400'}`} />
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-mono font-bold text-white text-xs">{disk.path}</span>
                          <span className="rounded bg-slate-800 px-1.5 py-0.2 text-[10px] font-semibold text-sky-400 border border-slate-700">
                            {disk.type}
                          </span>
                          <span className="text-emerald-400 font-semibold">{disk.sizeHuman}</span>
                        </div>
                        <div className="text-slate-300 font-medium text-[11px] mt-0.5">{disk.model}</div>
                        <div className="text-[10px] font-mono text-slate-400">SN: {disk.serial}</div>
                      </div>
                    </div>

                    <div className={`h-4 w-4 rounded-full border flex items-center justify-center ${
                      targetDrive === disk.path ? 'border-redwolf-primary bg-redwolf-primary' : 'border-slate-600'
                    }`}>
                      {targetDrive === disk.path && <div className="h-1.5 w-1.5 rounded-full bg-white" />}
                    </div>
                  </div>
                ))}
              </div>

              {/* Partitioning Preset */}
              <div className="pt-4 border-t border-surface-border">
                <div className="text-xs font-semibold text-slate-300 mb-2">Partitioning Layout</div>
                <div className="grid grid-cols-2 gap-3">
                  <div 
                    onClick={() => setPartitioning('standard')}
                    className={`cursor-pointer rounded-lg border p-3 ${partitioning === 'standard' ? 'border-redwolf-primary bg-redwolf-primary/10' : 'border-surface-border bg-surface-panel/40'}`}
                  >
                    <div className="font-semibold text-white">Standard EFI + Root</div>
                    <div className="text-[11px] text-slate-400 mt-1">/boot/efi (FAT32) + / (ext4) + cidata</div>
                  </div>
                  <div 
                    onClick={() => setPartitioning('lvm')}
                    className={`cursor-pointer rounded-lg border p-3 ${partitioning === 'lvm' ? 'border-redwolf-primary bg-redwolf-primary/10' : 'border-surface-border bg-surface-panel/40'}`}
                  >
                    <div className="font-semibold text-white">LVM Volume Group</div>
                    <div className="text-[11px] text-slate-400 mt-1">Dynamic thin provisioning &amp; snapshots</div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* STEP 3: Network Config */}
          {step === 3 && !isDeploying && (
            <div className="space-y-4">
              <div className="text-sm font-semibold text-white">Production Network Configuration</div>
              <p className="text-slate-400">Cloud-Init binds network settings directly by physical <strong>MAC address</strong> to prevent interface renumbering across vendors.</p>

              {/* Mode switch */}
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={() => setNetworkMode('static')}
                  className={`flex-1 rounded-lg border py-2 font-semibold ${networkMode === 'static' ? 'border-redwolf-primary bg-redwolf-primary/10 text-white' : 'border-surface-border text-slate-400'}`}
                >
                  Static Production IP
                </button>
                <button
                  type="button"
                  onClick={() => setNetworkMode('dhcp')}
                  className={`flex-1 rounded-lg border py-2 font-semibold ${networkMode === 'dhcp' ? 'border-redwolf-primary bg-redwolf-primary/10 text-white' : 'border-surface-border text-slate-400'}`}
                >
                  Production DHCP
                </button>
              </div>

              {networkMode === 'static' && (
                <div className="grid grid-cols-2 gap-3 pt-2">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-300 mb-1">Static IPv4 Address / CIDR</label>
                    <input
                      type="text"
                      value={staticIp}
                      onChange={(e) => setStaticIp(e.target.value)}
                      className="w-full rounded-lg border border-surface-border bg-surface-panel px-3 py-2 text-white font-mono focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-300 mb-1">Default Gateway</label>
                    <input
                      type="text"
                      value={gateway}
                      onChange={(e) => setGateway(e.target.value)}
                      className="w-full rounded-lg border border-surface-border bg-surface-panel px-3 py-2 text-white font-mono focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-[11px] font-semibold text-slate-300 mb-1">DNS Nameservers (comma-separated)</label>
                    <input
                      type="text"
                      value={dns}
                      onChange={(e) => setDns(e.target.value)}
                      className="w-full rounded-lg border border-surface-border bg-surface-panel px-3 py-2 text-white font-mono focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                </div>
              )}

              {/* LACP Bonding Toggle */}
              <div className="pt-3 border-t border-surface-border flex items-center justify-between">
                <div>
                  <div className="font-semibold text-white">802.3ad LACP Bonding (bond0)</div>
                  <div className="text-[11px] text-slate-400">Aggregates primary and secondary 10GbE/25GbE interfaces</div>
                </div>
                <input
                  type="checkbox"
                  checked={enableBonding}
                  onChange={(e) => setEnableBonding(e.target.checked)}
                  className="h-4 w-4 rounded accent-redwolf-primary"
                />
              </div>
            </div>
          )}

          {/* STEP 4: Credentials */}
          {step === 4 && !isDeploying && (
            <div className="space-y-4">
              <div className="text-sm font-semibold text-white">Root Password &amp; SSH Authentication</div>
              <p className="text-slate-400">Passwords are converted to SHA-512 crypt hashes before injection into the Cloud-Init <code>user-data</code> volume.</p>

              <div>
                <label className="block text-[11px] font-semibold text-slate-300 mb-1">Root User Password</label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={rootPassword}
                    onChange={(e) => setRootPassword(e.target.value)}
                    className="flex-1 rounded-lg border border-surface-border bg-surface-panel px-3 py-2 text-white font-mono focus:border-redwolf-primary focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => setRootPassword(`RW#${Math.random().toString(36).slice(-8)}!9`)}
                    className="rounded-lg border border-surface-border bg-surface-panel px-3 text-xs font-semibold text-slate-200 hover:text-white"
                  >
                    Regenerate
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-semibold text-slate-300 mb-1">Authorized SSH Public Keys</label>
                <textarea
                  rows={3}
                  value={sshKey}
                  onChange={(e) => setSshKey(e.target.value)}
                  className="w-full rounded-lg border border-surface-border bg-surface-panel p-3 text-white font-mono text-[11px] focus:border-redwolf-primary focus:outline-none"
                />
              </div>
            </div>
          )}

          {/* STEP 5: Review & Deploy */}
          {step === 5 && !isDeploying && (
            <div className="space-y-4">
              <div className="text-sm font-semibold text-white">Review Provisioning Parameters</div>
              
              <div className="rounded-xl border border-surface-border bg-surface-panel/60 p-4 space-y-2.5 font-mono">
                <div className="flex justify-between border-b border-surface-border/50 pb-2">
                  <span className="text-slate-400">Target Node:</span>
                  <strong className="text-white">{node.model} ({node.serialNumber})</strong>
                </div>
                <div className="flex justify-between border-b border-surface-border/50 pb-2">
                  <span className="text-slate-400">Operating System:</span>
                  <span className="text-emerald-400 font-bold">{selectedOS}</span>
                </div>
                <div className="flex justify-between border-b border-surface-border/50 pb-2">
                  <span className="text-slate-400">Boot Storage Drive:</span>
                  <span className="text-sky-400">{targetDrive} ({partitioning.toUpperCase()})</span>
                </div>
                <div className="flex justify-between border-b border-surface-border/50 pb-2">
                  <span className="text-slate-400">Production IP:</span>
                  <span className="text-white">{networkMode === 'static' ? staticIp : 'DHCP'}</span>
                </div>
                <div className="flex justify-between border-b border-surface-border/50 pb-2">
                  <span className="text-slate-400">LACP 802.3ad Bonding:</span>
                  <span className="text-white">{enableBonding ? 'Enabled (bond0)' : 'Single Port'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">Cloud-Init Datasource:</span>
                  <span className="text-amber-400">Local NoCloud (cidata partition)</span>
                </div>
              </div>
            </div>
          )}

          {/* Live Deployment Progress Simulation */}
          {isDeploying && (
            <div className="space-y-5 py-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Loader2 className="h-5 w-5 animate-spin text-redwolf-primary" />
                  <span className="text-sm font-bold text-white">Streaming &amp; Provisioning Bare-Metal Node</span>
                </div>
                <span className="text-base font-mono font-bold text-redwolf-primary">{deployProgress}%</span>
              </div>

              {/* Progress bar */}
              <div className="h-2.5 w-full rounded-full bg-slate-800 overflow-hidden">
                <div 
                  className="h-full bg-gradient-to-r from-redwolf-primary to-rose-500 transition-all duration-500"
                  style={{ width: `${deployProgress}%` }}
                />
              </div>

              <div className="text-xs font-semibold text-slate-300 flex items-center gap-2">
                <span className="h-2 w-2 rounded-full bg-emerald-400 animate-ping" />
                {currentStage}
              </div>

              {/* Streaming Logs */}
              <div className="rounded-xl border border-surface-border bg-black/80 p-4 font-mono text-[11px] text-slate-300 space-y-1 max-h-48 overflow-y-auto">
                {liveLogs.map((log, i) => (
                  <div key={i} className="flex gap-2">
                    <span className="text-slate-600">&gt;</span>
                    <span>{log}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

        </div>

        {/* Modal Footer Controls */}
        {!isDeploying && (
          <div className="border-t border-surface-border bg-surface-panel/80 px-6 py-4 flex items-center justify-between">
            {step > 1 ? (
              <button
                type="button"
                onClick={() => setStep((s) => (s - 1) as any)}
                className="flex items-center gap-1.5 rounded-lg border border-surface-border px-3.5 py-1.5 text-xs font-medium text-slate-300 hover:text-white"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Back
              </button>
            ) : <div />}

            {step < 5 ? (
              <button
                type="button"
                onClick={() => setStep((s) => (s + 1) as any)}
                className="flex items-center gap-1.5 rounded-lg bg-surface-card border border-surface-border px-4 py-1.5 text-xs font-semibold text-white hover:bg-surface-panel transition-all"
              >
                Continue
                <ArrowRight className="h-3.5 w-3.5" />
              </button>
            ) : (
              <button
                type="button"
                onClick={handleStartDeployment}
                className="flex items-center gap-2 rounded-lg bg-redwolf-primary px-5 py-2 text-xs font-bold text-white shadow-glow-red hover:bg-redwolf-hover transition-all"
              >
                <Play className="h-4 w-4 fill-current" />
                Start Bare-Metal Cloud-Init Streaming
              </button>
            )}
          </div>
        )}

      </div>
    </div>
  );
};
