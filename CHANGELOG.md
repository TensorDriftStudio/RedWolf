# Changelog

All notable changes to the RedWolf Bare-Metal & VM Automated Provisioning Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.2.4] - 2026-10-04 (Storage Autodetection & SELinux Relabeling Release)

### Fixed
- **Filesystem Autodetection & Multi-FS Mount Resilience:**
  - Introduced `MountTargetFilesystem` with multi-stage filesystem probing (`blkid -s TYPE`, `lsblk -no FSTYPE`) and explicit `-t <fstype>` mount arguments.
  - Added kernel module autoloading for `xfs`, `ext4`, and `btrfs` alongside dynamic trial fallback across supported filesystem drivers when BusyBox `mount` lacks superblock probing.
  - Populated `/etc/filesystems` (`ext4`, `xfs`, `btrfs`, `vfat`, `*`) in Alpine discovery RAMdisk (`scripts/init.sh`) to support native kernel filesystem cycling.
  - Replaced raw mount calls in `syncMemberESPs` and `extractImageToLVM` with `MountTargetFilesystem`.
- **SELinux Direct Credential Injection & Local Console Login:**
  - Added automatic creation of `/.autorelabel` in target root filesystem during `DirectInjectSecurityCredentials`.
  - Resolved local KVM / serial console login failures on AlmaLinux and RHEL distributions caused by missing `shadow_t` SELinux contexts on `/etc/shadow` under `SELINUX=enforcing`.
- **Partition Selection & EFI Resolution:**
  - Hardened `findRootPartitionFromJSON` and `detectRootPartNumber` to prioritize verified Linux root filesystem types and ignore `/boot` and `ESP` partitions.
  - Corrected EFI partition resolution in `detectEFIPartition` and `parseEFIPartitionFromJSON`, ensuring partition 1 EFI system partitions are correctly detected and registered in UEFI NVRAM.

- **UI Streamlining & Version Dynamic Synchronization:**
  - Removed redundant `"LIVE"` daemon connection status indicator from the sidebar footer.
  - Removed `"Scan Network"` trigger button from the top navigation bar.
  - Removed `"Subnet: ..."` badge from the Servers view header.
  - Synchronized frontend dashboard version dynamically with `GET /api/version`, eliminating stale hardcoded version tags in the Sidebar and LoginView.
  - Updated `docker/entrypoint.sh` to force-sync (`cp -f -a`) embedded discovery assets and TFTP bootloaders into persistent volumes upon container startup.

---

## [1.2.3] - 2026-10-04 (Storage & Partitioning Resilience Release)

### Fixed
- **Debian GenericCloud Root Partition Resolution:**
  - Made root partition detection distribution-aware in `findRootPartition`, `findRootPartitionFromJSON`, and `fallbackPartitionPath`.
  - Prioritized partition 1 (`ext4`) for Debian 12 / Debian 13 genericcloud images, preventing erroneous selection of stale partition 4 from prior RHEL/AlmaLinux installations.
  - Hardened partition discovery heuristic against minimal initramfs environments where `FSType` may not be populated prior to filesystem mounting.
- **Disk Pre-Wiping in Standard Streaming Engine:**
  - Integrated `WipeTargetDisk` into `executeStandardDeployment` prior to image streaming to clear existing partition signatures via `wipefs`, execute `blkdiscard`, and zero out the first 32 MiB and last 32 MiB of the target drive.
  - Eliminated stale primary and secondary GPT headers, ghost partitions, and kernel partition conflicts during bare-metal and VM reprovisioning.
- **Distribution-Aware GPT Root Partition & EFI Expansion:**
  - Updated `RepairGPTHeader`, `ExpandRootPartition`, and `detectRootPartNumber` to accept target `osType`, correctly expanding partition 1 to 100% capacity for Debian while preserving partition 4 expansion for AlmaLinux/RHEL.
  - Updated `detectEFIPartition` to default to partition 15 for Debian and partition 2 for AlmaLinux during UEFI NVRAM bootloader registration.

