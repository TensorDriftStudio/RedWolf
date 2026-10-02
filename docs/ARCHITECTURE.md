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
|  |  • Dynamic Boot Script Endpoint |  |  • High-speed OS Image Stream |                       |
|  |  • Authoritative or ProxyDHCP   |  |  • /boot.ipxe?mac=${net0/mac} |                       |
|  +---------------------------------+  +-------------------------------+                       |
|                                                                                               |
|  +-----------------------------------------------------------------------------------------+  |
|  |                                  Go Core Engine (Chi API)                               |  |
|  |  • Hardware Inventory Database (SQLite WAL / PostgreSQL)                                |  |
|  |  • Node State Machine (DISCOVERING -> READY -> PROVISIONING -> ACTIVE / ERROR)          |  |
|  |  • Multi-Vendor HAL Configuration Generator & Secure Credential Escrow                  |  |
|  |  • Cloud-Init Metadata Engine (user-data, meta-data, universal network-config)          |  |
|  +-----------------------------------------------------------------------------------------+  |
|                                                                                               |
|  +-----------------------------------------------------------------------------------------+  |
|  |                     Embedded Web UI Dashboard (React + TS + Tailwind)                   |  |
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
|  │    • Account slot verification  • Account slot verification       • Account slot verif. │  |
|  │    • Pass: 14-16 chars safe     • Pass: 14-16 chars safe          • Pass: 14-16 ch safe │  |
|  │    • Port: Dedicated RJ-45      • Port: Dedicated mode query/set  • Port: Channel check │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|                                                                                               |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|  │                         Machine-Readable Telemetry Extraction                           │  |
|  │    • Storage: nvme, megaraid_sas, mpt3sas, smartpqi, ahci via GNU util-linux (lsblk -J) │  |
|  │    • CPU: lscpu -J (Sockets, Cores, Threads, Microarchitecture)                         │  |
|  │    • Network: ip -j link / addr (Carrier detection, MAC, speed, boot link identification│  |
|  │    • Firmware: EFI vs BIOS detection via /sys/firmware/efi                              │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|                                                                                               |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
|  │                         Bare-Metal Image Streaming & Cloud-Init                         │  |
|  │    • Raw Cloud Image (.raw.zstd) sparse stream to target drive (/dev/nvme0n1 or /dev/sda│  |
|  │    • GPT Secondary boundary relocated to end of drive (sgdisk -e)                      │  |
|  │    • Root partition mounted in RAM; direct NoCloud seed injection (/var/lib/cloud/seed) │  |
|  │    • NVRAM bootloader registration (efibootmgr) with disk boot priority                 │  |
|  +─────────────────────────────────────────────────────────────────────────────────────────+  |
+-----------------------------------------------------------------------------------------------+
```

---

## 2. Core Subsystems

### 2.1 `redwolf-core` Orchestrator
* **Network Boot Control:** Uses `dnsmasq` under active supervision. Inspects DHCP Option 93 to serve `undionly.kpxe` for legacy BIOS and `ipxe.efi` for UEFI clients.
* **Infinite Boot Loop Prevention:** RedWolf generates dynamic `/boot.ipxe?mac=${net0/mac}` responses:
  - If node status is `ACTIVE`, iPXE exits (`exit 1`) to drop out of network boot into the local NVMe/SSD bootloader.
  - If node status is `DISCOVERING` or `READY_FOR_PROVISIONING`, iPXE loads the kernel and discovery RAMdisk.
* **State Machine (FSM):**
  - `DISCOVERING`: Node is booting the in-memory agent and gathering telemetry.
  - `READY_FOR_PROVISIONING`: Hardware validated, BMC reachable, awaiting operator deployment.
  - `PROVISIONING`: OS image is streaming to storage drive; Cloud-Init NoCloud seed injected into rootfs.
  - `ACTIVE`: Server boots into production operating system; subsequent PXE requests drop to local disk.
  - `ERROR`: Provisioning step encountered a hardware or network error.

### 2.2 In-Memory Discovery & Provisioning Engine
* **Universal Driver Pack:** The Alpine-based initramfs explicitly packages enterprise storage controller drivers (`megaraid_sas`, `mpt3sas`, `smartpqi`, `nvme`), 10GbE/25GbE/100GbE network drivers (`i40e`, `ixgbe`, `bnxt_en`, `tg3`, `mlx5_core`), filesystem tools (`xfsprogs`, `e2fsprogs`, `bmap-tools`, `sgdisk`), and GNU `util-linux` for full JSON parsing support (`lsblk -J`, `lscpu -J`).
* **Hardware Abstraction Layer (HAL):**
  - Matches platform via `dmidecode -s system-manufacturer` with fallback to `baseboard-manufacturer`.
  - Non-destructive BMC discovery: inspects existing users before updating credentials.
  - Discovers storage drives by immutable serial numbers and bus types.

### 2.3 Bare-Metal Cloud-Init Streamer
* Streams official vendor cloud images (`.raw.zstd`) directly onto the target disk.
* Automatically repairs the GPT header boundary to the end of the physical disk (`sgdisk -e`).
* Temporarily mounts the root partition in RAM to write `/var/lib/cloud/seed/nocloud/`:
  - `user-data`: user accounts, root password hash (`$6$`), authorized SSH keys, default packages.
  - `meta-data`: `instance-id`, `local-hostname`.
  - `network-config`: MAC-address matched universal network configuration (Netplan v2 modern routing or Cloud-Init v1 format).
* Sets UEFI boot priority via `efibootmgr` and reboots.
* On reboot, Cloud-Init executes directly on bare metal, resizing the root partition via `cloud-init-growpart` to fill the physical disk without encountering partition blocking.
