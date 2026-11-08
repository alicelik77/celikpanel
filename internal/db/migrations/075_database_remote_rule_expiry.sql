-- 075 (alpha.130, docs/DATABASE-REMOTE-ACCESS.md "Timed access"): an
-- address in remote access to databases may be allowed for a limited time.
--
-- database_remote_rules.expires_at: when the rule ends, as UTC text
-- ('YYYY-MM-DD HH:MM:SS', the form of created_at); NULL = until removed.
-- The customer chooses 1 day (the default), 7 days or no end when adding an
-- address and may choose again later. The panel's sync loop removes an
-- expired rule exactly like a manual removal (the address filter, the
-- pg_hba line or MariaDB account go) and writes an audit entry; a rule past
-- its end is never applied even before that removal.
--
-- Additive only: one new nullable column. Every rule of alpha.129 keeps
-- NULL, so nothing that is open closes by this update. The release before
-- this one refuses a ledger that carries entry 75; going back is the
-- pre-update snapshot restore, as for every migration.
--
-- Süreli erişim: bir adres sınırlı süre için açılabilir; NULL süresizdir.
-- Yalnız ekleme; mevcut kurallar süresiz kalır.

ALTER TABLE database_remote_rules ADD COLUMN expires_at TEXT;

CREATE INDEX idx_database_remote_rules_expiry ON database_remote_rules(expires_at) WHERE expires_at IS NOT NULL;
