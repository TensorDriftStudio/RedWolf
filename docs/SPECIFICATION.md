# RedWolf Technical Specification & Hardware Abstraction Layer

## 1. Scope & System Overview

**RedWolf** is an enterprise-grade automated hardware inventory discovery and operating system provisioning platform. It is engineered for Data Center server environments, supporting multi-vendor bare-metal architectures as well as virtual machines.

### Supported Vendor Platforms
* **Dell PowerEdge:** 13th, 14th, 15th, and 16th generation (reference model: **Dell PowerEdge R640**, iDRAC 8/9).
* **Supermicro:** Intel and AMD platforms (X10, X11, X12, H11, H12, H13 series with AMI MegaRAC BMC).
* **ASRock Rack:** Server motherboards (EPYCD8, ROMED8, B650D4, Paul series with ASPEED AST2500/AST2600 BMC).
* **Generic x86_64 & Virtualization:** Standards-compliant IPMI 2.0 / Redfish servers and virtual machines (KVM, Proxmox, VMware ESXi).

---

## 2. Hardware Abstraction Layer (HAL) & Vendor Profiles

Vendor BMCs, storage topologies, and network enumeration diverge significantly across manufacturers. RedWolf implements a modular **Vendor Profile Pattern** triggered by DMI manufacturer detection (`dmidecode -s system-manufacturer`).

```text
                                [Node Boots in RAM]
                                         │
                         Detect DMI System Manufacturer
                                         │
        ┌────────────────────────────────┼────────────────────────────────┐
        ▼                                ▼                                ▼
  [DellProfile]                 [SupermicroProfile]              [ASRockRackProfile]
  • Chassis: Dell Inc.          • Chassis: Supermicro            • Chassis: ASRockRack
  • BMC: iDRAC 8/9 (KCS)        • BMC: MegaRAC (KCS)             • BMC: MegaRAC / ASPEED
  • User 2: "root"              • User 2: "ADMIN"                • User 2: "admin"
  • Password: Up to 20/32 char  • Password: 12-16 char safe      • Password: 12-16 char safe
  • Port: Dedicated RJ-45       • Port: Raw cmd force Dedicated  • Port: Force Dedicated
  • Boot drives: BOSS / NVMe    • Boot drives: SATADOM / NVMe    • Boot drives: M.2 NVMe / SATA
```

### 2.1 BMC Credential Standards & Cross-Vendor Compatibility
Standard IPMI 2.0 implementations over the KCS (Keyboard Controller Style) LPC bus have an RFC specification buffer limit of **16 to 20 bytes** for user passwords. While Dell iDRAC 9 web console allows up to 32 characters, Supermicro and ASRock Rack MegaRAC BMCs will reject or silently truncate passwords longer than 16 characters (`0xd5` parameter out of range).

* **RedWolf Cross-Vendor Password Policy:**
  * Password length: Strictly **14 to 16 characters**.
  * Complexity: At least one uppercase letter, one lowercase letter, one digit, and one standard symbol (`!@#$%^&*`).
  * Universal compatibility: Accepted without truncation by Dell iDRAC, Supermicro AMI MegaRAC, and ASRock Rack ASPEED BMCs.

### 2.2 BMC Physical Interface Mode (Dedicated vs. Shared NC-SI)
* **Dell PowerEdge:** Out-of-band management is wired to a dedicated enterprise port.
* **Supermicro & ASRock Rack:** The BMC network port mode can be configured in BIOS to *Dedicated*, *Shared (LAN1 NC-SI)*, or *Failover*. If the BMC is set to Shared while a technician connects the dedicated IPMI port, the BMC will never receive a DHCP lease.
* **Remediation via Discovery Agent:**
  * For Supermicro, the agent executes OEM raw IPMI commands to ensure the Dedicated management port is active:
    ```bash
    # Supermicro: Query LAN port mode (0x00=Dedicated, 0x01=Shared, 0x02=Failover)
    ipmitool raw 0x30 0x70 0x0c 0
    # Supermicro: Force Dedicated mode
    ipmitool raw 0x30 0x70 0x0c 1 0
    ```
  * Configures DHCP mode:
    ```bash
    ipmitool lan set 1 ipsrc dhcp
    ```
  * **Asynchronous Polling with Backoff:** The agent polls `ipmitool lan print 1` every 3 seconds for up to 30 seconds to allow the management switch to negotiate link and dispatch the DHCP lease.

