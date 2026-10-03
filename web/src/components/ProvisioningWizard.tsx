import React, { useState, useEffect } from 'react';
import type { ServerNode, DeploymentConfig, OperatingSystem, CloudInitTemplate } from '../types';
import { VendorBadge } from './Badges';
import { 
  X, Check, ArrowRight, ArrowLeft, HardDrive, Play, Loader2, Terminal, FileCode, ChevronDown, ChevronUp, CheckCircle2, AlertCircle
} from 'lucide-react';

interface ProvisioningWizardProps {
  node: ServerNode | null;
  onClose: () => void;
  onDeploy: (config: DeploymentConfig) => Promise<void>;
}

export const ProvisioningWizard: React.FC<ProvisioningWizardProps> = ({ node, onClose, onDeploy }) => {
  if (!node) return null;

  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);

  // Form State
  const [selectedOS, setSelectedOS] = useState<OperatingSystem>('AlmaLinux 9');
  const [targetDrive, setTargetDrive] = useState<string>(node.storage[0]?.byId || node.storage[0]?.path || '/dev/nvme0n1');
  const [partitioning, setPartitioning] = useState<'standard' | 'lvm'>('standard');
  const [networkMode, setNetworkMode] = useState<'static' | 'dhcp'>('static');
  const [staticIp, setStaticIp] = useState('10.10.100.25');
  const [gateway, setGateway] = useState('10.10.100.1');
  const [dns, setDns] = useState('1.1.1.1, 8.8.8.8');
  const [enableBonding, setEnableBonding] = useState(false);
  const [rootPassword, setRootPassword] = useState('RedWolf#2026!');
  const [sshKey, setSshKey] = useState('');

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

  // Execution & Live Telemetry State
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployError, setDeployError] = useState<string | null>(null);

  const isNodeProvisioning = node.status === 'PROVISIONING';
  const isNodeActive = node.status === 'ACTIVE' && (node.provisioningState?.progress === 100 || isDeploying);
  const activeDeploy = isDeploying || isNodeProvisioning || isNodeActive;

  const currentProgress = node.provisioningState?.progress ?? (activeDeploy ? (isNodeActive ? 100 : 5) : 0);
  const currentStage = node.provisioningState?.stage || (isNodeActive ? 'Provisioning completed successfully.' : activeDeploy ? 'Connecting to discovery agent on target server...' : '');
  const liveLogs = node.provisioningState?.logs && node.provisioningState.logs.length > 0
    ? node.provisioningState.logs
    : [`[${new Date().toISOString().substring(11, 19)}] Provisioning manifest registered with appliance daemon`];

  // Distributions list
  const distros = [
    {
      id: 'AlmaLinux 9' as OperatingSystem,
      name: 'AlmaLinux 9.4',
      kernel: 'Linux 5.14',
    },
    {
      id: 'AlmaLinux 8' as OperatingSystem,
      name: 'AlmaLinux 8.10',
      kernel: 'Linux 4.18',
    },
    {
      id: 'Debian 12' as OperatingSystem,
      name: 'Debian 12',
      kernel: 'Linux 6.1 LTS',
    },
    {
      id: 'AlmaLinux 10' as OperatingSystem,
      name: 'AlmaLinux 10',
      kernel: 'Linux 6.x',
    },
    {
      id: 'Debian 13' as OperatingSystem,
      name: 'Debian 13',
      kernel: 'Linux 6.6',
    },
  ];

  const handleStartDeployment = async () => {
    setIsDeploying(true);
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
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : 'Deployment failed';
      setDeployError(errMsg);
      setIsDeploying(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="w-full max-w-2xl rounded-sm border border-[#212836] bg-[#121620] flex flex-col max-h-[85vh] shadow-2xl">
        
        {/* Modal Header */}
        <div className="border-b border-[#212836] bg-[#151b24] px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <VendorBadge vendor={node.vendor} />
            <h2 className="text-xs font-semibold text-white">
              {activeDeploy ? 'Provisioning Status' : 'Deploy OS'} — <span className="font-mono">{node.model}</span> (SN: {node.serialNumber})
            </h2>
          </div>
          <button 
            onClick={onClose} 
            className="rounded p-1 text-slate-400 hover:text-white transition-colors"
            title={activeDeploy ? "Minimize to background" : "Close"}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Step Progression Bar */}
        {!activeDeploy && (
          <div className="grid grid-cols-4 border-b border-[#212836] bg-[#0c0e14] text-center text-xs">
            {[
              { num: 1 as const, label: 'Distribution' },
              { num: 2 as const, label: 'Target Storage' },
              { num: 3 as const, label: 'Network' },
              { num: 4 as const, label: 'Security' },
            ].map((s) => (
              <div 
                key={s.num}
                className={`py-2 border-r border-[#212836] last:border-r-0 flex items-center justify-center gap-1.5 font-medium ${
                  step === s.num 
                    ? 'bg-[#151b24] text-white border-b-2 border-b-redwolf-primary' 
                    : step > s.num 
                    ? 'text-emerald-400' 
                    : 'text-slate-500'
                }`}
              >
                <span className={`h-4 w-4 rounded-full text-[10px] font-mono flex items-center justify-center ${
                  step === s.num ? 'bg-redwolf-primary text-white' : step > s.num ? 'bg-emerald-950 text-emerald-400 border border-emerald-800' : 'bg-[#181f2c] text-slate-500'
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
          <div className="mx-4 mt-3 p-2.5 rounded-sm bg-red-950/60 border border-red-800 text-xs text-red-300">
            {deployError}
          </div>
        )}

        {/* Body Content */}
        <div className="p-4 flex-1 overflow-y-auto text-xs">
          
          {/* STEP 1: OS Selection */}
          {step === 1 && !activeDeploy && (
            <div className="space-y-3">
              <div className="text-xs font-medium text-slate-300">Select Operating System</div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {distros.map((distro) => (
                  <div
                    key={distro.id}
                    onClick={() => setSelectedOS(distro.id)}
                    className={`cursor-pointer rounded-sm border p-2.5 transition-colors ${
                      selectedOS === distro.id
                        ? 'border-redwolf-primary bg-[#181f2c]'
                        : 'border-[#212836] bg-[#0f1218] hover:bg-[#151b24]'
                    }`}
                  >
                    <div className="flex justify-between items-start">
                      <span className="font-semibold text-white text-xs">{distro.name}</span>
                      <span className="text-[10px] text-slate-400 font-mono">
                        {distro.kernel}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* STEP 2: Storage Target */}
          {step === 2 && !activeDeploy && (
            <div className="space-y-3">
              <div className="text-xs font-medium text-slate-300">Select Target Drive</div>

              <div className="space-y-2">
                {(node.storage || []).map((disk) => {
                  const diskTarget = disk.byId || disk.path;
                  return (
                    <div
                      key={diskTarget}
                      onClick={() => setTargetDrive(diskTarget)}
                      className={`cursor-pointer rounded-sm border p-2.5 transition-colors flex items-center justify-between ${
                        targetDrive === diskTarget
                          ? 'border-redwolf-primary bg-[#181f2c]'
                          : 'border-[#212836] bg-[#0f1218] hover:bg-[#151b24]'
                      }`}
                    >
                      <div className="flex items-center gap-2.5">
                        <HardDrive className="h-4 w-4 text-slate-500" />
                        <div>
                          <div className="font-semibold text-white text-xs">{disk.name} — {disk.sizeHuman} ({disk.type})</div>
                          <div className="text-[10px] font-mono text-slate-400 mt-0.5">{disk.model} (SN: {disk.serial})</div>
                        </div>
                      </div>
                      <span className="font-mono text-[11px] text-sky-400 max-w-[200px] truncate" title={diskTarget}>
                        {disk.byId ? disk.byId.replace('/dev/disk/by-id/', 'by-id/...') : disk.path}
                      </span>
                    </div>
                  );
                })}
              </div>

              {/* Partitioning Preset */}
              <div className="pt-2 border-t border-[#212836]">
                <div className="text-[11px] font-medium text-slate-400 mb-1.5">Partitioning Layout</div>
                <div className="grid grid-cols-2 gap-2">
                  <div 
                    onClick={() => setPartitioning('standard')}
                    className={`cursor-pointer rounded-sm border p-2 ${partitioning === 'standard' ? 'border-redwolf-primary bg-[#181f2c]' : 'border-[#212836] bg-[#0f1218]'}`}
                  >
                    <div className="font-medium text-white text-xs">Standard (EFI + Root)</div>
                    <div className="text-[10px] text-slate-400 mt-0.5">ESP + Root expanded to disk size</div>
                  </div>
                  <div 
                    onClick={() => setPartitioning('lvm')}
                    className={`cursor-pointer rounded-sm border p-2 ${partitioning === 'lvm' ? 'border-redwolf-primary bg-[#181f2c]' : 'border-[#212836] bg-[#0f1218]'}`}
                  >
                    <div className="font-medium text-white text-xs">LVM</div>
                    <div className="text-[10px] text-slate-400 mt-0.5">Logical Volume Manager</div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* STEP 3: Network Config */}
          {step === 3 && !activeDeploy && (
            <div className="space-y-3">
              <div className="text-xs font-medium text-slate-300">Network Configuration</div>

              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setNetworkMode('static')}
                  className={`flex-1 rounded-sm border py-1.5 text-xs font-medium ${networkMode === 'static' ? 'border-redwolf-primary bg-[#181f2c] text-white' : 'border-[#212836] bg-[#0f1218] text-slate-400'}`}
                >
                  Static IP
                </button>
                <button
                  type="button"
                  onClick={() => setNetworkMode('dhcp')}
                  className={`flex-1 rounded-sm border py-1.5 text-xs font-medium ${networkMode === 'dhcp' ? 'border-redwolf-primary bg-[#181f2c] text-white' : 'border-[#212836] bg-[#0f1218] text-slate-400'}`}
                >
                  DHCP Automatic
                </button>
              </div>

              {networkMode === 'static' && (
                <div className="grid grid-cols-2 gap-3 pt-1">
                  <div>
                    <label className="block text-[11px] font-medium text-slate-400 mb-1">Static IPv4 Address</label>
                    <input
                      type="text"
                      value={staticIp}
                      onChange={(e) => setStaticIp(e.target.value)}
                      className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 font-mono text-white text-xs focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-medium text-slate-400 mb-1">Default Gateway</label>
                    <input
                      type="text"
                      value={gateway}
                      onChange={(e) => setGateway(e.target.value)}
                      className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 font-mono text-white text-xs focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-[11px] font-medium text-slate-400 mb-1">DNS Resolvers</label>
                    <input
                      type="text"
                      value={dns}
                      onChange={(e) => setDns(e.target.value)}
                      className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 font-mono text-white text-xs focus:border-redwolf-primary focus:outline-none"
                    />
                  </div>
                </div>
              )}

              <div className="pt-2 border-t border-[#212836] flex items-center justify-between">
                <div>
                  <div className="font-medium text-white text-xs">Enable LACP 802.3ad Bonding</div>
                  <div className="text-[10px] text-slate-400">Combines dual NICs into trunk</div>
                </div>
                <input
                  type="checkbox"
                  checked={enableBonding}
                  onChange={(e) => setEnableBonding(e.target.checked)}
                  className="rounded border-[#283244] bg-[#0c0e14] text-redwolf-primary focus:ring-0"
                />
              </div>
            </div>
          )}

          {/* STEP 4: Security Credentials & Cloud-Init Template */}
          {step === 4 && !activeDeploy && (
            <div className="space-y-3">
              <div className="text-xs font-medium text-slate-300">Cloud-Init &amp; Authentication</div>

              {/* Template Selector */}
              <div>
                <label className="block text-[11px] font-medium text-slate-400 mb-1">
                  Template
                </label>
                <select
                  value={selectedTemplateId}
                  onChange={(e) => handleTemplateChange(e.target.value)}
                  className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 text-white text-xs focus:border-redwolf-primary focus:outline-none"
                >
                  {templates.map((tpl) => (
                    <option key={tpl.id} value={tpl.id}>
                      {tpl.name} {tpl.isDefault ? '(Default)' : ''}
                    </option>
                  ))}
                </select>
              </div>

              {/* Collapsible YAML Customizer */}
              <div className="pt-1">
                <button
                  type="button"
                  onClick={() => setShowCustomYaml(!showCustomYaml)}
                  className="flex items-center gap-1.5 text-xs text-sky-400 hover:text-sky-300 transition-colors"
                >
                  <FileCode className="h-3.5 w-3.5" />
                  <span>{showCustomYaml ? 'Hide Cloud-Config YAML' : 'Edit Cloud-Config YAML'}</span>
                  {showCustomYaml ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
                </button>

                {showCustomYaml && (
                  <div className="mt-2">
                    <textarea
                      rows={5}
                      value={customUserData}
                      onChange={(e) => setCustomUserData(e.target.value)}
                      className="w-full rounded-sm border border-[#232b3b] bg-[#080a0f] p-2 font-mono text-[11px] text-emerald-400 focus:border-redwolf-primary focus:outline-none leading-relaxed"
                    />
                  </div>
                )}
              </div>
              
              <div>
                <label className="block text-[11px] font-medium text-slate-400 mb-1">Root Password</label>
                <input
                  type="password"
                  value={rootPassword}
                  onChange={(e) => setRootPassword(e.target.value)}
                  className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 font-mono text-white text-xs focus:border-redwolf-primary focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-[11px] font-medium text-slate-400 mb-1">Public SSH Key (Optional)</label>
                <textarea
                  rows={2}
                  value={sshKey}
                  onChange={(e) => setSshKey(e.target.value)}
                  placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5..."
                  className="w-full rounded-sm border border-[#232b3b] bg-[#0c0e14] p-1.5 font-mono text-xs text-white placeholder-slate-600 focus:border-redwolf-primary focus:outline-none"
                />
              </div>
            </div>
          )}

          {/* Live Deployment Console Terminal */}
          {activeDeploy && (
            <div className="space-y-3 py-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  {isNodeActive || currentProgress >= 100 ? (
                    <CheckCircle2 className="h-4 w-4 text-emerald-400" />
                  ) : node.status === 'ERROR' ? (
                    <AlertCircle className="h-4 w-4 text-rose-400" />
                  ) : (
                    <Loader2 className="h-4 w-4 animate-spin text-amber-400" />
                  )}
                  <span className="font-medium text-white text-xs">{currentStage}</span>
                </div>
                <span className="font-mono font-bold text-white text-xs">{currentProgress}%</span>
              </div>

              {/* Progress Bar */}
              <div className="h-1.5 w-full bg-[#1e2430] rounded-sm overflow-hidden">
                <div 
                  className={`h-full transition-all duration-300 ${
                    isNodeActive || currentProgress >= 100
                      ? 'bg-emerald-500'
                      : node.status === 'ERROR'
                      ? 'bg-rose-500'
                      : 'bg-redwolf-primary'
                  }`}
                  style={{ width: `${currentProgress}%` }}
                />
              </div>

              {/* Console Output Log */}
              <div className="rounded-sm border border-[#212836] bg-[#080a0f] p-3 font-mono text-[11px] text-slate-300 space-y-1 max-h-52 overflow-y-auto">
                <div className="flex items-center justify-between text-slate-500 border-b border-[#1a202c] pb-1 mb-2">
                  <div className="flex items-center gap-1.5">
                    <Terminal className="h-3.5 w-3.5" />
                    <span>Real-time Provisioning Telemetry</span>
                  </div>
                  <span className="text-[10px] text-slate-500 font-mono">
                    Target: {node.provisioningState?.targetDrive || targetDrive}
                  </span>
                </div>
                {liveLogs.map((log, index) => (
                  <div key={`${index}-${log.slice(0, 32)}`} className="leading-relaxed">
                    <span className="text-sky-400">&gt;</span> {log}
                  </div>
                ))}
              </div>

              {/* Status footer information */}
              <div className="flex items-center justify-between text-[11px] text-slate-400 pt-1">
                <span>Distribution: <span className="font-medium text-slate-200">{node.provisioningState?.os || selectedOS}</span></span>
                <span>Target IP: <span className="font-mono text-slate-200">{node.provisioningState?.targetIp || (networkMode === 'static' ? staticIp : 'DHCP')}</span></span>
              </div>
            </div>
          )}

        </div>

        {/* Modal Footer Controls */}
        {!activeDeploy ? (
          <div className="border-t border-[#212836] bg-[#151b24] px-4 py-2.5 flex items-center justify-between">
            <button
              onClick={() => setStep((prev) => (prev === 4 ? 3 : prev === 3 ? 2 : 1))}
              disabled={step === 1}
              className="flex items-center gap-1 rounded-sm border border-[#283244] bg-[#0f1218] px-3 py-1 text-xs font-medium text-slate-300 hover:bg-[#1a202c] disabled:opacity-30 transition-colors"
            >
              <ArrowLeft className="h-3 w-3" />
              Previous
            </button>

            {step < 4 ? (
              <button
                onClick={() => setStep((prev) => (prev === 1 ? 2 : prev === 2 ? 3 : 4))}
                className="flex items-center gap-1 rounded-sm bg-[#182638] border border-[#283f5e] hover:bg-[#20324a] px-3 py-1 text-xs font-medium text-sky-200 transition-colors"
              >
                Next
                <ArrowRight className="h-3 w-3" />
              </button>
            ) : (
              <button
                onClick={handleStartDeployment}
                className="flex items-center gap-1.5 rounded-sm bg-redwolf-primary px-4 py-1 text-xs font-semibold text-white hover:bg-redwolf-hover transition-colors shadow-sm"
              >
                <Play className="h-3 w-3 fill-current" />
                Start Provisioning
              </button>
            )}
          </div>
        ) : (
          <div className="border-t border-[#212836] bg-[#151b24] px-4 py-2.5 flex items-center justify-between">
            <span className="text-[11px] text-slate-400">
              {isNodeActive || currentProgress >= 100 
                ? 'Server is online and ready for production workloads.' 
                : 'Task is executing zero-touch automation on bare-metal hardware.'}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={onClose}
                className={`rounded-sm px-3 py-1 text-xs font-medium transition-colors ${
                  isNodeActive || currentProgress >= 100
                    ? 'bg-emerald-600 hover:bg-emerald-500 text-white'
                    : 'border border-[#283244] bg-[#0f1218] hover:bg-[#1a202c] text-slate-300'
                }`}
              >
                {isNodeActive || currentProgress >= 100 ? 'Done' : 'Run in Background'}
              </button>
            </div>
          </div>
        )}

      </div>
    </div>
  );
};
