import React, { useState } from 'react';
import type { ServerNode } from '../types';
import { VendorBadge, StatusBadge } from './Badges';
import { X, Cpu, HardDrive, Network, Shield, ExternalLink, Play } from 'lucide-react';

interface NodeDrawerProps {
  node: ServerNode | null;
  onClose: () => void;
  onDeploy: (node: ServerNode) => void;
}

export const NodeDrawer: React.FC<NodeDrawerProps> = ({ node, onClose, onDeploy }) => {
  const [activeTab, setActiveTab] = useState<'compute' | 'storage' | 'network' | 'bmc'>('compute');

  if (!node) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-black/60 backdrop-blur-sm animate-fade-in">
      <div className="absolute inset-y-0 right-0 flex max-w-full pl-10">
        <div className="w-screen max-w-xl border-l border-surface-border bg-surface-card p-6 shadow-2xl flex flex-col justify-between">
          
          {/* Header */}
          <div>
            <div className="flex items-start justify-between">
              <div>
                <div className="flex items-center gap-2 mb-1.5">
                  <VendorBadge vendor={node.vendor} />
                  <span className="text-xs font-mono text-slate-400">SN: {node.serialNumber}</span>
                </div>
                <h2 className="text-lg font-bold text-white">{node.model}</h2>
                <div className="mt-1 flex items-center gap-2">
                  <StatusBadge status={node.status} progress={node.provisioningState?.progress} />
                  <span className="text-xs text-slate-500 font-mono">BIOS {node.biosVersion} ({node.firmwareMode})</span>
                </div>
              </div>
              <button
                onClick={onClose}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-surface-panel hover:text-white transition-all"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Navigation Tabs */}
            <div className="mt-6 flex border-b border-surface-border text-xs">
              <button
                onClick={() => setActiveTab('compute')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition-all ${
                  activeTab === 'compute'
                    ? 'border-redwolf-primary text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                <Cpu className="h-4 w-4" />
                Compute
              </button>
              <button
                onClick={() => setActiveTab('storage')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition-all ${
                  activeTab === 'storage'
                    ? 'border-redwolf-primary text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                <HardDrive className="h-4 w-4" />
                Storage ({node.storage.length})
              </button>
              <button
                onClick={() => setActiveTab('network')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition-all ${
                  activeTab === 'network'
                    ? 'border-redwolf-primary text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                <Network className="h-4 w-4" />
                NICs ({node.nics.length})
              </button>
              <button
                onClick={() => setActiveTab('bmc')}
                className={`flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition-all ${
                  activeTab === 'bmc'
                    ? 'border-redwolf-primary text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                <Shield className="h-4 w-4" />
                BMC / OOB
              </button>
            </div>

            {/* Tab Contents */}
            <div className="mt-4 space-y-4 text-xs">
              
              {/* Tab: Compute */}
              {activeTab === 'compute' && (
                <div className="space-y-4">
                  <div className="rounded-lg border border-surface-border bg-surface-panel/60 p-4">
                    <div className="text-xs font-semibold text-slate-300 mb-2">Processor Details</div>
                    <div className="text-white font-medium">{node.cpu.model}</div>
                    <div className="grid grid-cols-3 gap-2 mt-3 pt-3 border-t border-surface-border/60 text-slate-400">
                      <div>Sockets: <strong className="text-white">{node.cpu.sockets}</strong></div>
                      <div>Cores/Socket: <strong className="text-white">{node.cpu.coresPerSocket}</strong></div>
                      <div>Total Threads: <strong className="text-white">{node.cpu.totalThreads}</strong></div>
                    </div>
                  </div>

                  <div className="rounded-lg border border-surface-border bg-surface-panel/60 p-4">
                    <div className="text-xs font-semibold text-slate-300 mb-2">System Memory</div>
                    <div className="text-lg font-bold text-white">{node.memory.totalHuman}</div>
                    <div className="grid grid-cols-2 gap-2 mt-3 pt-3 border-t border-surface-border/60 text-slate-400">
                      <div>Slot Population: <strong className="text-white">{node.memory.slotsUsed} of {node.memory.slotsTotal}</strong></div>
                      <div>Type &amp; Speed: <strong className="text-white">{node.memory.type} @ {node.memory.speedMhz} MHz</strong></div>
                    </div>
                  </div>
                </div>
              )}

              {/* Tab: Storage */}
              {activeTab === 'storage' && (
                <div className="space-y-3 max-h-[380px] overflow-y-auto pr-1">
                  {node.storage.map((disk, idx) => (
                    <div key={idx} className="rounded-lg border border-surface-border bg-surface-panel/60 p-3.5">
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs font-bold text-white">{disk.path}</span>
                          <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] font-semibold text-sky-400 border border-slate-700">
                            {disk.type}
                          </span>
                        </div>
                        <span className="font-bold text-emerald-400 text-xs">{disk.sizeHuman}</span>
                      </div>
                      <div className="text-slate-300 font-medium">{disk.model}</div>
                      <div className="mt-2 text-[11px] font-mono text-slate-400 break-all space-y-0.5">
                        <div>Serial: <span className="text-slate-200">{disk.serial}</span></div>
                        <div>by-id: <span className="text-slate-500">{disk.byId}</span></div>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {/* Tab: Network */}
              {activeTab === 'network' && (
                <div className="space-y-3 max-h-[380px] overflow-y-auto pr-1">
                  {node.nics.map((nic, idx) => (
                    <div key={idx} className="rounded-lg border border-surface-border bg-surface-panel/60 p-3.5">
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-2">
                          <span className="font-mono font-bold text-white text-xs">{nic.name}</span>
                          {nic.isBoot && (
                            <span className="rounded bg-redwolf-primary/20 px-1.5 py-0.5 text-[10px] font-semibold text-red-300 border border-redwolf-primary/40">
                              PXE Boot NIC
                            </span>
                          )}
                        </div>
                        <span className={`text-[11px] font-semibold flex items-center gap-1 ${nic.carrier ? 'text-emerald-400' : 'text-slate-500'}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${nic.carrier ? 'bg-emerald-400' : 'bg-slate-600'}`} />
                          {nic.carrier ? `${nic.speedMbps / 1000} GbE Link Up` : 'No Link'}
                        </span>
                      </div>
                      <div className="mt-2 grid grid-cols-2 gap-2 text-[11px] font-mono text-slate-400">
                        <div>MAC: <strong className="text-slate-200">{nic.mac}</strong></div>
                        <div>Driver: <span className="text-slate-300">{nic.driver}</span></div>
                        <div className="col-span-2 text-slate-500">Slot: {nic.pciSlot}</div>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {/* Tab: BMC */}
              {activeTab === 'bmc' && (
                <div className="space-y-4">
                  <div className="rounded-lg border border-surface-border bg-surface-panel/60 p-4">
                    <div className="text-xs font-semibold text-slate-300 mb-2">Management Controller</div>
                    <div className="text-white font-medium">{node.bmc.vendor}</div>
                    
                    <div className="mt-4 space-y-2 font-mono text-xs">
                      <div className="flex justify-between border-b border-surface-border/50 pb-2">
                        <span className="text-slate-400">BMC IP Address:</span>
                        <a 
                          href={`https://${node.bmc.ip}`} 
                          target="_blank" 
                          rel="noreferrer"
                          className="text-sky-400 hover:underline flex items-center gap-1"
                        >
                          {node.bmc.ip} <ExternalLink className="h-3 w-3" />
                        </a>
                      </div>
                      <div className="flex justify-between border-b border-surface-border/50 pb-2">
                        <span className="text-slate-400">BMC MAC Address:</span>
                        <span className="text-slate-200">{node.bmc.mac}</span>
                      </div>
                      <div className="flex justify-between border-b border-surface-border/50 pb-2">
                        <span className="text-slate-400">Port Mode:</span>
                        <span className="text-slate-200">{node.bmc.portMode} Port</span>
                      </div>
                      <div className="flex justify-between pt-1">
                        <span className="text-slate-400">Credentials Hardened:</span>
                        <span className="text-emerald-400 font-semibold flex items-center gap-1">
                          Configured by RedWolf
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              )}

            </div>
          </div>

          {/* Footer Action */}
          <div className="pt-6 border-t border-surface-border flex items-center justify-between">
            <span className="text-xs text-slate-500 font-mono">Discovered: {new Date(node.discoveredAt).toLocaleTimeString()}</span>
            {node.status === 'READY_FOR_PROVISIONING' ? (
              <button
                onClick={() => {
                  onClose();
                  onDeploy(node);
                }}
                className="flex items-center gap-2 rounded-lg bg-redwolf-primary px-4 py-2 text-xs font-semibold text-white shadow-glow-red hover:bg-redwolf-hover transition-all"
              >
                <Play className="h-3.5 w-3.5 fill-current" />
                Launch Provisioning Wizard
              </button>
            ) : (
              <button
                onClick={onClose}
                className="rounded-lg border border-surface-border px-3.5 py-1.5 text-xs font-medium text-slate-300 hover:text-white"
              >
                Close
              </button>
            )}
          </div>

        </div>
      </div>
    </div>
  );
};
