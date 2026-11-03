-- 061 (alpha.118): AI and automation applications that run as a site's
-- long-running service (docs/AI-APPS.md). Additive only: three new tables and
-- their indexes; no existing table, trigger or value changes. The release
-- before this one refuses a ledger that carries entry 61; going back is the
-- pre-update snapshot restore, as for every migration.
--
-- Columns are plain integers without REFERENCES (project note in 004). The
-- native state (the systemd unit celiksvc-<install id>, the application's
-- folder and environment file) belongs to the server and keeps running
-- without the Panel (D-022); these rows are the Panel's record of it.
--
-- service_app_offerings: the administrator's offered layer for service apps
-- (D-042 software part, offering id app:<app_id>). An app is closed until a
-- row says offered = 1; the row carries the risk summary digest the
-- administrator saw, so a release that changes the app's version, licence,
-- limits or risks asks again. held = 1 is the administrator's security hold:
-- every install is stopped and none may start or be installed until it is
-- lifted. The limits are the administrator's defaults for every install.
CREATE TABLE service_app_offerings (
    app_id TEXT PRIMARY KEY CHECK (length(app_id) BETWEEN 1 AND 41),
    offered INTEGER NOT NULL DEFAULT 0 CHECK (offered IN (0, 1)),
    risk_digest TEXT NOT NULL DEFAULT '' CHECK (length(risk_digest) <= 64),
    memory_mb INTEGER NOT NULL CHECK (memory_mb BETWEEN 256 AND 65536),
    cpu_percent INTEGER NOT NULL CHECK (cpu_percent BETWEEN 10 AND 1600),
    tasks_max INTEGER NOT NULL CHECK (tasks_max BETWEEN 32 AND 8192),
    held INTEGER NOT NULL DEFAULT 0 CHECK (held IN (0, 1)),
    held_reason TEXT NOT NULL DEFAULT '' CHECK (length(held_reason) <= 500),
    changed_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- service_app_installs: one application on one domain (at most one per
-- domain). version is what runs ('' until the first install succeeded);
-- target_version what the running operation installs. port and aux_port are
-- loopback ports (aux_port 0 when the app needs one port). The site's
-- earlier project type and port are kept so an uninstall puts the domain
-- back as it was.
CREATE TABLE service_app_installs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL,
    domain_id INTEGER NOT NULL UNIQUE,
    site_id INTEGER NOT NULL,
    app_id TEXT NOT NULL CHECK (length(app_id) BETWEEN 1 AND 41),
    version TEXT NOT NULL DEFAULT '' CHECK (length(version) <= 40),
    target_version TEXT NOT NULL DEFAULT '' CHECK (length(target_version) <= 40),
    port INTEGER NOT NULL UNIQUE CHECK (port BETWEEN 21000 AND 29998),
    aux_port INTEGER NOT NULL DEFAULT 0 CHECK (aux_port = 0 OR aux_port BETWEEN 21000 AND 29999),
    state TEXT NOT NULL DEFAULT 'installing'
        CHECK (state IN ('installing', 'running', 'stopped', 'failed', 'removing')),
    previous_project_type TEXT NOT NULL DEFAULT 'php' CHECK (length(previous_project_type) <= 20),
    previous_app_port INTEGER,
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_service_app_installs_subscription ON service_app_installs(subscription_id);
CREATE INDEX idx_service_app_installs_app ON service_app_installs(app_id);
CREATE UNIQUE INDEX idx_service_app_installs_aux_port ON service_app_installs(aux_port) WHERE aux_port <> 0;

-- service_app_operations: every install, update and removal is a durable
-- operation (D-025). A crash leaves it 'running'; the Panel continues it at
-- start from its step, and every Agent step is repeatable (same operation id
-- changes nothing twice). Polling only reads this table. class says what a
-- failure is (verified_failure, prerequisite, unknown) and reason a code the
-- screen words; message is the server's own (redacted) words. No secret is
-- ever written here.
CREATE TABLE service_app_operations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 8 AND 64),
    install_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('install', 'update', 'uninstall')),
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'succeeded', 'failed')),
    step TEXT NOT NULL DEFAULT 'prepare' CHECK (length(step) <= 32),
    class TEXT NOT NULL DEFAULT '' CHECK (length(class) <= 32),
    reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 64),
    message TEXT NOT NULL DEFAULT '' CHECK (length(message) <= 2000),
    remove_data INTEGER NOT NULL DEFAULT 0 CHECK (remove_data IN (0, 1)),
    actor_id INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX idx_service_app_operations_one_running
    ON service_app_operations(install_id) WHERE state = 'running';
CREATE INDEX idx_service_app_operations_install ON service_app_operations(install_id, created_at);
