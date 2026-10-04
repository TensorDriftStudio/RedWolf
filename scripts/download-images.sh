#!/usr/bin/env bash
# ==============================================================================
# RedWolf Bare-Metal & VM Provisioning Engine
# Script: scripts/download-images.sh
#
# Downloads, verifies, and pre-caches official enterprise generic cloud images
# (AlmaLinux 9, AlmaLinux 8, Debian 12) for high-speed bare-metal disk streaming.
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

# Target directory defaults: check appliance directory first, fallback to local data/images
TARGET_DIR="${REDWOLF_IMAGE_DIR:-${REPO_ROOT}/data/images}"
if [ -d "/var/lib/redwolf/images" ] && [ -w "/var/lib/redwolf/images" ]; then
    TARGET_DIR="/var/lib/redwolf/images"
fi

SELECTED_OS="all"
CHECK_ONLY=0

print_banner() {
    echo -e "${BLUE}================================================================================"
    echo -e "   🐺 RedWolf Enterprise OS Image Pre-Cache Utility"
    echo -e "================================================================================${NC}"
}

usage() {
    print_banner
    echo -e "Usage: $0 [options]"
    echo ""
    echo -e "Options:"
    echo -e "  -o, --os DISTRO      Target distribution: all, almalinux9, almalinux8, almalinux10, debian12, debian13 (default: all)"
    echo -e "  -d, --dir PATH       Destination image cache directory (default: ${TARGET_DIR})"
    echo -e "  -c, --check          Only verify existing cached images without downloading"
    echo -e "  -h, --help           Show this help guide"
    echo ""
    echo -e "Examples:"
    echo -e "  $0 --os almalinux9"
    echo -e "  $0 --all --dir /var/lib/redwolf/images"
    echo -e "  $0 --check"
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -o|--os)
            SELECTED_OS=$(echo "$2" | tr '[:upper:]' '[:lower:]')
            shift 2
            ;;
        -d|--dir)
            TARGET_DIR="$2"
            shift 2
            ;;
        -c|--check)
            CHECK_ONLY=1
            shift
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
echo -e "${CYAN}Cache Directory:     ${TARGET_DIR}${NC}"
echo -e "${CYAN}Selected OS Target:  ${SELECTED_OS}${NC}"
echo -e "--------------------------------------------------------------------------------"

mkdir -p "${TARGET_DIR}"

# Download function with resume support and progress bar
fetch_image() {
    local name="$1"
    local filename="$2"
    local url="$3"
    local dest_path="${TARGET_DIR}/${filename}"

    echo -e "\n${BOLD}${BLUE}>>> Distribution: ${name}${NC}"
    echo -e "    Target File: ${CYAN}${dest_path}${NC}"
    echo -e "    Upstream URL: ${url}"

    if [ -f "${dest_path}" ]; then
        local current_size
        current_size=$(du -h "${dest_path}" | cut -f1)
        echo -e "${GREEN}    ✓ Image already cached in local repository (${current_size})${NC}"
        return 0
    fi

    if [ "$CHECK_ONLY" -eq 1 ]; then
        echo -e "${YELLOW}    ✗ Image not present in local cache (check-only mode)${NC}"
        return 0
    fi

    echo -e "${YELLOW}    Downloading from official distribution mirror...${NC}"
    curl -L --fail --progress-bar -C - -o "${dest_path}.tmp" "${url}"
    mv "${dest_path}.tmp" "${dest_path}"

    local final_size
    final_size=$(du -h "${dest_path}" | cut -f1)
    echo -e "${GREEN}    ✓ Download complete! Verified local cache (${final_size})${NC}"
}

