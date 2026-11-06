import { useEffect, useState } from 'react';
import { startVisibleReads } from '../lib/visibleReads';
import { Activity } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { PageHeader } from './PageHeader';
import { RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { ProblemsNow } from './ProblemsNow';

// Server monitoring (operator request, 23 Jul: "we should have a monitoring
// page"). The dashboard strip answers "how is it NOW"; this page answers
// "what happened while I was away". Charts are plain SVG — no chart library,
// per the no-external-dependency constitution.
//
// alpha.126 ("Monitoring tells the truth and names the problem"):
//   - "Problems now" at the top: what is wrong, who acts, the next action.
//   - Each point is the average of 30 seconds (it was a 200 ms snapshot that
//     drew an idle machine as spikes); the server sends at most 360 points
//     per range, each with its average and its highest value. The line is
//     the average, the faint band reaches the highest value, and the CPU
//     card names its short peak (the highest single second) in words only.
//   - Ranges 1 hour to 30 days; the CPU card says how many vCPU the
//     percentage is spread across.
//
// Sunucu izleme. alpha.126: üstte "Şu anki sorunlar"; her nokta 30 saniyenin
// ortalaması; sunucu aralık başına en çok 360 nokta (ortalama ve en yüksek)
// gönderir; çizgi ortalama, soluk bant en yüksek değer; işlemci kartı kısa
// tepeyi yalnız sözle söyler; aralıklar 1 saatten 30 güne.

interface Point {
    ts: string;
    cpu: number;
    cpu_max: number;
    cpu_peak?: number;
    mem_used: number;
    mem_used_max: number;
    mem_total: number;
    disk_used: number;
    disk_total: number;
    load1: number;
    load1_max: number;
}

interface History {
    samples: Point[];
    latest: Point | null;
    vcpus: number;
}

const RANGES = ['1h', '6h', '24h', '7d', '30d'] as const;
type Range = (typeof RANGES)[number];
const REFRESH_MS = 60_000;

const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

function decodePoint(raw: unknown): Point | null {
    if (!raw || typeof raw !== 'object') return null;
    const p = raw as Record<string, unknown>;
    if (typeof p.ts !== 'string' || typeof p.cpu !== 'number') return null;
    return {
        ts: p.ts,
        cpu: p.cpu,
        cpu_max: typeof p.cpu_max === 'number' ? p.cpu_max : p.cpu,
        cpu_peak: typeof p.cpu_peak === 'number' ? p.cpu_peak : undefined,
        mem_used: num(p.mem_used),
        mem_used_max: typeof p.mem_used_max === 'number' ? p.mem_used_max : num(p.mem_used),
        mem_total: num(p.mem_total),
        disk_used: num(p.disk_used),
        disk_total: num(p.disk_total),
        load1: num(p.load1),
        load1_max: typeof p.load1_max === 'number' ? p.load1_max : num(p.load1),
    };
}

// An answer without the samples is not "no samples yet".
// Örnekleri taşımayan yanıt "henüz örnek yok" değildir.
function decodeHistory(raw: unknown): History {
    const body = raw as { samples?: unknown; latest?: unknown; vcpus?: unknown } | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.samples)) throw new Error('shape');
    const samples = body.samples.map(decodePoint).filter((p): p is Point => p !== null);
    return { samples, latest: decodePoint(body.latest), vcpus: num(body.vcpus) };
}

