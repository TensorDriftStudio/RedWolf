#!/usr/bin/env bash
# ==============================================================================
# RedWolf Bare-Metal & VM Provisioning Engine
# Script: scripts/simulate-node.sh
#
# Simulates physical multi-vendor bare-metal servers (Dell PowerEdge, Supermicro,
# ASRock Rack) booting into the RedWolf discovery environment.
#
# Supports two simulation engines:
#  1. mock (Default): High-speed synthetic enterprise agent simulating hardware
#     telemetry, BMC inventory, task polling, and sparse streaming deployment.
#  2. qemu: Containerized QEMU virtual machine booting the real Alpine discovery
#     kernel (vmlinuz) and RAMdisk (initramfs.img).
# ==============================================================================

set -euo pipefail

# ANSI Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default parameter values
SERVER_URL="http://127.0.0.1:8080"
VENDOR="dell"
SIM_MODE="mock"
NODE_COUNT=1
DRY_RUN=0

print_banner() {
    echo -e "${BLUE}================================================================================"
    echo -e "   🐺 RedWolf Multi-Vendor Bare-Metal Node Simulator"
    echo -e "================================================================================${NC}"
}

usage() {
    print_banner
    echo -e "Usage: $0 [options]"
    echo ""
    echo -e "Options:"
    echo -e "  -v, --vendor VENDOR    Target vendor profile: dell, supermicro, asrock (default: dell)"
    echo -e "  -s, --server URL       RedWolf Core API URL (default: http://127.0.0.1:8080)"
    echo -e "  -m, --mode MODE        Simulation mode: mock, qemu (default: mock)"
    echo -e "  -n, --count N          Number of simulated nodes to register (default: 1)"
    echo -e "  -h, --help             Show this help guide"
    echo ""
    echo -e "Examples:"
    echo -e "  $0 --vendor dell --count 2"
    echo -e "  $0 --vendor supermicro --server http://192.168.1.100:8080"
    echo -e "  $0 --mode qemu --vendor asrock"
    exit 0
}

# Parse command line flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        -v|--vendor)
            VENDOR=$(echo "$2" | tr '[:upper:]' '[:lower:]')
            shift 2
            ;;
        -s|--server)
            SERVER_URL="$2"
            shift 2
            ;;
        -m|--mode)
            SIM_MODE=$(echo "$2" | tr '[:upper:]' '[:lower:]')
            shift 2
            ;;
        -n|--count)
            NODE_COUNT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo -e "${RED}[ERROR] Unknown option: $1${NC}" >&2
            usage
            ;;
    esac
done

print_banner
echo -e "${CYAN}Target Server:     ${SERVER_URL}${NC}"
echo -e "${CYAN}Vendor Profile:    ${VENDOR}${NC}"
echo -e "${CYAN}Simulation Mode:   ${SIM_MODE}${NC}"
echo -e "${CYAN}Node Instances:    ${NODE_COUNT}${NC}"
echo -e "--------------------------------------------------------------------------------"

# Verify API connectivity
echo -ne "${YELLOW}Checking connection to RedWolf API (${SERVER_URL}/healthz)... ${NC}"
if curl -s -f -m 3 "${SERVER_URL}/healthz" > /dev/null 2>&1; then
    echo -e "${GREEN}ONLINE${NC}"
else
    echo -e "${RED}OFFLINE / UNREACHABLE${NC}"
    echo -e "${YELLOW}[WARN] RedWolf Core is not reachable at ${SERVER_URL}. Continuing in mock registration mode.${NC}"
fi

# ==============================================================================
# Simulation Engine: QEMU Containerized Bare-Metal Boot
# ==============================================================================
run_qemu_node() {
    local instance_idx="$1"
    echo -e "\n${BLUE}>>> Launching Containerized QEMU Virtual Server Node [Node #${instance_idx}]...${NC}"

    local kernel_path="${REPO_ROOT}/assets/discovery/vmlinuz"
    local initrd_path="${REPO_ROOT}/assets/discovery/initramfs.img"

    if [ ! -f "$kernel_path" ] || [ ! -f "$initrd_path" ]; then
        echo -e "${RED}[ERROR] Discovery boot assets not found at ${REPO_ROOT}/assets/discovery/!${NC}"
        echo -e "${YELLOW}Please run: bash scripts/build-discovery-ramfs.sh first.${NC}"
        exit 1
    fi

    local mac_suffix=$(printf "%02x" "$instance_idx")
    local mac="52:54:00:12:34:${mac_suffix}"
    local disk_img="/tmp/redwolf-sim-disk-${instance_idx}.raw"

    echo -e "${CYAN}Creating virtual 20GB sparse NVMe disk: ${disk_img}${NC}"
    truncate -s 20G "$disk_img"

    echo -e "${GREEN}Booting Alpine LTS Kernel directly into RedWolf Discovery RAMdisk...${NC}"
    echo -e "${CYAN}MAC: ${mac} | SMBIOS Vendor: ${VENDOR}${NC}"

    local smbios_vendor="Dell Inc."
    local smbios_model="PowerEdge R640"
    if [ "$VENDOR" = "supermicro" ]; then
        smbios_vendor="Supermicro"
        smbios_model="SYS-1029P-WTR"
    elif [ "$VENDOR" = "asrock" ]; then
        smbios_vendor="ASRockRack"
        smbios_model="EPYCD8-2T"
    fi

    docker run --rm -it \
        --net=host \
        -v "${kernel_path}":/boot/vmlinuz:ro \
        -v "${initrd_path}":/boot/initramfs.img:ro \
        -v "${disk_img}":/disk.raw \
        alpine:3.20 sh -c "
            apk add --no-cache qemu-system-x86_64 >/dev/null 2>&1
            qemu-system-x86_64 \
                -m 2048 \
                -smp 2 \
                -nographic \
                -kernel /boot/vmlinuz \
                -initrd /boot/initramfs.img \
                -append 'console=ttyS0 redwolf.server=${SERVER_URL} redwolf.debug=0' \
                -smbios type=1,manufacturer='${smbios_vendor}',product='${smbios_model}',serial='RW-SIM-${instance_idx}' \
                -drive file=/disk.raw,format=raw,if=none,id=nvm \
                -device nvme,serial=SIMNVME${instance_idx},drive=nvm \
                -netdev user,id=net0 \
                -device virtio-net-pci,netdev=net0,mac=${mac}
        "
}

