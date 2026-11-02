-- 052 (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md): plans, rights and the
-- room to play. New tables and new NULL columns; no column is dropped and
-- no existing value is changed. The only rows added to an old table are the
-- 'Unlimited' subscriptions of step 3 (with their sqlite_sequence counter).
-- The release before this one refuses a ledger that carries entry 52; going
-- back is the pre-update snapshot restore, as for every migration, and it
-- drops everything written after the update.
--
-- A rights set has three parts. Quantities stay where the quota checks
-- already read them (service_plans.max_*, subscriptions.max_*,
-- reseller_pools), so one number has one home. Permissions and software live
-- here: rights_sets holds the two "everything offered" flags of one owner,
-- rights_set_items the chosen items ("permission:<id>" or an offering id of
-- D-017 such as "component:php-fpm:8.3").
--
-- Owners: 'plan' (a customer plan, service_plans.id), 'reseller_plan'
-- (reseller_plans.id), 'subscription' (a Custom subscription) and 'reseller'
-- (a Custom reseller, users.id). The owner id is a plain integer; triggers
-- below remove a set together with its owner.
--
-- States, read by code (cmd/panel/rights_model.go):
--   subscription: plan_id set -> bound to that plan; else a 'subscription'
--     set -> Custom; else Unlimited (the built-in plan, not a row).
--   reseller: a reseller_plan_bindings row -> bound; else a 'reseller' set ->
--     Custom; else Unlimited reseller.
--
-- Bir hak kümesi: sayılar mevcut yerlerinde kalır; izinler ve yazılımlar
-- burada. Yalnız ekleme; eski sürüme dönüş anlık görüntü geri yüklemesidir.

CREATE TABLE rights_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('plan', 'reseller_plan', 'subscription', 'reseller')),
    owner_id INTEGER NOT NULL CHECK (owner_id > 0),
    all_permissions INTEGER NOT NULL DEFAULT 0 CHECK (all_permissions IN (0, 1)),
    all_software INTEGER NOT NULL DEFAULT 0 CHECK (all_software IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (owner_kind, owner_id)
);

CREATE TABLE rights_set_items (
    set_id INTEGER NOT NULL REFERENCES rights_sets(id) ON DELETE CASCADE,
    item TEXT NOT NULL CHECK (length(item) BETWEEN 3 AND 200),
    PRIMARY KEY (set_id, item)
);

-- Reseller plans: the administrator's templates for resellers. Quantities
-- are totals over the reseller's tree; 0 is unlimited. The built-in
-- "Unlimited reseller" is not a row.
CREATE TABLE reseller_plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
    max_customers INTEGER NOT NULL DEFAULT 0 CHECK (max_customers >= 0),
    max_domains INTEGER NOT NULL DEFAULT 0 CHECK (max_domains >= 0),
    max_databases INTEGER NOT NULL DEFAULT 0 CHECK (max_databases >= 0),
    max_email_accounts INTEGER NOT NULL DEFAULT 0 CHECK (max_email_accounts >= 0),
    disk_quota_mb INTEGER NOT NULL DEFAULT 0 CHECK (disk_quota_mb >= 0),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- A reseller bound to a reseller plan. A plan with a bound reseller cannot
-- be deleted (no ON DELETE action).
CREATE TABLE reseller_plan_bindings (
    reseller_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    reseller_plan_id INTEGER NOT NULL REFERENCES reseller_plans(id),
    bound_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_reseller_plan_bindings_plan ON reseller_plan_bindings(reseller_plan_id);

-- Notices the screens read: accounts that became Unlimited in this
-- migration, and reseller pools at 80% / 100% of real use. One open notice
-- per kind, account and subject. Kinds are validated in code.
CREATE TABLE rights_notices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    user_id INTEGER NOT NULL CHECK (user_id > 0),
    subject TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    acknowledged_at TEXT
);
CREATE UNIQUE INDEX idx_rights_notices_open ON rights_notices(kind, user_id, subject) WHERE acknowledged_at IS NULL;
CREATE INDEX idx_rights_notices_user ON rights_notices(user_id);

-- Subscription lifecycle, ready for a future billing system (K7). status
-- keeps its 001 values (active, suspended, expired). ends_at NULL = never.
-- starts_at NULL on an older row means created_at. suspend_reason is one of
-- manual, expired, payment, security (validated in code). The 001 column
-- expires_at was never read or written by the Panel and is not copied: a
-- value in it never ended anything, and as ends_at it would suspend that
-- subscription's services at the first start after the update. Every
-- existing subscription starts with no end date (K7) and expires_at stays
-- unused.
ALTER TABLE subscriptions ADD COLUMN starts_at TEXT;
ALTER TABLE subscriptions ADD COLUMN ends_at TEXT;
ALTER TABLE subscriptions ADD COLUMN suspend_reason TEXT;
ALTER TABLE subscriptions ADD COLUMN suspended_at TEXT;

