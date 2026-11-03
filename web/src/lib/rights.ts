import type { TranslationKey } from '../i18n/en';
import type { ServicePlan } from './api';
import type { ApiError } from './apiError';
import { readApiError } from './apiError';

// Plans and rights (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md). Every
// account's rights are Unlimited, bound to a plan, or Custom; a rights set is
// quantities, permissions (which jobs) and software (which component and
// version, by offering id). The server enforces every right; these screens
// only show and ask (P6). This module holds the addresses, the decoders and
// the pure helpers every rights screen shares, so the screens agree on names
// and on what is selectable.
//
// Planlar ve haklar: her hesabın hakları Sınırsız, bir plana bağlı ya da
// Özel'dir. Sunucu her hakkı uygular; ekranlar yalnız gösterir ve sorar. Bu
// modül adresleri, çözücüleri ve saf yardımcıları tutar.

export const RIGHTS_CATALOG_URL = '/api/v1/rights/catalog';
export const RIGHTS_NOTICES_URL = '/api/v1/rights/notices';
export const RESELLER_PLANS_URL = '/api/v1/reseller-plans';
export const OFFERED_URL = '/api/v1/offered-versions';
export const subscriptionRightsURL = (id: number) => `/api/v1/subscriptions/${id}/rights`;
export const subscriptionEndsAtURL = (id: number) => `/api/v1/subscriptions/${id}/ends-at`;
export const userRightsURL = (id: number) => `/api/v1/users/${id}/rights`;

/** The closed set of permissions, in the server's order (section 4.2). */
export const PERMISSION_IDS = [
    'dns_edit', 'dnssec', 'mail', 'databases', 'cron', 'file_manager', 'backups', 'ssl_acme', 'ssl_custom',
    'site_protect_redirects', 'site_proxy', 'app_installer', 'vpn', 'team_members', 'file_access', 'ssh',
    // alpha.118, in the server's order: own remote backup destinations, AI and
    // automation apps, then the site tools (rights_site_tools.go appends them).
    'remote_backup_destinations', 'service_apps', 'wordpress_tools', 'malware_scan',
] as const;
export type PermissionId = (typeof PERMISSION_IDS)[number];

export type RightsState = 'unlimited' | 'plan' | 'custom';

export interface RightsPayload {
    all_permissions: boolean;
    all_software: boolean;
    permissions: string[];
    software: string[];
}

export interface RightsItem {
    id: string;
    name?: string;
    granted: boolean;
    /** Which layer closes it: administrator, reseller, subscription or add_on. */
    refused_by?: string;
    grantor?: string;
}

export interface RightsView {
    subscription_id: number;
    state: RightsState;
    plan_id?: number;
    plan_name?: string;
    unrestricted: boolean;
    own: RightsPayload;
    permissions: RightsItem[];
    software: RightsItem[];
    installed_unknown?: boolean;
}

export interface Limits {
    max_domains: number;
    max_databases: number;
    max_email_accounts: number;
    disk_quota_mb: number;
    /** Monthly website traffic in MB (alpha.118, D-044); 0 is unlimited. */
    traffic_quota_mb: number;
}

export interface PoolNumbers extends Limits {
    max_customers: number;
}

export interface PoolUsage {
    customers: number;
    domains: number;
    databases: number;
    mailboxes: number;
    disk_used_bytes: number;
    /** The tree's website traffic this month (alpha.118). */
    traffic_month_bytes: number;
}

export interface SubscriptionRights extends RightsView {
    limits: Limits;
    /** The end date (UTC, "YYYY-MM-DD ..."), or null: it never ends (K7). */
    ends_at: string | null;
}

export interface ResellerRights {
    role: 'reseller';
    rights: RightsView;
    pool: PoolNumbers | null;
    used: PoolUsage;
    over_limit: string[];
}

export interface CustomerRightsList {
    role: 'customer';
    subscriptions: { id: number; name: string; state: string; plan_name?: string }[];
}

export interface InUseLine {
    item: string;
    count: number;
    what: string;
}

