import { useEffect, useState } from 'react';
import { AlertTriangle, ArrowRightLeft, CheckCircle2, Gauge, Undo2 } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { autoMoveLeadKey, autoMoveLineKey, autoMoveReasonKey, withSiteLimitsLane, type SiteLimitsLaneKey, type SiteLimitsLaneT } from '../i18n/siteLimitsLane';
import { formatDateTime } from '../lib/dateFormat';
import { readApiError, apiErrorText } from '../lib/apiError';
import { confirmAction, followOperation, type OperationEnd, type OperationStep } from '../lib/confirmAction';
import { readRemote, useRemote } from '../lib/remote';
import { startVisibleReads } from '../lib/visibleReads';
import { unitText, whoText } from '../lib/settingLimits';
import {
    appliedOf, dbOwnerByOrigin, decodeMoveAnswer, decodeSiteLimits, eventAtSiteLimit, eventKey, followsAutoMove, keptUnits, limitOf, lowerUnits, MOVE_WAIT_SECONDS, moveReasonWords, moveSteps, moveWhyKey,
    newMoveRequestID, notMovedReason, putBackSplit, type DbOwnerSetting, type MoveReason, type SiteLimitKey, type SiteLimitValue, type SiteLimitsView,
} from '../lib/siteLimits';
import {
    decodeTasksAnswer, keptCronBackKey, keptTasksRunningKey, lineList, movedTaskCount, privilegedList, readTasksPreview, tasksAnswerKey, unresolvedSteps, KEPT_MOVE, type TasksAnswer,
} from '../lib/siteTasks';
import { Button, Checking, CouldNotCheck } from './ui';

// The limits of a site's programs together (S4 R2a, docs/SITE-SANDBOX.md
// section 8), on the domain's Overview when the site runs a program. Everyone
// who sees the domain reads the three limits, whether they apply at all, what
// the programs use now and, in plain words, when they reached a limit, what
// the plan allows and who can raise it (a reseller is that provider). An
// event the kernel does not prove at the site's limit never names the plan.
// The administrator also reads where each program runs, what keeps one from
// moving and who acts, and the server owner's own additions. The panel moves
// every site into its limits by itself (alpha.150, D-048): while it does, one
// calm line; the move is offered only as Try again next to a move that did
// not bring this site inside, and for a site whose tasks were put back into
// cron on purpose.
//
// Bir sitenin programlarının birlikte sınırları: herkes sınırları, uygulanıp
// uygulanmadığını, şu anki kullanımı ve ne zaman aşıldığını okur; panel her
// siteyi kendiliğinden sınırlarının içine alır, taşıma yalnız bir hatanın
// yanında "Yeniden dene" olarak durur.

const ORDER: { key: SiteLimitKey; label: TranslationKey }[] = [
    { key: 'site.memory_mb', label: 'slim.memory' },
    { key: 'site.tasks', label: 'slim.tasks' },
    { key: 'site.cpu_percent', label: 'slim.cpu' },
];

