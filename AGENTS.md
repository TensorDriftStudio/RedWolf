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

**RedWolf** is a modern, open-source bare-metal and virtual machine (VM) automated provisioning platform designed for zero-touch deployment in enterprise data center racks and homelabs.

### Target Platforms (Multi-Vendor):
1. **Dell PowerEdge:** 13th, 14th, 15th, 16th Gen (R640, R740, R750 with iDRAC 8/9).
2. **Supermicro:** Intel & AMD platforms (X10, X11, X12, H11, H12 with AMI MegaRAC BMC).
3. **ASRock Rack:** Server motherboards (EPYCD8, ROMED8, B650D4 with ASPEED AST2500/AST2600).
4. **Generic x86_64 & Virtualization:** Standards-compliant IPMI 2.0 / Redfish servers and VMs (KVM, Proxmox, VMware ESXi).

---

## 🏛️ Multi-Vendor Hardware Abstraction Layer (HAL) Rules

1. **Vendor Detection:**
   - Detect manufacturer dynamically via DMI (`dmidecode -s system-manufacturer`).
   - Trigger vendor-specific profile: `DellProfile`, `SupermicroProfile`, `ASRockRackProfile`, or `GenericIPMIProfile`.

2. **BMC Automation & Universal Credential Standards:**
   - **Password Length Safety:** RFC standard IPMI 2.0 KCS password buffers are limited to **16 bytes** on Supermicro and ASRock Rack. Generated passwords must strictly be **14 to 16 characters** long with mixed case, digits, and symbols (`!@#$%^&*`). Do NOT generate >16 char passwords on MegaRAC BMCs.
   - **BMC Network Mode:** Ensure the management interface is set to **Dedicated** port mode (executing Supermicro raw IPMI command `raw 0x30 0x70 0x0c 1 0` if required) so DHCP works regardless of BIOS defaults.
   - **Polling Backoff:** Implement retry loops (3s interval, up to 30s) when polling for the assigned BMC IP address.

3. **Deterministic Storage Provisioning (No Blind `/dev/sda`):**
   - Disks must be inventoried using machine-readable JSON: `lsblk -J -b -o NAME,SIZE,TYPE,MODEL,SERIAL,WWN,TRAN,ROTA`.
   - Never hardcode `/dev/sda`. Support NVMe drives (`/dev/nvme0n1`), Dell BOSS RAID cards, Supermicro SATADOM, and SAS/SATA drives.
   - Image streaming engine writes to explicitly operator-selected disk by immutable identifier (`/dev/disk/by-id/...` or verified device name).

4. **MAC-Matched Cloud-Init Networking:**
   - Network card kernel names vary across vendors (`eno1` on Dell, `enp3s0f0` on Supermicro, `eth0` on ASRock).
   - All Cloud-Init `network-config` version 2 templates must bind to **MAC addresses** (`match: macaddress: "..."`).

5. **Bare-Metal Cloud-Init Streaming (`cidata`):**
   - Stream compressed official cloud raw images (`.raw.zstd`) directly to the target storage drive.
   - Automatically format and write the NoCloud **`cidata`** partition on the target disk containing `user-data`, `meta-data`, and `network-config`.
   - Register NVRAM boot entry using `efibootmgr` on UEFI platforms.

---

## 🛠️ Technology Stack Standards

* **Core Backend:** Go 1.23+ with Chi router, SQLite (WAL mode), and managed `dnsmasq` subprocess for PXE/iPXE boot loop prevention.
* **Discovery Agent:** Alpine Linux initramfs packaging enterprise network drivers (`ixgbe`, `i40e`, `bnxt_en`, `tg3`), `ipmitool`, `lsblk`, `lscpu`, and static Go agent binary.
* **Frontend:** React / Vue 3 + TypeScript + Vite + TailwindCSS embedded directly in Go binary (`embed.FS`).
* **Containerization:** Docker / Podman using `network_mode: host` for Layer-2 DHCP broadcast visibility.