export function MonitoringPage() {
    const { t } = useI18n();
    const [range, setRange] = useState<Range>('24h');
    // One address per range. The page reads again every minute; a read that
    // fails keeps the charts it had, under a notice that says when they were
    // read, instead of wiping them into "no samples yet".
    // Aralık başına bir adres. Sayfa her dakika yeniden okur; başarısız okuma
    // grafikleri silmez.
    const history = useRemote(`/api/v1/metrics/history?range=${range}`, decodeHistory);
    const refresh = history.retry;

    useEffect(() => {
        // alpha.125: no refetch while the tab is hidden; one when shown again.
        // Gizli sekmede yeniden okuma yok; görününce bir kez okur.
        return startVisibleReads(refresh, REFRESH_MS, { setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number, clearInterval: (id) => clearInterval(id) });
    }, [refresh]);

    const pctOf = (used: number, total: number) => (total > 0 ? (used / total) * 100 : 0);
    const pct = (n: number) => t('monitoring.pct', { n: n.toFixed(0) });
    const fmtGB = (n: number) => `${(n / 1024 ** 3).toFixed(1)} GB`;

    return (
        <div className="p-6 md:p-8">
            <PageHeader
                title={t('nav.monitoring')}
                subtitle={t('monitoring.subtitle')}
                breadcrumb={[t('common.home'), t('nav.monitoring')]}
            />

            <ProblemsNow here="/monitoring" />

            <div className="mb-4 flex flex-wrap items-center gap-1" role="group" aria-label={t('nav.monitoring')}>
                {RANGES.map((key) => (
                    <button
                        key={key}
                        type="button"
                        aria-pressed={range === key}
                        onClick={() => setRange(key)}
                        className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors max-sm:min-h-11 ${
                            range === key ? 'bg-primary text-primary-fg' : 'text-fg-muted hover:bg-surface-2'
                        }`}
                    >
                        {t(`monitoring.range.${key}` as TranslationKey)}
                    </button>
                ))}
            </div>

            <RemoteGate
                remote={history.remote}
                checking={t('monitoring.checking')}
                failed={t('monitoring.unknown')}
                onRetry={() => void history.retry()}
                busy={history.reading}
                className="py-3"
            >
                {({ value }) => {
                    const samples = value.samples;
                    // "Not enough samples yet" is the server's answer: it sent
                    // fewer than two.
                    if (samples.length < 2) {
                        return (
                            <div className="flex flex-col items-center gap-2 rounded-xl border border-border bg-surface p-10 text-center">
                                <Activity className="h-8 w-8 text-fg-subtle" />
                                <p className="text-sm text-fg-muted">{t('monitoring.empty')}</p>
                            </div>
                        );
                    }
                    const last = value.latest ?? samples[samples.length - 1];
                    const peaks = samples.map((s) => s.cpu_peak).filter((v): v is number => typeof v === 'number');
                    const shortPeak = peaks.length > 0 ? Math.max(...peaks) : undefined;
                    const loadMax = samples.map((s) => s.load1_max);
                    const loadScale = Math.max(1, ...loadMax) * 1.15;
                    return (
                        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                            <MetricChart
                                title={t('monitoring.cpu')}
                                badge={value.vcpus > 0 ? t('monitoring.vcpu', { n: value.vcpus }) : undefined}
                                average={samples.map((s) => s.cpu)}
                                highest={samples.map((s) => s.cpu_max)}
                                max={100}
                                current={pct(last.cpu)}
                                footer={[
                                    t('monitoring.highest', { value: pct(Math.max(...samples.map((s) => s.cpu_max))) }),
                                    shortPeak !== undefined ? t('monitoring.shortPeak', { value: pct(shortPeak) }) : '',
                                ]}
                                hint={value.vcpus > 0 ? t('monitoring.cpuHint', { n: value.vcpus }) : undefined}
                                stroke="var(--color-primary, #3b82f6)"
                            />
                            <MetricChart
                                title={t('monitoring.memory')}
                                average={samples.map((s) => pctOf(s.mem_used, s.mem_total))}
                                highest={samples.map((s) => pctOf(s.mem_used_max, s.mem_total))}
                                max={100}
                                current={`${pct(pctOf(last.mem_used, last.mem_total))} · ${fmtGB(last.mem_used)} / ${fmtGB(last.mem_total)}`}
                                footer={[t('monitoring.highest', { value: pct(Math.max(...samples.map((s) => pctOf(s.mem_used_max, s.mem_total)))) })]}
                                stroke="#8b5cf6"
                            />
                            <MetricChart
                                title={t('monitoring.disk')}
                                average={samples.map((s) => pctOf(s.disk_used, s.disk_total))}
                                max={100}
                                current={`${pct(pctOf(last.disk_used, last.disk_total))} · ${fmtGB(last.disk_used)} / ${fmtGB(last.disk_total)}`}
                                footer={[t('monitoring.highest', { value: pct(Math.max(...samples.map((s) => pctOf(s.disk_used, s.disk_total)))) })]}
                                stroke="#f59e0b"
                            />
                            <MetricChart
                                title={t('monitoring.load')}
                                average={samples.map((s) => s.load1)}
                                highest={loadMax}
                                max={loadScale}
                                current={last.load1.toFixed(2)}
                                footer={[t('monitoring.highest', { value: Math.max(...loadMax).toFixed(2) })]}
                                stroke="#10b981"
                            />
                        </div>
                    );
                }}
            </RemoteGate>
        </div>
    );
}

// One SVG area chart. viewBox 0..100 × 0..40 stretched to the card; the
// series is normalized against `max` so percent charts share a fixed scale
// (a half-empty disk must LOOK half empty) while load auto-scales. The line
// is the bucket average; the faint band above it reaches the bucket's
// highest value.
// Tek SVG alan grafiği. Çizgi kova ortalaması; üstündeki soluk bant kovanın
// en yüksek değerine uzanır.
function MetricChart({
    title,
    badge,
    average,
    highest,
    max,
    current,
    footer,
    hint,
    stroke,
}: {
    title: string;
    badge?: string;
    average: number[];
    highest?: number[];
    max: number;
    current: string;
    footer: string[];
    hint?: string;
    stroke: string;
}) {
    const W = 100;
    const H = 40;
    const n = average.length;
    const x = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * W);
    const y = (v: number) => H - Math.min(Math.max(v / max, 0), 1) * H;
    const path = (series: number[]) => series.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(2)},${y(v).toFixed(2)}`).join(' ');
    const line = path(average);
    const area = `${line} L${W},${H} L0,${H} Z`;
    // The band: up along the highest values, back along the averages.
    const band = highest && highest.length === n
        ? `${path(highest)} ${average.map((_, i) => `L${x(n - 1 - i).toFixed(2)},${y(average[n - 1 - i]).toFixed(2)}`).join(' ')} Z`
        : '';

    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            <div className="mb-2 flex items-baseline justify-between gap-3">
                <h3 className="text-sm font-semibold text-fg">
                    {title}
                    {badge && <span className="ml-2 text-xs font-normal text-fg-subtle">{badge}</span>}
                </h3>
                <span className="font-mono text-sm text-fg-muted">{current}</span>
            </div>
            <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-28 w-full" aria-hidden="true">
                {band && <path d={band} fill={stroke} opacity="0.18" />}
                <path d={area} fill={stroke} opacity="0.12" />
                <path d={line} fill="none" stroke={stroke} strokeWidth="0.8" vectorEffect="non-scaling-stroke" />
            </svg>
            <div className="mt-1 flex flex-wrap justify-between gap-x-3 text-xs text-fg-subtle">
                {footer.filter(Boolean).map((text) => <span key={text}>{text}</span>)}
            </div>
            {hint && <p className="mt-2 text-xs leading-5 text-fg-subtle">{hint}</p>}
        </div>
    );
}
