import { BrandMark } from './BrandMark';
import { BuildCommit } from './BuildCommit';
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { LogOut, ChevronDown, Menu, KeyRound, Mail, UserCheck, DownloadCloud } from 'lucide-react';
import { api } from '../lib/api';
import { Link } from '../router';
import { availablePanelUpdate, usePanelUpdateDiscovery, wholeVersion, type DiscoveredTarget } from '../lib/panelUpdateDiscovery';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { navItemsForRole, navGroups, type NavAccessContext, type NavItem } from '../nav';
import { publishComponentCensus, useComponentCensus } from '../lib/componentCensus';
import { decodeList, lastKnown, useRemote } from '../lib/remote';
import { INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones, domainsCount } from '../lib/infrastructureZones';
import { ThemeSwitcher } from './ThemeSwitcher';
import { SkinSwitcher } from './SkinSwitcher';
import { LanguageSwitcher } from './LanguageSwitcher';
import { DesktopPageHeaderTargetContext } from './pageHeaderSlot';
import type { TranslationKey } from '../i18n/en';

// The password dialog opens from My account since v0.1.0-alpha.98 (D-037);
// the menu opens it only for a role without that page. It is fetched when it
// is opened, not with every page load.
// Parola penceresi alpha.98'den beri Hesabım'dan açılır; menü yalnız o sayfası
// olmayan rol için açar. Açıldığında getirilir, her sayfa yüklemesinde değil.
const ChangePasswordModal = lazy(() => import('./ChangePasswordModal').then((module) => ({ default: module.ChangePasswordModal })));
// v0.1.0-alpha.99: an additional user changes their own email where they change
// their password. Its words are screen copy, so the item is offered only once
// that copy has arrived.
const ChangeEmailModal = lazy(() => import('./ChangeEmailModal').then((module) => ({ default: module.ChangeEmailModal })));

// The single inherited shell: a dark navigation rail (grouped, with live
// counts) beside a light content column with a top bar. Feature pages
// render in the outlet unchanged.
//
// Tek kalıtımsal kabuk: koyu bir navigasyon rayı (gruplu, canlı sayılarla)
// ve yanında üst çubuklu açık bir içerik sütunu. Özellik sayfaları içerik
// alanında değişmeden render edilir.
interface LayoutProps {
    children: React.ReactNode;
    currentPage?: string;
    onPageChange?: (id: string) => void;
    mode?: 'panel' | 'setup';
    setupNavigation?: React.ReactNode;
}

// null is a count this panel does not have, which is not the same as zero:
// a host nobody has scanned answers `is_installed: null` for every component,
// and a badge reading 0 there would report a census nobody took.
// null, panelin sahip OLMADIĞI bir sayıdır; sıfır demek değildir.
type Counts = Partial<Record<'domains' | 'databases' | 'services', number | null>>;
type PanelRuntime = {
    version: string;
    commit: string;
    agent_commit: string;
    agent_matches: boolean;
    hostname: string;
    ipv4: string;
};
type ServerIdentity = Pick<PanelRuntime, 'hostname' | 'ipv4'>;

function decodePanelRuntime(value: unknown): PanelRuntime | null {
    if (!value || typeof value !== 'object') return null;
    const raw = value as Record<string, unknown>;
    if (typeof raw.version !== 'string' || raw.version.length > 80
        || typeof raw.commit !== 'string' || raw.commit.length > 80
        || typeof raw.agent_commit !== 'string' || raw.agent_commit.length > 80
        || typeof raw.agent_matches !== 'boolean') {
        return null;
    }

    const hostname = typeof raw.hostname === 'string' && raw.hostname.length <= 253
        ? raw.hostname.trim()
        : '';
    const ipv4 = typeof raw.ipv4 === 'string' && raw.ipv4.length <= 64
        ? raw.ipv4.trim()
        : '';
    return {
        version: raw.version,
        commit: raw.commit,
        agent_commit: raw.agent_commit,
        agent_matches: raw.agent_matches,
        hostname,
        ipv4,
    };
}

// The rail can be collapsed to icons (alpha.108). The choice is remembered
// per browser; storage can be missing or throw (private window, blocked site
// data), so every access is guarded and the rail simply starts expanded.
// Ray, yalnız simgelere daraltılabilir; seçim tarayıcı başına hatırlanır.
const RAIL_STORAGE_KEY = 'celikpanel.sidebar.collapsed';
function readRailCollapsed(): boolean {
    try {
        return window.localStorage.getItem(RAIL_STORAGE_KEY) === '1';
    } catch {
        return false;
    }
}
function writeRailCollapsed(collapsed: boolean) {
    try {
        window.localStorage.setItem(RAIL_STORAGE_KEY, collapsed ? '1' : '0');
    } catch {
        /* the choice just is not remembered */
    }
}

