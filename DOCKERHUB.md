# 🐺 RedWolf — Bare-Metal & VM Automated Provisioning Appliance

[![Docker Image Version](https://img.shields.io/docker/v/wolverandover/redwolf?sort=semver&style=flat-square&color=crimson&label=Version)](https://hub.docker.com/r/wolverandover/redwolf)
[![Docker Pulls](https://img.shields.io/docker/pulls/wolverandover/redwolf?style=flat-square&color=2496ED)](https://hub.docker.com/r/wolverandover/redwolf)
[![Docker Image Size](https://img.shields.io/docker/image-size/wolverandover/redwolf/latest?style=flat-square&color=4c1)](https://hub.docker.com/r/wolverandover/redwolf)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://github.com/TensorDriftStudio/RedWolf/blob/main/LICENSE)
[![GitHub Repository](https://img.shields.io/badge/GitHub-TensorDriftStudio%2FRedWolf-181717.svg?style=flat-square&logo=github)](https://github.com/TensorDriftStudio/RedWolf)

**RedWolf** is a modern, open-source bare-metal and virtual machine automated provisioning platform designed for zero-touch deployment in enterprise data centers, colocation facilities, and homelabs.

This official Docker image provides the complete, pre-packaged RedWolf appliance: high-performance **Go 1.23+** core engine, embedded **React 19** dashboard, integrated **PXE/iPXE/TFTP** server, dynamic **dnsmasq** DHCP engine, and prebuilt in-memory **Alpine Linux discovery boot assets**. No local compilation or external runtime dependencies required.

---

## ⚡ Key Capabilities

* **Multi-Vendor Hardware Abstraction Layer (HAL):**
  * **Dell PowerEdge:** 13th, 14th, 15th, and 16th Gen (R640, R740, R750 with iDRAC 8/9).
  * **Supermicro:** Intel and AMD platforms (X10, X11, X12, H11, H12 with AMI MegaRAC BMC).
  * **ASRock Rack:** Server motherboards (EPYCD8, ROMED8, B650D4 with ASPEED AST2500/AST2600).
  * **Generic x86_64 & Virtualization:** Standards-compliant IPMI 2.0 / Redfish servers and VMs (KVM, Proxmox, VMware ESXi).
* **Zero-Touch Hardware Discovery:** Boots bare-metal nodes into an ephemeral Alpine RAMdisk, collects granular inventory (CPUs, RAM DIMMs, network interfaces, storage drives), configures dedicated BMC networking, and escrows credentials into an encrypted AES-256-GCM vault.
* **Deterministic Enterprise Storage Layouts:**
  * **Standard Partitioning:** Auto-expanding GPT root partition with online filesystem expansion (`xfs_growfs` / `resize2fs`).
  * **LVM Engine:** Volume Group and Logical Volume allocation (`/`, `/var`, `/home`, `/tmp`, `swap`) with dynamic UI auto-fit.
  * **Software RAID (`mdadm`):** Automated RAID 0, 1, 5, 10 array creation with dual `mdadm.conf` sync and redundant EFI bootloader replication.
  * **Safety First:** Drives are addressed via immutable symlinks (`/dev/disk/by-id/...`), eliminating blind `/dev/sda` corruption.
* **Cloud-Init Streaming:** Direct raw cloud image deployment (`.raw.zstd`) with NoCloud seed injection and MAC-matched network definitions.
* **Modern OS Catalog:** Out-of-the-box support for AlmaLinux 9/10 LTS, Debian 12/13 LTS, and Ubuntu 22.04/24.04 LTS.

---

## 🚀 Quick Start

> **Important Networking Note:** RedWolf acts as a Layer-2 PXE/DHCP boot server. It requires `--network host` (or a dedicated Macvlan interface) and elevated network capabilities (`NET_ADMIN`, `NET_BIND_SERVICE`, `NET_RAW`) to observe and respond to DHCP broadcast packets on your provisioning subnet.

### Option 1: Docker Compose (Recommended)

Save the following configuration as `docker-compose.yml`:

```yaml
services:
  redwolf:
    image: wolverandover/redwolf:latest
    container_name: redwolf-core
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_ADMIN
      - NET_BIND_SERVICE
      - NET_RAW
    environment:
      - REDWOLF_HTTP_PORT=8080
      - REDWOLF_PROVISIONING_INTERFACE=eth0
      - REDWOLF_DB_PATH=/var/lib/redwolf/db/redwolf.db
      - REDWOLF_IMAGE_DIR=/var/lib/redwolf/images
      - REDWOLF_TFTP_DIR=/var/lib/redwolf/tftp
    volumes:
      - ./data/db:/var/lib/redwolf/db
      - ./data/images:/var/lib/redwolf/images
      - ./data/tftp:/var/lib/redwolf/tftp
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "5"
```

Start the appliance:

```bash
docker compose up -d
docker compose logs -f redwolf
```

### Option 2: Standalone `docker run`

```bash
# Create local data directories for persistence
mkdir -p data/db data/images data/tftp

# Run RedWolf appliance
docker run -d \
  --name redwolf-core \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  --cap-add NET_BIND_SERVICE \
  --cap-add NET_RAW \
  -e REDWOLF_HTTP_PORT=8080 \
  -e REDWOLF_PROVISIONING_INTERFACE=eth0 \
  -e REDWOLF_DB_PATH=/var/lib/redwolf/db/redwolf.db \
  -e REDWOLF_IMAGE_DIR=/var/lib/redwolf/images \
  -e REDWOLF_TFTP_DIR=/var/lib/redwolf/tftp \
  -v $(pwd)/data/db:/var/lib/redwolf/db \
  -v $(pwd)/data/images:/var/lib/redwolf/images \
  -v $(pwd)/data/tftp:/var/lib/redwolf/tftp \
  wolverandover/redwolf:latest
```

---

## 🌐 Accessing the Console

Once running, access the web console in your browser:

```text
http://<SERVER_IP>:8080
```

1. Log in using local administrator credentials or configure corporate LDAP / Active Directory.
2. Open **Settings → PXE Engine & Network** to define your provisioning subnet, DHCP pool, and gateway.
3. Open **Settings → OS Images** to download base cloud raw images with one click.
4. Rack and power on your target servers with PXE network boot enabled.

---

## ⚙️ Configuration & Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `REDWOLF_HTTP_PORT` | `8080` | TCP port for Web Dashboard, REST API, and image streaming. |
| `REDWOLF_PROVISIONING_INTERFACE` | `eth0` | Host physical interface facing the provisioning VLAN/subnet. |
| `REDWOLF_SERVER_URL` | Auto-detected | Base HTTP URL used by target nodes to pull iPXE scripts and OS images. |
| `REDWOLF_DB_PATH` | `/var/lib/redwolf/db/redwolf.db` | Path to persistent SQLite database (WAL mode). |
| `REDWOLF_IMAGE_DIR` | `/var/lib/redwolf/images` | Storage path for downloaded cloud OS images and discovery ramdisks. |
| `REDWOLF_TFTP_DIR` | `/var/lib/redwolf/tftp` | TFTP root path serving iPXE bootloaders (`ipxe.efi`, `undionly.kpxe`). |

---

## 📁 Persistent Volume Mounts

| Container Path | Purpose |
| :--- | :--- |
| `/var/lib/redwolf/db` | Persistent SQLite database containing node inventories, settings, and state machine transitions. |
| `/var/lib/redwolf/images` | Cached OS cloud images (`.raw.zstd`) and in-memory discovery RAMdisk artifacts. |
| `/var/lib/redwolf/tftp` | TFTP bootstrap files and iPXE firmware binaries. |

---

## 🔌 Exposed Network Ports

When using `--network host`, the following ports are serviced directly on the host interface:

| Port | Protocol | Purpose |
| :--- | :--- | :--- |
| `67` | UDP | DHCP Server (PXE discovery & address allocation via dnsmasq) |
| `69` | UDP | TFTP Server (iPXE initial bootstrap loader) |
| `8080` | TCP | RedWolf Web UI, REST API, iPXE chainloading scripts & OS image streaming |

---

## 🏷️ Image Tags

* `wolverandover/redwolf:latest` — Tracks the latest stable release.
* `wolverandover/redwolf:1.3.3` — Specific immutable release version.

---

## 📚 Documentation & Support

* **Source Code & Documentation:** [github.com/TensorDriftStudio/RedWolf](https://github.com/TensorDriftStudio/RedWolf)
* **Issue Tracker:** [Report bugs or feature requests](https://github.com/TensorDriftStudio/RedWolf/issues)
* **License:** Apache License 2.0
