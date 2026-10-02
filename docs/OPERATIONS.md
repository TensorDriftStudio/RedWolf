# RedWolf Bare-Metal & VM Provisioning Operations Guide

This document provides operational instructions, deployment runbooks, and enterprise architectural references for running the **RedWolf** bare-metal provisioning platform in production data centers and virtualized homelabs.

---

## 1. Zero-Touch Technician Runbook

### Physical Rack Deployment:
1. **Rack Mounting:** Secure the server (Dell PowerEdge R640/R740/R750, Supermicro X11/X12/H11/H12, or ASRock Rack EPYCD8/ROMED8) in the rack.
2. **Cabling (3 Cables Required):**
   - **Power:** Redundant AC power cables to PDU A and PDU B.
   - **Out-of-Band Management (BMC):** Dedicated IPMI / iDRAC RJ-45 port connected to the out-of-band management switch.
   - **Provisioning Network (NIC 1):** Primary 1GbE/10GbE/25GbE interface connected to the RedWolf provisioning VLAN / subnet.
3. **Power On:** Press the chassis power button or activate via PDU relay. Ensure the BIOS boot order is set to PXE/Network Boot as the default fallback.

---

## 2. In-Memory Discovery & Vendor HAL Automation

Once powered on, the node boots into the RedWolf Discovery RAMdisk:
1. **Network Boot Handshake:**
   - Client sends DHCP broadcast -> RedWolf managed `dnsmasq` responds with iPXE binary (`undionly.kpxe` or `ipxe.efi`).
   - iPXE queries `http://<REDWOLF_IP>:8080/boot.ipxe?mac=${net0/mac}`.
   - Core API renders dynamic iPXE script directing the client to `/assets/discovery/vmlinuz` and `/assets/discovery/initramfs.img`.
2. **Alpine RAMdisk Initialization (`scripts/init.sh`):**
   - Mounts `/proc`, `/sys`, `/dev` (devtmpfs), and `/run`.
   - Populates hardware nodes via `mdev -s`.
   - Modprobes storage drivers (`nvme`, `ahci`, `megaraid_sas`, `mpt3sas`, `smartpqi`), enterprise NIC drivers (`ixgbe`, `i40e`, `bnxt_en`, `mlx5_core`, `tg3`), and IPMI drivers (`ipmi_si`, `ipmi_devintf`).
   - Acquires DHCP lease on active interface via `udhcpc`.
   - Executes `/usr/local/bin/redwolf-discovery`.
3. **Hardware Cataloging & BMC In-Band Escrow:**
   - Queries DMI chassis, serial number, processor topology, RAM DIMMs, block drives, and NIC MACs.
   - Contacts local BMC via KCS (`/dev/ipmi0`):
     - Configures BMC port mode to **Dedicated** (via OEM raw command on Supermicro if necessary).
     - Verifies BMC network mode is DHCP and queries acquired BMC IP.
     - Automatically generates a 15-character RFC-compliant universal password (upper, lower, digits, symbols) safe for MegaRAC 16-byte KCS buffer limits.
     - Submits encrypted credentials to RedWolf Core AES-256-GCM Vault.
   - Node status transitions to `READY_FOR_PROVISIONING`.

---

## 3. Web Console Provisioning Workflow

1. **Authentication:**
   - Log in to RedWolf at `http://<REDWOLF_IP>:8080` using Local Admin credentials or corporate directory (Active Directory / OpenLDAP).
2. **Node Selection:**
   - Browse the server table. Filter by vendor (Dell, Supermicro, ASRock Rack) or status (`READY_FOR_PROVISIONING`).
   - Click a node row to open the **Hardware Telemetry Drawer**:
     - **Compute:** CPU model, socket/core/thread breakdown, total RAM, DIMM slots.
     - **Storage:** Block storage inventory with model, transport bus (NVMe, SAS, SATA), and capacity.
     - **Network:** Physical interfaces, MAC addresses, link carrier speeds.
     - **BMC Security:** Out-of-band management IP, port mode, and AES-256 Vault card with show/hide password toggle and password rotation trigger.
3. **Trigger Deployment:**
   - Click **Deploy Node** to launch the Provisioning Wizard.
   - Choose Target OS:
     - AlmaLinux 9 / AlmaLinux 8 / AlmaLinux 10
     - Debian 12 / Debian 13
   - Select the target storage drive deterministically by immutable identifier (`/dev/disk/by-id/...`).
   - Configure root credentials, authorized SSH public keys, and network addressing (DHCP or Static IP with VLAN tagging).
   - Click **Authorize & Start Provisioning**.

---

## 4. Bare-Metal OS Deployment Pipeline

When an authorized task is picked up by the node agent:
1. **Sparse Disk Streaming:**
   - Agent streams compressed cloud raw image (`.raw.zstd` or `.raw.gz`) directly to target disk.
   - 4 MiB buffer chunks evaluate zero blocks to skip physical writes, maximizing SSD/NVMe endurance and streaming throughput.
2. **Secondary GPT Boundary Relocation:**
   - Runs `sgdisk -e` to relocate the secondary GPT partition header to the physical end of the block device.
   - Notifies kernel to refresh partition boundaries via `partprobe`.
3. **NoCloud Cloud-Init Injection:**
   - Mounts the target root partition in RAM (`/mnt/redwolf-target`).
   - Writes `/var/lib/cloud/seed/nocloud/`:
     - `meta-data`: hostname and instance UUID.
     - `user-data`: root password, authorized SSH keys, growpart auto-resize directives.
     - `network-config`: Cloud-Init v2 network specification matching exact interface MAC addresses (`match: macaddress: ...`).
   - Safely unmounts root partition.
4. **UEFI Bootloader Registration:**
   - Invokes `efibootmgr` to register the OS EFI boot binary in NVRAM with top boot priority.
5. **Reboot into Production:**
   - Agent issues `reboot -f`.
   - BIOS boots into local NVRAM entry; Cloud-Init expands root filesystem on initial startup.
   - Node status in RedWolf transitions to `ACTIVE`.

---

## 5. Directory Service & Security Administration

Navigate to **Settings** in the top navigation bar to configure:
* **Authentication Mode:**
  - **Local Only:** Standalone credentials.
  - **OpenLDAP / FreeIPA:** RFC 4511 directory integration.
  - **Microsoft Active Directory:** Kerberos/NTLM/LDAPS (Port 636 or StartTLS 389).
* **Live Connection Diagnostic:**
  - Click **Test Directory Connection** to perform real-time bind verification, certificate chain validation, and search query latency checks.
* **PXE & Subnet Parameters:**
  - Configure provisioning interface, DHCP lease ranges, router gateways, and upstream DNS resolvers.

---

## 6. Build & Simulation Tooling

### Building Boot Assets:
```bash
# Builds Alpine Linux LTS kernel and initramfs archive
bash scripts/build-discovery-ramfs.sh
```

### Simulating Bare-Metal Nodes:
```bash
# Simulate 1 Dell PowerEdge R640 server
bash scripts/simulate-node.sh --vendor dell --count 1

# Simulate Supermicro and ASRock Rack servers
bash scripts/simulate-node.sh --vendor supermicro --count 1
bash scripts/simulate-node.sh --vendor asrock --count 1

# Boot an actual containerized QEMU virtual machine
bash scripts/simulate-node.sh --mode qemu --vendor dell
```
