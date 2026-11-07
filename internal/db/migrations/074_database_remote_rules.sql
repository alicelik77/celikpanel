-- 074 (alpha.129, docs/DATABASE-REMOTE-ACCESS.md): remote access to
-- databases.
--
-- database_remote_rules: one row per rule a customer, reseller or the
-- administrator added: one database (databases_v2), one database user
-- (database_users) and one address in canonical prefix form
-- (203.0.113.7/32, 198.51.100.0/24, 2001:db8::7/128), validated in code
-- (internal/dbremote) without CHECK, as the project rule of 004 says. The
-- engine and its port come from the database's server. Rows go with the
-- database or the user they name. created_by is the account that added the
-- rule (kept as NULL when that account is deleted).
--
-- The rows are the panel's wish; the server is the record of what is in
-- force (the pg_hba block, the MariaDB accounts, the nftables file; Agent
-- record /etc/celikpanel/database-remote.json). Those keep working without
-- the panel (D-022). A rule whose subscription lost the database_remote
-- permission, is suspended or expired, or whose server offer is off, is
-- kept and not applied.
--
-- Additive only: one new table and one index; no existing row or value
-- changes. The release before this one refuses a ledger that carries entry
-- 74; going back is the pre-update snapshot restore, as for every migration.
--
-- Veritabanlarına dışarıdan erişim kuralları: bir veritabanı, bir kullanıcı,
-- bir adres. Yalnız ekleme.

CREATE TABLE database_remote_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    database_id INTEGER NOT NULL REFERENCES databases_v2(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES database_users(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(database_id, user_id, address)
);

CREATE INDEX idx_database_remote_rules_user ON database_remote_rules(user_id);
