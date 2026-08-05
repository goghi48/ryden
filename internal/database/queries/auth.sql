-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_name, nickname)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, password_hash, display_name, avatar_url, created_at, updated_at, nickname, avatar_revision;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at, nickname, avatar_revision
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at, nickname, avatar_revision
FROM users
WHERE id = $1;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = $2,
    nickname = $3,
    avatar_url = $4,
    updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, display_name, avatar_url, created_at, updated_at, nickname, avatar_revision;

-- name: CreateRefreshSession :exec
INSERT INTO refresh_sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetRefreshSessionForUpdate :one
SELECT id, user_id, token_hash, expires_at, revoked_at, replaced_by, created_at
FROM refresh_sessions
WHERE token_hash = $1
FOR UPDATE;

-- name: RotateRefreshSession :execrows
UPDATE refresh_sessions
SET revoked_at = now(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeRefreshSession :execrows
UPDATE refresh_sessions
SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;