export interface RightsPreview {
    from: string;
    from_plan_name?: string;
    to: string;
    to_plan_name?: string;
    leaves_plan: boolean;
    added: string[];
    removed: string[];
    in_use: InUseLine[];
    over_limit: string[];
    outside_grantor: string[];
    installed_unknown?: boolean;
    subscriptions?: number;
    message: string;
}

export interface ResellerPlan extends PoolNumbers {
    id: number;
    name: string;
    rights?: RightsPayload;
    resellers?: number;
}

export interface RightsNotice {
    id: number;
    kind: string;
    user_id: number;
    username: string;
    subject: string;
    detail: string;
    created_at: string;
}

const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((item): item is string => typeof item === 'string') : []);
const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

function decodePayload(raw: unknown): RightsPayload {
    if (!isRecord(raw)) throw new Error('shape');
    return {
        all_permissions: raw.all_permissions === true,
        all_software: raw.all_software === true,
        permissions: strings(raw.permissions),
        software: strings(raw.software),
    };
}

function decodeItems(raw: unknown): RightsItem[] {
    if (raw === null) return [];
    if (!Array.isArray(raw)) throw new Error('shape');
    return raw.map((item) => {
        if (!isRecord(item) || typeof item.id !== 'string' || typeof item.granted !== 'boolean') throw new Error('shape');
        return item as unknown as RightsItem;
    });
}

function decodeState(raw: unknown): RightsState {
    if (raw === 'unlimited' || raw === 'plan' || raw === 'custom') return raw;
    throw new Error('shape');
}

export function decodeRightsView(raw: unknown): RightsView {
    if (!isRecord(raw)) throw new Error('shape');
    return {
        subscription_id: num(raw.subscription_id),
        state: decodeState(raw.state),
        plan_id: typeof raw.plan_id === 'number' ? raw.plan_id : undefined,
        plan_name: typeof raw.plan_name === 'string' && raw.plan_name ? raw.plan_name : undefined,
        unrestricted: raw.unrestricted === true,
        own: decodePayload(raw.own),
        permissions: decodeItems(raw.permissions),
        software: decodeItems(raw.software),
        installed_unknown: raw.installed_unknown === true || undefined,
    };
}

function decodeLimits(raw: unknown): Limits {
    if (!isRecord(raw)) throw new Error('shape');
    return {
        max_domains: num(raw.max_domains),
        max_databases: num(raw.max_databases),
        max_email_accounts: num(raw.max_email_accounts),
        disk_quota_mb: num(raw.disk_quota_mb),
        traffic_quota_mb: num(raw.traffic_quota_mb),
    };
}

function decodePool(raw: unknown): PoolNumbers {
    if (!isRecord(raw)) throw new Error('shape');
    return { ...decodeLimits(raw), max_customers: num(raw.max_customers) };
}

/** GET /api/v1/rights/catalog: what the reader may choose or hand out. */
export function decodeCatalog(raw: unknown): RightsView {
    if (!isRecord(raw)) throw new Error('shape');
    return decodeRightsView(raw.rights);
}

/** GET /api/v1/subscriptions/{id}/rights. */
export function decodeSubscriptionRights(raw: unknown): SubscriptionRights {
    if (!isRecord(raw)) throw new Error('shape');
    const lifecycle = isRecord(raw.lifecycle) ? raw.lifecycle : {};
    const endsAt = typeof lifecycle.ends_at === 'string' && lifecycle.ends_at !== '' ? lifecycle.ends_at : null;
    return { ...decodeRightsView(raw), limits: decodeLimits(raw.limits), ends_at: endsAt };
}

export function decodePoolUsage(raw: unknown): PoolUsage {
    if (!isRecord(raw)) throw new Error('shape');
    return {
        customers: num(raw.customers),
        domains: num(raw.domains),
        databases: num(raw.databases),
        mailboxes: num(raw.mailboxes),
        disk_used_bytes: num(raw.disk_used_bytes),
        traffic_month_bytes: num(raw.traffic_month_bytes),
    };
}

