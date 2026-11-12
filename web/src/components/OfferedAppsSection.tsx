import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { LayoutGrid } from 'lucide-react';
import { useI18n } from '../i18n';
import { RemoteGate } from './ui';
import { apiErrorActionLabel } from '../lib/apiError';
import { useNavigate } from '../router';
import { invalidate, lastKnown, useRemote } from '../lib/remote';
import { APP_CATALOG_URL, appText, decodeCatalog } from '../lib/appCatalog';
import { confirmAction } from '../lib/confirmAction';
import { applyOffers, type OffersApplied } from '../lib/rights';
import type { OfferedPart } from './PlanRights';
import { useDraft } from './kit/save/draft';
import { ChangedMark, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// The administrator's offered applications (alpha.118, D-042): every catalog
// application with its version, licence, source, how its download is checked,
// what it needs and a short risk note. An application is offered only when
// the administrator ticks it and saves (third-party software is gated at the
// offer step: Save asks first, with the risk, and the change then runs in
// the progress window); WordPress stays offered on a server that had it.
// Withdrawing changes nothing installed: the sites keep their application,
// no new one is installed. The Unlimited plans follow at once (P4). Fetched
// when the Offered tab is opened (its own chunk).
// alpha.132: a tick no longer saves; the ticks are a draft saved through
// applyOffers (lib/rights.ts) by the Offered tab's one save bar, which this
// part feeds through `onPart`.
//
// Yöneticinin sunduğu uygulamalar: her katalog uygulaması sürümü, lisansı,
// kaynağı, indirmenin nasıl doğrulandığı, gereksinimleri ve kısa bir risk
// notuyla. Uygulama yalnız yönetici işaretleyip kaydettiğinde sunulur.
export function OfferedAppsSection({ onPart }: { onPart: (part: OfferedPart | null) => void }) {
    const { t, locale } = useI18n();
    const say = useSaveWords();
    const navigate = useNavigate();
    const catalog = useRemote(APP_CATALOG_URL, decodeCatalog);
    const list = lastKnown(catalog.remote)?.value;
    const known = useMemo(() => (list ? Object.fromEntries(list.map((app) => [app.offering_id, app.offered])) : null), [list]);
    const draft = useDraft<Record<string, boolean>>(known);
    const [report, setReport] = useState<OffersApplied | null>(null);
    const appOf = (id: string) => list?.find((app) => app.offering_id === id);

    const save = () => draft.save(async (changes) => {
        setReport(null);
        const entries = Object.entries(changes) as [string, boolean][];
        const send = (report?: Parameters<typeof applyOffers>[1]['report']) => applyOffers(entries, {
            t,
            report,
            nameOf: (id) => appOf(id)?.name ?? id,
            reread: async () => {
                invalidate('rights');
                const reread = await catalog.retry();
                return (id) => (reread.state === 'known' ? reread.value.find((app) => app.offering_id === id)?.offered : undefined);
            },
        });
        const offered = entries.filter(([, on]) => on).map(([id]) => appOf(id)).filter((app) => app !== undefined);
        const done: { result?: OffersApplied } = {};
        if (offered.length > 0) {
            // Offering asks first, with the risk; the changes then run in the
            // progress window.
            await confirmAction({
                message: offered.map((app) => t('appOffer.confirm', { name: app.name, risk: appText(app, 'risk', locale) })).join('\n\n'),
                confirmLabel: say('save'),
                progressTitle: t('rights.offered.applying'),
                action: async (report) => {
                    const result = await send(report);
                    done.result = result;
                    if (result.failed.length === 0) return { state: 'done' };
                    return { state: result.lost ? 'unknown' : 'failed', reason: `${result.failed.map((line) => line.text).join(' ')} ${t('rights.offered.next')}` };
                },
            });
            // Declined: nothing was sent and the ticks stay.
            if (!done.result) return 'refused';
        } else done.result = await send();
        if (done.result.failed.length === 0) return 'saved';
        setReport(done.result);
        return done.result.lost ? 'unknown' : 'refused';
    });

    // The tab's bar gets stable functions over this render's draft.
    const latest = useRef({ save, reset: draft.reset, check: () => void catalog.retry() });
    latest.current = { save, reset: () => { setReport(null); draft.reset(); }, check: () => void catalog.retry() };
    const partSave = useCallback(() => latest.current.save(), []);
    const partReset = useCallback(() => latest.current.reset(), []);
    const partCheck = useCallback(() => latest.current.check(), []);
    const blocked = draft.count > 0 && catalog.remote.state !== 'known';
    const unknown = draft.outcome === 'unknown';
    useEffect(() => {
        onPart({ count: draft.count, saving: draft.saving, blocked, unknown, held: draft.held, checking: catalog.reading, save: partSave, reset: partReset, check: partCheck });
    }, [onPart, draft.count, draft.saving, blocked, unknown, draft.held, catalog.reading, partSave, partReset, partCheck]);
    useEffect(() => () => onPart(null), [onPart]);

    return (
        <section className="mt-6" data-offered-apps="">
            <h3 className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg"><LayoutGrid className="h-4 w-4" />{t('appOffer.title')}</h3>
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('appOffer.intro')}</p>
            <RemoteGate remote={catalog.remote} checking={t('apps.checking')} failed={t('apps.unknown')} onRetry={() => void catalog.retry()} busy={catalog.reading}>
                {(shown) => (
                    <ul className="grid gap-2 lg:grid-cols-2">
                        {shown.value.map((app) => {
                            const offered = draft.values?.[app.offering_id] ?? app.offered;
                            const changed = draft.isChanged(app.offering_id);
                            // The word beside the name is the server's state; the mark says what Save changes.
                            const live = draft.base?.[app.offering_id] ?? app.offered;
                            return (
                                <li key={app.id} className={`rounded-lg border border-border bg-surface p-3 ${changed ? changedRowClass : ''}`} data-offered-app={app.id} data-offered={offered ? 'yes' : 'no'}>
                                    <label className="flex cursor-pointer items-start gap-2.5 max-sm:min-h-11">
                                        <input type="checkbox" checked={offered} disabled={draft.saving || shown.stale}
                                            onChange={(e) => draft.set(app.offering_id, e.target.checked)} className="mt-1 h-4 w-4 shrink-0 accent-primary"
                                            aria-label={t('rights.offered.toggle', { name: app.name })} />
                                        <span className="min-w-0">
                                            <span className="font-medium text-fg">{app.name} {app.version}</span>
                                            <span className={`ml-2 text-xs ${live ? 'text-fg-muted' : 'font-semibold text-warning'}`} data-offered-now={live ? 'yes' : 'no'}>{t(live ? 'rights.offered.yes' : 'rights.offered.no')}</span>
                                            {changed && <ChangedMark switched={offered} />}
                                            <span className="block text-xs text-fg-muted">{appText(app, 'description', locale)}</span>
                                        </span>
                                    </label>
                                    {draft.isMoved(app.offering_id) && draft.base && (
                                        <ServerMovedNote value={t(draft.base[app.offering_id] ? 'rights.offered.yes' : 'rights.offered.no')} onTake={() => draft.take(app.offering_id)} />
                                    )}
                                    <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-fg-muted">
                                        <dt>{t('apps.licence')}</dt><dd className="break-words">{app.license}</dd>
                                        <dt>PHP</dt><dd>{app.php || '—'}</dd>
                                        <dt>{t('apps.extensions')}</dt><dd className="break-words">{app.extensions.join(', ') || '—'}</dd>
                                        <dt>{t('apps.database')}</dt><dd>{app.databases.join(', ') || '—'}</dd>
                                        <dt>{t('apps.source')}</dt><dd className="break-all">{app.source}</dd>
                                        <dt>{t('apps.verified')}</dt><dd className="break-words">{app.pinned ? t('apps.pinnedSha') : ''} {appText(app, 'checked', locale)}</dd>
                                        <dt>{t('apps.risk')}</dt><dd className="break-words">{appText(app, 'risk', locale)}</dd>
                                    </dl>
                                </li>
                            );
                        })}
                    </ul>
                )}
            </RemoteGate>
            {report && report.failed.length > 0 && (
                <div className="mt-3 rounded-lg border border-danger/40 bg-danger/5 px-3 py-2 text-sm leading-6 text-fg" role="alert" data-offered-report="">
                    {report.saved.length > 0 && <p>{t('rights.offered.savedSome', { what: report.saved.join(', ') })}</p>}
                    {report.failed.map((line) => (
                        <p key={line.text} className="flex flex-wrap items-center gap-x-3 gap-y-1">
                            <span className="min-w-0 max-w-[75ch]">{line.text}</span>
                            {line.fix && <button type="button" onClick={() => navigate(line.fix!.action!)} className="text-xs font-semibold text-primary hover:underline max-sm:min-h-11">{apiErrorActionLabel(line.fix, t)}</button>}
                        </p>
                    ))}
                </div>
            )}
        </section>
    );
}
