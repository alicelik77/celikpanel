-- 054 (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md section 8): transfer of a
-- subscription to another owner, and a customer's request for one (K3).
-- Additive only: one new table, two indexes and one trigger. No existing
-- table, trigger or value changes. The release before this one refuses a
-- ledger that carries entry 54; going back is the pre-update snapshot
-- restore, as for every migration.
--
-- A transfer changes only Panel records: subscriptions.owner_id (and, when a
-- reseller's own plan cannot follow, the subscription's rights state) plus
-- the old owner's additional-user grants on it. The host layout is keyed by
-- subscription and domain ids, so nothing on the server changes
-- (cmd/panel/transfer.go says why). The whole change is one transaction;
-- this row is written in it, with the before-image, so the record and the
-- change cannot disagree.
--
-- The 029 guards on subscriptions.owner_id stay as they are: the transfer
-- removes the old owner's grants in the same transaction before it changes
-- the owner, so the guards see no conflicting grant.
--
-- status:
--   requested  a customer asked; the reseller (approver_reseller_id) or the
--              administrator (approver_reseller_id NULL) decides
--   completed  the owner was changed in the same transaction as this row
--   rejected   the approver said no (decision_reason)
--   cancelled  the requester withdrew it, or the subscription was deleted
--
-- request_id is the idempotency key of the call that made the row;
-- decision_request_id that of the approval or rejection. The same key with
-- the same content answers with the stored row and changes nothing.
--
-- Bir aboneliğin başka sahibe devri ve müşterinin devir isteği. Yalnız
-- panel kayıtları değişir; sunucuda bir şey değişmez. Satır, değişiklikle
-- aynı işlemde ve önceki hâliyle yazılır.

CREATE TABLE subscription_transfers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL UNIQUE CHECK (length(request_id) BETWEEN 8 AND 80),
    subscription_id INTEGER NOT NULL CHECK (subscription_id > 0),
    subscription_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('requested', 'completed', 'rejected', 'cancelled')),
    from_owner_id INTEGER NOT NULL CHECK (from_owner_id > 0),
    to_owner_id INTEGER CHECK (to_owner_id IS NULL OR to_owner_id > 0),
    -- The receiver as the requesting customer wrote it (free text, checked
    -- only when the approver chooses the receiver).
    requested_to TEXT NOT NULL DEFAULT '' CHECK (length(requested_to) <= 100),
    note TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    approver_reseller_id INTEGER CHECK (approver_reseller_id IS NULL OR approver_reseller_id > 0),
    requested_by INTEGER NOT NULL CHECK (requested_by > 0),
    requested_acting_as INTEGER,
    decided_by INTEGER,
    decided_acting_as INTEGER,
    decision_request_id TEXT UNIQUE CHECK (decision_request_id IS NULL OR length(decision_request_id) BETWEEN 8 AND 80),
    decision_reason TEXT NOT NULL DEFAULT '' CHECK (length(decision_reason) <= 500),
    -- JSON: owner, plan, rights state, limits and the removed grants before
    -- the change (completed rows only).
    before_image TEXT,
    -- JSON: what the transfer changed (completed rows only).
    result TEXT,
    -- When the receiver and the requester saw the outcome in the panel.
    receiver_seen_at TEXT,
    requester_seen_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    decided_at TEXT,
    CHECK (status <> 'completed' OR (to_owner_id IS NOT NULL AND before_image IS NOT NULL AND decided_at IS NOT NULL))
);

-- At most one open request per subscription.
CREATE UNIQUE INDEX idx_subscription_transfers_open
    ON subscription_transfers(subscription_id) WHERE status = 'requested';
CREATE INDEX idx_subscription_transfers_subscription ON subscription_transfers(subscription_id, id);

-- A deleted subscription leaves no open request behind; the rows stay as
-- evidence.
CREATE TRIGGER subscription_transfers_close_on_delete
AFTER DELETE ON subscriptions
BEGIN
    UPDATE subscription_transfers
    SET status = 'cancelled', decision_reason = 'the subscription was deleted', decided_at = datetime('now')
    WHERE subscription_id = OLD.id AND status = 'requested';
END;
