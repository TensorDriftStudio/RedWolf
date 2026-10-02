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
   - **Polling Backoff:** Implement retry loops (3s interval, up to 90-120s) with link-carrier verification to accommodate upstream switch STP/RSTP negotiation when polling for the assigned BMC IP address.

3. **Deterministic Storage Provisioning (No Blind `/dev/sda`):**
   - Disks must be inventoried using machine-readable JSON: `lsblk -J -b -o NAME,SIZE,TYPE,MODEL,SERIAL,WWN,TRAN,ROTA`.
   - Never hardcode `/dev/sda`. Support NVMe drives (`/dev/nvme0n1`), Dell BOSS RAID cards, Supermicro SATADOM, and SAS/SATA drives.
   - Image streaming engine writes to explicitly operator-selected disk by immutable identifier (`/dev/disk/by-id/...` or verified device name).

4. **MAC-Matched Cloud-Init Networking:**
   - Network card kernel names vary across vendors (`eno1` on Dell, `enp3s0f0` on Supermicro, `eth0` on ASRock).
   - All Cloud-Init `network-config` version 2 templates must bind to **MAC addresses** (`match: macaddress: "..."`).

5. **Bare-Metal Cloud-Init Streaming & NoCloud Injection:**
   - Stream compressed official cloud raw images (`.raw.zstd`) directly to the target storage drive with sparse transfer (`bmaptool` / `dd conv=sparse`).
   - Fix secondary GPT header boundary (`sgdisk -e`), mount target root partition in RAM, and write Cloud-Init NoCloud seed (`/var/lib/cloud/seed/nocloud/`) containing `user-data`, `meta-data`, and `network-config`.
   - Register NVRAM boot entry with disk boot priority using `efibootmgr` on UEFI platforms.

---

## 🛠️ Technology Stack Standards

* **Core Backend:** Go 1.23+ with Chi router, SQLite (WAL mode), and managed `dnsmasq` subprocess for multi-architecture PXE/iPXE boot loop prevention.
* **Discovery Agent:** Alpine Linux initramfs packaging enterprise network drivers (`ixgbe`, `i40e`, `bnxt_en`, `tg3`, `mlx5_core`), storage drivers (`megaraid_sas`, `mpt3sas`, `smartpqi`, `nvme`), GNU `util-linux`, `ipmitool`, `sgdisk`, `xfsprogs`, `e2fsprogs`, and static Go agent binary.
* **Frontend:** React + TypeScript + Vite + TailwindCSS embedded directly in Go binary (`embed.FS`).
* **Containerization:** Docker / Podman using `network_mode: host` (or `macvlan`) for Layer-2 DHCP broadcast visibility.

---

## 💎 Mandatory Enterprise Engineering Standards for AI Assistants

All AI assistants and human contributors must strictly adhere to the following enterprise rules (detailed in [`.agents/rules/enterprise-standards.md`](file:///home/dawid/RedWolf/.agents/rules/enterprise-standards.md)):

1. **No Stubs, Mocks, or Incomplete Code:**
   - Never output placeholder comments such as `// TODO: implement later` or pass-through dummy code. All edge cases, errors, and cancellation paths must be fully resolved.
2. **Idiomatic Go 1.23+ Architecture:**
   - **Layered Clean Architecture:** Strict separation between pure Domain Models, Ports/Interfaces, and Infrastructure Adapters.
   - **Structured Logging (`log/slog`):** Never use `fmt.Println` or unstructured loggers. Every log entry must include contextual key-value pairs (`node_id`, `mac`, `drive`).
   - **Context & Cancellation:** `ctx context.Context` is mandatory as the first parameter for all I/O, database, and process calls. Long-running routines must monitor `ctx.Done()`.
   - **Subprocess Security:** Never invoke shells (`sh -c` or `bash -c`). Use `exec.CommandContext(ctx, binary, args...)` with discrete argument vectors to prevent command injection.
   - **SQLite Concurrency:** SQLite must use WAL mode (`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`). State transitions must execute in explicit transactions.
3. **Strict TypeScript & UI Reliability:**
   - `any` and unconstrained type casts are strictly prohibited.
   - Deeply nested telemetry properties must use defensive optional chaining (`?.`) and fallback defaults.
   - State management must be encapsulated in reusable custom hooks.
4. **Idempotency & Hardware Safety:**
   - All agent routines and provisioning steps must be idempotent.
   - Image deployment must target immutable device symlinks (`/dev/disk/by-id/...`).
   - Telemetry collection must be strictly non-destructive and read-only. Credential changes and disk formatting require explicit operator authorization.
