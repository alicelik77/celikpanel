// PostgreSQL majors side by side (v0.1.0-alpha.108; the owner's words of
// 2026-10-25: 16, 17 and 18 are different things, each installed on its own
// port, ports editable; with 16 present, 17 is installed on another port,
// the databases are moved, then 16 is removed). This card shows every
// cluster, CelikPanel's records on each, the majors that can be installed
// beside them and on which port, changes a cluster's port after showing who
// connects, copies a database to another cluster with per-table row counts
// as proof, and switches CelikPanel's record to the copy only as its own
// confirmed step, reversible until the source is dropped. Every operation
// says the current reason, who acts and what happens next (D-024); a running
// step says that nothing needs to be done; nothing is sent twice.
//
// PostgreSQL ana sürümleri yan yana: kümeler, portlar, taşımalar.
import { useEffect, useRef, useState } from 'react';
import { ArrowRightLeft, Database, DownloadCloud, Layers, Network } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote, type RemoteHandle } from '../lib/remote';
import { LifeLine, type Tone } from './LifeLine';
import { useComponentOperation } from './ComponentOperation';
import { InstallPreviewNotes, installPreviewAckKey, installPreviewGate } from './InstallPreviewNotes';
import { decodeInstallPreview, installPreviewURL } from '../lib/installPreview';
import {
    decodePGClusters,
    decodePGPortPreview,
    pgPortPreviewURL,
    pgPortValue,
    pgPost,
    pgRef,
    PG_CLUSTERS_URL,
    PG_MOVES_URL,
    type PGCluster,
    type PGClusters,
    type PGMove,
    type PGOffer,
    type PGProof,
    type PGRecord,
} from '../lib/pgClusters';

export { decodePGClusters } from '../lib/pgClusters';

type Result = { tone: Tone; text: string } | null;