export function SiteLimitsCard({ domainId, runsPrograms = true }: { domainId: number; runsPrograms?: boolean }) {
    const { t: screenT, locale } = useI18n();
    const t = withSiteLimitsLane(locale, screenT);
    const url = `/api/v1/domains/${domainId}/site-limits`;
    const handle = useRemote(url, decodeSiteLimits);
    const remote = handle.remote;
    // alpha.150 fix round G5: while the panel moves sites (or this site is outside its limits) the card reads
    // itself again about once a minute, and stops when there is nothing to follow. Reads only.
    const shown = remote.state === 'known' ? remote.value : remote.state === 'unknown' ? (remote.previous?.value as SiteLimitsView | undefined) : undefined;
    const follow = followsAutoMove(shown);
    const reread = handle.retry;
    useEffect(() => (follow ? startVisibleReads(reread, 60_000, { setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number, clearInterval: (id) => clearInterval(id) }) : undefined), [follow, reread]);
    if (remote.state === 'loading') return <Checking label={t('slim.checking')} className="mb-4" />;
    if (remote.state === 'unknown' && !remote.previous) {
        return <div className="mb-4"><CouldNotCheck text={t('slim.unreadLimits')} onRetry={() => void handle.retry()} busy={handle.reading} /></div>;
    }
    const view = remote.state === 'known' ? remote.value : (remote.previous!.value as SiteLimitsView);
    // A site without programs: only the administrator's tasks part says anything.
    if (!view.admin && !runsPrograms) return null;

    const amountText = (key: SiteLimitKey, n: number) => {
        if (n === 0) return t('limits.plan.none');
        if (key === 'site.memory_mb') return unitText(t, 'mb', n);
        if (key === 'site.tasks') return t('slim.value.tasks', { n });
        return t('slim.value.cpu', { n });
    };
    const valueText = (limit: SiteLimitValue | undefined) => (limit ? amountText(limit.key, limit.value) : '');
    const who = (limit: SiteLimitValue | undefined) => {
        if (!limit) return '';
        // alpha.149 (lab CHECK 2): the server's default bounding the reseller, which nobody set.
        if (limit.set_by === 'server_default_bound') return t(view.admin ? 'slim.who.bound' : 'slim.who.boundReseller');
        return limit.set_by === 'server_default' || limit.set_by === 'default' ? t('slim.who.default') : whoText(screenT, limit.set_by, limit.set_by_name);
    };
    const memory = limitOf(view, 'site.memory_mb');
    const provider = view.admin || view.reseller === true;
    const applies = view.applies ?? 'yes';
    const fix = view.admin ? 'slim.fix.admin' : view.reseller ? 'slim.fix.reseller' : 'slim.fix.customer';

    return (
        <section className="mb-4 rounded-xl border border-border bg-surface p-4" aria-labelledby={`site-limits-${domainId}`} data-site-limits="">
            <h2 id={`site-limits-${domainId}`} className="flex items-center gap-2 text-sm font-semibold text-fg">
                <Gauge className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {t('slim.title')}
            </h2>
            <p className="mt-1 max-w-[75ch] text-sm text-fg-muted">{t('slim.intro')}</p>

            <dl className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-3">
                {ORDER.map(({ key, label }) => {
                    const limit = limitOf(view, key);
                    // The limit in force when it is not the plan's (review B3, C6).
                    const applied = appliedOf(view, key);
                    return (
                        <div key={key} className="rounded-lg bg-surface-2 px-3 py-2" data-site-limit={key}>
                            <dt className="text-xs text-fg-muted">{t(label)}</dt>
                            <dd className="text-sm font-semibold text-fg">{valueText(limit)}</dd>
                            {applied !== undefined && <dd className="text-xs text-warning" data-site-limit-applied="">{t('slim.applied', { value: amountText(key, applied) })}</dd>}
                            {provider && limit && <dd className="text-xs text-fg-subtle">{t('slim.who.line', { who: who(limit) })}</dd>}
                        </div>
                    );
                })}
            </dl>
            {!view.admin && <p className="mt-2 text-xs text-fg-subtle">{t(view.reseller ? 'slim.who.reseller' : 'slim.who.customer')}</p>}
            {view.known && applies !== 'yes' && (
                <p className="mt-2 flex items-start gap-2 text-sm text-fg" role="status" data-site-limits-applies={applies}>
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    <span className="min-w-0 flex-1">{t(`slim.applies.${applies}` as TranslationKey, { detail: view.unsupported ?? '' })}</span>
                </p>
            )}
            {view.known && view.live && applies === 'yes' && (
                <p className="mt-2 text-sm text-fg-muted" data-site-limits-using="">
                    {t('slim.using', { memory: unitText(t, 'mb', view.memory_current_mb ?? 0), tasks: view.tasks_current ?? 0 })}
                </p>
            )}

            {!view.known ? (
                <p className="mt-3 flex items-start gap-2 text-sm text-fg-muted" role="status">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    <span className="min-w-0 flex-1">{t('slim.unread')}</span>
                    <Button onClick={() => void handle.retry()} loading={handle.reading}>{t('pnow.retry')}</Button>
                </p>
            ) : view.events.length === 0 ? (
                // "Stayed within their limits" only where limits apply.
                applies === 'yes' && (
                    <p className="mt-3 flex items-center gap-2 text-sm text-fg-muted" data-site-limits-events="none">
                        <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                        {t('slim.noEvents')}
                    </p>
                )
            ) : (
                <ul className="mt-3 space-y-2" data-site-limits-events="">
                    {view.events.map((event) => {
                        const key = eventKey(event.kind);
                        const limit = limitOf(view, key);
                        // The number reached is the one in force; where it is not the
                        // plan's, raising the plan would not change it, so no fix names it.
                        const applied = appliedOf(view, key);
                        const atLimit = eventAtSiteLimit(event.kind) && applied === undefined;
                        // alpha.149 (lab CHECK 2): the limit that held then, or none; never today's.
                        const then = atLimit || eventAtSiteLimit(event.kind) ? event.limit : undefined;
                        return (
                            <li key={event.kind} className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-fg" data-site-limit-event={event.kind}>
                                <p>
                                    {t(`slim.event.${event.kind}` as TranslationKey, { time: formatDateTime(event.last_at, locale), count: event.count })}
                                    {then !== undefined && then > 0 && <> {t('slim.event.limitThen', { limit: amountText(key, then) })}</>}
                                    {event.repeated && <> {t('slim.event.repeated')}</>}
                                </p>
                                {atLimit && <p className="mt-1 text-fg-muted">{t(fix as TranslationKey, { who: who(limit) })}</p>}
                            </li>
                        );
                    })}
                </ul>
            )}

            {view.admin && <AdminPart domainId={domainId} view={view} memory={memory?.value ?? 0} onDone={() => void handle.retry()} />}
        </section>
    );
}

