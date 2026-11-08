-- 079 (alpha.130, docs/MAIL-RELAY.md): outgoing mail through another server
-- (smarthost), server-wide.
--
-- mail_relay_settings: the server's one row. enabled is the administrator's
-- switch (off until turned on). host and port name the relay (validated in
-- code: a host name or IPv4 address, port 25, 465, 587 or 2525); username is
-- the relay's sign-in name. password_sealed is the relay password sealed
-- with the panel's secret key (internal/secrets); it is never sent back to a
-- screen and is cleared when the relay is turned off. The apply_* columns
-- record the last attempt to make the server hold the state (apply_state
-- applied, refused, failed or unknown, with the Agent's stable code and
-- words, never a password).
--
-- The row is the panel's wish; Postfix's main.cf, master.cf and the
-- root-only map /etc/postfix/celikpanel-relay-sasl are the native state and
-- keep relaying with the panel stopped or removed (D-022).
--
-- Additive only: three new tables and two seeded rows; no existing row or
-- value changes. The release before this one refuses a ledger that carries
-- this entry; going back is the pre-update snapshot restore, as for every
-- migration.
--
-- Postanın başka bir sunucu üzerinden gönderilmesi: sunucunun tek satırı.
-- Parola panelin anahtarıyla mühürlüdür, ekrana geri dönmez. Yalnız ekleme.

CREATE TABLE mail_relay_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 587,
    username TEXT NOT NULL DEFAULT '',
    password_sealed TEXT NOT NULL DEFAULT '',
    apply_state TEXT NOT NULL DEFAULT '',
    apply_code TEXT NOT NULL DEFAULT '',
    apply_detail TEXT NOT NULL DEFAULT '',
    applied_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO mail_relay_settings (id) VALUES (1);

-- mail_server_options: the server's one row of the other mail server
-- settings (docs/MAIL-SERVER-SETTINGS-DESIGN.md). subaddressing is the
-- administrator's switch for user+tag@domain delivery (Postfix
-- recipient_delimiter = +). The *_apply_* columns record the last attempt to
-- make the server hold subaddressing and the sender list (state applied,
-- refused, failed or unknown, with the Agent's stable code and words).
--
-- mail_sender_rules: the server's sender allow and block list, one address
-- (user@example.com) or whole domain (example.com, with its subdomains) per
-- row, validated in code; action allow or block. created_by is the
-- administrator who added it (kept as NULL when that account is deleted).
-- The native state is CelikPanel's access map in /etc/postfix; an empty list
-- leaves nothing there.
--
-- Diğer posta sunucusu ayarları: alt adresleme anahtarı ve sunucunun
-- gönderen izin/engel listesi. Yalnız ekleme.

CREATE TABLE mail_server_options (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    subaddressing INTEGER NOT NULL DEFAULT 0 CHECK (subaddressing IN (0, 1)),
    subaddressing_apply_state TEXT NOT NULL DEFAULT '',
    subaddressing_apply_code TEXT NOT NULL DEFAULT '',
    subaddressing_apply_detail TEXT NOT NULL DEFAULT '',
    sender_apply_state TEXT NOT NULL DEFAULT '',
    sender_apply_code TEXT NOT NULL DEFAULT '',
    sender_apply_detail TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO mail_server_options (id) VALUES (1);

CREATE TABLE mail_sender_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    value TEXT NOT NULL UNIQUE,
    action TEXT NOT NULL CHECK (action IN ('allow', 'block')),
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
