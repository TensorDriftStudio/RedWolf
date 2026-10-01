# RedWolf Multi-Vendor System Architecture

## 1. High-Level Modular Design

RedWolf coordinates physical bare-metal hardware discovery and operating system provisioning across diverse server vendors (**Dell PowerEdge**, **Supermicro**, **ASRock Rack**, and generic platforms):

```text
+-----------------------------------------------------------------------------------------------+
|                                     REDWOLF CORE CONTAINER                                    |
|                              (Runs with Docker network_mode: host)                            |
|                                                                                               |
|  +---------------------------------+  +-------------------------------+                       |
|  |     dnsmasq Process Manager     |  |       Embedded HTTP Server    |                       |
|  |  • DHCP Option 93 (UEFI/BIOS)   |  |  • Serves iPXE chainloader    |                       |
|  |  • Option 77 (iPXE User-Class)  |  |  • Serves vmlinuz & initramfs |                       |
|  |  • TFTP Server (bootstrap)      |  |  • High-speed OS Image Stream |                       |
|  +---------------------------------+  +-------------------------------+                       |
|                                                                                               |
|  +-----------------------------------------------------------------------------------------+  |
|  |                                  Go Core Engine (Chi API)                               |  |
|  |  • Hardware Inventory Database (SQLite WAL / PostgreSQL)                                |  |
|  |  • Node Finite State Machine (Discovered -> Ready -> Provisioning -> Active)            |  |
|  |  • Multi-Vendor HAL Configuration Generator                                             |  |
|  |  • Cloud-Init Metadata Engine (user-data, meta-data, network-config v2)                   |  |
|  +-----------------------------------------------------------------------------------------+  |
|                                                                                               |
|  +-----------------------------------------------------------------------------------------+  |
|  |                     Embedded Web UI Dashboard (React/Vue + TS via embed.FS)             |  |
|  +-----------------------------------------------------------------------------------------+  |
+-----------------------------------------------|-----------------------------------------------+
                                                │ (PXE Network & HTTP Telemetry)
                                                ▼
+-----------------------------------------------------------------------------------------------+
|                       MULTI-VENDOR DISCOVERY AGENT (Alpine in-memory RAMdisk)                 |
|                                                                                               |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|  │                         Hardware Abstraction Layer (HAL) Drivers                        │  |
|  │    [DellProfile]                [SupermicroProfile]               [ASRockRackProfile]   │  |
|  │    • iDRAC 8/9 (KCS)            • AMI MegaRAC (KCS)               • ASPEED MegaRAC      │  |
|  │    • User: root                 • User: ADMIN                     • User: admin         │  |
|  │    • Pass: 14-16 chars          • Pass: 14-16 chars               • Pass: 14-16 chars   │  |
|  │    • Port: Dedicated            • Port: Force Dedicated (Raw)     • Port: Force Ded.    │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|                                                                                               |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|  │                         Machine-Readable Telemetry Extraction                           │  |
|  │    • CPU: lscpu -J (Sockets, Cores, Threads, Microarchitecture)                         │  |
|  │    • Storage: lsblk -J (NVMe, SATA, SAS, Dell BOSS, SATADOM by Serial & by-id path)     │  |
|  │    • Network: ip -j link / addr (Carrier detection, MAC, speed, boot link identification│  |
|  │    • Firmware: EFI vs BIOS detection via /sys/firmware/efi                              │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|                                                                                               |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|  │                         Bare-Metal Image Streaming & Cloud-Init                         │  |
|  │    • Raw Cloud Image (.zstd) stream directly to target drive (/dev/nvme0n1 or /dev/sda) │  |
|  │    • Local 'cidata' filesystem injection (Cloud-Init NoCloud datasource)                │  |
|  │    • NVRAM bootloader registration (efibootmgr)                                         │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
+-----------------------------------------------------------------------------------------------+
```

---

## 2. Core Subsystems

### 2.1 `redwolf-core` Orchestrator
* **Network Boot Control:** Uses `dnsmasq` under active supervision. When network settings change, RedWolf renders a new configuration and signals `dnsmasq` with `SIGHUP` or restarts the process.
* **Infinite Boot Loop Blocker:** Differentiates the initial NIC PXE ROM from subsequent iPXE boots using DHCP option 77.
* **State Machine (FSM):**
  - `DISCOVERED`: Server is detected booting on the provisioning network.
  - `COLLECTING_TELEMETRY`: In-memory agent is cataloging hardware and configuring BMC.
  - `READY_FOR_PROVISIONING`: Hardware validated, BMC reachable, awaiting operator deployment.
  - `PROVISIONING`: OS image is streaming to storage drive; Cloud-Init partition formatted.
  - `ACTIVE`: Server boots into production operating system.
  - `FAILED`: Provisioning step encountered a hardware or network error.

### 2.2 In-Memory Discovery & Provisioning Engine
* **Universal Driver Pack:** The Alpine-based initramfs explicitly packages enterprise 10GbE/25GbE/100GbE drivers (`i40e`, `ixgbe`, `bnxt_en`, `tg3`, `mlx5_core`) and IPMI kernel modules (`ipmi_si`, `ipmi_devintf`).
* **Hardware Abstraction Layer (HAL):**
  * Auto-selects vendor profile based on `dmidecode -s system-manufacturer`.
  * Manages BMC credentials and interface modes using vendor-compatible rules.
  * Discovers all storage targets by immutable serial numbers and bus types.

### 2.3 Bare-Metal Cloud-Init Streamer
* Traditional OS installers (like Anaconda or debian-installer) are slow and require maintenance across distro versions.
* RedWolf streams official vendor cloud images (`.raw.zstd`) directly onto the target disk.
* RedWolf writes the `cidata` filesystem containing:
  - `user-data`: user accounts, root password hash (`$6$`), authorized SSH keys, default packages.
  - `meta-data`: `instance-id`, `local-hostname`.
  - `network-config`: Netplan v2 / NetworkManager configuration matching network cards by **MAC address**.
* On reboot, Cloud-Init executes directly on the bare-metal machine, resizing the root partition to fill the physical disk and configuring production networking.
