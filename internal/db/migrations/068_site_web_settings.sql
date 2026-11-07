-- 068 (alpha.128): web server settings of a site (PLESK-GAP G3, G19, G26,
-- G28; SITE-SETTINGS-DESIGN §2.4). One new table; no existing table or row
-- is changed, so the upgrade keeps all data. A release before this one
-- refuses a ledger that carries this entry; a return to it is the pre-update
-- snapshot restore, as for every migration (there is no down migration).
--
-- site_web_settings: the site's choices, sent with every render of its vhost
-- and written by the Agent as plain nginx files in the site's Panel-owned
-- directory (settings.site, directives.site). nginx reads those files, so
-- the site keeps working as configured without CelikPanel (D-022); this row
-- is the Panel's record of what the screen saved. A site without a row never
-- saved a web server setting and its vhost renders exactly as before.
--   settings_json         the fields (transport.SiteWebSettings), JSON
--   directives            the additional nginx directives text ("" = none)
--   directives_by_admin   1: the administrator saved the text, so only the
--                         syntax, size and nginx -t checks apply to it
--   directives_sha256     SHA-256 of the text (the audit names this, never
--                         the text)
--
-- 068 (alpha.128): bir sitenin web sunucusu ayarları. Tek yeni tablo; var olan
-- tablo ve satır değişmez. Satır, ekranın kaydettiğinin Panel kaydıdır; nginx
-- Agent'ın yazdığı düz dosyaları okur.
CREATE TABLE site_web_settings (
    site_id INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    settings_json TEXT NOT NULL DEFAULT '{}' CHECK (length(settings_json) <= 65536),
    directives TEXT NOT NULL DEFAULT '' CHECK (length(directives) <= 16384),
    directives_by_admin INTEGER NOT NULL DEFAULT 0 CHECK (directives_by_admin IN (0, 1)),
    directives_sha256 TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- node_site_env (PLESK-GAP G19): a Node.js site's environment variables as
-- the Panel knows them. The values themselves are in one root-owned systemd
-- EnvironmentFile on the server, which the application unit names; this row
-- holds the public values and only the NAMES of the secrets, so a secret is
-- never stored in the Panel or shown back.
--   env_public       {"NAME": "value"} of the variables not marked secret
--   env_secret_keys  ["NAME", ...] of the secrets
CREATE TABLE node_site_env (
    site_id INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    env_public TEXT NOT NULL DEFAULT '{}' CHECK (length(env_public) <= 524288),
    env_secret_keys TEXT NOT NULL DEFAULT '[]' CHECK (length(env_secret_keys) <= 65536),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- G27: site log retention. The site logs stay in /var/log/nginx and the
-- distribution's logrotate rule keeps rotating them; a plan, a subscription
-- or the server (panel setting site_logs_keep_days) may keep a site's ROTATED
-- logs for fewer days, and the Agent removes the older ones. The subscription
-- value wins over the plan's, the plan's over the server's.
CREATE TABLE log_retention_plans (
    plan_id INTEGER PRIMARY KEY REFERENCES service_plans(id) ON DELETE CASCADE,
    keep_days INTEGER NOT NULL CHECK (keep_days BETWEEN 1 AND 3650),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE log_retention_subscriptions (
    subscription_id INTEGER PRIMARY KEY REFERENCES subscriptions(id) ON DELETE CASCADE,
    keep_days INTEGER NOT NULL CHECK (keep_days BETWEEN 1 AND 3650),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
