-- 059 (alpha.118): WordPress tools and malware scanning of hosted sites.
-- Additive only: five new tables and their indexes. No existing table,
-- trigger or value changes; no site, file, database or service changes. The
-- release before this one refuses a ledger that carries entry 59; going back
-- is the pre-update snapshot restore, as for every migration.
--
-- Columns are plain integers without REFERENCES (project note in 004). A row
-- whose domain or subscription is gone is ignored by every reader.
--
-- The two new permissions (wordpress_tools, malware_scan) need no rows: the
-- Unlimited plans and every set with "all permissions" include them; a
-- Custom set gets them only when someone adds them (D-042 P3, P7: no silent
-- change).

-- wordpress_operations: one update or copy of a WordPress site. The Panel
-- runs it in the background; polling only reads. A backup is made first
-- (backup_name) and restored when an update changed files and failed.
-- At most one open operation per site; a copy also holds its target.
CREATE TABLE wordpress_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL UNIQUE CHECK (length(operation_id) = 32),
    domain_id INTEGER NOT NULL,
    subscription_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('update', 'clone')),
    scope TEXT NOT NULL DEFAULT '' CHECK (scope IN ('', 'core', 'plugin', 'theme')),
    items TEXT NOT NULL DEFAULT '[]' CHECK (length(items) <= 8000),
    core_version TEXT NOT NULL DEFAULT '' CHECK (length(core_version) <= 40),
    target_domain_id INTEGER,
    origin TEXT NOT NULL DEFAULT 'manual' CHECK (origin IN ('manual', 'scheduled')),
    status TEXT NOT NULL CHECK (status IN (
        'queued', 'backing_up', 'updating', 'restoring', 'cloning',
        'applied', 'failed', 'rolled_back', 'needs_review')),
    backup_name TEXT CHECK (backup_name IS NULL OR length(backup_name) <= 200),
    code TEXT CHECK (code IS NULL OR length(code) <= 64),
    detail TEXT CHECK (detail IS NULL OR length(detail) <= 1000),
    requested_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX idx_wordpress_operations_open
    ON wordpress_operations(domain_id)
    WHERE status IN ('queued', 'backing_up', 'updating', 'restoring', 'cloning');
CREATE UNIQUE INDEX idx_wordpress_operations_open_target
    ON wordpress_operations(target_domain_id)
    WHERE target_domain_id IS NOT NULL
      AND status IN ('queued', 'backing_up', 'updating', 'restoring', 'cloning');
CREATE INDEX idx_wordpress_operations_domain ON wordpress_operations(domain_id, id);

-- wordpress_site_settings: the per-site switch "install minor and security
-- core releases by themselves" (with a backup first) and when it last looked.
CREATE TABLE wordpress_site_settings (
    domain_id INTEGER PRIMARY KEY,
    auto_minor_core INTEGER NOT NULL DEFAULT 0 CHECK (auto_minor_core IN (0, 1)),
    last_auto_check TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- malware_scans: one scan of a site (domain_id) or of a whole subscription
-- (domain_id NULL). One runs at a time on the server; at most one is open
-- per scope. 'prerequisite' = ClamAV was not ready (who acts: the
-- administrator); 'interrupted' = the Panel restarted during the scan.
CREATE TABLE malware_scans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL,
    domain_id INTEGER,
    origin TEXT NOT NULL DEFAULT 'manual' CHECK (origin IN ('manual', 'scheduled')),
    status TEXT NOT NULL CHECK (status IN (
        'queued', 'running', 'clean', 'infected', 'failed', 'prerequisite', 'interrupted')),
    sites_total INTEGER NOT NULL DEFAULT 0,
    sites_done INTEGER NOT NULL DEFAULT 0,
    files_scanned INTEGER NOT NULL DEFAULT 0,
    infected INTEGER NOT NULL DEFAULT 0,
    current_domain TEXT CHECK (current_domain IS NULL OR length(current_domain) <= 253),
    code TEXT CHECK (code IS NULL OR length(code) <= 64),
    detail TEXT CHECK (detail IS NULL OR length(detail) <= 1000),
    requested_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    started_at TEXT,
    finished_at TEXT
);
CREATE UNIQUE INDEX idx_malware_scans_open
    ON malware_scans(subscription_id, COALESCE(domain_id, 0))
    WHERE status IN ('queued', 'running');
CREATE INDEX idx_malware_scans_scope ON malware_scans(subscription_id, domain_id, id);

-- malware_findings: one infected file of a scan. Nothing is deleted
-- automatically: 'found' until a person quarantines it; 'deleted' only after
-- a confirmed delete from quarantine.
CREATE TABLE malware_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id INTEGER NOT NULL,
    subscription_id INTEGER NOT NULL,
    domain_id INTEGER NOT NULL,
    path TEXT NOT NULL CHECK (length(path) BETWEEN 1 AND 4096),
    signature TEXT NOT NULL CHECK (length(signature) BETWEEN 1 AND 200),
    state TEXT NOT NULL DEFAULT 'found' CHECK (state IN ('found', 'quarantined', 'restored', 'deleted')),
    quarantine_id TEXT CHECK (quarantine_id IS NULL OR length(quarantine_id) = 32),
    updated_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_malware_findings_scan ON malware_findings(scan_id, id);
CREATE INDEX idx_malware_findings_domain ON malware_findings(domain_id, state);

-- malware_notices: one notice per scan with findings and reader (the
-- customer, its reseller, each administrator). Shown in the panel until
-- seen; handed to the server's mail system when it is available.
CREATE TABLE malware_notices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    audience TEXT NOT NULL CHECK (audience IN ('customer', 'reseller', 'administrator')),
    body TEXT NOT NULL CHECK (length(body) <= 20000),
    mail_state TEXT NOT NULL DEFAULT 'pending' CHECK (mail_state IN ('pending', 'sent', 'failed', 'not_available', 'no_address')),
    mail_detail TEXT,
    seen_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (scan_id, user_id)
);
CREATE INDEX idx_malware_notices_user ON malware_notices(user_id, seen_at);
