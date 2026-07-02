-- Инициализация схемы vpn-manager

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Типы VPN-провайдеров
CREATE TYPE provider_type AS ENUM (
    'amneziawg',
    'wireguard',
    'vless',
    'hysteria2',
    'tuic',
    'shadowsocks'
);

-- Типы правил маршрутизации
CREATE TYPE rule_type AS ENUM (
    'domain',
    'domain_suffix',
    'domain_keyword',
    'ip',
    'cidr',
    'asn',
    'geoip',
    'geosite'
);

-- Типы интерфейсов
CREATE TYPE interface_type AS ENUM (
    'amneziawg',
    'wireguard',
    'tun'
);

-- Состояния
CREATE TYPE interface_state AS ENUM (
    'up',
    'down',
    'error'
);

CREATE TYPE check_status AS ENUM (
    'up',
    'down',
    'degraded'
);

-- ============================================================================
-- Провайдеры VPN
-- ============================================================================
CREATE TABLE vpn_providers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    provider_type provider_type NOT NULL,
    config       JSONB       NOT NULL DEFAULT '{}',
    enabled      BOOLEAN     NOT NULL DEFAULT true,
    priority     INT         NOT NULL DEFAULT 100,
    health_host  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vpn_providers_type ON vpn_providers(provider_type);
CREATE INDEX idx_vpn_providers_enabled ON vpn_providers(enabled);

-- ============================================================================
-- Интерфейсы (awg0, wg0, sing-box tun)
-- ============================================================================
CREATE TABLE interfaces (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT            NOT NULL UNIQUE,
    provider_id UUID            REFERENCES vpn_providers(id) ON DELETE CASCADE,
    type        interface_type  NOT NULL,
    state       interface_state NOT NULL DEFAULT 'down',
    local_ip    INET,
    config      JSONB           NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE INDEX idx_interfaces_provider ON interfaces(provider_id);
CREATE INDEX idx_interfaces_state ON interfaces(state);

-- ============================================================================
-- Профили маршрутизации
-- ============================================================================
CREATE TABLE profiles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT,
    is_default  BOOLEAN     NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_profiles_default ON profiles(is_default) WHERE is_default = true;

-- ============================================================================
-- Правила маршрутизации (привязаны к профилю)
-- ============================================================================
CREATE TABLE routing_rules (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id  UUID        NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    provider_id UUID        REFERENCES vpn_providers(id) ON DELETE SET NULL,
    rule_type   rule_type   NOT NULL,
    value       TEXT        NOT NULL,
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    priority    INT         NOT NULL DEFAULT 500,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_routing_rules_profile ON routing_rules(profile_id);
CREATE INDEX idx_routing_rules_provider ON routing_rules(provider_id);
CREATE INDEX idx_routing_rules_type ON routing_rules(rule_type);
CREATE INDEX idx_routing_rules_enabled ON routing_rules(enabled);

-- ============================================================================
-- Разрешённые IP-адреса (результат авто-резолва доменов)
-- ============================================================================
CREATE TABLE resolved_routes (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id     UUID        NOT NULL REFERENCES routing_rules(id) ON DELETE CASCADE,
    ip          INET        NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_resolved_routes_unique ON resolved_routes(rule_id, ip);
CREATE INDEX idx_resolved_routes_ip ON resolved_routes USING GIST (ip inet_ops);

-- ============================================================================
-- Проверки доступности
-- ============================================================================
CREATE TABLE health_checks (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID         NOT NULL REFERENCES vpn_providers(id) ON DELETE CASCADE,
    status      check_status NOT NULL,
    latency_ms  INT,
    error       TEXT,
    checked_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_health_checks_provider ON health_checks(provider_id, checked_at DESC);

-- ============================================================================
-- Журнал событий
-- ============================================================================
CREATE TABLE audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   UUID,
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_created ON audit_log(created_at DESC);
