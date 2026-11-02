-- 055 (alpha.117, D-043): a security emergency on one component or one
-- version of it. Additive only: six new tables and their indexes. No
-- existing table, trigger or value changes. The release before this one
-- refuses a ledger that carries entry 55; going back is the pre-update
-- snapshot restore, as for every migration.
--
-- An emergency moves through four separate steps, lightest first:
-- withdraw (nobody chooses it for anything new), stop (units stopped and
-- locked, affected sites show a maintenance page), move (the administrator
-- moves affected sites to another version, one by one) and remove (the
-- package goes; customer data stays, D-038 point 8). Re-enable puts back what
-- the stop changed. Each step that changes the server is an operation with
-- checkpoints, like suspension (053): a crash leaves it 'running' and the
-- Panel continues the same operation at start; polling only reads.
--
-- Columns are plain integers without REFERENCES (project note in 004).
--
-- security_emergencies: one row per emergency. component is php, node,
-- mariadb or postgresql; version is the PHP line ("8.5"), the exact Node.js
-- version ("22.11.0") or '' for a whole component. stage is the furthest
-- step reached; resolved closes it. At most one open emergency per target.
CREATE TABLE security_emergencies (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    component TEXT NOT NULL CHECK (component IN ('php', 'node', 'mariadb', 'postgresql')),
    version TEXT NOT NULL DEFAULT '' CHECK (length(version) <= 32),
    reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    withdrawn INTEGER NOT NULL DEFAULT 1 CHECK (withdrawn IN (0, 1)),
    stage TEXT NOT NULL DEFAULT 'withdrawn' CHECK (stage IN ('withdrawn', 'stopped', 'removed', 'resolved')),
    recommended TEXT CHECK (recommended IS NULL OR length(recommended) <= 32),
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_by INTEGER,
    resolved_at TEXT
);
CREATE UNIQUE INDEX idx_security_emergencies_open
    ON security_emergencies(component, version) WHERE stage <> 'resolved';

-- security_emergency_operations: one stop, move, remove or re-enable. At
-- most one runs per emergency. 'partial' is a finished move in which some
-- sites could not be moved (they stay on the maintenance page).
CREATE TABLE security_emergency_operations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    emergency_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('stop', 'move', 'remove', 'reenable')),
    target_version TEXT CHECK (target_version IS NULL OR length(target_version) <= 32),
    reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    keep_withdrawn INTEGER NOT NULL DEFAULT 0 CHECK (keep_withdrawn IN (0, 1)),
    actor_id INTEGER,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'completed', 'partial', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error_kind TEXT CHECK (error_kind IS NULL OR error_kind IN ('verified_failure', 'prerequisite', 'unknown')),
    last_error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT
);
CREATE UNIQUE INDEX idx_security_emergency_operations_running
    ON security_emergency_operations(emergency_id) WHERE state = 'running';
CREATE INDEX idx_security_emergency_operations_emergency
    ON security_emergency_operations(emergency_id, created_at);

-- security_emergency_effects: the checkpoints of one operation, one row per
-- native resource: the units of the component, a site's Node.js app, a
-- site's page, a site's move, the package.
CREATE TABLE security_emergency_effects (
    operation_id TEXT NOT NULL,
    seq INTEGER NOT NULL CHECK (seq >= 0),
    kind TEXT NOT NULL CHECK (kind IN ('units', 'app', 'site', 'move', 'package')),
    target TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ref)),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'captured', 'done', 'kept', 'failed', 'manual')),
    detail TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (operation_id, seq)
);

-- security_emergency_sites: the sites an emergency affects, fixed when it is
-- stopped. maintenance = 1 while the site shows the maintenance page; the
-- render reads it (and only while the site still uses the affected version,
-- so a site moved by any path shows itself again). moved_to records a move.
CREATE TABLE security_emergency_sites (
    emergency_id TEXT NOT NULL,
    domain_id INTEGER NOT NULL CHECK (domain_id > 0),
    site_id INTEGER NOT NULL CHECK (site_id > 0),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    domain TEXT NOT NULL,
    maintenance INTEGER NOT NULL DEFAULT 1 CHECK (maintenance IN (0, 1)),
    moved_to TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (emergency_id, domain_id)
);
CREATE INDEX idx_security_emergency_sites_domain ON security_emergency_sites(domain_id);

-- security_emergency_before_images: the native state found before the stop
-- (the component's units, a site's Node.js app unit), as JSON. Re-enable
-- restores these and then removes them. No secret is stored.
CREATE TABLE security_emergency_before_images (
    emergency_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('units', 'app')),
    target TEXT NOT NULL,
    before TEXT NOT NULL CHECK (json_valid(before)),
    captured_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (emergency_id, kind, target)
);

-- security_notices: what an affected customer or reseller is told, in the
-- panel (until seen) and by e-mail when the panel's mail path is available.
-- body is the notice's facts as JSON (sites, reason, alternative).
CREATE TABLE security_notices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    emergency_id TEXT NOT NULL,
    user_id INTEGER NOT NULL CHECK (user_id > 0),
    kind TEXT NOT NULL CHECK (kind IN ('emergency', 'resolved')),
    audience TEXT NOT NULL CHECK (audience IN ('customer', 'reseller')),
    body TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    mail_state TEXT NOT NULL DEFAULT 'pending' CHECK (mail_state IN ('pending', 'sent', 'not_available', 'no_address', 'failed')),
    mail_detail TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    seen_at TEXT
);
CREATE UNIQUE INDEX idx_security_notices_one ON security_notices(emergency_id, user_id, kind);
CREATE INDEX idx_security_notices_user ON security_notices(user_id, seen_at);
