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
