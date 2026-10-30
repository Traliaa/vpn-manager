# iPhone Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement task-by-task. Steps use checkboxes for tracking.

**Goal:** Управлять из VPN Manager прозрачным шлюзом для iPhone Алексей 192.168.1.71 с распределением по профилю.

**Architecture:** Менеджер владеет userspace AmneziaWG и единственным процессом sing-box. TUN принимает только выбранный /32; прямой и VPN-выходы изолированы маршрутами, профиль задаёт выбор. Keenetic направляет на LXC только iPhone после готовности и Linux-проверок.

**Tech Stack:** Go 1.25+, Svelte 5, PostgreSQL, Docker, sing-box 1.11.7 в образе, amneziawg-go/awg, Linux iproute2/nftables.

**Spec:** ../specs/2026-10-29-iphone-gateway-design.md

## Global Constraints
- Только iPhone 192.168.1.71/32, MAC 74:29:59:be:b8:a6; LXC 192.168.1.40; Keenetic 192.168.1.1. Не захватывать /24, localhost или собственный адрес менеджера.
- Управление через существующую панель/API; сохранить прежний NAT-режим.
- Никаких kernel modules на Proxmox, общего DHCP/default route/firewall reset.
- Только синтетические ключи в новых/изменяемых тестах. Не копировать реальные ключи из Downloads в код, Git, логи или план.
- Безуспешное применение сохраняет предыдущую конфигурацию; VPN-назначения не уходят напрямую при отказе VPN.
- Пользовательский процесс: локальная разработка → GitHub → сборка на сервере.

## Review Focus
- Повторное включение/отключение и падение дочернего процесса: нет дублей интерфейсов/правил и ложного статуса up (Tasks 1–3).
- Ошибка после частичной настройки: удаляются только собственные изменения, старая конфигурация работает (Tasks 2–3).
- DNS, QUIC, ECH и IPv6: подтверждённые границы доменного сопоставления и отсутствие незаметного обхода (Tasks 3, 6).
- Конфликт порта/чужой процесс: не убивать чужой sing-box через pkill; ошибка видна в UI (Tasks 2, 4).
- Сохранённые настройки после рестарта/обновления провайдера: восстановление только после готовности, безопасный rollback (Tasks 1, 3–6).

## Task 1: Correct AmneziaWG import and lifecycle
**Files:** internal/vpn/importer/{parser.go,converter.go,converter_test.go,parser_test.go}; internal/vpn/amneziawg/{controller.go,userspace.go,controller_test.go}; internal/vpn/service/service.go; Dockerfile.
**Interfaces:** Existing vpn.Provider remains unchanged. Add optional H1–H4 and remaining supported obfuscation fields to amneziawg.Config with backward-compatible JSON. Controller owns userspace process and commands; inject internal command/netlink adapters for tests.
- [ ] Add failing round-trip test preserving all Budapest fields, including H1–H4 and S1/S2; assert no real keys in fixtures. Test malformed keys/endpoints/allowed IPs cause zero network operations.
- [ ] Run `go test ./internal/vpn/importer ./internal/vpn/amneziawg`; demonstrate regression before implementation.
- [ ] Preserve raw supported AmneziaWG settings; configure kernel backend correctly or fall back to pinned userspace backend. Never downgrade AmneziaWG to ordinary WireGuard. Userspace process survives request-context completion and is stopped/reaped by lifecycle.
- [ ] Test repeated Connect/Disconnect/Close, failed startup/configuration cleanup, typed nil config, cancellation, expired handshake, unique stable interface names. Check status reflects process and handshake rather than enabled flag.
- [ ] Add pinned reproducible amneziawg-go/awg to Docker image, verify their protocol support against supplied config without committing secrets. Run focused tests and Linux smoke check; commit.

## Task 2: Transactional sing-box ownership and interface outbounds
**Files:** internal/vpn/singbox/{controller.go,routing.go,controller_test.go,interface_outbound.go}; internal/vpn/service/service.go; internal/routing/{activator.go,activator_test.go}; internal/app/module.go.
**Interfaces:** Keep Outbound.Tag()/BuildOutbound(). New InterfaceOutbound emits direct outbound bound to the VPN interface, with route table/mark dedicated to VPN output. Controller owns only its child process. Existing SetOutbounds/GenerateConfigWithRouting return errors on validation or readiness failure.
- [ ] Write failing tests: missing outbound tag rejects activation; config check failure retains old disk/runtime config; child exit is not running; restarting does not kill unrelated PID. Domain/IP OR semantics must be represented by separate rules, not an unintended AND.
- [ ] Implement temporary-file validation before atomic replacement, rollback on reload failure, bounded readiness/error output with secret redaction, managed child Wait/Stop/Close. Avoid pgrep/pkill as ownership checks.
- [ ] Register AWG interface outbound and outbound tag during provider sync, remove/update consistently on disable/config change. Preserve direct via eth0 and prevent VPN endpoint recursion.
- [ ] Run `go test -race ./internal/vpn/... ./internal/routing/...` and validate generated JSON with the deployed sing-box version. Commit.

