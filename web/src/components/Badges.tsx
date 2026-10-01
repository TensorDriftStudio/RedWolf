import React from 'react';
import type { Vendor, NodeStatus } from '../types';
import { CheckCircle2, AlertTriangle, Loader2 } from 'lucide-react';

export const VendorBadge: React.FC<{ vendor: Vendor }> = ({ vendor }) => {
  switch (vendor) {
    case 'Dell Inc.':
      return (
        <span className="inline-flex items-center gap-1 rounded-md border border-sky-800/60 bg-sky-950/40 px-2 py-0.5 text-xs font-semibold text-sky-300">
          <span className="h-1.5 w-1.5 rounded-full bg-sky-400" />
          Dell PowerEdge
        </span>
      );
    case 'Supermicro':
      return (
        <span className="inline-flex items-center gap-1 rounded-md border border-emerald-800/60 bg-emerald-950/40 px-2 py-0.5 text-xs font-semibold text-emerald-300">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          Supermicro
        </span>
      );
    case 'ASRockRack':
      return (
        <span className="inline-flex items-center gap-1 rounded-md border border-purple-800/60 bg-purple-950/40 px-2 py-0.5 text-xs font-semibold text-purple-300">
          <span className="h-1.5 w-1.5 rounded-full bg-purple-400" />
          ASRock Rack
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1 rounded-md border border-slate-700 bg-slate-800/50 px-2 py-0.5 text-xs font-medium text-slate-300">
          {vendor}
        </span>
      );
  }
};

export const StatusBadge: React.FC<{ status: NodeStatus; progress?: number }> = ({ status, progress }) => {
  switch (status) {
    case 'READY_FOR_PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1 text-xs font-semibold text-emerald-300">
          <span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
          Ready to Deploy
        </span>
      );
    case 'DISCOVERING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-1 text-xs font-semibold text-amber-300">
          <Loader2 className="h-3 w-3 animate-spin text-amber-400" />
          Discovering &amp; BMC Config
        </span>
      );
    case 'PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-cyan-500/40 bg-cyan-500/10 px-2.5 py-1 text-xs font-semibold text-cyan-300">
          <Loader2 className="h-3 w-3 animate-spin text-cyan-400" />
          Streaming OS {progress ? `(${progress}%)` : ''}
        </span>
      );
    case 'ACTIVE':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-blue-500/40 bg-blue-500/10 px-2.5 py-1 text-xs font-semibold text-blue-300">
          <CheckCircle2 className="h-3 w-3 text-blue-400" />
          Active in Production
        </span>
      );
    case 'ERROR':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-rose-500/40 bg-rose-500/10 px-2.5 py-1 text-xs font-semibold text-rose-300">
          <AlertTriangle className="h-3 w-3 text-rose-400" />
          Provisioning Failed
        </span>
      );
  }
};
