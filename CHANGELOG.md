# Changelog

All notable changes to the RedWolf Bare-Metal & VM Automated Provisioning Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
