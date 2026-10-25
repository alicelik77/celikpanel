// "Offered to customers" (v0.1.0-alpha.108, D-038 §14): on a runtime's own
// Manage page the administrator chooses, per version line, whether customers
// may pick it for their sites. Hiding a version changes no site: a site
// already on it keeps it and its owner's page says so. An administrator can
// always choose any installed version. Each change is one request, audited by
// the server; the card shows what the server holds after it answers.
// "Müşterilere sunulan": yönetici, müşterilerin seçebileceği sürümleri seçer.
import { useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { CouldNotCheck, Checking } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { useSupportTable } from '../lib/supportTable';
import { SupportBadge } from './SupportMark';
import { decodeOfferedVersions, offeredLine, OFFERED_VERSIONS_URL, type OfferedComponent } from '../lib/offeredVersions';

const NAMES: Record<OfferedComponent, string> = { php: 'PHP', node: 'Node.js' };

/** The lines to show: every installed one, and any hidden one no longer installed. */
export function offeredLines(component: OfferedComponent, installed: string[], hidden: string[]): { line: string; versions: string[] }[] {
    const byLine = new Map<string, string[]>();
    for (const version of installed) {
        const line = offeredLine(component, version);
        if (line) byLine.set(line, [...(byLine.get(line) ?? []), version]);
    }
    for (const line of hidden) if (!byLine.has(line)) byLine.set(line, []);
    const numeric = (line: string) => line.split('.').map((part) => Number(part));
    return [...byLine.entries()]
        .map(([line, versions]) => ({ line, versions }))
        .sort((a, b) => {
            const [x, y] = [numeric(a.line), numeric(b.line)];
            return (y[0] - x[0]) || ((y[1] ?? 0) - (x[1] ?? 0));
        });
}

export function OfferedVersionsCard({ component, installed }: { component: OfferedComponent; installed: string[] }) {
    const { t } = useI18n();
    const handle = useRemote(OFFERED_VERSIONS_URL, decodeOfferedVersions);
    const support = useSupportTable();
    const [pending, setPending] = useState<string | null>(null);
    const [failure, setFailure] = useState<string | null>(null);
    const name = NAMES[component];
    const remote = handle.remote;
    const hidden = remote.state === 'known' ? remote.value.hidden[component] : [];
    const lines = offeredLines(component, installed, hidden);
    if (lines.length === 0) return null;

    const change = async (line: string, offered: boolean) => {
        if (pending) return;
        setPending(line);
        setFailure(null);
        try {
            const res = await fetch(OFFERED_VERSIONS_URL, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ component, version: line, offered }),
            });
            if (!res.ok) setFailure(apiErrorText(await readApiError(res), t, 'life.offer.failed'));
        } catch {
            // No answer: the list read again says what the server holds; the
            // change is not sent a second time.
            setFailure(t('life.offer.lost'));
        }
        await handle.retry();
        setPending(null);
    };

    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-offered-versions={component}>
            <h3 className="text-sm font-semibold text-fg">{t('life.offer.title')}</h3>
            <p className="mt-1 max-w-[75ch] text-sm leading-6 text-fg-muted">{t('life.offer.intro', { name })}</p>
            {remote.state === 'loading' && <Checking className="mt-3" label={t('life.offer.checking')} />}
            {remote.state === 'unknown' && (
                <CouldNotCheck className="mt-3" text={t('life.offer.unknown')} onRetry={() => void handle.retry()} busy={handle.reading} />
            )}
            {remote.state === 'known' && (
                <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
                    {lines.map(({ line, versions }) => {
                        const offered = !hidden.includes(line);
                        const label = versions.length > 1 ? versions.join(', ') : versions[0] ?? line;
                        return (
                            <li key={line} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2" data-offered-line={line} data-offered={offered ? 'yes' : 'no'}>
                                <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 max-sm:min-h-11">
                                    <input
                                        type="checkbox"
                                        checked={offered}
                                        disabled={pending !== null || handle.reading}
                                        onChange={(event) => void change(line, event.target.checked)}
                                        className="h-4 w-4 shrink-0 accent-primary"
                                        aria-label={t('life.offer.toggle', { name, version: line })}
                                    />
                                    <span className="min-w-0 text-sm text-fg">
                                        <span className="font-medium">{name} {label}</span>
                                        {versions.length === 0 && <span className="text-fg-muted"> {t('life.offer.notInstalled')}</span>}
                                    </span>
                                </label>
                                <SupportBadge table={support} component={component} version={line} />
                                <span className={`text-xs ${offered ? 'text-fg-muted' : 'font-semibold text-warning'}`}>
                                    {t((offered ? 'life.offer.offered' : 'life.offer.hidden') as TranslationKey)}
                                </span>
                            </li>
                        );
                    })}
                </ul>
            )}
            {pending && <Checking className="mt-2" label={t('life.offer.saving', { name, version: pending })} />}
            {failure && <p className="mt-2 text-sm text-danger" role="alert" data-offered-failure="">{failure}</p>}
        </section>
    );
}

/** The note under a site's version picker when its version is hidden from customers. */
export function OfferedVersionNote({ component, version, offered, restricted }: {
    component: OfferedComponent;
    version: string;
    offered: boolean;
    restricted: boolean;
}) {
    const { t } = useI18n();
    if (!version || offered) return null;
    return (
        <p className="mt-1.5 rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-sm leading-6 text-fg" data-offered-note={restricted ? 'customer' : 'admin'}>
            {t(restricted ? 'life.offer.customerKept' : 'life.offer.adminKept', { name: NAMES[component], version })}
        </p>
    );
}
