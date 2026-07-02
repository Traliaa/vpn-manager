---
name: project-overview
description: Цели и архитектура проекта vpn-manager
metadata:
  type: reference
---

# vpn-manager — проект управления VPN и маршрутизацией

## Цель

Сервис, который управляет маршрутизацией трафика через разные VPN-провайдеры (AmneziaWG, WireGuard, VLESS, Hysteria2, TUIC, Shadowsocks) с REST API и веб-интерфейсом.

## Инфраструктура

- **Proxmox VE**, LXC `vpn-gateway` (Debian 13).
- IP контейнера: **192.168.2.138**, PVE: **192.168.2.39**.
- Keenetic — основной роутер.
- Cockpit, Nginx Proxy Manager.
- **sing-box** v1.13.14 — установлен, используется как клиент (без TUN, без route rules).
- **AmneziaWG** — установлен, работает через `awg-quick`.
- **nftables** — NAT для AWG: `oifname "awg0" masquerade`.

## Keenetic limitation

DNS Route Lists маршрутизируют только DNS, а не TCP/UDP трафик. Вся маршрутизация делается внутри vpn-manager.

## Технологии

- **Backend**: Go (1.25), `uber-go/fx`, `chi`, `pgx/v5`, `golang-migrate`.
- **Database**: PostgreSQL (изначально SQLite, но перешли на PG).
- **API**: REST.
- **Управление**: sing-box config + systemd, nftables.

## Архитектура

```
Web UI → REST API → vpn-manager → { AmneziaWG, sing-box, nftables } → Интернет
```

## VPN Protocol Controllers

### Готово
- **WireGuard** — через `wgctrl` (golang.zx2c4.com/wireguard/wgctrl)
- **AmneziaWG** — через `wgctrl` (использует тот же kernel интерфейс)

### Планируется (через sing-box как backend)
- **VLESS**
- **Hysteria2**
- **TUIC**
- **Shadowsocks**

Подход: генерировать JSON-конфиг для sing-box, управлять через его REST API (или перезагрузку конфига + systemctl).

## Функции

1. **Проверка доступности VPN** — health checker, авто-переключение при падении.
2. **Профили** (Direct, Amnezia, VLESS, Gaming, Streaming) — переключение одной кнопкой.
3. **Маршрутизация** по доменам, IP, ASN, GeoIP, GeoSite.
4. **Авто-обновление IP** — DNS → IP → обновление маршрутов (таблица `resolved_routes`).
5. **nftables/ipset** управление.

## Этапы разработки

1. ✅ Каркас, конфиг, REST API, PostgreSQL.
2. ✅ Управление AmneziaWG, WireGuard, статус VPN.
3. 🟡 Маршрутизация по доменам, авто-обновление IP, sing-box интеграция.
4. ❌ Веб-интерфейс.
5. ❌ Home Assistant, Telegram.
