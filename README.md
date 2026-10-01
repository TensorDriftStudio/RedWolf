<p align="center">
  <img src="assets/logo/redwolf-horizontal.svg" alt="RedWolf Logo" width="560">
</p>

<p align="center">
  <strong>Modern, open-source automated provisioning platform for Bare Metal servers (Dell, Supermicro, ASRock Rack) and Virtual Machines</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Status-In%20Development-crimson.svg?style=flat-square" alt="Status">
  <img src="https://img.shields.io/badge/License-Apache%202.0%20%2F%20GPLv3-blue.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Platforms-Dell%20%7C%20Supermicro%20%7C%20ASRock%20Rack-darkred.svg?style=flat-square" alt="Platforms">
  <img src="https://img.shields.io/badge/OS%20Targets-AlmaLinux%20%7C%20Debian-orange.svg?style=flat-square" alt="Targets">
</p>

---

## 🐺 About RedWolf

**RedWolf** is an open-source bare-metal and virtual machine automation system built for data center infrastructure engineers, sysadmins, and DevOps teams. Its purpose is to streamline server installation, turning bare hardware racking into a zero-touch, hands-off provisioning process.

RedWolf features a built-in **Hardware Abstraction Layer (HAL)** supporting major enterprise hardware platforms:
* **Dell PowerEdge:** 13th, 14th, 15th, and 16th Gen (R640, R740, R750 with iDRAC 8/9).
* **Supermicro:** Intel and AMD platforms (X10, X11, X12, H11, H12 with AMI MegaRAC BMC).
* **ASRock Rack:** Server motherboards (EPYCD8, ROMED8, B650D4 series with ASPEED AST2500/2600 BMC).
* **Virtualization & Generic x86_64:** Standard IPMI 2.0 / Redfish servers and VMs (KVM, Proxmox, VMware ESXi).

---

## ⚡ Technician Deployment Workflow

Traditional provisioning requires connecting local crash carts/monitors, manually configuring BIOS and BMC settings, flashing USB media, or manually assigning static IPs. **RedWolf eliminates all manual setup:**

```mermaid
sequenceDiagram
    autonumber
    actor Tech as 👷 DC Technician
    participant Srv as 🖥️ Target Server (Dell/Supermicro/ASRock)
    participant RW as 🐺 RedWolf (dnsmasq/Core)
    participant BMC as 🔌 BMC (iDRAC / MegaRAC / ASPEED)
    actor Admin as 💻 Admin (RedWolf GUI)

    Tech->>Srv: 1. Mount server in rack
    Tech->>Srv: 2. Connect Power, BMC OOB port, and Provisioning NIC
    Tech->>Srv: 3. Power on server with PXE Boot
    Note over Srv,RW: Server requests DHCP on provisioning subnet
    RW->>Srv: 4. Serves iPXE and in-memory RedWolf Discovery Agent
    Srv->>Srv: 5. Discovery Agent boots into RAM
    Srv->>Srv: 6. Gathers CPU, platform DMI, RAM, NIC MACs, storage drives
    Srv->>BMC: 7. In-band KCS: sets vendor-compatible BMC credentials & enables DHCP
    BMC-->>Srv: 8. Queries assigned BMC IP address
    Srv->>RW: 9. Transmits full hardware inventory report via API
    RW->>Admin: 10. Server appears in GUI (Status: "Ready for Provisioning")
    Admin->>RW: 11. Selects OS (AlmaLinux / Debian) + target disk + root pass + network
    RW->>Srv: 12. Streams raw cloud image & injects NoCloud 'cidata' partition
    Srv->>Srv: 13. Reboots into production OS with Cloud-Init applied
```

### Step-by-Step Breakdown:

1. **Physical Server Mounting:**
   The technician mounts the server into the rack and connects 3 cables:
   - **a) Power:** Redundant Power Supplies (PSU1 / PSU2)
   - **b) BMC Port:** Dedicated Out-of-band (OOB) management RJ-45 port
   - **c) Provisioning Network:** Primary network interface (NIC 1)

2. **Zero-Touch PXE Boot:**
   The technician powers on the server. The node boots via PXE. RedWolf Core's managed `dnsmasq` serves the iPXE loader and streams the in-memory **RedWolf Discovery Agent** over fast HTTP.

