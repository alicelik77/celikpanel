-- alpha.114 (Plesk-parity trio: password-protected folders, free-form
-- redirects, two-factor recovery codes). Four new tables; no existing table or
-- row is changed, so the upgrade from 046 keeps all data. A release before
-- this one refuses a ledger that carries entry 47; a return to it is the
-- pre-update snapshot restore, as for every migration (there is no down
-- migration).
--
-- site_protected_dirs / site_protected_users: a folder of a site's web
-- content that asks for a user name and password (nginx auth_basic). Only a
-- bcrypt hash is stored, never a password. The password files nginx reads are
-- written from these rows by the Agent, outside the web root.
--
-- site_redirects: a free-form redirect of one site, from a path (exactly, or
-- the folder below it) to a target address, with the status code and whether
-- the visitor's query string is kept.
--
-- two_factor_recovery_codes: single-use codes that stand in for an
-- authenticator code at sign-in. Only a SHA-256 digest of each code is stored;
-- used_at is set when a code is spent. Turning two-factor sign-in off deletes
-- the account's codes.
--
-- alpha.114 (Plesk eşdeğeri üçlü: parolalı klasörler, serbest yönlendirmeler,
-- iki faktörlü kurtarma kodları). Dört yeni tablo; var olan tablo ve satır
-- değişmez.
CREATE TABLE site_protected_dirs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    path TEXT NOT NULL CHECK (length(path) BETWEEN 2 AND 255 AND substr(path, 1, 1) = '/'),
    realm TEXT NOT NULL DEFAULT 'Restricted area' CHECK (length(realm) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (site_id, path)
);

CREATE TABLE site_protected_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dir_id INTEGER NOT NULL REFERENCES site_protected_dirs(id) ON DELETE CASCADE,
    username TEXT NOT NULL CHECK (length(username) BETWEEN 1 AND 64),
    password_hash TEXT NOT NULL CHECK (length(password_hash) = 60),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (dir_id, username)
);

CREATE INDEX idx_site_protected_users_dir ON site_protected_users(dir_id);

CREATE TABLE site_redirects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    match_type TEXT NOT NULL CHECK (match_type IN ('exact', 'prefix')),
    source_path TEXT NOT NULL CHECK (length(source_path) BETWEEN 1 AND 255 AND substr(source_path, 1, 1) = '/'),
    target_url TEXT NOT NULL CHECK (length(target_url) BETWEEN 1 AND 2048),
    status_code INTEGER NOT NULL CHECK (status_code IN (301, 302, 307, 308)),
    keep_query INTEGER NOT NULL DEFAULT 0 CHECK (keep_query IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (site_id, match_type, source_path)
);

CREATE TABLE two_factor_recovery_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL CHECK (length(code_hash) = 64),
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, code_hash)
);

CREATE INDEX idx_two_factor_recovery_codes_user ON two_factor_recovery_codes(user_id);
