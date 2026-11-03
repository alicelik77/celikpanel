-- 060 (alpha.118, owner request "uzak yedek", docs/REMOTE-BACKUPS.md):
-- backups can also be sent to storage the owner controls elsewhere
-- (S3-compatible object storage or an SFTP server). Additive only: three new
-- tables and their indexes. No existing table, trigger or value changes; the
-- local backups and their schedules work exactly as before. The release
-- before this one refuses a ledger that carries entry 60; going back is the
-- pre-update snapshot restore, as for every migration. Remote objects are
-- not touched by a rollback: they stay where they are, readable with gpg.
--
-- remote_backup_destinations: owner_user_id NULL is a server-wide
-- destination of the administrator; otherwise the reseller's or customer's
-- own (needs the permission "remote_backup_destinations", D-042).
-- secret_sealed and passphrase_sealed hold internal/secrets values
-- ("enc:v1:..."); config_json holds no secret. host_key is the SFTP server
-- key a person confirmed; an empty one refuses every connection.
--
-- remote_backup_policies: one schedule of one destination. domain_id NULL is
-- the administrator's "every site on this server" schedule.
--
-- remote_backup_operations: one upload or restore. The id is the durable
-- operation identity; request_identity (D-029) makes a repeated request
-- return the same operation. state: queued | running | succeeded | failed.
-- failure_class: verified_failure | prerequisite | unknown. Polling only
-- reads this table; the Panel's worker alone moves an operation.
--
-- Uzak yedek: hedefler, zamanlamalar ve işlemler için üç yeni tablo. Yerel
-- yedekler değişmez. Gizli değerler mühürlüdür.

CREATE TABLE remote_backup_destinations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
    kind TEXT NOT NULL CHECK (kind IN ('s3', 'sftp')),
    config_json TEXT NOT NULL CHECK (length(config_json) <= 8192),
    secret_sealed TEXT NOT NULL DEFAULT '',
    passphrase_sealed TEXT NOT NULL,
    passphrase_check TEXT NOT NULL DEFAULT '',
    host_key TEXT NOT NULL DEFAULT '',
    offered_to_accounts INTEGER NOT NULL DEFAULT 0 CHECK (offered_to_accounts IN (0, 1)),
    last_test_at TEXT,
    last_test_result TEXT NOT NULL DEFAULT '',
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_remote_backup_destinations_owner ON remote_backup_destinations(owner_user_id);

CREATE TABLE remote_backup_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    destination_id INTEGER NOT NULL REFERENCES remote_backup_destinations(id) ON DELETE CASCADE,
    domain_id INTEGER REFERENCES domains(id) ON DELETE CASCADE,
    frequency TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly')),
    backup_type TEXT NOT NULL CHECK (backup_type IN ('files', 'full')),
    keep_daily INTEGER NOT NULL CHECK (keep_daily BETWEEN 0 AND 90),
    keep_weekly INTEGER NOT NULL CHECK (keep_weekly BETWEEN 0 AND 104),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX idx_remote_backup_policies_site ON remote_backup_policies(destination_id, domain_id) WHERE domain_id IS NOT NULL;
CREATE UNIQUE INDEX idx_remote_backup_policies_server ON remote_backup_policies(destination_id) WHERE domain_id IS NULL;
CREATE INDEX idx_remote_backup_policies_domain ON remote_backup_policies(domain_id);

CREATE TABLE remote_backup_operations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    kind TEXT NOT NULL CHECK (kind IN ('upload', 'restore')),
    destination_id INTEGER,
    domain_id INTEGER NOT NULL CHECK (domain_id > 0),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    policy_id INTEGER,
    backup_name TEXT NOT NULL DEFAULT '',
    backup_type TEXT NOT NULL DEFAULT '',
    object_key TEXT NOT NULL DEFAULT '',
    transient INTEGER NOT NULL DEFAULT 0 CHECK (transient IN (0, 1)),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'failed')),
    phase TEXT NOT NULL DEFAULT '',
    bytes_done INTEGER NOT NULL DEFAULT 0,
    bytes_total INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    failure_class TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL DEFAULT '',
    failure_detail TEXT NOT NULL DEFAULT '',
    verified TEXT NOT NULL DEFAULT '',
    requested_by INTEGER,
    request_identity TEXT UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    started_at TEXT,
    heartbeat_at TEXT,
    finished_at TEXT
);
CREATE INDEX idx_remote_backup_operations_domain ON remote_backup_operations(domain_id, created_at);
CREATE INDEX idx_remote_backup_operations_state ON remote_backup_operations(state);
CREATE INDEX idx_remote_backup_operations_policy ON remote_backup_operations(policy_id, domain_id, state);
