-- 064 (alpha.126): monitoring that tells the truth and keeps 30 days.
--
-- Until this release every metrics_samples row held one 200 ms CPU snapshot
-- taken every 30 seconds, so a short burst (the release watchdog's run, a
-- package job) could be recorded as the whole half minute. From this release
-- the cpu column holds the average over the 30 seconds since the previous
-- sample and cpu_peak the highest one-second value inside them, shown as the
-- "short peak". Rows written before this release keep cpu_peak NULL: they are
-- snapshots, and nothing that needs an average (a sustained-load problem)
-- reads them as one.
--
-- metrics_rollups keeps the longer history the owner chose on 5 Nov 2026:
-- five-minute buckets for 7 days and one-hour buckets for 30 days, each with
-- the average and the highest value of its samples (disk_used is the highest
-- disk use in the bucket; cpu_peak the highest short peak, NULL when the
-- bucket holds only older snapshot rows). bucket is the bucket's
-- start in the same RFC 3339 UTC text as metrics_samples.ts
-- ('2026-11-05T10:05:00Z'), so a range is a plain text comparison. The raw
-- samples are still kept for 48 hours.
--
-- Additive only: one NULL column on metrics_samples and one new table; no
-- existing row or value changes. The release before this one refuses a
-- ledger that carries entry 64; going back is the pre-update snapshot
-- restore, as for every migration. Losing these rows loses only history the
-- panel can measure again; no hosted service reads them.
--
-- 064 (alpha.126): doğru ölçen ve 30 gün saklayan izleme. cpu artık önceki
-- örnekten bu yana 30 saniyenin ortalaması, cpu_peak içindeki en yüksek bir
-- saniyelik değerdir ("kısa tepe"); eski satırlarda cpu_peak NULL kalır.
-- metrics_rollups: 7 gün beş dakikalık, 30 gün bir saatlik kovalar (ortalama
-- ve en yüksek). Yalnız ekleme; eski hiçbir satır değişmez.
ALTER TABLE metrics_samples ADD COLUMN cpu_peak REAL;

CREATE TABLE metrics_rollups (
    resolution   INTEGER NOT NULL CHECK (resolution IN (300, 3600)),
    bucket       TEXT    NOT NULL CHECK (length(bucket) = 20 AND bucket GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:00Z'),
    samples      INTEGER NOT NULL CHECK (samples > 0),
    cpu_avg      REAL    NOT NULL,
    cpu_max      REAL    NOT NULL,
    cpu_peak     REAL,
    mem_used_avg INTEGER NOT NULL,
    mem_used_max INTEGER NOT NULL,
    mem_total    INTEGER NOT NULL,
    disk_used    INTEGER NOT NULL,
    disk_total   INTEGER NOT NULL,
    load1_avg    REAL    NOT NULL,
    load1_max    REAL    NOT NULL,
    PRIMARY KEY (resolution, bucket)
);
