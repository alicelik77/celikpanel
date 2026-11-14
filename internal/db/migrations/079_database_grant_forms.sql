-- 079 (alpha.134, docs/DATABASE-USERS.md "Rights by exact name"): MariaDB
-- reads a database-level grant's name as a pattern (`_` = any one
-- character), and every name CelikPanel made holds `_`, so the grants it
-- wrote before alpha.134 could also cover another customer's database.
-- From alpha.134 grants are written by the exact name (`shop\_a`.*), and the
-- panel rewrites the pattern-form grants it can prove its own: a recorded
-- user on a recorded database, from localhost or a recorded remote copy.
-- Every other pattern-form grant is only listed for the administrator.
--
-- database_grant_forms: one row per pattern-form grant row of an engine
-- (mysql.db: account, host and the name as stored), written BEFORE the
-- engine is changed (the before-image: privileges and their flags). kind:
-- ours (rewritten), owner_row (ours by record, but holding the grant option
-- or a privilege CelikPanel cannot name: left, listed), not_ours (not
-- provable: left, listed). state: captured (before-image written, nothing
-- changed yet or the change is being made), done (the exact form is there
-- and the pattern form gone; reason says converted, already_exact or gone),
-- left (listed only), blocked (three failures in a row; tried again later).
-- reached: the other databases the pattern covered when it was captured
-- (ours: kept as history, "it could also reach ...; it no longer can") or
-- covers now (left rows), at most 50 names (reached_more counts the rest).
-- username is empty for the anonymous account (''@host). Names only; never
-- a password or a hash.
--
-- database_grant_form_engines: one row per engine (the machine's own MariaDB
-- or a registered one): where the rewrite stands (waiting, running, done,
-- list_only for an engine that is not MariaDB, blocked), why, and when it
-- was last checked. A status read never starts work; the panel's own loop
-- does, after an update of the panel is verified.
--
-- Additive only: two new tables. Going back to alpha.133 (schema 077, two
-- steps back: this migration and 078) is the pre-update snapshot restore,
-- as for every migration; the engine keeps the exact-form
-- grants already written (alpha.133 reads them as described in
-- docs/DATABASE-USERS.md), and updating again resumes from the engine.
--
-- Kalıp biçimli haklar: panelin kendi kayıtlarıyla kanıtladıkları tam ada
-- çevrilir (önce görüntü yazılır), diğerleri yalnız listelenir.

CREATE TABLE database_grant_forms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    engine_key TEXT NOT NULL CHECK (length(engine_key) BETWEEN 1 AND 300),
    server_id INTEGER REFERENCES database_servers(id) ON DELETE SET NULL,
    username TEXT NOT NULL CHECK (length(username) <= 128),
    host TEXT NOT NULL CHECK (length(host) <= 255),
    stored_name TEXT NOT NULL CHECK (length(stored_name) BETWEEN 1 AND 192),
    database_name TEXT CHECK (database_name IS NULL OR length(database_name) BETWEEN 1 AND 64),
    privileges TEXT NOT NULL DEFAULT '' CHECK (length(privileges) <= 400),
    flags TEXT NOT NULL DEFAULT '' CHECK (length(flags) <= 64),
    grant_option INTEGER NOT NULL DEFAULT 0 CHECK (grant_option IN (0, 1)),
    kind TEXT NOT NULL CHECK (kind IN ('ours', 'owner_row', 'not_ours')),
    state TEXT NOT NULL CHECK (state IN ('captured', 'done', 'left', 'blocked')),
    reached TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(reached) AND length(reached) <= 8192),
    reached_more INTEGER NOT NULL DEFAULT 0 CHECK (reached_more >= 0),
    reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 64),
    failures INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    captured_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (engine_key, username, host, stored_name)
);

CREATE INDEX idx_database_grant_forms_state ON database_grant_forms(engine_key, kind, state);

CREATE TABLE database_grant_form_engines (
    engine_key TEXT PRIMARY KEY CHECK (length(engine_key) BETWEEN 1 AND 300),
    server_id INTEGER REFERENCES database_servers(id) ON DELETE SET NULL,
    state TEXT NOT NULL CHECK (state IN ('waiting', 'running', 'done', 'list_only', 'blocked')),
    reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 64),
    failures INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    version TEXT NOT NULL DEFAULT '' CHECK (length(version) <= 120),
    checked_at TEXT,
    -- Since when a right CelikPanel gave on this engine could not be read
    -- back: its account may not read mysql.db (NULL: not so).
    grants_unverifiable_since TEXT,
    -- Since when the engine has waited or been blocked for its present
    -- reason (NULL while it runs or is done): one notification once that
    -- lasts a day, and again only for a later, separate wait.
    waiting_since TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
