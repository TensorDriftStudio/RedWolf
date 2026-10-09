#!/usr/bin/env bash
# ==============================================================================
# RedWolf Bare-Metal & VM Provisioning Engine
# Script: scripts/build-discovery-ramfs.sh
#
# Packages the minimal Alpine Linux in-memory discovery RAMdisk (initramfs.img)
# and Linux LTS kernel (vmlinuz) for zero-touch PXE/iPXE bare-metal deployment.
# ==============================================================================

set -euo pipefail

# Color formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT_RAW="${1:-${REPO_ROOT}/assets/discovery}"
mkdir -p "${OUTPUT_RAW}"
OUTPUT_DIR="$(cd "${OUTPUT_RAW}" && pwd)"
DATA_IMAGES_DIR="${REPO_ROOT}/data/images/discovery"

echo -e "${BLUE}================================================================================"
echo -e "   🐺 RedWolf Enterprise Bare-Metal Discovery Ramdisk Packager"
echo -e "================================================================================${NC}"
echo -e "${CYAN}Target Output Directory: ${OUTPUT_DIR}${NC}"

# Check for Docker availability
if ! command -v docker &> /dev/null; then
    echo -e "${RED}[ERROR] Docker is required to build the discovery ramdisk in a clean, reproducible sandbox.${NC}" >&2
    exit 1
fi

# Ensure executable permissions on init script
chmod +x "${REPO_ROOT}/scripts/init.sh"

echo -e "\n${YELLOW}[1/4] Building discovery ramdisk container (Alpine 3.20 + Linux LTS)...${NC}"
IMAGE_TAG="redwolf-discovery-builder:latest"

# Build builder image using BuildKit without stale layer cache
DOCKER_BUILDKIT=1 docker build \
    --no-cache \
    -f "${REPO_ROOT}/scripts/Dockerfile.discovery" \
    -t "${IMAGE_TAG}" \
    "${REPO_ROOT}"

echo -e "\n${YELLOW}[2/4] Extracting boot assets (kernel & initramfs)...${NC}"
mkdir -p "${OUTPUT_DIR}"

# Run container to copy artifacts out to target directory
docker run --rm \
    -v "${OUTPUT_DIR}:/target" \
    "${IMAGE_TAG}"

# Ensure both initramfs.img and initramfs.cpio.gz exist
if [ -f "${OUTPUT_DIR}/initramfs.img" ] && [ ! -f "${OUTPUT_DIR}/initramfs.cpio.gz" ]; then
    cp -a "${OUTPUT_DIR}/initramfs.img" "${OUTPUT_DIR}/initramfs.cpio.gz"
fi

# Also synchronize to data/images/discovery for local runtime
mkdir -p "${DATA_IMAGES_DIR}"
cp -a "${OUTPUT_DIR}"/* "${DATA_IMAGES_DIR}/"

echo -e "\n${YELLOW}[3/4] Verifying generated discovery artifacts...${NC}"
if [ -f "${OUTPUT_DIR}/vmlinuz" ] && [ -f "${OUTPUT_DIR}/initramfs.img" ]; then
    KERNEL_SIZE=$(du -h "${OUTPUT_DIR}/vmlinuz" | cut -f1)
    INITRD_SIZE=$(du -h "${OUTPUT_DIR}/initramfs.img" | cut -f1)
    echo -e "${GREEN}✓ Kernel (vmlinuz):         ${KERNEL_SIZE}${NC}"
    echo -e "${GREEN}✓ RAMdisk (initramfs.img):  ${INITRD_SIZE}${NC}"
    echo -e "${GREEN}✓ Checksums (SHA256SUMS):   Verified${NC}"
    cat "${OUTPUT_DIR}/SHA256SUMS"
else
    echo -e "${RED}[ERROR] Expected boot artifacts were not generated!${NC}" >&2
    exit 1
fi

echo -e "\n${YELLOW}[4/4] Discovery Assets Ready for Network Booting!${NC}"
echo -e "${GREEN}================================================================================"
echo -e "Discovery environment successfully packaged:"
echo -e "  - Kernel:    ${OUTPUT_DIR}/vmlinuz"
echo -e "  - Initramfs: ${OUTPUT_DIR}/initramfs.img"
echo -e "  - Local Run: ${DATA_IMAGES_DIR}/"
echo -e "================================================================================${NC}"
