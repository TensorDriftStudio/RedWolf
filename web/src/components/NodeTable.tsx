import React from 'react';
import type { ServerNode } from '../types';
import { VendorBadge, StatusBadge } from './Badges';
import { ExternalLink, HardDrive, Server, Play, ChevronRight, Loader2 } from 'lucide-react';

interface NodeTableProps {
  nodes: ServerNode[];
  isLoading?: boolean;
  onSelectNode: (node: ServerNode) => void;
  onDeployNode: (node: ServerNode) => void;
}

export const NodeTable: React.FC<NodeTableProps> = ({ nodes, isLoading = false, onSelectNode, onDeployNode }) => {
  if (isLoading && nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-sm border border-[#212836] p-12 text-center bg-[#151b24]">
        <Loader2 className="h-6 w-6 text-red-500 animate-spin mb-2" />
        <h3 className="text-xs font-medium text-slate-300">Discovering servers...</h3>
      </div>
    );
  }

  if (nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-sm border border-dashed border-[#263143] p-12 text-center bg-[#151b24]">
        <Server className="h-7 w-7 text-slate-500 mb-2" />
        <h3 className="text-xs font-medium text-slate-200">No servers discovered</h3>
        <p className="mt-1 text-[11px] text-slate-400">
          Waiting for PXE network boot broadcasts on the provisioning subnet.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-sm border border-[#212836] bg-[#151b24]">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs border-collapse">
          {/* High-Density AWX / PatternFly Table Header */}
          <thead className="border-b border-[#212836] bg-[#10141d] text-[11px] font-semibold text-slate-400">
            <tr>
              <th className="py-2.5 pl-4 pr-3">Server</th>
              <th className="px-3 py-2.5">Vendor</th>
              <th className="px-3 py-2.5">Status</th>
              <th className="px-3 py-2.5">Compute</th>
              <th className="px-3 py-2.5">Storage</th>
              <th className="px-3 py-2.5">Management</th>
              <th className="py-2.5 pl-3 pr-4 text-right">Actions</th>
            </tr>
          </thead>

          {/* Table Body */}
          <tbody className="divide-y divide-[#1e2430] font-normal text-slate-300">
            {nodes.map((node) => {
              const primaryDrive = node.storage?.[0];
              const totalDrives = node.storage?.length ?? 0;

              return (
                <tr 
                  key={node.id}
                  className="hover:bg-[#1a212d] transition-colors cursor-pointer"
                  onClick={() => onSelectNode(node)}
                >
                  {/* Model & Service Tag */}
                  <td className="py-2.5 pl-4 pr-3">
                    <div className="font-semibold text-white text-xs flex items-center gap-1.5">
                      {node.model}
                      <span className="text-[9px] font-mono rounded bg-[#10141d] border border-[#212836] px-1 text-slate-400">
                        {node.firmwareMode}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-[11px] text-slate-400 font-mono mt-0.5">
                      <span>SN: {node.serialNumber}</span>
                      <span className="text-slate-600">•</span>
                      <span>BIOS {node.biosVersion}</span>
                    </div>
                  </td>

                  {/* Vendor Badge */}
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <VendorBadge vendor={node.vendor} />
                  </td>

                  {/* Status Badge */}
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <StatusBadge status={node.status} progress={node.provisioningState?.progress} />
                  </td>

                  {/* CPU / RAM */}
                  <td className="px-3 py-2.5">
                    <div className="text-white text-xs">
                      {node.cpu.sockets > 1 ? `${node.cpu.sockets}x ` : ''}
                      {node.cpu.model.replace(/\(R\)|\(TM\)|CPU|Processor|@.*/gi, '').trim()}
                    </div>
                    <div className="text-[11px] text-slate-400 font-mono mt-0.5">
                      <strong className="text-slate-200">{node.memory.totalHuman}</strong> ({node.memory.slotsUsed}/{node.memory.slotsTotal} slots)
                    </div>
                  </td>

                  {/* Storage Target */}
                  <td className="px-3 py-2.5">
                    {primaryDrive ? (
                      <div>
                        <div className="flex items-center gap-1.5 text-xs text-slate-200 font-mono">
                          <HardDrive className="h-3 w-3 text-slate-500" />
                          <span>{primaryDrive.sizeHuman} {primaryDrive.type}</span>
                          {totalDrives > 1 && (
                            <span className="text-[10px] text-slate-500">(+{totalDrives - 1})</span>
                          )}
                        </div>
                        <div className="text-[10px] font-mono text-slate-400 truncate max-w-[160px]" title={primaryDrive.byId || primaryDrive.model}>
                          {primaryDrive.byId ? primaryDrive.byId.split('/').pop() : primaryDrive.model}
                        </div>
                      </div>
                    ) : (
                      <span className="text-slate-500 italic text-[11px]">No drives</span>
                    )}
                  </td>

                  {/* BMC Out-of-band */}
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    {node.bmc.ip ? (
                      <a
                        href={`https://${node.bmc.ip}`}
                        target="_blank"
                        rel="noreferrer"
                        onClick={(e) => e.stopPropagation()}
                        className="inline-flex items-center gap-1 font-mono text-xs text-sky-400 hover:underline"
                        title={`Open ${node.bmc.vendor} Console`}
                      >
                        {node.bmc.ip}
                        <ExternalLink className="h-2.5 w-2.5" />
                      </a>
                    ) : (
                      <span className="text-slate-500 font-mono text-xs">Resolving IP...</span>
                    )}
                    <div className="text-[10px] text-slate-500 font-mono mt-0.5">
                      {node.bmc.portMode}
                    </div>
                  </td>

                  {/* Quick Action Button */}
                  <td className="py-2.5 pl-3 pr-4 text-right whitespace-nowrap">
                    {node.status === 'READY_FOR_PROVISIONING' ? (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onDeployNode(node);
                        }}
                        className="inline-flex items-center gap-1.5 rounded-sm bg-redwolf-primary px-2.5 py-1 text-xs font-medium text-white hover:bg-redwolf-hover transition-colors shadow-sm"
                      >
                        <Play className="h-2.5 w-2.5 fill-current" />
                        Deploy
                      </button>
                    ) : (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelectNode(node);
                        }}
                        className="inline-flex items-center gap-1 rounded-sm border border-[#232b3b] bg-[#10141d] px-2 py-1 text-xs font-medium text-slate-300 hover:bg-[#1d2533] hover:text-white transition-colors"
                      >
                        Inspect
                        <ChevronRight className="h-3 w-3 text-slate-500" />
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
