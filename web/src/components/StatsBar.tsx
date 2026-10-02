import React from 'react';
import { Server, CheckCircle2, Loader2, HardDrive } from 'lucide-react';
import type { ServerNode } from '../types';

interface StatsBarProps {
  nodes: ServerNode[];
}

export const StatsBar: React.FC<StatsBarProps> = ({ nodes }) => {
  const total = nodes.length;
  const ready = nodes.filter(n => n.status === 'READY_FOR_PROVISIONING').length;
  const provisioning = nodes.filter(n => n.status === 'PROVISIONING' || n.status === 'DISCOVERING').length;
  const active = nodes.filter(n => n.status === 'ACTIVE').length;

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {/* Total Servers */}
      <div className="border-l-2 border-l-slate-400 border border-enterprise-border bg-enterprise-panel p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-enterprise-textMuted uppercase tracking-wider">Total Servers</div>
          <div className="text-xl font-bold font-mono text-white mt-0.5">{total}</div>
        </div>
        <Server className="h-5 w-5 text-enterprise-textDim" />
      </div>

      {/* Ready for Provisioning */}
      <div className="border-l-2 border-l-[#1f6feb] border border-enterprise-border bg-enterprise-panel p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-[#79c0ff] uppercase tracking-wider">Ready to Deploy</div>
          <div className="text-xl font-bold font-mono text-white mt-0.5">{ready}</div>
        </div>
        <CheckCircle2 className="h-5 w-5 text-[#1f6feb]" />
      </div>

      {/* In-Flight Provisioning */}
      <div className="border-l-2 border-l-[#d29922] border border-enterprise-border bg-enterprise-panel p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-[#e3b341] uppercase tracking-wider">Deploying / Discovering</div>
          <div className="text-xl font-bold font-mono text-white mt-0.5">{provisioning}</div>
        </div>
        <Loader2 className={`h-5 w-5 text-[#d29922] ${provisioning > 0 ? 'animate-spin' : ''}`} />
      </div>

      {/* Active in Production */}
      <div className="border-l-2 border-l-[#238636] border border-enterprise-border bg-enterprise-panel p-3 rounded-sm flex items-center justify-between">
        <div>
          <div className="text-[11px] font-medium text-[#7ee787] uppercase tracking-wider">Active in Production</div>
          <div className="text-xl font-bold font-mono text-white mt-0.5">{active}</div>
        </div>
        <HardDrive className="h-5 w-5 text-[#238636]" />
      </div>
    </div>
  );
};
