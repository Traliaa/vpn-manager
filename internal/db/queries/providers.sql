-- Провайдеры
-- name: ListProviders :many
SELECT * FROM vpn_providers ORDER BY priority ASC NULLS LAST, name ASC;

-- name: GetProvider :one
SELECT * FROM vpn_providers WHERE id = $1;

-- name: CreateProvider :one
INSERT INTO vpn_providers (name, provider_type, config, enabled, priority, health_host)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateProvider :one
UPDATE vpn_providers
SET name         = COALESCE($2, name),
    provider_type = COALESCE($3, provider_type),
    config       = COALESCE($4, config),
    enabled      = COALESCE($5, enabled),
    priority     = COALESCE($6, priority),
    health_host  = COALESCE($7, health_host),
    updated_at   = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProvider :exec
DELETE FROM vpn_providers WHERE id = $1;

-- name: ListEnabledProviders :many
SELECT * FROM vpn_providers WHERE enabled = true ORDER BY priority ASC NULLS LAST;

-- name: FindProviderByPeerKey :one
SELECT * FROM vpn_providers
WHERE provider_type = $1
  AND config->'peer'->>'public_key' = $2
LIMIT 1;
