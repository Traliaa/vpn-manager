-- Правила маршрутизации
-- name: ListRulesByProfile :many
SELECT * FROM routing_rules
WHERE profile_id = $1
ORDER BY priority ASC NULLS LAST, created_at ASC;

-- name: GetRule :one
SELECT * FROM routing_rules WHERE id = $1;

-- name: CreateRule :one
INSERT INTO routing_rules (profile_id, provider_id, rule_type, value, enabled, priority, description)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateRule :one
UPDATE routing_rules
SET provider_id  = COALESCE($2, provider_id),
    rule_type    = COALESCE($3, rule_type),
    value        = COALESCE($4, value),
    enabled      = COALESCE($5, enabled),
    priority     = COALESCE($6, priority),
    description  = COALESCE($7, description),
    updated_at   = now()
WHERE id = $1
RETURNING *;

-- name: DeleteRule :exec
DELETE FROM routing_rules WHERE id = $1;

-- name: ListEnabledRulesByProfile :many
SELECT r.*, p.provider_type, p.name as provider_name
FROM routing_rules r
LEFT JOIN vpn_providers p ON r.provider_id = p.id
WHERE r.profile_id = $1 AND r.enabled = true
ORDER BY r.priority ASC NULLS LAST;

-- name: ListEnabledDomainRules :many
SELECT r.*, p.provider_type, p.name as provider_name
FROM routing_rules r
LEFT JOIN vpn_providers p ON r.provider_id = p.id
WHERE r.enabled = true
  AND r.rule_type IN ('domain', 'domain_suffix', 'domain_keyword');

-- name: GetRulesByProvider :many
SELECT * FROM routing_rules WHERE provider_id = $1 AND enabled = true;
