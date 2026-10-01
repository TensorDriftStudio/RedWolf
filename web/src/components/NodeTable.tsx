import React from 'react';
import type { ServerNode } from '../types';
import { VendorBadge, StatusBadge } from './Badges';
import { ExternalLink, HardDrive, Cpu, Play, Eye } from 'lucide-react';

interface NodeTableProps {
  nodes: ServerNode[];
  onSelectNode: (node: ServerNode) => void;
  onDeployNode: (node: ServerNode) => void;
}

export const NodeTable: React.FC<NodeTableProps> = ({ nodes, onSelectNode, onDeployNode }) => {
  if (nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-surface-border p-12 text-center">
        <Cpu className="h-10 w-10 text-slate-500 mb-3" />
        <h3 className="text-sm font-semibold text-white">No nodes match your filters</h3>
        <p className="mt-1 text-xs text-slate-400">Boot servers via PXE on the provisioning network to trigger auto-discovery.</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-xl border border-surface-border bg-surface-card shadow-card">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs">
          {/* Table Header */}
          <thead className="border-b border-surface-border bg-surface-panel/80 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
            <tr>
              <th className="py-3.5 pl-4 pr-3 sm:pl-6">Hardware / Platform</th>
              <th className="px-3 py-3.5">Vendor</th>
              <th className="px-3 py-3.5">Status</th>
              <th className="px-3 py-3.5">Compute (CPU / RAM)</th>
              <th className="px-3 py-3.5">Target Storage</th>
              <th className="px-3 py-3.5">BMC / iDRAC IP</th>
              <th className="py-3.5 pl-3 pr-4 text-right sm:pr-6">Action</th>
            </tr>
          </thead>

          {/* Table Body */}
          <tbody className="divide-y divide-surface-border/60 font-medium text-slate-300">
            {nodes.map((node) => {
              const primaryDrive = node.storage[0];
              const totalDrives = node.storage.length;

              return (
                <tr 
                  key={node.id}
                  className="transition-colors hover:bg-surface-panel/60 cursor-pointer"
                  onClick={() => onSelectNode(node)}
                >
                  {/* Platform / Serial */}
                  <td className="py-3.5 pl-4 pr-3 sm:pl-6">
                    <div className="flex items-center gap-2.5">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-slate-800/80 text-slate-300 border border-slate-700/50">
                        <Cpu className="h-4 w-4 text-redwolf-primary" />
                      </div>
                      <div>
                        <div className="font-semibold text-white text-xs">{node.model}</div>
                        <div className="flex items-center gap-2 text-[11px] text-slate-400 font-mono mt-0.5">
                          <span>SN: {node.serialNumber}</span>
                          <span className="text-slate-600">•</span>
                          <span className="text-[10px] text-slate-400 rounded bg-slate-800/80 px-1 py-0.2 border border-slate-700/40">
                            {node.firmwareMode}
                          </span>
                        </div>
                      </div>
                    </div>
                  </td>

                  {/* Vendor Badge */}
                  <td className="px-3 py-3.5 whitespace-nowrap">
                    <VendorBadge vendor={node.vendor} />
                  </td>

                  {/* Status Badge */}
                  <td className="px-3 py-3.5 whitespace-nowrap">
                    <StatusBadge status={node.status} progress={node.provisioningState?.progress} />
                  </td>

                  {/* CPU / RAM */}
                  <td className="px-3 py-3.5">
                    <div className="text-white text-xs">
                      {node.cpu.sockets > 1 ? `${node.cpu.sockets}x ` : ''}
                      {node.cpu.model.replace(/\(R\)|\(TM\)|CPU|Processor|@.*/gi, '').trim()}
                    </div>
                    <div className="text-[11px] text-slate-400 mt-0.5">
                      <strong className="text-slate-200">{node.memory.totalHuman}</strong> RAM ({node.memory.slotsUsed}/{node.memory.slotsTotal} slots)
                    </div>
                  </td>

                  {/* Storage */}
                  <td className="px-3 py-3.5">
                    {primaryDrive ? (
                      <div>
                        <div className="flex items-center gap-1.5 text-xs text-slate-200">
                          <HardDrive className="h-3.5 w-3.5 text-slate-400" />
                          <span>{primaryDrive.sizeHuman} {primaryDrive.type}</span>
                          {totalDrives > 1 && (
                            <span className="text-[10px] text-slate-400 font-mono">+{totalDrives - 1} more</span>
                          )}
                        </div>
                        <div className="text-[11px] font-mono text-slate-400 mt-0.5 truncate max-w-[140px]" title={primaryDrive.model}>
                          {primaryDrive.model}
                        </div>
                      </div>
                    ) : (
                      <span className="text-slate-500 italic">No storage detected</span>
                    )}
                  </td>

                  {/* BMC IP */}
                  <td className="px-3 py-3.5 whitespace-nowrap">
                    {node.bmc.ip ? (
                      <a
                        href={`https://${node.bmc.ip}`}
                        target="_blank"
                        rel="noreferrer"
                        onClick={(e) => e.stopPropagation()}
                        className="inline-flex items-center gap-1 font-mono text-xs text-sky-400 hover:text-sky-300 hover:underline"
                        title={`Open ${node.bmc.vendor} Web Interface`}
                      >
                        {node.bmc.ip}
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    ) : (
                      <span className="text-slate-500 font-mono">DHCP acquiring...</span>
                    )}
                    <div className="text-[10px] text-slate-400 font-mono mt-0.5">
                      {node.bmc.portMode} Port
                    </div>
                  </td>

                  {/* Actions */}
                  <td className="py-3.5 pl-3 pr-4 text-right sm:pr-6 whitespace-nowrap">
                    {node.status === 'READY_FOR_PROVISIONING' ? (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onDeployNode(node);
                        }}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-redwolf-primary px-3 py-1.5 text-xs font-semibold text-white shadow-glow-red hover:bg-redwolf-hover transition-all"
                      >
                        <Play className="h-3 w-3 fill-current" />
                        Deploy Server
                      </button>
                    ) : (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelectNode(node);
                        }}
                        className="inline-flex items-center gap-1 rounded-lg border border-surface-border bg-surface-panel px-2.5 py-1.5 text-xs font-medium text-slate-300 hover:border-slate-600 hover:text-white transition-all"
                      >
                        <Eye className="h-3 w-3" />
                        Details
                      </button>
                    )}
                  </td>

                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
};
