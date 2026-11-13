import { useEffect, useRef, useState } from 'react';
import { CalendarClock, CheckCircle2, Cloud, History, Plus, RotateCcw, UploadCloud } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useAuth } from '../auth/AuthContext';
import { useNavigate } from '../router';
import { readApiError, type ApiError } from '../lib/apiError';
import { invalidate, refreshRemote, useRefreshEvery, useRemote, type Remote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';
import { Button, Checking, Dialog, RemoteGate, ResultUnknown, inputClass } from './ui';
import {
    decodeObjects, decodeSiteOverview, formatBytes, isRemoteGuidanceCode, operationActive, progressPercent, siteRemoteURL,
    type RemoteGuidance, type RemoteObject, type RemoteOperation, type RemotePolicy, type SiteRemoteOverview,
} from '../lib/remoteBackup';
import {
    AddRefusedNote, GuidanceBox, PolicyLine, RemoteHelp, RemoteRefusal, ScheduleEditor, formatServerTime, type ScheduleBody,
} from './RemoteBackupShared';

// A site's remote copies (alpha.118, W4), in its Backups tab: the
// destinations that may hold this site's backups, the schedules that send
// them, what was sent or restored lately and what is stored at a destination.
//
// The screen follows an operation only by reading: while one waits or runs,
// the overview is read again every few seconds, and never otherwise. Reading
// never starts a send or a restore (the server's worker runs them one at a
// time). Local backups and their schedule work without any of this.
//
// Sitenin uzak kopyaları: hedefler, zamanlamalar, son gönderme ve geri
// yüklemeler, hedefteki yedekler. Ekran işlemi yalnız okuyarak izler; okuma
// asla gönderme ya da geri yükleme başlatmaz.

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const FOLLOW_EVERY_MS = 3000;

export function DomainRemoteBackups({ domainId, readOnly, sendName, onSendClosed }: {
    domainId: number;
    readOnly: boolean;
    /** A local backup the person chose "Also send to…" for; null when none. */
    sendName: string | null;
    onSendClosed: () => void;
}) {
    const { t } = useI18n();
    const url = siteRemoteURL(domainId);
    const overview = useRemote(url, decodeSiteOverview);
    const answer = useLostAnswer(() => overview.retry());
    const value = overview.remote.state === 'known' ? overview.remote.value : null;
    const operations = value?.operations ?? [];
    const following = operations.some(operationActive);
    useRefreshEvery(overview, FOLLOW_EVERY_MS, following);

    // The object list at a destination: asked for on request only (it asks the
    // remote storage, which can take a while).
    const [objectsDestination, setObjectsDestination] = useState('');
    const [objectsShown, setObjectsShown] = useState<number | null>(null);
    const objectsURL = objectsShown === null ? null : `${url}/objects?destination_id=${objectsShown}`;
    const objects = useRemote(objectsURL, decodeObjects);

    // When an operation that was followed has finished, what it changed is read
    // again: the local list (a schedule makes a local backup, a restore keeps a
    // safety backup), the databases after a restore and the list at the
    // destination after a send.
    const followed = useRef<Set<string>>(new Set());
    useEffect(() => {
        if (!value) return;
        const finished = value.operations.filter((op) => followed.current.has(op.id) && !operationActive(op));
        followed.current = new Set(value.operations.filter(operationActive).map((op) => op.id));
        if (finished.length === 0) return;
        refreshRemote(`/api/v1/domains/${domainId}/backups`);
        if (finished.some((op) => op.kind === 'restore' && op.state === 'succeeded')) invalidate('databases');
        if (finished.some((op) => op.kind === 'upload' && op.state === 'succeeded') && objectsURL) void objects.retry();
    }, [value]);

    const [restoring, setRestoring] = useState<RemoteObject | null>(null);
    const [sending, setSending] = useState(false);
    const [refusal, setRefusal] = useState<{ problem: ApiError; sentence: TranslationKey } | null>(null);
    const known = overview.remote.state === 'known' && !overview.reading && !answer.holding;

    const queued = async (res: Response | null, sentence: TranslationKey, okKey: TranslationKey, againKey: TranslationKey) => {
        if (!res) return false;
        if (!res.ok) {
            const problem = await readApiError(res);
            setRefusal({ problem, sentence });
            showToast('error', t(sentence));
            return false;
        }
        // The operation is queued (202); whether it is a new one or the one
        // already waiting is in the answer, and the list is read again below.
        const body: unknown = await res.json().then((value: unknown) => value, () => null);
        const created = !(body && typeof body === 'object' && (body as { created?: unknown }).created === false);
        answer.settle();
        setRefusal(null);
        showToast('success', t(created ? okKey : againKey));
        void overview.retry();
        return true;
    };

    const send = async (destinationId: number) => {
        if (!sendName || readOnly) return;
        setSending(true);
        try {
            const res = await answer.send(`${url}/send`, {
                method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ backup_name: sendName, destination_id: destinationId }),
            });
            await queued(res, 'rbak.send.failed', 'rbak.send.queued', 'rbak.send.already');
            onSendClosed();
        } finally {
            setSending(false);
        }
    };

    const restore = async (object: RemoteObject, destinationId: number) => {
        if (readOnly) return;
        setSending(true);
        try {
            const res = await answer.send(`${url}/restore`, {
                method: 'POST', headers: JSON_HEADERS,
                body: JSON.stringify({ destination_id: destinationId, object_key: object.objectKey, confirm: true }),
            });
            await queued(res, 'rbak.restore.failed', 'rbak.restore.queued', 'rbak.restore.already');
            setRestoring(null);
        } finally {
            setSending(false);
        }
    };

    return (
        <section className="space-y-4 rounded-xl border border-border bg-surface p-5" aria-labelledby="rbak-site-title" aria-busy={overview.reading}>
            <div>
                <h3 id="rbak-site-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                    <UploadCloud className="h-4 w-4 text-primary" aria-hidden="true" />
                    {t('rbak.site.title')}
                </h3>
                <p className="mt-1 max-w-[75ch] text-sm text-fg-muted">{t('rbak.site.intro')}</p>
            </div>

            <ResultUnknown answer={answer} />
            {refusal && <RemoteRefusal problem={refusal.problem} sentence={refusal.sentence} />}

            <RemoteGate remote={overview.remote} checking={t('rbak.site.checking')} failed={t('rbak.site.unknown')}
                onRetry={() => void overview.retry()} busy={overview.reading}>
                {(shown) => {
                    const site = shown.value;
                    const canChange = !readOnly && known && !shown.stale;
                    const nameOf = (id: number) => site.destinations.find((d) => d.id === id)?.name
                        ?? site.policies.find((p) => p.destinationId === id)?.destinationName ?? `#${id}`;
                    const restoreRunning = site.operations.some((op) => op.kind === 'restore' && operationActive(op));
                    return (
                        <div className="space-y-5">
                            {site.destinations.length === 0 && <NoDestination site={site} />}

                            <Operations operations={site.operations} nameOf={nameOf} />

                            {site.destinations.length > 0 && (
                                <Schedules domainId={domainId} site={site} canChange={canChange} answer={answer}
                                    onChanged={() => void overview.retry()} />
                            )}

                            {site.destinations.length > 0 && (
                                <section aria-labelledby="rbak-objects-title" className="space-y-3">
                                    <h4 id="rbak-objects-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                                        <Cloud className="h-4 w-4 text-primary" aria-hidden="true" />
                                        {t('rbak.objects.title')}
                                    </h4>
                                    <div className="flex flex-wrap items-end gap-2">
                                        <label className="min-w-0 text-sm" htmlFor="rbak-objects-destination">
                                            <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.destination')}</span>
                                            <select id="rbak-objects-destination" className={`${inputClass} field-lg`}
                                                value={objectsDestination || String(site.destinations[0].id)}
                                                onChange={(event) => setObjectsDestination(event.target.value)}>
                                                {site.destinations.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                                            </select>
                                        </label>
                                        <Button className="max-sm:min-h-11 max-sm:w-full max-sm:justify-center" loading={objects.reading && objectsShown !== null}
                                            onClick={() => {
                                                const id = Number(objectsDestination || site.destinations[0].id);
                                                if (id === objectsShown) void objects.retry();
                                                else setObjectsShown(id);
                                            }}>
                                            {t('rbak.objects.show')}
                                        </Button>
                                    </div>
                                    {objectsShown !== null && (
                                        <ObjectList remoteState={objects.remote} reading={objects.reading} onRetry={() => void objects.retry()}
                                            canRestore={canChange && !restoreRunning} restoreRunning={restoreRunning}
                                            onRestore={(object) => setRestoring(object)} />
                                    )}
                                </section>
                            )}

                            {sendName && (
                                <SendDialog name={sendName} destinations={site.destinations} busy={sending} holding={answer.holding}
                                    onSend={(id) => void send(id)} onClose={onSendClosed} />
                            )}
                            {restoring && objectsShown !== null && (
                                <RestoreDialog object={restoring} destinationName={nameOf(objectsShown)} busy={sending} holding={answer.holding}
                                    onConfirm={() => void restore(restoring, objectsShown)} onClose={() => setRestoring(null)} />
                            )}
                        </div>
                    );
                }}
            </RemoteGate>

            <RemoteHelp />
        </section>
    );
}

