# Gateway API

All endpoints are under `/api/v1`. Existing `enabled` and `interface` fields remain available.

## Profile gateway

`POST /gateway/enable`

```json
{"mode":"profile","client_ip":"192.168.1.71","lan_interface":"eth0"}
```

The pilot accepts only `192.168.1.71`; prefixes, local manager addresses, loopback and multicast are rejected. `provider_name` cannot be combined with profile mode. The active routing profile selects VPN outbounds; unmatched traffic goes direct. Domain rules use sniffed names and are independent of cached DNS addresses. ECH or traffic without an observable hostname cannot be reliably matched by a domain rule. IPv6 needs separate router verification before rollout.

The manager owns `vmtun0`, routing table 51840, priority 18040 and nft table `inet vpn_manager_profile`. It refuses foreign objects instead of replacing them. Host main routes and global forwarding settings are preserved; IPv4 forwarding must already be enabled.

`GET /gateway/status` returns `enabled`, `interface`, `mode`, `client_ip`, `ready`, `error`. `enabled` is desired capture state, while `ready` also checks the owned proxy process. A running proxy alone does not establish successful VPN handshakes or end-to-end client connectivity.

`POST /gateway/disable` switches the selected device to direct forwarding. It retains host-specific masquerade and suppresses ICMP redirects to that host, because the router may still use the manager as its gateway. It does not mean the router policy was removed.

After restoring the client's normal policy on the router, `POST /gateway/release` with `{"router_policy_removed":true}` removes the fallback. Capture must first be disabled. The confirmation is an assertion by the caller; the manager cannot inspect router policy itself.

Persist `/var/lib/vpn-manager` across container recreation. Override the state file with `GATEWAY_STATE_PATH` if necessary. Private state contains the desired client configuration and exact ownership snapshots. Volatile nft handles are excluded from ownership fingerprints. Startup rejects changed network objects rather than taking them over. Capture requires a successfully activated routing profile. Startup first removes the old TUN input, reconstructs providers and the profile, then restores capture. If reconstruction fails, the selected source remains blocked. A service shutdown preserves the source rule and blackhole fallback, preventing direct fallback when the proxy dies; startup restores the TUN route after proxy readiness. Every subsequent sing-box reload, including rollback restarts, restores the owned TUN route before reporting success.

## Legacy NAT gateway

`POST /gateway/enable` with `{"provider_name":"name"}` retains the legacy behavior through a connected interface provider. It cannot be combined with a configured profile gateway; release the profile first.

## Provider updates

A provider update validates replacements before disconnecting a working provider. Failed startup restores the previous provider and outbound. The API also restores the previous database record and routing profile, returning 502 to report that the requested update was not applied. If restoration itself fails, the error explicitly reports incomplete rollback. Provider deletion reports runtime cleanup failures separately.

## Validation still required before pilot rollout

- Run real userspace AWG/TUN/nft/ip-rule tests in an isolated Linux namespace.
- Verify reboot/recreation, partial network-command failure and proxy/VPN outages.
- Verify router policy support, original client source address, address reservation and IPv6.
- Check iPhone direct sites, VPN sites, DNS and QUIC; verify other devices remain unchanged.


## Router-selected clients

`POST /gateway/enable` accepts an additive mode:

```json
{"mode":"router","client_subnet":"192.168.1.0/24","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}
```

The canonical private IPv4 prefix (/16 through /30) and manager address must match the actual selected LAN interface. Router and manager addresses must be distinct non-network/non-broadcast addresses in that subnet. Additional manager aliases within the subnet are rejected until explicit bypass support is provided. `client_ip` and `provider_name` cannot be combined with router mode.

Only forwarded source traffic arriving on that LAN interface is intercepted. The router and manager sources bypass capture through owned priorities 18038/18039; subnet capture uses 18040 and table 51840. Private destinations continue through existing routes; direct fallback NAT excludes private destinations and infrastructure sources. Locally created manager traffic is unaffected. Unselected devices do not send their traffic to this server and remain unaffected. No individual client registration is required here: assign the VPN Manager policy in Keenetic. IPv6 remains outside this IPv4 mode and must be verified separately before enabling an external IPv6 path.

Status adds `client_subnet`, `router_ip`, `manager_ip`; `mode` is `router` while such scope is retained, including disabled direct mode. Existing single-client requests/state stay supported. An enabled pilot may expand to a matching connected LAN scope through the same enable endpoint. The transaction validates ownership, stages broader rules, atomically replaces the owned nft table, removes the pilot rule, and persists the new scope. Failures restore the previous scope; incomplete rollback is an error and never reports ready. Other scope changes require coordinated disable/release.

Disable retains direct forwarding/NAT for all router-selected clients. Release requires `router_policy_removed=true` for **all** assigned clients. Keep the manager itself on its WAN-only Keenetic policy. Selecting devices changes who sends packets to the gateway; it does not authorize new inbound access or expose the panel publicly.


### Keenetic LAN source NAT

Live packet capture established that this Keenetic replaces selected clients' source addresses with its LAN address when forwarding through a same-LAN next hop. For this topology set `router_nat:true` in router mode. The manager then captures the forwarded router source too, and includes it in direct fallback masquerading. Its own source remains excluded. Locally originated manager connections use the separate WAN-only Keenetic policy, preventing recirculation. Without this flag the original router-source bypass is preserved for compatibility.

An enabled router scope may upgrade from `router_nat:false` to `true` with every other field unchanged. Only the difference in owned rules is changed, under the same rollback/ownership transaction; common rules are never duplicated. Status exposes `router_nat`. The panel defaults new router setup to NAT compatibility and offers an explicit upgrade button for an existing scope. Source NAT means the manager cannot identify individual original clients; client selection remains entirely in Keenetic.
