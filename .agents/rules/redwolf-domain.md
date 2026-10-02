# RedWolf Domain Rules

## Multi-Vendor Support
RedWolf supports bare-metal servers from **Dell PowerEdge**, **Supermicro**, **ASRock Rack**, and generic IPMI/VM platforms.

## Key Hardware Invariants
1. **BMC Compatibility & Safe Credential Escrow:**
   - Password limit: strictly 14 to 16 characters (to prevent MegaRAC/ASPEED 16-byte IPMI truncation on Supermicro/ASRock Rack).
   - Read-only discovery by default: do NOT overwrite BMC credentials without operator confirmation.
   - Dedicated management port mode query and configuration with 90-120s STP switch delay awareness.
2. **Storage Targeting:**
   - Never assume `/dev/sda`. Support NVMe (`/dev/nvmeXn1`), Dell BOSS, SATADOM, and SAS/SATA by Serial and immutable `/dev/disk/by-id/` path.
3. **Network Configuration:**
   - Always match network adapters by MAC address in Cloud-Init `network-config` templates (Netplan v2 modern routes or universal Cloud-Init v1 format).
4. **Bare-Metal Cloud-Init Deployment:**
   - Stream compressed official cloud raw images (`.raw.zstd`) directly to target drive using sparse transfer (`bmaptool` or `dd conv=sparse`).
   - Fix secondary GPT header boundary (`sgdisk -e`).
   - Mount target root partition in RAM and inject Cloud-Init NoCloud seed directly into `/var/lib/cloud/seed/nocloud/`.
   - Register NVRAM bootloader entry (`efibootmgr`) with local disk boot priority.
