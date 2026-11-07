-- 067 (alpha.128): outgoing mail limits, the compromised-mailbox pause and
-- plan mail settings (gaps G1 and G12; owner decisions of 7 Nov 2026).
--
-- mail_outgoing_settings: the server's one row. enabled is the administrator's
-- switch (off until turned on; nothing is counted before). The defaults are
-- recipients per hour: 500 per mailbox, 750 per domain, 1000 per hosting
-- account (subscription); 0 means no limit at that level. A mailbox younger
-- than new_mailbox_hours sends at most new_mailbox_per_hour. A mailbox whose
-- hour reaches spike_factor times its own normal hour (the median of its
-- active hours of the last 7 days) and at least spike_minimum recipients, or
-- that collects refusal_pause limit refusals in one hour, is paused. The
-- apply_* columns record the last attempt to make the server hold the state
-- (applied_hash: the state the server holds; attempted_hash: the state last
-- sent; apply_state applied, refused or failed with the Agent's stable code
-- and words).
--
-- mailbox_outgoing: one mailbox's own value (NULL = the default, 0 = no
-- limit, else recipients per hour) and its security pause. A paused mailbox
-- is refused permanently until its password changes; paused_reason is spike,
-- refusals or manual; resumed_at is the last password change that ended a
-- pause (hours before it are not judged again). Deleted with its mailbox.
--
-- mail_plan_limits: plan mail settings (G12) of a customer plan
-- (service_plans.id) or a Custom subscription. mailbox_size_mb is the largest
-- mailbox size the plan allows (NULL = up to the server's 1 TB, the same as
-- before). outgoing_* replace the server defaults for that plan (NULL = the
-- server default, 0 = no limit). These keys plug into the generic ceilings
-- model (setting_ceilings, migration 066) as mail.mailbox_size_mb and
-- mail.outgoing.{mailbox,domain,subscription}; until the integrator joins
-- them the mail settings read this table. Deleted with their owner.
--
-- mail_outgoing_hourly: what each mailbox sent per UTC hour, read from
-- Postfix's own log (accepted messages and recipients, refusals by a limit,
-- by the pause, and sender-address mismatches). Kept 35 days; the report and
-- the pause read it. It never holds a message, subject or recipient address.
--
-- domain_mail_options (G25): a domain's mail service options. incoming_off
-- refuses mail from other servers for the domain (send only); unknown_reply
-- is the text refused unknown addresses get (NULL = Postfix's own). The
-- native state is CelikPanel's recipient access map in /etc/postfix; the
-- domain's mailboxes and mail are never touched. Deleted with the domain.
--
-- mail_outgoing_notices: "limit reached" (at most one per mailbox per hour
-- and person) and "mailbox paused" notices to the customer, the reseller and
-- every administrator; mail_state as usage_notices.
--
-- Additive only: six new tables, one seeded row, four triggers; no existing
-- row or value changes. The release before this one refuses a ledger that
-- carries entry 67; going back is the pre-update snapshot restore, as for
-- every migration. Nothing a hosted service needs is here: postfwd's rule
-- file and lists on the server are the native state and keep working with
-- the panel stopped or removed (D-022).

CREATE TABLE mail_outgoing_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    mailbox_per_hour INTEGER NOT NULL DEFAULT 500 CHECK (mailbox_per_hour BETWEEN 0 AND 1000000),
    domain_per_hour INTEGER NOT NULL DEFAULT 750 CHECK (domain_per_hour BETWEEN 0 AND 1000000),
    subscription_per_hour INTEGER NOT NULL DEFAULT 1000 CHECK (subscription_per_hour BETWEEN 0 AND 1000000),
    new_mailbox_per_hour INTEGER NOT NULL DEFAULT 50 CHECK (new_mailbox_per_hour BETWEEN 0 AND 1000000),
    new_mailbox_hours INTEGER NOT NULL DEFAULT 24 CHECK (new_mailbox_hours BETWEEN 0 AND 720),
    spike_factor INTEGER NOT NULL DEFAULT 5 CHECK (spike_factor BETWEEN 2 AND 100),
    spike_minimum INTEGER NOT NULL DEFAULT 100 CHECK (spike_minimum BETWEEN 10 AND 1000000),
    refusal_pause INTEGER NOT NULL DEFAULT 100 CHECK (refusal_pause BETWEEN 0 AND 1000000),
    applied_hash TEXT,
    applied_at TEXT,
    apply_state TEXT CHECK (apply_state IS NULL OR apply_state IN ('applied', 'refused', 'failed')),
    apply_code TEXT,
    apply_detail TEXT,
    attempted_at TEXT,
    attempted_hash TEXT,
    verified_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO mail_outgoing_settings (id) VALUES (1);

CREATE TABLE mailbox_outgoing (
    email_account_id INTEGER PRIMARY KEY REFERENCES email_accounts(id) ON DELETE CASCADE,
    per_hour INTEGER CHECK (per_hour IS NULL OR per_hour BETWEEN 0 AND 1000000),
    paused_at TEXT,
    paused_reason TEXT CHECK (paused_reason IS NULL OR paused_reason IN ('spike', 'refusals', 'manual')),
    paused_detail TEXT,
    resumed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE mail_plan_limits (
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('plan', 'subscription')),
    owner_id INTEGER NOT NULL CHECK (owner_id > 0),
    mailbox_size_mb INTEGER CHECK (mailbox_size_mb IS NULL OR mailbox_size_mb BETWEEN 1 AND 1048576),
    outgoing_mailbox INTEGER CHECK (outgoing_mailbox IS NULL OR outgoing_mailbox BETWEEN 0 AND 1000000),
    outgoing_domain INTEGER CHECK (outgoing_domain IS NULL OR outgoing_domain BETWEEN 0 AND 1000000),
    outgoing_subscription INTEGER CHECK (outgoing_subscription IS NULL OR outgoing_subscription BETWEEN 0 AND 1000000),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (owner_kind, owner_id)
);

CREATE TRIGGER mail_plan_limits_follow_plan
AFTER DELETE ON service_plans
BEGIN
    DELETE FROM mail_plan_limits WHERE owner_kind = 'plan' AND owner_id = OLD.id;
END;

CREATE TABLE mail_outgoing_hourly (
    address TEXT NOT NULL CHECK (length(address) BETWEEN 3 AND 320),
    hour TEXT NOT NULL,
    messages INTEGER NOT NULL DEFAULT 0 CHECK (messages >= 0),
    recipients INTEGER NOT NULL DEFAULT 0 CHECK (recipients >= 0),
    limit_refusals INTEGER NOT NULL DEFAULT 0 CHECK (limit_refusals >= 0),
    limit_kind TEXT CHECK (limit_kind IS NULL OR limit_kind IN ('mailbox', 'domain', 'subscription')),
    paused_refusals INTEGER NOT NULL DEFAULT 0 CHECK (paused_refusals >= 0),
    mismatch_refusals INTEGER NOT NULL DEFAULT 0 CHECK (mismatch_refusals >= 0),
    PRIMARY KEY (address, hour)
);
CREATE INDEX idx_mail_outgoing_hourly_hour ON mail_outgoing_hourly(hour);

CREATE TABLE mail_outgoing_notices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL CHECK (user_id > 0),
    audience TEXT NOT NULL CHECK (audience IN ('customer', 'reseller', 'administrator')),
    kind TEXT NOT NULL CHECK (kind IN ('limit_reached', 'paused')),
    address TEXT NOT NULL,
    subscription_id INTEGER NOT NULL DEFAULT 0 CHECK (subscription_id >= 0),
    period TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    mail_state TEXT NOT NULL DEFAULT 'pending' CHECK (mail_state IN ('pending', 'sent', 'not_available', 'no_address', 'failed')),
    mail_detail TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    seen_at TEXT
);
CREATE UNIQUE INDEX idx_mail_outgoing_notices_one ON mail_outgoing_notices(user_id, kind, address, period);
CREATE INDEX idx_mail_outgoing_notices_user ON mail_outgoing_notices(user_id, seen_at);

CREATE TABLE domain_mail_options (
    domain_id INTEGER PRIMARY KEY REFERENCES domains(id) ON DELETE CASCADE,
    incoming_off INTEGER NOT NULL DEFAULT 0 CHECK (incoming_off IN (0, 1)),
    unknown_reply TEXT CHECK (unknown_reply IS NULL OR length(unknown_reply) BETWEEN 1 AND 200),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TRIGGER mail_outgoing_notices_follow_user
AFTER DELETE ON users
BEGIN
    DELETE FROM mail_outgoing_notices WHERE user_id = OLD.id;
END;

CREATE TRIGGER mail_plan_limits_follow_subscription
AFTER DELETE ON subscriptions
BEGIN
    DELETE FROM mail_plan_limits WHERE owner_kind = 'subscription' AND owner_id = OLD.id;
    DELETE FROM mail_outgoing_notices WHERE subscription_id = OLD.id;
END;
