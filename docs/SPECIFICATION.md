# RedWolf Technical Specification

## 1. Scope & System Overview

**RedWolf** is an automated inventory discovery and operating system provisioning platform engineered for Data Center server environments, bare-metal hardware (with **Dell PowerEdge R640** serving as reference hardware), and virtual machines.

This document outlines the detailed functional requirements, communication protocols, telemetry schemas, and lifecycle workflows.

---

## 2. Network Topology & Physical Racking

### 2.1 Physical Cable Connections (Dell PowerEdge R640 Reference)
In the server cabinet, the data center technician completes the following physical connections:
1. **Power:** PSU1 and PSU2 connected to independent A/B power distribution units (PDUs).
2. **BMC / iDRAC Port:** Dedicated 1GbE RJ-45 iDRAC 9 port connected to the Out-of-Band (OOB) management VLAN.
3. **Provisioning Port:** Primary network interface (`NIC 1` / LOM port 1 / `eno1`) connected to the dedicated provisioning VLAN.

```text
+---------------------------------------------------------------+
|                      DELL PowerEdge R640                      |
|                                                               |
|  [PSU 1] [PSU 2]     [iDRAC RJ-45]     [NIC 1] [NIC 2] [NIC 3]|
+-----|-------|--------------|--------------|-------------------+
      |       |              |              |
    Power A / B           OOB VLAN       Provisioning VLAN
                        (Management)    (RedWolf DHCP/TFTP)
```

---

## 3. Auto-Discovery & Network Boot (PXE / iPXE)

### 3.1 Network Services Managed by RedWolf Core
RedWolf automatically starts and orchestrates two core network services on the provisioning interface:
1. **DHCP Server:**
   - Listens for `DHCPDISCOVER` packets on the provisioning broadcast domain.
   - Detects client architecture via DHCP Option 93 (UEFI x86_64 vs Legacy x86 BIOS).
   - Returns `Next-Server` (RedWolf IP) and `Bootfile-Name` (`ipxe.efi` for UEFI / `undionly.kpxe` for legacy BIOS).
2. **TFTP & HTTP Boot Services:**
   - Transfers the initial iPXE chainloader via TFTP.
   - iPXE immediately chains over high-throughput HTTP to retrieve the Linux kernel (`vmlinuz`) and the **RedWolf Discovery Agent** initramfs (`initramfs.img`).

### 3.2 RedWolf Discovery Agent Lifecycle
The discovery agent executes completely in RAM without touching local storage drives.

#### A. Hardware Inventory Gathering:
- **Processor (CPU):**
  - Queried via `/proc/cpuinfo` and `lscpu`.
  - Captures processor model (e.g. `Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz`).
  - Total sockets, physical cores per socket, logical threads per socket.
  - Virtualization extensions (VT-x, AMD-V) and crypto flags (AES-NI).
- **Platform & Chassis DMI:**
  - Extracted via `dmidecode -s system-manufacturer`, `system-product-name`, and `system-serial-number`.
  - Validates `Dell Inc. PowerEdge R640` and records the Dell Service Tag.
- **System Memory (RAM):**
  - Total usable and installed memory capacity in bytes / GiB.
  - DIMM slot mapping via `dmidecode -t memory`: channel layout, memory type (DDR4 ECC Registered), and module frequencies.
- **Network Interface Cards (NICs):**
  - Discovered via `/sys/class/net/*` and `ip -j link`.
  - Records MAC addresses, physical link states, negotiated speeds (1GbE / 10GbE / 25GbE), and PCI bus locations.
- **Storage Devices:**
  - Enumerated via `lsblk -J -b -o NAME,SIZE,TYPE,MODEL,SERIAL,ROTA,TRAN`.
  - Distinguishes NVMe SSDs, SAS/SATA drives, and Dell BOSS-S1 boot controllers.

#### B. BMC / iDRAC Automation (In-Band via IPMI KCS):
- New or unconfigured servers often do not have static IP addresses or knowable credentials on the iDRAC port.
- The discovery agent loads the `ipmi_si` and `ipmi_devintf` kernel modules to communicate over the local motherboard KCS (Keyboard Controller Style) interface.
- Without requiring network access to iDRAC, the agent executes:
  ```bash
  # 1. Configure dedicated administrator account on BMC
  ipmitool user set name 2 <REDWOLF_BMC_USER>
  ipmitool user set password 2 <REDWOLF_BMC_PASSWORD>
  ipmitool user enable 2
  ipmitool channel setaccess 1 2 callin=on ipmi=on link=on privilege=4

  # 2. Configure BMC interface to acquire IP via DHCP
  ipmitool lan set 1 ipsrc dhcp

  # 3. Poll for assigned BMC IP address
  ipmitool lan print 1 | grep "IP Address"
  ```