CREATE TRIGGER rights_sets_follow_subscription
AFTER DELETE ON subscriptions
BEGIN
    DELETE FROM rights_sets WHERE owner_kind = 'subscription' AND owner_id = OLD.id;
END;

CREATE TRIGGER rights_sets_follow_plan
AFTER DELETE ON service_plans
BEGIN
    DELETE FROM rights_sets WHERE owner_kind = 'plan' AND owner_id = OLD.id;
END;

CREATE TRIGGER rights_sets_follow_reseller_plan
AFTER DELETE ON reseller_plans
BEGIN
    DELETE FROM rights_sets WHERE owner_kind = 'reseller_plan' AND owner_id = OLD.id;
END;

CREATE TRIGGER rights_sets_follow_user
AFTER DELETE ON users
BEGIN
    DELETE FROM rights_sets WHERE owner_kind = 'reseller' AND owner_id = OLD.id;
    DELETE FROM rights_notices WHERE user_id = OLD.id;
END;

-- Migration of existing records (PLANS-AND-RIGHTS.md section 9). No site,
-- mail, database or service changes.

-- 1. Every plan keeps its numbers; its permissions and software are
--    "everything the administrator offers", so nothing bound to it changes.
INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software)
SELECT 'plan', id, 1, 1 FROM service_plans;

-- 2. A subscription without a plan becomes Custom with today's numbers and
--    everything offered. The administrator's own workspace stays Unlimited.
--    VPN and the app installer stay what the add-on ledger says: on only
--    where the subscription holds an active grant.
INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software)
SELECT 'subscription', s.id, 0, 1
FROM subscriptions s JOIN users u ON u.id = s.owner_id
WHERE s.plan_id IS NULL
  AND NOT (u.role = 'admin' AND s.name = 'Admin Subscription');

INSERT INTO rights_set_items (set_id, item)
SELECT rs.id, 'permission:' || p.column1
FROM rights_sets rs,
     (VALUES ('dns_edit'), ('dnssec'), ('mail'), ('databases'), ('cron'),
             ('file_manager'), ('backups'), ('ssl_acme'), ('ssl_custom'),
             ('site_protect_redirects'), ('site_proxy'), ('team_members'),
             ('file_access'), ('ssh')) p
WHERE rs.owner_kind = 'subscription';

INSERT INTO rights_set_items (set_id, item)
SELECT rs.id, 'permission:' || e.product_id
FROM rights_sets rs
JOIN subscription_entitlements e ON e.subscription_id = rs.owner_id
WHERE rs.owner_kind = 'subscription'
  AND e.product_id IN ('vpn', 'app_installer')
  AND e.status = 'active';

-- 3. A customer without a subscription becomes Unlimited: it gets one
--    subscription with every limit 0 (unlimited). Today such an account can
--    create nothing; afterwards it can, so the administrator is shown the
--    list (notice first, while the condition still holds).
INSERT INTO rights_notices (kind, user_id, subject, detail)
SELECT 'unlimited_after_migration', u.id, 'customer', u.username
FROM users u
WHERE u.role = 'customer'
  AND COALESCE(NULLIF(u.account_type, ''), 'account') = 'account'
  AND NOT EXISTS (SELECT 1 FROM subscriptions s WHERE s.owner_id = u.id);

INSERT INTO subscriptions
    (owner_id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb, status, starts_at)
SELECT u.id, 'Unlimited', 0, 0, 0, 0, 0, 'active', datetime('now')
FROM users u
WHERE u.role = 'customer'
  AND COALESCE(NULLIF(u.account_type, ''), 'account') = 'account'
  AND NOT EXISTS (SELECT 1 FROM subscriptions s WHERE s.owner_id = u.id);

-- 4. A reseller without a pool is the Unlimited reseller (nothing to
--    write). A reseller with a pool becomes a Custom reseller: the pool
--    numbers stay in reseller_pools, permissions and software are
--    everything offered.
INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software)
SELECT 'reseller', rp.reseller_id, 1, 1
FROM reseller_pools rp JOIN users u ON u.id = rp.reseller_id
WHERE u.role = 'reseller';