---

## 3. Network Architecture & Zero-Loop PXE/iPXE Boot

### 3.1 Network Services Managed by RedWolf Core
RedWolf Core manages the provisioning broadcast domain using containerized, battle-tested network services:
1. **Network Boot Orchestration (`dnsmasq`):**
   * Handles DHCP Option 93 (Client System Architecture) to deliver appropriate loaders:
     * Architecture `0000` (x86 BIOS): `undionly.kpxe`
     * Architecture `0007` / `0009` (x86_64 UEFI): `ipxe.efi`
     * Architecture `0011` (ARM64 UEFI): `ipxe-arm64.efi`
   * **Infinite Boot Loop Prevention:** RedWolf inspects DHCP Option 77 (`user-class`). If the request originates from standard PXE ROM, it serves iPXE. If the request originates from `iPXE`, it chains to the HTTP boot script:
     ```text
     dhcp-match=set:ipxe,77,"iPXE"
     dhcp-boot=tag:!ipxe,ipxe.efi
     dhcp-boot=tag:ipxe,http://<REDWOLF_IP>:8080/boot.ipxe
     ```
2. **High-Speed HTTP Asset Server:**
   * Serves Linux kernel (`vmlinuz`), discovery ramdisk (`initramfs.img`), and raw compressed OS images over HTTP (avoiding slow UDP TFTP bottlenecks).

---

## 4. In-Memory Discovery Engine (`redwolf-discovery`)

The discovery agent executes entirely in RAM. It strictly parses machine-readable JSON outputs to eliminate regex breakage across varying kernel releases.

### 4.1 Telemetry Collection Commands
| Component | Command | Extracted Fields |
| :--- | :--- | :--- |
| **System & Chassis** | `dmidecode -s ...` | Manufacturer, Product Name, Serial Number / Service Tag, UUID |
| **Firmware Mode** | `[ -d /sys/firmware/efi ]` | `UEFI` vs `BIOS` |
| **CPU** | `lscpu -J` | Architecture, Model, Sockets, Cores per Socket, Threads per Socket, Virtualization flags |
| **Memory** | `dmidecode -t memory` | Total capacity, slot population, type (DDR4/DDR5 ECC Reg), frequency |
| **Storage Devices** | `lsblk -J -b -o NAME,SIZE,TYPE,MODEL,SERIAL,WWN,TRAN,ROTA` | NVMe, SATA, SAS, Dell BOSS, Supermicro SATADOM, exact bytes, serial number |
| **Network Interfaces** | `ip -j link` & `ethtool` | MAC address, link speed, carrier status, PCI bus address, driver name |
| **Active Boot Interface** | `/proc/net/pnp` / route | Identifies the exact MAC used to boot from RedWolf |

### 4.2 Telemetry JSON Schema Example
```json
{
  "hardware": {
    "manufacturer": "Supermicro",
    "product_name": "SYS-1029P-WTRT",
    "serial_number": "S123456789X",
    "uuid": "4c4c4544-004a-4410-804d-b3c04f523432",
    "firmware_mode": "UEFI"
  },
  "cpu": {
    "model": "Intel(R) Xeon(R) Silver 4210R CPU @ 2.40GHz",
    "sockets": 2,
    "cores_per_socket": 10,
    "threads_per_socket": 20,
    "total_threads": 40
  },
  "memory": {
    "total_bytes": 68719476736,
    "total_human": "64 GiB",
    "slots_used": 4,
    "slots_total": 12,
    "type": "DDR4 ECC Registered"
  },
  "storage_devices": [
    {
      "name": "nvme0n1",
      "path": "/dev/nvme0n1",
      "by_id": "/dev/disk/by-id/nvme-SAMSUNG_MZQL21T9HCJR-00A07_S64BNG0R101234",
      "size_bytes": 1920383410176,
      "size_human": "1.92 TB",
      "type": "nvme",
      "transport": "nvme",
      "model": "SAMSUNG MZQL21T9HCJR-00A07",
      "serial": "S64BNG0R101234"
    },
    {
      "name": "sda",
      "path": "/dev/sda",
      "by_id": "/dev/disk/by-id/ata-SATADOM-SL_3SE_20191024AA123456",
      "size_bytes": 64023257088,
      "size_human": "64 GB",
      "type": "disk",
      "transport": "sata",
      "model": "SATADOM-SL 3SE",
      "serial": "20191024AA123456"
    }
  ],
  "network_interfaces": [
    {
      "name": "enp3s0f0",
      "mac": "ac:1f:6b:80:12:34",
      "carrier": true,
      "is_boot_interface": true,
      "speed_mbps": 10000,
      "driver": "ixgbe",
      "pci_slot": "0000:03:00.0"
    }
  ],
  "bmc": {
    "vendor": "Supermicro",
    "mac": "ac:1f:6b:80:99:aa",
    "ip_address": "192.168.100.52",
    "dhcp_enabled": true,
    "channel": 1,
    "port_mode": "Dedicated",
    "credentials_updated": true
  }
}
```

