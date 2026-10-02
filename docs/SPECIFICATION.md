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

Vendor BMCs, storage topologies, and network enumeration diverge significantly across manufacturers. RedWolf implements a modular **Vendor Profile Pattern** triggered by DMI manufacturer detection (`dmidecode -s system-manufacturer`, falling back to `baseboard-manufacturer`).

```text
                                [Node Boots in RAM]
                                         │
                         Detect DMI System / Baseboard Mfr
                                         │
        ┌────────────────────────────────┼────────────────────────────────┐
        ▼                                ▼                                ▼
  [DellProfile]                 [SupermicroProfile]              [ASRockRackProfile]
  • Chassis: Dell Inc.          • Chassis: Supermicro            • Chassis: ASRockRack / ASRock
  • BMC: iDRAC 8/9 (KCS)        • BMC: MegaRAC (KCS)             • BMC: MegaRAC / ASPEED
  • Verified Admin User slot    • Verified Admin User slot       • Verified Admin User slot
  • Password: 14-16 char safe   • Password: 14-16 char safe      • Password: 14-16 char safe
  • Port: Dedicated RJ-45       • Port: Raw cmd force Dedicated  • Port: Channel check & Dedicated
  • Boot drives: BOSS / NVMe    • Boot drives: SATADOM / NVMe    • Boot drives: M.2 NVMe / SATA
```

### 2.1 BMC Credential Standards & Escrow Policy
Standard IPMI 2.0 implementations over the KCS (Keyboard Controller Style) LPC bus have an RFC specification buffer limit of **16 to 20 bytes** for user passwords. While Dell iDRAC 9 web console allows up to 32 characters, Supermicro and ASRock Rack MegaRAC BMCs will reject or silently truncate passwords longer than 16 characters (`0xd5` parameter out of range).

* **RedWolf Cross-Vendor Password Policy:**
  * Password length: Strictly **14 to 16 characters**.
  * Complexity: At least one uppercase letter, one lowercase letter, one digit, and one standard symbol (`!@#$%^&*`).
  * Universal compatibility: Accepted without truncation by Dell iDRAC, Supermicro AMI MegaRAC, and ASRock Rack ASPEED BMCs.
* **Non-Destructive Credential Discovery & Escrow:**
  * Telemetry discovery is **read-only** by default. RedWolf will not automatically overwrite existing administrator credentials.
  * Account inspection (`ipmitool user list 1`) determines the active administrator user ID before making changes.
  * BMC credential synchronization is an explicit operator-confirmed action. Generated credentials are encrypted (AES-256-GCM) in RedWolf Core and made accessible to authorized administrators via the UI.

### 2.2 BMC Physical Interface Mode & Switch Port Negotiation
* **Dell PowerEdge:** Out-of-band management is wired to a dedicated enterprise RJ-45 port.
* **Supermicro & ASRock Rack:** The BMC network port mode can be configured in BIOS to *Dedicated*, *Shared (LAN1 NC-SI)*, or *Failover*. If the BMC is set to Shared while a technician connects the dedicated IPMI port, the BMC will never receive a DHCP lease.
* **Vendor Remediation via Discovery Agent:**
  * For Supermicro X10/X11 platforms, the agent queries and sets the Dedicated management port:
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
  * **Carrier-Aware Polling with STP Backoff:** When switching physical PHY interfaces, enterprise switch ports running Spanning Tree Protocol (STP / RSTP) require 30–50 seconds for link state negotiation if PortFast is disabled. The agent polls with an extended timeout (up to 90–120 seconds), validating link carrier before querying the acquired IP address.

---

## 3. Network Architecture & Zero-Loop PXE/iPXE Boot

### 3.1 Network Services Managed by RedWolf Core
RedWolf Core manages the provisioning broadcast domain using containerized network services:

1. **Multi-Architecture DHCP Boot Orchestration (`dnsmasq`):**
   * Inspects DHCP Option 93 (Client System Architecture) to deliver architecture-compliant initial loaders:
     * Architecture `0000` (x86 BIOS): `undionly.kpxe`
     * Architecture `0007` / `0009` (x86_64 UEFI): `ipxe.efi`
     * Architecture `0011` (ARM64 UEFI): `ipxe-arm64.efi`
   * **Multi-Arch `dnsmasq` Configuration:**
     ```text
     dhcp-match=set:bios,option:client-arch,0
     dhcp-match=set:efi64,option:client-arch,7
     dhcp-match=set:efi64,option:client-arch,9
     dhcp-match=set:ipxe,77,"iPXE"

     # Deliver initial iPXE loader based on client firmware
     dhcp-boot=tag:!ipxe,tag:bios,undionly.kpxe
     dhcp-boot=tag:!ipxe,tag:efi64,ipxe.efi

     # Dynamic MAC-keyed iPXE chainloader
     dhcp-boot=tag:ipxe,http://<REDWOLF_IP>:8080/boot.ipxe?mac=${net0/mac}
     ```

2. **Infinite Boot Loop Prevention via Dynamic State Machine:**
   * RedWolf Core dynamically renders `/boot.ipxe?mac=...`:
     * When node status is `DISCOVERING` or `READY_FOR_PROVISIONING`: streams `vmlinuz` and `initramfs.img`.
     * When node status is `ACTIVE`: renders an iPXE chainloader that drops to local storage:
       ```text
       #!ipxe
       echo RedWolf: Node is ACTIVE. Booting from local drive...
       exit 1
       ```
   * The provisioned UEFI NVRAM boot order is set via `efibootmgr -o` so the local storage bootloader precedes network PXE boot.

3. **Deployment Network Topologies:**
   * **Dedicated Provisioning VLAN (Default):** Authoritative DHCP serving leases and PXE boot metadata.
   * **Shared Enterprise Subnet (ProxyDHCP):** `dnsmasq` runs in ProxyDHCP mode (`dhcp-range=...,proxy`), delivering bootloader filenames and options without issuing IP leases.

---

## 4. In-Memory Discovery Engine (`redwolf-discovery`)

The discovery agent executes entirely in RAM. It strictly parses machine-readable JSON outputs from GNU utilities (`util-linux`) to eliminate regex fragility across kernel releases.

### 4.1 Required Kernel Drivers & Alpine Packages
The Alpine initramfs explicitly bundles:
* **Storage Drivers:** `nvme`, `ahci`, `megaraid_sas` (Dell PERC / Broadcom MegaRAID), `mpt3sas` (Broadcom SAS HBAs), `smartpqi` (HPE Smart Array).
* **Network Drivers:** `i40e`, `ixgbe`, `bnxt_en`, `tg3`, `mlx5_core`.
* **IPMI Modules:** `ipmi_si`, `ipmi_devintf`, `ipmi_msghandler`.
* **Filesystem Tools:** `util-linux` (providing full `lsblk -J` and `lscpu -J`), `xfsprogs`, `e2fsprogs`, `dosfstools`, `bmap-tools`, `sgdisk`.

### 4.2 Telemetry Collection Commands
| Component | Command | Extracted Fields |
| :--- | :--- | :--- |
| **System & Chassis** | `dmidecode -s ...` | Manufacturer, Product Name, Serial Number / Service Tag, UUID |
| **Firmware Mode** | `[ -d /sys/firmware/efi ]` | `UEFI` vs `BIOS` |
| **CPU** | `lscpu -J` | Architecture, Model, Sockets, Cores per Socket, Threads per Socket, Virtualization flags |
| **Memory** | `dmidecode -t memory` | Total capacity, slot population, type (DDR4/DDR5 ECC Reg), frequency |
| **Storage Devices** | `lsblk -J -b -o NAME,SIZE,TYPE,MODEL,SERIAL,WWN,TRAN,ROTA` | Physical disks, NVMe, SATA, SAS, Dell BOSS, SATADOM, exact bytes, serial number |
| **Network Interfaces** | `ip -j link` & `ethtool` | MAC address, link speed, carrier status, PCI bus address, driver name |
| **Active Boot Interface** | `/proc/net/pnp` / route | Identifies the exact MAC used to boot from RedWolf |

