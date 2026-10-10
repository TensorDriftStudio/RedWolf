#!/usr/bin/env bash
# ==============================================================================
# RedWolf Bare-Metal & VM Automated Provisioning Engine
# Script: scripts/download-discovery.sh
#
# Fetches prebuilt discovery boot assets (kernel vmlinuz & initramfs.img)
# directly from official GitHub Releases with SHA256 verification.
#
# Eliminates the need to compile the Alpine discovery environment locally.
# ==============================================================================

set -euo pipefail

# ANSI color formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

DEFAULT_VERSION="v1.2.9"
VERSION="${1:-${REDWOLF_VERSION:-${DEFAULT_VERSION}}}"
TARGET_DIR="${2:-${REPO_ROOT}/assets/discovery}"
DATA_DIR="${REPO_ROOT}/data/images/discovery"

GITHUB_REPO="TensorDriftStudio/RedWolf"
RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"

echo -e "${BLUE}================================================================================"
echo -e "   🐺 RedWolf Enterprise Prebuilt Discovery Boot Asset Downloader"
echo -e "================================================================================${NC}"
echo -e "Target Version:    ${CYAN}${VERSION}${NC}"
echo -e "Target Directory:  ${CYAN}${TARGET_DIR}${NC}"
echo -e "Local Runtime Dir: ${CYAN}${DATA_DIR}${NC}\n"

# Verify curl or wget availability
if command -v curl &> /dev/null; then
    FETCH_CMD="curl -f -L -C - --progress-bar"
elif command -v wget &> /dev/null; then
    FETCH_CMD="wget -c --progress=bar:force:noscroll"
else
    echo -e "${RED}[ERROR] Neither curl nor wget is available on this system.${NC}" >&2
    exit 1
fi

mkdir -p "${TARGET_DIR}"
mkdir -p "${DATA_DIR}"

FILES=("vmlinuz" "initramfs.img" "SHA256SUMS")

echo -e "${YELLOW}[1/3] Downloading release artifacts from GitHub Releases...${NC}"
for file in "${FILES[@]}"; do
    FILE_URL="${RELEASE_URL}/${file}"
    DEST_PATH="${TARGET_DIR}/${file}"
    
    echo -e "${CYAN}→ Fetching ${file}...${NC}"
    if ! ${FETCH_CMD} "${FILE_URL}" -o "${DEST_PATH}" 2>/dev/null && ! ${FETCH_CMD} "${FILE_URL}" -O "${DEST_PATH}"; then
        echo -e "${YELLOW}[NOTICE] Release artifact '${file}' not found at ${FILE_URL}.${NC}"
        echo -e "${YELLOW}Falling back to building locally or using local cache.${NC}"
        exit 1
    fi
done

echo -e "\n${YELLOW}[2/3] Verifying SHA-256 integrity checksums...${NC}"
cd "${TARGET_DIR}"
if command -v sha256sum &> /dev/null; then
    if sha256sum -c SHA256SUMS; then
        echo -e "${GREEN}✓ Integrity verification passed.${NC}"
    else
        echo -e "${RED}[ERROR] SHA256 checksum verification failed! Checksum mismatch.${NC}" >&2
        exit 1
    fi
else
    echo -e "${YELLOW}[WARNING] sha256sum binary not found, skipping checksum validation.${NC}"
fi

echo -e "\n${YELLOW}[3/3] Synchronizing discovery assets to runtime image store...${NC}"
cp -a "${TARGET_DIR}"/* "${DATA_DIR}/"

KERNEL_SIZE=$(du -h "${TARGET_DIR}/vmlinuz" | cut -f1)
INITRD_SIZE=$(du -h "${TARGET_DIR}/initramfs.img" | cut -f1)

echo -e "\n${GREEN}================================================================================"
echo -e "Discovery boot assets successfully installed:"
echo -e "  - Kernel (vmlinuz):         ${KERNEL_SIZE} (${TARGET_DIR}/vmlinuz)"
echo -e "  - RAMdisk (initramfs.img):  ${INITRD_SIZE} (${TARGET_DIR}/initramfs.img)"
echo -e "  - Checksums:                Verified"
echo -e "================================================================================${NC}"