/** No destination may hold this site's backups: why, who can change it, and where. */
function NoDestination({ site }: { site: SiteRemoteOverview }) {
    const { t } = useI18n();
    const { role } = useAuth();
    const navigate = useNavigate();
    const admin = role === 'admin';
    const guidance: RemoteGuidance = {
        result: 'prerequisite', code: '', reason: t('rbak.site.noDestination'),
        whoActs: admin || site.canAddDestination ? 'you' : 'administrator',
        nextAction: t(admin ? 'rbak.site.addAdmin' : site.canAddDestination ? 'rbak.site.addOwn' : 'rbak.site.askProvider'),
        resumes: '',
    };
    return (
        <div className="space-y-2" data-no-destination="">
            <GuidanceBox guidance={guidance} context="operation" resumesText={t('rbak.site.resumes')} />
            {admin ? (
                <Button className="max-sm:min-h-11" onClick={() => navigate('/settings?section=remote-backups')}>{t('rbak.site.openSettings')}</Button>
            ) : site.canAddDestination ? (
                <Button className="max-sm:min-h-11" onClick={() => navigate('/account')}>{t('rbak.site.openAccount')}</Button>
            ) : (
                <AddRefusedNote layer={site.addRefusedBy} />
            )}
        </div>
    );
}

function Operations({ operations, nameOf }: { operations: RemoteOperation[]; nameOf: (id: number) => string }) {
    const { t, locale } = useI18n();
    return (
        <section aria-labelledby="rbak-ops-title" className="space-y-2">
            <h4 id="rbak-ops-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                <History className="h-4 w-4 text-primary" aria-hidden="true" />
                {t('rbak.site.operations')}
            </h4>
            {operations.length === 0 ? (
                <p className="text-sm text-fg-muted">{t('rbak.site.noOperations')}</p>
            ) : (
                <ul className="space-y-2" aria-live="polite">
                    {operations.map((op) => {
                        const active = operationActive(op);
                        const percent = progressPercent(op.bytesDone, op.bytesTotal);
                        const time = formatServerTime(op.finishedAt || op.createdAt, locale);
                        return (
                            <li key={op.id} className="space-y-2 rounded-lg border border-border p-3" data-operation={op.state}>
                                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                                    <span className="font-medium text-fg">
                                        {t(op.kind === 'restore' ? 'rbak.op.restore' : 'rbak.op.upload', { destination: nameOf(op.destinationId) })}
                                    </span>
                                    {op.scheduled && <span className="rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">{t('rbak.op.scheduled')}</span>}
                                    {time && <span className="text-xs text-fg-muted">{t(op.finishedAt ? 'rbak.op.finished' : 'rbak.op.started', { time })}</span>}
                                </div>
                                {op.backupName && <p className="break-all font-mono text-xs text-fg-muted">{op.backupName}</p>}
                                {active && (
                                    <div>
                                        <div className="h-2 w-full overflow-hidden rounded-full bg-surface-2" role="progressbar"
                                            aria-label={t('rbak.op.progress')} aria-valuemin={0} aria-valuemax={100}
                                            aria-valuenow={percent ?? undefined} aria-valuetext={percent === null ? t('rbak.op.sizeUnknown') : `${percent}%`}>
                                            <div className={`h-full rounded-full bg-primary transition-all ${percent === null ? 'w-1/4 animate-pulse' : ''}`}
                                                style={percent === null ? undefined : { width: `${percent}%` }} />
                                        </div>
                                        {op.bytesTotal > 0 && (
                                            <p className="mt-1 text-xs text-fg-muted">
                                                {t('rbak.op.bytes', { done: formatBytes(op.bytesDone), total: formatBytes(op.bytesTotal) })}
                                            </p>
                                        )}
                                    </div>
                                )}
                                {op.state === 'succeeded' ? (
                                    <p className="flex items-center gap-1.5 text-xs text-success">
                                        <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
                                        {t(op.kind === 'restore' ? 'rbak.done.restore' : 'rbak.done.upload')}
                                    </p>
                                ) : (
                                    <GuidanceBox guidance={op.guidance} context="operation" kind={op.kind} phase={op.phase} nextAttemptAt={op.nextAttemptAt} />
                                )}
                            </li>
                        );
                    })}
                </ul>
            )}
        </section>
    );
}