---

## [1.2.2] - 2026-10-04 (Security & Provisioning Release)

### Fixed
- **Root Password Provisioning & Direct Shadow Injection:**
  - Added `DirectInjectSecurityCredentials` to directly compute and write POSIX/glibc-compliant SHA-512 crypt hashes (`$6$...`) into `/etc/shadow` on the mounted target root filesystem, eliminating dependency on first-boot cloud-init execution.
  - Injected OpenSSH drop-in configuration (`/etc/ssh/sshd_config.d/99-redwolf-root.conf`) with `PermitRootLogin yes` and `PasswordAuthentication yes` to overcome upstream distro policies (`prohibit-password` on Debian, `no` on AlmaLinux) that reject root password logins over SSH.
  - Explicitly configured `disable_root: false` and `passwd: "$6$..."` in Cloud-Init `user-data` to prevent upstream cloud-init defaults from locking the root account.
  - Added direct placement of operator SSH public keys into `/root/.ssh/authorized_keys` with strict `0600` permissions.
- **LVM Device Node Resolution & Storage Initialization:**
  - Added `resolveLVMDeviceNode` ensuring active block device path discovery (`/dev/<vg>/<lv>` and `/dev/mapper/<vg>-<lv>`) in Alpine minimal discovery environment.
  - Added module autoloading for `dm_mod`, `xfs`, `ext4`, `vfat`, and `loop` in discovery `init.sh` with automatic `/dev/mapper/control` node creation.
  - Hardened swap volume creation fallback to 1GB if proportional allocation exceeds volume group free extents.
- **Web UI Wizard Credentials Feedback:**
  - Added real-time character count and validation feedback for the root password in Step 4 of the Provisioning Wizard.
  - Added root authentication status to the Deployment Manifest Summary card.
  - Enforced client-side guards on the "Start Provisioning" action when credentials are underspecified.

---

## [1.2.1] - 2026-10-04 (Patch Release)

### Fixed
- **Multi-Tab Authentication & Fleet Telemetry Ingestion:**
  - Resolved `authorization bearer token or session cookie required` error on fresh tab login by linking `useNodes` lifecycle to `isAuthenticated`.
  - Inhibited unauthenticated background requests to `/api/nodes`, `/api/settings`, and `/api/templates` prior to sign-in.
  - Added cross-tab authentication synchronization via `storage` events and automatic redirection on session expiration.
  - Persisted tokens before state transitions to eliminate race conditions during authorization checks.
- **Debian Cloud Image Streaming & Compression:**
  - Added dynamic zstd compression for raw Debian generic cloud images with user-agent mirror compliance.
  - Prevented race conditions with locked image directory resolution.

### Added
- **Docker Hub Documentation:**
  - Added comprehensive `DOCKERHUB.md` enterprise overview with architecture diagrams and container configuration guides.

---

## [1.2.0] - 2026-10-04 (Storage & Platform Release)

### Added
- **Full Logical Volume Manager (LVM) Provisioning Engine:**
  - Automated GPT partitioning (`512MB ESP`, `1024MB /boot`, `8e00 LVM PV`).
  - Dynamic Volume Group (`vg_system`) creation with proportional logical volume layout (`lv_root`, `lv_var`, `lv_home`, `lv_tmp`, `lv_swap`).
  - Hierarchical mounting, root filesystem loop copy, UUID-matched `/etc/fstab` generation, and EFI bootloader registration.
  - Interactive drive-capacity allocator in Web UI with real-time capacity meters and one-click "Auto-fit to Drive" scaling.
