-- 066 (alpha.128, docs/SITE-LIMITS.md): site settings within limits, plan
-- overuse policy and the reseller overselling choice.
--
-- setting_policy: one row per limited setting key, the server level of the
-- rights chain (server ⊇ reseller ⊇ plan or Custom ⊇ site). server_ceiling
-- NULL is "no server ceiling"; server_default NULL is "the native default"
-- (php.ini, the pool, the database server). customer_editable 0 fixes the
-- key server-wide: sites show it read-only. Keys are registered in code
-- (cmd/panel/setting_limits.go); a row whose key this release does not know
-- grants and limits nothing. Other workstreams add their own keys with an
-- INSERT in their own migration (for example mail.outgoing_per_hour).
--
-- setting_ceilings: a ceiling of one owner, the same owners as rights_sets
-- ('plan', 'reseller_plan', 'subscription' = a Custom subscription,
-- 'reseller' = a Custom reseller). No row is "up to the level above". Rows
-- go with their owner (triggers as for rights_sets).
--
-- service_plans / subscriptions: overuse_policy 'close' is today's rule
-- (D-044: traffic closes the websites, 7 days over disk suspends); 'notify'
-- only sends notices. A full disk still stops new uploads in both. The
-- notify_percent_* columns replace the fixed 80 % notice level (50-95).
-- A subscription bound to a plan carries the plan's values (as max_*).
--
-- reseller_plans / reseller_pools: overselling 'allowed' is the owner's K2
-- rule (allocations may add up to more than the pool; real use may not);
-- 'not_allowed' also refuses an allocation beyond what is left.
--
-- Rights: the new permissions php_settings and php_version are added to
-- every existing rights set that lists its permissions one by one, so every
-- account keeps exactly what it could do before this release (both were
-- open to everyone). nginx_directives is not added: it is closed until the
-- administrator offers it.
--
-- Additive only: two new tables, new NOT NULL columns with defaults and new
-- rights items; no existing value changes meaning. Hosted services read
-- nothing here; limits are written into the native pool and nginx files and
-- keep working without the panel (D-022). The release before this one
-- refuses a ledger that carries entry 66; going back is the pre-update
-- snapshot restore, as for every migration.
--
-- Sınırlar içinde site ayarları, planın aşım kuralı ve bayinin dağıtım
-- seçimi. Yalnız ekleme; hizmetler buradan bir şey okumaz.

CREATE TABLE setting_policy (
    key TEXT PRIMARY KEY CHECK (length(key) BETWEEN 3 AND 64 AND key GLOB '[a-z]*.[a-z]*'),
    server_ceiling INTEGER CHECK (server_ceiling IS NULL OR server_ceiling >= 0),
    server_default INTEGER CHECK (server_default IS NULL OR server_default >= 0),
    customer_editable INTEGER NOT NULL DEFAULT 1 CHECK (customer_editable IN (0, 1)),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- The owner's ceilings of 7 Nov 2026: 512 MB memory, 300 s run time, 256 MB
-- upload, 20 PHP processes. Defaults stay native (NULL) so no site changes.
-- Scheduled tasks: at least 5 minutes between runs for new tasks (gap G5);
-- database connections and task counts have no server ceiling until the
-- administrator sets one.
INSERT INTO setting_policy (key, server_ceiling, server_default) VALUES
    ('php.memory_limit_mb', 512, NULL),
    ('php.max_execution_time_s', 300, NULL),
    ('php.upload_max_mb', 256, NULL),
    ('php.max_input_vars', 10000, NULL),
    ('php.max_children', 20, NULL),
    ('db.max_user_connections', NULL, NULL),
    ('cron.max_jobs', NULL, NULL),
    ('cron.min_interval_minutes', 5, NULL);

CREATE TABLE setting_ceilings (
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('plan', 'reseller_plan', 'subscription', 'reseller')),
    owner_id INTEGER NOT NULL CHECK (owner_id > 0),
    key TEXT NOT NULL CHECK (length(key) BETWEEN 3 AND 64),
    value INTEGER NOT NULL CHECK (value >= 0),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (owner_kind, owner_id, key)
);

CREATE TRIGGER setting_ceilings_follow_subscription
AFTER DELETE ON subscriptions
BEGIN
    DELETE FROM setting_ceilings WHERE owner_kind = 'subscription' AND owner_id = OLD.id;
END;

CREATE TRIGGER setting_ceilings_follow_plan
AFTER DELETE ON service_plans
BEGIN
    DELETE FROM setting_ceilings WHERE owner_kind = 'plan' AND owner_id = OLD.id;
END;

CREATE TRIGGER setting_ceilings_follow_reseller_plan
AFTER DELETE ON reseller_plans
BEGIN
    DELETE FROM setting_ceilings WHERE owner_kind = 'reseller_plan' AND owner_id = OLD.id;
END;

CREATE TRIGGER setting_ceilings_follow_user
AFTER DELETE ON users
BEGIN
    DELETE FROM setting_ceilings WHERE owner_kind = 'reseller' AND owner_id = OLD.id;
END;

ALTER TABLE service_plans ADD COLUMN overuse_policy TEXT NOT NULL DEFAULT 'close'
    CHECK (overuse_policy IN ('close', 'notify'));
ALTER TABLE service_plans ADD COLUMN notify_percent_disk INTEGER NOT NULL DEFAULT 80
    CHECK (notify_percent_disk BETWEEN 50 AND 95);
ALTER TABLE service_plans ADD COLUMN notify_percent_traffic INTEGER NOT NULL DEFAULT 80
    CHECK (notify_percent_traffic BETWEEN 50 AND 95);

ALTER TABLE subscriptions ADD COLUMN overuse_policy TEXT NOT NULL DEFAULT 'close'
    CHECK (overuse_policy IN ('close', 'notify'));
ALTER TABLE subscriptions ADD COLUMN notify_percent_disk INTEGER NOT NULL DEFAULT 80
    CHECK (notify_percent_disk BETWEEN 50 AND 95);
ALTER TABLE subscriptions ADD COLUMN notify_percent_traffic INTEGER NOT NULL DEFAULT 80
    CHECK (notify_percent_traffic BETWEEN 50 AND 95);

ALTER TABLE reseller_plans ADD COLUMN overselling TEXT NOT NULL DEFAULT 'allowed'
    CHECK (overselling IN ('allowed', 'not_allowed'));
ALTER TABLE reseller_pools ADD COLUMN overselling TEXT NOT NULL DEFAULT 'allowed'
    CHECK (overselling IN ('allowed', 'not_allowed'));

INSERT OR IGNORE INTO rights_set_items (set_id, item)
    SELECT id, 'permission:php_settings' FROM rights_sets WHERE all_permissions = 0;
INSERT OR IGNORE INTO rights_set_items (set_id, item)
    SELECT id, 'permission:php_version' FROM rights_sets WHERE all_permissions = 0;