export function Layout({ children, currentPage = '', onPageChange, mode = 'panel', setupNavigation }: LayoutProps) {
    const { t } = useI18n();
    const setupMode = mode === 'setup';
    const { role, user } = useAuth();
    const navAccess: NavAccessContext = {
        accountType: typeof user?.account_type === 'string' ? user.account_type : undefined,
        teamMembers: user?.features?.team_members === true,
    };
    // The domain badge reads the address the Domains page and a domain's own
    // page read, so the rail adds no request of its own and follows theirs: a
    // domain added or removed there moves the badge. It is a number only for
    // an answer the server gave. While that is read, and when it could not be
    // read, there is no badge; a refresh that failed keeps the earlier number
    // rather than dropping to none and back.
    // Alan adı rozeti, Alan Adları sayfasının ve bir alan adının kendi
    // sayfasının okuduğu adresi okur; ray kendi isteğini eklemez. Yalnız
    // sunucunun verdiği yanıt için sayıdır. Okunurken ve okunamadığında rozet
    // yoktur; başarısız yenileme önceki sayıyı korur.
    const domainList = useRemote(setupMode ? null : '/api/v1/domains', decodeList<unknown>);
    // The badge counts what the Domains page lists: the domains and, for an
    // administrator, the DNS-only infrastructure zone beside them, read from
    // the same address the Domains page and the Dashboard read (alpha.95
    // review). A number only when both answered; a failed refresh keeps the
    // earlier count only when both had one.
    // Rozet Alan adları sayfasının listelediğini sayar: alan adları ve
    // yöneticide yalnız-DNS altyapı bölgesi. İki okuma da yanıtladığında sayıdır.
    const zoneList = useRemote(setupMode || role !== 'admin' ? null : INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones);
    const domainCount = setupMode ? undefined : lastKnown(domainsCount(domainList.remote, role === 'admin' ? zoneList.remote : null))?.value;
    const [panelRuntime, setPanelRuntime] = useState<PanelRuntime | null>(null);
    const [mobileOpen, setMobileOpen] = useState(false);
    const [railCollapsed, setRailCollapsed] = useState(readRailCollapsed);
    const toggleRail = useCallback(() => {
        setRailCollapsed((current) => {
            writeRailCollapsed(!current);
            return !current;
        });
    }, []);
    const [desktopPageHeaderTarget, setDesktopPageHeaderTarget] = useState<HTMLDivElement | null>(null);
    const [desktopPageHeaderCount, setDesktopPageHeaderCount] = useState(0);
    const registerDesktopPageHeader = useCallback(() => {
        setDesktopPageHeaderCount((count) => count + 1);
        return () => setDesktopPageHeaderCount((count) => Math.max(0, count - 1));
    }, []);
    const desktopPageHeaderSlot = useMemo(
        () => ({ target: desktopPageHeaderTarget, register: registerDesktopPageHeader }),
        [desktopPageHeaderTarget, registerDesktopPageHeader],
    );
    const hasDesktopPageHeader = desktopPageHeaderCount > 0;

    // Live counts feed the sidebar badges. Failures are silent — a missing
    // badge is better than a broken shell.
    // Canlı sayılar kenar çubuğu rozetlerini besler. Hatalar sessizdir —
    // eksik bir rozet, bozuk bir kabuktan iyidir.
    useEffect(() => {
        if (setupMode) return;
        // Services are an admin-only view; only the admin sidebar shows the badge.
        // Servisler yalnızca yönetici görünümüdür; rozeti yalnızca yönetici çubuğu gösterir.
        if (role === 'admin') {
            // The badge answers "how many components are installed?".
            // Catalogue size belongs on the Components page and made a
            // fresh server look as though 26 services were already present.
            // A host this panel has never looked at cannot answer that
            // question at all — every row comes back null — so the badge
            // carries no number there. Where some rows are known and some
            // are not, the badge counts what is known: an unchecked row
            // joins neither side (R-040).
            // Bu panelin hiç bakmadığı makine bu soruyu yanıtlayamaz; rozet
            // orada sayı taşımaz. Bazı satırlar biliniyorsa rozet bilineni
            // sayar; bakılmamış satır hiçbir yakaya katılmaz.
            //
            // This read seeds the shared census; it does not own it. The
            // badge below renders whatever the last host-wide payload said,
            // whichever screen received it, so a check the operator runs on
            // the dashboard or on a component page moves the badge without a
            // reload — and without this shell polling or asking twice.
            // Bu okuma paylaşılan sayımı tohumlar, sahiplenmez. Rozet, hangi
            // ekran aldıysa alsın son sistem geneli yükün söylediğini çizer;
            // operatörün panoda ya da bileşen sayfasında çalıştırdığı kontrol
            // rozeti yeniden yükleme olmadan oynatır.
            api.getServices()
                .then(publishComponentCensus)
                .catch(() => {});
        }
    }, [role, setupMode]);

    // One number, read from where every screen publishes it. Non-admins never
    // see this item, so they never carry its count.
    // Tek sayı, her ekranın yayınladığı yerden okunur.
    const serviceCensus = useComponentCensus();
    const sidebarCounts = useMemo<Counts>(
        () => (role === 'admin' ? { domains: domainCount, services: serviceCensus } : { domains: domainCount }),
        [domainCount, role, serviceCensus],
    );

    // The existing admin-only version request now carries the bounded machine
    // identity too. One no-store response feeds both the build stamp and every
    // persistent shell label without sampling dashboard metrics again.
    useEffect(() => {
        if (role !== 'admin') {
            setPanelRuntime(null);
            return;
        }

        let cancelled = false;
        fetch('/api/v1/panel/version', { cache: 'no-store', credentials: 'same-origin' })
            .then(async (response) => (response.ok ? decodePanelRuntime(await response.json()) : null))
            .then((runtime) => {
                if (!cancelled) setPanelRuntime(runtime);
            })
            .catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [role]);

    // The Panel's stored update check (D-035): an administrator sees a small
    // link beside the version when a newer signed release was found. It is a
    // stored read with no network call; nobody else sees anything.
    // Yöneticiye, daha yeni imzalı sürüm bulunduysa sürüm satırının yanında
    // küçük bir bağlantı gösterilir; başka kimse bir şey görmez.
    const discovery = usePanelUpdateDiscovery(role === 'admin' && !setupMode);
    const updateTarget = role === 'admin' && !setupMode ? availablePanelUpdate(discovery.value) : null;

    const serverIdentity: ServerIdentity | null = panelRuntime
        && (panelRuntime.hostname || panelRuntime.ipv4)
        ? { hostname: panelRuntime.hostname, ipv4: panelRuntime.ipv4 }
        : null;

    return (
        <DesktopPageHeaderTargetContext.Provider value={desktopPageHeaderSlot}>
            <div className="flex h-screen bg-bg text-fg">
                <Sidebar
                    role={role}
                    access={navAccess}
                    counts={sidebarCounts}
                    currentPage={currentPage}
                    onPageChange={(id) => {
                        onPageChange?.(id);
                        setMobileOpen(false);
                    }}
                    mobileOpen={mobileOpen}
                    onCloseMobile={() => setMobileOpen(false)}
                    collapsed={railCollapsed}
                    onToggleCollapsed={toggleRail}
                    expandedHeader={hasDesktopPageHeader || setupMode}
                    setupMode={setupMode}
                    setupNavigation={setupNavigation}
                    serverIdentity={serverIdentity}
                    panelRuntime={panelRuntime}
                    updateTarget={updateTarget}
                />

                <div className="flex min-w-0 flex-1 flex-col">
                    <ImpersonationBanner />
                    <header
                        className={'flex h-14 shrink-0 items-center gap-3 border-b border-border bg-surface px-4 md:px-6 ' +
                            (hasDesktopPageHeader || setupMode ? 'xl:h-auto xl:min-h-[90px] xl:py-2' : '')}
                    >
                        {!setupMode && <button
                            className="flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:bg-surface-2 md:hidden"
                            onClick={() => setMobileOpen(true)}
                            aria-label={t('nav.menu')}
                        >
                            <Menu className="h-5 w-5" />
                        </button>}
                        {serverIdentity && (
                            <ServerIdentityLabel identity={serverIdentity} placement="mobile" />
                        )}
                        <div
                            ref={setDesktopPageHeaderTarget}
                            className="hidden min-w-0 flex-1 self-stretch xl:flex xl:items-center"
                            data-shell-page-header-target
                        >{setupMode && <p className="text-lg font-semibold">{t('setup.settingsTitle')}</p>}</div>
                        <div className="ml-auto flex shrink-0 items-center gap-2 sm:gap-3">
                            <div className="hidden sm:block">
                                <LanguageSwitcher />
                            </div>
                            <div className={setupMode ? 'hidden sm:block' : undefined}><SkinSwitcher /></div>
                            <ThemeSwitcher />
                            <UserMenu access={navAccess} />
                        </div>
                    </header>

                    <main id="celikpanel-main-content" tabIndex={-1} className="min-h-0 flex-1 overflow-auto">{children}</main>
                    {setupMode && <footer data-setup-build className="shrink-0 border-t border-border bg-surface px-4 py-2 text-xs text-fg-muted [overflow-wrap:anywhere] md:hidden">
                        <BuildStamp runtime={panelRuntime} />
                    </footer>}
                </div>
            </div>
        </DesktopPageHeaderTargetContext.Provider>
    );
}

function Sidebar({
    role,
    access,
    counts,
    currentPage,
    onPageChange,
    mobileOpen,
    onCloseMobile,
    collapsed,
    onToggleCollapsed,
    expandedHeader,
    setupMode,
    setupNavigation,
    serverIdentity,
    panelRuntime,
    updateTarget,
}: {
    role: ReturnType<typeof useAuth>['role'];
    access: NavAccessContext;
    counts: Counts;
    currentPage: string;
    onPageChange: (id: string) => void;
    mobileOpen: boolean;
    onCloseMobile: () => void;
    collapsed: boolean;
    onToggleCollapsed: () => void;
    expandedHeader: boolean;
    setupMode: boolean;
    setupNavigation?: React.ReactNode;
    serverIdentity: ServerIdentity | null;
    panelRuntime: PanelRuntime | null;
    updateTarget: DiscoveredTarget | null;
}) {
    const { t } = useI18n();
    const items = navItemsForRole(role, access);

    // The desktop rail may be collapsed to icons; the phone drawer never is
    // (it is opened on demand and has the room). The setup shell keeps its
    // own navigation and is never collapsed.
    // Masaüstü ray simgelere daralabilir; telefon çekmecesi hiç daralmaz;
    // kurulum kabuğu da daralmaz.
    const buildContent = (desktop: boolean) => {
    const rail = desktop && collapsed && !setupMode;
    return (
        <div className={`flex h-full ${rail ? 'w-16' : 'w-64'} flex-col bg-sidebar text-sidebar-fg`}>
            <div
                className={`flex h-14 items-center gap-2.5 border-b border-sidebar-border ${rail ? 'justify-center px-0' : 'px-5'} ` +
                    (expandedHeader ? 'xl:h-[90px]' : '')}
            >
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-fg">
                    <BrandMark className="h-5 w-5" />
                </div>
                <span className={rail ? 'sr-only' : 'text-lg font-bold text-white'}>{t('app.name')}</span>
            </div>

            {setupMode ? <div className="min-h-0 flex-1 overflow-y-auto px-3 py-6" data-setup-navigation>
                {setupNavigation || <p className="px-3 text-sm font-semibold">{t('setup.settingsTitle')}</p>}
            </div> : <nav className={`flex-1 space-y-4 overflow-y-auto ${rail ? 'px-2' : 'px-3'} py-4`}>
                {desktop && <RailToggle collapsed={collapsed} onToggle={onToggleCollapsed} />}
                {navGroups.map((group) => {
                    const groupItems = items.filter((i) => i.group === group.id);
                    if (groupItems.length === 0) return null;
                    return (
                        <div key={group.id}>
                            {group.labelKey && (rail ? (
                                <div className="mx-2 mb-2 border-t border-sidebar-border" aria-hidden="true" />
                            ) : (
                                <p className="px-3 pb-1.5 text-xs font-semibold uppercase tracking-wider text-sidebar-heading">
                                    {t(group.labelKey)}
                                </p>
                            ))}
                            <div className="space-y-0.5">
                                {groupItems.map((item) => (
                                    <SidebarItem
                                        key={item.id}
                                        item={item}
                                        active={currentPage === item.id}
                                        count={item.countKey ? counts[item.countKey] : undefined}
                                        rail={rail}
                                        onClick={() => onPageChange(item.id)}
                                    />
                                ))}
                            </div>
                        </div>
                    );
                })}
            </nav>}

            <div className={`shrink-0 border-t border-sidebar-border ${rail ? 'flex justify-center px-2' : 'px-4'} py-3 text-xs text-sidebar-muted [overflow-wrap:anywhere]`}>
                {rail ? (
                    updateTarget && (
                        <Link
                            to="/settings?section=updates"
                            onClick={onCloseMobile}
                            data-update-available={updateTarget.version}
                            title={t('panelUpdate.notice.sidebar', { version: updateTarget.version })}
                            aria-label={t('panelUpdate.notice.sidebar', { version: updateTarget.version })}
                            className="flex h-9 w-9 items-center justify-center rounded-lg text-sidebar-fg hover:bg-sidebar-hover hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
                        >
                            <DownloadCloud className="h-[18px] w-[18px] shrink-0" aria-hidden="true" />
                        </Link>
                    )
                ) : (<>
                {serverIdentity && (
                    <ServerIdentityLabel identity={serverIdentity} placement="sidebar" />
                )}
                <div className={serverIdentity ? 'border-t border-sidebar-border pt-3' : ''}>
                    <BuildStamp runtime={panelRuntime} />
                    {/* An available update is not a warning: plain rail ink,
                        an icon and words, no badge colour (D-035, DESIGN.md).
                        Güncelleme uyarı değildir: rengi yok, ikon ve sözcük. */}
                    {updateTarget && (
                        <Link
                            to="/settings?section=updates"
                            onClick={onCloseMobile}
                            data-update-available={updateTarget.version}
                            className="mt-2 inline-flex min-h-6 items-center gap-1.5 font-medium text-sidebar-fg underline decoration-sidebar-border underline-offset-4 hover:text-white hover:decoration-current focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70 max-md:min-h-11"
                        >
                            <DownloadCloud className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                            <span>{wholeVersion(t('panelUpdate.notice.sidebar', { version: updateTarget.version }), updateTarget.version)}</span>
                        </Link>
                    )}
                </div>
                </>)}
            </div>
        </div>
    );
    };
    const content = buildContent(false);

    return (
        <>
            <aside id="celikpanel-sidebar" className="hidden shrink-0 md:block" data-sidebar-collapsed={collapsed && !setupMode ? 'true' : 'false'}>{buildContent(true)}</aside>
            {mobileOpen && (
                <div className="fixed inset-0 z-40 md:hidden">
                    <div className="absolute inset-0 bg-scrim/80" onClick={onCloseMobile} />
                    <div className="absolute left-0 top-0 h-full">{content}</div>
                </div>
            )}
        </>
    );
}

function SidebarItem({
    item,
    active,
    count,
    rail = false,
    onClick,
}: {
    item: NavItem;
    active: boolean;
    count?: number | null;
    rail?: boolean;
    onClick: () => void;
}) {
    const { t } = useI18n();
    const Icon = item.icon;
    const label = t(item.labelKey);
    // Collapsed, the label is gone from view, so the button keeps its name
    // (with the count it would have shown) and says it in a tooltip, on hover
    // and on keyboard focus.
    // Daraltılmışken etiket görünmez; düğme adını (sayıyla) korur ve ipucu
    // gösterir: fare üstünde ve klavye odağında.
    const railName = count === null ? `${label}, ${t('services.notChecked')}` : count !== undefined && count > 0 ? `${label}, ${count}` : label;
    const tip = useRailTip();
    return (
        <button
            onClick={onClick}
            {...(rail ? { title: railName, 'aria-label': railName, ...tip.handlers } : {})}
            className={`flex w-full items-center ${rail ? 'justify-center px-0' : 'gap-3 px-3'} rounded-lg py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70 ${
                active
                    ? 'bg-sidebar-active text-sidebar-active-fg'
                    // The lit plate belongs to the current route, so hover cannot take
                    // it. The ground shifts and the label goes to full white, which is
                    // what actually carries the state. / Aydinlik zemin acik rotanindir;
                    // fare ustunde zemin degisir ve etiket beyaza cikar.
                    : 'text-sidebar-fg hover:bg-sidebar-hover hover:text-white'
            }`}
        >
            <Icon className="h-[18px] w-[18px] shrink-0" />
            {rail && tip.node(railName)}
            {!rail && <span className="flex-1 text-left">{label}</span>}
            {/* A dash is the honest badge for a count this panel does not
                have — the same mark the Components page uses for a group
                nothing is known about. It is not styled down: "not checked
                yet" is a state the operator acts on, and the item it sits on
                opens the page that runs the check.
                Sahip olunmayan bir sayının dürüst rozeti tiredir — Bileşenler
                sayfasının, hakkında hiçbir şey bilinmeyen grup için kullandığı
                işaretin aynısı. */}
            {/* A count is a plain muted number, not a filled pill: on the
                owner's screens the pill read as a notification count
                (alpha.95, 2026-10-17). Null is still not zero.
                Sayı dolu hap değil, sade soluk bir sayıdır; null yine sıfır değildir. */}
            {rail ? null : <>{count === null ? (
                <span
                    title={t('services.notChecked')}
                    className={`text-xs font-medium ${navCountTone(active)}`}
                    data-nav-count="unknown"
                >
                    <span aria-hidden="true">—</span>
                    <span className="sr-only">{t('services.notChecked')}</span>
                </span>
            ) : count !== undefined && count > 0 ? (
                <span className={`text-xs font-medium tabular-nums ${navCountTone(active)}`} data-nav-count="">
                    {count}
                </span>
            ) : null}</>}
        </button>
    );
}

// A tooltip for the collapsed rail that also shows on keyboard focus. It is
// drawn with fixed positioning from the button's own box, because the rail's
// nav scrolls and would clip an absolutely positioned one. The native title
// serves the pointer; this one appears for keyboard focus only, so the two
// never double. Escape dismisses it.
// Daraltılmış ray için, klavye odağında da görünen ipucu; fare için yerel
// title kullanılır, bu yalnız klavye odağında çıkar; Escape kapatır.
function useRailTip() {
    const [at, setAt] = useState<{ top: number; left: number } | null>(null);
    const handlers = {
        onFocus: (event: React.FocusEvent<HTMLElement>) => {
            let visible = true;
            try {
                visible = event.currentTarget.matches(':focus-visible');
            } catch {
                visible = true;
            }
            if (!visible) return;
            const box = event.currentTarget.getBoundingClientRect();
            setAt({ top: box.top + box.height / 2, left: box.right + 8 });
        },
        onBlur: () => setAt(null),
        onKeyDown: (event: React.KeyboardEvent<HTMLElement>) => {
            if (event.key === 'Escape') setAt(null);
        },
    };
    const node = (text: string) => at && (
        <span
            aria-hidden="true"
            data-rail-tip=""
            style={{ position: 'fixed', top: at.top, left: at.left, transform: 'translateY(-50%)' }}
            className="pointer-events-none z-50 whitespace-nowrap rounded-md border border-border bg-surface px-2.5 py-1 text-xs font-medium text-fg shadow-lg"
        >
            {text}
        </span>
    );
    return { handlers, node };
}

// The collapse control: a real button that says whether the rail is
// expanded and which element it controls. Collapsed, it is an icon with the
// same tooltip behaviour as the items.
// Daraltma düğmesi: gerçek bir düğme; ray açık mı, hangi öğeyi denetliyor.
function RailToggle({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
    const { t } = useI18n();
    const tip = useRailTip();
    const label = t(collapsed ? 'nav.expand' : 'nav.collapse');
    return (
        <button
            type="button"
            onClick={onToggle}
            aria-expanded={!collapsed}
            aria-controls="celikpanel-sidebar"
            aria-label={label}
            title={label}
            data-sidebar-toggle=""
            {...(collapsed ? tip.handlers : {})}
            className={`flex w-full items-center ${collapsed ? 'justify-center px-0' : 'gap-3 px-3'} rounded-lg py-2 text-sm font-medium text-sidebar-fg transition-colors hover:bg-sidebar-hover hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70`}
        >
            <ChevronDown className={`h-[18px] w-[18px] shrink-0 ${collapsed ? '-rotate-90' : 'rotate-90'}`} aria-hidden="true" />
            {collapsed ? tip.node(label) : <span className="flex-1 text-left">{label}</span>}
        </button>
    );
}

const navCountTone = (active: boolean) => (active ? 'text-white/80' : 'text-sidebar-muted');

function ServerIdentityLabel({ identity, placement }: {
    identity: ServerIdentity;
    placement: 'mobile' | 'sidebar';
}) {
    const { t } = useI18n();
    const title = [identity.hostname, identity.ipv4].filter(Boolean).join(' · ');
    const accessibleLabel = [
        t('dashboard.serverInfo'),
        identity.hostname ? t('dashboard.hostname') + ': ' + identity.hostname : '',
        identity.ipv4 ? t('dashboard.ipv4') + ': ' + identity.ipv4 : '',
    ].filter(Boolean).join(', ');

    if (placement === 'mobile') {
        return (
            <section
                data-server-identity="mobile"
                className="min-w-0 max-w-[10rem] md:hidden"
                aria-label={accessibleLabel}
                title={title}
            >
                {identity.hostname && (
                    <p className="truncate text-xs font-semibold text-fg">{identity.hostname}</p>
                )}
                {identity.ipv4 && (
                    <p className="truncate font-mono text-xs text-fg-muted" dir="ltr">
                        {identity.ipv4}
                    </p>
                )}
            </section>
        );
    }

    return (
        <section
            data-server-identity="sidebar"
            className="mb-3 min-w-0"
            aria-label={accessibleLabel}
            title={title}
        >
            <p className="mb-1 text-xs font-semibold uppercase tracking-wider text-sidebar-heading">
                {t('dashboard.serverInfo')}
            </p>
            {identity.hostname && (
                <p className="truncate font-medium text-sidebar-fg" title={identity.hostname}>
                    {identity.hostname}
                </p>
            )}
            {identity.ipv4 && (
                <p
                    className="truncate font-mono text-xs text-sidebar-muted"
                    title={identity.ipv4}
                    dir="ltr"
                >
                    {identity.ipv4}
                </p>
            )}
        </section>
    );
}

// ImpersonationBanner: a loud, always-visible band while an operator browses
// as another account, with the way back one click away.
// ImpersonationBanner: bir operatör başka bir hesap olarak gezinirken hep
// görünen belirgin bir şerit; dönüş yolu tek tık uzakta.
function ImpersonationBanner() {
    const { user } = useAuth();
    const { t } = useI18n();
    if (!user.impersonating) return null;

    const exit = async () => {
        await fetch('/api/v1/auth/unimpersonate', { method: 'POST' });
        window.location.assign('/');
    };

    return (
        <div className="flex flex-wrap items-center justify-center gap-2 bg-warning px-4 py-1.5 text-sm font-medium text-warning-fg">
            <UserCheck className="h-4 w-4" />
            {user.impersonator
                ? t('imp.bannerBy', { name: user.username, operator: user.impersonator })
                : t('imp.banner', { name: user.username })}
            <button onClick={exit} className="rounded-md bg-black/15 px-2.5 py-0.5 font-semibold hover:bg-black/25">
                {t('imp.return')}
            </button>
        </div>
    );
}

// UserMenu is the personal menu: the current user and role, then My account
// first (the person's own password and two-step sign-in, D-037), the language
// on a phone, and Sign out. A disclosure, not a menu widget: the button says
// whether it is open, Tab walks its links and buttons, Escape closes it and
// gives focus back to the button, and it closes when focus leaves it (Tab past
// its last item). aria-controls names the panel only while it exists. A role without My account (an additional
// user) keeps the password change here, where it always was, and since
// v0.1.0-alpha.99 the email change beside it.
// UserMenu kişisel menüdür: kullanıcı ve rol, sonra ilk sırada Hesabım
// (D-037), telefonda dil ve Çıkış. Açılır-kapanır bir bölmedir: düğme açık
// olup olmadığını söyler, Tab içinde gezer, Escape kapatıp odağı düğmeye
// verir; odak bölmeden çıkınca (Tab ile son öğeden öteye) kapanır. Hesabım'ı
// olmayan rol (ek kullanıcı) şifre değiştirmeyi burada tutar.
function UserMenu({ access }: { access: NavAccessContext }) {
    const { user, role, logout } = useAuth();
    const { t, screensReady, screensFailed } = useI18n();
    const [open, setOpen] = useState(false);
    const [showPassword, setShowPassword] = useState(false);
    const [showEmail, setShowEmail] = useState(false);
    const ref = useRef<HTMLDivElement>(null);
    const buttonRef = useRef<HTMLButtonElement>(null);
    const roleKey = `role.${role}` as TranslationKey;
    const hasAccountPage = navItemsForRole(role, access).some((item) => item.id === 'account');

    useEffect(() => {
        if (!open) return;
        const onClick = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== 'Escape') return;
            setOpen(false);
            buttonRef.current?.focus();
        };
        document.addEventListener('mousedown', onClick);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onClick);
            document.removeEventListener('keydown', onKey);
        };
    }, [open]);

    const itemClass = 'flex w-full items-center gap-2 px-4 py-2.5 text-sm text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg max-sm:min-h-11';

    return (
        <div
            className="relative"
            ref={ref}
            onBlur={(event) => {
                if (open && !event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false);
            }}
        >
            <button
                ref={buttonRef}
                type="button"
                aria-expanded={open}
                aria-controls={open ? 'personal-menu' : undefined}
                onClick={() => setOpen((o) => !o)}
                className="flex items-center gap-2 rounded-lg border border-border bg-surface-2 py-1 pl-1 pr-2 text-sm transition-colors hover:border-border-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
                <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-xs font-semibold text-primary-fg">
                    {user.username.slice(0, 2).toUpperCase()}
                </span>
                <span className="hidden max-w-[8rem] truncate font-medium sm:block">{user.username}</span>
                <ChevronDown className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
            </button>

            {open && (
                <div id="personal-menu" data-personal-menu="" className="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-xl border border-border bg-surface shadow-lg">
                    <div className="border-b border-border px-4 py-3">
                        <p className="truncate text-sm font-semibold">{user.username}</p>
                        <p className="text-xs text-fg-subtle">{t(roleKey)}</p>
                    </div>
                    {hasAccountPage && (
                        <Link to="/account" onClick={() => setOpen(false)} className={itemClass} data-personal-menu-item="account">
                            <UserCheck className="h-4 w-4" aria-hidden="true" />
                            {t('nav.account')}
                        </Link>
                    )}
                    <div className="p-2 sm:hidden">
                        <LanguageSwitcher />
                    </div>
                    {!hasAccountPage && (
                        <button
                            type="button"
                            onClick={() => {
                                setOpen(false);
                                setShowPassword(true);
                            }}
                            className={itemClass}
                        >
                            <KeyRound className="h-4 w-4" aria-hidden="true" />
                            {t('profile.changePassword')}
                        </button>
                    )}
                    {!hasAccountPage && screensReady && !screensFailed && (
                        <button
                            type="button"
                            onClick={() => {
                                setOpen(false);
                                setShowEmail(true);
                            }}
                            className={itemClass}
                            data-personal-menu-item="email"
                        >
                            <Mail className="h-4 w-4" aria-hidden="true" />
                            {t('profile.changeEmail')}
                        </button>
                    )}
                    <button
                        type="button"
                        onClick={async () => {
                            await api.logout();
                            logout();
                            window.location.reload();
                        }}
                        className={itemClass.replace('hover:text-fg', 'hover:text-danger')}
                        data-personal-menu-item="logout"
                    >
                        <LogOut className="h-4 w-4" aria-hidden="true" />
                        {t('user.logout')}
                    </button>
                </div>
            )}

            {showPassword && <Suspense fallback={null}><ChangePasswordModal onClose={() => setShowPassword(false)} /></Suspense>}
            {showEmail && <Suspense fallback={null}><ChangeEmailModal current={null} onClose={() => setShowEmail(false)} /></Suspense>}
        </div>
    );
}

