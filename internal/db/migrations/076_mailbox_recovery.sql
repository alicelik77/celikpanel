-- 076 (alpha.130): mailbox recovery address and reset link
-- (ROADMAP alpha.130 Mail item 4; docs/MAILBOX-RECOVERY-REVIEW.md).
-- 076: posta kutusu kurtarma adresi ve sıfırlama bağlantısı.
--
-- Additive only: two new tables. No existing row or column changes; nothing a
-- hosted service reads is here (the mailbox password stays in Dovecot's
-- passwd-file, written only by the Agent). The release before this one
-- refuses a ledger that carries this entry; going back is the pre-update
-- snapshot restore, as for every migration (D-025).
-- Yalnızca eklemeli: iki yeni tablo. Var olan satır ya da kolon değişmez;
-- barındırılan hiçbir hizmet buradan okumaz.
--
-- mailbox_recovery_addresses: at most one recovery address per mailbox.
--   address          the canonical address (lower case) links are sent to;
--   subscription_id  the mailbox domain's subscription when it was set,
--   owner_user_id    that subscription's owner when it was set: a transfer
--                    to another subscription or owner voids the address;
--   local_domain_id  when the address's own domain is hosted here, that
--                    domain's row at confirmation: deleted and added again
--                    (a new row) voids the address;
--   confirmed_at     NULL until the link mailed to the address was opened;
--                    an unconfirmed address never receives a reset link;
--   lang             the language of the mail it receives (en|tr).
-- mailbox_recovery_addresses: posta kutusu başına en çok bir kurtarma adresi.
-- Onaylanmamış adrese sıfırlama bağlantısı gitmez; abonelik ya da sahibi
-- değişince adres geçersizdir.
CREATE TABLE mailbox_recovery_addresses (
    email_account_id INTEGER PRIMARY KEY REFERENCES email_accounts(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    subscription_id INTEGER NOT NULL,
    owner_user_id INTEGER NOT NULL,
    local_domain_id INTEGER,
    lang TEXT NOT NULL DEFAULT 'en',
    set_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    set_at TEXT NOT NULL,
    confirmed_at TEXT
);

-- mailbox_recovery_links: the SHA-256 of each single-use token, never the
-- token. purpose: confirm (24 h, confirms the address it was sent to) |
-- reset (30 min, sets a new mailbox password). address is where the link
-- was sent; a link is void once the mailbox's recovery address differs.
-- password_fingerprint (reset only) is the Agent's fingerprint of the stored
-- password field when the link was made; the reset applies only while it is
-- unchanged. Validated in code, no CHECK (project rule of 004).
-- mailbox_recovery_links: tek kullanımlık jetonun yalnız SHA-256 özeti.
CREATE TABLE mailbox_recovery_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    purpose TEXT NOT NULL,
    email_account_id INTEGER NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    password_fingerprint TEXT,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    revoked_at TEXT
);

CREATE INDEX idx_mailbox_recovery_links_account ON mailbox_recovery_links(email_account_id, purpose);
