-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (created_at, updated_at, token, user_id, expires_at) 
VALUES (
    NOW(),
    NOW(),
    $1,
    $2,
    $3
);

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET 
    revoked_at = NOW(),
    updated_at = NOW()
WHERE token = $1;