---

## 5. Storage-Safe Image Streaming & Cloud-Init Injection

Bare-metal servers do not have cloud hypervisors to attach virtual metadata ISOs. RedWolf solves this with **Direct Stream & NoCloud Injection**:

```text
[RedWolf Core]
       │
       │ HTTP Image Stream (zstd compressed)
       ▼
[Discovery Agent in RAM]
       │
       ├─► 1. Write OS image to explicitly selected target drive (e.g. /dev/nvme0n1)
       │      curl -s http://.../almalinux-9.raw.zstd | zstd -d | dd of=/dev/nvme0n1 bs=4M
       │
       ├─► 2. Re-read partition table (partx -u /dev/nvme0n1)
       │
       ├─► 3. Create Cloud-Init 'cidata' partition on target disk
       │      Format FAT32/ext4 with filesystem label: "cidata"
       │      Write /cidata/user-data, /cidata/meta-data, /cidata/network-config
       │
       ├─► 4. Register NVRAM Bootloader Entry (efibootmgr)
       │
       └─► 5. System reboot into production OS
```

### 5.1 Deterministic Storage Target Selection
* The GUI prompts the user to select the boot drive with clear serial numbers and transport types (e.g. `Samsung PM9A3 1.92TB NVMe [SN: S64BNG0R...]`).
* RedWolf **never** writes blindly to `/dev/sda`. It writes directly to the immutable device path `/dev/disk/by-id/<ID>` or verified kernel name.

### 5.2 MAC-Based Cloud-Init Network Matching
Because Linux kernel device names vary across vendors (`eno1` on Dell, `enp3s0f0` on Supermicro, `eth0` on ASRock), Cloud-Init `network-config` version 2 configurations must bind to **MAC addresses**, never device names:

```yaml
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "ac:1f:6b:80:12:34"
      set-name: eth0
      addresses:
        - 10.10.20.50/24
      gateway4: 10.10.20.1
      nameservers:
        addresses:
          - 1.1.1.1
          - 8.8.8.8
```

---

## 6. Supported Operating System Matrix

| Operating System | Image Format | Cloud-Init Version | Storage Support |
| :--- | :--- | :--- | :--- |
| **AlmaLinux 8** | GenericCloud `.raw.zstd` | Cloud-Init 22.x | NVMe, SATA, SAS, LVM, Software RAID |
| **AlmaLinux 9** | GenericCloud `.raw.zstd` | Cloud-Init 23.x | NVMe, SATA, SAS, LVM, Software RAID |
| **AlmaLinux 10** | GenericCloud `.raw.zstd` | Cloud-Init 24.x | NVMe, SATA, SAS, LVM, Software RAID |
| **Debian 12 (Bookworm)** | GenericCloud `.raw.zstd` | Cloud-Init 22.x / 23.x | NVMe, SATA, SAS, LVM, Software RAID |
| **Debian 13 (Trixie)** | GenericCloud `.raw.zstd` | Cloud-Init 24.x | NVMe, SATA, SAS, LVM, Software RAID |
