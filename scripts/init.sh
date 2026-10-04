#!/bin/sh
# ==============================================================================
# RedWolf Discovery & Bare-Metal Provisioning Engine
# In-Memory RAMdisk System Init Script
#
# Target Platforms:
#  - Dell PowerEdge (iDRAC 8/9 - R640, R740, R750)
#  - Supermicro (AMI MegaRAC BMC - X10, X11, X12, H11, H12)
#  - ASRock Rack (ASPEED AST2500/AST2600 - EPYCD8, ROMED8, B650D4)
#  - Enterprise Virtualization (KVM/QEMU, Proxmox, VMware ESXi)
# ==============================================================================

set -o pipefail

export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export HOME=/root

# 1. Mount virtual kernel filesystems
mount -t proc none /proc
mount -t sysfs none /sys
mount -t devtmpfs none /dev 2>/dev/null || mount -t tmpfs none /dev

mkdir -p /dev/pts /dev/shm /run /tmp /mnt/redwolf-target
mount -t devpts devpts /dev/pts
mount -t tmpfs tmpfs /dev/shm
mount -t tmpfs tmpfs /run

# Reopen standard streams to guarantee console visibility on screen
exec 0</dev/console 1>/dev/console 2>/dev/console

# Banner output
cat << 'EOF'
================================================================================
   🐺 RedWolf Enterprise Provisioning Engine - In-Memory Discovery Agent
================================================================================
EOF

# 2. Dynamic Hardware & Device Node Detection (mdev)
echo /sbin/mdev > /proc/sys/kernel/hotplug
mdev -s

# 3. Load Storage, IPMI, and Network Kernel Drivers
echo "[RedWolf Init] Loading hardware drivers..."
DRIVERS="
ipmi_si
ipmi_devintf
ipmi_msghandler
nvme
ahci
megaraid_sas
mpt3sas
smartpqi
sd_mod
ixgbe
i40e
bnxt_en
mlx5_core
tg3
e1000e
r8169
virtio_pci
virtio_net
virtio_blk
virtio_scsi
vmxnet3
vmw_pvscsi
mptspi
e1000
"

for drv in $DRIVERS; do
    modprobe "$drv" 2>&1 || true
done

# Coldplug uevents trigger and modalias auto-loading for all bus devices (PCI, USB, etc.)
if [ -d /sys/bus ]; then
    find /sys/devices -name uevent -exec sh -c 'echo add > "{}" 2>/dev/null' \; 2>/dev/null || true
    find /sys/bus -name modalias -exec sh -c 'cat "{}" 2>/dev/null | xargs -r modprobe 2>/dev/null' \; 2>/dev/null || true
    mdev -s
fi