function Schedules({ domainId, site, canChange, answer, onChanged }: {
    domainId: number;
    site: SiteRemoteOverview;
    canChange: boolean;
    answer: ReturnType<typeof useLostAnswer>;
    onChanged: () => void;
}) {
    const { t } = useI18n();
    const policyURL = `${siteRemoteURL(domainId)}/policy`;
    const [editing, setEditing] = useState<{ policy: RemotePolicy | null } | null>(null);
    const [working, setWorking] = useState<number | 'new' | null>(null);
    const [refusal, setRefusal] = useState<{ problem: ApiError; sentence: TranslationKey } | null>(null);
    const own = site.policies.filter((p) => !p.everySite);
    const free = site.destinations.filter((d) => !own.some((p) => p.destinationId === d.id));

    const save = async (body: ScheduleBody) => {
        setWorking(editing?.policy?.destinationId ?? 'new');
        setRefusal(null);
        try {
            const res = await answer.send(policyURL, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(body) });
            if (!res) return;
            if (!res.ok) {
                setRefusal({ problem: await readApiError(res), sentence: 'rbak.sched.saveFailed' });
                return;
            }
            answer.settle();
            showToast('success', t('rbak.sched.saved'));
            setEditing(null);
            onChanged();
        } finally {
            setWorking(null);
        }
    };

    const turnOff = async (policy: RemotePolicy) => {
        setWorking(policy.destinationId);
        setRefusal(null);
        try {
            const res = await answer.send(`${policyURL}?destination_id=${policy.destinationId}`, { method: 'DELETE' });
            if (!res) return;
            if (!res.ok) {
                setRefusal({ problem: await readApiError(res), sentence: 'rbak.sched.offFailed' });
                return;
            }
            answer.settle();
            showToast('success', t('rbak.sched.offDone'));
            onChanged();
        } finally {
            setWorking(null);
        }
    };

    return (
        <section aria-labelledby="rbak-sched-title" className="space-y-2">
            <h4 id="rbak-sched-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                <CalendarClock className="h-4 w-4 text-primary" aria-hidden="true" />
                {t('rbak.site.schedules')}
            </h4>
            <p className="max-w-[75ch] text-xs text-fg-muted">{t('rbak.site.schedulesNote')}</p>
            {refusal && <RemoteRefusal problem={refusal.problem} sentence={refusal.sentence} />}
            {site.policies.length === 0 && <p className="text-sm text-fg-muted">{t('rbak.site.noSchedules')}</p>}
            <ul className="space-y-2">
                {site.policies.map((policy) => (
                    <li key={`${policy.everySite ? 'all' : 'site'}-${policy.destinationId}`} className="rounded-lg border border-border p-3">
                        {editing?.policy && !policy.everySite && editing.policy.destinationId === policy.destinationId && !editing.policy.everySite ? (
                            <ScheduleEditor idPrefix={`rbak-site-policy-${policy.destinationId}`} destinations={site.destinations} initial={policy}
                                busy={working !== null} onSave={(body) => void save(body)} onCancel={() => setEditing(null)} />
                        ) : (
                            <PolicyLine policy={policy} readOnly={!canChange || policy.everySite} disabled={!canChange || working !== null}
                                busy={working === policy.destinationId}
                                onEdit={() => setEditing({ policy })} onOff={() => void turnOff(policy)} />
                        )}
                    </li>
                ))}
            </ul>
            {canChange && (editing && editing.policy === null ? (
                <ScheduleEditor idPrefix="rbak-site-policy-new" destinations={free} busy={working !== null}
                    onSave={(body) => void save(body)} onCancel={() => setEditing(null)} />
            ) : free.length > 0 && (
                <Button icon={Plus} disabled={working !== null} className="max-sm:min-h-11" onClick={() => setEditing({ policy: null })}>
                    {t('rbak.site.addSchedule')}
                </Button>
            ))}
        </section>
    );
}

