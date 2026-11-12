import { LayoutGrid } from 'lucide-react';
import { useI18n } from '../i18n';
import { RemoteGate } from './ui';
import type { RemoteHandle } from '../lib/remote';
import { appText, type CatalogApp } from '../lib/appCatalog';
import type { Draft } from './kit/save/draft';
import { ChangedMark, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';

// The administrator's offered applications (alpha.118, D-042): every catalog
// application with its version, licence, source, how its download is checked,
// what it needs and a short risk note. An application is offered only when
// the administrator ticks it and saves (third-party software is gated at the
// offer step: Save asks first, with the risk); WordPress stays offered on a
// server that had it. Withdrawing changes nothing installed: the sites keep
// their application, no new one is installed. The Unlimited plans follow at
// once (P4). Fetched when the Offered tab is opened (its own chunk).
// alpha.132: a tick changes the Offered tab's one draft (PlanRights.tsx
// OfferedItemsTab), which reads the list and saves; this part draws it.
//
// Yöneticinin sunduğu uygulamalar: her katalog uygulaması sürümü, lisansı,
// kaynağı, indirmenin nasıl doğrulandığı, gereksinimleri ve kısa bir risk
// notuyla. Uygulama yalnız yönetici işaretleyip kaydettiğinde sunulur.
export function OfferedAppsSection({ catalog, draft }: { catalog: RemoteHandle<CatalogApp[]>; draft: Draft<Record<string, boolean>> }) {
    const { t, locale } = useI18n();
    return (
        <section className="mt-6" data-offered-apps="">
            <h3 className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg"><LayoutGrid className="h-4 w-4" />{t('appOffer.title')}</h3>
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('appOffer.intro')}</p>
            <RemoteGate remote={catalog.remote} checking={t('apps.checking')} failed={t('apps.unknown')} onRetry={() => void catalog.retry()} busy={catalog.reading}>
                {(known) => (
                    <ul className="grid gap-2 lg:grid-cols-2">
                        {known.value.map((app) => {
                            const offered = draft.values?.[app.offering_id] ?? app.offered;
                            const changed = draft.isChanged(app.offering_id);
                            return (
                                <li key={app.id} className={`rounded-lg border border-border bg-surface p-3 ${changed ? changedRowClass : ''}`} data-offered-app={app.id} data-offered={offered ? 'yes' : 'no'}>
                                    <label className="flex cursor-pointer items-start gap-2.5 max-sm:min-h-11">
                                        <input type="checkbox" checked={offered} disabled={draft.saving || known.stale}
                                            onChange={(e) => draft.set(app.offering_id, e.target.checked)} className="mt-1 h-4 w-4 shrink-0 accent-primary"
                                            aria-label={t('rights.offered.toggle', { name: app.name })} />
                                        <span className="min-w-0">
                                            <span className="font-medium text-fg">{app.name} {app.version}</span>
                                            <span className={`ml-2 text-xs ${offered ? 'text-fg-muted' : 'font-semibold text-warning'}`}>{t(offered ? 'rights.offered.yes' : 'rights.offered.no')}</span>
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
        </section>
    );
}
