# Router-selected Devices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Implement inline, followed by one fresh whole-change review.

**Goal:** Choose clients only in Keenetic, with no per-device registration in VPN Manager.
**Architecture:** Preserve single-client mode. Add router mode with a validated directly connected private LAN prefix, router and manager exclusions, owned source policy rules and scoped NAT. Upgrade the pilot through a rollback-safe scope transaction.
**Tech Stack:** Go, Svelte, Linux policy routing/nftables, sing-box.
**Spec:** docs/superpowers/specs/2026-10-30-router-selected-devices-design.md

## Global Constraints
- LAN 192.168.1.0/24, eth0; router 192.168.1.1 and manager 192.168.1.40 excluded.
- Preserve existing single-client state, foreign network objects, host main routes and non-selected devices.
- Disable keeps direct fallback; proxy failure blocks captured public traffic; no automatic IPv6 enablement.

## Review Focus
- Partial transition errors must restore the previous scope and private state.
- Same-subnet responses, local traffic and own/router source must bypass capture.
- Restart must rebuild every missing owned source rule, including exclusion rules.
- Invalid subnet/interface combinations must fail before mutation.
- Reload and rollback must retain the owned TUN route and fail-closed fallback.

### Task 1: Scope and lifecycle
Files: internal/vpn/gateway/{profile.go,state.go,manager.go,scope.go,scope_test.go}, Linux traffic tests.
Interfaces: ProfileConfig gains ClientSubnet, RouterIP, ManagerIP; legacy ClientIP stays. EnableProfile supports initial router scope and a specific pilot-to-router migration.
- [ ] Write failing validation, source-rule exclusion, restart and transition rollback tests; observe failure.
- [ ] Implement strict canonical private connected-prefix validation and exclusion rules in owned priority slots; generalized rule reconstruction and scoped NAT.
- [ ] Implement transactional pilot expansion with previous runtime/state recovery on error.
- [ ] Run gateway tests, then full internal tests. Commit.

### Task 2: API and panel
Files: internal/api/gateway_handlers{,_test}.go, frontend/src/pages/Dashboard.svelte, docs/api/gateway.md.
Interfaces: POST gateway/enable mode=router with client_subnet, router_ip, manager_ip, lan_interface; GET status exposes fields and router mode.
- [ ] Write failing API tests for ambiguous fields and status contract; observe failure.
- [ ] Implement additive contract and UI router scope option and pilot upgrade action.
- [ ] Document router-only client selection, direct fallback and IPv6 limits.
- [ ] Run internal tests/race/vet and frontend build. Commit.

### Task 3: Isolated proof and rollout
Files: Linux traffic test and release evidence.
- [ ] Extend isolated Linux proof: .71 and .72 both follow profile; .1/.40 and non-forwarded traffic bypass; legacy pilot still works.
- [ ] Exercise reload, proxy death/recovery, persisted restore, direct fallback and failed scope migration.
- [ ] Fresh review, fix material findings with failing regression tests, run final checks.
- [ ] Publish pinned revision, build on server with rollback backups, upgrade existing pilot only, validate health and routing.
- [ ] Rename Keenetic displayed policy VPN_Manager, preserve client assignments and save. Report actual checks and limits.
