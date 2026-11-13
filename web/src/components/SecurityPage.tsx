import { Suspense, lazy, useCallback, useEffect, useRef, useState } from 'react';
import { startVisibleReads } from '../lib/visibleReads';
import { ShieldAlert, ShieldCheck, ShieldOff, ShieldQuestion } from 'lucide-react';
import { useI18n } from '../i18n';
import { Link, useLocation } from '../router';
import { PageHeader } from './PageHeader';
import { RemoteGate } from './ui';
import { FirewallBar } from './FirewallPanel';
import { SecurityAuditCard } from './SecurityAuditCard';
import { SecurityUpdatesCard } from './SecurityUpdatesCard';
// alpha.128 (G21): the administrator's password level, a chunk of its own.
const PasswordLevelCard = lazy(() => import('./ServerSettingLimits').then((module) => ({ default: module.PasswordLevelCard })));
import { fetchHostMutationReadiness, type HostMutationReadiness } from '../lib/panelUpdateAdmission';
import { useManagedServices } from '../lib/managedServices';
import { useRemote } from '../lib/remote';
import { EMERGENCY_COMPONENTS, SECURITY_EMERGENCIES_URL, decodeEmergencyList, emergencyPath, openEmergenciesFor } from '../lib/securityEmergency';
import { isServiceRunning } from '../lib/serviceStatus';

// alpha.128 ops (G4): panel access and every account's sessions, in their own chunk.
const PanelAccessCard = lazy(() => import('./PanelSessions').then((module) => ({ default: module.PanelAccessCard })));
const AllSessionsCard = lazy(() => import('./PanelSessions').then((module) => ({ default: module.AllSessionsCard })));
// alpha.130: SSH password sign-in, with its own words, in its own chunk.
const SSHPasswordLoginCard = lazy(() => import('./SSHPasswordLoginCard').then((module) => ({ default: module.SSHPasswordLoginCard })));
// alpha.133: Open to the outside (connections to customers' databases), in its own chunk.
const OutsideConnectionsCard = lazy(() => import('./DatabaseRemoteServerPanel').then((module) => ({ default: module.DatabaseRemoteServerPanel })));

// Server → Security (D-037, owner 2026-10-22): one page that answers "is my
// server safe?". Nothing here is new: the firewall box that used to open from
// the Components page's firewall line, with its settings; Fail2ban's state as
// the stored component record says it (managed on its own component page);
// and the read-only audit of exposed services that was Settings → Server
// security. Each keeps its own reads, refusals and texts.
//
// Sunucu → Güvenlik: "sunucum güvende mi?" sorusunu yanıtlayan tek sayfa.
// Burada yeni bir şey yok: Bileşenler sayfasındaki güvenlik duvarı satırından
// açılan kutu ve ayarları; kayıtlı bileşen kaydının söylediği Fail2ban durumu
// (yönetimi kendi bileşen sayfasında); Ayarlar → Sunucu güvenliği olan salt
// okunur dışa açık servis denetimi. Her biri kendi okumasını ve metnini korur.
const READINESS_POLL_MS = 5000;

