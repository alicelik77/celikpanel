-- 070 (alpha.128, G24): visitor statistics per site.
--
-- One row per site and finished day, computed from the site's own nginx
-- access log (combined format) when its statistics are opened, and kept so
-- the history outlives log rotation. Totals only: visitor addresses are
-- counted in memory by the Agent and never stored; top_pages holds paths
-- without their query string and top_referrers referring hosts only (each a
-- JSON array of {name, count}, at most ten). day is the server's local date
-- (YYYY-MM-DD) as nginx wrote it. Today is never stored; it is read live.
-- Rows go with their domain. Additive: nothing existing changes.
--
-- 070 (alpha.128, G24): site başına günlük ziyaretçi toplamları; adresler
-- saklanmaz.

CREATE TABLE site_visitor_stats_daily (
    domain_id     INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    day           TEXT    NOT NULL CHECK (length(day) = 10 AND day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    hits          INTEGER NOT NULL DEFAULT 0 CHECK (hits >= 0),
    page_views    INTEGER NOT NULL DEFAULT 0 CHECK (page_views >= 0),
    visitors      INTEGER NOT NULL DEFAULT 0 CHECK (visitors >= 0),
    bots          INTEGER NOT NULL DEFAULT 0 CHECK (bots >= 0),
    bytes_sent    INTEGER NOT NULL DEFAULT 0 CHECK (bytes_sent >= 0),
    status_2xx    INTEGER NOT NULL DEFAULT 0,
    status_3xx    INTEGER NOT NULL DEFAULT 0,
    status_4xx    INTEGER NOT NULL DEFAULT 0,
    status_5xx    INTEGER NOT NULL DEFAULT 0,
    top_pages     TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(top_pages) AND json_type(top_pages) = 'array'),
    top_referrers TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(top_referrers) AND json_type(top_referrers) = 'array'),
    computed_at   TEXT    NOT NULL,
    PRIMARY KEY (domain_id, day)
);
