import React from 'react';
import { Server, CheckCircle2, Loader2, Cpu } from 'lucide-react';
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
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:gap-4">
      {/* Total Nodes */}
      <div className="flex items-center gap-3.5 rounded-xl border border-surface-border bg-surface-card p-4 shadow-card">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-800/80 text-slate-300 border border-slate-700/50">
          <Server className="h-5 w-5" />
        </div>
        <div>
          <div className="text-xs font-medium text-slate-400">Total Nodes</div>
          <div className="text-xl font-bold tracking-tight text-white">{total}</div>
        </div>
      </div>

      {/* Ready for Provisioning */}
      <div className="flex items-center gap-3.5 rounded-xl border border-emerald-950/60 bg-emerald-950/20 p-4 shadow-card relative overflow-hidden">
        <div className="absolute -right-4 -top-4 h-16 w-16 rounded-full bg-emerald-500/10 blur-xl pointer-events-none" />
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-emerald-900/40 text-emerald-400 border border-emerald-700/50">
          <CheckCircle2 className="h-5 w-5" />
        </div>
        <div>
          <div className="text-xs font-medium text-emerald-400/80 flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
            Ready to Deploy
          </div>
          <div className="text-xl font-bold tracking-tight text-emerald-300">{ready}</div>
        </div>
      </div>

      {/* In-Flight Provisioning */}
      <div className="flex items-center gap-3.5 rounded-xl border border-amber-950/60 bg-amber-950/20 p-4 shadow-card">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-amber-900/40 text-amber-400 border border-amber-700/50">
          <Loader2 className={`h-5 w-5 ${provisioning > 0 ? 'animate-spin' : ''}`} />
        </div>
        <div>
          <div className="text-xs font-medium text-amber-400/80">In Progress / Discovering</div>
          <div className="text-xl font-bold tracking-tight text-amber-300">{provisioning}</div>
        </div>
      </div>

      {/* Active in Production */}
      <div className="flex items-center gap-3.5 rounded-xl border border-surface-border bg-surface-card p-4 shadow-card">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-redwolf-primary/20 text-redwolf-500 border border-redwolf-primary/40">
          <Cpu className="h-5 w-5 text-red-400" />
        </div>
        <div>
          <div className="text-xs font-medium text-slate-400">Active Production</div>
          <div className="text-xl font-bold tracking-tight text-white">{active}</div>
        </div>
      </div>
    </div>
  );
};