/** GET /api/v1/users/{id}/rights: a reseller's state, or a customer's subscriptions. */
export function decodeUserRights(raw: unknown): ResellerRights | CustomerRightsList {
    if (!isRecord(raw)) throw new Error('shape');
    if (raw.role === 'customer') {
        if (!Array.isArray(raw.subscriptions) && raw.subscriptions !== null) throw new Error('shape');
        const subscriptions = (raw.subscriptions ?? []) as unknown[];
        return {
            role: 'customer',
            subscriptions: subscriptions.map((entry) => {
                if (!isRecord(entry) || typeof entry.id !== 'number') throw new Error('shape');
                return {
                    id: entry.id,
                    name: typeof entry.name === 'string' ? entry.name : '',
                    state: typeof entry.state === 'string' ? entry.state : '',
                    plan_name: typeof entry.plan_name === 'string' && entry.plan_name ? entry.plan_name : undefined,
                };
            }),
        };
    }
    if (raw.role !== 'reseller') throw new Error('shape');
    return {
        role: 'reseller',
        rights: decodeRightsView(raw.rights),
        pool: raw.pool === null || raw.pool === undefined ? null : decodePool(raw.pool),
        used: decodePoolUsage(raw.used),
        over_limit: strings(raw.over_limit),
    };
}

export function decodePreview(raw: unknown): RightsPreview {
    if (!isRecord(raw)) throw new Error('shape');
    const inUse = Array.isArray(raw.in_use) ? raw.in_use : [];
    return {
        from: typeof raw.from === 'string' ? raw.from : '',
        from_plan_name: typeof raw.from_plan_name === 'string' && raw.from_plan_name ? raw.from_plan_name : undefined,
        to: typeof raw.to === 'string' ? raw.to : '',
        to_plan_name: typeof raw.to_plan_name === 'string' && raw.to_plan_name ? raw.to_plan_name : undefined,
        leaves_plan: raw.leaves_plan === true,
        added: strings(raw.added),
        removed: strings(raw.removed),
        in_use: inUse.filter(isRecord).map((line) => ({ item: String(line.item ?? ''), count: num(line.count), what: String(line.what ?? '') })),
        over_limit: strings(raw.over_limit),
        outside_grantor: strings(raw.outside_grantor),
        installed_unknown: raw.installed_unknown === true || undefined,
        subscriptions: typeof raw.subscriptions === 'number' ? raw.subscriptions : undefined,
        message: typeof raw.message === 'string' ? raw.message : '',
    };
}

export interface ResellerPlansAnswer {
    plans: ResellerPlan[];
    builtin: { key: string; name: string; kind: string }[];
}

export function decodeResellerPlans(raw: unknown): ResellerPlansAnswer {
    if (!isRecord(raw) || (!Array.isArray(raw.plans) && raw.plans !== null)) throw new Error('shape');
    const plans = ((raw.plans ?? []) as unknown[]).map((entry) => {
        if (!isRecord(entry) || typeof entry.id !== 'number' || typeof entry.name !== 'string') throw new Error('shape');
        return {
            ...decodePool(entry),
            id: entry.id,
            name: entry.name,
            rights: isRecord(entry.rights) ? decodePayload(entry.rights) : undefined,
            resellers: num(entry.resellers),
        };
    });
    const builtin = Array.isArray(raw.builtin) ? raw.builtin.filter(isRecord).map((b) => ({ key: String(b.key ?? ''), name: String(b.name ?? ''), kind: String(b.kind ?? '') })) : [];
    return { plans, builtin };
}

export function decodeNotices(raw: unknown): RightsNotice[] {
    if (!isRecord(raw) || (!Array.isArray(raw.notices) && raw.notices !== null)) throw new Error('shape');
    return ((raw.notices ?? []) as unknown[]).map((entry) => {
        if (!isRecord(entry) || typeof entry.id !== 'number' || typeof entry.kind !== 'string') throw new Error('shape');
        return {
            id: entry.id,
            kind: entry.kind,
            user_id: num(entry.user_id),
            username: typeof entry.username === 'string' ? entry.username : '',
            subject: typeof entry.subject === 'string' ? entry.subject : '',
            detail: typeof entry.detail === 'string' ? entry.detail : '',
            created_at: typeof entry.created_at === 'string' ? entry.created_at : '',
        };
    });
}

