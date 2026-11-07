import { useRemote } from '../lib/remote';
import { CouldNotCheck } from './ui';
import { formatSize, useDataToolsWords } from './dataToolsWords';

// Visitor statistics of one site (alpha.128, G24): the last 7 and 30 days
// from the site's own access log, finished days kept by the Panel, today
// read live. Totals only: no address, no query string, referrers by host.
//
// Bir sitenin ziyaretçi istatistikleri: son 7 ve 30 gün; yalnız toplamlar.

interface StatsDay {
    day: string;
    hits: number;
    pageViews: number;
    visitors: number;
    bots: number;
    bytesSent: number;
}

interface Ranked {
    name: string;
    count: number;
}

interface Stats {
    today: string;
    days: StatsDay[];
    topPages: Ranked[];
    topReferrers: Ranked[];
    readFailed: boolean;
    partial: boolean;
}

const num = (x: unknown) => (typeof x === 'number' && Number.isFinite(x) ? x : 0);
const ranked = (x: unknown): Ranked[] => (Array.isArray(x) ? x : [])
    .filter((item): item is { name: string; count: number } => !!item && typeof item.name === 'string')
    .map((item) => ({ name: item.name, count: num(item.count) }));

function decodeStats(raw: unknown): Stats {
    const v = (raw ?? {}) as Record<string, unknown>;
    return {
        today: typeof v.today === 'string' ? v.today : '',
        days: (Array.isArray(v.days) ? v.days : []).map((d: Record<string, unknown>) => ({
            day: typeof d.day === 'string' ? d.day : '',
            hits: num(d.hits), pageViews: num(d.page_views), visitors: num(d.visitors), bots: num(d.bots), bytesSent: num(d.bytes_sent),
        })),
        topPages: ranked(v.top_pages),
        topReferrers: ranked(v.top_referrers),
        readFailed: v.read_failed === true,
        partial: v.partial === true,
    };
}

function sum(days: StatsDay[], pick: (d: StatsDay) => number) {
    return days.reduce((total, d) => total + pick(d), 0);
}

export function VisitorStats({ domainId }: { domainId: number }) {
    const { t, locale } = useDataToolsWords();
    const stats = useRemote(`/api/v1/domains/${domainId}/stats/visitors`, decodeStats, { freshFor: 60_000 });
    if (stats.remote.state === 'loading') {
        return <p className="text-sm text-fg-muted">{t('vst.reading')}</p>;
    }
    if (stats.remote.state === 'unknown') {
        return <CouldNotCheck text={t('vst.unread')} onRetry={() => void stats.retry()} busy={stats.reading} />;
    }
    const value = stats.remote.value;
    const byDay = [...value.days].sort((a, b) => a.day.localeCompare(b.day));
    const week = byDay.slice(-7);
    const today = byDay.find((d) => d.day === value.today);
    const tiles = [
        { label: t('vst.today'), visitors: today?.visitors ?? 0, views: today?.pageViews ?? 0 },
        { label: t('vst.week'), visitors: sum(week, (d) => d.visitors), views: sum(week, (d) => d.pageViews) },
        { label: t('vst.month'), visitors: sum(byDay, (d) => d.visitors), views: sum(byDay, (d) => d.pageViews) },
    ];
    const dayLabel = (day: string) => new Date(`${day}T12:00:00`).toLocaleDateString(locale, { weekday: 'short', day: 'numeric', month: 'short' });
    return (
        <section className="space-y-3 rounded-lg border border-border bg-surface-2/50 p-4">
            <div>
                <h4 className="text-sm font-semibold text-fg">{t('vst.title')}</h4>
                <p className="text-xs text-fg-muted">{t('vst.hint')}</p>
            </div>
            {value.readFailed && <p className="text-xs text-fg-muted">{t('vst.readFailed')}</p>}
            {value.partial && <p className="text-xs text-fg-muted">{t('vst.partial')}</p>}
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                {tiles.map((tile) => (
                    <div key={tile.label} className="rounded-lg border border-border bg-surface p-3">
                        <p className="text-xs text-fg-muted">{tile.label}</p>
                        <p className="text-lg font-semibold text-fg">{tile.visitors.toLocaleString(locale)}</p>
                        <p className="text-xs text-fg-muted">{t('vst.visitorsViews', { views: tile.views.toLocaleString(locale) })}</p>
                    </div>
                ))}
            </div>
            {week.length > 0 && (
                <div className="overflow-x-auto">
                    <table className="w-full text-left text-xs">
                        <thead className="text-fg-muted">
                            <tr><th className="py-1 pr-3 font-medium">{t('vst.day')}</th><th className="py-1 pr-3 font-medium">{t('vst.visitors')}</th><th className="py-1 pr-3 font-medium">{t('vst.views')}</th><th className="py-1 pr-3 font-medium">{t('vst.hits')}</th><th className="py-1 font-medium">{t('vst.sent')}</th></tr>
                        </thead>
                        <tbody className="text-fg">
                            {[...week].reverse().map((d) => (
                                <tr key={d.day} className="border-t border-border">
                                    <td className="py-1 pr-3">{dayLabel(d.day)}</td>
                                    <td className="py-1 pr-3">{d.visitors.toLocaleString(locale)}</td>
                                    <td className="py-1 pr-3">{d.pageViews.toLocaleString(locale)}</td>
                                    <td className="py-1 pr-3">{d.hits.toLocaleString(locale)}</td>
                                    <td className="py-1">{formatSize(d.bytesSent)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {week.length === 0 && <p className="text-sm text-fg-muted">{t('vst.none')}</p>}
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                {[{ title: t('vst.topPages'), items: value.topPages }, { title: t('vst.topReferrers'), items: value.topReferrers }].map((list) => (
                    <div key={list.title}>
                        <p className="mb-1 text-xs font-semibold text-fg-muted">{list.title}</p>
                        {list.items.length === 0 ? <p className="text-xs text-fg-subtle">{t('vst.noneYet')}</p> : (
                            <ol className="space-y-0.5 text-xs">
                                {list.items.map((item) => (
                                    <li key={item.name} className="flex justify-between gap-2">
                                        <span className="truncate font-mono text-fg">{item.name}</span>
                                        <span className="text-fg-muted">{item.count.toLocaleString(locale)}</span>
                                    </li>
                                ))}
                            </ol>
                        )}
                    </div>
                ))}
            </div>
        </section>
    );
}
