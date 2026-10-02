import type { ServerNode } from '../types';

export const initialNodes: ServerNode[] = [
  {
    id: 'node-dell-r640-01',
    vendor: 'Dell Inc.',
    model: 'PowerEdge R640',
    serialNumber: '4X9Z8Y2',
    firmwareMode: 'UEFI',
    biosVersion: '2.16.0',
    status: 'READY_FOR_PROVISIONING',
    cpu: {
      model: 'Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz',
      sockets: 2,
      coresPerSocket: 18,
      threadsPerSocket: 36,
      totalThreads: 72,
      arch: 'x86_64'
    },
    memory: {
      totalBytes: 137438953472,
      totalHuman: '128 GiB',
      slotsUsed: 4,
      slotsTotal: 24,
      type: 'DDR4 ECC Registered',
      speedMhz: 2666
    },
    storage: [
      {
        name: 'sda',
        path: '/dev/sda',
        byId: '/dev/disk/by-id/ata-DELL_BOSS-S1_00000000000000000001',
        sizeBytes: 960197124096,
        sizeHuman: '960 GB',
        type: 'BOSS',
        transport: 'pcie',
        model: 'DELL BOSS-S1 RAID1',
        serial: 'BOSS-S1-4X9Z8'
      },
      {
        name: 'nvme0n1',
        path: '/dev/nvme0n1',
        byId: '/dev/disk/by-id/nvme-SAMSUNG_MZQL21T9HCJR-00A07_S64BNG0R101234',
        sizeBytes: 1920383410176,
        sizeHuman: '1.92 TB',
        type: 'NVMe',
        transport: 'nvme',
        model: 'Samsung PM9A3 U.2 NVMe',
        serial: 'S64BNG0R101234'
      }
    ],
    nics: [
      {
        name: 'eno1',
        mac: 'b0:4f:13:2a:44:80',
        speedMbps: 10000,
        carrier: true,
        isBoot: true,
        driver: 'ixgbe',
        pciSlot: 'Embedded LOM 1'
      },
      {
        name: 'eno2',
        mac: 'b0:4f:13:2a:44:81',
        speedMbps: 10000,
        carrier: true,
        isBoot: false,
        driver: 'ixgbe',
        pciSlot: 'Embedded LOM 2'
      }
    ],
    bmc: {
      vendor: 'Dell iDRAC9 Enterprise',
      ip: '10.10.20.45',
      mac: 'b0:4f:13:2a:44:8e',
      dhcp: true,
      channel: 1,
      portMode: 'Dedicated',
      credentialsUpdated: true
    },
    discoveredAt: '2026-10-02T00:01:15Z'
  },
  {
    id: 'node-smc-1029p-02',
    vendor: 'Supermicro',
    model: 'SYS-1029P-WTRT',
    serialNumber: 'S389102X9481',
    firmwareMode: 'UEFI',
    biosVersion: '3.4b',
    status: 'READY_FOR_PROVISIONING',
    cpu: {
      model: 'Intel(R) Xeon(R) Silver 4210R CPU @ 2.40GHz',
      sockets: 2,
      coresPerSocket: 10,
      threadsPerSocket: 20,
      totalThreads: 40,
      arch: 'x86_64'
    },
    memory: {
      totalBytes: 68719476736,
      totalHuman: '64 GiB',
      slotsUsed: 4,
      slotsTotal: 12,
      type: 'DDR4 ECC Registered',
      speedMhz: 2400
    },
    storage: [
      {
        name: 'sda',
        path: '/dev/sda',
        byId: '/dev/disk/by-id/ata-SATADOM-SL_3SE_20191024AA123456',
        sizeBytes: 64023257088,
        sizeHuman: '64 GB',
        type: 'SATADOM',
        transport: 'sata',
        model: 'Innodisk SATADOM-SL 3SE',
        serial: '20191024AA123456'
      },
      {
        name: 'nvme0n1',
        path: '/dev/nvme0n1',
        byId: '/dev/disk/by-id/nvme-Micron_7450_MTFDKCC3T8TFR_223456789012',
        sizeBytes: 3840755982336,
        sizeHuman: '3.84 TB',
        type: 'NVMe',
        transport: 'nvme',
        model: 'Micron 7450 PRO U.3 NVMe',
        serial: '223456789012'
      }
    ],
    nics: [
      {
        name: 'enp3s0f0',
        mac: 'ac:1f:6b:80:12:34',
        speedMbps: 10000,
        carrier: true,
        isBoot: true,
        driver: 'ixgbe',
        pciSlot: '0000:03:00.0'
      },
      {
        name: 'enp3s0f1',
        mac: 'ac:1f:6b:80:12:35',
        speedMbps: 10000,
        carrier: false,
        isBoot: false,
        driver: 'ixgbe',
        pciSlot: '0000:03:00.1'
      }
    ],
    bmc: {
      vendor: 'Supermicro MegaRAC',
      ip: '10.10.20.52',
      mac: 'ac:1f:6b:80:99:aa',
      dhcp: true,
      channel: 1,
      portMode: 'Dedicated',
      credentialsUpdated: true
    },
    discoveredAt: '2026-10-02T00:03:42Z'
  },
  {
    id: 'node-asrock-romed8-03',
    vendor: 'ASRockRack',
    model: 'ROMED8-2T (AMD EPYC)',
    serialNumber: 'M80-C90182741',
    firmwareMode: 'UEFI',
    biosVersion: 'L3.30',
    status: 'DISCOVERING',
    cpu: {
      model: 'AMD EPYC 7742 64-Core Processor',
      sockets: 1,
      coresPerSocket: 64,
      threadsPerSocket: 128,
      totalThreads: 128,
      arch: 'x86_64'
    },
    memory: {
      totalBytes: 274877906944,
      totalHuman: '256 GiB',
      slotsUsed: 8,
      slotsTotal: 8,
      type: 'DDR4 ECC Registered',
      speedMhz: 3200
    },
    storage: [
      {
        name: 'nvme0n1',
        path: '/dev/nvme0n1',
        byId: '/dev/disk/by-id/nvme-KIOXIA-KCM6DRUL3T84_7120A001T941',
        sizeBytes: 3840755982336,
        sizeHuman: '3.84 TB',
        type: 'NVMe',
        transport: 'nvme',
        model: 'Kioxia CM6-R Enterprise NVMe',
        serial: '7120A001T941'
      }
    ],
    nics: [
      {
        name: 'eth0',
        mac: '70:85:c2:d4:ee:10',
        speedMbps: 10000,
        carrier: true,
        isBoot: true,
        driver: 'ixgbe',
        pciSlot: '0000:05:00.0'
      }
    ],
    bmc: {
      vendor: 'ASPEED AST2500',
      ip: '10.10.20.58',
      mac: '70:85:c2:d4:ff:01',
      dhcp: true,
      channel: 1,
      portMode: 'Dedicated',
      credentialsUpdated: true
    },
    discoveredAt: '2026-10-02T00:07:05Z'
  },
  {
    id: 'node-dell-r740-04',
    vendor: 'Dell Inc.',
    model: 'PowerEdge R740xd',
    serialNumber: '9B2F1K8',
    firmwareMode: 'UEFI',
    biosVersion: '2.14.2',
    status: 'ACTIVE',
    cpu: {
      model: 'Intel(R) Xeon(R) Gold 6248R CPU @ 3.00GHz',
      sockets: 2,
      coresPerSocket: 24,
      threadsPerSocket: 48,
      totalThreads: 96,
      arch: 'x86_64'
    },
    memory: {
      totalBytes: 412316860416,
      totalHuman: '384 GiB',
      slotsUsed: 12,
      slotsTotal: 24,
      type: 'DDR4 ECC Registered',
      speedMhz: 2933
    },
    storage: [
      {
        name: 'sda',
        path: '/dev/sda',
        byId: '/dev/disk/by-id/ata-Micron_5300_MTFDDAK960TDS_210987654321',
        sizeBytes: 960197124096,
        sizeHuman: '960 GB',
        type: 'SSD',
        transport: 'sata',
        model: 'Micron 5300 PRO SATA',
        serial: '210987654321'
      }
    ],
    nics: [
      {
        name: 'eno1',
        mac: 'd4:ae:52:84:11:00',
        speedMbps: 25000,
        carrier: true,
        isBoot: true,
        driver: 'bnxt_en',
        pciSlot: 'Broadcom 25GbE rNDC'
      }
    ],
    bmc: {
      vendor: 'Dell iDRAC9 Enterprise',
      ip: '10.10.20.46',
      mac: 'd4:ae:52:84:11:fe',
      dhcp: true,
      channel: 1,
      portMode: 'Dedicated',
      credentialsUpdated: true
    },
    provisioningState: {
      progress: 100,
      stage: 'Active in Production',
      os: 'AlmaLinux 9.4 (Cloud-Init)',
      targetDrive: '/dev/sda',
      targetIp: '10.10.100.12',
      logs: [
        'Zstandard sparse raw image streaming complete (1.8 GB / 32s)',
        'sgdisk -e: Relocated backup GPT header to end of disk',
        'Mounted rootfs in RAM; injected /var/lib/cloud/seed/nocloud/ (MAC-matched network-config)',
        'efibootmgr: registered boot entry AlmaLinux 9 in NVRAM with disk boot priority',
        'System rebooted successfully. Cloud-Init finalized root filesystem expansion.'
      ]
    },
    discoveredAt: '2026-10-01T21:40:00Z'
  }
];
