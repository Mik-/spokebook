-- +goose Up
-- Login names are unique regardless of case. password_hash is an argon2id
-- hash in PHC format. After too many failed logins, the account is locked
-- until locked_until.
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    failed_logins INTEGER NOT NULL DEFAULT 0,
    locked_until  TEXT,
    created_at    TEXT NOT NULL
);

-- id is the SHA-256 of the session token (hex), so a leaked database
-- doesn't reveal valid cookies.
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- credential is the WebAuthn credential (public key, sign count, flags) as
-- JSON; only package auth interprets it.
CREATE TABLE passkeys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id BLOB NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    credential    BLOB NOT NULL,
    created_at    TEXT NOT NULL,
    last_used_at  TEXT
);
CREATE INDEX passkeys_user_idx ON passkeys (user_id);

-- +goose Down
DROP TABLE passkeys;
DROP TABLE sessions;
DROP TABLE users;
