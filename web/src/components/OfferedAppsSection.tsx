import { useState } from 'react';
import { LayoutGrid } from 'lucide-react';
import { useI18n } from '../i18n';
import { showToast } from './Toast';
import { Checking, ErrorBanner, RemoteGate } from './ui';
import type { ApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { OFFERED_URL, sendRights } from '../lib/rights';
import { APP_CATALOG_URL, decodeCatalog, type CatalogApp } from '../lib/appCatalog';
import { confirmAction } from '../lib/confirmAction';

// The administrator's offered applications (alpha.118, D-042): every catalog
// application with its version, licence, source, how its download is checked,
// what it needs and a short risk note. An application is offered only when
// the administrator ticks it (third-party software is gated at the offer
// step); WordPress stays offered on a server that had it. Withdrawing changes
// nothing installed: the sites keep their application, no new one is
// installed. The Unlimited plans follow at once (P4). Fetched when the
// Offered tab is opened (its own chunk).
//
// Yöneticinin sunduğu uygulamalar: her katalog uygulaması sürümü, lisansı,
// kaynağı, indirmenin nasıl doğrulandığı, gereksinimleri ve kısa bir risk
// notuyla. Uygulama yalnız yönetici işaretlediğinde sunulur.
export function OfferedAppsSection() {
    const { t } = useI18n();
    const catalog = useRemote(APP_CATALOG_URL, decodeCatalog);
    const [pending, setPending] = useState<string | null>(null);
    const [failure, setFailure] = useState<ApiError | null>(null);

    const change = async (app: CatalogApp, offered: boolean) => {
        if (pending) return;
        if (offered && !(await confirmAction({ message: t('appOffer.confirm', { name: app.name, risk: app.risk }) }))) return;
        setPending(app.id);
        setFailure(null);
        try {
            const result = await sendRights(OFFERED_URL, 'PUT', { offering_id: app.offering_id, offered });
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            if (result.kind === 'refused') setFailure(result.error);
            invalidate('rights');
            await catalog.retry();
        } finally {
            setPending(null);
        }
    };

    return (
        <section className="mt-6" data-offered-apps="">
            <h3 className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg"><LayoutGrid className="h-4 w-4" />{t('appOffer.title')}</h3>
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('appOffer.intro')}</p>
            <RemoteGate remote={catalog.remote} checking={t('apps.checking')} failed={t('apps.unknown')} onRetry={() => void catalog.retry()} busy={catalog.reading}>
                {(known) => (
                    <ul className="grid gap-2 lg:grid-cols-2">
                        {known.value.map((app) => (
                            <li key={app.id} className="rounded-lg border border-border bg-surface p-3" data-offered-app={app.id} data-offered={app.offered ? 'yes' : 'no'}>
                                <label className="flex cursor-pointer items-start gap-2.5 max-sm:min-h-11">
                                    <input type="checkbox" checked={app.offered} disabled={pending !== null || known.stale}
                                        onChange={(e) => void change(app, e.target.checked)} className="mt-1 h-4 w-4 shrink-0 accent-primary"
                                        aria-label={t('rights.offered.toggle', { name: app.name })} />
                                    <span className="min-w-0">
                                        <span className="font-medium text-fg">{app.name} {app.version}</span>
                                        <span className={`ml-2 text-xs ${app.offered ? 'text-fg-muted' : 'font-semibold text-warning'}`}>{t(app.offered ? 'rights.offered.yes' : 'rights.offered.no')}</span>
                                        <span className="block text-xs text-fg-muted">{app.description}</span>
                                    </span>
                                </label>
                                <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-fg-muted">
                                    <dt>{t('apps.licence')}</dt><dd className="break-words">{app.license}</dd>
                                    <dt>PHP</dt><dd>{app.php || '—'}</dd>
                                    <dt>{t('apps.extensions')}</dt><dd className="break-words">{app.extensions.join(', ') || '—'}</dd>
                                    <dt>{t('apps.database')}</dt><dd>{app.databases.join(', ') || '—'}</dd>
                                    <dt>{t('apps.source')}</dt><dd className="break-all">{app.source}</dd>
                                    <dt>{t('apps.verified')}</dt><dd className="break-words">{app.pinned ? t('apps.pinnedSha') : ''} {app.checked}</dd>
                                    <dt>{t('apps.risk')}</dt><dd className="break-words">{app.risk}</dd>
                                </dl>
                            </li>
                        ))}
                    </ul>
                )}
            </RemoteGate>
            {pending && <Checking className="mt-2" label={t('rights.offered.saving', { name: pending })} />}
            <ErrorBanner className="mt-3" error={failure} sentence="rights.offered.failed" />
        </section>
    );
}
