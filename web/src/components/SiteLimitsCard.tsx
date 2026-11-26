import { useState } from 'react';
import { AlertTriangle, ArrowRightLeft, CheckCircle2, Gauge } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { withSiteLimitsLane, type SiteLimitsLaneKey } from '../i18n/siteLimitsLane';
import { formatDateTime } from '../lib/dateFormat';
import { readApiError, apiErrorText } from '../lib/apiError';
import { confirmAction, followOperation, type OperationEnd, type OperationStep } from '../lib/confirmAction';
import { useRemote } from '../lib/remote';
import { unitText, whoText } from '../lib/settingLimits';
import {
    appliedOf, decodeMoveAnswer, decodeSiteLimits, eventAtSiteLimit, eventKey, keptUnits, limitOf, lowerUnits, MOVE_WAIT_SECONDS, moveReasonWords, moveSteps, moveWhyKey,
    newMoveRequestID, notMovedReason, putBackSplit, type MoveReason, type SiteLimitKey, type SiteLimitValue, type SiteLimitsView,
} from '../lib/siteLimits';
import { Button, Checking, CouldNotCheck } from './ui';

// The limits of a site's programs together (S4 R2a, docs/SITE-SANDBOX.md
// section 8), on the domain's Overview when the site runs a program. Everyone
// who sees the domain reads the three limits, whether they apply at all, what
// the programs use now and, in plain words, when they reached a limit, what
// the plan allows and who can raise it (a reseller is that provider). An
// event the kernel does not prove at the site's limit never names the plan.
// The administrator also reads where each program runs, what keeps one from
// moving and who acts, the server owner's own additions, and moves the ones
// still outside with one action: the page waits in the panel's own window
// with the steps, and the card reads again when it is done.
//
// Bir sitenin programlarının birlikte sınırları: herkes sınırları, uygulanıp
// uygulanmadığını, şu anki kullanımı ve ne zaman aşıldığını okur; yönetici
// dışarıda kalan programları tek işlemle taşır.

const ORDER: { key: SiteLimitKey; label: TranslationKey }[] = [
    { key: 'site.memory_mb', label: 'slim.memory' },
    { key: 'site.tasks', label: 'slim.tasks' },
    { key: 'site.cpu_percent', label: 'slim.cpu' },
];

export function SiteLimitsCard({ domainId }: { domainId: number }) {
    const { t: screenT, locale } = useI18n();
    const t = withSiteLimitsLane(locale, screenT);
    const url = `/api/v1/domains/${domainId}/site-limits`;
    const handle = useRemote(url, decodeSiteLimits);
    const remote = handle.remote;
    if (remote.state === 'loading') return <Checking label={t('slim.checking')} className="mb-4" />;
    if (remote.state === 'unknown' && !remote.previous) {
        return <div className="mb-4"><CouldNotCheck text={t('slim.unreadLimits')} onRetry={() => void handle.retry()} busy={handle.reading} /></div>;
    }
    const view = remote.state === 'known' ? remote.value : (remote.previous!.value as SiteLimitsView);

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
    // alpha.149 (lab D7): a reason in the page's own words, never the agent's English text.
    const reasonText = (reason: MoveReason | undefined) => {
        const words = moveReasonWords(reason);
        return t(words.key as SiteLimitsLaneKey, words.values);
    };
    const [working, setWorking] = useState(false);
    // A lost answer keeps the request id, so pressing again returns the first
    // answer instead of starting a second move.
    const [requestID, setRequestID] = useState(newMoveRequestID);
    const units = view.units ?? [];
    const lower = lowerUnits(view);
    const edited = view.slice_file === 'owner_edited' || view.slice_file === 'foreign' || view.slice_file === 'unknown_origin';
    const owner = (view.db_owner ?? []).map((s) => `${s.user} (${t(`limits.key.${s.key}` as TranslationKey)}: ${s.value || '—'})`);

    // The product's own question and progress window: the page stays locked
    // with the steps until the server answers, then says how it ended.
    const ask = async () => {
        const moving = units.filter((u) => u.placement === 'outside' && !u.blocked);
        const step = (id: string, label: string, state: OperationStep['state'], note?: string): OperationStep => ({ id, label, state, note, since: Date.now() });
        setWorking(true);
        try {
            await confirmAction({
                title: t('slim.move.confirm', { count: view.needs_move ?? 0 }),
                message: t('slim.move.body', { seconds: MOVE_WAIT_SECONDS }) + (lower.length > 0 ? '\n\n' + t('slim.move.lowerWarn', { list: lower.map((u) => u.label).join(', ') }) : ''),
                confirmLabel: t('slim.move.go'),
                cancelLabel: t('slim.move.cancel'),
                progressTitle: t('slim.move.title'),
                action: async (report): Promise<OperationEnd> => {
                    report.steps([
                        step('write', t('slim.move.step.write'), 'running'),
                        ...moving.map((u) => step(u.unit, `${t('slim.move.step.restart')}: ${u.label}`, 'pending')),
                        step('check', t('slim.move.step.check'), 'pending'),
                    ]);
                    let sent: Response;
                    try {
                        sent = await fetch(`/api/v1/domains/${domainId}/site-limits/move`, {
                            method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: requestID }),
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
                    report.steps(moveSteps(answer, units, {
                        write: t('slim.move.step.write'), restart: t('slim.move.step.restart'), check: t('slim.move.step.check'),
                        reason: reasonText, why: t(moveWhyKey(answer.reason) as SiteLimitsLaneKey),
                    }).map((s) => step(s.id, s.label, s.state, s.note)));
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
                    if (!answer.done) return { state: 'failed', reason: answer.kept ? t('slim.why.kept_earlier') : t(moveWhyKey(answer.reason) as SiteLimitsLaneKey) };
                    return { state: 'done', message: [t('slim.move.done'), notMoved.length > 0 ? t('slim.move.notMoved', { list: notMoved.join('; ') }) : ''].filter(Boolean).join(' ') };
                },
            });
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
                    if (answer.done) return { state: 'done' };
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
            {view.busy && <p className="mt-2 text-sm text-fg-muted">{t('slim.move.busy')}</p>}
            {(view.needs_move ?? 0) > 0 && view.applies !== 'unsupported' && (
                <div className="mt-3 space-y-2 rounded-lg border border-border-strong bg-surface-2 px-3 py-2">
                    <p className="text-sm font-semibold text-fg">{t('slim.move.title')}</p>
                    <p className="text-sm text-fg-muted">{t('slim.move.body', { seconds: MOVE_WAIT_SECONDS })}</p>
                    <Button variant="primary" icon={ArrowRightLeft} loading={working} disabled={working || view.busy === true} onClick={() => void ask()}>
                        {t('slim.move.button')}
                    </Button>
                </div>
            )}
        </div>
    );
}

function labelOf(units: SiteLimitsView['units'], unit: string): string {
    const found = (units ?? []).find((u) => u.unit === unit);
    return found ? `${found.label} (${unit})` : unit;
}