3. **Autonomous Takeover & Vendor HAL Execution:**
   - The Discovery Agent automatically detects the system manufacturer via DMI.
   - **Hardware Inventory Discovery:**
     - CPU topology, cores, threads, and microarchitecture via `lscpu -J`
     - Platform DMI chassis model and Serial Number / Service Tag
     - Total RAM capacity and DIMM slot population
     - MAC addresses, link states, and PCI topology for all network cards
     - Storage drives enumerated by serial number, bus (NVMe, SAS, SATA, BOSS, SATADOM), and capacity via `lsblk -J`
   - **Vendor BMC Automation (In-Band via KCS `/dev/ipmi0`):**
     - Sets vendor-compatible administrator credentials (safe 14–16 character password policy compliant across Dell, Supermicro, and ASRock Rack).
     - Configures BMC interface mode to Dedicated management port (executing raw OEM commands where required for Supermicro/ASRock).
     - Enables DHCP on the BMC and queries the acquired BMC IP address.

4. **Real-time RedWolf GUI Dashboard:**
   - Full hardware inventory appears instantly in the Web GUI with the status **"Ready for Provisioning"**.
   - Direct web link to the server's BMC console (`https://<BMC_IP>`).

5. **Bare-Metal Cloud-Init OS Deployment:**
   - **Supported Distributions:** AlmaLinux 8, 9, 10; Debian 12 (Bookworm), 13 (Trixie).
   - **Deployment Pipeline:**
     - **Deterministic Disk Targeting:** Operator selects the exact target drive by serial number / bus (e.g. `Samsung PM9A3 1.92TB NVMe`).
     - **Image Streaming:** RedWolf streams compressed official generic cloud images (`.raw.zstd`) directly onto the disk.
     - **NoCloud Injection:** RedWolf formats a local `cidata` filesystem on the disk containing `user-data`, `meta-data`, and MAC-matched `network-config` (Netplan v2).
     - **Reboot:** Server reboots directly into production with Cloud-Init executing on bare metal.

---

## 📁 Repository Layout

```text
RedWolf/
├── AGENTS.md                  # Comprehensive AI guidelines & architecture context (English only)
├── GEMINI.md                  # Symlink to AI guidelines
├── README.md                  # Main project documentation (this file)
├── .agents/
│   └── rules/
│       ├── language-policy.md # Mandatory English-only enforcement rule
│       └── redwolf-domain.md  # Core domain rules for assistants
├── assets/
│   └── logo/                  # Vector SVG and high-resolution PNG brand assets
│       ├── redwolf-horizontal.svg       # Primary horizontal logo (light theme)
│       ├── redwolf-horizontal-dark.svg  # Horizontal logo for dark GUI
│       ├── redwolf-icon.svg             # Standalone wolf emblem / icon
│       ├── redwolf-logo.svg             # Full stacked logo
│       ├── redwolf-logo-dark.svg        # Full stacked logo dark
│       ├── favicon.ico / favicon.png    # Browser and application favicons
│       ├── index.html                   # Interactive brand asset gallery
│       └── *.png                        # Transparent PNG exports (32px to 1024px)
├── docs/
│   ├── ARCHITECTURE.md        # Modular multi-vendor system architecture document
│   └── SPECIFICATION.md       # Full engineering specification and HAL details
```

---

## 🎨 Visual Assets & Branding

Lossless vector SVGs and transparent PNGs are available in [`assets/logo/`](file:///home/dawid/RedWolf/assets/logo/):
* **Horizontal Dark Logo:** [`assets/logo/redwolf-horizontal-dark.svg`](file:///home/dawid/RedWolf/assets/logo/redwolf-horizontal-dark.svg) (for Web GUI navbar).
* **Icon / Emblem:** [`assets/logo/redwolf-icon.svg`](file:///home/dawid/RedWolf/assets/logo/redwolf-icon.svg) (favicon, avatars, collapsed sidebar).
* **Interactive Showcase:** View [`assets/logo/index.html`](file:///home/dawid/RedWolf/assets/logo/index.html).

---

## 🚀 Roadmap

- [x] Initial architectural design and multi-vendor specifications (Dell, Supermicro, ASRock Rack).
- [x] Vectorization and standardization of logo assets (SVG, PNG, dark/light themes).
- [x] Contributor and AI assistant guidelines (`AGENTS.md`, `.agents/rules/`).
- [ ] Implement RedWolf Core in Go (Chi API, SQLite, managed `dnsmasq` orchestration).
- [ ] Build multi-vendor `redwolf-discovery` in-memory bootable image (Alpine + enterprise NIC drivers + HAL scripts).
- [ ] Implement RedWolf Web GUI Dashboard with disk selector and live node states.
- [ ] Develop bare-metal raw image streaming and NoCloud `cidata` partition injector.
- [ ] Integration validation on Dell PowerEdge, Supermicro, and ASRock Rack hardware.

---

## 📄 License

RedWolf is developed as open-source software. License details will be published alongside the initial source release.
