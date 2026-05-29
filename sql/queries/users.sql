-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password) 
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1;

-- name: GetUserFromRefreshToken :one
SELECT refresh_tokens.user_id FROM refresh_tokens
WHERE refresh_tokens.token = $1
  AND refresh_tokens.expires_at > NOW()
  AND refresh_tokens.revoked_at IS NULL;

-- name: UpdateEmailAndPassword :one
UPDATE users
SET
    email = $1,
    hashed_password = $2
WHERE id = $3
RETURNING *;

-- name: UpgradeRedSubscription :exec
UPDATE users
SET
    is_chirpy_red = TRUE
WHERE id = $1;
