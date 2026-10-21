import { LayoutDashboard, Globe, Database, Server, Users, Settings, DownloadCloud, ScrollText, Package, Shield, Activity, Network, ShieldCheck, Wrench, User, type LucideIcon } from 'lucide-react';
import type { TranslationKey } from './i18n/en';
import type { Role } from './auth/AuthContext';

// The single source of navigation. Each entry declares the roles that may
// see it and an optional group heading; the sidebar is this list filtered
// by the current user's role, and route guards check the same list. Adding
// a section is one entry here — no per-role layout, exactly as
// docs/UI_ARCHITECTURE.md prescribes.
//
// Navigasyonun tek kaynağı. Her girdi, onu görebilecek rolleri ve isteğe
// bağlı bir grup başlığını bildirir; kenar çubuğu bu listenin mevcut
// kullanıcının rolüne göre süzülmüş halidir ve rota koruyucuları aynı
// listeyi kontrol eder.
//
// 'personal' items are the signed-in person's own pages. They are not drawn in
// the rail: the personal menu at the top right opens them (D-037), and the
// route guard reads them from this same list.
// 'personal' öğeler kişinin kendi sayfalarıdır; rayda çizilmez, sağ üstteki
// kişisel menü açar (D-037). Rota koruyucusu onları bu listeden okur.
export type NavGroup = 'main' | 'hosting' | 'server' | 'personal';

export interface NavItem {
    id: string;
    path: string;
    labelKey: TranslationKey;
    icon: LucideIcon;
    roles: Role[];
    group: NavGroup;
    countKey?: 'domains' | 'databases' | 'services';
}

export interface NavAccessContext {
    accountType?: string;
    teamMembers?: boolean;
}

const ACCOUNT_ROLES: Role[] = ['admin', 'reseller', 'customer'];
const DOMAIN_ROLES: Role[] = [...ACCOUNT_ROLES, 'additional_user'];

export const navItems: NavItem[] = [
    { id: 'dashboard', path: '/', labelKey: 'nav.dashboard', icon: LayoutDashboard, roles: DOMAIN_ROLES, group: 'main' },
    { id: 'domains', path: '/domains', labelKey: 'nav.domains', icon: Globe, roles: DOMAIN_ROLES, group: 'hosting', countKey: 'domains' },
    // Admin-only UNTIL the v2 DB API is tenant-scoped: the backend hardcodes
    // subscription 1 (database_v2_handlers.go "TODO: Get from auth") and the
    // whole /api/v2/ prefix is admin-gated — non-admins only ever saw a
    // broken page here. Tenants manage their DBs on the domain detail tab.
    // B1 role split (Jul 17): the v2 DB API is tenant-scoped and the blanket
    // admin gate is gone — Databases is self-service again. Server
    // REGISTRATION stays admin-only in the backend; this page has no
    // register button anyway (servers are auto-discovered).
    // B1 rol ayrımı (17 Tem): v2 DB API'si kiracı-kapsamlı, battaniye admin
    // kilidi kalktı — Databases yeniden self-servis. Sunucu KAYDI arka uçta
    // admin'de kalır; bu sayfada zaten kayıt düğmesi yok (oto-keşif).
    { id: 'databases', path: '/databases', labelKey: 'nav.databases', icon: Database, roles: ['admin', 'reseller', 'customer'], group: 'hosting', countKey: 'databases' },
    { id: 'users', path: '/users', labelKey: 'nav.users', icon: Users, roles: ['admin', 'reseller', 'customer'], group: 'hosting' },
    { id: 'addons', path: '/addons', labelKey: 'nav.addons', icon: Package, roles: ['admin', 'reseller'], group: 'hosting' },
    { id: 'vpn', path: '/vpn', labelKey: 'nav.vpn', icon: Shield, roles: ACCOUNT_ROLES, group: 'hosting' },
    // Every page is where its name says it is (D-037, owner 2026-10-22): the
    // server's tools, its security and its maintenance live under Server;
    // Settings holds only what is chosen once and decides how the panel
    // behaves; the person's own account is in the personal menu. Only the
    // place moved: each page keeps the roles it had inside Settings.
    // Her sayfa adının söylediği yerdedir (D-037): sunucunun araçları,
    // güvenliği ve bakımı Sunucu altında; Ayarlar'da yalnız bir kez seçilip
    // panelin davranışını belirleyenler; kişinin kendi hesabı kişisel menüde.
    // Yalnız yer değişti; her sayfa Ayarlar içindeki rollerini korur.
    { id: 'services', path: '/services', labelKey: 'nav.services', icon: Server, roles: ['admin'], group: 'server', countKey: 'services' },
    { id: 'dns', path: '/dns-infrastructure', labelKey: 'nav.dnsPeers', icon: Network, roles: ['admin'], group: 'server' },
    { id: 'monitoring', path: '/monitoring', labelKey: 'nav.monitoring', icon: Activity, roles: ['admin'], group: 'server' },
    { id: 'security', path: '/security', labelKey: 'nav.security', icon: ShieldCheck, roles: ['admin'], group: 'server' },
    { id: 'maintenance', path: '/maintenance', labelKey: 'nav.maintenance', icon: Wrench, roles: ['admin'], group: 'server' },
    { id: 'import', path: '/import', labelKey: 'nav.import', icon: DownloadCloud, roles: ['admin'], group: 'server' },
    { id: 'audit', path: '/audit', labelKey: 'nav.audit', icon: ScrollText, roles: ['admin'], group: 'server' },
    // Settings has nothing left for a reseller or a customer: their only
    // section was their own account, which is now My account. /settings
    // takes them there (settingsRedirect below).
    // Bayi ve müşteri için Ayarlar'da bir şey kalmadı; /settings onları
    // Hesabım'a götürür.
    { id: 'settings', path: '/settings', labelKey: 'nav.settings', icon: Settings, roles: ['admin'], group: 'server' },
    { id: 'account', path: '/account', labelKey: 'nav.account', icon: User, roles: ACCOUNT_ROLES, group: 'personal' },
];

