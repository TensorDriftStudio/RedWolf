import React from 'react';
import type { Vendor, NodeStatus } from '../types';
import { CheckCircle2, AlertTriangle, Loader2 } from 'lucide-react';

export const VendorBadge: React.FC<{ vendor: Vendor }> = ({ vendor }) => {
  switch (vendor) {
    case 'Dell Inc.':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-[#1f3a52] bg-[#122433] px-2 py-0.5 text-[11px] font-mono font-medium text-[#79c0ff]">
          <span className="h-1.5 w-1.5 rounded-full bg-[#388bfd]" />
          Dell PowerEdge
        </span>
      );
    case 'Supermicro':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-[#1b3d2f] bg-[#11261d] px-2 py-0.5 text-[11px] font-mono font-medium text-[#7ee787]">
          <span className="h-1.5 w-1.5 rounded-full bg-[#3fb950]" />
          Supermicro
        </span>
      );
    case 'ASRockRack':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-[#3b2b52] bg-[#221833] px-2 py-0.5 text-[11px] font-mono font-medium text-[#d2a8ff]">
          <span className="h-1.5 w-1.5 rounded-full bg-[#a371f7]" />
          ASRock Rack
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-[#2a3344] bg-[#1d2330] px-2 py-0.5 text-[11px] font-mono font-medium text-[#8b97a8]">
          <span className="h-1.5 w-1.5 rounded-full bg-[#637083]" />
          {vendor}
        </span>
      );
  }
};

export const StatusBadge: React.FC<{ status: NodeStatus; progress?: number }> = ({ status, progress }) => {
  switch (status) {
    case 'READY_FOR_PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-status-readyBorder bg-status-readyBg px-2.5 py-0.5 text-[11px] font-medium text-status-readyText">
          <span className="h-1.5 w-1.5 rounded-full bg-[#1f6feb]" />
          Ready to Deploy
        </span>
      );
    case 'DISCOVERING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-enterprise-border bg-enterprise-panel px-2.5 py-0.5 text-[11px] font-medium text-enterprise-textMuted">
          <Loader2 className="h-3 w-3 animate-spin text-enterprise-textMuted" />
          Discovering
        </span>
      );
    case 'PROVISIONING':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-status-warnBorder bg-status-warnBg px-2.5 py-0.5 text-[11px] font-medium text-status-warnText">
          <Loader2 className="h-3 w-3 animate-spin text-[#d29922]" />
          Provisioning {progress ? `(${progress}%)` : ''}
        </span>
      );
    case 'ACTIVE':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-status-activeBorder bg-status-activeBg px-2.5 py-0.5 text-[11px] font-medium text-status-activeText">
          <CheckCircle2 className="h-3 w-3 text-[#3fb950]" />
          Active
        </span>
      );
    case 'ERROR':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-sm border border-status-errorBorder bg-status-errorBg px-2.5 py-0.5 text-[11px] font-medium text-status-errorText">
          <AlertTriangle className="h-3 w-3 text-[#f85149]" />
          Error
        </span>
      );
  }
};