function AdminPart({ domainId, view, memory, onDone }: { domainId: number; view: SiteLimitsView; memory: number; onDone: () => void }) {
    const { t: screenT, locale } = useI18n();
    const t = withSiteLimitsLane(locale, screenT);
    const [working, setWorking] = useState(false);
    // A lost answer keeps the request id, so pressing again returns the first
    // answer instead of starting a second move.
    const [requestID, setRequestID] = useState(newMoveRequestID);
    // The move's own id (moveSiteIntoLimits keeps it after a lost answer, as above).
    const [moveID] = useState(() => ({ id: newMoveRequestID() }));
    const units = view.units ?? [];
    const edited = view.slice_file === 'owner_edited' || view.slice_file === 'foreign' || view.slice_file === 'unknown_origin';
    // alpha.149 (review of PR #370, finding 13): the server owner's firm limits and the defaults changed on
    // the database user itself are two sentences; neither is imposed again.
    const dbValue = (s: DbOwnerSetting) => `${s.user} (${t(`limits.key.${s.key}` as TranslationKey)}: ${s.value || '—'})`;
    const dbByOrigin = dbOwnerByOrigin(view);
    const owner = dbByOrigin.owner.map(dbValue);
    const onUser = dbByOrigin.onUser.map(dbValue);
    // S4 R2b: the site's scheduled tasks move with the same action.
    const tasks = view.tasks;
    const [undoID, setUndoID] = useState(newMoveRequestID);
    // alpha.150 (D-048): the panel moves every site by itself; the card offers the move only
    // next to a move that did not bring this site inside (Try again) and for a site whose
    // tasks were put back on purpose.
    const auto = view.auto_move;
    const offer = view.applies !== 'unsupported' && !view.move_interrupted?.kept ? auto?.offer : undefined;

    // "Put the tasks back into cron": the panel's own question, then the steps.
    const undo = async () => {
        setWorking(true);
        try {
            await confirmAction({
                title: t('slim.tasks.undo.confirm'),
                message: t('slim.tasks.undo.body'),
                confirmLabel: t('slim.tasks.undo.go'),
                cancelLabel: t('slim.move.cancel'),
                progressTitle: t('slim.tasks.undo.title'),
                action: async (report): Promise<OperationEnd> => {
                    report.steps([{ id: 'undo', label: t('slim.tasks.undo.step'), state: 'running', since: Date.now() }]);
                    let sent: Response;
                    try {
                        sent = await fetch(`/api/v1/domains/${domainId}/site-limits/tasks-undo`, {
                            method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: undoID }),
                        });
                    } catch {
                        return { state: 'unknown', reason: t('slim.move.unknown') };
                    }
                    if (!sent.ok) return { state: 'failed', reason: apiErrorText(await readApiError(sent), t) };
                    let answer: TasksAnswer;
                    try {
                        answer = decodeTasksAnswer(await sent.json());
                    } catch {
                        return { state: 'unknown', reason: t('slim.move.unknown') };
                    }
                    setUndoID(newMoveRequestID());
                    // alpha.150 fix round (F6): tasks whose timer was switched off on the server came back paused.
                    if (answer.outcome === 'undone') {
                        return answer.paused.length > 0
                            ? { state: 'done', message: t('slim.tasks.undo.done'), kept: { title: t('slim.tasks.undo.paused'), rows: answer.paused.map((l) => `${l.line}: ${l.text}`) } }
                            : { state: 'done', message: t('slim.tasks.undo.done') };
                    }
                    if (answer.outcome === 'unknown') return { state: 'unknown', reason: t(tasksAnswerKey(answer, true) as TranslationKey) };
                    if (answer.refusal === KEPT_MOVE) return { state: 'blocked', reason: t('slim.why.kept_earlier') };
                    return { state: answer.outcome === 'refused' ? 'blocked' : 'failed', reason: t(tasksAnswerKey(answer, true) as TranslationKey) };
                },
            });
        } finally {
            setWorking(false);
            onDone();
        }
    };

    // The move (Try again, or the move of a site put back on purpose) in the panel's own
    // question and window; the card reads again when it ends.
    const ask = async () => {
        setWorking(true);
        try {
            await moveSiteIntoLimits(domainId, view, t, moveID);
        } finally {
            setWorking(false);
            onDone();
        }
    };

    // "Try again" for a move whose put-back did not complete (review N2): the
    // put-back runs now in the panel's own window; nothing new is moved.
    const retry = async () => {
        setWorking(true);
        try {
            await followOperation({
                progressTitle: t('pnow.retry'),
                action: async (report): Promise<OperationEnd> => {
                    report.steps([{ id: 'put_back', label: t('slim.move.step.restart'), state: 'running', since: Date.now() }]);
                    let sent: Response;
                    try {
                        sent = await fetch(`/api/v1/domains/${domainId}/site-limits/move`, {
                            method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: requestID, recover: true }),
                        });
                    } catch {
                        return { state: 'unknown', reason: t('slim.move.unknown') };
                    }
                    if (!sent.ok) return { state: 'failed', reason: apiErrorText(await readApiError(sent), t) };
                    let answer;
                    try {
                        answer = decodeMoveAnswer(await sent.json());
                    } catch {
                        return { state: 'unknown', reason: t('slim.move.unknown') };
                    }
                    setRequestID(newMoveRequestID());
                    // Second fix round (R8): a program Try again put back that runs but listens on no port is named.
                    const silent = answer.runs_no_port ?? [];
                    if (answer.done) return silent.length > 0 ? { state: 'done', message: t('slim.kept.runsNoPort', { list: silent.map((u) => labelOf(units, u)).join(', ') }) } : { state: 'done' };
                    return { state: 'failed', reason: answer.kept ? t('slim.kept', { time: formatDateTime(view.move_interrupted?.at ?? '', locale) }) : t(moveWhyKey(answer.reason) as SiteLimitsLaneKey) };
                },
            });
        } finally {
            setWorking(false);
            onDone();
        }
    };
    const kept = keptUnits(view);

    return (
        <div className="mt-4 border-t border-border pt-3" data-site-limits-admin="">
            {edited && <p className="mb-2 text-sm text-warning">{t('slim.sliceEdited')}</p>}
            {(view.drop_ins ?? []).length > 0 && <p className="mb-2 text-sm text-fg-muted" data-site-limits-dropins="">{t('slim.dropIns', { list: (view.drop_ins ?? []).join(', ') })}</p>}
            {(view.apply_total ?? 0) > 0 && <p className="mb-2 text-sm text-fg-muted" role="status">{t('slim.applying', { done: view.apply_done ?? 0, total: view.apply_total ?? 0 })}</p>}
            {view.move_interrupted?.kept ? (
                <div className="mb-2 space-y-1 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-fg" role="status" data-site-limits-kept="">
                    <p>{t('slim.kept', { time: formatDateTime(view.move_interrupted.at, locale) })}</p>
                    {view.move_interrupted.tasks && (
                        // S4 R2b (alpha.150): the kept record is of the site's scheduled tasks.
                        // alpha.150: how the tasks run now, when the agent knows it, comes first.
                        <p data-site-limits-kept-tasks={view.move_interrupted.tasks_step ?? ''} data-site-limits-kept-tasks-running={view.move_interrupted.tasks_running ?? ''}>
                            {/* alpha.150 fix round (F1): with a crontab there, the steps by hand would overwrite it: Try again decides. */}
                            {[keptTasksRunningKey(view.move_interrupted.tasks_running) ? t(keptTasksRunningKey(view.move_interrupted.tasks_running) as TranslationKey) : '',
                                view.move_interrupted.tasks_running === 'cron'
                                    // Fix round 4 (N3): the way out when that crontab is not the recorded one.
                                    ? t(keptCronBackKey(view.move_interrupted.tasks_step) as TranslationKey, { site: view.move_interrupted.tasks_site ?? '', account: view.move_interrupted.tasks_account ?? '', steps: unresolvedSteps(view.move_interrupted) })
                                    : t('slim.tasks.unresolved', { step: view.move_interrupted.tasks_step ?? '', steps: unresolvedSteps(view.move_interrupted) })].filter(Boolean).join(' ')}
                        </p>
                    )}
                    {kept.map((k) => (
                        <p key={k.unit} data-site-limits-kept-unit={k.state}>
                            {/* alpha.149 (lab D6): put back but not running: said so, who acts, the next step. */}
                            {/* Fix round W3: not known yet, or running but not answering: look first, never "start it". */}
                            {k.state === 'failed'
                                ? t('slim.kept.notRunning', { unit: labelOf(units, k.unit), name: k.unit })
                                : t(k.state === 'unverified' || k.state === 'not_answering' ? 'slim.kept.look' : 'slim.kept.unit',
                                    { unit: labelOf(units, k.unit), state: t(`slim.kept.${k.state}` as TranslationKey), name: k.unit })}
                        </p>
                    ))}
                    <Button loading={working} disabled={working || view.busy === true} onClick={() => void retry()}>{t('pnow.retry')}</Button>
                </div>
            ) : view.move_interrupted && <p className="mb-2 text-sm text-warning" data-site-limits-interrupted="">{t('slim.interrupted', { time: formatDateTime(view.move_interrupted.at, locale) })}</p>}
            {owner.length > 0 && <p className="mb-2 text-sm text-fg-muted" data-site-limits-dbowner="">{t('slim.dbOwner', { list: owner.join('; ') })}</p>}
            {onUser.length > 0 && <p className="mb-2 text-sm text-fg-muted" data-site-limits-dbonuser="">{t('slim.dbOnUser', { list: onUser.join('; ') })}</p>}
            {units.length > 0 && (
                <>
                    <h3 className="text-xs font-semibold text-fg">{t('slim.units.title')}</h3>
                    <ul className="mt-1 space-y-1 text-sm">
                        {units.map((u) => (
                            <li key={u.unit} className="text-fg-muted" data-site-limit-unit={u.placement}>
                                <span className="font-medium text-fg">{u.label}</span> ({u.unit}): {t(`slim.place.${u.placement}` as TranslationKey, { name: u.unit })}
                                {u.blocked && <span className="block text-xs text-warning" data-site-limit-blocked={u.blocked}>{t(`slim.blocked.${u.blocked}` as TranslationKey)}</span>}
                                {u.lower && u.placement === 'outside' && !u.blocked && (
                                    <span className="block text-xs text-warning">{t('slim.lower', { own: unitText(t, 'mb', u.own_memory_mb ?? 0), site: unitText(t, 'mb', memory) })}</span>
                                )}
                            </li>
                        ))}
                    </ul>
                </>
            )}
            {tasks && (
                <div className="mt-3" data-site-limits-tasks={tasks.unknown ? 'unknown' : tasks.moved ? 'moved' : 'cron'}>
                    <h3 className="text-xs font-semibold text-fg">{t('slim.tasks.title')}</h3>
                    <p className="mt-1 text-sm text-fg-muted">{t(tasks.unknown ? 'slim.tasks.unknown' : tasks.moved ? 'slim.tasks.moved' : 'slim.tasks.cron')}</p>
                    {(tasks.reappeared ?? 0) > 0 && (
                        <p className="mt-1 text-sm text-warning" data-site-limits-tasks-returned={tasks.by_site ? 'site' : 'owner'}>
                            {t(tasks.by_site ? 'slim.tasks.reappearedSite' : 'slim.tasks.reappeared', { n: tasks.reappeared ?? 0 })}
                        </p>
                    )}
                    {(tasks.mail_submission === 'owner' || tasks.mail_submission === 'absent' || tasks.mail_submission === 'owner_file') && (
                        <p className="mt-1 text-sm text-fg-muted" data-site-limits-mail={tasks.mail_submission}>
                            {t(tasks.mail_submission === 'owner' ? 'slim.tasks.mailOwner' : tasks.mail_submission === 'owner_file' ? 'slim.tasks.mailOwnerFile' : 'slim.tasks.mailAbsent')}
                        </p>
                    )}
                    {tasks.recovery && (
                        <p className="mt-1 text-sm text-warning">{t('slim.tasks.recovery', { time: formatDateTime(tasks.recovery.split(' ')[0], locale), outcome: tasks.recovery.split(' ').slice(1).join(' ') })}</p>
                    )}
                    {tasks.moved && (
                        <Button className="mt-2" icon={Undo2} loading={working} disabled={working || view.busy === true} onClick={() => void undo()} data-site-tasks-undo="">
                            {t('slim.tasks.undo.button')}
                        </Button>
                    )}
                </div>
            )}
            {view.busy && <p className="mt-2 text-sm text-fg-muted">{t('slim.move.busy')}</p>}
            {/* alpha.150 (D-048): while the panel moves sites by itself, one calm line; nothing to press. */}
            {(auto?.total ?? 0) > 0 && (
                <p className="mt-2 text-sm text-fg-muted" role="status" data-site-limits-auto="running">
                    {t(autoMoveLineKey(auto?.waiting) as SiteLimitsLaneKey, { done: auto?.done ?? 0, total: auto?.total ?? 0 })}
                </p>
            )}
            {offer === 'retry' && auto && (
                <div className="mt-3 space-y-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-fg" role="status" data-site-limits-auto="retry">
                    <p>{t(autoMoveLeadKey(auto) as SiteLimitsLaneKey)} {t(autoMoveReasonKey(auto) as SiteLimitsLaneKey)}</p>
                    <Button loading={working} disabled={working || view.busy === true} onClick={() => void ask()}>{t('pnow.retry')}</Button>
                </div>
            )}
            {offer === 'move' && (
                <div className="mt-3 space-y-2 rounded-lg border border-border-strong bg-surface-2 px-3 py-2" data-site-limits-auto="move">
                    <p className="text-sm text-fg-muted">{t('slim.auto.putBack')}</p>
                    <Button variant="primary" icon={ArrowRightLeft} loading={working} disabled={working || view.busy === true} onClick={() => void ask()}>
                        {t('slim.move.button')}
                    </Button>
                </div>
            )}
        </div>
    );
}