// Group order and their heading translation keys. A group with no visible
// items for the current role is skipped by the sidebar.
// Grup sırası ve başlık çeviri anahtarları. Mevcut rol için görünür öğesi
// olmayan bir grup kenar çubuğunda atlanır.
// The personal group is not here: it is the personal menu, not the rail.
// Kişisel grup burada değildir; o raya değil kişisel menüye aittir.
export const navGroups: { id: Exclude<NavGroup, 'personal'>; labelKey?: TranslationKey }[] = [
    { id: 'main' },
    { id: 'hosting', labelKey: 'nav.group.hosting' },
    { id: 'server', labelKey: 'nav.group.server' },
];

export function isNavItemAllowed(item: NavItem, role: Role, access?: NavAccessContext): boolean {
    if (!item.roles.includes(role)) return false;
    if (item.id === 'users' && role === 'customer') {
        return access?.accountType === 'account' && access.teamMembers === true;
    }
    return true;
}

export function navItemsForRole(role: Role, access?: NavAccessContext): NavItem[] {
    return navItems.filter((item) => isNavItemAllowed(item, role, access));
}

// canAccessPath guards routes from the same fail-closed navigation contract.
// Child pages inherit the nearest segment-aware parent; paths with no known
// parent are denied instead of accidentally inheriting customer access.
// canAccessPath rotaları korur: bir rol yalnızca görebileceği bir nav
// öğesine eşlenen bir yolu açabilir. Bilinmeyen yollar varsayılan izinli.
export function canAccessPath(role: Role, path: string, access?: NavAccessContext): boolean {
    const normalizedPath = path.split('?')[0].replace(/\/$/, '') || '/';
    const match = [...navItems]
        .sort((a, b) => b.path.length - a.path.length)
        .find((item) => item.path === '/'
            ? normalizedPath === '/'
            : normalizedPath === item.path || normalizedPath.startsWith(`${item.path}/`));
    return match ? isNavItemAllowed(match, role, access) : false;
}

// Settings sections that moved to their own pages in v0.1.0-alpha.98 (D-037),
// by their old address. An old link, a bookmark or a guidance text from an
// earlier release still arrives: /settings?section=<id> is replaced (no extra
// history entry) by the page that holds the section now.
// v0.1.0-alpha.98'de kendi sayfasına taşınan Ayarlar bölümleri, eski
// adresleriyle. Eski bağlantı, yer imi ya da önceki sürümün yönlendirme metni
// yine ulaşır: /settings?section=<id>, bölümün bugünkü sayfasıyla değiştirilir.
export const movedSettingsSections: Partial<Record<string, string>> = {
    account: '/account',
    security: '/security',
    dns: '/dns-infrastructure',
    'system-databases': '/maintenance',
};

// Where /settings sends this person, or null to stay. A section that moved
// goes to its page; a person for whom Settings has no section any more (a
// reseller or a customer: their only section was their own account) goes to
// My account, whatever section the address named, as Settings used to open
// their account section for any section it could not show them.
// /settings'in bu kişiyi nereye gönderdiği; kalacaksa null.
export function settingsRedirect(search: string, role: Role): string | null {
    const section = new URLSearchParams(search).get('section');
    if (role !== 'admin') return ACCOUNT_ROLES.includes(role) ? '/account' : null;
    return (section && Object.prototype.hasOwnProperty.call(movedSettingsSections, section) && movedSettingsSections[section]) || null;
}
