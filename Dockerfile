FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/vpn-manager ./cmd/vpn-manager

FROM alpine:3.21

# Install runtime deps + sing-box
RUN apk add --no-cache ca-certificates tzdata iproute2 iptables nftables wget

# Download sing-box binary
ARG TARGETARCH
RUN case "$TARGETARCH" in \
      amd64) arch="amd64" ;; \
      arm64|aarch64) arch="arm64" ;; \
      arm) arch="armv7" ;; \
      *) arch="amd64" ;; \
    esac; \
    wget -qO /tmp/sing-box.tar.gz \
      "https://github.com/SagerNet/sing-box/releases/download/v1.11.7/sing-box-1.11.7-linux-${arch}.tar.gz" && \
    tar xzf /tmp/sing-box.tar.gz -C /tmp/ && \
    mv /tmp/sing-box-*/sing-box /usr/local/bin/sing-box && \
    rm -rf /tmp/sing-box-* /tmp/sing-box.tar.gz && \
    chmod +x /usr/local/bin/sing-box

# Create vpn user and config directories
RUN addgroup -S vpn && adduser -S vpn -G vpn && \
    mkdir -p /etc/sing-box && \
    chown vpn:vpn /etc/sing-box

COPY --from=builder /bin/vpn-manager /usr/local/bin/vpn-manager
COPY migrations/ /migrations/

USER vpn

EXPOSE 8080

ENTRYPOINT ["vpn-manager"]
