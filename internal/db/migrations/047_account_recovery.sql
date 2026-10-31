-- 047: Account links: password reset, invitation and email verification
-- (v0.1.0-alpha.114, P6; ROADMAP v0.3 "Customer and reseller first-class").
-- 047: Hesap bağlantıları: parola sıfırlama, davet ve e-posta doğrulama.
--
-- Additive only: one new table, one nullable column and one trigger. Existing
-- rows keep every value; every existing address starts unverified, so no
-- reset link is ever sent to an address nobody has confirmed.
-- Yalnızca eklemeli: bir tablo, bir boş bırakılabilir kolon ve bir tetikleyici.
-- Var olan her adres doğrulanmamış başlar; doğrulanmamış adrese sıfırlama
-- bağlantısı gitmez.
--
-- account_links holds the SHA-256 of each single-use link token, never the
-- token. purpose: password_reset | invitation | email_verify (validated in
-- code, no CHECK: project rule of 004). auth_epoch is the user's credential
-- epoch when the link was made: a reset or invitation link is void once the
-- password, suspension or two-factor state changed. email is the address the
-- link was sent to (or, for a copied invitation link, the account's address);
-- a link is void once the account's address differs. delivery: email | link.
-- account_links, her tek kullanımlık bağlantı jetonunun SHA-256 özetini
-- tutar, jetonun kendisini asla.
CREATE TABLE account_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    purpose TEXT NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    auth_epoch INTEGER NOT NULL,
    delivery TEXT NOT NULL DEFAULT 'email',
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    revoked_at TEXT
);

CREATE INDEX idx_account_links_user_purpose ON account_links(user_id, purpose);

-- When the person proved they receive mail at their address. NULL: not proved.
-- Kişinin adresine gelen postayı aldığını kanıtladığı an. NULL: kanıtlanmadı.
ALTER TABLE users ADD COLUMN email_verified_at TEXT;

-- Whatever path changes an address (the own-email card, an administrator or
-- reseller, a team owner, the CLI), the new address is not verified.
-- Adresi hangi yol değiştirirse değiştirsin yeni adres doğrulanmamıştır.
CREATE TRIGGER clear_email_verification_on_change
AFTER UPDATE OF email ON users
WHEN NEW.email IS NOT OLD.email
BEGIN
    UPDATE users SET email_verified_at = NULL WHERE id = NEW.id;
END;
