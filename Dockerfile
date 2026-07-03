# Stage 1: Svelte frontend build
FROM node:22-bookworm-slim AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install -g npm@11.18.0 && npm ci --no-audit --no-fundCOPY frontend/ .
RUN npm run build

# Stage 2: Go build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# Copy built frontend into the Go embed directory
COPY --from=frontend /src/dist /src/internal/web/ui

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/vpn-manager ./cmd/vpn-manager

# Stage 3: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata iproute2 iptables nftables

RUN addgroup -S vpn && adduser -S vpn -G vpn

COPY --from=builder /bin/vpn-manager /usr/local/bin/vpn-manager
COPY migrations/ /migrations/

USER vpn

EXPOSE 8080

ENTRYPOINT ["vpn-manager"]
