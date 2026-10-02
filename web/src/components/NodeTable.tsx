import React from 'react';
import type { ServerNode } from '../types';
import { VendorBadge, StatusBadge } from './Badges';
import { ExternalLink, HardDrive, Cpu, Play, ChevronRight } from 'lucide-react';

interface NodeTableProps {
  nodes: ServerNode[];
  onSelectNode: (node: ServerNode) => void;
  onDeployNode: (node: ServerNode) => void;
}

export const NodeTable: React.FC<NodeTableProps> = ({ nodes, onSelectNode, onDeployNode }) => {
  if (nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-sm border border-dashed border-enterprise-border p-12 text-center bg-enterprise-panel">
        <Cpu className="h-8 w-8 text-enterprise-textDim mb-2" />
        <h3 className="text-xs font-semibold text-white">No managed server nodes found</h3>
        <p className="mt-1 text-[11px] text-enterprise-textMuted">
          Power on bare-metal servers with network boot enabled on the provisioning broadcast domain.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-sm border border-enterprise-border bg-enterprise-panel">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs border-collapse">
          {/* PatternFly / Rancher High-Density Table Header */}
          <thead className="border-b border-enterprise-border bg-[#13171f] text-[10px] font-semibold uppercase tracking-wider text-enterprise-textMuted">
            <tr>
              <th className="py-2.5 pl-4 pr-3">Server / Chassis</th>
              <th className="px-3 py-2.5">Manufacturer</th>
              <th className="px-3 py-2.5">Provisioning State</th>
              <th className="px-3 py-2.5">Compute (CPU &amp; RAM)</th>
              <th className="px-3 py-2.5">Primary Storage Target</th>
              <th className="px-3 py-2.5">Out-of-Band BMC</th>
              <th className="py-2.5 pl-3 pr-4 text-right">Actions</th>
            </tr>
          </thead>

          {/* Table Body */}
          <tbody className="divide-y divide-enterprise-borderSubtle font-normal text-slate-300">
            {nodes.map((node) => {
              const primaryDrive = node.storage[0];
              const totalDrives = node.storage.length;

              return (
                <tr 
                  key={node.id}
                  className="hover:bg-[#1e2430] transition-colors cursor-pointer group"
                  onClick={() => onSelectNode(node)}
                >
                  {/* Model & Service Tag */}
                  <td className="py-2.5 pl-4 pr-3">
                    <div className="font-semibold text-white text-xs flex items-center gap-1.5">
                      {node.model}
                      <span className="text-[9px] font-mono rounded-sm bg-[#13171f] border border-enterprise-border px-1 text-enterprise-textDim">
                        {node.firmwareMode}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-[11px] text-enterprise-textMuted font-mono mt-0.5">
                      <span>SN: {node.serialNumber}</span>
                      <span className="text-enterprise-border">•</span>
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
                    <div className="text-[11px] text-enterprise-textMuted font-mono mt-0.5">
                      <strong className="text-slate-200">{node.memory.totalHuman}</strong> ({node.memory.slotsUsed}/{node.memory.slotsTotal} DIMMs)
                    </div>
                  </td>

                  {/* Storage Target */}
                  <td className="px-3 py-2.5">
                    {primaryDrive ? (
                      <div>
                        <div className="flex items-center gap-1.5 text-xs text-slate-200 font-mono">
                          <HardDrive className="h-3 w-3 text-enterprise-textDim" />
                          <span>{primaryDrive.sizeHuman} {primaryDrive.type}</span>
                          {totalDrives > 1 && (
                            <span className="text-[10px] text-enterprise-textDim">(+{totalDrives - 1} more)</span>
                          )}
                        </div>
                        <div className="text-[10px] font-mono text-enterprise-textMuted truncate max-w-[150px]" title={primaryDrive.byId || primaryDrive.model}>
                          {primaryDrive.byId ? primaryDrive.byId.split('/').pop() : primaryDrive.model}
                        </div>
                      </div>
                    ) : (
                      <span className="text-enterprise-textDim italic text-[11px]">No drives detected</span>
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
                        className="inline-flex items-center gap-1 font-mono text-xs text-[#58a6ff] hover:underline"
                        title={`Open ${node.bmc.vendor} Management Console`}
                      >
                        {node.bmc.ip}
                        <ExternalLink className="h-2.5 w-2.5" />
                      </a>
                    ) : (
                      <span className="text-enterprise-textDim font-mono text-xs">DHCP resolving...</span>
                    )}
                    <div className="text-[10px] text-enterprise-textMuted font-mono mt-0.5">
                      {node.bmc.portMode} Port
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
                        className="inline-flex items-center gap-1 rounded-sm bg-redwolf-primary px-2.5 py-1 text-xs font-medium text-white hover:bg-redwolf-hover transition-colors"
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
                        className="inline-flex items-center gap-1 rounded-sm border border-enterprise-border bg-[#13171f] px-2 py-1 text-xs font-medium text-slate-300 hover:bg-enterprise-hover hover:text-white transition-colors"
                      >
                        Inspect
                        <ChevronRight className="h-3 w-3 text-enterprise-textDim" />
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
