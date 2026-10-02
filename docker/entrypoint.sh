#!/bin/sh
set -e

echo "🐺 Initializing RedWolf Core Appliance..."

# Environment defaults
REDWOLF_HTTP_PORT="${REDWOLF_HTTP_PORT:-8080}"
REDWOLF_PROVISIONING_INTERFACE="${REDWOLF_PROVISIONING_INTERFACE:-eth0}"
REDWOLF_DB_PATH="${REDWOLF_DB_PATH:-/var/lib/redwolf/db/redwolf.db}"
REDWOLF_IMAGE_DIR="${REDWOLF_IMAGE_DIR:-/var/lib/redwolf/images}"
REDWOLF_TFTP_DIR="${REDWOLF_TFTP_DIR:-/var/lib/redwolf/tftp}"
REDWOLF_CONF_DIR="/etc/redwolf"
REDWOLF_LOG_DIR="/var/log/redwolf"

export REDWOLF_HTTP_PORT REDWOLF_PROVISIONING_INTERFACE REDWOLF_DB_PATH REDWOLF_IMAGE_DIR REDWOLF_TFTP_DIR REDWOLF_CONF_DIR REDWOLF_LOG_DIR

# Create required runtime directories
mkdir -p "$(dirname "$REDWOLF_DB_PATH")" "$REDWOLF_IMAGE_DIR" "$REDWOLF_TFTP_DIR" "$REDWOLF_CONF_DIR" "$REDWOLF_LOG_DIR"

# Seed embedded discovery boot assets into persistent image directory if empty
if [ -d "/usr/share/redwolf/assets/discovery" ] && [ ! -d "$REDWOLF_IMAGE_DIR/discovery" ]; then
    echo "📦 Seeding discovery boot assets to $REDWOLF_IMAGE_DIR/discovery..."
    cp -a /usr/share/redwolf/assets/discovery "$REDWOLF_IMAGE_DIR/"
fi

# Seed embedded TFTP bootloaders (ipxe.efi, undionly.kpxe) if missing in persistent directory
if [ -d "/usr/share/redwolf/assets/tftp" ]; then
    echo "📦 Seeding TFTP bootstrap loaders into $REDWOLF_TFTP_DIR..."
    cp -n /usr/share/redwolf/assets/tftp/* "$REDWOLF_TFTP_DIR/" 2>/dev/null || cp -a /usr/share/redwolf/assets/tftp/* "$REDWOLF_TFTP_DIR/"
fi

# Detect host interface IP for dynamic iPXE URL if not explicitly provided
if [ -z "$REDWOLF_IP" ]; then
    REDWOLF_IP=$(ip -4 addr show "$REDWOLF_PROVISIONING_INTERFACE" 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -n 1 || echo "")
    if [ -z "$REDWOLF_IP" ]; then
        echo "⚠️  Warning: Could not detect IP on interface '$REDWOLF_PROVISIONING_INTERFACE'. Falling back to 127.0.0.1."
        REDWOLF_IP="127.0.0.1"
    else
        echo "📡 Detected Provisioning IP: $REDWOLF_IP on $REDWOLF_PROVISIONING_INTERFACE"
    fi
fi

# Render active dnsmasq.conf from template
sed -e "s/# interface=eth0/interface=$REDWOLF_PROVISIONING_INTERFACE/" \
    -e "s|# dhcp-boot=tag:ipxe,http://<REDWOLF_IP>:8080/boot.ipxe?mac=\${net0/mac}|dhcp-boot=tag:ipxe,http://$REDWOLF_IP:$REDWOLF_HTTP_PORT/boot.ipxe?mac=\${net0/mac}|" \
    /etc/redwolf/dnsmasq.conf.template > /etc/redwolf/dnsmasq.conf

echo "✅ dnsmasq configuration initialized at /etc/redwolf/dnsmasq.conf"

# Execute requested command or run default RedWolf binary
if [ $# -eq 0 ]; then
    set -- /usr/local/bin/redwolf
fi

exec "$@"