/** The administrator's hidden items (offering ids and "permission:<id>"). */
export function decodeHiddenOfferings(raw: unknown): string[] {
    if (!isRecord(raw)) throw new Error('shape');
    return strings(raw.hidden_offerings);
}

// ---- names ------------------------------------------------------------------

const COMPONENT_NAMES: Record<string, string> = {
    'php-fpm': 'PHP',
    node: 'Node.js',
    mariadb: 'MariaDB',
    mysql: 'MySQL',
    postgresql: 'PostgreSQL',
    roundcube: 'Roundcube',
    redis: 'Redis',
    memcached: 'Memcached',
};

const ACME_NAMES: Record<string, string> = {
    letsencrypt: 'Let’s Encrypt',
    zerossl: 'ZeroSSL',
    buypass: 'Buypass',
    google: 'Google Trust Services',
};

/** A software item's name: product names stay as they are in every language. */
export function softwareName(id: string): string {
    const parts = id.split(':');
    if (parts[0] === 'component') {
        const base = COMPONENT_NAMES[parts[1]] ?? parts[1];
        return parts[2] ? `${base} ${parts[2]}` : base;
    }
    if (parts[0] === 'integration' && parts[1] === 'acme') return ACME_NAMES[parts[2]] ?? parts[2];
    if (parts[0] === 'extension') return parts[3] ? `${parts[2]} (${parts[1]})` : (parts[2] ?? id);
    if (parts[0] === 'app') return APP_NAMES[parts[1]] ?? parts[1];
    return id;
}

// The catalog applications (alpha.118): product names, the same in every language.
const APP_NAMES: Record<string, string> = {
    wordpress: 'WordPress', prestashop: 'PrestaShop', nextcloud: 'Nextcloud', drupal: 'Drupal',
    joomla: 'Joomla', mediawiki: 'MediaWiki', phpbb: 'phpBB',
    // AI and automation apps (service apps) share the namespace.
    n8n: 'n8n', 'open-webui': 'Open WebUI',
};

/** The group a software item is shown under. */
export function softwareGroup(id: string): string {
    const parts = id.split(':');
    if (parts[0] === 'component') return `component:${parts[1]}`;
    if (parts[0] === 'integration') return `integration:${parts[1]}`;
    return parts[0] ?? id;
}

/** A group's heading: a product name, or a word the caller translates. */
export function softwareGroupName(group: string): { product?: string; key?: 'acme' | 'extension' | 'app' } {
    const [kind, name] = group.split(':');
    if (kind === 'component') return { product: COMPONENT_NAMES[name] ?? name };
    if (kind === 'integration' && name === 'acme') return { key: 'acme' };
    if (kind === 'app') return { key: 'app' };
    return { key: 'extension' };
}

const versionParts = (id: string) => (id.split(':')[2] ?? '').split('.').map((part) => Number(part) || 0);

/** Software items grouped by component, newest version first within a group. */
export function groupSoftware<T extends { id: string }>(items: T[]): { group: string; items: T[] }[] {
    const groups = new Map<string, T[]>();
    for (const item of items) {
        const group = softwareGroup(item.id);
        groups.set(group, [...(groups.get(group) ?? []), item]);
    }
    const order = (group: string) => {
        const known = ['component:php-fpm', 'component:node', 'component:mariadb', 'component:postgresql', 'integration:acme'];
        const at = known.indexOf(group);
        return at === -1 ? known.length : at;
    };
    return [...groups.entries()]
        .sort(([a], [b]) => order(a) - order(b) || a.localeCompare(b))
        .map(([group, list]) => ({
            group,
            items: [...list].sort((a, b) => {
                const [x, y] = [versionParts(a.id), versionParts(b.id)];
                for (let i = 0; i < Math.max(x.length, y.length); i++) {
                    if ((y[i] ?? 0) !== (x[i] ?? 0)) return (y[i] ?? 0) - (x[i] ?? 0);
                }
                return a.id.localeCompare(b.id);
            }),
        }));
}

