# Gateway API

All endpoints are under `/api/v1`. Existing `enabled` and `interface` fields remain available.

## Profile gateway

`POST /gateway/enable`

```json
{"mode":"profile","client_ip":"192.168.1.71","lan_interface":"eth0"}
```

Only one private IPv4 host is accepted; prefixes, local manager addresses, loopback and multicast are rejected. `provider_name` cannot be combined with profile mode. The active routing profile selects VPN outbounds; unmatched traffic goes direct. Domain rules use sniffed names and are independent of cached DNS addresses. ECH or traffic without an observable hostname cannot be reliably matched by a domain rule. IPv6 needs separate router verification before rollout.

The manager owns `vmtun0`, routing table 51840, priority 18040 and nft table `inet vpn_manager_profile`. It refuses foreign objects instead of replacing them. Host main routes and global forwarding settings are preserved; IPv4 forwarding must already be enabled.

`GET /gateway/status` returns `enabled`, `interface`, `mode`, `client_ip`, `ready`, `error`. `enabled` is desired capture state, while `ready` also checks the owned proxy process. A running proxy alone does not establish successful VPN handshakes or end-to-end client connectivity.

`POST /gateway/disable` switches the selected device to direct forwarding. It retains host-specific masquerade and suppresses ICMP redirects to that host, because the router may still use the manager as its gateway. It does not mean the router policy was removed.

After restoring the client's normal policy on the router, `POST /gateway/release` with `{"router_policy_removed":true}` removes the fallback. Capture must first be disabled. The confirmation is an assertion by the caller; the manager cannot inspect router policy itself.

Persist `/var/lib/vpn-manager` across container recreation. Override the state file with `GATEWAY_STATE_PATH` if necessary. Private state contains the desired client configuration and exact ownership snapshots. Startup rejects changed network objects rather than taking them over. A service shutdown preserves the source rule and blackhole fallback, preventing direct fallback when the proxy dies; startup restores the TUN route after proxy readiness.

## Legacy NAT gateway

`POST /gateway/enable` with `{"provider_name":"name"}` retains the legacy behavior through a connected interface provider. It cannot be combined with a configured profile gateway; release the profile first.

## Provider updates

A provider update persists supplied configuration and reconciles enabled/disabled/configuration changes with runtime. A 502 response explicitly means desired database settings were saved, but runtime application failed. Inspect provider status and retry `/vpn/sync`; do not interpret that response as an atomic database rollback. Provider deletion uses the same reconciliation requirement.

## Validation still required before pilot rollout

- Run real userspace AWG/TUN/nft/ip-rule tests in an isolated Linux namespace.
- Verify reboot/recreation, partial network-command failure and proxy/VPN outages.
- Verify router policy support, original client source address, address reservation and IPv6.
- Check iPhone direct sites, VPN sites, DNS and QUIC; verify other devices remain unchanged.
