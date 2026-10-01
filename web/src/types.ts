export type Vendor = 'Dell Inc.' | 'Supermicro' | 'ASRockRack';

export type NodeStatus = 
  | 'DISCOVERING'
  | 'READY_FOR_PROVISIONING'
  | 'PROVISIONING'
  | 'ACTIVE'
  | 'ERROR';

export interface StorageDevice {
  name: string;
  path: string;
  byId: string;
  sizeBytes: number;
  sizeHuman: string;
  type: 'NVMe' | 'SSD' | 'SATA' | 'BOSS' | 'SATADOM';
  transport: 'nvme' | 'sata' | 'sas' | 'pcie';
  model: string;
  serial: string;
}

export interface NetworkInterface {
  name: string;
  mac: string;
  speedMbps: number;
  carrier: boolean;
  isBoot: boolean;
  driver: string;
  pciSlot: string;
}

export interface BMCInfo {
  vendor: string;
  ip: string;
  mac: string;
  dhcp: boolean;
  channel: number;
  portMode: 'Dedicated' | 'Shared' | 'Failover';
  credentialsUpdated: boolean;
}

export interface ServerNode {
  id: string;
  vendor: Vendor;
  model: string;
  serialNumber: string;
  firmwareMode: 'UEFI' | 'BIOS';
  biosVersion: string;
  status: NodeStatus;
  cpu: {
    model: string;
    sockets: number;
    coresPerSocket: number;
    threadsPerSocket: number;
    totalThreads: number;
    arch: string;
  };
  memory: {
    totalBytes: number;
    totalHuman: string;
    slotsUsed: number;
    slotsTotal: number;
    type: string;
    speedMhz: number;
  };
  storage: StorageDevice[];
  nics: NetworkInterface[];
  bmc: BMCInfo;
  provisioningState?: {
    progress: number;
    stage: string;
    os: string;
    targetDrive: string;
    targetIp: string;
    logs: string[];
  };
  discoveredAt: string;
}

export interface DeploymentConfig {
  nodeId: string;
  os: 'AlmaLinux 8' | 'AlmaLinux 9' | 'AlmaLinux 10' | 'Debian 12' | 'Debian 13';
  targetDrivePath: string;
  partitioningPreset: 'standard' | 'lvm' | 'raid1';
  rootPassword: string;
  sshKeys: string[];
  networkMode: 'static' | 'dhcp';
  staticIp?: string;
  netmaskCidr?: number;
  gateway?: string;
  dnsServers?: string[];
  vlanTag?: number;
  enableBonding: boolean;
  bondInterfaces?: string[];
}
