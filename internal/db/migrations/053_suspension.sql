-- alpha.117 (subscription suspension, PLANS-AND-RIGHTS 9A). New tables only;
-- no existing row or column is changed. subscriptions.status keeps its meaning
-- (active|suspended|expired) and stays the one value other parts read; these
-- tables hold why a subscription is suspended, the operation that converges
-- the server to it, and what was there before, so a resume restores exactly.
--
-- Columns are plain integers without REFERENCES (project note in 004): the
-- rows outlive nothing, and code removes them with their subscription.
--
-- subscription_suspensions: one row per subscription that has, or had, a
-- cause. A subscription is suspended while any cause is set:
--   cause_manual       an administrator or reseller suspended it;
--   cause_reseller_id  its reseller (the account) was suspended;
--   cause_expired      its end date passed (expired_ends_at: which one).
-- expiry_waived_ends_at: an operator resumed it after that end date passed,
-- so the periodic check does not suspend it again for the same date.
CREATE TABLE subscription_suspensions (
    subscription_id INTEGER PRIMARY KEY CHECK (subscription_id > 0),
    cause_manual INTEGER NOT NULL DEFAULT 0 CHECK (cause_manual IN (0, 1)),
    cause_reseller_id INTEGER CHECK (cause_reseller_id IS NULL OR cause_reseller_id > 0),
    cause_expired INTEGER NOT NULL DEFAULT 0 CHECK (cause_expired IN (0, 1)),
    expired_ends_at TEXT,
    expiry_waived_ends_at TEXT,
    reason TEXT CHECK (reason IS NULL OR length(reason) <= 500),
    suspended_at TEXT,
    suspended_by INTEGER,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- reseller_suspensions: a reseller account whose services are suspended. Its
-- customers' subscriptions carry cause_reseller_id while this row exists.
CREATE TABLE reseller_suspensions (
    reseller_id INTEGER PRIMARY KEY CHECK (reseller_id > 0),
    reason TEXT CHECK (reason IS NULL OR length(reason) <= 500),
    suspended_at TEXT NOT NULL DEFAULT (datetime('now')),
    suspended_by INTEGER
);

-- suspension_operations: one suspend or resume of one subscription. At most
-- one is running per subscription; a crash leaves it 'running' and the Panel
-- continues the same operation at start. Polling only reads these rows.
CREATE TABLE suspension_operations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    kind TEXT NOT NULL CHECK (kind IN ('suspend', 'resume')),
    trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'reseller', 'expired')),
    actor_id INTEGER,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error_kind TEXT CHECK (error_kind IS NULL OR error_kind IN ('verified_failure', 'prerequisite', 'unknown')),
    last_error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT
);
CREATE UNIQUE INDEX idx_suspension_operations_running
    ON suspension_operations(subscription_id) WHERE state = 'running';
CREATE INDEX idx_suspension_operations_subscription
    ON suspension_operations(subscription_id, created_at);

-- suspension_effects: the checkpoints of one operation, one row per native
-- resource. pending -> captured (before-image stored) -> done; or kept (the
-- owner changed it; left alone, D-022), failed, manual (outside this server).
CREATE TABLE suspension_effects (
    operation_id TEXT NOT NULL,
    seq INTEGER NOT NULL CHECK (seq >= 0),
    kind TEXT NOT NULL CHECK (kind IN ('web', 'app', 'site_account', 'mail', 'database', 'vpn')),
    target TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ref)),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'captured', 'done', 'kept', 'failed', 'manual')),
    detail TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (operation_id, seq)
);

-- suspension_before_images: the native state found before the first
-- suspension of a subscription, per resource, as JSON (no secret is stored:
-- an expiry field, the mail refusal and nologin flags, a lock flag, a Node.js app
-- unit's running and boot state). A resume restores these and then
-- removes them.
CREATE TABLE suspension_before_images (
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    kind TEXT NOT NULL CHECK (kind IN ('app', 'site_account', 'mail', 'database')),
    target TEXT NOT NULL,
    before TEXT NOT NULL CHECK (json_valid(before)),
    captured_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (subscription_id, kind, target)
);