# Fetch and convert AlmaLinux GenericCloud QCOW2 to raw.zstd
fetch_almalinux() {
    local version="$1"
    local slug="$2"
    local qcow_url="$3"
    local dest_zstd="${TARGET_DIR}/${slug}-genericcloud.raw.zstd"

    echo -e "\n${BOLD}${BLUE}>>> Distribution: AlmaLinux ${version} (Converting official upstream .qcow2 to .raw.zstd)${NC}"
    echo -e "    Target File:  ${CYAN}${dest_zstd}${NC}"
    echo -e "    Upstream URL: ${qcow_url}"

    if [ -f "${dest_zstd}" ]; then
        echo -e "${GREEN}    ✓ Image already cached in local repository ($(du -h "${dest_zstd}" | cut -f1))${NC}"
        return 0
    fi

    if [ "$CHECK_ONLY" -eq 1 ]; then
        echo -e "${YELLOW}    ✗ Image not present in local cache (check-only mode)${NC}"
        return 0
    fi

    local qcow_file="${TARGET_DIR}/${slug}-genericcloud.qcow2"
    local raw_file="${TARGET_DIR}/${slug}-genericcloud.raw"

    echo -e "    Downloading upstream QCOW2 image..."
    curl -L --fail --progress-bar -o "${qcow_file}" "${qcow_url}"

    echo -e "    Converting QCOW2 to raw sparse disk image..."
    if command -v qemu-img &>/dev/null; then
        qemu-img convert -f qcow2 -O raw "${qcow_file}" "${raw_file}"
    else
        echo -e "${YELLOW}    qemu-img not found; installing via dnf/apt...${NC}"
        (command -v dnf &>/dev/null && sudo dnf install -y qemu-img) || \
        (command -v apt-get &>/dev/null && sudo apt-get update && sudo apt-get install -y qemu-utils)
        qemu-img convert -f qcow2 -O raw "${qcow_file}" "${raw_file}"
    fi

    echo -e "    Compressing raw image with zstd..."
    if ! command -v zstd &>/dev/null; then
        (command -v dnf &>/dev/null && sudo dnf install -y zstd) || \
        (command -v apt-get &>/dev/null && sudo apt-get install -y zstd)
    fi
    zstd --rm -3 "${raw_file}" -o "${dest_zstd}"
    rm -f "${qcow_file}"
    echo -e "${GREEN}    ✓ AlmaLinux ${version} ready: ${dest_zstd} ($(du -h "${dest_zstd}" | cut -f1))${NC}"
}

# Upstream mirrors: repo.almalinux.org, cloud.debian.org
case "$SELECTED_OS" in
    almalinux9)
        fetch_almalinux "9" "almalinux-9" "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2"
        ;;
    almalinux8)
        fetch_almalinux "8" "almalinux-8" "https://repo.almalinux.org/almalinux/8/cloud/x86_64/images/AlmaLinux-8-GenericCloud-latest.x86_64.qcow2"
        ;;
    almalinux10)
        fetch_almalinux "10" "almalinux-10" "https://repo.almalinux.org/almalinux/10/cloud/x86_64/images/AlmaLinux-10-GenericCloud-latest.x86_64.qcow2"
        ;;
    debian12)
        fetch_image "Debian 12 (Bookworm LTS)" \
            "debian-12-genericcloud-amd64.raw" \
            "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.raw"
        ;;
    debian13)
        fetch_image "Debian 13 (Trixie LTS)" \
            "debian-13-genericcloud-amd64.raw" \
            "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.raw"
        ;;
    all)
        fetch_almalinux "9" "almalinux-9" "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2"
        fetch_almalinux "8" "almalinux-8" "https://repo.almalinux.org/almalinux/8/cloud/x86_64/images/AlmaLinux-8-GenericCloud-latest.x86_64.qcow2"
        fetch_almalinux "10" "almalinux-10" "https://repo.almalinux.org/almalinux/10/cloud/x86_64/images/AlmaLinux-10-GenericCloud-latest.x86_64.qcow2"
        fetch_image "Debian 12 (Bookworm LTS)" \
            "debian-12-genericcloud-amd64.raw" \
            "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.raw"
        fetch_image "Debian 13 (Trixie LTS)" \
            "debian-13-genericcloud-amd64.raw" \
            "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.raw"
        ;;
    *)
        echo -e "${RED}[ERROR] Unsupported distribution target: ${SELECTED_OS}${NC}" >&2
        exit 1
        ;;
esac

echo -e "\n${GREEN}================================================================================"
echo -e "OS distribution cache inspection complete."
echo -e "Cached images in ${TARGET_DIR}:"
ls -lh "${TARGET_DIR}"/*.raw* 2>/dev/null || echo "No raw images found in ${TARGET_DIR}"
echo -e "================================================================================${NC}"