## Task 3: Profile gateway with strict client scope and rollback
**Files:** internal/vpn/gateway/{manager.go,profile.go,profile_test.go,state.go}; internal/vpn/singbox/{gateway.go,gateway_test.go}; internal/config/config.go; internal/app/module.go.
**Interfaces:** Add gateway.ProfileConfig{ClientIP string, LANInterface string}; EnableProfile(context.Context, ProfileConfig) error alongside legacy Enable(ctx,iface). Status extends mode/client/ready/error without removing enabled/interface. Sing-box gateway configuration adds TUN and sniffing without automatic global routes.
- [ ] Failing tests accept 192.168.1.71/32 and reject /24, empty/multicast/loopback/LXC address; only selected source and incoming LAN interface match the routing rule. Local destinations bypass TUN.
- [ ] Build isolated TUN routing table and client-specific rule, distinct VPN outbound table, local/endpoint exceptions and exact cleanup. Preserve unrelated rules and original sysctls. Gateway never redirects own outbound traffic back to TUN.
- [ ] Stage state in a private application data directory, activate only after sing-box/VPN readiness, persist after success. Restore on startup after dependencies; disable must remove own state/rules even after partial failure.
- [ ] Test failure at every command boundary, reenable/disable/restart, missing tun permissions, VPN outage with normal direct traffic continuing. Add Linux network-namespace integration harness with synthetic endpoints; no live home-route mutations in tests. Commit.

## Task 4: Backward-compatible API and panel
**Files:** internal/api/{gateway_handlers.go,gateway_handlers_test.go,router.go}; frontend/src/pages/Dashboard.svelte; frontend/src/lib/api.js; docs/api/gateway.md.
**Interfaces:** POST /gateway/enable accepts legacy provider_name as before or explicit mode=profile, client_ip, lan_interface; incompatible combinations return 400. GET /gateway/status includes mode/client_ip/ready/error. Disable is idempotent. Existing provider/routing API paths remain unchanged.
- [ ] Add handler tests for invalid bodies, /32 limits, nil/error provider statuses, setup errors, legacy requests, profile mode and accurate status. Use existing httptest approach; document exact request/response contract.
- [ ] Add panel mode «По активному профилю», client IP input, readiness/failure text and disable/rollback guidance. Keep enable controls disabled while a request is pending; do not show success on rejected or incomplete activation.
- [ ] Test API locally and frontend production build. Reuse existing API artifacts if present; no external Postman publication or extra account setup. Commit.

## Task 5: Full verification, self-review, release preparation
**Files:** new/modified files above; docs/operations/iphone-gateway.md; Dockerfile and deploy config as necessary.
- [ ] Run `go test ./internal/...`, `go test -race ./internal/...`, `go vet ./...`, `go build ./cmd/...`, frontend `npm ci` + `npm run build`, and Linux integration harness/image config validation. Distinguish Darwin unit checks from Linux networking evidence.
- [ ] Review full diff against design: lifecycle races/leaks, cancellation, atomic updates, recovery, API/DB compatibility, secret handling and exact source limitation. Fix defects and repeat affected checks.
- [ ] Inspect server git revision/local changes, deployed compose and actual runtime overrides without printing secrets. Record rollback image/revision and previous host service state. Keep database and reverse-proxy configuration intact.
- [ ] Commit reviewed changes, push to existing GitHub origin/appropriate branch, verify pushed SHA. Build that exact SHA on server; validate image before replacing app. Do not trust old cached image or default compose database settings.

## Task 6: Single-device rollout and acceptance
**Files:** docs/operations/iphone-gateway.md (actual results and rollback steps).
- [ ] Deploy verified app image with saved rollback. Resolve old host sing-box conflict only after replacement readiness, and restore old service if replacement fails.
- [ ] Update Budapest configuration from original .conf to restore fields lost by old importer. Enable provider and profile; verify handshake and DNS/TCP/UDP paths from synthetic isolated client before touching iPhone.
- [ ] Confirm Keenetic per-device policy support and preservation of source IP; reserve iPhone address. Create policy for only MAC 74:29:59:be:b8:a6 with next-hop 192.168.1.40; never use default network policy. Verify IPv6 scope; if safe per-device handling is unavailable, do not switch or claim all traffic is handled.
- [ ] Enable client-specific profile gateway through panel and then switch only iPhone. Verify ingress/counters, VPN destination exit, ordinary direct exit, LAN access, and unchanged sample other-device traffic. Request user's on-phone check; do not claim device acceptance before result.
- [ ] Test disable/reenable and server/app restart recovery. On failure revert iPhone policy first, then gateway/app. Record remaining ECH/DNS/IPv6 limits explicitly.

## Baseline evidence
`go test ./internal/...` passed on local checkout 42cb1db before product changes. No lifecycle/gateway tests exist yet; this does not establish Linux network behavior.
