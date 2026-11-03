-- 063 (alpha.119): runtime apps, a customer's own program run as a sandboxed
-- site service behind the site's HTTPS (docs/RUNTIME-APPS.md). This release
-- implements uploaded programs (kind 'program'); the kind 'python' is known to
-- the schema for the Python apps workstream. Additive only: three new tables
-- and their indexes; no existing table, trigger or value changes. The release
-- before this one refuses a ledger that carries entry 63; going back is the
-- pre-update snapshot restore, as for every migration.
--
-- Columns are plain integers without REFERENCES (project note in 004). The
-- native state (the systemd unit celikrt-<app id>, its environment file under
-- /etc/celikpanel-runtime and the app folder <site home>/app) belongs to the
-- server and keeps running without the Panel (D-022); these rows are the
-- Panel's record of it.
--
-- runtime_offerings: the administrator's offered layer (D-042 software part).
-- offering_id is 'runtime:program' or, for the Python kind,
-- 'component:python:<minor>'. A kind is closed until a row says offered = 1
-- with the risk summary digest the administrator saw. accepted_level is
-- 'reduced' only when the administrator accepted the named sandbox gaps of a
-- host that does not enforce the full sandbox (accepted_gaps, comma
-- separated); a host with a gap not listed there is refused. held = 1 is the
-- security hold: every app of the offering is stopped and none may start or
-- deploy until it is lifted. The limits are written into every unit.
CREATE TABLE runtime_offerings (
    offering_id TEXT PRIMARY KEY
        CHECK (offering_id = 'runtime:program' OR (offering_id GLOB 'component:python:3.[0-9]*' AND length(offering_id) <= 24)),
    kind TEXT NOT NULL CHECK (kind IN ('program', 'python')),
    offered INTEGER NOT NULL DEFAULT 0 CHECK (offered IN (0, 1)),
    risk_digest TEXT NOT NULL DEFAULT '' CHECK (length(risk_digest) <= 64),
    memory_mb INTEGER NOT NULL CHECK (memory_mb BETWEEN 64 AND 65536),
    cpu_percent INTEGER NOT NULL CHECK (cpu_percent BETWEEN 10 AND 1600),
    tasks_max INTEGER NOT NULL CHECK (tasks_max BETWEEN 16 AND 8192),
    accepted_level TEXT NOT NULL DEFAULT 'full' CHECK (accepted_level IN ('full', 'reduced')),
    accepted_gaps TEXT NOT NULL DEFAULT '' CHECK (length(accepted_gaps) <= 200),
    held INTEGER NOT NULL DEFAULT 0 CHECK (held IN (0, 1)),
    held_reason TEXT NOT NULL DEFAULT '' CHECK (length(held_reason) <= 500),
    changed_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- runtime_apps: one app on one domain (at most one per domain, in the site's
-- <home>/app folder). entry is the program under app/bin; args a JSON array
-- of tokens ({port} the only placeholder); env_public a JSON object of the
-- environment values that are not secret; env_secret_keys a JSON array of the
-- secret keys (their values are only in the server's root-owned file, never
-- here). port is a loopback port of 30000-34999. deployed = 1 once a deploy
-- published the app. The site's earlier project type and port are kept so a
-- removal puts the domain back as it was.
CREATE TABLE runtime_apps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL,
    domain_id INTEGER NOT NULL UNIQUE,
    site_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('program', 'python')),
    offering_id TEXT NOT NULL CHECK (length(offering_id) BETWEEN 1 AND 24),
    entry TEXT NOT NULL CHECK (length(entry) BETWEEN 1 AND 300),
    args TEXT NOT NULL DEFAULT '[]' CHECK (length(args) <= 9000),
    env_public TEXT NOT NULL DEFAULT '{}' CHECK (length(env_public) <= 300000),
    env_secret_keys TEXT NOT NULL DEFAULT '[]' CHECK (length(env_secret_keys) <= 5000),
    health_path TEXT NOT NULL DEFAULT '/' CHECK (length(health_path) BETWEEN 1 AND 201),
    port INTEGER NOT NULL UNIQUE CHECK (port BETWEEN 30000 AND 34999),
    state TEXT NOT NULL DEFAULT 'created'
        CHECK (state IN ('created', 'deploying', 'running', 'stopped', 'failed', 'removing')),
    deployed INTEGER NOT NULL DEFAULT 0 CHECK (deployed IN (0, 1)),
    previous_project_type TEXT NOT NULL DEFAULT 'php' CHECK (length(previous_project_type) <= 20),
    previous_app_port INTEGER,
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_runtime_apps_subscription ON runtime_apps(subscription_id);
CREATE INDEX idx_runtime_apps_offering ON runtime_apps(offering_id);

-- runtime_app_operations: every deploy and removal is a durable operation
-- (D-025). A crash leaves it 'running'; the Panel continues it at start from
-- its step, and every Agent step is repeatable. Polling only reads this
-- table. class says what a failure is (verified_failure, prerequisite,
-- unknown) and reason a code the screen words; message is the server's own
-- (redacted) words. No secret is ever written here.
CREATE TABLE runtime_app_operations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 8 AND 64),
    app_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('deploy', 'remove')),
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'succeeded', 'failed')),
    step TEXT NOT NULL DEFAULT 'validate' CHECK (length(step) <= 32),
    class TEXT NOT NULL DEFAULT '' CHECK (length(class) <= 32),
    reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 64),
    message TEXT NOT NULL DEFAULT '' CHECK (length(message) <= 2000),
    remove_files INTEGER NOT NULL DEFAULT 0 CHECK (remove_files IN (0, 1)),
    actor_id INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX idx_runtime_app_operations_one_running
    ON runtime_app_operations(app_id) WHERE state = 'running';
CREATE INDEX idx_runtime_app_operations_app ON runtime_app_operations(app_id, created_at);
