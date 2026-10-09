# ==============================================================================
# Stage 1: Production Web Dashboard Assets (Compiled with Vite inside Docker)
# ==============================================================================
FROM node:22-alpine AS frontend-builder
WORKDIR /app

# Copy dependency specifications first to leverage Docker layer caching
COPY web/package.json web/package-lock.json ./
RUN npm ci

# Copy frontend source files and compile production bundle
COPY web/ ./
RUN npm run build

# ==============================================================================
# Stage 2: Build Backend Engine (Go 1.23+)
# ==============================================================================
FROM golang:1.23-alpine AS backend-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

ARG VERSION=v1.2.5
ARG GIT_COMMIT=release
ARG BUILD_DATE=""

RUN CGO_ENABLED=0 go build -ldflags="-s -w \
    -X github.com/tensordriftstudio/redwolf/internal/version.Version=${VERSION} \
    -X github.com/tensordriftstudio/redwolf/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/tensordriftstudio/redwolf/internal/version.BuildDate=${BUILD_DATE}" \
    -o /bin/redwolf ./cmd/redwolf && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/redwolf-discovery ./cmd/redwolf-discovery

# ==============================================================================
# Stage 3: Production Appliance Container (Alpine Linux)
# ==============================================================================
FROM alpine:3.20

LABEL org.opencontainers.image.title="RedWolf Bare-Metal Provisioning Engine"
LABEL org.opencontainers.image.description="Multi-vendor bare-metal discovery and operating system provisioning appliance"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Install production networking daemons, utilities, and certificate authorities
RUN apk add --no-cache \
    dnsmasq \
    iproute2 \
    ipmitool \
    util-linux \
    ca-certificates \
    tzdata \
    curl \
    zstd \
    gptfdisk \
    qemu-img

# Prepare application and runtime data paths
RUN mkdir -p \
    /var/lib/redwolf/db \
    /var/lib/redwolf/images \
    /var/lib/redwolf/tftp \
    /etc/redwolf \
    /var/log/redwolf \
    /usr/share/redwolf/web

# Copy compiled frontend dashboard assets
COPY --from=frontend-builder /app/dist /usr/share/redwolf/web

# Copy compiled RedWolf Go binaries
COPY --from=backend-builder /bin/redwolf /usr/local/bin/redwolf
COPY --from=backend-builder /bin/redwolf-discovery /usr/local/bin/redwolf-discovery
COPY --from=backend-builder /bin/redwolf-discovery /var/lib/redwolf/images/redwolf-discovery

# Copy discovery kernel, initramfs, TFTP bootloaders, and media assets
COPY assets/ /usr/share/redwolf/assets/

# Copy container runtime templates and entrypoint
COPY docker/dnsmasq.conf.template /etc/redwolf/dnsmasq.conf.template
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Persistent volumes for SQLite database and raw OS images
VOLUME ["/var/lib/redwolf/db", "/var/lib/redwolf/images"]

# DHCP (67/udp), TFTP (69/udp), HTTP Web Dashboard & Asset Streamer (8080/tcp)
EXPOSE 67/udp 69/udp 8080/tcp

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/redwolf"]