export function SecurityPage() {
    const { t } = useI18n();
    const location = useLocation();
    const { readiness, reread } = useHostMutationReadiness();
    const readinessMessage = readiness === null
        ? t('services.mutationReadiness.checking')
        : readiness.ready
            ? ''
            : t(`services.mutationReadiness.${readiness.reason}` as Parameters<typeof t>[0]);

    // /security#firewall is where the Components page's firewall line leads.
    // /security#firewall, Bileşenler sayfasındaki güvenlik duvarı satırının gittiği yerdir.
    useEffect(() => {
        if (location.hash !== '#firewall') return;
        document.getElementById('firewall')?.scrollIntoView?.({ block: 'start' });
    }, [location.hash]);

    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.security')} subtitle={t('securityPage.subtitle')} breadcrumb={[t('common.home'), t('nav.security')]} />
            <div className="grid max-w-5xl gap-5">
                <div id="firewall" className="scroll-mt-24" data-security-section="firewall">
                    {readiness?.ready === false && (
                        <section role="status" className="mb-4 rounded-xl border border-warning-mark/60 bg-warning-mark/20 px-4 py-3 text-sm text-fg">
                            <span className="font-semibold">{t('services.mutationReadiness.title')}</span>{' '}
                            {readinessMessage}{' '}
                            {t('services.mutationReadiness.autoResume')}
                        </section>
                    )}
                    <FirewallBar
                        place="security"
                        readiness={readiness}
                        readinessMessage={readinessMessage}
                        onRefreshReadiness={reread}
                        refreshKey={0}
                    />
                </div>
                {/* alpha.107 (D-038 §4): the operating system's own security updates. */}
                <SecurityUpdatesCard />
                <Suspense fallback={null}><SSHPasswordLoginCard /></Suspense>
                <Suspense fallback={null}><PanelAccessCard /></Suspense>
                <Suspense fallback={null}><AllSessionsCard /></Suspense>
                <Fail2banStatus />
                <Suspense fallback={null}><OutsideConnectionsCard /></Suspense>
                <EmergencyArea />
                <div data-security-section="audit">
                    <SecurityAuditCard />
                </div>
                <Suspense fallback={null}><PasswordLevelCard /></Suspense>
            </div>
        </div>
    );
}

// The page reads the server's readiness for a change the way the Components
// page does: on arrival, then every 5 seconds while it is open, and once more
// after each firewall change. A read only; it never starts anything.
// Sayfa, değişiklik hazırlığını Bileşenler sayfası gibi okur: açılışta, açıkken
// 5 saniyede bir ve her güvenlik duvarı değişikliğinden sonra. Yalnız okumadır.
function useHostMutationReadiness(): { readiness: HostMutationReadiness | null; reread: () => Promise<boolean> } {
    const [readiness, setReadiness] = useState<HostMutationReadiness | null>(null);
    const alive = useRef(true);
    const reread = useCallback(async () => {
        const value = await fetchHostMutationReadiness();
        if (alive.current) setReadiness(value);
        return value.ready;
    }, []);
    useEffect(() => {
        alive.current = true;
        const stop = startVisibleReads(reread, READINESS_POLL_MS, { setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number, clearInterval: (id) => clearInterval(id) }, { immediate: true });
        return () => {
            alive.current = false;
            stop();
        };
    }, [reread]);
    return { readiness, reread };
}

// Fail2ban's state, as the stored component record says it. The record is
// read, never probed; a record nobody took is "not checked yet", not "not
// installed". Management (jails, bans, starting it) stays on its component
// page, where this card leads.
// Fail2ban durumu, kayıtlı bileşen kaydının söylediği gibi. Kayıt okunur,
// makine yoklanmaz; kimsenin almadığı kayıt "kurulu değil" değil "henüz
// bakılmadı"dır. Yönetim kendi bileşen sayfasında kalır.
type Fail2banState = 'running' | 'installedUnknown' | 'notRunning' | 'notInstalled' | 'notChecked';

// A known snapshot without a Fail2ban record is a known answer: this server's
// catalogue has no Fail2ban, so it is not installed. "Not checked yet" is only
// a record whose installed state nobody observed.
// Fail2ban kaydı olmayan bilinen anlık görüntü bilinen bir yanıttır: kurulu değil.
export function fail2banState(services: Record<string, unknown>[]): Fail2banState {
    const record = services.find((service) => service.id === 'fail2ban');
    if (!record) return 'notInstalled';
    if (record.is_installed === null || record.is_installed === undefined) return 'notChecked';
    if (record.is_installed === false) return 'notInstalled';
    // One running rule with the Dashboard and Components (lib/serviceStatus).
    // Pano ve Bileşenler ile tek çalışma kuralı.
    if (isServiceRunning(record.status)) return 'running';
    const status = typeof record.status === 'string' ? record.status.trim().toLowerCase() : '';
    if (status === 'unknown' || status === '') return 'installedUnknown';
    return 'notRunning';
}