export const permissionOf = (item: string): string | null => (item.startsWith('permission:') ? item.slice('permission:'.length) : null);

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** One rights item in words: a permission by its plain label, software by its name. */
export function itemLabel(item: string, t: T): string {
    const permission = permissionOf(item);
    if (permission !== null) {
        const key = `rights.perm.${permission}` as TranslationKey;
        const text = t(key);
        return text === key ? permission : text;
    }
    return softwareName(item);
}

/** The state as every screen names it: Unlimited, Plan: <name>, or Custom. */
export function stateName(state: string, planName: string | undefined, t: T, reseller = false): string {
    if (state === 'unlimited') return t(reseller ? 'rights.state.unlimitedReseller' : 'rights.state.unlimited');
    if (state === 'plan' || state.startsWith('plan:')) return t('rights.state.plan', { name: planName ?? '' });
    return t('rights.state.custom');
}

// ---- the set being edited ---------------------------------------------------

export interface RightsSelection {
    permissions: string[];
    software: string[];
}

/**
 * What a set opens, as a selection over the grantor's catalogue: an "everything
 * offered" part selects every item the grantor may give.
 */
export function selectionOf(own: RightsPayload, grantor: RightsView): RightsSelection {
    const grantable = (items: RightsItem[]) => items.filter((item) => item.granted).map((item) => item.id);
    return {
        permissions: own.all_permissions ? grantable(grantor.permissions) : [...own.permissions],
        software: own.all_software ? grantable(grantor.software) : [...own.software],
    };
}

/** The payload a selection is saved as: items chosen one by one. */
export function payloadOf(selection: RightsSelection): RightsPayload {
    return {
        all_permissions: false,
        all_software: false,
        permissions: PERMISSION_IDS.filter((id) => selection.permissions.includes(id)),
        software: [...new Set(selection.software)].sort(),
    };
}

/** Whether the grantor may hand out an item now (only these are selectable). */
export function grantable(grantor: RightsView, kind: 'permissions' | 'software', id: string): boolean {
    return grantor.unrestricted || grantor[kind].some((item) => item.id === id && item.granted);
}

/** Items of a selection the grantor may not hand out (kept visible, not selectable). */
export function outsideGrantor(selection: RightsSelection, grantor: RightsView): string[] {
    return [
        ...selection.permissions.filter((id) => !grantable(grantor, 'permissions', id)).map((id) => `permission:${id}`),
        ...selection.software.filter((id) => !grantable(grantor, 'software', id)),
    ];
}

export function toggle(list: string[], id: string, on: boolean): string[] {
    return on ? (list.includes(id) ? list : [...list, id]) : list.filter((item) => item !== id);
}

/** 0 is unlimited; a negative or broken number is 0. */
export function quantity(text: string): number {
    const n = Number.parseInt(text, 10);
    return Number.isFinite(n) && n > 0 ? n : 0;
}

// ---- notices ----------------------------------------------------------------

export const MIGRATION_NOTICE = 'unlimited_after_migration';
export const POOL_NOTICES = ['reseller_pool_80', 'reseller_pool_100'];
/** alpha.118 (D-044): traffic is measured now; every traffic limit started as unlimited. */
export const TRAFFIC_MEASURED_NOTICE = 'traffic_measured';

// ---- sending a change ---------------------------------------------------------

export type SendResult =
    | { kind: 'done'; body: unknown }
    | { kind: 'refused'; error: ApiError; preview: RightsPreview | null; status: number }
    | { kind: 'lost' };

/**
 * Sends one change. A refusal keeps its preview when the server sent one (a
 * confirmation the server asks for); an answer that did not arrive is "lost":
 * it is not known whether the change was made, and nothing is sent again.
 */
