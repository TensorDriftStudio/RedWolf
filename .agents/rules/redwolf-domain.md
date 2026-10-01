# RedWolf Domain Rules

## Multi-Vendor Support
RedWolf supports bare-metal servers from **Dell PowerEdge**, **Supermicro**, **ASRock Rack**, and generic IPMI/VM platforms.

## Key Hardware Invariants
1. **BMC Compatibility:**
   - Password limit: 14 to 16 characters (to prevent MegaRAC/ASPEED 16-byte IPMI truncation on Supermicro/ASRock Rack).
   - Enforce Dedicated BMC management port mode via raw IPMI where required.
2. **Storage Targeting:**
   - Never assume `/dev/sda`. Support NVMe (`/dev/nvmeXn1`), Dell BOSS, SATADOM, and SAS/SATA by Serial and `/dev/disk/by-id/`.
3. **Network Configuration:**
   - Always match network adapters by MAC address in Cloud-Init `network-config` v2 templates.
4. **Bare-Metal Cloud-Init Deployment:**
   - Stream generic cloud raw images directly to target drive using `zstd`.
   - Inject the Cloud-Init NoCloud `cidata` partition.