- **Cross-Vendor Software RAID (`mdadm`) Provisioning:**
  - Support for RAID 0, RAID 1, RAID 5, and RAID 10 configurations across multiple storage drives.
  - Dual `mdadm.conf` generation (`/etc/mdadm.conf` for AlmaLinux/RHEL and `/etc/mdadm/mdadm.conf` for Debian/Ubuntu).
  - Redundant FAT32 ESP formatting with `dosfstools` and multi-drive EFI bootloader replication.
- **Dynamic Partition Auto-Expansion:**
  - Deterministic GPT root partition boundary repair (`parted resizepart`, `partprobe`) on official cloud raw images.
  - Online filesystem growth support (`xfs_growfs` for XFS on AlmaLinux, `resize2fs` for ext4 on Debian).
- **Modern LTS Operating System Catalog:**
  - Added official **AlmaLinux 10** and **Debian 13 LTS** cloud raw images with direct automated download pipelines.
  - Pre-flight image availability checks in Provisioning Wizard preventing deployment of uncached images.
- **Enterprise Discovery Asset Automation:**
  - Added `scripts/download-discovery.sh` to download prebuilt discovery ramdisks from official GitHub Releases.
  - Added automatic release pipeline (`.github/workflows/release.yml`) for discovery boot assets and multi-registry container publishing.
  - Added runtime fallback in `entrypoint.sh` to fetch prebuilt discovery assets if absent.

---

## [1.1.0] - 2026-10-02 (Enterprise Release)

### Added
- **Enterprise Semantic Versioning Framework:**
  - Added `internal/version/version.go` providing structured version info (`Version`, `GitCommit`, `BuildDate`, `Edition`, `GoVersion`, `Platform`).
  - Added `GET /api/version` REST API endpoint exposing build and edition metadata for automation and telemetry.
  - Linked Docker build args (`VERSION`, `GIT_COMMIT`, `BUILD_DATE`) to Go linker flags (`-ldflags`).
  - Added root `VERSION` and `CHANGELOG.md` for deterministic CI/CD release tagging.
- **Dynamic DHCP & Network Service Management:**
  - Integrated dynamic `dhcp-range`, `dhcp-boot`, and DNS settings in `dnsmasq` adapter with live subprocess reload on configuration changes.
- **Automated OS Image Management:**
  - Built-in asynchronous cloud raw image downloader (`internal/service/image_catalog.go`) with live progress tracking in the Settings UI.
- **Fleet Node Lifecycle Management:**
  - Added interactive node state reset (`POST /api/nodes/{id}/reset`) to re-trigger discovery or clear stuck deployment state.
  - Added node decommission (`DELETE /api/nodes/{id}`) removing ephemeral hardware inventory records.

### Changed
- **Branding & UI Consistency:**
  - Replaced temporary lightning bolt icon with official RedWolf SVG logo (`assets/logo/redwolf-icon.svg` and `redwolf-horizontal-dark.svg`).
  - Standardized application title to **RedWolf GUI** across HTML page title, navigation header, and footer.
  - Updated all badges to reflect `v1.1.0 Enterprise`.

### Security
- **Credential Hygiene:**
  - Removed all default credential hints and quick-fill buttons (`admin / admin123`) from the production login view.
  - Hardened BMC credential escrow and rotation mechanisms.
  - Added `ipmitool` package to production appliance container for direct hardware BMC management.

---

## [1.0.0] - 2026-10-01 (Initial MVP Baseline)

### Added
- Initial core architecture for bare-metal discovery and provisioning.
- Embedded Alpine Linux discovery agent (`redwolf-discovery`) with hardware inventory (`dmidecode`, `lsblk`, `ipmitool`, `ethtool`).
- Multi-Vendor HAL for Dell PowerEdge, Supermicro, and ASRock Rack.
- iPXE/TFTP bootloader integration (`ipxe.efi`, `undionly.kpxe`).
- SQLite storage engine with WAL journal mode and busy timeout concurrency.
- React 19 + TypeScript + Vite + TailwindCSS operator console.
- Multi-identity provider authentication (Local Admin, OpenLDAP, Active Directory).