/**
 * The move of a site into its limits: its scheduled tasks first, then its programs, with the
 * same request (POST .../site-limits/move) the panel's own worker uses (alpha.150, D-048). It is
 * offered only next to a move that did not bring the site inside (Try again, on the card and in
 * Problems now) and for a site whose tasks were put back into cron on purpose. The product's own
 * question and progress window: the page stays locked with the steps until the server answers,
 * then says how it ended. id keeps the request id after an answer that was lost, so pressing
 * again returns the first answer instead of starting a second move.
 */
export async function moveSiteIntoLimits(domainId: number, view: SiteLimitsView, t: SiteLimitsLaneT, id: { id: string }): Promise<void> {
    const units = view.units ?? [];
    const lower = lowerUnits(view);
    const tasks = view.tasks;
    const tasksToMove = !!tasks && !tasks.moved && !tasks.unknown && view.applies !== 'unsupported';
    // alpha.149 (lab D7): a reason in the page's own words, never the agent's English text.
    const reasonText = (reason: MoveReason | undefined) => {
        const words = moveReasonWords(reason);
        return t(words.key as SiteLimitsLaneKey, words.values);
    };
    const tasksResult = (answer: TasksAnswer | undefined): string => {
        if (!answer) return '';
        if (answer.refusal === KEPT_MOVE) return t('slim.why.kept_earlier');
        // alpha.150 (main's rule): a code worded here, never the agent's English.
        switch (answer.outcome) {
            case 'moved': return t('slim.tasks.done.moved');
            case 'already': return '';
            case 'stayed': return t('slim.tasks.done.stayed', { list: lineList(answer.blocking) });
            default: return t(tasksAnswerKey(answer, false) as TranslationKey);
        }
    };
    // What the move would make of the crontab, read before the question.
    let tasksPreview: TasksAnswer | 'unknown' | null = null;
    if (tasksToMove) {
        try {
            tasksPreview = await readTasksPreview(domainId);
        } catch {
            // Not known: the question says so; the move checks every line itself.
            tasksPreview = 'unknown';
        }
    }
    const previewText = (p: TasksAnswer | 'unknown' | null): string => {
        if (!p) return '';
        if (p === 'unknown') return t('slim.tasks.previewUnknown');
        if (p.outcome === 'stayed') return t('slim.tasks.blocking', { list: lineList(p.blocking) });
        if (p.outcome !== 'preview') return p.error ?? '';
        const n = movedTaskCount(p);
        const privileged = privilegedList(p);
        // Mail: through this server only while its submission services are
        // CelikPanel's own; otherwise the card says sites cannot send through it.
        const mail = !p.mail ? '' : p.mail_submission === 'owner' ? t('slim.tasks.mailOwner')
            : p.mail_submission === 'absent' ? t('slim.tasks.mailAbsent')
                : p.mail_submission === 'owner_file' ? t('slim.tasks.mailOwnerFile') : t('slim.tasks.mail');
        return [n > 0 ? t('slim.tasks.rows', { n }) : t('slim.tasks.none'), n > 0 ? t('stasks.timers') : '', mail,
            privileged ? t('slim.tasks.privileged', { list: privileged }) : ''].filter(Boolean).join(' ');
    };
    const moving = units.filter((u) => u.placement === 'outside' && !u.blocked);
    const step = (stepID: string, label: string, state: OperationStep['state'], note?: string): OperationStep => ({ id: stepID, label, state, note, since: Date.now() });
    await confirmAction({
        title: (view.needs_move ?? 0) > 0 ? t('slim.move.confirm', { count: view.needs_move ?? 0 }) : t('slim.tasks.title'),
        message: [(view.needs_move ?? 0) > 0 ? t('slim.move.body', { seconds: MOVE_WAIT_SECONDS }) : '',
            lower.length > 0 ? t('slim.move.lowerWarn', { list: lower.map((u) => u.label).join(', ') }) : '',
            previewText(tasksPreview)].filter(Boolean).join('\n\n'),
        confirmLabel: t('slim.move.go'),
        cancelLabel: t('slim.move.cancel'),
        progressTitle: t('slim.move.title'),
        action: async (report): Promise<OperationEnd> => {
            report.steps([
                ...(tasksToMove ? [step('tasks', t('slim.tasks.step'), 'running')] : []),
                step('write', t('slim.move.step.write'), tasksToMove ? 'pending' : 'running'),
                ...moving.map((u) => step(u.unit, `${t('slim.move.step.restart')}: ${u.label}`, 'pending')),
                step('check', t('slim.move.step.check'), 'pending'),
            ]);
            let sent: Response;
            try {
                sent = await fetch(`/api/v1/domains/${domainId}/site-limits/move`, {
                    method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: id.id }),
                });
            } catch {
                return { state: 'unknown', reason: t('slim.move.unknown') };
            }
            if (!sent.ok) return { state: 'failed', reason: apiErrorText(await readApiError(sent), t) };
            let answer;
            let raw: { tasks?: unknown };
            try {
                raw = await sent.json() as { tasks?: unknown };
                answer = decodeMoveAnswer(raw);
            } catch {
                return { state: 'unknown', reason: t('slim.move.unknown') };
            }
            id.id = newMoveRequestID();
            let tasksAnswer: TasksAnswer | undefined;
            try {
                tasksAnswer = raw.tasks ? decodeTasksAnswer(raw.tasks) : undefined;
            } catch {
                tasksAnswer = undefined;
            }
            const tasksOK = !tasksAnswer || tasksAnswer.outcome === 'moved' || tasksAnswer.outcome === 'already' || tasksAnswer.outcome === 'crontab_back';
            report.steps([
                ...(tasksToMove ? [step('tasks', t('slim.tasks.step'), tasksOK ? 'done' : 'failed', tasksOK ? undefined : tasksResult(tasksAnswer))] : []),
                ...(units.length > 0 ? moveSteps(answer, units, {
                    write: t('slim.move.step.write'), restart: t('slim.move.step.restart'), check: t('slim.move.step.check'),
                    reason: reasonText, why: t(moveWhyKey(answer.reason) as SiteLimitsLaneKey),
                }).map((s) => step(s.id, s.label, s.state, s.note)) : []),
            ]);
            const notMoved = answer.units.flatMap((u) => {
                const reason = notMovedReason(u.outcome);
                if (!reason) return [];
                return [`${labelOf(units, u.unit)} (${t(reason === 'changed' ? 'slim.move.changed' : `slim.blocked.${reason}` as TranslationKey)})`];
            });
            const failed = answer.units.find((u) => u.outcome === 'failed');
            if (answer.time_up) return { state: 'failed', reason: t('slim.move.timeUp') };
            if (answer.rolled_back && failed) {
                // "Put back" only for what was confirmed back and running again; each of
                // the rest is named with why (review W1; alpha.149, lab D6/D7).
                const { back, asWere, left } = putBackSplit(answer);
                const notBack = left.map((name) => {
                    const u = answer.units.find((x) => x.unit === name);
                    return u?.back?.code === 'changed' ? `${labelOf(units, name)} (${t('slim.move.changed')})`
                        : t('slim.move.backNot', { unit: labelOf(units, name), reason: reasonText(u?.back) });
                });
                return {
                    state: 'failed',
                    reason: [t('slim.move.failedUnit', { unit: labelOf(units, failed.unit), reason: reasonText(failed.reason), name: failed.unit }),
                        back.length > 0 ? t('slim.move.back', { list: back.map((u) => labelOf(units, u)).join(', ') }) : '',
                        asWere.length > 0 ? t('slim.move.backStopped', { list: asWere.map((u) => labelOf(units, u)).join(', ') }) : '', ...notBack].filter(Boolean).join(' '),
                };
            }
            // A kept move record, or a tasks part whose result is not known, starts no
            // programs move (alpha.150: one record for programs and tasks); the card says so.
            if (tasksAnswer?.refusal === KEPT_MOVE) return { state: 'blocked', reason: t('slim.why.kept_earlier') };
            if (tasksAnswer?.outcome === 'unknown') return { state: 'unknown', reason: tasksAnswer.error || t('slim.move.unknown') };
            if (!answer.done) {
                const why = answer.kept ? t('slim.why.kept_earlier') : t(moveWhyKey(answer.reason) as SiteLimitsLaneKey);
                return { state: 'failed', reason: [why, tasksResult(tasksAnswer)].filter(Boolean).join(' ') };
            }
            if (!tasksOK) return { state: tasksAnswer?.outcome === 'stayed' ? 'blocked' : 'failed', reason: tasksResult(tasksAnswer) };
            return { state: 'done', message: [units.length > 0 ? t('slim.move.done') : '', tasksResult(tasksAnswer), notMoved.length > 0 ? t('slim.move.notMoved', { list: notMoved.join('; ') }) : ''].filter(Boolean).join(' ') };
        },
    });
}

/**
 * Try again from Problems now (alpha.150, D-048): the site's card is read first (what the move
 * would take, as the card's own Try again sees it), then the same move. 'unread': the card could
 * not be read, so nothing was sent. A site's request id is kept here until an answer is read.
 */
export async function retrySiteMove(domainId: number, t: SiteLimitsLaneT): Promise<'done' | 'unread'> {
    const read = await readRemote(`/api/v1/domains/${domainId}/site-limits`, decodeSiteLimits);
    if (read.state !== 'known') return 'unread';
    let id = retryIDs.get(domainId);
    if (!id) retryIDs.set(domainId, (id = { id: newMoveRequestID() }));
    await moveSiteIntoLimits(domainId, read.value, t, id);
    return 'done';
}

const retryIDs = new Map<number, { id: string }>();

function labelOf(units: SiteLimitsView['units'], unit: string): string {
    const found = (units ?? []).find((u) => u.unit === unit);
    return found ? `${found.label} (${unit})` : unit;
}