// The footer stamp, read from the SERVER. The version used to be a literal
// typed into this file, and the commit came from the frontend bundle only — so
// a deploy that replaced the backend changed neither, while common.buildHint
// promised the operator this is how you see a new build land. Now the version
// and the panel's commit come from the running binary, and a panel/agent
// mismatch is shown as a warning rather than hidden behind a reassuring stamp.
//
// Footer damgası, SUNUCUDAN okunur. Sürüm eskiden bu dosyaya yazılmış bir
// metindi ve commit yalnız ön yüz paketinden geliyordu — yani arka ucu
// değiştiren bir dağıtım ikisini de değiştirmiyordu, oysa common.buildHint
// operatöre yeni yapının indiğini böyle göreceğini söylüyordu. Artık sürüm ve
// panelin commit'i çalışan binary'den gelir; panel/agent uyuşmazlığı da güven
// veren bir damganın arkasına gizlenmek yerine uyarı olarak gösterilir.
export function BuildStamp({ runtime }: { runtime: PanelRuntime | null }) {
    const { t } = useI18n();
    if (!runtime) return <>{t('app.name')}</>;
    const v = runtime;

    // The full commit is a 40-character unbroken token. In a 256px rail with
    // 16px of padding on each side it measured 24px WIDER than the aside it
    // sits in, so the tail of the hash was drawn on the page background —
    // outside the rail, at every width. A hash is identification, not prose:
    // the leading characters are the part an operator reads and compares, so
    // the stamp shows those and keeps the whole value one hover (or one
    // screen reader) away instead of dropping the commit or letting it spill.
    //
    // Tam commit 40 karakterlik bölünmez bir belirteçtir. 256px genişliğinde,
    // iki yanı 16px boşluklu bir rayda içinde durduğu aside'dan 24px DAHA
    // GENİŞ ölçülüyordu; yani hash'in kuyruğu her genişlikte rayın dışına,
    // sayfa zeminine çiziliyordu. Hash düzyazı değil kimliktir: operatörün
    // okuyup karşılaştırdığı yer baş taraftır; damga onu gösterir, tam değeri
    // ise commit'i atmadan ya da taşırmadan bir üşürme kadar uzakta tutar.
    // "build" and the short id are one unit that never breaks inside: with
    // break-all the id broke after its third character ("build f01" /
    // "57e6c8f16"), which reads as a different number (owner, alpha.83 and
    // alpha.84). The line now wraps only before the unit; the full id stays in
    // the title and for screen readers.
    // "build" ve kısa kimlik içinden bölünmeyen tek birimdir; satır yalnız
    // birimden önce kırılır, tam kimlik başlıkta ve ekran okuyucuda kalır.

    return (
        <>
            {t('app.name')} · {v.version} ·{' '}
            <span className="inline-block whitespace-nowrap font-mono" data-build-stamp="" title={`${t('common.buildHint')} — ${v.commit}`}>
                <BuildCommit commit={v.commit} />
            </span>
            {!v.agent_matches && v.agent_commit !== '' && (
                <span className="ml-1.5 rounded bg-warning/15 px-1.5 py-0.5 text-warning" title={t('common.agentMismatchHint', { commit: v.agent_commit })}>
                    {t('common.agentMismatch')}
                </span>
            )}
        </>
    );
}
