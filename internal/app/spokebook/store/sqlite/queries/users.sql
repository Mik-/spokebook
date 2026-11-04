-- name: CreateUser :exec
INSERT INTO users (id, name, password_hash, created_at) VALUES (?, ?, ?, ?);

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByName :one
SELECT * FROM users WHERE name = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at;

-- name: SetPassword :execrows
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: SetLoginFailures :execrows
UPDATE users SET failed_logins = ?, locked_until = ? WHERE id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: TouchSession :execrows
UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = ? AND id <> ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= ?;

-- name: CreatePasskey :exec
INSERT INTO passkeys (id, user_id, credential_id, name, credential, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetPasskey :one
SELECT * FROM passkeys WHERE user_id = ? AND id = ?;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM passkeys WHERE credential_id = ?;

-- name: ListPasskeys :many
SELECT * FROM passkeys WHERE user_id = ? ORDER BY created_at;

-- name: UpdatePasskeyCredential :execrows
UPDATE passkeys SET credential = ?, last_used_at = ? WHERE user_id = ? AND id = ?;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE user_id = ? AND id = ?;
