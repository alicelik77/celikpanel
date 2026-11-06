-- 065 (alpha.127): monitoring stage 2 and 3 - machine detail, alerts and
-- site checks.
--
-- metrics_samples gains four NULL columns, the network and disk throughput
-- of the 30 seconds the row covers, in bytes per second: net_rx/net_tx from
-- /proc/net/dev (every interface but loopback) and disk_read/disk_write from
-- /proc/diskstats (whole disks, not their partitions). Rows written before
-- this release keep them NULL ("not measured", never zero).
--
-- metrics_process_snapshots: the five busiest processes, captured by the
-- sampler when the 30-second CPU average or memory use reaches the alert
-- level (at most once per ten minutes for each). processes is a JSON array of
-- {pid, name, user, cpu, rss}; the name is the kernel's short process name
-- (/proc/<pid>/stat), never the command line, so no argument (a password on
-- a command line) is ever stored. Kept 30 days like the hourly history.
--
-- monitoring_alerts: one row per alert. A "Problems now" item that is still
-- there after the owner's delay (default 10 minutes) opens one; it stays one
-- row while the problem lasts (key de-duplicates) and closes only after the
-- problem has been gone for the clear delay (hysteresis). state pending is a
-- problem seen but not yet past the delay; it is deleted when the problem
-- goes away before that. mail_state says what happened to the email: none
-- (in the panel only), pending, sent, suppressed (the same key was emailed
-- in the last six hours, or the hourly cap was reached), not_confirmed (no
-- administrator has a confirmed address), not_available (no mail system the
-- panel can hand mail to) or failed (the hand-over was refused; it is not
-- tried again, the panel says so). At most one pending or open row per key.
--
-- site_uptime: the latest check of each hosted site, made by the panel from
-- the server itself every five minutes (HTTP(S) on the loopback address with
-- the site's name), with the certificate the site serves. down_since is the
-- first failed check of the current failure.
--
-- Additive only: four NULL columns on metrics_samples and three new tables;
-- no existing row or value changes. The release before this one refuses a
-- ledger that carries entry 65; going back is the pre-update snapshot
-- restore, as for every migration. Nothing a hosted service reads is here:
-- alerts are optional panel automation (D-022); with the panel stopped there
-- are no alerts, and the owner's own services run on.
--
-- 065 (alpha.127): izleme 2. ve 3. aşama. metrics_samples dört NULL sütun
-- alır (ağ ve disk hızı, bayt/saniye); eski satırlarda NULL kalır.
-- metrics_process_snapshots: işlemci ya da bellek uyarı düzeyine gelince en
-- yoğun beş işlem (yalnız kısa işlem adı, komut satırı asla). monitoring_alerts:
-- gecikmeden sonra hâlâ süren sorun bir uyarı açar; sorun sürdükçe tek satır
-- kalır, sorun gecikme kadar yok olunca kapanır. site_uptime: her sitenin
-- sunucunun kendisinden yapılan son denetimi. Yalnız ekleme; eski hiçbir
-- satır değişmez. Panel dururken uyarı yoktur; sahibin hizmetleri sürer.
ALTER TABLE metrics_samples ADD COLUMN net_rx REAL;
ALTER TABLE metrics_samples ADD COLUMN net_tx REAL;
ALTER TABLE metrics_samples ADD COLUMN disk_read REAL;
ALTER TABLE metrics_samples ADD COLUMN disk_write REAL;

CREATE TABLE metrics_process_snapshots (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        TEXT    NOT NULL CHECK (length(ts) = 20 AND ts GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z'),
    reason    TEXT    NOT NULL CHECK (reason IN ('cpu', 'memory')),
    value     REAL    NOT NULL,
    processes TEXT    NOT NULL CHECK (json_valid(processes) AND json_type(processes) = 'array')
);
CREATE INDEX idx_metrics_process_snapshots_ts ON metrics_process_snapshots(ts);

CREATE TABLE monitoring_alerts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key         TEXT    NOT NULL CHECK (length(key) BETWEEN 1 AND 300),
    kind        TEXT    NOT NULL CHECK (length(kind) BETWEEN 1 AND 64),
    severity    TEXT    NOT NULL CHECK (severity IN ('warning', 'critical')),
    subject     TEXT    NOT NULL DEFAULT '',
    name        TEXT    NOT NULL DEFAULT '',
    value       REAL    NOT NULL DEFAULT 0,
    state       TEXT    NOT NULL CHECK (state IN ('pending', 'open', 'resolved')),
    first_seen  TEXT    NOT NULL,
    opened_at   TEXT,
    last_seen   TEXT    NOT NULL,
    clear_since TEXT,
    resolved_at TEXT,
    mail_state  TEXT    NOT NULL DEFAULT 'none'
                CHECK (mail_state IN ('none', 'pending', 'sent', 'suppressed', 'not_confirmed', 'not_available', 'failed')),
    mail_detail TEXT,
    seen_at     TEXT,
    CHECK ((state = 'pending') = (opened_at IS NULL)),
    CHECK ((state = 'resolved') = (resolved_at IS NOT NULL))
);
CREATE UNIQUE INDEX idx_monitoring_alerts_active_key ON monitoring_alerts(key) WHERE state IN ('pending', 'open');
CREATE INDEX idx_monitoring_alerts_opened ON monitoring_alerts(opened_at);

CREATE TABLE site_uptime (
    domain         TEXT    PRIMARY KEY,
    checked_at     TEXT    NOT NULL,
    state          TEXT    NOT NULL CHECK (state IN ('up', 'down')),
    scheme         TEXT    NOT NULL CHECK (scheme IN ('http', 'https')),
    http_status    INTEGER,
    response_ms    INTEGER,
    reason         TEXT    NOT NULL DEFAULT '',
    cert_not_after TEXT,
    cert_valid     INTEGER CHECK (cert_valid IN (0, 1)),
    down_since     TEXT,
    CHECK ((state = 'down') = (down_since IS NOT NULL))
);