- The acquired BMC IP address, BMC MAC address, and credential confirmation are stored in the telemetry payload.

#### C. Telemetry Payload Transmission:
The agent posts a structured JSON payload to the RedWolf Core API endpoint: `POST /api/v1/discovery/report`.

```json
{
  "system": {
    "manufacturer": "Dell Inc.",
    "model": "PowerEdge R640",
    "serial_number": "4X9Z8Y2",
    "bios_version": "2.16.0"
  },
  "cpu": {
    "model": "Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz",
    "sockets": 2,
    "cores_per_socket": 18,
    "threads_per_socket": 36,
    "total_threads": 72
  },
  "memory": {
    "total_bytes": 137438953472,
    "total_human": "128 GiB",
    "slots_used": 4,
    "slots_total": 24,
    "type": "DDR4 ECC Registered"
  },
  "network_interfaces": [
    {
      "name": "eno1",
      "mac": "b0:4f:13:2a:44:80",
      "speed_mbps": 10000,
      "link_detected": true,
      "pci_slot": "Embedded LOM 1"
    },
    {
      "name": "eno2",
      "mac": "b0:4f:13:2a:44:81",
      "speed_mbps": 10000,
      "link_detected": false,
      "pci_slot": "Embedded LOM 2"
    }
  ],
  "bmc": {
    "type": "iDRAC9",
    "mac": "b0:4f:13:2a:44:8e",
    "ip_address": "192.168.100.45",
    "dhcp_enabled": true,
    "credentials_updated": true
  },
  "storage_devices": [
    {
      "name": "sda",
      "size_bytes": 960197124096,
      "size_human": "960 GB",
      "type": "SSD",
      "model": "DELL BOSS-S1"
    },
    {
      "name": "sdb",
      "size_bytes": 1920383410176,
      "size_human": "1.92 TB",
      "type": "NVMe",
      "model": "Samsung PM9A3"
    }
  ]
}
```

---

## 4. RedWolf GUI Node Representation

Upon receipt of the telemetry report:
1. The server record in the database transitions to state `READY_FOR_PROVISIONING`.
2. The Web Dashboard displays an interactive server card with the **"Ready for Provisioning"** badge.
3. The operator sees:
   - Full hardware summary (CPU, RAM, MAC table, physical disks).
   - Direct link to the iDRAC web console (`https://<BMC_IP>`).
   - A **"Deploy Server"** button launching the provisioning configuration wizard.

---

## 5. Cloud-Init OS Provisioning Engine

### 5.1 Supported Operating Systems
RedWolf provides automated deployment workflows for:
- **AlmaLinux 8**
- **AlmaLinux 9**
- **AlmaLinux 10**
- **Debian 12 (Bookworm)**
- **Debian 13 (Trixie)**

### 5.2 Wizard Configuration Parameters:
1. **Operating System Selection:** Target distribution and version.
2. **Storage Layout & Partitioning:**
   - Target drive selection (e.g. `/dev/sda` or Dell BOSS-S1 virtual drive).
   - Layout mode: Standard partitions (`/boot/efi`, `/boot`, `/`, `swap`) or LVM with flexible volume groups.
3. **Security & Access Control:**
   - Root password (hashed using SHA-512 crypt `$6$`).
   - Authorized SSH public keys for root and default administrative accounts.
4. **Target Production Networking:**
   - Interface selection or NIC bonding (e.g. `bond0` combining `eno1` + `eno2` using 802.3ad LACP).
   - Network addressing: Static IP (IPv4 CIDR, default gateway, DNS servers) or production DHCP.
   - Optional VLAN tagging (802.1Q).

### 5.3 Metadata Rendering & Execution
RedWolf renders the metadata and serves it via an authenticated HTTP endpoint (`http://<REDWOLF_IP>/cloud-init/<MAC>/`):
- `user-data`: Defines users, authorized keys, password hashes, package repositories, and initial tooling (`qemu-guest-agent`, `curl`, `htop`).
- `meta-data`: Defines `instance-id` and `local-hostname`.
- `network-config`: Standard Netplan v2 or NetworkManager configuration.

The provisioning engine streams the target base image to disk, injects the Cloud-Init configuration, triggers local bootloader installation, and commands the node to reboot into production.
