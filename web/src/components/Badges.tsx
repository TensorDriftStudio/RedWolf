import React from 'react';
import type { Vendor, NodeStatus } from '../types';
import { CheckCircle2, AlertTriangle, Loader2 } from 'lucide-react';

export const VendorBadge: React.FC<{ vendor: Vendor }> = ({ vendor }) => {
  switch (vendor) {
    case 'Dell Inc.':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-[#132337] border border-[#1e3a5f] text-sky-300">
          <span className="h-1.5 w-1.5 rounded-full bg-sky-400" />
          Dell PowerEdge
        </span>
      );
    case 'Supermicro':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-[#112419] border border-[#1b432b] text-emerald-300">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          Supermicro
        </span>
      );
    case 'ASRockRack':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-[#241a35] border border-[#3d295c] text-purple-300">
          <span className="h-1.5 w-1.5 rounded-full bg-purple-400" />
          ASRock Rack
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-[#181f2a] border border-[#273244] text-slate-300">
          <span className="h-1.5 w-1.5 rounded-full bg-slate-400" />
          {vendor}
        </span>
      );
  }
};

export const StatusBadge: React.FC<{ status: NodeStatus; progress?: number }> = ({ status, progress }) => {
  switch (status) {
    case 'READY_FOR_PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-blue-950/60 border border-blue-800/80 text-blue-300">
          <span className="h-1.5 w-1.5 rounded-full bg-blue-400" />
          Ready to Deploy
        </span>
      );
    case 'DISCOVERING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-[#161c26] border border-[#263143] text-slate-400">
          <Loader2 className="h-3 w-3 animate-spin text-slate-400" />
          Discovering
        </span>
      );
    case 'PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-amber-950/60 border border-amber-800/80 text-amber-300">
          <Loader2 className="h-3 w-3 animate-spin text-amber-400" />
          Provisioning {progress ? `(${progress}%)` : ''}
        </span>
      );
    case 'ACTIVE':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-emerald-950/60 border border-emerald-800/80 text-emerald-300">
          <CheckCircle2 className="h-3 w-3 text-emerald-400" />
          Active
        </span>
      );
    case 'ERROR':
      return (
        <span className="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-medium bg-rose-950/60 border border-rose-800/80 text-rose-300">
          <AlertTriangle className="h-3 w-3 text-rose-400" />
          Error
        </span>
      );
  }
};
