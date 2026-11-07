-- 071 (alpha.128, G15): remote API tokens.
--
-- api_tokens: one row per token an account made on My account. The token is
-- cpk_<token_id>_<secret>. token_id (12 characters of base32) is kept in plain
-- text to find the row; of the secret (32 random bytes, base64url) only its
-- SHA-256 is kept, so the database never holds a usable token. The full token
-- is shown once, in the answer to its creation.
--
-- scope is read (GET of the listed read routes) or manage (also the listed
-- change routes); every other route refuses a token (cmd/panel
-- api_token_auth.go). auth_epoch is the owner's users.auth_epoch when the token
-- was made: a password change or reset, a two-factor change or an account
-- change ends the token, as it ends the account's sessions. Times are Unix
-- seconds (UTC). name and scope are validated in code, without CHECK
-- (project rule of 004); the token layer treats any scope other than manage
-- as read. expires_at is required: a token always ends. revoked_at is
-- set when the owner removes it; the row is kept for the audit trail and never
-- authenticates again. last_used_at and last_used_ip are written at most once a
-- minute.
--
-- Additive only: one new table and one index; no existing row or value
-- changes. The release before this one refuses a ledger that carries entry
-- 71; going back is the pre-update snapshot restore, as for every migration.
-- Nothing a hosted service reads is here: the remote API is optional panel
-- automation (D-022), off on the server until the administrator turns it on
-- (panel_settings remote_api_enabled); with the panel stopped the owner's
-- services run on.
CREATE TABLE api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_id TEXT NOT NULL UNIQUE CHECK (length(token_id) = 12),
    secret_sha256 TEXT NOT NULL CHECK (length(secret_sha256) = 64),
    scope TEXT NOT NULL,
    auth_epoch INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at > created_at),
    last_used_at INTEGER,
    last_used_ip TEXT,
    revoked_at INTEGER,
    revoked_by INTEGER
);

CREATE INDEX idx_api_tokens_user ON api_tokens(user_id, revoked_at);
