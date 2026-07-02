-- Профили
-- name: ListProfiles :many
SELECT * FROM profiles ORDER BY name ASC;

-- name: GetProfile :one
SELECT * FROM profiles WHERE id = $1;

-- name: CreateProfile :one
INSERT INTO profiles (name, description, is_default)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateProfile :one
UPDATE profiles
SET name        = COALESCE($2, name),
    description = COALESCE($3, description),
    is_default  = COALESCE($4, is_default),
    updated_at  = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProfile :exec
DELETE FROM profiles WHERE id = $1;

-- name: GetDefaultProfile :one
SELECT * FROM profiles WHERE is_default = true LIMIT 1;

-- name: ResetDefaultProfile :exec
UPDATE profiles SET is_default = false WHERE is_default = true;
