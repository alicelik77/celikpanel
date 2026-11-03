-- 057 (alpha.118, D-044, docs/PLANS-AND-RIGHTS.md section 4.1; owner
-- decisions of 3 November 2026): plan limits become real. Traffic of every
-- website is measured (outgoing bytes to visitors, per calendar month) and
-- disk is measured per subscription (site files, mail and databases).
--
-- Additive, with one deliberate value change: every stored traffic limit
-- (subscriptions.bandwidth_quota_mb, service_plans.bandwidth_quota_mb) is set
-- to 0, "unlimited". The old values were never shown on any screen and never
-- applied (most are the 102400 column default of 001/004), so enforcing them
-- now would close websites on a number nobody chose. Behaviour is unchanged:
-- unlimited before, unlimited after. The administrator is told once
-- (rights_notices 'traffic_measured') and sets real limits in the plans. No
-- site, mailbox, database or service changes. The release before this one
-- refuses a ledger that carries entry 57; going back is the pre-update
-- snapshot restore, as for every migration.
--
-- Trafik ve disk artık ölçülür. Kayıtlı trafik sınırları 0'a (sınırsız)
-- çekilir: eski değerler hiç gösterilmedi ve hiç uygulanmadı.

-- traffic_daily: bytes per website per day (server local date, YYYY-MM-DD).
-- Written only by the collector (cmd/panel/traffic_collect.go) inside the
-- same transaction that records the Agent batch sequence, so a batch is
-- counted exactly once. subscription_id is the domain's subscription when
-- the bytes were counted.
CREATE TABLE traffic_daily (
    domain_id INTEGER NOT NULL CHECK (domain_id > 0),
    day TEXT NOT NULL CHECK (length(day) = 10),
    source TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web')),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    bytes_out INTEGER NOT NULL DEFAULT 0 CHECK (bytes_out >= 0),
    bytes_in INTEGER NOT NULL DEFAULT 0 CHECK (bytes_in >= 0),
    requests INTEGER NOT NULL DEFAULT 0 CHECK (requests >= 0),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (domain_id, day, source)
);
CREATE INDEX idx_traffic_daily_subscription ON traffic_daily(subscription_id, day);

-- traffic_collect_state: the one collector's place in the Agent's batch
-- sequence. agent_epoch names the Agent's store; a new store (a reinstalled
-- Agent) starts a new sequence and is recognised by its epoch.
CREATE TABLE traffic_collect_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    agent_epoch TEXT NOT NULL DEFAULT '',
    last_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_seq >= 0),
    last_success_at TEXT,
    last_attempt_at TEXT,
    last_error TEXT,
    logrotate TEXT
);
INSERT INTO traffic_collect_state (id) VALUES (1);

