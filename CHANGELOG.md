# Changelog

All notable changes to the RedWolf Bare-Metal & VM Automated Provisioning Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
