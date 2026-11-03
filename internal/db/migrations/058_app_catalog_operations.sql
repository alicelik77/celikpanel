-- 058 (alpha.118, docs/APP-CATALOG.md): the application catalog. One durable
-- row per installation of a catalog application (PrestaShop, Nextcloud,
-- Drupal, Joomla, MediaWiki, phpBB). WordPress keeps its own table
-- (application_install_operations, 028) and installer unchanged. A new table
-- only: no existing row or column changes. Going back to the release before
-- is the pre-update snapshot restore, as for every migration; an older Panel
-- does not read this table, and the installed applications keep running
-- (they are ordinary files, a database and an nginx include of the site).
--
-- The row is written before any privileged step and moves through:
--   reserved -> database_creating -> database_ready -> files_installing
--   -> files_ready -> setting_up -> applied
-- with setup_failed (files in place, the site closed with a "being set up"
-- page, Retry runs the setup again), failed (nothing left on the server) and
-- needs_review (an answer was lost; the administrator reconciles), then
-- removing -> removed. A domain holds at most one live application: the
-- partial unique index refuses a second install at the database itself.
--
-- admin_secret_encrypted holds the generated administrator password, sealed
-- with the Panel's secret key, only until it was shown once; it is then
-- cleared. No password is ever stored in plain text or logged.
--
-- Katalog uygulamaları: her kurulum için bir kalıcı satır. Yalnız yeni tablo;
-- eski satır ve sütun değişmez. Bir alan adında en çok bir canlı uygulama.

CREATE TABLE app_catalog_operations (
    operation_id TEXT PRIMARY KEY
        CHECK (length(operation_id) = 32 AND operation_id NOT GLOB '*[^a-f0-9]*'),
    app_id TEXT NOT NULL
        CHECK (length(app_id) BETWEEN 2 AND 41 AND app_id NOT GLOB '*[^a-z0-9-]*'),
    app_version TEXT NOT NULL CHECK (length(app_version) BETWEEN 1 AND 40),
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    database_server_id INTEGER NOT NULL REFERENCES database_servers(id),
    database_id INTEGER,
    database_user_id INTEGER,
    database_name TEXT NOT NULL,
    database_user TEXT NOT NULL,
    cleanup_token_encrypted TEXT NOT NULL,
    admin_secret_encrypted TEXT,
    admin_path TEXT,
    status TEXT NOT NULL CHECK (status IN (
        'reserved', 'database_creating', 'database_ready', 'files_installing',
        'files_ready', 'setting_up', 'setup_failed', 'applied',
        'removing', 'removed', 'failed', 'needs_review'
    )),
    error_kind TEXT,
    last_error TEXT,
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_app_catalog_operations_live_domain
    ON app_catalog_operations(domain_id) WHERE status NOT IN ('failed', 'removed');

CREATE INDEX idx_app_catalog_operations_subscription
    ON app_catalog_operations(subscription_id, status);

CREATE INDEX idx_app_catalog_operations_status
    ON app_catalog_operations(status, updated_at);