-- traffic_sources: whether each website's traffic is being measured, as the
-- Agent last reported it: measuring, or unknown with a reason code (the
-- traffic log setting is missing, the owner changed CelikPanel's file, ...).
CREATE TABLE traffic_sources (
    domain_id INTEGER PRIMARY KEY CHECK (domain_id > 0),
    state TEXT NOT NULL CHECK (state IN ('measuring', 'not_counted', 'unknown')),
    reason TEXT,
    checked_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- traffic_web_blocks: a subscription whose websites show the "traffic limit
-- reached" page this period (web only: mail, databases and file access keep
-- working). cause: 'subscription' (its own limit) or 'pool' (its reseller's
-- pool). wanted is what the collector decided; applied is what the vhosts
-- were last rendered with, so a render that failed is retried.
CREATE TABLE traffic_web_blocks (
    subscription_id INTEGER PRIMARY KEY CHECK (subscription_id > 0),
    period TEXT NOT NULL CHECK (length(period) = 7),
    cause TEXT NOT NULL CHECK (cause IN ('subscription', 'pool')),
    wanted INTEGER NOT NULL DEFAULT 1 CHECK (wanted IN (0, 1)),
    applied INTEGER NOT NULL DEFAULT 0 CHECK (applied IN (0, 1)),
    since TEXT NOT NULL DEFAULT (datetime('now')),
    last_error TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- subscription_disk_usage: the last measurement of one subscription. state
-- 'measured' or 'unknown' (detail says why). over_since: the first
-- measurement that found it over its disk limit in the current episode; the
-- 7-day countdown starts there and ends when a measurement is under the limit.
-- upload_blocked: at or over the limit, new uploads are closed: the panel's
-- file manager refuses them, FTP and SFTP are read-only and PHP takes no
-- uploaded file. upload_applied is what the server was last brought to
-- (upload_error says why it could not be), so a failure is retried.
CREATE TABLE subscription_disk_usage (
    subscription_id INTEGER PRIMARY KEY CHECK (subscription_id > 0),
    web_bytes INTEGER NOT NULL DEFAULT 0 CHECK (web_bytes >= 0),
    mail_bytes INTEGER NOT NULL DEFAULT 0 CHECK (mail_bytes >= 0),
    db_bytes INTEGER NOT NULL DEFAULT 0 CHECK (db_bytes >= 0),
    method TEXT NOT NULL DEFAULT 'walk' CHECK (method IN ('walk')),
    state TEXT NOT NULL DEFAULT 'unknown' CHECK (state IN ('measured', 'partial', 'unknown')),
    detail TEXT,
    measured_at TEXT,
    attempted_at TEXT,
    requested_at TEXT,
    over_since TEXT,
    upload_blocked INTEGER NOT NULL DEFAULT 0 CHECK (upload_blocked IN (0, 1)),
    upload_applied INTEGER NOT NULL DEFAULT 0 CHECK (upload_applied IN (0, 1)),
    upload_error TEXT
);

-- usage_notices: what a customer, its reseller and the administrator are
-- told about traffic and disk (80%, 100%, the disk countdown and a disk
-- suspension). period keeps one notice per episode: the month for traffic,
-- the over_since date for disk. Shown in the panel until seen and handed to
-- the server's own mail system when the panel's account mail is available.
CREATE TABLE usage_notices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL CHECK (user_id > 0),
    audience TEXT NOT NULL CHECK (audience IN ('customer', 'reseller', 'administrator')),
    kind TEXT NOT NULL CHECK (kind IN ('traffic_80', 'traffic_100', 'pool_traffic_80', 'pool_traffic_100',
                                       'disk_80', 'disk_100', 'disk_countdown', 'disk_suspended')),
    subscription_id INTEGER NOT NULL DEFAULT 0 CHECK (subscription_id >= 0),
    reseller_id INTEGER NOT NULL DEFAULT 0 CHECK (reseller_id >= 0),
    period TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    mail_state TEXT NOT NULL DEFAULT 'pending' CHECK (mail_state IN ('pending', 'sent', 'not_available', 'no_address', 'failed')),
    mail_detail TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    seen_at TEXT
);
CREATE UNIQUE INDEX idx_usage_notices_one ON usage_notices(user_id, kind, subscription_id, reseller_id, period);
CREATE INDEX idx_usage_notices_user ON usage_notices(user_id, seen_at);

-- The reseller pool's traffic (K2: the real total of the tree, per month)
-- and a reseller plan's. 0 is unlimited.
ALTER TABLE reseller_pools ADD COLUMN traffic_quota_mb INTEGER NOT NULL DEFAULT 0 CHECK (traffic_quota_mb >= 0);
ALTER TABLE reseller_plans ADD COLUMN traffic_quota_mb INTEGER NOT NULL DEFAULT 0 CHECK (traffic_quota_mb >= 0);

-- The disk cause of a suspension: over the disk limit for 7 days. It is
-- cleared when a measurement is under the limit; an operator's resume clears
-- it and starts a new 7-day period. It never clears another cause.
ALTER TABLE subscription_suspensions ADD COLUMN cause_disk INTEGER NOT NULL DEFAULT 0 CHECK (cause_disk IN (0, 1));

-- Never shown, never applied: unlimited (see the head of this file).
UPDATE subscriptions SET bandwidth_quota_mb = 0 WHERE bandwidth_quota_mb IS NULL OR bandwidth_quota_mb <> 0;
UPDATE service_plans SET bandwidth_quota_mb = 0 WHERE bandwidth_quota_mb IS NULL OR bandwidth_quota_mb <> 0;

INSERT INTO rights_notices (kind, user_id, subject, detail)
SELECT 'traffic_measured', u.id, 'traffic', 'Traffic is measured from this release; every traffic limit starts as unlimited.'
FROM users u WHERE u.role = 'admin';
