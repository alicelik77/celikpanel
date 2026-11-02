-- 056 (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md section 8; owner decision
-- of 2 November 2026): after a subscription transfer, the passwords and keys
-- the previous owner knew are changed on the server. Additive only: two new
-- tables and one index. No existing table, trigger or value changes. The
-- release before this one refuses a ledger that carries entry 56; going back
-- is the pre-update snapshot restore, as for every migration.
--
-- The owner change commits first (054's transaction); in the same
-- transaction one transfer_rotations row and its steps are written and the
-- subscription's VPN peers are marked revoked. The Panel's worker then
-- changes the server step by step; a crash leaves the operation 'running'
-- and the worker continues it at start. Polling only reads these rows.
--
-- No password is stored here: each run makes a new random one, and a
-- database user's is kept sealed in database_users once the engine has it.
-- before holds only states (enabled, key count, password state).
--
-- transfer_rotations.state: running | completed | failed (error_kind says
-- verified_failure, prerequisite or unknown).
-- transfer_rotation_effects.state:
--   pending, running, done, failed,
--   manual    CelikPanel cannot change it here (another server, legacy row)
--   replaced  the new owner set that credential first
--
-- Devirden sonra eski sahibin bildiği parola ve anahtarlar sunucuda
-- değiştirilir. Yalnız iki yeni tablo; parola saklanmaz.

CREATE TABLE transfer_rotations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    transfer_id INTEGER NOT NULL UNIQUE CHECK (transfer_id > 0),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    actor_id INTEGER,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error_kind TEXT CHECK (error_kind IS NULL OR error_kind IN ('verified_failure', 'prerequisite', 'unknown')),
    last_error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT
);
CREATE INDEX idx_transfer_rotations_state ON transfer_rotations(state, created_at);

CREATE TABLE transfer_rotation_effects (
    operation_id TEXT NOT NULL,
    seq INTEGER NOT NULL CHECK (seq >= 0),
    kind TEXT NOT NULL CHECK (kind IN ('site_account', 'vpn', 'mailbox', 'database_user', 'protected_dir')),
    target TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ref)),
    before TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(before)),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'done', 'failed', 'manual', 'replaced')),
    detail TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (operation_id, seq)
);