export async function sendRights(url: string, method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', body?: unknown): Promise<SendResult> {
    let res: Response;
    try {
        res = await fetch(url, {
            method,
            headers: { 'Content-Type': 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
    } catch {
        return { kind: 'lost' };
    }
    if (!res.ok) {
        let text = '';
        try {
            text = await res.text();
        } catch {
            text = '';
        }
        let preview: RightsPreview | null = null;
        try {
            const parsed = JSON.parse(text) as { preview?: unknown };
            if (parsed && isRecord(parsed.preview)) preview = decodePreview(parsed.preview);
        } catch {
            preview = null;
        }
        const error = await readApiError(new Response(text, { status: res.status }));
        return { kind: 'refused', error, preview, status: res.status };
    }
    let answer: unknown = null;
    try {
        answer = await res.json();
    } catch {
        // The change was made (2xx); only its answer could not be read.
        answer = null;
    }
    return { kind: 'done', body: answer };
}

// ---- numbers, plans and the account form (pure; the screens share them) -----

/** The numbers of a set: a subscription's limits, or a reseller's pool with customers. */
export type Numbers = PoolNumbers;

type Say = T;

export const NO_LIMITS: Numbers = { max_customers: 0, max_domains: 0, max_databases: 0, max_email_accounts: 0, disk_quota_mb: 0, traffic_quota_mb: 0 };

/** A limit in words: the number, or "Unlimited". */
export function limitWords(n: number, t: Say, unit = ''): string {
    if (n <= 0) return t('rights.unlimitedShort');
    if (unit === 'MB') return n >= 1024 ? `${(n / 1024).toFixed(n % 1024 === 0 ? 0 : 1)} GB` : `${n} MB`;
    return String(n);
}

const QTY_KINDS: Record<string, TranslationKey> = {
    domains: 'rights.qty.domains',
    databases: 'rights.qty.databases',
    mailboxes: 'rights.qty.mailboxes',
    'email accounts': 'rights.qty.mailboxes',
    disk: 'rights.qty.disk',
    traffic: 'rights.qty.traffic',
    customers: 'rights.qty.customers',
};

/**
 * A line the server wrote about usage above a limit ("domains: 5 in use, limit
 * 3", with an account in front of it) in the reader's words. A line in any
 * other form is shown as the server wrote it.
 */
export function overLimitText(line: string, t: Say): string {
    const match = /^(?:(.*?) - |reseller #(\d+) )?(domains|databases|mailboxes|disk|traffic|customers): (\d+)( MB)? in use, (?:limit|pool) (\d+)( MB)?$/.exec(line.trim());
    if (!match) return line;
    const [, account, reseller, kind, used, usedUnit, limit] = match;
    const what = t(QTY_KINDS[kind]);
    const unit = usedUnit ? ' MB' : '';
    const sentence = t('rights.over.line', { what, used: used + unit, limit: limit + unit });
    if (account) return t('rights.over.account', { account, sentence });
    if (reseller) return t('rights.over.reseller', { id: reseller, sentence });
    return sentence;
}

const IN_USE_WHAT: Record<string, TranslationKey> = {
    sites: 'rights.inUse.sites',
    databases: 'rights.inUse.databases',
    mailboxes: 'rights.inUse.mailboxes',
    'VPN devices': 'rights.inUse.vpn',
};

export function inUseText(line: InUseLine, t: Say): string {
    const what = IN_USE_WHAT[line.what];
    return t('rights.inUse.line', {
        item: itemLabel(line.item, t),
        used: what ? t(what, { n: line.count }) : `${line.count} ${line.what}`,
    });
}


/** A short summary of a set's permission and software parts. */
export function rightsSummary(rights: { all_permissions: boolean; all_software: boolean; permissions: string[]; software: string[] } | undefined, t: Say): string {
    if (!rights) return t('rights.summary.everything');
    if (rights.all_permissions && rights.all_software) return t('rights.summary.everything');
    const permissions = rights.all_permissions ? t('rights.summary.allPermissions') : t('rights.summary.permissions', { n: rights.permissions.length });
    const software = rights.all_software ? t('rights.summary.allSoftware') : t('rights.summary.software', { n: rights.software.length });
    return `${permissions} · ${software}`;
}

export type PlanKind = 'customer' | 'reseller';

export interface EditedPlan {
    id: number;
    name: string;
    numbers: Numbers;
    bandwidth: number;
    /** The plan's own set as stored, to tell an untouched "everything offered" apart. */
    rights?: ServicePlan['rights'];
}

/** A new plan starts with modest limits; 0 is unlimited. */
export const NEW_PLAN: EditedPlan = {
    id: 0,
    name: '',
    numbers: { max_customers: 0, max_domains: 5, max_databases: 10, max_email_accounts: 50, disk_quota_mb: 10240, traffic_quota_mb: 0 },
    bandwidth: 0,
};

export function editedFromCustomerPlan(plan: ServicePlan): EditedPlan {
    return {
        id: plan.id,
        name: plan.name,
        numbers: { max_customers: 0, max_domains: plan.max_domains, max_databases: plan.max_databases, max_email_accounts: plan.max_email_accounts, disk_quota_mb: plan.disk_quota_mb, traffic_quota_mb: plan.bandwidth_quota_mb ?? 0 },
        bandwidth: plan.bandwidth_quota_mb ?? 0,
        rights: plan.rights,
    };
}

export function editedFromResellerPlan(plan: ResellerPlan): EditedPlan {
    return {
        id: plan.id,
        name: plan.name,
        numbers: { max_customers: plan.max_customers, max_domains: plan.max_domains, max_databases: plan.max_databases, max_email_accounts: plan.max_email_accounts, disk_quota_mb: plan.disk_quota_mb, traffic_quota_mb: plan.traffic_quota_mb ?? 0 },
        bandwidth: 0,
        rights: plan.rights,
    };
}

/** The body a plan is saved with. Rights go only when they were chosen. */
export function planBody(kind: PlanKind, plan: EditedPlan, selection: RightsSelection | null, flags: { narrowing?: boolean; overUsage?: boolean } = {}): Record<string, unknown> {
    const { max_customers, traffic_quota_mb, ...limits } = plan.numbers;
    // A customer plan keeps its traffic limit in bandwidth_quota_mb (D-044).
    const body: Record<string, unknown> = kind === 'reseller'
        ? { name: plan.name.trim(), max_customers, ...limits, traffic_quota_mb }
        : { name: plan.name.trim(), ...limits, bandwidth_quota_mb: traffic_quota_mb };
    if (selection) body.rights = payloadOf(selection);
    if (flags.narrowing) body.confirm_narrowing = true;
    if (flags.overUsage && kind === 'customer') body.accept_over_usage = true;
    return body;
}

/** The plans an account may be given: global ones, and its reseller's own. */
export function assignablePlans(plans: ServicePlan[], user: { parent_id?: number }, isAdmin: boolean): ServicePlan[] {
    return plans.filter((plan) => plan.owner_id === undefined || plan.owner_id === null || (!isAdmin ? true : plan.owner_id === user.parent_id));
}

export interface NewAccountRights {
    state: RightsState;
    planID: number;
    selection: RightsSelection;
    numbers: Numbers;
}

export const UNLIMITED_CHOICE: NewAccountRights = { state: 'unlimited', planID: 0, selection: { permissions: [], software: [] }, numbers: NO_LIMITS };

/** The `rights` of POST /api/v1/users. */
export function newAccountRightsBody(choice: NewAccountRights, role: string): Record<string, unknown> {
    if (choice.state === 'unlimited') return { state: 'unlimited' };
    if (choice.state === 'plan') return { state: 'plan', plan_id: choice.planID };
    const { max_customers, ...limits } = choice.numbers;
    return { state: 'custom', rights: payloadOf(choice.selection), quantities: role === 'reseller' ? { ...limits, max_customers } : limits };
}

/** What still keeps Create off in the Rights section, in its own words. */
export function newAccountRightsMissing(choice: NewAccountRights, t: Say): string[] {
    return choice.state === 'plan' && choice.planID <= 0 ? [t('rights.choosePlan')] : [];
}

