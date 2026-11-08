import { useEffect, useState } from 'react';
import { AlertTriangle, Bell, CheckCircle2, Globe, Server, XCircle } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { monitoringAlertsTranslation } from '../i18n/monitoringAlerts';
import { apiErrorText, readApiError } from '../lib/apiError';
import { componentName } from '../lib/componentName';
import { useRemote, type RemoteHandle } from '../lib/remote';
import { startVisibleReads } from '../lib/visibleReads';
import { Button, inputClass, RemoteGate } from './ui';
import { ALERTS_URL, isProblemKind, problemVars } from './ProblemsNow';

// Monitoring stages 2 and 3 (alpha.127): the alerts with their one settings
// block, the services, and the website checks. Every list is the server's
// stored reading (cmd/panel/monitoring_alerts.go, monitoring_services.go,
// monitoring_uptime.go): a page load never waits for systemd or a site.
// Each list reads again every minute while the tab is visible, so nothing
// needs a reload; nothing here starts a change except the settings' Save and
// "Mark all as seen", whose buttons are busy until the answer is in.
//
// İzleme 2. ve 3. aşama: uyarılar ve tek ayar bloğu, hizmetler, site
// denetimleri. Her liste sunucunun saklı okumasıdır; sayfa systemd'yi ya da
// siteyi beklemez. Görünür sekmede her dakika yeniden okunur.

const REFRESH_MS = 60_000;

// One JSON change, same origin, never cached (the page's only two changes).
function sendJSON(url: string, body: unknown, method = 'POST'): Promise<Response> {
    return fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), cache: 'no-store', credentials: 'same-origin' });
}

/** The I18nProvider's words with this page's own copy in front (route-scoped, no boot cost). */
export function useMonitoringWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => monitoringAlertsTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}
const SERVICES_URL = '/api/v1/metrics/services';
const SITES_URL = '/api/v1/metrics/sites';

const timers = { setInterval: (callback: () => void, ms: number) => setInterval(callback, ms) as unknown as number, clearInterval: (id: number) => clearInterval(id) };

function useMinuteReads<T>(handle: RemoteHandle<T>) {
    const reread = handle.retry;
    useEffect(() => startVisibleReads(reread, REFRESH_MS, timers), [reread]);
}

const MAIL_STATES = ['none', 'pending', 'sent', 'suppressed', 'not_confirmed', 'not_available', 'failed'] as const;

export interface AlertItem {
    id: number;
    kind: string;
    severity: 'warning' | 'critical';
    subject?: string;
    name?: string;
    value: number;
    state: 'open' | 'resolved';
    opened_at: string;
    resolved_at?: string;
    mail_state: (typeof MAIL_STATES)[number];
    mail_detail?: string;
    seen: boolean;
}

export interface AlertsReport {
    settings: { delay_minutes: number; email: boolean; site_checks: boolean };
    delay_choices: number[];
    email: { confirmed: number; own_set: boolean; own_verified: boolean; mail: { available: boolean; reason?: string } };
    alerts: AlertItem[];
    unseen: number;
}

export function decodeAlerts(raw: unknown): AlertsReport {
    const body = raw as Partial<AlertsReport> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.alerts) || !body.settings || !body.email) throw new Error('shape');
    const alerts = body.alerts.filter((a): a is AlertItem => (
        !!a && typeof a === 'object' && typeof a.id === 'number' && isProblemKind(a.kind)
        && (a.state === 'open' || a.state === 'resolved') && typeof a.value === 'number' && MAIL_STATES.includes(a.mail_state)
    ));
    const choices = Array.isArray(body.delay_choices) ? body.delay_choices.filter((n): n is number => typeof n === 'number') : [];
    return {
        settings: body.settings,
        delay_choices: choices.length > 0 ? choices : [body.settings.delay_minutes],
        email: body.email,
        alerts,
        unseen: alerts.filter((a) => !a.seen).length,
    };
}