function ObjectList({ remoteState, reading, onRetry, canRestore, restoreRunning, onRestore }: {
    remoteState: Remote<RemoteObject[]>;
    reading: boolean;
    onRetry: () => void;
    canRestore: boolean;
    restoreRunning: boolean;
    onRestore: (object: RemoteObject) => void;
}) {
    const { t, locale } = useI18n();
    if (remoteState.state === 'loading') return <Checking label={t('rbak.objects.checking')} />;
    // A destination that refused the listing says why with its own guidance.
    if (remoteState.state === 'unknown' && !remoteState.previous && isRemoteGuidanceCode(remoteState.reason.code)) {
        return (
            <div className="space-y-2">
                <RemoteRefusal problem={remoteState.reason} sentence="rbak.objects.unknown" />
                <Button loading={reading} onClick={onRetry} className="max-sm:min-h-11">{t('common.retry')}</Button>
            </div>
        );
    }
    return (
        <RemoteGate remote={remoteState} checking={t('rbak.objects.checking')} failed={t('rbak.objects.unknown')} onRetry={onRetry} busy={reading}>
            {(shown) => shown.value.length === 0 ? (
                <p className="text-sm text-fg-muted">{t('rbak.objects.empty')}</p>
            ) : (
                <div className="space-y-2">
                    {restoreRunning && <p className="text-xs text-fg-muted" role="note">{t('rbak.objects.restoreRunning')}</p>}
                    <ul className="space-y-2">
                        {shown.value.map((object) => {
                            const created = object.createdAt && !object.createdAt.startsWith('0001') ? new Date(object.createdAt).toLocaleString(locale) : '';
                            return (
                                <li key={object.objectKey} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border p-3" data-object="">
                                    <div className="min-w-0">
                                        <p className="break-all font-mono text-xs text-fg">{object.backupName}</p>
                                        <div className="mt-1 flex flex-wrap gap-1.5 text-xs text-fg-muted">
                                            {created && <span>{created}</span>}
                                            <span>{formatBytes(object.size)}</span>
                                            <span className="rounded bg-surface-2 px-1.5 py-0.5">{t(object.group === 'schedule' ? 'rbak.objects.schedule' : 'rbak.objects.manual')}</span>
                                            {object.otherServer && <span className="rounded bg-warning/15 px-1.5 py-0.5 text-warning">{t('rbak.objects.otherServer')}</span>}
                                            {!object.hasSidecar && <span className="rounded bg-surface-2 px-1.5 py-0.5" title={t('rbak.objects.noSidecarHint')}>{t('rbak.objects.noSidecar')}</span>}
                                        </div>
                                    </div>
                                    {canRestore && (
                                        <Button icon={RotateCcw} disabled={shown.stale} className="max-sm:min-h-11"
                                            aria-label={t('rbak.objects.restoreNamed', { name: object.backupName })} onClick={() => onRestore(object)}>
                                            {t('rbak.objects.restore')}
                                        </Button>
                                    )}
                                </li>
                            );
                        })}
                    </ul>
                </div>
            )}
        </RemoteGate>
    );
}

