FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/vpn-manager ./cmd/vpn-manager

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata iproute2 iptables nftables

RUN addgroup -S vpn && adduser -S vpn -G vpn

COPY --from=builder /bin/vpn-manager /usr/local/bin/vpn-manager
COPY migrations/ /migrations/

USER vpn

EXPOSE 8080

ENTRYPOINT ["vpn-manager"]
