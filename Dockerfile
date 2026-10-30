FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/vpn-manager ./cmd/vpn-manager

# Official AmneziaWG userspace client, pinned to v3.1.20260828.
FROM golang:1.26-alpine AS awg-builder
RUN apk add --no-cache git ca-certificates
WORKDIR /awg
RUN git init && git remote add origin https://github.com/amnezia-vpn/amneziawg-go.git && \
    git fetch --depth 1 origin b5928efb6ca19f0153958460c3d141f04abc5c2e && \
    git checkout --detach FETCH_HEAD
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/amneziawg-go .

FROM alpine:3.21

# Install runtime deps + sing-box
RUN apk add --no-cache ca-certificates tzdata iproute2 iptables nftables wget procps-ng

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

# Create config directory
RUN mkdir -p /etc/sing-box

COPY --from=builder /bin/vpn-manager /usr/local/bin/vpn-manager
COPY --from=awg-builder /bin/amneziawg-go /usr/local/bin/amneziawg-go
COPY migrations/ /migrations/

EXPOSE 8080

ENTRYPOINT ["vpn-manager"]
