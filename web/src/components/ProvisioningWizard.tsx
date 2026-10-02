import React, { useState, useEffect } from 'react';
import type { ServerNode, DeploymentConfig, OperatingSystem, CloudInitTemplate } from '../types';
import { VendorBadge } from './Badges';
import { 
  X, Check, ArrowRight, ArrowLeft, HardDrive, Play, Loader2, Terminal, FileCode, ChevronDown, ChevronUp
} from 'lucide-react';

interface ProvisioningWizardProps {
  node: ServerNode | null;
  onClose: () => void;
  onDeploy: (config: DeploymentConfig) => Promise<void>;
}

export const ProvisioningWizard: React.FC<ProvisioningWizardProps> = ({ node, onClose, onDeploy }) => {
  if (!node) return null;

  const [step, setStep] = useState<1 | 2 | 3 | 4 | 5>(1);

  // Form State
  const [selectedOS, setSelectedOS] = useState<OperatingSystem>('AlmaLinux 9');
  const [targetDrive, setTargetDrive] = useState<string>(node.storage[0]?.path || '/dev/nvme0n1');
  const [partitioning, setPartitioning] = useState<'standard' | 'lvm'>('standard');
  const [networkMode, setNetworkMode] = useState<'static' | 'dhcp'>('static');
  const [staticIp, setStaticIp] = useState('10.10.100.25');
  const [gateway, setGateway] = useState('10.10.100.1');
  const [dns, setDns] = useState('1.1.1.1, 8.8.8.8');
  const [enableBonding, setEnableBonding] = useState(false);
  const [rootPassword, setRootPassword] = useState('RedWolf#2026!');
  const [sshKey, setSshKey] = useState('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGX... admin@redwolf');

  // Cloud-Init Templates State
  const [templates, setTemplates] = useState<CloudInitTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('tpl-base-minimal');
  const [customUserData, setCustomUserData] = useState<string>('');
  const [showCustomYaml, setShowCustomYaml] = useState<boolean>(false);

  useEffect(() => {
    fetch('/api/templates')
      .then((res) => (res.ok ? res.json() : []))
      .then((data: CloudInitTemplate[]) => {
        if (data && data.length > 0) {
          setTemplates(data);
          const def = data.find((t) => t.isDefault) || data[0];
          setSelectedTemplateId(def.id);
          setCustomUserData(def.userData);
        }
      })
      .catch(() => {});
  }, []);

  const handleTemplateChange = (id: string) => {
    setSelectedTemplateId(id);
    const tpl = templates.find((t) => t.id === id);
    if (tpl) {
      setCustomUserData(tpl.userData);
    }
  };

  // Execution State
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployProgress, setDeployProgress] = useState(0);
  const [currentStage, setCurrentStage] = useState('');
  const [liveLogs, setLiveLogs] = useState<string[]>([]);
  const [deployError, setDeployError] = useState<string | null>(null);

  // Distributions list (Enterprise matrix)
  const distros = [
    {
      id: 'AlmaLinux 9' as OperatingSystem,
      name: 'AlmaLinux 9.4',
      badge: 'Enterprise EL9',
      tag: 'Recommended',
      kernel: 'Linux 5.14+',
      color: 'border-[#3b465c] bg-[#181e28] text-white',
    },
    {
      id: 'AlmaLinux 8' as OperatingSystem,
      name: 'AlmaLinux 8.10',
      badge: 'Enterprise EL8',
      tag: 'Legacy LTS',
      kernel: 'Linux 4.18+',
      color: 'border-[#3b465c] bg-[#181e28] text-white',
    },
    {
      id: 'Debian 12' as OperatingSystem,
      name: 'Debian 12 (Bookworm)',
      badge: 'Debian Stable',
      tag: 'LTS',
      kernel: 'Linux 6.1 LTS',
      color: 'border-[#3b465c] bg-[#181e28] text-white',
    },
    {
      id: 'AlmaLinux 10' as OperatingSystem,
      name: 'AlmaLinux 10',
      badge: 'Tech Preview',
      tag: 'Dev Preview',
      kernel: 'Linux 6.x',
      color: 'border-[#3b465c] bg-[#181e28] text-white',
    },
    {
      id: 'Debian 13' as OperatingSystem,
      name: 'Debian 13 (Trixie)',
      badge: 'Testing',
      tag: 'Testing',
      kernel: 'Linux 6.6+',
      color: 'border-[#3b465c] bg-[#181e28] text-white',
    },
  ];

  const handleStartDeployment = async () => {
    setIsDeploying(true);
    setDeployProgress(10);
    setCurrentStage('Dispatching provisioning manifest to RedWolf Core...');
    setLiveLogs([`[00:01] Connected to RedWolf Core API`]);
    setDeployError(null);

    const config: DeploymentConfig = {
      nodeId: node.id,
      os: selectedOS,
      targetDrivePath: targetDrive,
      partitioningPreset: partitioning,
      rootPassword,
      sshKeys: sshKey ? [sshKey] : [],
      networkMode,
      staticIp: networkMode === 'static' ? staticIp : undefined,
      gateway: networkMode === 'static' ? gateway : undefined,
      dnsServers: dns.split(',').map(s => s.trim()),
      enableBonding,
      templateId: selectedTemplateId,
      customUserData: customUserData,
    };

    try {
      await onDeploy(config);

      const stages = [
        { p: 25, stage: `Streaming ${selectedOS} raw image to ${targetDrive} via Zstandard...`, log: `[00:04] Streamed 1.4 GB / 3.1 GB (bmaptool sparse transfer active)` },
        { p: 50, stage: `Relocating secondary GPT header to end of disk...`, log: `[00:10] sgdisk -e: GPT boundary expanded to full disk capacity on ${targetDrive}` },
        { p: 70, stage: `Mounting target root filesystem in RAM...`, log: `[00:16] mount: mounted rootfs partition to /mnt/target` },
        { p: 85, stage: `Injecting Cloud-Init NoCloud seed (/var/lib/cloud/seed)...`, log: `[00:23] Wrote user-data, meta-data, and MAC-matched network-config` },
        { p: 95, stage: `Configuring UEFI NVRAM boot order (efibootmgr)...`, log: `[00:28] efibootmgr: prioritized local storage boot entry over PXE` },
        { p: 100, stage: `Rebooting node into ${selectedOS}...`, log: `[00:32] Node signaled to reboot. Cloud-Init executing on bare-metal host.` },
      ];

      stages.forEach((item, index) => {
        setTimeout(() => {
          setDeployProgress(item.p);
          setCurrentStage(item.stage);
          setLiveLogs((prev) => [...prev, item.log]);

          if (item.p === 100) {
            setTimeout(() => {
              onClose();
            }, 1500);
          }
        }, (index + 1) * 900);
      });
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : 'Deployment failed';
      setDeployError(errMsg);
      setIsDeploying(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div className="w-full max-w-3xl rounded-sm border border-enterprise-border bg-enterprise-header flex flex-col max-h-[90vh] shadow-2xl">
        
        {/* Modal Header */}
        <div className="border-b border-enterprise-border bg-enterprise-panel px-5 py-3.5 flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2 mb-0.5">
              <span className="text-[10px] font-mono uppercase tracking-wider text-redwolf-primary font-bold">Zero-Touch Provisioning</span>
              <span className="text-enterprise-textDim">•</span>
              <VendorBadge vendor={node.vendor} />
            </div>
            <h2 className="text-sm font-bold text-white flex items-center gap-2">
              Deploy Operating System — <span className="font-mono">{node.model}</span> (SN: {node.serialNumber})
            </h2>
          </div>
          {!isDeploying && (
            <button onClick={onClose} className="rounded-sm p-1 text-enterprise-textDim hover:text-white">
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        {/* Step Progression Bar (SUSE Rancher Style) */}
        {!isDeploying && (
          <div className="grid grid-cols-4 border-b border-enterprise-border bg-[#11141b] text-center text-xs">
            {[
              { num: 1, label: 'Distribution' },
              { num: 2, label: 'Target Storage' },
              { num: 3, label: 'Network' },
              { num: 4, label: 'Security' },
            ].map((s) => (
              <div 
                key={s.num}
                className={`py-2 border-r border-enterprise-border last:border-r-0 flex items-center justify-center gap-1.5 font-medium ${
                  step === s.num 
                    ? 'bg-enterprise-panel text-white border-b-2 border-b-redwolf-primary' 
                    : step > s.num 
                    ? 'text-[#7ee787]' 
                    : 'text-enterprise-textDim'
                }`}
              >
                <span className={`h-4 w-4 rounded-full text-[10px] font-mono flex items-center justify-center ${
                  step === s.num ? 'bg-redwolf-primary text-white' : step > s.num ? 'bg-[#1b3d2f] text-[#7ee787]' : 'bg-[#1e2430] text-enterprise-textDim'
                }`}>
                  {step > s.num ? <Check className="h-2.5 w-2.5" /> : s.num}
                </span>
                <span>{s.label}</span>
              </div>
            ))}
          </div>
        )}

        {/* Error Alert */}
        {deployError && (
          <div className="mx-5 mt-4 p-2.5 rounded-sm bg-[#311417] border border-[#da3633] text-xs text-[#f85149]">
            {deployError}
          </div>
        )}

        {/* Body Content */}
        <div className="p-5 flex-1 overflow-y-auto text-xs">
          
          {/* STEP 1: OS Selection */}
          {step === 1 && !isDeploying && (
            <div className="space-y-3">
              <div className="text-xs font-semibold text-white">Select Production Operating System Image</div>
              <p className="text-enterprise-textMuted text-[11px]">
                RedWolf streams pre-tested official GenericCloud raw images directly onto bare-metal storage, bypassing traditional installer bottlenecks.
              </p>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-2">
                {distros.map((distro) => (
                  <div
                    key={distro.id}
                    onClick={() => setSelectedOS(distro.id)}
                    className={`cursor-pointer rounded-sm border p-3 transition-colors ${
                      selectedOS === distro.id
                        ? 'border-redwolf-primary bg-enterprise-panel'
                        : 'border-enterprise-border bg-[#13171f] hover:bg-[#1a202c]'
                    }`}
                  >
                    <div className="flex justify-between items-start">
                      <span className="font-bold text-white text-xs">{distro.name}</span>
                      <span className="rounded-sm bg-[#1e2430] border border-enterprise-border px-1.5 py-0.5 text-[9px] font-mono text-enterprise-textMuted">
                        {distro.badge}
                      </span>
                    </div>
                    <div className="text-[10px] text-enterprise-textDim font-mono mt-1">
                      Kernel: {distro.kernel}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* STEP 2: Storage Target */}
          {step === 2 && !isDeploying && (
            <div className="space-y-3">
              <div className="text-xs font-semibold text-white">Select Installation Disk Target</div>
              <p className="text-enterprise-textMuted text-[11px]">
                Target storage is resolved deterministically by immutable identifier (<code className="text-slate-300">/dev/disk/by-id/</code>).
              </p>

              <div className="space-y-2 mt-2">
                {node.storage.map((disk) => (
                  <div
                    key={disk.path}
                    onClick={() => setTargetDrive(disk.path)}
                    className={`cursor-pointer rounded-sm border p-3 transition-colors flex items-center justify-between ${
                      targetDrive === disk.path
                        ? 'border-redwolf-primary bg-enterprise-panel'
                        : 'border-enterprise-border bg-[#13171f] hover:bg-[#1a202c]'
                    }`}
                  >
                    <div className="flex items-center gap-2.5">
                      <HardDrive className="h-4 w-4 text-slate-400" />
                      <div>
                        <div className="font-bold text-white text-xs">{disk.name} — {disk.sizeHuman} ({disk.type})</div>
                        <div className="text-[10px] font-mono text-enterprise-textMuted mt-0.5">{disk.model} (SN: {disk.serial})</div>
                      </div>
                    </div>
                    <span className="font-mono text-[11px] text-[#58a6ff]">{disk.path}</span>
                  </div>
                ))}
              </div>

              {/* Partitioning Preset */}
              <div className="pt-3 border-t border-enterprise-border">
                <div className="text-xs font-semibold text-white mb-2">Partitioning Layout</div>
                <div className="grid grid-cols-2 gap-2">
                  <div 
                    onClick={() => setPartitioning('standard')}
                    className={`cursor-pointer rounded-sm border p-2.5 ${partitioning === 'standard' ? 'border-redwolf-primary bg-enterprise-panel' : 'border-enterprise-border bg-[#13171f]'}`}
                  >
                    <div className="font-semibold text-white">Standard EFI + Root</div>
                    <div className="text-[10px] text-enterprise-textMuted mt-0.5">ESP + Root (auto-expanded via growpart)</div>
                  </div>
                  <div 
                    onClick={() => setPartitioning('lvm')}
                    className={`cursor-pointer rounded-sm border p-2.5 ${partitioning === 'lvm' ? 'border-redwolf-primary bg-enterprise-panel' : 'border-enterprise-border bg-[#13171f]'}`}
                  >
                    <div className="font-semibold text-white">LVM Layout</div>
                    <div className="text-[10px] text-enterprise-textMuted mt-0.5">Multi-volume layout (Tier 2 scripted)</div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* STEP 3: Network Config */}
          {step === 3 && !isDeploying && (
            <div className="space-y-3">
              <div className="text-xs font-semibold text-white">Production Network Configuration</div>
              <p className="text-enterprise-textMuted text-[11px]">
                Cloud-Init binds network configuration directly to physical <strong>MAC addresses</strong>.
              </p>

              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setNetworkMode('static')}
                  className={`flex-1 rounded-sm border py-1.5 font-medium ${networkMode === 'static' ? 'border-redwolf-primary bg-enterprise-panel text-white' : 'border-enterprise-border bg-[#13171f] text-slate-400'}`}
                >
                  Static IP
                </button>
                <button
                  type="button"
                  onClick={() => setNetworkMode('dhcp')}
                  className={`flex-1 rounded-sm border py-1.5 font-medium ${networkMode === 'dhcp' ? 'border-redwolf-primary bg-enterprise-panel text-white' : 'border-enterprise-border bg-[#13171f] text-slate-400'}`}
                >
                  DHCP Automatic
                </button>
              </div>

              {networkMode === 'static' && (
                <div className="grid grid-cols-2 gap-3 pt-2">
                  <div>
                    <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">Static IPv4 Address</label>
                    <input
                      type="text"
                      value={staticIp}
                      onChange={(e) => setStaticIp(e.target.value)}
                      className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-1.5 font-mono text-white text-xs focus:border-[#1f6feb] focus:outline-none"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">Default Gateway</label>
                    <input
                      type="text"
                      value={gateway}
                      onChange={(e) => setGateway(e.target.value)}
                      className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-1.5 font-mono text-white text-xs focus:border-[#1f6feb] focus:outline-none"
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">DNS Resolvers</label>
                    <input
                      type="text"
                      value={dns}
                      onChange={(e) => setDns(e.target.value)}
                      className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-1.5 font-mono text-white text-xs focus:border-[#1f6feb] focus:outline-none"
                    />
                  </div>
                </div>
              )}

              <div className="pt-2 border-t border-enterprise-border flex items-center justify-between">
                <div>
                  <div className="font-medium text-white">Enable LACP 802.3ad Bonding</div>
                  <div className="text-[10px] text-enterprise-textMuted">Binds dual 10GbE/25GbE interfaces into high-availability trunk</div>
                </div>
                <input
                  type="checkbox"
                  checked={enableBonding}
                  onChange={(e) => setEnableBonding(e.target.checked)}
                  className="rounded-sm border-enterprise-border text-redwolf-primary focus:ring-0"
                />
              </div>
            </div>
          )}

          {/* STEP 4: Security Credentials & Cloud-Init Template */}
          {step === 4 && !isDeploying && (
            <div className="space-y-3">
              <div className="text-xs font-semibold text-white">Cloud-Init Configuration &amp; Authentication</div>

              {/* Template Selector */}
              <div>
                <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">
                  Cloud-Init Template
                </label>
                <select
                  value={selectedTemplateId}
                  onChange={(e) => handleTemplateChange(e.target.value)}
                  className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-2 font-medium text-white text-xs focus:border-[#1f6feb] focus:outline-none"
                >
                  {templates.map((tpl) => (
                    <option key={tpl.id} value={tpl.id}>
                      {tpl.name} {tpl.isDefault ? '(Default)' : ''} — [{tpl.distro}]
                    </option>
                  ))}
                </select>
                {templates.find((t) => t.id === selectedTemplateId) && (
                  <div className="text-[11px] text-enterprise-textMuted bg-[#13171f] p-2 mt-1.5 rounded border border-enterprise-border">
                    {templates.find((t) => t.id === selectedTemplateId)?.description}
                  </div>
                )}
              </div>

              {/* Collapsible YAML Customizer */}
              <div className="pt-1 border-t border-enterprise-border">
                <button
                  type="button"
                  onClick={() => setShowCustomYaml(!showCustomYaml)}
                  className="flex items-center gap-1.5 text-xs text-[#58a6ff] hover:text-[#79c0ff] transition-colors"
                >
                  <FileCode className="h-3.5 w-3.5" />
                  <span>{showCustomYaml ? 'Hide Cloud-Config YAML' : 'Preview / Customize Cloud-Config YAML'}</span>
                  {showCustomYaml ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
                </button>

                {showCustomYaml && (
                  <div className="mt-2">
                    <textarea
                      rows={6}
                      value={customUserData}
                      onChange={(e) => setCustomUserData(e.target.value)}
                      className="w-full rounded-sm border border-enterprise-border bg-[#0a0d12] p-2 font-mono text-[11px] text-emerald-400 focus:border-[#1f6feb] focus:outline-none leading-relaxed"
                    />
                    <div className="text-[10px] text-enterprise-textDim mt-1">
                      Must start with <code>#cloud-config</code>. RedWolf automatically appends the authorized SSH keys and password below.
                    </div>
                  </div>
                )}
              </div>
              
              <div>
                <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">Initial Root Password</label>
                <input
                  type="password"
                  value={rootPassword}
                  onChange={(e) => setRootPassword(e.target.value)}
                  className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-1.5 font-mono text-white text-xs focus:border-[#1f6feb] focus:outline-none"
                />
                <div className="text-[10px] text-enterprise-textDim mt-0.5">Encrypted with SHA-512 crypt in Cloud-Init user-data</div>
              </div>

              <div>
                <label className="block text-[11px] font-medium text-enterprise-textMuted mb-1">Public SSH Key</label>
                <textarea
                  rows={2}
                  value={sshKey}
                  onChange={(e) => setSshKey(e.target.value)}
                  className="w-full rounded-sm border border-enterprise-border bg-enterprise-panel p-1.5 font-mono text-xs text-white focus:border-[#1f6feb] focus:outline-none"
                />
              </div>
            </div>
          )}

          {/* Live Deployment Console Terminal */}
          {isDeploying && (
            <div className="space-y-3 py-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Loader2 className="h-4 w-4 animate-spin text-[#d29922]" />
                  <span className="font-medium text-white text-xs">{currentStage}</span>
                </div>
                <span className="font-mono font-bold text-white text-xs">{deployProgress}%</span>
              </div>

              {/* Progress Bar (PatternFly Flat Bar) */}
              <div className="h-1.5 w-full bg-[#1e2430] rounded-sm overflow-hidden">
                <div 
                  className="h-full bg-redwolf-primary transition-all duration-300"
                  style={{ width: `${deployProgress}%` }}
                />
              </div>

              {/* Console Output Log */}
              <div className="rounded-sm border border-enterprise-border bg-[#0a0d12] p-3 font-mono text-[11px] text-slate-300 space-y-1 max-h-56 overflow-y-auto">
                <div className="flex items-center gap-1.5 text-enterprise-textDim border-b border-enterprise-borderSubtle pb-1 mb-2">
                  <Terminal className="h-3.5 w-3.5" />
                  <span>RedWolf Provisioning Stream</span>
                </div>
                {liveLogs.map((log, index) => (
                  <div key={index} className="leading-relaxed">
                    <span className="text-[#58a6ff]">&gt;</span> {log}
                  </div>
                ))}
              </div>
            </div>
          )}

        </div>

        {/* Modal Footer Controls */}
        {!isDeploying && (
          <div className="border-t border-enterprise-border bg-enterprise-panel px-5 py-3 flex items-center justify-between">
            <button
              onClick={() => step > 1 && setStep((prev) => (prev - 1) as any)}
              disabled={step === 1}
              className="flex items-center gap-1 rounded-sm border border-enterprise-border bg-[#13171f] px-3 py-1 text-xs font-medium text-slate-300 hover:bg-enterprise-hover disabled:opacity-30 transition-colors"
            >
              <ArrowLeft className="h-3 w-3" />
              Previous
            </button>

            {step < 4 ? (
              <button
                onClick={() => setStep((prev) => (prev + 1) as any)}
                className="flex items-center gap-1 rounded-sm bg-[#1f6feb] px-3 py-1 text-xs font-medium text-white hover:bg-[#388bfd] transition-colors"
              >
                Next Step
                <ArrowRight className="h-3 w-3" />
              </button>
            ) : (
              <button
                onClick={handleStartDeployment}
                className="flex items-center gap-1.5 rounded-sm bg-redwolf-primary px-4 py-1 text-xs font-semibold text-white hover:bg-redwolf-hover transition-colors"
              >
                <Play className="h-3 w-3 fill-current" />
                Start Bare-Metal Provisioning
              </button>
            )}
          </div>
        )}

      </div>
    </div>
  );
};
