-- Интерфейсы
-- name: ListInterfaces :many
SELECT * FROM interfaces ORDER BY name ASC;

-- name: GetInterface :one
SELECT * FROM interfaces WHERE id = $1;

-- name: GetInterfaceByName :one
SELECT * FROM interfaces WHERE name = $1;

-- name: UpsertInterface :one
INSERT INTO interfaces (name, provider_id, type, state, local_ip, config)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (name) DO UPDATE
SET provider_id = COALESCE($2, interfaces.provider_id),
    type        = COALESCE($3, interfaces.type),
    state       = COALESCE($4, interfaces.state),
    local_ip    = COALESCE($5, interfaces.local_ip),
    config      = COALESCE($6, interfaces.config),
    updated_at  = now()
RETURNING *;

-- name: UpdateInterfaceState :one
UPDATE interfaces SET state = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteInterface :exec
DELETE FROM interfaces WHERE id = $1;