export function PostgreSQLClustersCard() {
    const { t } = useI18n();
    const handle = useRemote(PG_CLUSTERS_URL, decodePGClusters);
    return (
        <section className="rounded-2xl border border-border bg-surface" data-pg-clusters="">
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Layers className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">{t('pgc.title')}</h2>
            </header>
            <div className="space-y-4 p-5">
                <p className="text-sm text-fg-muted">{t('pgc.desc')}</p>
                <RemoteGate remote={handle.remote} checking={t('pgc.checking')} failed={t('pgc.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <ClustersBody value={value} handle={handle} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function ClustersBody({ value, handle }: { value: PGClusters; handle: RemoteHandle<PGClusters> }) {
    const { t } = useI18n();
    const operation = useComponentOperation();
    const [result, setResult] = useState<Result>(null);
    const [installing, setInstalling] = useState<PGOffer | null>(null);
    const [portFor, setPortFor] = useState<PGCluster | null>(null);
    const [moveFrom, setMoveFrom] = useState<PGCluster | null>(null);
    const [moveAction, setMoveAction] = useState<{ move: PGMove; action: MoveAction } | null>(null);
    const [syncing, setSyncing] = useState(false);
    // The port shown before an install, kept to compare with the cluster the
    // install made once the operation has finished.
    const expected = useRef<{ major: string; port: number } | null>(null);
    const [installNote, setInstallNote] = useState<Result>(null);
    const wasLocked = useRef(operation.locked);
    useEffect(() => {
        if (wasLocked.current && !operation.locked && expected.current) void handle.retry();
        wasLocked.current = operation.locked;
    }, [operation.locked, handle]);
    useEffect(() => {
        const want = expected.current;
        if (!want || operation.locked) return;
        const made = value.clusters.find((cluster) => cluster.version === want.major && cluster.name === 'main');
        if (!made) return;
        expected.current = null;
        setInstallNote(made.port === want.port
            ? { tone: 'ok', text: t('pgc.install.asShown', { major: want.major, port: made.port }) }
            : { tone: 'warn', text: t('pgc.install.otherPort', { major: want.major, port: made.port, shown: want.port }) });
    }, [value, operation.locked, t]);

    if (!value.supported) {
        return <LifeLine tone="info" attr="pgc-unsupported">{t('pgc.unsupported')}</LifeLine>;
    }
    const online = value.clusters.filter((cluster) => cluster.online);
    const activeMoves = value.moves.filter((move) => move.state === 'copied' || move.state === 'switched' || move.state === 'switched_back');
    const pastMoves = value.moves.filter((move) => !activeMoves.includes(move));
    const syncRecords = async (cluster: string) => {
        const [version, name] = cluster.split('/');
        setSyncing(true);
        try {
            const answer = await pgPost(`${PG_CLUSTERS_URL}/records`, { version, name });
            setResult(answer.ok
                ? { tone: 'ok', text: t('pgc.records.synced', { cluster, port: String(answer.body.port ?? '') }) }
                : { tone: 'bad', text: t('pgc.failed', { reason: String(answer.body.error ?? '') }) });
        } catch {
            setResult({ tone: 'bad', text: t('pgc.lost') });
        } finally {
            setSyncing(false);
            void handle.retry();
        }
    };
    return (
        <div className="space-y-4">
            {value.mismatches.map((mismatch) => (
                <div key={mismatch.cluster} className="space-y-2" data-pg-mismatch={mismatch.cluster}>
                    <LifeLine tone="warn" attr="pgc-mismatch">{t('pgc.records.mismatch', { cluster: mismatch.cluster, record: mismatch.record_port, port: mismatch.cluster_port })}</LifeLine>
                    <div className="pl-6">
                        <Button onClick={() => void syncRecords(mismatch.cluster)} loading={syncing} disabled={syncing} className="max-sm:min-h-11">{t('pgc.records.sync')}</Button>
                    </div>
                </div>
            ))}
            {installNote && <LifeLine tone={installNote.tone} attr="pgc-install-note">{installNote.text}</LifeLine>}
            {result && <LifeLine tone={result.tone} attr="pgc-result"><KeepWords text={result.text} /></LifeLine>}
            <ul className="space-y-3">
                {value.clusters.map((cluster) => (
                    <ClusterRow
                        key={pgRef(cluster)}
                        cluster={cluster}
                        records={value.records[pgRef(cluster)] ?? []}
                        canMove={online.length > 1 && cluster.online && cluster.databases.length > 0}
                        onPort={() => setPortFor(cluster)}
                        onMove={() => setMoveFrom(cluster)}
                    />
                ))}
            </ul>
            {value.unplaced.length > 0 && (
                <LifeLine tone="warn" attr="pgc-unplaced">{t('pgc.records.unplaced', { names: value.unplaced.map((record) => `${record.name} (${record.host || 'localhost'}:${record.port})`).join(', ') })}</LifeLine>
            )}
            {activeMoves.length > 0 && (
                <div className="space-y-2" data-pg-moves="">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">{t('pgc.moves.title')}</h3>
                    {activeMoves.map((move) => <MoveRow key={move.id} move={move} onAction={(action) => setMoveAction({ move, action })} />)}
                </div>
            )}
            {pastMoves.length > 0 && (
                <details className="text-sm text-fg-muted">
                    <summary className="cursor-pointer">{t('pgc.moves.history', { count: pastMoves.length })}</summary>
                    <ul className="mt-2 space-y-1 pl-4">
                        {pastMoves.map((move) => (
                            <li key={move.id}>{t(`pgc.moves.state.${move.state}` as TranslationKey, { database: move.database, from: move.source_cluster, to: move.target_cluster })}</li>
                        ))}
                    </ul>
                </details>
            )}
            <OffersBlock value={value} locked={operation.locked} onInstall={setInstalling} />
            {installing && (
                <InstallMajorDialog
                    offer={installing}
                    value={value}
                    onCancel={() => setInstalling(null)}
                    onConfirm={async (confirmed) => {
                        const offer = installing;
                        setInstalling(null);
                        expected.current = { major: offer.major, port: offer.port };
                        setInstallNote(null);
                        const started = await operation.startInstall({
                            serviceId: 'postgresql',
                            name: `PostgreSQL ${offer.major}`,
                            package: offer.package,
                            checkInstalls: true,
                            confirmInstalls: confirmed,
                        });
                        if (!started) expected.current = null;
                    }}
                />
            )}
            {portFor && <PortDialog cluster={portFor} onClose={(done) => { setPortFor(null); if (done) { setResult(done); void handle.retry(); } }} />}
            {moveFrom && <MoveDialog source={moveFrom} targets={online.filter((cluster) => pgRef(cluster) !== pgRef(moveFrom))} records={value.records[pgRef(moveFrom)] ?? []} onClose={(done) => { setMoveFrom(null); if (done) { setResult(done); void handle.retry(); } }} />}
            {moveAction && <MoveActionDialog move={moveAction.move} action={moveAction.action} onClose={(done) => { setMoveAction(null); if (done) { setResult(done); void handle.retry(); } }} />}
        </div>
    );
}

function ClusterRow({ cluster, records, canMove, onPort, onMove }: { cluster: PGCluster; records: PGRecord[]; canMove: boolean; onPort: () => void; onMove: () => void }) {
    const { t } = useI18n();
    return (
        <li className="rounded-xl border border-border p-4" data-pg-cluster={pgRef(cluster)}>
            <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-semibold text-fg">
                    {t('pgc.cluster.name', { version: cluster.version, cluster: pgRef(cluster) })}
                </p>
                <span className={`rounded px-2 py-0.5 text-xs font-medium ${cluster.online ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'}`}>
                    {cluster.online ? t('pgc.cluster.online') : t('pgc.cluster.down', { status: cluster.status })}
                </span>
            </div>
            <dl className="mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-sm sm:grid-cols-[max-content_1fr]">
                <dt className="text-fg-subtle">{t('pgc.cluster.port')}</dt>
                <dd className="font-mono text-fg">{cluster.port}</dd>
                <dt className="text-fg-subtle">{t('pgc.cluster.data')}</dt>
                <dd className="break-all font-mono text-xs text-fg">{cluster.data_directory}</dd>
                <dt className="text-fg-subtle">{t('pgc.cluster.databases')}</dt>
                <dd className="text-fg">
                    {!cluster.databases_known
                        ? t('pgc.cluster.databasesUnknown', { reason: cluster.databases_error ?? '' })
                        : cluster.databases.length === 0
                            ? t('pgc.cluster.noDatabases')
                            : t('pgc.cluster.databaseList', { count: cluster.databases.length, names: cluster.databases.join(', ') })}
                </dd>
                <dt className="text-fg-subtle">{t('pgc.cluster.records')}</dt>
                <dd className="text-fg" data-pg-records="">
                    {records.length === 0
                        ? t('pgc.cluster.noRecords')
                        : records.map((record) => recordLine(record, t)).join('; ')}
                </dd>
            </dl>
            <div className="mt-3 flex flex-wrap gap-2">
                <Button onClick={onPort} disabled={!cluster.online} icon={Network} className="max-sm:min-h-11" data-pg-port-offer="">{t('pgc.port.offer')}</Button>
                <Button onClick={onMove} disabled={!canMove} icon={ArrowRightLeft} className="max-sm:min-h-11" data-pg-move-offer="">{t('pgc.move.offer')}</Button>
            </div>
            {!cluster.online && <p className="mt-2 text-xs text-fg-subtle">{t('pgc.cluster.downHint', { unit: `postgresql@${cluster.version}-${cluster.name}` })}</p>}
        </li>
    );
}

function recordLine(record: PGRecord, t: (key: TranslationKey, vars?: Record<string, string | number>) => string): string {
    return t('pgc.cluster.record', {
        name: record.name,
        domain: record.domain || '—',
        host: record.host || 'localhost',
        port: record.port,
        users: record.users.join(', ') || '—',
    });
}

function OffersBlock({ value, locked, onInstall }: { value: PGClusters; locked: boolean; onInstall: (offer: PGOffer) => void }) {
    const { t } = useI18n();
    if (value.offers.length === 0) return null;
    return (
        <div className="space-y-2" data-pg-offers="">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">{t('pgc.offers.title')}</h3>
            <ul className="space-y-2">
                {value.offers.map((offer) => (
                    <li key={offer.major} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-3 py-2" data-pg-offer={offer.major}>
                        <span className="min-w-0 text-sm text-fg">
                            {offer.origin
                                ? t(offer.origin === 'distribution' ? 'pgc.offers.distribution' : 'pgc.offers.pgdg', { major: offer.major, candidate: offer.candidate ?? '', port: offer.port })
                                : offer.needs_repository
                                    ? t('pgc.offers.needsRepo', { major: offer.major })
                                    : t('pgc.offers.notOffered', { major: offer.major })}
                        </span>
                        {offer.origin && (
                            <Button onClick={() => onInstall(offer)} disabled={locked} icon={DownloadCloud} className="max-sm:min-h-11" data-pg-install-offer={offer.major}>
                                {t('pgc.offers.install', { major: offer.major })}
                            </Button>
                        )}
                    </li>
                ))}
            </ul>
        </div>
    );
}

function InstallMajorDialog({ offer, value, onCancel, onConfirm }: { offer: PGOffer; value: PGClusters; onCancel: () => void; onConfirm: (confirmed: string[]) => void }) {
    const { t } = useI18n();
    const url = installPreviewURL([{ id: 'postgresql', package: offer.package }]);
    const preview = useRemote(url, decodeInstallPreview);
    const [acknowledgedFor, setAcknowledgedFor] = useState<string | null>(null);
    const ackKey = installPreviewAckKey(url, preview);
    const gate = installPreviewGate(preview, ackKey !== null && acknowledgedFor === ackKey);
    return (
        <Dialog
            id="pg-install"
            icon={Database}
            title={t('pgc.install.title', { major: offer.major })}
            width="lg"
            onDismiss={onCancel}
            actions={
                <>
                    <Button onClick={onCancel}>{t('common.cancel')}</Button>
                    <Button variant="primary" icon={DownloadCloud} onClick={() => onConfirm(gate.confirmed)} disabled={gate.hold} data-pg-install-confirm="">
                        {t('pgc.install.confirm', { major: offer.major })}
                    </Button>
                </>
            }
        >
            <div className="space-y-3 text-sm leading-6 text-fg">
                <LifeLine tone="info" attr="pgc-install-port">
                    {!value.creates_cluster
                        ? t('pgc.install.noCluster', { major: offer.major })
                        : offer.kept_cluster
                            ? t('pgc.install.keptCluster', { major: offer.major, port: offer.port })
                            : t('pgc.install.port', { major: offer.major, port: offer.port, used: value.used_ports.join(', ') || '—' })}
                </LifeLine>
                <LifeLine tone="info" attr="pgc-install-origin">
                    {t(offer.origin === 'pgdg' ? 'pgc.install.fromPgdg' : 'pgc.install.fromDistribution', { pkg: offer.package, candidate: offer.candidate ?? '' })}
                </LifeLine>
                <LifeLine tone="info" attr="pgc-install-firewall">{t('pgc.firewall')}</LifeLine>
                <p className="text-fg-muted">{t('pgc.install.after')}</p>
                {url && (
                    <InstallPreviewNotes
                        handle={preview}
                        acknowledged={ackKey !== null && acknowledgedFor === ackKey}
                        onAcknowledge={(on) => setAcknowledgedFor(on ? ackKey : null)}
                        componentLabel={(id) => (id === 'postgresql' ? 'PostgreSQL' : id)}
                        avoidable={false}
                    />
                )}
            </div>
        </Dialog>
    );
}

function PortDialog({ cluster, onClose }: { cluster: PGCluster; onClose: (done: Result) => void }) {
    const { t } = useI18n();
    const [text, setText] = useState(String(cluster.port === 5432 ? 5440 : cluster.port + 1));
    const [asked, setAsked] = useState<number | null>(null);
    const preview = useRemote(asked ? pgPortPreviewURL(cluster, asked) : null, decodePGPortPreview);
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<string | null>(null);
    const port = pgPortValue(text);
    const known = asked !== null && preview.remote.state === 'known' && !preview.reading ? preview.remote.value : null;
    const refusal = known?.preview.refusal;
    const change = async () => {
        if (!known || refusal || !asked) return;
        setBusy(true);
        setFailure(null);
        try {
            const answer = await pgPost(`${PG_CLUSTERS_URL}/port`, { version: cluster.version, name: cluster.name, port: asked, expected_sha256: known.preview.config_sha256 });
            if (answer.ok) {
                onClose({ tone: 'ok', text: t('pgc.port.done', { cluster: pgRef(cluster), from: cluster.port, to: asked, records: String(answer.body.records ?? 0) }) });
                return;
            }
            if (answer.body.rolled_back) {
                onClose({ tone: 'warn', text: t(answer.body.restored ? 'pgc.port.rolledBack' : 'pgc.port.rolledBackDown', { cluster: pgRef(cluster), from: cluster.port, to: asked, reason: String(answer.body.error ?? '') }) });
                return;
            }
            setFailure(t('pgc.failed', { reason: String(answer.body.error ?? '') }));
        } catch {
            onClose({ tone: 'bad', text: t('pgc.lost') });
        } finally {
            setBusy(false);
        }
    };
    return (
        <Dialog
            id="pg-port"
            icon={Network}
            title={t('pgc.port.title', { cluster: pgRef(cluster) })}
            width="lg"
            busy={busy}
            onDismiss={() => onClose(null)}
            actions={
                <>
                    <Button onClick={() => onClose(null)} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant="primary" onClick={() => void change()} loading={busy} disabled={busy || !known || Boolean(refusal)} data-pg-port-confirm="">
                        {t('pgc.port.confirm', { port: asked ?? '' })}
                    </Button>
                </>
            }
        >
            <div className="space-y-3 text-sm leading-6 text-fg">
                <p>{t('pgc.port.intro', { cluster: pgRef(cluster), port: cluster.port })}</p>
                <div className="flex flex-wrap items-end gap-2">
                    <label className="flex flex-col gap-1 text-xs font-medium text-fg-subtle">
                        {t('pgc.port.label')}
                        <input
                            className="w-32 rounded-lg border border-border bg-surface px-3 py-2 font-mono text-sm text-fg"
                            inputMode="numeric"
                            value={text}
                            onChange={(event) => { setText(event.target.value); setAsked(null); }}
                            disabled={busy}
                            data-pg-port-input=""
                        />
                    </label>
                    <Button onClick={() => setAsked(port)} disabled={busy || port === null} data-pg-port-preview="">{t('pgc.port.show')}</Button>
                </div>
                {port === null && <p className="text-xs text-danger">{t('pgc.port.invalid')}</p>}
                {asked !== null && preview.remote.state === 'loading' && <LifeLine tone="info">{t('pgc.port.checking')}</LifeLine>}
                {asked !== null && preview.remote.state === 'unknown' && <LifeLine tone="warn">{t('pgc.port.unknown')}</LifeLine>}
                {known && (
                    <div className="space-y-2" data-pg-port-plan="">
                        {refusal ? (
                            <LifeLine tone="bad" attr="pgc-port-refusal"><KeepWords text={t('pgc.port.refused', { reason: refusal })} /></LifeLine>
                        ) : (
                            <>
                                <LifeLine tone="info" attr="pgc-port-line">
                                    {t('pgc.port.line', { path: known.preview.config_path, before: known.preview.line_before || '—', after: known.preview.line_after })}
                                </LifeLine>
                                <LifeLine tone="warn" attr="pgc-port-restart">{t('pgc.port.restart', { cluster: pgRef(cluster) })}</LifeLine>
                            </>
                        )}
                        <LifeLine tone="info" attr="pgc-port-records">
                            {known.records.length === 0
                                ? t('pgc.port.noRecords')
                                : t('pgc.port.records', { list: known.records.map((record) => `${record.name} (${record.host || 'localhost'}:${record.port})`).join(', '), to: asked ?? '' })}
                        </LifeLine>
                        <LifeLine tone="info" attr="pgc-port-sessions">
                            {!known.preview.sessions_known
                                ? t('pgc.port.sessionsUnknown')
                                : known.preview.sessions.length === 0
                                    ? t('pgc.port.noSessions')
                                    : t('pgc.port.sessions', { list: known.preview.sessions.map((session) => `${session.database || '—'} · ${session.user || '—'} · ${session.client}${session.application ? ' · ' + session.application : ''} ×${session.count}`).join('; ') })}
                        </LifeLine>
                        {known.preview.phppgadmin_port === cluster.port && (
                            <LifeLine tone="warn" attr="pgc-port-tool">{t('pgc.port.tool', { port: cluster.port })}</LifeLine>
                        )}
                        <LifeLine tone="info" attr="pgc-port-apps">{t('pgc.port.apps')}</LifeLine>
                        <LifeLine tone="info" attr="pgc-port-firewall">{t('pgc.firewall')}</LifeLine>
                    </div>
                )}
                {busy && <LifeLine tone="info" attr="pgc-port-busy">{t('pgc.port.busy', { cluster: pgRef(cluster) })}</LifeLine>}
                {failure && <LifeLine tone="bad" attr="pgc-port-failure"><KeepWords text={failure} /></LifeLine>}
            </div>
        </Dialog>
    );
}

function ProofTable({ proof }: { proof: PGProof }) {
    const { t } = useI18n();
    return (
        <div className="space-y-2" data-pg-proof={proof.equal ? 'equal' : 'differs'}>
            <LifeLine tone={proof.equal ? 'ok' : 'bad'} attr="pgc-proof">
                {proof.equal
                    ? t('pgc.proof.equal', { tables: proof.tables_total, from: proof.source_version ?? '', to: proof.target_version ?? '' })
                    : t('pgc.proof.differs', { mismatches: proof.mismatches, tables: proof.tables_total })}
            </LifeLine>
            {proof.tables.length > 0 && (
                <div className="max-h-56 overflow-auto rounded-lg border border-border">
                    <table className="w-full text-left text-xs">
                        <thead className="bg-surface-2 text-fg-subtle">
                            <tr>
                                <th className="px-2 py-1 font-medium">{t('pgc.proof.table')}</th>
                                <th className="px-2 py-1 text-right font-medium">{t('pgc.proof.source')}</th>
                                <th className="px-2 py-1 text-right font-medium">{t('pgc.proof.target')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {proof.tables.map((row) => (
                                <tr key={`${row.schema}.${row.table}`} className={row.source === row.target ? '' : 'text-danger'}>
                                    <td className="break-all px-2 py-1 font-mono">{row.schema}.{row.table}</td>
                                    <td className="px-2 py-1 text-right font-mono">{row.source < 0 ? '—' : row.source}</td>
                                    <td className="px-2 py-1 text-right font-mono">{row.target < 0 ? '—' : row.target}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {proof.tables.length < proof.tables_total && <p className="text-xs text-fg-subtle">{t('pgc.proof.capped', { shown: proof.tables.length, tables: proof.tables_total })}</p>}
            {proof.roles_created.length > 0 && <p className="text-xs text-fg-subtle">{t('pgc.proof.rolesCreated', { roles: proof.roles_created.join(', ') })}</p>}
            {proof.roles_kept.length > 0 && <LifeLine tone="warn" attr="pgc-proof-kept">{t('pgc.proof.rolesKept', { roles: proof.roles_kept.join(', ') })}</LifeLine>}
        </div>
    );
}

function MoveDialog({ source, targets, records, onClose }: { source: PGCluster; targets: PGCluster[]; records: PGRecord[]; onClose: (done: Result) => void }) {
    const { t } = useI18n();
    const [database, setDatabase] = useState(source.databases[0] ?? '');
    const [target, setTarget] = useState(targets[0] ? pgRef(targets[0]) : '');
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<{ text: string; proof?: PGProof } | null>(null);
    const chosen = targets.find((cluster) => pgRef(cluster) === target);
    const record = records.find((item) => item.name === database && !item.legacy);
    const copy = async () => {
        if (!chosen || !database) return;
        setBusy(true);
        setFailure(null);
        try {
            const answer = await pgPost(PG_MOVES_URL, { database, source_port: source.port, target_port: chosen.port });
            if (answer.ok) {
                onClose({ tone: 'ok', text: t(answer.body.recorded ? 'pgc.move.copied' : 'pgc.move.copiedUnrecorded', { database, from: pgRef(source), to: pgRef(chosen) }) });
                return;
            }
            setFailure({ text: t('pgc.failed', { reason: String(answer.body.error ?? '') }), proof: answer.body.proof as PGProof | undefined });
        } catch {
            onClose({ tone: 'bad', text: t('pgc.lost') });
        } finally {
            setBusy(false);
        }
    };
    return (
        <Dialog
            id="pg-move"
            icon={ArrowRightLeft}
            title={t('pgc.move.title', { cluster: pgRef(source) })}
            width="lg"
            busy={busy}
            onDismiss={() => onClose(null)}
            actions={
                <>
                    <Button onClick={() => onClose(null)} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant="primary" onClick={() => void copy()} loading={busy} disabled={busy || !chosen || !database} data-pg-move-confirm="">
                        {t('pgc.move.confirm')}
                    </Button>
                </>
            }
        >
            <div className="space-y-3 text-sm leading-6 text-fg">
                <div className="flex flex-wrap gap-3">
                    <label className="flex flex-col gap-1 text-xs font-medium text-fg-subtle">
                        {t('pgc.move.database')}
                        <select className="rounded-lg border border-border bg-surface px-3 py-2 text-sm text-fg" value={database} onChange={(event) => setDatabase(event.target.value)} disabled={busy}>
                            {source.databases.map((name) => <option key={name} value={name}>{name}</option>)}
                        </select>
                    </label>
                    <label className="flex flex-col gap-1 text-xs font-medium text-fg-subtle">
                        {t('pgc.move.target')}
                        <select className="rounded-lg border border-border bg-surface px-3 py-2 text-sm text-fg" value={target} onChange={(event) => setTarget(event.target.value)} disabled={busy}>
                            {targets.map((cluster) => <option key={pgRef(cluster)} value={pgRef(cluster)}>{t('pgc.move.targetOption', { cluster: pgRef(cluster), port: cluster.port })}</option>)}
                        </select>
                    </label>
                </div>
                <LifeLine tone="info" attr="pgc-move-steps">{t('pgc.move.steps')}</LifeLine>
                <LifeLine tone="info" attr="pgc-move-record">
                    {record ? t('pgc.move.recordAfter', { database, port: chosen?.port ?? '' }) : t('pgc.move.unrecorded', { database })}
                </LifeLine>
                <LifeLine tone="info" attr="pgc-move-writes">{t('pgc.move.writes')}</LifeLine>
                {busy && <LifeLine tone="info" attr="pgc-move-busy">{t('pgc.move.busy', { database })}</LifeLine>}
                {failure && (
                    <div className="space-y-2">
                        <LifeLine tone="bad" attr="pgc-move-failure"><KeepWords text={failure.text} /></LifeLine>
                        {failure.proof && failure.proof.tables_total > 0 && <ProofTable proof={failure.proof} />}
                    </div>
                )}
            </div>
        </Dialog>
    );
}

type MoveAction = 'switch' | 'switch-back' | 'drop-source' | 'drop-copy';

function moveActions(move: PGMove): MoveAction[] {
    const recorded = move.database_id !== null;
    switch (move.state) {
        case 'copied':
            return recorded ? ['switch', 'drop-copy'] : ['drop-source', 'drop-copy'];
        case 'switched':
            return ['switch-back', 'drop-source'];
        case 'switched_back':
            return ['switch', 'drop-copy'];
        default:
            return [];
    }
}

function MoveRow({ move, onAction }: { move: PGMove; onAction: (action: MoveAction) => void }) {
    const { t } = useI18n();
    return (
        <div className="space-y-2 rounded-xl border border-border p-4" data-pg-move={move.id} data-pg-move-state={move.state}>
            <p className="text-sm font-medium text-fg">{t(`pgc.moves.state.${move.state}` as TranslationKey, { database: move.database, from: move.source_cluster, to: move.target_cluster })}</p>
            <p className="text-sm text-fg-muted">{t(`pgc.moves.next.${move.state}` as TranslationKey, { database: move.database, from: `${move.source_cluster} (${move.source_port})`, to: `${move.target_cluster} (${move.target_port})` })}</p>
            <ProofTable proof={move.proof} />
            <div className="flex flex-wrap gap-2">
                {moveActions(move).map((action) => (
                    <Button key={action} variant={action.startsWith('drop') ? 'danger' : 'secondary'} onClick={() => onAction(action)} className="max-sm:min-h-11" data-pg-move-action={action}>
                        {t(`pgc.moves.action.${action}` as TranslationKey)}
                    </Button>
                ))}
            </div>
        </div>
    );
}

function MoveActionDialog({ move, action, onClose }: { move: PGMove; action: MoveAction; onClose: (done: Result) => void }) {
    const { t } = useI18n();
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<string | null>(null);
    const vars = { database: move.database, from: `${move.source_cluster} (${move.source_port})`, to: `${move.target_cluster} (${move.target_port})`, port: action === 'switch-back' ? move.source_port : move.target_port };
    const run = async () => {
        setBusy(true);
        setFailure(null);
        try {
            const answer = await pgPost(`${PG_MOVES_URL}/${move.id}/${action}`, undefined);
            if (answer.ok) {
                onClose({ tone: 'ok', text: t(`pgc.moves.done.${action}` as TranslationKey, vars) });
                return;
            }
            setFailure(t('pgc.failed', { reason: String(answer.body.error ?? '') }));
        } catch {
            onClose({ tone: 'bad', text: t('pgc.lost') });
        } finally {
            setBusy(false);
        }
    };
    return (
        <Dialog
            id="pg-move-action"
            icon={ArrowRightLeft}
            tone={action.startsWith('drop') ? 'danger' : 'default'}
            title={t(`pgc.moves.dialog.${action}.title` as TranslationKey, vars)}
            width="lg"
            busy={busy}
            onDismiss={() => onClose(null)}
            actions={
                <>
                    <Button onClick={() => onClose(null)} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant={action.startsWith('drop') ? 'danger' : 'primary'} onClick={() => void run()} loading={busy} disabled={busy} data-pg-move-action-confirm={action}>
                        {t(`pgc.moves.action.${action}` as TranslationKey)}
                    </Button>
                </>
            }
        >
            <div className="space-y-3 text-sm leading-6 text-fg">
                <p><KeepWords text={t(`pgc.moves.dialog.${action}.body` as TranslationKey, vars)} /></p>
                {failure && <LifeLine tone="bad" attr="pgc-move-action-failure"><KeepWords text={failure} /></LifeLine>}
            </div>
        </Dialog>
    );
}
