// SCREEN catalogue part "pgLeftovers" (alpha.130): a removed PostgreSQL
// version's cluster whose files stayed - removed by itself when it holds only
// what PostgreSQL made at installation, otherwise kept until the
// administrator chooses "Archive and remove" - its progress steps and
// refusals, and the choice of which cluster's files the PostgreSQL page's
// editors open when several run. Split out of "databaseClusters", which had
// no headroom left.
// Key prefixes kept here: pgc.leftover., pgc.pick., opstep.pgLeftover.,
// err.PG_LEFTOVER_, err.PG_NOT_A_LEFTOVER, err.PG_CLUSTERS_UNKNOWN.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "pgLeftovers" ekran parçası: kaldırılmış PostgreSQL sürümünden kalan küme,
// "Arşivle ve kaldır" ve birden çok kümede hangi kümenin dosyalarının
// düzenlendiği. "databaseClusters" dolduğu için ayrıldı.

export const enPgLeftoversScreens = {
    'pgc.leftover.badge': 'Not used — old version {version}',
    'pgc.leftover.fresh': 'PostgreSQL {version} was removed from this server. This cluster holds only what PostgreSQL made when it was installed, so CelikPanel removes it by itself shortly after it starts, and port {port} becomes free. You do not need to do anything.',
    'pgc.leftover.kept': 'PostgreSQL {version} was removed from this server, but this cluster’s files stayed because they may hold your data. CelikPanel deletes nothing here without the administrator. It cannot run and no site uses it; it only keeps port {port} claimed. If you no longer need it, Archive and remove keeps a copy of all its files first.',
    'pgc.leftover.reason': 'Why it was kept: {reason}.',
    'pgc.leftover.archive': 'Archive and remove',
    'pgc.leftover.confirmTitle': 'Archive and remove cluster {cluster}?',
    'pgc.leftover.confirmBody': 'First its files — the configuration in /etc/postgresql/{version}/{name}, the data directory {data} and its logs — are kept in an archive under /var/backups/celikpanel that only root can read; the exact file name is shown when it is done. Then they are removed from the server.\nPort {port}, which it still claims, becomes free.\nNo site uses it: without its program it cannot run.',
    'pgc.leftover.progress': 'Archiving and removing cluster {cluster}',
    'pgc.leftover.done': 'Cluster {cluster} was archived in {archive} and removed. Port {port} is free.',
    'pgc.leftover.failed': 'The cluster was not archived or removed.',
    'pgc.leftover.unchanged': 'Nothing was changed: {reason}',
    'pgc.leftover.partial': 'Cluster {cluster} was archived in {archive}, but removing its files stopped, so some may still be on the server. The archive is kept. Next: try Archive and remove again, or remove what is left yourself. The server said: {reason}',
    'pgc.leftover.unknown': 'The answer did not arrive, so whether cluster {cluster} was archived and removed is not known. The clusters are read again; nothing is sent a second time.',
    'dashboard.audit.action.postgresql.cluster.leftover.removed': 'removed a leftover PostgreSQL cluster of a removed version ({target})',
    'dashboard.audit.action.postgresql.cluster.leftover.failed': 'could not remove a leftover PostgreSQL cluster ({target})',
    'pgc.pick.label': 'Edit the files of',
    'pgc.pick.cluster': 'PostgreSQL {version} · {name}',
    'opstep.pgLeftover.check': 'Checking that cluster {cluster} is a leftover nothing uses',
    'opstep.pgLeftover.archive': 'Archiving its files under /var/backups/celikpanel, then removing them',
    'err.PG_NOT_A_LEFTOVER': 'Nothing was changed: this cluster is no longer a removed PostgreSQL version’s leftover (it runs again, or it is gone). The list shows the clusters as they are now.',
    'err.PG_LEFTOVER_HAS_RECORDS': 'Nothing was changed: CelikPanel still records databases on this cluster. Remove those database records on the Databases page, or move them to another cluster, then archive and remove it.',
    'err.PG_LEFTOVER_DATA_OUTSIDE_STANDARD': 'Nothing was changed: this cluster keeps its data outside PostgreSQL’s standard place, and CelikPanel does not remove data it did not place. Keep or remove that directory yourself.',
    'err.PG_CLUSTERS_UNKNOWN': 'Nothing was changed: the PostgreSQL clusters could not be read. Check that the CelikPanel Agent runs (sudo systemctl status celikpanel-agent), then try again.',
} as const;

export type PgLeftoversScreenKey = keyof typeof enPgLeftoversScreens;