# ==============================================================================
# Simulation Engine: High-Speed Synthetic Enterprise Node Agent
# ==============================================================================
run_mock_node() {
    local instance_idx="$1"
    local seed=$(( 100 + instance_idx ))
    local mac_suffix=$(printf "%02x" "$instance_idx")
    local bmc_suffix=$(printf "%02x" $(( instance_idx + 10 )))

    local vendor_name="Dell Inc."
    local model_name="PowerEdge R640"
    local serial_num="DELL-SIM-$(printf "%04d" "$instance_idx")"
    local bmc_mode="Dedicated (iDRAC 9 Enterprise)"
    local bmc_ip="192.168.0.$(( 120 + instance_idx ))"
    local primary_mac="ac:1f:6b:88:99:${mac_suffix}"
    local secondary_mac="ac:1f:6b:88:99:${bmc_suffix}"

    if [ "$VENDOR" = "supermicro" ]; then
        vendor_name="Supermicro"
        model_name="SYS-1029P-WTR (X11DPi-NT)"
        serial_num="SMCI-SIM-$(printf "%04d" "$instance_idx")"
        bmc_mode="Dedicated (AMI MegaRAC IPMI 2.0)"
        bmc_ip="192.168.0.$(( 140 + instance_idx ))"
        primary_mac="00:25:90:ea:71:${mac_suffix}"
        secondary_mac="00:25:90:ea:71:${bmc_suffix}"
    elif [ "$VENDOR" = "asrock" ]; then
        vendor_name="ASRockRack"
        model_name="EPYCD8-2T"
        serial_num="ASRK-SIM-$(printf "%04d" "$instance_idx")"
        bmc_mode="Dedicated (ASPEED AST2500)"
        bmc_ip="192.168.0.$(( 160 + instance_idx ))"
        primary_mac="70:85:c2:d3:44:${mac_suffix}"
        secondary_mac="70:85:c2:d3:44:${bmc_suffix}"
    fi

    echo -e "\n${BOLD}${BLUE}>>> Starting Simulated Hardware Node [${instance_idx}/${NODE_COUNT}]: ${vendor_name} ${model_name}${NC}"
    echo -e "    Serial: ${serial_num} | Boot MAC: ${primary_mac} | BMC IP: ${bmc_ip}"

    # Build telemetry JSON payload matching RedWolf domain.ServerNode specification
    local telemetry_payload
    telemetry_payload=$(cat << EOF
{
  "vendor": "${vendor_name}",
  "model": "${model_name}",
  "serialNumber": "${serial_num}",
  "firmwareMode": "UEFI",
  "biosVersion": "2.12.1",
  "status": "DISCOVERING",
  "cpu": {
    "model": "Intel(R) Xeon(R) Gold 6248R CPU @ 3.00GHz",
    "sockets": 2,
    "coresPerSocket": 24,
    "threadsPerSocket": 48,
    "totalThreads": 96,
    "arch": "x86_64"
  },
  "memory": {
    "totalBytes": 137438953472,
    "totalHuman": "128.0 GiB",
    "slotsUsed": 4,
    "slotsTotal": 24,
    "type": "DDR4 ECC RDIMM",
    "speedMhz": 3200
  },
  "storage": [
    {
      "name": "nvme0n1",
      "path": "/dev/nvme0n1",
      "byId": "/dev/disk/by-id/nvme-SAMSUNG_MZQL2960HCJR-00A07_${serial_num}",
      "sizeBytes": 1024209543168,
      "sizeHuman": "953.9 GiB",
      "type": "disk",
      "transport": "nvme",
      "model": "SAMSUNG MZQL2960HCJR-00A07",
      "serial": "S5XJNA0N${instance_idx}00123"
    },
    {
      "name": "sda",
      "path": "/dev/sda",
      "byId": "/dev/disk/by-id/scsi-35000039828100${instance_idx}a",
      "sizeBytes": 1920383410176,
      "sizeHuman": "1.74 TiB",
      "type": "disk",
      "transport": "sas",
      "model": "DELL PERC H730P",
      "serial": "PERC730P${serial_num}"
    }
  ],
  "nics": [
    {
      "name": "eno1",
      "mac": "${primary_mac}",
      "speedMbps": 25000,
      "carrier": true,
      "isBoot": true,
      "driver": "i40e",
      "pciSlot": "0000:18:00.0"
    },
    {
      "name": "eno2",
      "mac": "${secondary_mac}",
      "speedMbps": 25000,
      "carrier": false,
      "isBoot": false,
      "driver": "i40e",
      "pciSlot": "0000:18:00.1"
    }
  ],
  "bmc": {
    "vendor": "${vendor_name}",
    "ip": "${bmc_ip}",
    "mac": "${secondary_mac}",
    "dhcp": true,
    "channel": 1,
    "portMode": "Dedicated",
    "credentialsUpdated": false
  }
}
EOF
)

    echo -ne "    Registering telemetry with RedWolf Core... "
    local resp
    resp=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "${telemetry_payload}" \
        "${SERVER_URL}/api/nodes/telemetry" 2>/dev/null || echo "")

    if [ -z "$resp" ] || [[ "$resp" == *"error"* && "$resp" != *"id"* ]]; then
        echo -e "${RED}FAILED${NC} (Check server connection)"
        return 1
    fi

    local node_id
    node_id=$(echo "$resp" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4 || echo "")
    if [ -z "$node_id" ]; then
        node_id="node-${primary_mac//:/}"
    fi

    echo -e "${GREEN}REGISTERED!${NC} (Node ID: ${CYAN}${node_id}${NC})"
    echo -e "    Entering background deployment poller for MAC: ${CYAN}${primary_mac}${NC}"

    # Launch background polling process to pick up operator deployment orders
    (
        local poll_count=0
        while [ $poll_count -lt 300 ]; do # Poll for up to 25 minutes
            sleep 4
            poll_count=$(( poll_count + 1 ))

            local task_json
            task_json=$(curl -s --no-keepalive -m 4 "${SERVER_URL}/api/nodes/task/by-mac/${primary_mac}" 2>/dev/null || echo "")

            if [ -n "$task_json" ] && [[ "$task_json" == *"taskId"* || "$task_json" == *"task_id"* ]]; then
                local task_id target_os target_drive
                task_id=$(echo "$task_json" | grep -o '"taskId":"[^"]*' | head -n 1 | cut -d'"' -f4)
                target_os=$(echo "$task_json" | grep -o '"os":"[^"]*' | head -n 1 | cut -d'"' -f4)
                target_drive=$(echo "$task_json" | grep -o '"targetDrivePath":"[^"]*' | head -n 1 | cut -d'"' -f4)

                echo -e "\n${BOLD}${GREEN}>>> [Node ${node_id}] Received deployment order! OS: ${target_os} -> ${target_drive}${NC}"

                # Simulate bare-metal streaming steps
                local stages=(
                    "15:Streaming OS Image...:Transferring sparse raw blocks"
                    "35:Streaming OS Image...:Sparse write throughput 420 MB/s"
                    "65:Streaming OS Image...:Flushing disk write cache"
                    "75:Relocating GPT Header:Updating secondary partition table"
                    "85:Injecting Cloud-Init:Mounting rootfs and writing NoCloud seed"
                    "95:Configuring NVRAM:Registering UEFI bootloader with efibootmgr"
                    "100:Active in Production:Deployment complete. Rebooting into OS."
                )

                for stage in "${stages[@]}"; do
                    IFS=":" read -r pct title desc <<< "$stage"
                    sleep 2
                    curl -s --no-keepalive -m 4 -X POST \
                        -H "Content-Type: application/json" \
                        -d "{\"progress\":${pct},\"stage\":\"${title}\",\"log\":\"${desc}\"}" \
                        "${SERVER_URL}/api/nodes/${node_id}/progress" > /dev/null 2>&1 || true
                    echo -e "    [${node_id}] Progress: ${pct}% - ${title}"
                done

                echo -e "${BOLD}${GREEN}>>> [Node ${node_id}] PROVISIONING COMPLETE! Node is ACTIVE in Production.${NC}\n"
                break
            fi
        done
    ) </dev/null >/dev/null 2>&1 &
    disown || true
}

# ==============================================================================
# Main Orchestration Loop
# ==============================================================================
if [ "$SIM_MODE" = "qemu" ]; then
    run_qemu_node 1
else
    for (( i=1; i<=NODE_COUNT; i++ )); do
        run_mock_node "$i"
    done

    echo -e "\n${GREEN}================================================================================"
    echo -e "Simulated bare-metal nodes are running and cataloged in RedWolf!"
    echo -e "Open the Web Dashboard at: ${CYAN}${SERVER_URL}${NC}"
    echo -e "You can select any node in the list and click 'Deploy Node' to watch"
    echo -e "the live end-to-end bare-metal provisioning workflow in real time."
    echo -e "================================================================================${NC}"
fi
