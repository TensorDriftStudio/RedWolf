import React from 'react';
import { Server, CheckCircle2, Loader2, HardDrive } from 'lucide-react';
import type { ServerNode } from '../types';

interface StatsBarProps {
  nodes: ServerNode[];
  isLoading?: boolean;
}

export const StatsBar: React.FC<StatsBarProps> = ({ nodes, isLoading = false }) => {
  const isInitialLoading = isLoading && nodes.length === 0;
  const total = nodes.length;
  const ready = nodes.filter(n => n.status === 'READY_FOR_PROVISIONING').length;
  const provisioning = nodes.filter(n => n.status === 'PROVISIONING' || n.status === 'DISCOVERING').length;
  const active = nodes.filter(n => n.status === 'ACTIVE').length;

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {/* Total Servers */}
      <div className="border border-[#212836] bg-[#151b24] p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-slate-400">Total Servers</div>
          <div className="text-lg font-bold font-mono text-white mt-0.5">
            {isInitialLoading ? <span className="text-slate-600 animate-pulse">—</span> : total}
          </div>
        </div>
        <Server className="h-4 w-4 text-slate-500" />
      </div>

      {/* Ready for Provisioning */}
      <div className="border border-[#212836] bg-[#151b24] p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-slate-400">Ready to Deploy</div>
          <div className="text-lg font-bold font-mono text-blue-400 mt-0.5">
            {isInitialLoading ? <span className="text-slate-600 animate-pulse">—</span> : ready}
          </div>
        </div>
        <CheckCircle2 className="h-4 w-4 text-blue-400" />
      </div>

      {/* In-Flight Provisioning */}
      <div className="border border-[#212836] bg-[#151b24] p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-slate-400">Deploying</div>
          <div className="text-lg font-bold font-mono text-amber-400 mt-0.5">
            {isInitialLoading ? <span className="text-slate-600 animate-pulse">—</span> : provisioning}
          </div>
        </div>
        <Loader2 className={`h-4 w-4 text-amber-400 ${provisioning > 0 ? 'animate-spin' : ''}`} />
      </div>

      {/* Active in Production */}
      <div className="border border-[#212836] bg-[#151b24] p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-slate-400">Active</div>
          <div className="text-lg font-bold font-mono text-emerald-400 mt-0.5">
            {isInitialLoading ? <span className="text-slate-600 animate-pulse">—</span> : active}
          </div>
        </div>
        <HardDrive className="h-4 w-4 text-emerald-400" />
      </div>
    </div>
  );
};
