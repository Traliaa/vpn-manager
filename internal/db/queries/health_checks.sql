-- Проверки здоровья
-- name: CreateHealthCheck :one
INSERT INTO health_checks (provider_id, status, latency_ms, error)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListHealthChecks :many
SELECT * FROM health_checks
WHERE provider_id = $1
ORDER BY checked_at DESC
LIMIT $2;

-- name: GetLatestHealthCheck :one
SELECT * FROM health_checks
WHERE provider_id = $1
ORDER BY checked_at DESC
LIMIT 1;

-- name: GetProviderUptime :one
SELECT
    COUNT(*) FILTER (WHERE status = 'up')::float / GREATEST(COUNT(*), 1)::float * 100 as uptime_pct,
    COUNT(*) FILTER (WHERE status = 'down')::int as failures,
    COUNT(*)::int as total_checks
FROM health_checks
WHERE provider_id = $1
  AND checked_at > now() - $2::interval;
