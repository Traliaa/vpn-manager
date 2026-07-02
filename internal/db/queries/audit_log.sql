-- Журнал аудита
-- name: CreateAuditLog :one
INSERT INTO audit_log (action, entity_type, entity_id, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_log
ORDER BY created_at DESC
LIMIT $1
OFFSET $2;

-- name: ListAuditLogsByEntity :many
SELECT * FROM audit_log
WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at DESC
LIMIT $3;