function Fail2banStatus() {
    const { t } = useI18n();
    const services = useManagedServices();
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-security-section="fail2ban">
            <h2 className="text-base font-semibold text-fg">Fail2ban</h2>
            <p className="mb-3 mt-0.5 text-sm text-fg-muted">{t('securityPage.f2b.desc')}</p>
            <RemoteGate
                remote={services.remote}
                checking={t('securityPage.f2b.checking')}
                failed={t('securityPage.f2b.unknown')}
                onRetry={() => void services.retry()}
                busy={services.reading}
            >
                {({ value }) => {
                    const state = fail2banState(value.services);
                    const Icon = state === 'running' ? ShieldCheck
                        : state === 'notChecked' || state === 'installedUnknown' ? ShieldQuestion
                            : state === 'notInstalled' ? ShieldOff : ShieldAlert;
                    const tone = state === 'running' ? 'text-success'
                        : state === 'notRunning' ? 'text-warning' : 'text-fg-subtle';
                    const installed = state === 'running' || state === 'installedUnknown' || state === 'notRunning';
                    return (
                        <div className="flex flex-wrap items-start gap-x-3 gap-y-2" data-fail2ban-state={state}>
                            <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${tone}`} aria-hidden="true" />
                            <p className="min-w-0 flex-1 basis-64 text-sm text-fg">{t(`securityPage.f2b.${state}` as Parameters<typeof t>[0])}</p>
                            {/* alpha.122: not installed opens Fail2ban's own install dialog in
                                Components; nothing starts until it is confirmed there.
                                Kurulu değilse Fail2ban'ın kurulum penceresini açar. */}
                            <Link
                                to={installed ? '/components/fail2ban' : state === 'notInstalled' ? '/components?component=fail2ban&install=' : '/components?component=fail2ban'}
                                className="inline-flex items-center text-sm font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 max-sm:min-h-11"
                                data-install-link={state === 'notInstalled' ? 'fail2ban' : undefined}
                            >
                                {installed ? t('securityPage.f2b.manage') : state === 'notInstalled' ? t('services.installTitle', { name: t('services.component.name.fail2ban') }) : t('securityPage.f2b.openComponents')}
                            </Link>
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

// alpha.120 (D-043): one place that says what the security emergency is and
// why only these four have it, with a link to each one's steps on its own
// page. An open emergency is named; reading never starts anything.
// Güvenlik acil durumu: ne olduğu, neden yalnız bu dördünde olduğu ve her
// birinin kendi sayfasındaki adımlarına bağlantı. Okumak hiçbir şey başlatmaz.
function EmergencyArea() {
    const { t } = useI18n();
    const list = useRemote(SECURITY_EMERGENCIES_URL, decodeEmergencyList);
    const known = list.remote.state === 'known' ? list.remote.value : null;
    const anyOpen = known !== null && EMERGENCY_COMPONENTS.some((entry) => openEmergenciesFor(known, entry.component).length > 0);
    return (
        <section id="emergency" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-security-section="emergency">
            <h2 className="text-base font-semibold text-fg">{t('emg.area.title')}</h2>
            <p className="mt-0.5 max-w-[75ch] text-sm text-fg-muted">{t('emg.area.intro')}</p>
            {known !== null && !anyOpen && <p className="mt-2 text-sm text-fg">{t('emg.area.none')}</p>}
            {list.remote.state === 'unknown' && <p className="mt-2 text-sm text-warning">{t('emg.area.unknown')}</p>}
            <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-2">
                {EMERGENCY_COMPONENTS.map((entry) => {
                    const open = known !== null && openEmergenciesFor(known, entry.component).length > 0;
                    return (
                        <li key={entry.component} className="flex flex-wrap items-center gap-2" data-emergency-link={entry.serviceId}>
                            <Link
                                to={emergencyPath(entry.serviceId)}
                                className="inline-flex items-center text-sm font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 max-sm:min-h-11"
                            >
                                {t('emg.area.go', { name: entry.name })}
                            </Link>
                            {open && <span className="rounded bg-danger/10 px-2 py-0.5 text-xs font-semibold text-danger" data-emergency-open={entry.serviceId}>{t('emg.area.openOne')}</span>}
                        </li>
                    );
                })}
            </ul>
        </section>
    );
}
