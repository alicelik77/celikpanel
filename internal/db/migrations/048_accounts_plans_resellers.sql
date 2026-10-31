-- alpha.114 (accounts, plans and resellers; ROADMAP v0.3 "Multi-Tenant
-- Reality"). One new table and two nullable columns; no row is changed.
--
-- reseller_pools: what the administrator lets one reseller hand out. Each
-- limit is a total over the reseller's whole tree (the reseller's own
-- subscriptions and every subscription of its customers); 0 means
-- unlimited, the same meaning a plan's 0 has. A reseller without a row has
-- no pool limits, which is how every reseller created before this release
-- keeps working exactly as it did.
--
-- sessions.impersonator_id: the operator who opened this session with "Log
-- in as". NULL for every ordinary sign-in and for every session made before
-- this release. The authentication middleware refuses the session once the
-- operator is gone, suspended or no longer manages the account, and a
-- password change or suspension of the operator deletes it.
--
-- audit_logs.acting_as: the account a row's user (the real person) was
-- acting as through "Log in as". NULL for every row written before this
-- release and for every action a person did as themselves.
--
-- Both columns are plain integers without a REFERENCES clause, like the
-- other columns added after 001: SQLite cannot drop a column that takes part
-- in a foreign key, and the rule lives in code (project note in 004).
CREATE TABLE reseller_pools (
    reseller_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    max_customers INTEGER NOT NULL DEFAULT 0 CHECK (max_customers >= 0),
    max_domains INTEGER NOT NULL DEFAULT 0 CHECK (max_domains >= 0),
    max_databases INTEGER NOT NULL DEFAULT 0 CHECK (max_databases >= 0),
    max_email_accounts INTEGER NOT NULL DEFAULT 0 CHECK (max_email_accounts >= 0),
    disk_quota_mb INTEGER NOT NULL DEFAULT 0 CHECK (disk_quota_mb >= 0),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE sessions ADD COLUMN impersonator_id INTEGER CHECK (impersonator_id IS NULL OR impersonator_id > 0);
CREATE INDEX idx_sessions_impersonator ON sessions(impersonator_id);

ALTER TABLE audit_logs ADD COLUMN acting_as INTEGER CHECK (acting_as IS NULL OR acting_as > 0);
