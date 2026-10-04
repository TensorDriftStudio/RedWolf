export type Vendor = 'Dell Inc.' | 'Supermicro' | 'ASRockRack' | 'Generic';

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
  type: string;
  transport: 'nvme' | 'sata' | 'sas' | 'pcie' | string;
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

export type OperatingSystem = 'AlmaLinux 8' | 'AlmaLinux 9' | 'AlmaLinux 10' | 'Debian 12' | 'Debian 13';

export type RAIDLevel = 'none' | 'raid0' | 'raid1' | 'raid10';

export interface LVMVolume {
  name: string;
  mountPoint: string;
  sizeGb: number;
  fsType: 'xfs' | 'ext4';
}

export interface StorageConfig {
  layoutMode: 'standard' | 'lvm' | 'raid1';
  raidLevel?: RAIDLevel;
  targetDrives?: string[];
  lvmVolumes?: LVMVolume[];
  swapSizeGb?: number;
}

export interface DeploymentConfig {
  nodeId: string;
  os: OperatingSystem;
  targetDrivePath: string;
  partitioningPreset: 'standard' | 'lvm' | 'raid1';
  storage?: StorageConfig;
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
  templateId?: string;
  customUserData?: string;
}

export interface CloudInitTemplate {
  id: string;
  name: string;
  description: string;
  distro: string;
  userData: string;
  isDefault: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface OSImageDownloadStatus {
  os: OperatingSystem;
  filename: string;
  totalBytes: number;
  copiedBytes: number;
  progress: number;
  status: 'downloading' | 'converting' | 'completed' | 'error';
  error?: string;
}

export interface OSImageInfo {
  filename: string;
  os: OperatingSystem;
  displayName: string;
  sizeBytes: number;
  present: boolean;
  downloadUrl?: string;
  downloadStatus?: OSImageDownloadStatus;
}

export type PowerState = 'POWERED_ON' | 'POWERED_OFF' | 'UNKNOWN';

export interface PowerStatusResponse {
  nodeId: string;
  powerState: PowerState;
  bmcIp: string;
}

export type AuthSource = 'LOCAL' | 'DIRECTORY' | 'LDAP' | 'ACTIVE_DIRECTORY';
export type UserRole = 'ADMIN' | 'OPERATOR' | 'VIEWER';
export type DirectoryType = 'active_directory' | 'ldap';

export interface User {
  id: string;
  username: string;
  displayName: string;
  email: string;
  role: UserRole;
  source: AuthSource;
  createdAt: string;
  lastLoginAt: string;
}

export interface CreateUserPayload {
  username: string;
  displayName: string;
  email: string;
  password: string;
  role: UserRole;
}

export interface UpdateUserPayload {
  displayName: string;
  email: string;
  role: UserRole;
}

export interface ChangePasswordPayload {
  newPassword: string;
}

export interface LoginResponse {
  token: string;
  user: User;
  expiresAt: string;
}


export interface LDAPConfig {
  enabled: boolean;
  host: string;
  port: number;
  useTls: boolean;
  startTls: boolean;
  insecureSkipVerify: boolean;
  bindDn: string;
  bindPassword?: string;
  baseDn: string;
  userFilter: string;
  groupSearchDn: string;
  adminGroupDn: string;
  operatorGroupDn: string;
}

export interface ActiveDirectoryConfig {
  enabled: boolean;
  domain: string;
  domainController: string;
  port: number;
  useLdaps: boolean;
  insecureSkipVerify: boolean;
  bindDn: string;
  bindPassword?: string;
  baseDn: string;
  userSearchFilter: string;
  adminGroup: string;
  operatorGroup: string;
}

export interface SystemSettings {
  general: {
    applianceName: string;
    serverUrl: string;
    provisioningInterface: string;
    defaultOs: OperatingSystem;
  };
  network: {
    subnetCidr: string;
    dhcpRangeStart: string;
    dhcpRangeEnd: string;
    gateway: string;
    dnsServers: string[];
    leaseDurationMinutes: number;
  };
  auth: {
    localAuthEnabled: boolean;
    ldap: LDAPConfig;
    activeDirectory: ActiveDirectoryConfig;
  };
  storage: {
    imageStorageDir: string;
    maxCacheSizeGb: number;
  };
  updatedAt: string;
}

export interface DirectoryTestResult {
  success: boolean;
  latencyMs: number;
  message: string;
  entriesFound: number;
  testedAt: string;
}

export interface VersionInfo {
  version: string;
  gitCommit: string;
  buildDate: string;
  edition: string;
  goVersion: string;
  platform: string;
}

