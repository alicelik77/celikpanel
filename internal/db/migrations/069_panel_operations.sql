-- 069 (alpha.128, workstream "ops"): panel sessions, notifications by
-- recipient, the customer action log and certificate requests.
--
-- Additive only: three NULL columns on sessions, three new tables and two
-- new indexes on audit_logs. No existing row or value changes. The release
-- before this one refuses a ledger that carries this entry; going back is
-- the pre-update snapshot restore, as for every migration (D-025). Nothing a
-- hosted service reads is here: sessions, notices and the action log are the
-- panel's own; a certificate request's private key file is kept by the
-- Agent outside this database and is never stored here (D-022).
--
-- 069 (alpha.128): panel oturumları, alıcıya göre bildirimler, müşteri işlem
-- günlüğü ve sertifika istekleri. Yalnız ekleme; var olan hiçbir satır
-- değişmez. Barındırılan bir hizmetin okuduğu hiçbir şey burada değildir.


-- sessions (G4): when the session was last used, from which address and with
-- which browser. last_seen_at is written at most once a minute per session;
-- a row written before this release keeps it NULL until its next use (it is
-- then set, never treated as idle). client_ip is the address the panel saw
-- (RemoteAddr); user_agent is cut to 200 characters and holds no secret. The
-- idle timeout and the panel access allowlist are panel_settings rows
-- (panel.sessions.idle_minutes, panel.access.allowlist), not schema.
ALTER TABLE sessions ADD COLUMN last_seen_at TEXT;
ALTER TABLE sessions ADD COLUMN client_ip TEXT CHECK (client_ip IS NULL OR length(client_ip) <= 64);
ALTER TABLE sessions ADD COLUMN user_agent TEXT CHECK (user_agent IS NULL OR length(user_agent) <= 200);


-- notification_preferences (G18): what one person wants emailed. mode all =
-- every notice, action = only what needs this person's action, none =
-- nothing by email (notices are still shown in the panel). extra_email is one
-- more address that receives the same mail (NULL = none). A person without a
-- row gets the default: all.
CREATE TABLE notification_preferences (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mode        TEXT    NOT NULL DEFAULT 'all' CHECK (mode IN ('all', 'action', 'none')),
    extra_email TEXT    CHECK (extra_email IS NULL OR length(extra_email) BETWEEN 3 AND 254),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);


-- account_notices (G11, G18): a notice for one person about one thing, for
-- events that are not usage notices (057) or monitoring alerts (065): a
-- subscription's end date coming (expiry_soon) or reached (expiry_today), and
-- later events of other features. needs_action says whether this person must
-- act; fix_path is the panel page where the fix is (always a local path).
-- One notice per person, kind, subject and period (the unique index): a
-- stage is never repeated. mail_state not_wanted = the person's preference
-- said not to email it.
CREATE TABLE account_notices (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audience     TEXT    NOT NULL CHECK (audience IN ('customer', 'reseller', 'administrator')),
    kind         TEXT    NOT NULL CHECK (length(kind) BETWEEN 1 AND 64),
    needs_action INTEGER NOT NULL DEFAULT 0 CHECK (needs_action IN (0, 1)),
    subject_key  TEXT    NOT NULL DEFAULT '' CHECK (length(subject_key) <= 200),
    period       TEXT    NOT NULL CHECK (length(period) BETWEEN 1 AND 64),
    body         TEXT    NOT NULL DEFAULT '{}' CHECK (length(body) <= 8192 AND json_valid(body)),
    fix_path     TEXT    NOT NULL DEFAULT '' CHECK (fix_path = '' OR (substr(fix_path, 1, 1) = '/' AND substr(fix_path, 2, 1) <> '/' AND length(fix_path) <= 300)),
    mail_state   TEXT    NOT NULL DEFAULT 'pending'
                 CHECK (mail_state IN ('pending', 'sent', 'not_wanted', 'not_available', 'no_address', 'failed')),
    mail_detail  TEXT,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    seen_at      TEXT
);
CREATE UNIQUE INDEX idx_account_notices_one ON account_notices(user_id, kind, subject_key, period);
CREATE INDEX idx_account_notices_user ON account_notices(user_id, seen_at);


-- audit_logs (G23): each account reads its own actions, and a reseller its
-- customers'. These two indexes keep those reads on an index.
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id, id);
CREATE INDEX idx_audit_logs_acting_as ON audit_logs(acting_as, id);


-- certificate_requests (G31): a certificate signing request made on this
-- server for a certificate bought elsewhere. The private key is generated
-- and kept by the Agent in a root-only file (key_file, an absolute path in
-- the Agent's private directory); it is never stored here, never sent to the
-- browser and never shown again. public_key_sha256 matches the issued
-- certificate to its key when it is uploaded. state waiting = no issued
-- certificate yet, installed = the issued certificate was installed
-- (installed_for says where), discarded = the request and its key were
-- removed by the administrator.
CREATE TABLE certificate_requests (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    common_name       TEXT    NOT NULL CHECK (length(common_name) BETWEEN 1 AND 253),
    alt_names         TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(alt_names) AND json_type(alt_names) = 'array'),
    key_type          TEXT    NOT NULL CHECK (key_type IN ('ecdsa-p256', 'rsa-2048', 'rsa-3072', 'rsa-4096')),
    csr_pem           TEXT    NOT NULL CHECK (length(csr_pem) BETWEEN 100 AND 16384),
    public_key_sha256 TEXT    NOT NULL CHECK (length(public_key_sha256) = 64),
    key_file          TEXT    NOT NULL CHECK (substr(key_file, 1, 1) = '/' AND length(key_file) <= 300),
    state             TEXT    NOT NULL DEFAULT 'waiting' CHECK (state IN ('waiting', 'installed', 'discarded')),
    created_by        INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    installed_at      TEXT,
    installed_for     TEXT    CHECK (installed_for IS NULL OR length(installed_for) <= 300),
    CHECK ((state = 'installed') = (installed_at IS NOT NULL))
);
CREATE INDEX idx_certificate_requests_state ON certificate_requests(state);
