-- 071 (alpha.128, gap G16; 072 on its workstream branch, docs/FTP-LOGINS.md): additional FTP/SFTP logins
-- of a site, each closed into one folder of the site.
--
-- ftp_logins: one row per login, for the plan's count and the screen. The
-- server is the record of the login itself (the sshd drop-in's login lines,
-- the account, the jail unit; cmd/agent/ftp_logins.go): logins keep working
-- without the panel (D-022). No password is stored here. The row is written
-- before the Agent makes the login (the count is checked in the same
-- transaction) and removed when the Agent refuses or removes it; a lost
-- answer keeps it, and the screen reads the server.
--
-- A login's name is unique on the server (it is a local account), so it is
-- unique here. Rows go with their domain.
--
-- Additive only: a new table. The release before this one refuses a ledger
-- that carries entry 71; going back is the pre-update snapshot restore, as
-- for every migration.
--
-- Sitenin ek FTP/SFTP girişleri: plan sayımı ve ekran için satır. Parola
-- tutulmaz; girişin kaydı sunucudadır. Yalnız ekleme.

CREATE TABLE ftp_logins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    login TEXT NOT NULL UNIQUE CHECK (
        length(login) BETWEEN 3 AND 32
        AND login GLOB '[a-z]*'
        AND login NOT GLOB '*[^a-z0-9_-]*'
        AND instr(login, '-') > 0
    ),
    folder TEXT NOT NULL CHECK (
        substr(folder, 1, 1) = '/'
        AND length(folder) <= 512
        AND instr(folder, '..') = 0
        AND instr(folder, '//') = 0
    ),
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_ftp_logins_domain ON ftp_logins(domain_id);

-- The plan's number of logins is a key of the limits chain (066,
-- cmd/panel/ftp_logins_limit.go): 10 per subscription on the server level;
-- a plan, reseller plan or Custom set may lower it (0: none).
INSERT INTO setting_policy (key, server_ceiling, server_default) VALUES ('ftp.max_logins', 10, NULL);
