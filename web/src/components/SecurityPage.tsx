import { useCallback, useEffect, useRef, useState } from 'react';
import { ShieldAlert, ShieldCheck, ShieldOff, ShieldQuestion } from 'lucide-react';
import { useI18n } from '../i18n';
import { Link, useLocation } from '../router';
import { PageHeader } from './PageHeader';
import { RemoteGate } from './ui';
import { FirewallBar } from './ServiceList';
import { SecurityAuditCard } from './SecurityAuditCard';
import { fetchHostMutationReadiness, type HostMutationReadiness } from '../lib/panelUpdateAdmission';
import { useManagedServices } from '../lib/managedServices';

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
                <Fail2banStatus />
                <div data-security-section="audit">
                    <SecurityAuditCard />
                </div>
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
        void reread();
        const timer = window.setInterval(() => void reread(), READINESS_POLL_MS);
        return () => {
            alive.current = false;
            window.clearInterval(timer);
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

export function fail2banState(services: Record<string, unknown>[]): Fail2banState {
    const record = services.find((service) => service.id === 'fail2ban');
    if (!record || record.is_installed === null || record.is_installed === undefined) return 'notChecked';
    if (record.is_installed === false) return 'notInstalled';
    if (record.status === 'running') return 'running';
    if (record.status === 'unknown' || record.status === '') return 'installedUnknown';
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
                        : state === 'notRunning' || state === 'notInstalled' ? 'text-warning' : 'text-fg-subtle';
                    const installed = state === 'running' || state === 'installedUnknown' || state === 'notRunning';
                    return (
                        <div className="flex flex-wrap items-start gap-x-3 gap-y-2" data-fail2ban-state={state}>
                            <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${tone}`} aria-hidden="true" />
                            <p className="min-w-0 flex-1 basis-64 text-sm text-fg">{t(`securityPage.f2b.${state}` as Parameters<typeof t>[0])}</p>
                            <Link
                                to={installed ? '/services/fail2ban' : '/services'}
                                className="inline-flex items-center text-sm font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 max-sm:min-h-11"
                            >
                                {installed ? t('securityPage.f2b.manage') : t('securityPage.f2b.openComponents')}
                            </Link>
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