---

## 5. Storage-Safe Image Streaming & Cloud-Init Injection

Bare-metal servers do not have cloud hypervisors to attach virtual metadata ISOs. RedWolf solves this by **direct image streaming and in-memory root filesystem injection**:

```text
[RedWolf Core]
       │
       │ HTTP Sparse Image Stream (zstd compressed / bmaptool)
       ▼
[Discovery Agent in RAM]
       │
       ├─► 1. Stream OS image to explicitly selected target drive (e.g. /dev/nvme0n1)
       │      curl -s http://.../almalinux-9.raw.zstd | zstd -d | dd of=/dev/nvme0n1 bs=4M conv=sparse
       │
       ├─► 2. Relocate and repair secondary GPT header to end of disk
       │      sgdisk -e /dev/nvme0n1 && partx -u /dev/nvme0n1
       │
       ├─► 3. Mount target root filesystem in RAM
       │      mount /dev/nvme0n1p3 /mnt/target
       │
       ├─► 4. Direct Cloud-Init NoCloud Injection (No extra partition needed)
       │      Write /mnt/target/var/lib/cloud/seed/nocloud/user-data
       │      Write /mnt/target/var/lib/cloud/seed/nocloud/meta-data
       │      Write /mnt/target/var/lib/cloud/seed/nocloud/network-config
       │      umount /mnt/target
       │
       ├─► 5. Register NVRAM Bootloader Entry (efibootmgr)
       │      efibootmgr -c -d /dev/nvme0n1 -p 1 -L "RedWolf OS" -l '\EFI\<distro>\shimx64.efi'
       │      efibootmgr -o <new_boot_order>
       │
       └─► 6. System reboot into production OS (Cloud-Init grows root partition cleanly)
```

### 5.1 Storage Engine Tiers
* **Tier 1 (Block Streaming):** High-speed streaming for single NVMe drives, SAS/SATA SSDs, Dell BOSS-S1/S2 RAID 1, and Hardware RAID virtual disks.
* **Tier 2 (Software RAID / LVM):** Managed multi-disk layouts requiring automated installer hooks or pre-partitioning steps.

### 5.2 Universal MAC-Based Cloud-Init Network Matching
Network configuration strictly binds to physical MAC addresses to prevent interface naming mismatches across distributions and hardware vendors:

```yaml
version: 1
config:
  - type: physical
    name: eth0
    mac_address: "ac:1f:6b:80:12:34"
    subnets:
      - type: static
        address: 10.10.20.50
        netmask: 255.255.255.0
        gateway: 10.10.20.1
        dns_nameservers:
          - 1.1.1.1
          - 8.8.8.8
```

For Netplan v2 compatible environments (Debian/Ubuntu), standard modern routing syntax is generated:
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
      routes:
        - to: default
          via: 10.10.20.1
      nameservers:
        addresses:
          - 1.1.1.1
          - 8.8.8.8
```

---

## 6. Supported Operating System Matrix

| Operating System | Image Format | Cloud-Init Version | Status |
| :--- | :--- | :--- | :--- |
| **AlmaLinux 9 (Current)** | GenericCloud `.raw.zstd` | Cloud-Init 23.x / 24.x | Production Ready (Recommended) |
| **AlmaLinux 8 (Legacy)** | GenericCloud `.raw.zstd` | Cloud-Init 22.x | Production Ready |
| **Debian 12 (Bookworm)** | GenericCloud `.raw.zstd` | Cloud-Init 22.x / 23.x | Production Ready (LTS) |
| **AlmaLinux 10** | GenericCloud `.raw.zstd` | Cloud-Init 24.x | Technology Preview / Development |
| **Debian 13 (Trixie)** | GenericCloud `.raw.zstd` | Cloud-Init 24.x | Technology Preview / Testing |
