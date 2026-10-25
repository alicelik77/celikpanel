-- alpha.108 (PostgreSQL majors side by side; owner direction of 2026-10-25,
-- replacing D-038 §7's major upgrade flow). One additive column and one new
-- table; no row is changed.
--
-- database_servers.cluster: which PostgreSQL cluster ("17/main") a server
-- record is. NULL is every record made before this release and every
-- MariaDB record: a PostgreSQL record with NULL is the cluster on its port,
-- which on an existing server is its single cluster, and the Panel ties it
-- to that cluster by name the first time it reads the clusters. The port
-- stays database_servers.port; databases_v2.server_id says which cluster a
-- database lives on.
--
-- postgresql_database_moves: one database copied to another cluster, with
-- its proof (every table's rows on both sides) and where the panel's record
-- points now: copied (the source is still the record), switched (the record
-- points at the copy; switching back is possible while the source exists),
-- switched_back, source_dropped or copy_dropped (the administrator's own
-- explicit last steps). database_id is NULL for a database CelikPanel has
-- no record of.
ALTER TABLE database_servers ADD COLUMN cluster TEXT CHECK (cluster IS NULL OR (length(cluster) BETWEEN 3 AND 80 AND cluster GLOB '[0-9]*/?*'));

CREATE TABLE postgresql_database_moves (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    database_id INTEGER REFERENCES databases_v2(id) ON DELETE SET NULL,
    database_name TEXT NOT NULL CHECK (length(database_name) BETWEEN 1 AND 63),
    source_cluster TEXT NOT NULL CHECK (length(source_cluster) BETWEEN 3 AND 80),
    target_cluster TEXT NOT NULL CHECK (length(target_cluster) BETWEEN 3 AND 80),
    source_port INTEGER NOT NULL CHECK (source_port BETWEEN 1 AND 65535),
    target_port INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    source_server_id INTEGER REFERENCES database_servers(id) ON DELETE SET NULL,
    target_server_id INTEGER REFERENCES database_servers(id) ON DELETE SET NULL,
    state TEXT NOT NULL CHECK (state IN ('copied', 'switched', 'switched_back', 'source_dropped', 'copy_dropped')),
    proof TEXT NOT NULL CHECK (length(proof) <= 65536 AND json_valid(proof)),
    created_by INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_postgresql_database_moves_database ON postgresql_database_moves(database_id);