function useClock() {
    const { locale } = useI18n();
    return (iso?: string) => {
        if (!iso) return '';
        const at = new Date(iso);
        if (Number.isNaN(at.getTime())) return '';
        const sameDay = at.toDateString() === new Date().toDateString();
        return at.toLocaleString(locale, sameDay ? { hour: '2-digit', minute: '2-digit' } : { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
    };
}

export function formatBytes(n: number): string {
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = Math.max(n, 0);
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

// --- Alerts -------------------------------------------------------------------

export function AlertsSection() {
    const { t } = useMonitoringWords();
    const clock = useClock();
    const handle = useRemote(ALERTS_URL, decodeAlerts);
    useMinuteReads(handle);
    const [marking, setMarking] = useState(false);
    const [markNote, setMarkNote] = useState('');

    const markSeen = async () => {
        setMarking(true);
        setMarkNote('');
        try {
            const res = await sendJSON(`${ALERTS_URL}/seen`, {});
            if (!res.ok) setMarkNote(apiErrorText(await readApiError(res), t));
        } catch {
            // Not known whether it arrived: said, and the list is read again.
            setMarkNote(t('malert.seenUnknown'));
        } finally {
            setMarking(false);
            void handle.retry();
        }
    };

    return (
        <section className="mb-6" aria-labelledby="alerts-title" data-alerts="">
            <RemoteGate
                remote={handle.remote}
                checking={t('malert.checking')}
                failed={t('malert.readFailed')}
                onRetry={() => void handle.retry()}
                busy={handle.reading}
                className="py-3"
            >
                {({ value }) => (
                    <>
                        <div className="mb-2 flex flex-wrap items-center gap-2">
                            <h2 id="alerts-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                                <Bell className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                                {t('malert.title')}
                            </h2>
                            {value.unseen > 0 && (
                                <span className="rounded-full bg-warning/15 px-2 py-0.5 text-xs font-semibold text-warning" data-alerts-unseen={value.unseen}>
                                    {t('malert.newCount', { n: value.unseen })}
                                </span>
                            )}
                            {value.unseen > 0 && (
                                <Button className="ml-auto" loading={marking} disabled={marking} onClick={() => void markSeen()}>
                                    {t('malert.markSeen')}
                                </Button>
                            )}
                        </div>
                        {value.alerts.length === 0 ? (
                            <p className="flex items-center gap-2 text-sm text-fg-muted" data-alerts-none="">
                                <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                                {t('malert.none', { n: value.settings.delay_minutes })}
                            </p>
                        ) : (
                            <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
                                {value.alerts.map((alert) => {
                                    const open = alert.state === 'open';
                                    const Icon = !open ? CheckCircle2 : alert.severity === 'critical' ? XCircle : AlertTriangle;
                                    const tone = !open ? 'text-fg-subtle' : alert.severity === 'critical' ? 'text-danger' : 'text-warning';
                                    return (
                                        <li key={alert.id} className="flex gap-3 p-3" data-alert={alert.kind} data-alert-state={alert.state}>
                                            <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${tone}`} aria-hidden="true" />
                                            <div className="min-w-0 flex-1 text-sm">
                                                <p className={open ? 'font-medium text-fg' : 'text-fg-muted'}>
                                                    {t(`pnow.${alert.kind}.title` as TranslationKey, problemVars(alert, t))}
                                                    {!alert.seen && <span className="ml-2 text-xs font-semibold text-warning">•</span>}
                                                </p>
                                                <p className="text-xs text-fg-subtle">
                                                    {open
                                                        ? t('malert.ongoing', { time: clock(alert.opened_at) })
                                                        : t('malert.ended', { start: clock(alert.opened_at), end: clock(alert.resolved_at) })}
                                                    {' · '}
                                                    {t(`malert.mail.${alert.mail_state}` as TranslationKey, { detail: alert.mail_detail ?? '' })}
                                                </p>
                                            </div>
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                        {markNote && <p role="status" className="mt-2 text-sm text-fg-muted">{markNote}</p>}
                        <p className="mt-2 text-xs leading-5 text-fg-subtle" data-alerts-panel-down="">{t('malert.panelDown')}</p>
                        <AlertSettings report={value} onSaved={() => void handle.retry()} />
                    </>
                )}
            </RemoteGate>
        </section>
    );
}

function AlertSettings({ report, onSaved }: { report: AlertsReport; onSaved: () => void }) {
    const { t } = useMonitoringWords();
    const [delay, setDelay] = useState(report.settings.delay_minutes);
    const [email, setEmail] = useState(report.settings.email);
    const [siteChecks, setSiteChecks] = useState(report.settings.site_checks);
    const [saving, setSaving] = useState(false);
    const [note, setNote] = useState<{ tone: 'ok' | 'problem'; text: string } | null>(null);
    const stored = report.settings;
    const changed = delay !== stored.delay_minutes || email !== stored.email || siteChecks !== stored.site_checks;
    // What the server holds is shown again whenever it changes (another
    // administrator, or this Save), so the form never shows stale values.
    useEffect(() => {
        setDelay(stored.delay_minutes);
        setEmail(stored.email);
        setSiteChecks(stored.site_checks);
    }, [stored.delay_minutes, stored.email, stored.site_checks]);

    const save = async () => {
        setSaving(true);
        setNote(null);
        try {
            const res = await sendJSON(`${ALERTS_URL}/settings`, { delay_minutes: delay, email, site_checks: siteChecks }, 'PUT');
            if (res.ok) setNote({ tone: 'ok', text: t('malert.saved') });
            else setNote({ tone: 'problem', text: apiErrorText(await readApiError(res), t) });
        } catch {
            setNote({ tone: 'problem', text: t('malert.saveUnknown') });
        } finally {
            setSaving(false);
            onSaved();
        }
    };

    const mail = report.email;
    return (
        <details className="mt-3 rounded-xl border border-border bg-surface p-4" data-alert-settings="">
            <summary className="cursor-pointer text-sm font-semibold text-fg max-sm:min-h-11">{t('malert.settings')}</summary>
            <div className="mt-3 space-y-3 text-sm">
                <label className="flex flex-wrap items-center gap-2">
                    <span className="text-fg">{t('malert.delay')}</span>
                    <select className={`${inputClass} w-auto`} value={delay} onChange={(e) => setDelay(Number(e.target.value))} data-alert-delay="">
                        {report.delay_choices.map((n) => <option key={n} value={n}>{t('malert.minutes', { n })}</option>)}
                    </select>
                </label>
                <label className="flex cursor-pointer items-start gap-3">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={email} onChange={(e) => setEmail(e.target.checked)} data-alert-email="" />
                    <span className="text-fg">{t('malert.email')}</span>
                </label>
                {email && (
                    <div className="ml-7 space-y-2 text-fg-muted" data-alert-email-state="">
                        {!mail.mail.available ? (
                            <p>{t('malert.mailUnavailable')}</p>
                        ) : mail.confirmed > 0 ? (
                            <p>{t('malert.emailTo', { n: mail.confirmed })}</p>
                        ) : (
                            <p>{t('malert.emailNone')}</p>
                        )}
                        {!mail.own_set ? (
                            <p>
                                {t('malert.emailNoAddress')}{' '}
                                <Link to="/account#email" className="font-semibold text-primary">{t('malert.openAccount')}</Link>
                            </p>
                        ) : !mail.own_verified && mail.mail.available ? (
                            <p data-alert-own-unconfirmed="">
                                {t('malert.emailUnconfirmed')}{' '}
                                <Link to="/account#email" className="font-semibold text-primary">{t('malert.openAccount')}</Link>
                            </p>
                        ) : null}
                    </div>
                )}
                <label className="flex cursor-pointer items-start gap-3">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={siteChecks} onChange={(e) => setSiteChecks(e.target.checked)} data-alert-sites="" />
                    <span className="text-fg">{t('malert.siteChecks')}</span>
                </label>
                <div className="flex flex-wrap items-center gap-3">
                    <Button variant="primary" loading={saving} disabled={saving || !changed} onClick={() => void save()} data-alert-save="">
                        {t('malert.save')}
                    </Button>
                    {note && <p role="status" className={note.tone === 'ok' ? 'text-fg-muted' : 'text-danger'}>{note.text}</p>}
                </div>
            </div>
        </details>
    );
}

// --- Services -----------------------------------------------------------------

interface ServiceRow {
    id: string;
    name: string;
    unit: string;
    active_state?: string;
    sub_state?: string;
    active_since?: string;
    restarts_last_hour: number;
    memory_bytes?: number;
    cpu?: number;
    /** alpha.130: the server's own rule (a oneshot "active (exited)" is up). */
    running?: boolean;
}

interface ServicesReport {
    state: 'known' | 'checking' | 'unreadable' | 'not_scanned';
    services: ServiceRow[];
}

export function decodeServices(raw: unknown): ServicesReport {
    const body = raw as Partial<ServicesReport> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.services)
        || !['known', 'checking', 'unreadable', 'not_scanned'].includes(body.state as string)) throw new Error('shape');
    const services = body.services.filter((s): s is ServiceRow => !!s && typeof s.id === 'string' && typeof s.unit === 'string');
    return { state: body.state as ServicesReport['state'], services };
}

export function ServicesSection() {
    const { t } = useMonitoringWords();
    const clock = useClock();
    const handle = useRemote(SERVICES_URL, decodeServices);
    useMinuteReads(handle);
    return (
        <section className="mt-6" aria-labelledby="services-title" data-services="">
            <h2 id="services-title" className="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
                <Server className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                {t('msvc.title')}
            </h2>
            <RemoteGate remote={handle.remote} checking={t('msvc.checking')} failed={t('msvc.unreadable')} onRetry={() => void handle.retry()} busy={handle.reading} className="py-3">
                {({ value }) => {
                    if (value.state === 'not_scanned') {
                        return (
                            <p className="text-sm text-fg-muted" data-services-state="not_scanned">
                                {t('msvc.notScanned')}{' '}
                                <Link to="/components" className="font-semibold text-primary">{t('msvc.openComponents')}</Link>
                            </p>
                        );
                    }
                    const known = value.state === 'known';
                    return (
                        <>
                            {!known && (
                                <p className="mb-2 text-xs text-fg-subtle" data-services-state={value.state}>
                                    {t(value.state === 'checking' ? 'msvc.checking' : 'msvc.unreadable')}
                                </p>
                            )}
                            <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
                                {value.services.map((service) => {
                                    const running = typeof service.running === 'boolean' ? service.running : service.active_state === 'active';
                                    const stateText = !known || !service.active_state
                                        ? ''
                                        : running
                                            ? (service.active_since ? t('msvc.runningSince', { time: clock(service.active_since) }) : t('msvc.running'))
                                            : t('msvc.notRunning', { state: service.sub_state ? `${service.active_state}, ${service.sub_state}` : service.active_state });
                                    const facts = [
                                        stateText,
                                        service.restarts_last_hour > 0 ? t('msvc.restarts', { n: service.restarts_last_hour }) : '',
                                        typeof service.cpu === 'number' ? t('msvc.cpu', { value: t('monitoring.pct', { n: service.cpu.toFixed(1) }) }) : '',
                                        typeof service.memory_bytes === 'number' ? t('msvc.memory', { value: formatBytes(service.memory_bytes) }) : '',
                                    ].filter(Boolean);
                                    return (
                                        <li key={service.unit} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-3 py-2 text-sm" data-service={service.id} data-service-running={known ? String(running) : 'unknown'}>
                                            <span className={`h-2 w-2 shrink-0 self-center rounded-full ${!known ? 'bg-fg-subtle' : running ? 'bg-success' : 'bg-danger'}`} aria-hidden="true" />
                                            <Link to={`/components/${encodeURIComponent(service.id)}`} className="font-medium text-fg hover:text-primary">
                                                {componentName({ id: service.id, name: service.name }, t)}
                                            </Link>
                                            <span className={`text-xs ${known && !running ? 'text-danger' : 'text-fg-subtle'}`}>{facts.join(' · ')}</span>
                                        </li>
                                    );
                                })}
                            </ul>
                        </>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

// --- Websites -----------------------------------------------------------------

interface SiteRow {
    domain: string;
    checked_at: string;
    state: 'up' | 'down';
    scheme: 'http' | 'https';
    http_status?: number;
    response_ms?: number;
    reason?: string;
    cert_days?: number;
    cert_valid?: boolean;
    down_since?: string;
    stale?: boolean;
}

interface SitesReport {
    enabled: boolean;
    sites: SiteRow[];
}

const SITE_REASONS = ['timeout', 'refused', 'tls', 'closed', 'http_5xx', 'error'];

export function decodeSites(raw: unknown): SitesReport {
    const body = raw as Partial<SitesReport> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.sites) || typeof body.enabled !== 'boolean') throw new Error('shape');
    const sites = body.sites.filter((s): s is SiteRow => !!s && typeof s.domain === 'string' && (s.state === 'up' || s.state === 'down'));
    return { enabled: body.enabled, sites };
}

export function SitesSection() {
    const { t } = useMonitoringWords();
    const clock = useClock();
    const handle = useRemote(SITES_URL, decodeSites);
    useMinuteReads(handle);
    return (
        <section className="mt-6" aria-labelledby="sites-title" data-sites="">
            <h2 id="sites-title" className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg">
                <Globe className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                {t('msite.title')}
            </h2>
            <p className="mb-2 text-xs leading-5 text-fg-subtle">{t('msite.hint')}</p>
            <RemoteGate remote={handle.remote} checking={t('malert.checking')} failed={t('msite.readFailed')} onRetry={() => void handle.retry()} busy={handle.reading} className="py-3">
                {({ value }) => {
                    if (!value.enabled) return <p className="text-sm text-fg-muted" data-sites-state="off">{t('msite.off')}</p>;
                    if (value.sites.length === 0) return <p className="text-sm text-fg-muted" data-sites-state="waiting">{t('msite.waiting')}</p>;
                    return (
                        <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
                            {value.sites.map((site) => {
                                const up = site.state === 'up';
                                const reason = SITE_REASONS.includes(site.reason ?? '') ? t(`msite.reason.${site.reason}` as TranslationKey) : t('msite.reason.error');
                                const cert = site.scheme === 'http'
                                    ? t('msite.noCert')
                                    : site.cert_valid === false
                                        ? (typeof site.cert_days === 'number' && site.cert_days < 0 ? t('msite.certExpired') : t('msite.certInvalid'))
                                        : typeof site.cert_days === 'number' ? t('msite.certDays', { days: site.cert_days }) : '';
                                const facts = [
                                    up
                                        ? t('msite.up', { status: site.http_status ?? '', ms: site.response_ms ?? '' })
                                        : t('msite.down', { time: clock(site.down_since), reason }),
                                    up ? cert : '',
                                    site.stale ? t('msite.stale', { time: clock(site.checked_at) }) : '',
                                ].filter(Boolean);
                                const certTrouble = up && site.scheme === 'https' && (site.cert_valid === false || (typeof site.cert_days === 'number' && site.cert_days < 14));
                                return (
                                    <li key={site.domain} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-3 py-2 text-sm" data-site={site.domain} data-site-state={site.state}>
                                        <span className={`h-2 w-2 shrink-0 self-center rounded-full ${up ? (certTrouble ? 'bg-warning' : 'bg-success') : 'bg-danger'}`} aria-hidden="true" />
                                        <Link to={`/domains/${encodeURIComponent(site.domain)}`} className="font-medium text-fg hover:text-primary">{site.domain}</Link>
                                        <span className={`text-xs ${up ? 'text-fg-subtle' : 'text-danger'}`}>{facts.join(' · ')}</span>
                                    </li>
                                );
                            })}
                        </ul>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

// --- Busiest processes at a spike ---------------------------------------------

export interface SpikeSnapshot {
    ts: string;
    reason: 'cpu' | 'memory';
    value: number;
    processes: { pid: number; name: string; user?: string; cpu: number; rss: number }[];
}

export function decodeSnapshots(raw: unknown): SpikeSnapshot[] {
    if (!Array.isArray(raw)) return [];
    return raw.filter((s): s is SpikeSnapshot => (
        !!s && typeof s.ts === 'string' && (s.reason === 'cpu' || s.reason === 'memory') && typeof s.value === 'number' && Array.isArray(s.processes)
    ));
}

/** The busiest processes of the highest spike of one kind in the range. */
export function SpikeNote({ snapshots, reason }: { snapshots: SpikeSnapshot[]; reason: 'cpu' | 'memory' }) {
    const { t } = useMonitoringWords();
    const clock = useClock();
    const mine = snapshots.filter((s) => s.reason === reason);
    if (mine.length === 0) return null;
    const top = mine.reduce((a, b) => (b.value > a.value ? b : a));
    const pct = (n: number) => t('monitoring.pct', { n: n.toFixed(0) });
    return (
        <div className="mt-2 rounded-lg bg-surface-2/60 p-2 text-xs text-fg-muted" data-spike={reason}>
            <p className="font-medium text-fg">{t('mmach.busiest', { time: clock(top.ts), value: pct(top.value) })}</p>
            <ol className="mt-1 space-y-0.5">
                {top.processes.map((p) => (
                    <li key={p.pid} className="font-mono">
                        {p.name}{p.user ? ` (${p.user})` : ''} · {reason === 'cpu' ? pct(p.cpu) : formatBytes(p.rss)}
                    </li>
                ))}
            </ol>
            <p className="mt-1 text-fg-subtle">{t('mmach.snapshotHint')}</p>
        </div>
    );
}

/** Where each snapshot sits along a chart that spans the samples (0..1). */
export function spikeMarks(snapshots: SpikeSnapshot[], reason: 'cpu' | 'memory', first?: string, last?: string): number[] {
    const start = first ? Date.parse(first) : NaN;
    const end = last ? Date.parse(last) : NaN;
    if (!(end > start)) return [];
    return snapshots
        .filter((s) => s.reason === reason)
        .map((s) => (Date.parse(s.ts) - start) / (end - start))
        .filter((x) => x >= 0 && x <= 1);
}