# 4. Bring up physical network interfaces and acquire DHCP lease
echo "[RedWolf Init] Initializing network interfaces..."
for iface_path in /sys/class/net/*; do
    [ -e "$iface_path" ] || continue
    iface=$(basename "$iface_path")
    [ "$iface" = "lo" ] && continue

    echo "[RedWolf Init] Bringing up interface: $iface"
    ip link set "$iface" up 2>/dev/null || true
done

# Allow switch port negotiation and spanning tree (STP/RSTP) to settle
sleep 2

# Attempt DHCP lease acquisition on available interfaces
echo "[RedWolf Init] Requesting DHCP lease..."
dhcp_acquired=0
for iface_path in /sys/class/net/*; do
    [ -e "$iface_path" ] || continue
    iface=$(basename "$iface_path")
    [ "$iface" = "lo" ] && continue

    carrier=$(cat "$iface_path/carrier" 2>/dev/null || echo "0")
    if [ "$carrier" = "1" ] || [ "$carrier" = "" ]; then
        echo "[RedWolf Init] Requesting DHCP lease on $iface..."
        if udhcpc -i "$iface" -n -q -t 5 -s /usr/share/udhcpc/default.script 2>/dev/null; then
            dhcp_acquired=1
            echo "[RedWolf Init] Successfully acquired lease on $iface"
        fi
    fi
done

# Fallback DHCP pass if no carrier was initially reported
if [ "$dhcp_acquired" -eq 0 ]; then
    echo "[RedWolf Init] No carrier detected on primary interfaces; retrying all interfaces..."
    for iface_path in /sys/class/net/*; do
        [ -e "$iface_path" ] || continue
        iface=$(basename "$iface_path")
        [ "$iface" = "lo" ] && continue
        udhcpc -i "$iface" -n -q -t 5 -s /usr/share/udhcpc/default.script 2>/dev/null || true
    done
fi

# 5. Parse kernel command line for RedWolf Core server URL and debug flags
REDWOLF_SERVER=""
DEBUG_MODE=0

for param in $(cat /proc/cmdline 2>/dev/null); do
    case "$param" in
        redwolf.server=*)
            REDWOLF_SERVER="${param#redwolf.server=}"
            ;;
        redwolf_server=*)
            REDWOLF_SERVER="${param#redwolf_server=}"
            ;;
        redwolf.debug=1|redwolf_debug=1|debug)
            DEBUG_MODE=1
            ;;
    esac
done

# Fallback: auto-derive from network default route or nameserver
if [ -z "$REDWOLF_SERVER" ]; then
    GW_IP=$(ip route 2>/dev/null | awk '/default/ {print $3}' | head -n 1)
    if [ -z "$GW_IP" ]; then
        GW_IP=$(awk '/nameserver/ {print $2}' /etc/resolv.conf 2>/dev/null | head -n 1)
    fi
    if [ -n "$GW_IP" ]; then
        REDWOLF_SERVER="http://${GW_IP}:8080"
        echo "[RedWolf Init] Auto-detected RedWolf Core Server URL via gateway: $REDWOLF_SERVER"
    fi
fi

if [ -n "$REDWOLF_SERVER" ]; then
    echo "[RedWolf Init] RedWolf Core Server URL: $REDWOLF_SERVER"
    export REDWOLF_SERVER
fi

# 6. Execute RedWolf Discovery & Provisioning Agent
echo "[RedWolf Init] Launching RedWolf Discovery Agent..."
EXIT_CODE=0

if [ -x /usr/local/bin/redwolf-discovery ]; then
    if [ -n "$REDWOLF_SERVER" ]; then
        /usr/local/bin/redwolf-discovery -server "$REDWOLF_SERVER"
    else
        /usr/local/bin/redwolf-discovery
    fi
    EXIT_CODE=$?
    echo "[RedWolf Init] Discovery agent completed with exit code: $EXIT_CODE"
else
    echo "[RedWolf Init] CRITICAL: /usr/local/bin/redwolf-discovery not found or executable!"
    EXIT_CODE=1
fi

# 7. Debug Shell or System Shutdown / Reboot
if [ "$DEBUG_MODE" -eq 1 ] || [ "$EXIT_CODE" -ne 0 ]; then
    echo ""
    echo "[RedWolf Init] ================= DIAGNOSTIC DUMP ================="
    echo "[RedWolf Init] Kernel Command Line:"
    cat /proc/cmdline 2>/dev/null || true
    echo ""
    echo "[RedWolf Init] Routing Table:"
    ip route 2>/dev/null || true
    echo ""
    echo "[RedWolf Init] Network Interfaces:"
    ip -br link 2>/dev/null || ip link 2>/dev/null || true
    echo ""
    echo "[RedWolf Init] IP Addresses:"
    ip -br addr 2>/dev/null || ip addr 2>/dev/null || true
    echo ""
    echo "[RedWolf Init] Storage Block Devices:"
    lsblk 2>/dev/null || cat /proc/partitions 2>/dev/null || true
    echo ""
    echo "[RedWolf Init] Loaded Kernel Modules:"
    lsmod 2>/dev/null || true
    echo "[RedWolf Init] ==================================================="
    echo ""

    if [ "$DEBUG_MODE" -eq 1 ]; then
        echo "[RedWolf Init] Debug flag active. Spawning interactive rescue shell..."
        /bin/sh
    else
        echo "[RedWolf Init] Agent exited with non-zero code ($EXIT_CODE). Sleeping 30s before reboot..."
        sleep 30
    fi
fi

echo "[RedWolf Init] Flushing buffers and restarting host..."
sync
sleep 2
reboot -f