function SendDialog({ name, destinations, busy, holding, onSend, onClose }: {
    name: string;
    destinations: { id: number; name: string }[];
    busy: boolean;
    holding: boolean;
    onSend: (destinationId: number) => void;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const [destination, setDestination] = useState(String(destinations[0]?.id ?? ''));
    return (
        <Dialog
            id="rbak-send"
            title={t('rbak.send.title')}
            description={t('rbak.send.body', { name })}
            icon={UploadCloud}
            busy={busy}
            onDismiss={onClose}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('rbak.cancel')}</Button>
                    <Button type="button" variant="primary" icon={UploadCloud} loading={busy} disabled={!destination || holding}
                        onClick={() => onSend(Number(destination))}>
                        {t('rbak.send.confirm')}
                    </Button>
                </>
            }
        >
            {destinations.length === 0 ? (
                <p className="text-sm text-fg-muted">{t('rbak.site.noDestination')}</p>
            ) : (
                <label className="block text-sm" htmlFor="rbak-send-destination">
                    <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.destination')}</span>
                    <select id="rbak-send-destination" className={`${inputClass} field-lg`} value={destination} disabled={busy}
                        onChange={(event) => setDestination(event.target.value)}>
                        {destinations.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select>
                </label>
            )}
        </Dialog>
    );
}

