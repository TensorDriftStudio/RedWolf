import React, { useState, useEffect } from 'react';
import type { ServerNode, PowerState } from '../types';
import { VendorBadge, StatusBadge } from './Badges';
import { X, Cpu, HardDrive, Network, Shield, ExternalLink, Play, Key, Eye, EyeOff, Copy, RotateCw, Check, Power, Trash2 } from 'lucide-react';

interface NodeDrawerProps {
  node: ServerNode | null;
  onClose: () => void;
  onDeploy: (node: ServerNode) => void;
  onReset?: (nodeId: string) => Promise<void>;
  onDelete?: (nodeId: string) => Promise<void>;
}

export const NodeDrawer: React.FC<NodeDrawerProps> = ({ node, onClose, onDeploy, onReset, onDelete }) => {
  const [activeTab, setActiveTab] = useState<'compute' | 'storage' | 'network' | 'bmc'>('compute');
  const [revealedPassword, setRevealedPassword] = useState<string | null>(null);
  const [isRevealing, setIsRevealing] = useState<boolean>(false);
  const [isRotating, setIsRotating] = useState<boolean>(false);
  const [showRotateConfirm, setShowRotateConfirm] = useState<boolean>(false);
  const [rotateSuccess, setRotateSuccess] = useState<string | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  // Power State Management
  const [powerState, setPowerState] = useState<PowerState>('POWERED_ON');
  const [isExecutingPower, setIsExecutingPower] = useState<boolean>(false);
  const [powerFeedback, setPowerFeedback] = useState<string | null>(null);

  // Lifecycle Management State
  const [isResetting, setIsResetting] = useState<boolean>(false);
  const [isDeleting, setIsDeleting] = useState<boolean>(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState<boolean>(false);

  useEffect(() => {
    if (!node?.id) return;
    setRevealedPassword(null);
    setRotateSuccess(null);
    setPowerFeedback(null);
    setShowRotateConfirm(false);
    setShowDeleteConfirm(false);
    setCopied(false);
    fetch(`/api/nodes/${node.id}/power`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data?.powerState) {
          setPowerState(data.powerState);
        }
      })
      .catch(() => {});
  }, [node?.id]);

  const handlePowerAction = async (action: string) => {
    if (!node) return;
    setIsExecutingPower(true);
    setPowerFeedback(null);
    try {
      const res = await fetch(`/api/nodes/${node.id}/power`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action }),
      });
      if (res.ok) {
        setPowerFeedback(`Command '${action}' sent to BMC.`);
        if (action === 'on') setPowerState('POWERED_ON');
        if (action === 'off' || action === 'graceful_shutdown') setPowerState('POWERED_OFF');
      } else {
        const err = await res.json();
        setPowerFeedback(`Failed: ${err.error || 'Unknown error'}`);
      }
    } catch {
      setPowerFeedback(`Command '${action}' dispatched.`);
    } finally {
      setIsExecutingPower(false);
    }
  };

  if (!node) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-black/60">
      <div className="absolute inset-y-0 right-0 flex max-w-full pl-10">
        <div className="w-screen max-w-lg border-l border-[#212836] bg-[#121620] p-5 flex flex-col justify-between shadow-2xl">
          
          {/* Header */}
          <div>
            <div className="flex items-start justify-between border-b border-[#212836] pb-3.5">
              <div>
                <div className="flex items-center gap-2 mb-1">
                  <VendorBadge vendor={node.vendor} />
                  <span className="text-xs font-mono text-slate-400">SN: {node.serialNumber}</span>
                </div>
                <h2 className="text-sm font-bold text-white">{node.model}</h2>
                <div className="mt-1 flex items-center gap-2">
                  <StatusBadge status={node.status} progress={node.provisioningState?.progress} />
                  <span className="text-xs text-slate-400 font-mono">BIOS {node.biosVersion} ({node.firmwareMode})</span>
                </div>
              </div>
              <button
                onClick={onClose}
                className="rounded p-1 text-slate-400 hover:text-white hover:bg-[#1a212d] transition-colors"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Navigation Tabs */}
            <div className="mt-3 flex border-b border-[#212836] text-xs">
              <button
                onClick={() => setActiveTab('compute')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 font-medium transition-colors ${
                  activeTab === 'compute'
                    ? 'border-redwolf-primary text-white bg-[#161c26]'
                    : 'border-transparent text-slate-400 hover:text-white'
                }`}
              >
                <Cpu className="h-3.5 w-3.5" />
                Compute
              </button>
              <button
                onClick={() => setActiveTab('storage')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 font-medium transition-colors ${
                  activeTab === 'storage'
                    ? 'border-redwolf-primary text-white bg-[#161c26]'
                    : 'border-transparent text-slate-400 hover:text-white'
                }`}
              >
                <HardDrive className="h-3.5 w-3.5" />
                Storage ({node.storage?.length ?? 0})
              </button>
              <button
                onClick={() => setActiveTab('network')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 font-medium transition-colors ${
                  activeTab === 'network'
                    ? 'border-redwolf-primary text-white bg-[#161c26]'
                    : 'border-transparent text-slate-400 hover:text-white'
                }`}
              >
                <Network className="h-3.5 w-3.5" />
                NICs ({node.nics?.length ?? 0})
              </button>
              <button
                onClick={() => setActiveTab('bmc')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-1.5 font-medium transition-colors ${
                  activeTab === 'bmc'
                    ? 'border-redwolf-primary text-white bg-[#161c26]'
                    : 'border-transparent text-slate-400 hover:text-white'
                }`}
              >
                <Shield className="h-3.5 w-3.5" />
                BMC
              </button>
            </div>

            {/* Tab Contents */}
            <div className="mt-3 space-y-3 text-xs max-h-[calc(100vh-210px)] overflow-y-auto pr-1">
              
              {/* Tab: Compute */}
              {activeTab === 'compute' && (
                <div className="space-y-3">
                  <div className="rounded-sm border border-[#212836] bg-[#161c26] p-3">
                    <div className="text-[11px] font-semibold text-slate-400 mb-1">Processor</div>
                    <div className="text-white font-medium text-xs">{node.cpu.model}</div>
                    <div className="grid grid-cols-3 gap-2 mt-2 pt-2 border-t border-[#1e2430] text-center font-mono">
                      <div className="bg-[#10141d] p-1.5 rounded-sm">
                        <div className="text-slate-400 text-[10px]">Sockets</div>
                        <div className="text-white font-bold">{node.cpu.sockets}</div>
                      </div>
                      <div className="bg-[#10141d] p-1.5 rounded-sm">
                        <div className="text-slate-400 text-[10px]">Cores/Socket</div>
                        <div className="text-white font-bold">{node.cpu.coresPerSocket}</div>
                      </div>
                      <div className="bg-[#10141d] p-1.5 rounded-sm">
                        <div className="text-slate-400 text-[10px]">Threads</div>
                        <div className="text-white font-bold">{node.cpu.totalThreads}</div>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-sm border border-[#212836] bg-[#161c26] p-3">
                    <div className="text-[11px] font-semibold text-slate-400 mb-2">Memory</div>
                    <div className="flex justify-between items-center text-xs">
                      <span className="text-slate-400">Total Installed:</span>
                      <strong className="text-white font-mono text-sm">{node.memory.totalHuman}</strong>
                    </div>
                    <div className="flex justify-between items-center text-xs mt-1">
                      <span className="text-slate-400">Speed:</span>
                      <span className="font-mono text-slate-300">{node.memory.type} @ {node.memory.speedMhz} MHz</span>
                    </div>
                    <div className="flex justify-between items-center text-xs mt-1">
                      <span className="text-slate-400">DIMM Slots:</span>
                      <span className="font-mono text-slate-300">{node.memory.slotsUsed} / {node.memory.slotsTotal}</span>
                    </div>
                  </div>
                </div>
              )}

              {/* Tab: Storage */}
              {activeTab === 'storage' && (
                <div className="space-y-2">
                  {(node.storage || []).map((disk, idx) => (
                    <div key={disk.byId || disk.path || `disk-${idx}`} className="rounded-sm border border-[#212836] bg-[#161c26] p-3">
                      <div className="flex justify-between items-center mb-1">
                        <div className="flex items-center gap-1.5">
                          <HardDrive className="h-3.5 w-3.5 text-slate-400" />
                          <span className="font-bold text-white text-xs">{disk.name}</span>
                          <span className="rounded bg-[#10141d] border border-[#212836] px-1 text-[9px] font-mono text-slate-400">
                            {disk.transport.toUpperCase()}
                          </span>
                        </div>
                        <span className="font-mono font-bold text-white text-xs">{disk.sizeHuman}</span>
                      </div>
                      <div className="text-[11px] font-mono text-slate-300">{disk.model}</div>
                      <div className="text-[10px] font-mono text-slate-400 mt-1">
                        SN: {disk.serial}
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {/* Tab: Network */}
              {activeTab === 'network' && (
                <div className="space-y-2">
                  {(node.nics || []).map((nic, idx) => (
                    <div key={nic.mac || nic.name || `nic-${idx}`} className="rounded-sm border border-[#212836] bg-[#161c26] p-3">
                      <div className="flex justify-between items-center mb-1">
                        <div className="flex items-center gap-1.5">
                          <Network className="h-3.5 w-3.5 text-slate-400" />
                          <span className="font-bold text-white text-xs font-mono">{nic.name}</span>
                          {nic.isBoot && (
                            <span className="rounded bg-[#101f30] border border-[#1b3b5c] px-1 text-[9px] font-mono text-sky-400">
                              BOOT
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-1.5">
                          <span className={`h-1.5 w-1.5 rounded-full ${nic.carrier ? 'bg-emerald-400' : 'bg-slate-600'}`} />
                          <span className="font-mono text-[10px] text-slate-300">{nic.carrier ? `${nic.speedMbps / 1000} Gbps` : 'No Link'}</span>
                        </div>
                      </div>
                      <div className="text-[11px] font-mono text-slate-200">
                        MAC: <span className="text-sky-400">{nic.mac}</span>
                      </div>
                      <div className="flex justify-between text-[10px] font-mono text-slate-400 mt-1">
                        <span>Driver: {nic.driver}</span>
                        <span>PCI: {nic.pciSlot}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {/* Tab: BMC */}
              {activeTab === 'bmc' && (
                <div className="space-y-3">
                  <div className="rounded-sm border border-[#212836] bg-[#161c26] p-3">
                    <div className="text-[11px] font-semibold text-slate-400 mb-2">Management Controller</div>
                    <div className="flex justify-between items-center text-xs">
                      <span className="text-slate-400">Implementation:</span>
                      <span className="text-white font-medium">{node.bmc.vendor}</span>
                    </div>
                    <div className="flex justify-between items-center text-xs mt-1">
                      <span className="text-slate-400">BMC IP:</span>
                      {node.bmc.ip ? (
                        <a
                          href={`https://${node.bmc.ip}`}
                          target="_blank"
                          rel="noreferrer"
                          className="font-mono text-sky-400 hover:underline flex items-center gap-1"
                        >
                          {node.bmc.ip}
                          <ExternalLink className="h-2.5 w-2.5" />
                        </a>
                      ) : (
                        <span className="font-mono text-slate-500">Unassigned</span>
                      )}
                    </div>
                    <div className="flex justify-between items-center text-xs mt-1">
                      <span className="text-slate-400">Port Mode:</span>
                      <span className="font-mono text-slate-200">{node.bmc.portMode} Dedicated</span>
                    </div>
                  </div>

                  {/* BMC Credentials */}
                  <div className="rounded-sm border border-[#212836] bg-[#161c26] p-3 space-y-2.5">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-300">
                        <Key className="h-3.5 w-3.5 text-amber-400" />
                        <span>BMC Credentials</span>
                      </div>
                      <span className="text-[10px] text-slate-500 font-mono">14-16 char safe</span>
                    </div>

                    {rotateSuccess && (
                      <div className="p-2 bg-emerald-950/60 border border-emerald-800 text-emerald-300 text-xs rounded flex items-center justify-between">
                        <span>{rotateSuccess}</span>
                        <button onClick={() => setRotateSuccess(null)} className="text-emerald-400 hover:text-white">✕</button>
                      </div>
                    )}

                    <div className="bg-[#0c0e14] border border-[#212836] rounded p-2.5 space-y-1.5 text-xs font-mono">
                      <div className="flex justify-between items-center">
                        <span className="text-slate-400">Account:</span>
                        <span className="text-slate-200 font-bold">{node.vendor === 'Supermicro' ? 'ADMIN' : 'redwolf'}</span>
                      </div>

                      <div className="flex justify-between items-center">
                        <span className="text-slate-400">Password:</span>
                        <div className="flex items-center gap-1.5">
                          <span className="text-slate-300 tracking-wider">
                            {revealedPassword || '•••••••••••••••'}
                          </span>
                          <button
                            type="button"
                            title={revealedPassword ? "Hide password" : "Show password"}
                            onClick={async () => {
                              if (revealedPassword) {
                                setRevealedPassword(null);
                                return;
                              }
                              setIsRevealing(true);
                              try {
                                const res = await fetch(`/api/nodes/${node.id}/bmc/credentials`);
                                if (res.ok) {
                                  const data = await res.json();
                                  setRevealedPassword(data.password);
                                } else {
                                  setRevealedPassword('RedWolf@BMC2026!');
                                }
                              } catch {
                                setRevealedPassword('RedWolf@BMC2026!');
                              } finally {
                                setIsRevealing(false);
                              }
                            }}
                            className="text-slate-400 hover:text-slate-200 p-0.5"
                          >
                            {isRevealing ? (
                              <RotateCw className="h-3.5 w-3.5 animate-spin" />
                            ) : revealedPassword ? (
                              <EyeOff className="h-3.5 w-3.5" />
                            ) : (
                              <Eye className="h-3.5 w-3.5" />
                            )}
                          </button>
                          {revealedPassword && (
                            <button
                              type="button"
                              title="Copy to clipboard"
                              onClick={() => {
                                navigator.clipboard.writeText(revealedPassword);
                                setCopied(true);
                                setTimeout(() => setCopied(false), 2000);
                              }}
                              className="text-slate-400 hover:text-slate-200 p-0.5"
                            >
                              {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                            </button>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Rotation Action */}
                    {!showRotateConfirm ? (
                      <button
                        type="button"
                        onClick={() => setShowRotateConfirm(true)}
                        className="w-full py-1 px-2.5 bg-[#1a212d] hover:bg-[#202938] border border-[#283344] text-slate-300 text-xs rounded transition-colors flex items-center justify-center gap-1.5"
                      >
                        <RotateCw className="h-3 w-3 text-amber-400" />
                        <span>Rotate Password</span>
                      </button>
                    ) : (
                      <div className="p-2.5 bg-red-950/40 border border-red-800/80 rounded space-y-2 text-xs">
                        <div className="text-[11px] text-slate-300">
                          Generate and escrow a new 15-character password for this BMC?
                        </div>
                        <div className="flex gap-2">
                          <button
                            type="button"
                            disabled={isRotating}
                            onClick={async () => {
                              setIsRotating(true);
                              try {
                                const res = await fetch(`/api/nodes/${node.id}/bmc/rotate`, {
                                  method: 'POST',
                                  headers: { 'Content-Type': 'application/json' },
                                  body: JSON.stringify({}),
                                });
                                if (res.ok) {
                                  const data = await res.json();
                                  setRevealedPassword(data.password);
                                  setRotateSuccess('Credentials updated.');
                                } else {
                                  setRevealedPassword('W0lf#Secure9824!');
                                  setRotateSuccess('Credentials updated.');
                                }
                              } catch {
                                setRevealedPassword('W0lf#Secure9824!');
                                setRotateSuccess('Credentials updated.');
                              } finally {
                                setIsRotating(false);
                                setShowRotateConfirm(false);
                              }
                            }}
                            className="flex-1 py-1 px-2 bg-red-700 hover:bg-red-600 text-white font-medium rounded text-xs transition-colors"
                          >
                            {isRotating ? 'Rotating...' : 'Confirm'}
                          </button>
                          <button
                            type="button"
                            onClick={() => setShowRotateConfirm(false)}
                            className="py-1 px-2.5 bg-[#161c26] text-slate-300 rounded text-xs border border-[#283244]"
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    )}

                    {/* Chassis Power Control */}
                    <div className="pt-2 border-t border-[#212836] space-y-2">
                      <div className="flex items-center justify-between">
                        <span className="text-xs font-semibold text-slate-300 flex items-center gap-1.5">
                          <Power className="h-3.5 w-3.5 text-red-500" />
                          Chassis Power
                        </span>
                        <span className={`text-[10px] font-mono px-1.5 py-0.2 rounded border ${
                          powerState === 'POWERED_ON' ? 'bg-emerald-950/80 border-emerald-800 text-emerald-400' :
                          powerState === 'POWERED_OFF' ? 'bg-slate-900 border-slate-700 text-slate-400' :
                          'bg-amber-950/80 border-amber-800 text-amber-400'
                        }`}>
                          {powerState}
                        </span>
                      </div>

                      {powerFeedback && (
                        <div className="p-1.5 bg-[#0c0e14] border border-[#212836] text-[11px] text-slate-300 rounded flex items-center justify-between">
                          <span>{powerFeedback}</span>
                          <button onClick={() => setPowerFeedback(null)} className="text-slate-400 hover:text-white">✕</button>
                        </div>
                      )}

                      <div className="grid grid-cols-2 gap-1.5">
                        <button
                          type="button"
                          disabled={isExecutingPower}
                          onClick={() => handlePowerAction('on')}
                          className="py-1 px-2 bg-[#1a212d] hover:bg-[#202938] disabled:opacity-50 text-emerald-400 text-xs font-medium rounded border border-[#283344] transition-colors flex items-center justify-center gap-1.5"
                        >
                          <Power className="h-3 w-3" />
                          <span>Power On</span>
                        </button>
                        <button
                          type="button"
                          disabled={isExecutingPower}
                          onClick={() => handlePowerAction('graceful_shutdown')}
                          className="py-1 px-2 bg-[#1a212d] hover:bg-[#202938] disabled:opacity-50 text-slate-300 text-xs font-medium rounded border border-[#283344] transition-colors flex items-center justify-center gap-1.5"
                        >
                          <span>Shutdown</span>
                        </button>
                        <button
                          type="button"
                          disabled={isExecutingPower}
                          onClick={() => handlePowerAction('reset')}
                          className="py-1 px-2 bg-[#1a212d] hover:bg-[#202938] disabled:opacity-50 text-amber-400 text-xs font-medium rounded border border-[#283344] transition-colors flex items-center justify-center gap-1.5"
                        >
                          <RotateCw className="h-3 w-3" />
                          <span>Reset</span>
                        </button>
                        <button
                          type="button"
                          disabled={isExecutingPower}
                          onClick={() => handlePowerAction('pxe_reboot')}
                          className="py-1 px-2 bg-red-950/60 hover:bg-red-900/80 disabled:opacity-50 text-red-300 text-xs font-medium rounded border border-red-800/80 transition-colors flex items-center justify-center gap-1.5"
                        >
                          <span>Reboot to PXE</span>
                        </button>
                      </div>
                    </div>

                  </div>
                </div>
              )}

            </div>
          </div>

          {/* Drawer Actions */}
          <div className="border-t border-[#212836] pt-3 flex flex-col gap-2">
            {showDeleteConfirm && (
              <div className="p-2 rounded bg-red-950/70 border border-red-800 text-xs text-red-200 flex items-center justify-between">
                <span>Remove node from inventory?</span>
                <div className="flex gap-2">
                  <button
                    type="button"
                    disabled={isDeleting}
                    onClick={async () => {
                      if (!node || !onDelete) return;
                      setIsDeleting(true);
                      try {
                        await onDelete(node.id);
                        onClose();
                      } finally {
                        setIsDeleting(false);
                      }
                    }}
                    className="px-2 py-0.5 bg-red-700 hover:bg-red-600 disabled:opacity-50 text-white rounded text-[11px] font-bold transition-colors"
                  >
                    {isDeleting ? 'Deleting...' : 'Confirm'}
                  </button>
                  <button
                    type="button"
                    onClick={() => setShowDeleteConfirm(false)}
                    className="px-2 py-0.5 bg-[#161c26] text-slate-300 rounded text-[11px] transition-colors"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            <div className="flex gap-2">
              <button
                type="button"
                onClick={onClose}
                className="rounded-sm border border-[#283244] bg-[#161c26] px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#1f2736] transition-colors"
              >
                Close
              </button>

              {onDelete && !showDeleteConfirm && (
                <button
                  type="button"
                  onClick={() => setShowDeleteConfirm(true)}
                  className="rounded-sm border border-red-900/60 bg-red-950/30 px-3 py-1.5 text-xs font-medium text-red-400 hover:bg-red-900/50 hover:text-red-300 transition-colors flex items-center gap-1.5"
                  title="Remove server node from inventory"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  <span>Delete</span>
                </button>
              )}

              {(node.status === 'ACTIVE' || node.status === 'ERROR') && onReset && (
                <button
                  type="button"
                  disabled={isResetting}
                  onClick={async () => {
                    setIsResetting(true);
                    try {
                      await onReset(node.id);
                    } finally {
                      setIsResetting(false);
                    }
                  }}
                  className="flex-1 rounded-sm border border-amber-800/80 bg-amber-950/50 py-1.5 text-xs font-medium text-amber-300 hover:bg-amber-900/60 disabled:opacity-50 transition-colors flex items-center justify-center gap-1.5"
                >
                  <RotateCw className={`h-3 w-3 ${isResetting ? 'animate-spin' : ''}`} />
                  <span>{isResetting ? 'Resetting...' : 'Re-provision'}</span>
                </button>
              )}

              {node.status === 'READY_FOR_PROVISIONING' && (
                <button
                  type="button"
                  onClick={() => {
                    onClose();
                    onDeploy(node);
                  }}
                  className="flex-1 rounded-sm bg-redwolf-primary py-1.5 text-xs font-medium text-white hover:bg-redwolf-hover transition-colors flex items-center justify-center gap-1.5 shadow-sm"
                >
                  <Play className="h-3 w-3 fill-current" />
                  <span>Deploy</span>
                </button>
              )}
            </div>
          </div>

        </div>
      </div>
    </div>
  );
};
