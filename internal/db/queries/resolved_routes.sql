-- Разрешённые маршруты
-- name: ListResolvedRoutesByRule :many
SELECT * FROM resolved_routes WHERE rule_id = $1 ORDER BY ip ASC;

-- name: UpsertResolvedRoute :one
INSERT INTO resolved_routes (rule_id, ip, last_seen)
VALUES ($1, $2, now())
ON CONFLICT (rule_id, ip) DO UPDATE
SET last_seen = now()
RETURNING *;

-- name: DeleteResolvedRoute :exec
DELETE FROM resolved_routes WHERE id = $1;

-- name: DeleteResolvedRoutesByRule :exec
DELETE FROM resolved_routes WHERE rule_id = $1;

-- name: DeleteStaleResolvedRoutes :exec
DELETE FROM resolved_routes WHERE last_seen < now() - $1::interval;

-- name: ListAllResolvedIps :many
SELECT DISTINCT rr.ip, r.provider_id
FROM resolved_routes rr
JOIN routing_rules r ON rr.rule_id = r.id
WHERE r.enabled = true;
