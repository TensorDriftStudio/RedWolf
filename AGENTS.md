# AGENTS.md - RedWolf Project Guidelines and AI Context

This file serves as the primary source of architectural context, domain knowledge, and development rules for AI assistants and contributors working on **RedWolf**.

---

## 🚨 MANDATORY LANGUAGE POLICY: ENGLISH ONLY

> **CRITICAL RULE FOR ALL AI ASSISTANTS AND CONTRIBUTORS:**
> **Do NOT use the Polish language (or any language other than English) anywhere in this repository or in conversations.**
> - All documentation, guides, and specifications must be in **English only**.
> - All code, comments, docstrings, variable/function names must be in **English only**.
> - All commit messages, PR descriptions, and git history must be in **English only**.
> - All UI text, error messages, and API payloads must be in **English only**.
> - All assistant responses and explanations to the user must be conducted strictly in **English**.

---

## 🐺 What is RedWolf?

**RedWolf** is a modern, open-source bare-metal and virtual machine (VM) automated provisioning platform designed for simplicity, speed, and zero-touch deployment in data center server racks and homelabs.

### Primary Use Case (Data Center Technician Workflow):
1. **Physical Server Installation:** A technician mounts an enterprise server (reference platform: **Dell PowerEdge R640**) into the rack and connects:
   - Redundant Power Supplies (PSU1 / PSU2)
   - Dedicated iDRAC / BMC port to the Out-of-band (OOB) management network
   - First network port (NIC1 / eth0 / onboard 1GbE/10GbE LOM) to the provisioning network
2. **PXE Boot:** The technician powers on the server and initiates a network boot (PXE), or the server boots via PXE by default if disks are unpartitioned.
3. **RedWolf Autonomous Takeover:**
   - RedWolf manages integrated **DHCP** and **TFTP/HTTP Boot** services on the dedicated provisioning network.
   - Upon PXE boot, the node loads and runs a lightweight in-memory micro-OS (**RedWolf Discovery Agent** / ramdisk).
   - The Discovery Agent automatically:
     - **Collects Hardware Inventory:** platform/chassis model (e.g. Dell PowerEdge R640, Service Tag), CPU details (model, sockets, cores, threads), RAM (total capacity, DIMM slots configuration), MAC addresses of all detected network interfaces, and disk storage devices.
     - **Automates BMC / iDRAC Configuration:** via the local in-band KCS interface using `ipmitool` / OpenIPMI / Redfish, configures a secure administrator username and password, enables DHCP mode on the BMC interface, and retrieves the assigned BMC IP address.
4. **GUI Presentation:**
   - The Discovery Agent sends a telemetry report via HTTP API to RedWolf Core.
   - The server appears in real-time in the RedWolf Web GUI with the status **"Ready for Provisioning"**.
5. **Operating System Deployment (Cloud-Init):**
   - The administrator selects the target operating system in the GUI:
     - **AlmaLinux 8, 9, 10**
     - **Debian 12 (Bookworm), 13 (Trixie)**
   - The administrator configures deployment parameters:
     - Disk partitioning scheme (LVM, software RAID, NVMe/SSD/HDD target drives, swap)
     - Root password and authorized SSH public keys
     - Target production network configuration (Static IP / DHCP, gateway, DNS, LACP bonding, VLANs)
   - RedWolf renders cloud-init (`user-data`, `meta-data`, `network-config`), streams the base OS image to disk, applies configuration, and reboots the server into a fully operational production state.

---

## 🏛️ System Architecture

1. **`redwolf-core` (Backend & Orchestrator):**
   - Finite State Machine (FSM): `DISCOVERED` -> `COLLECTING_TELEMETRY` -> `READY_FOR_PROVISIONING` -> `PROVISIONING` -> `ACTIVE` / `FAILED`.
   - Built-in/managed network services:
     - **DHCP Server:** provisioning subnet address allocation and PXE options (Next-Server, Bootfile Name for iPXE / UEFI).
     - **TFTP / HTTP Boot Server:** serving iPXE binaries, Linux kernels (`vmlinuz`), and discovery initramfs.
   - REST / WebSocket API for agents and the frontend.
   - Cloud-Init metadata templating engine (`user-data`, `meta-data`, `network-config` v2).

2. **`redwolf-discovery` (Discovery Agent & Live Boot Image):**
   - Minimal in-memory Linux micro-OS (initramfs based on Alpine Linux or minimal Buildroot/Debian kernel).
   - Discovery daemon extracting data using `dmidecode`, `lscpu`, `/sys`, `ip`, `ethtool`.
   - In-band BMC configuration modules (`ipmi_si`, `ipmi_devintf`, `ipmitool`).
   - Reports telemetry back to `redwolf-core` via HTTP POST (JSON payload).

3. **`redwolf-ui` (Web GUI Dashboard):**
   - Modern, reactive web application (SPA).
   - Real-time updates for discovered nodes.
   - Provisioning wizard: OS selection (AlmaLinux 8/9/10, Debian 12/13), storage layout, credentials, network setup.
   - Visual telemetry: CPU, RAM, storage, network interfaces, and iDRAC status.

4. **`redwolf-profiles` & `templates`:**
   - Cloud-init, kickstart, and preseed templates.
   - Hardware profiles (Dell PowerEdge R640 / iDRAC 9 defaults).

---

## 🛠️ Technical Guidelines for AI Assistants

1. **Strict Language Requirement:**
   - Every file created or updated must use **English exclusively**.
2. **Dell PowerEdge & BMC Automation:**
   - BMC IP addresses might not be preconfigured on newly mounted servers. Use the local KCS interface (`ipmitool lan set ...`, `ipmitool user set ...`) from the live discovery environment.
   - All BMC configuration scripts must be idempotent.
3. **PXE & Cloud-Init Reliability:**
   - Target images must use proper Cloud-Init configuration (`user-data`, `meta-data`, `network-config` version 2).
   - Passwords must be securely hashed (e.g. SHA-512 crypt `$6$`).
4. **Visual Assets:**
   - Always use official vector SVGs from [`assets/logo/`](file:///home/dawid/RedWolf/assets/logo/).