function RestoreDialog({ object, destinationName, busy, holding, onConfirm, onClose }: {
    object: RemoteObject;
    destinationName: string;
    busy: boolean;
    holding: boolean;
    onConfirm: () => void;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const replaces = object.type === 'full' ? 'rbak.restore.replacesFull' : object.type === 'database' ? 'rbak.restore.replacesDatabase' : 'rbak.restore.replacesFiles';
    return (
        <Dialog
            id="rbak-restore"
            title={t('rbak.restore.title')}
            description={t('rbak.restore.from', { name: object.backupName, destination: destinationName })}
            icon={RotateCcw}
            tone="danger"
            busy={busy}
            dismissible={!busy}
            onDismiss={onClose}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('rbak.cancel')}</Button>
                    <Button type="button" variant="danger" icon={RotateCcw} loading={busy} disabled={holding} onClick={onConfirm}>
                        {t('rbak.restore.confirm')}
                    </Button>
                </>
            }
        >
            <ul className="list-disc space-y-2 pl-5 text-sm text-fg">
                <li>{t(replaces as TranslationKey)}</li>
                <li>{t('rbak.restore.safety')}</li>
                <li>{t('rbak.restore.quota')}</li>
                <li>{t('rbak.restore.follow')}</li>
            </ul>
        </Dialog>
    );
}
