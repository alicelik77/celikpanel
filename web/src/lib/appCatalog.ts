// The application catalog (alpha.118, docs/APP-CATALOG.md): the decoders and
// addresses the domain's Applications tab and the administrator's offered
// applications share. The server enforces every gate; these screens show and
// ask.
//
// Uygulama kataloğu: alan adının Uygulamalar sekmesi ve yöneticinin sunulan
// uygulamaları ekranı için çözücüler ve adresler.

export const APP_CATALOG_URL = '/api/v1/apps';
export const domainAppsURL = (domainId: number) => `/api/v1/domains/${domainId}/apps`;
export const domainAppInstallURL = (domainId: number) => `/api/v1/domains/${domainId}/apps/install`;
/** Components, opened at PHP-FPM with the install of one PHP version asked for. */
export const phpExtensionsInstallPath = (pkg: string) => `/components?component=php-fpm&install=${encodeURIComponent(pkg)}`;
export const domainAppOperationURL = (domainId: number, operationId: string, action?: 'retry' | 'credentials') =>
    `/api/v1/domains/${domainId}/apps/${operationId}${action ? `/${action}` : ''}`;

export interface CatalogApp {
    id: string;
    kind: string;
    offering_id: string;
    name: string;
    version: string;
    license: string;
    homepage: string;
    description: string;
    source: string;
    pinned: boolean;
    checked: string;
    php: string;
    extensions: string[];
    databases: string[];
    disk_mb: number;
    risk: string;
    admin_user?: string;
    offered: boolean;
    /** The Turkish description, check and risk note (alpha.130); absent from older panels. */
    tr?: CatalogAppTexts;
}

/** One language's copy of an entry's reader texts. */
export interface CatalogAppTexts {
    description: string;
    checked: string;
    risk: string;
}

export interface MissingExtension {
    extension: string;
    package?: string;
    enable?: boolean;
}

export interface AppRequirements {
    /** ok, unmet (a named prerequisite) or unknown (could not be read). */
    state: 'ok' | 'unmet' | 'unknown';
    php_ok: boolean;
    php_reason?: string;
    missing: MissingExtension[];
    doc_root?: string;
    database_ok: boolean;
    database_reason?: string;
    disk_ok: boolean;
    disk_reason?: string;
    /** The site's PHP version line ("8.4"). */
    php_line?: string;
    /** The Components install that brings every missing extension ("php8.4-fpm"). */
    install_package?: string;
    detail?: string;
    /** Why an "unknown" had no answer: busy (no answer in time), refused (the agent refused the check). */
    reason?: 'busy' | 'refused';
}

export interface DomainApp extends CatalogApp {
    available: boolean;
    refused_by?: string;
    grantor?: string;
    requirements?: AppRequirements;
}

export interface InstalledApp {
    operation_id: string;
    app_id: string;
    name: string;
    version: string;
    status: string;
    site_url: string;
    admin_url?: string;
    admin_user?: string;
    database_name?: string;
    data_dir?: string;
    error_kind?: string;
    last_error?: string;
    credentials_pending?: boolean;
    can_retry?: boolean;
    can_remove?: boolean;
    legacy?: boolean;
}

export interface DomainApps {
    apps: DomainApp[];
    installed: InstalledApp[];
    occupied: boolean;
    php_version: string;
    /** The reader is the administrator (an application not offered is offered first). */
    administrator: boolean;
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown) => (typeof v === 'string' ? v : '');
const strings = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []);

function decodeTexts(raw: unknown): CatalogAppTexts | undefined {
    if (!isRecord(raw)) return undefined;
    const texts = { description: str(raw.description), checked: str(raw.checked), risk: str(raw.risk) };
    return texts.description || texts.checked || texts.risk ? texts : undefined;
}

/**
 * The entry's description, check or risk note in the screen's language: the
 * Turkish text on a Turkish screen when the server sent one, else the English
 * text the server always sends.
 */
export function appText(app: CatalogApp, field: keyof CatalogAppTexts, locale: string): string {
    return (locale === 'tr' && app.tr?.[field]) || app[field];
}

function decodeApp(raw: unknown): CatalogApp {
    if (!isRecord(raw) || typeof raw.id !== 'string' || typeof raw.name !== 'string') throw new Error('shape');
    return {
        id: raw.id, kind: str(raw.kind) || 'php-site', offering_id: str(raw.offering_id) || `app:${raw.id}`, name: raw.name,
        version: str(raw.version), license: str(raw.license), homepage: str(raw.homepage), description: str(raw.description),
        source: str(raw.source), pinned: raw.pinned === true, checked: str(raw.checked), php: str(raw.php),
        extensions: strings(raw.extensions), databases: strings(raw.databases),
        disk_mb: typeof raw.disk_mb === 'number' ? raw.disk_mb : 0, risk: str(raw.risk),
        admin_user: str(raw.admin_user) || undefined, offered: raw.offered !== false, tr: decodeTexts(raw.tr),
    };
}

function decodeRequirements(raw: unknown): AppRequirements | undefined {
    if (!isRecord(raw)) return undefined;
    const state = raw.state === 'ok' || raw.state === 'unmet' ? raw.state : 'unknown';
    const missing = Array.isArray(raw.missing) ? raw.missing.filter(isRecord).map((m) => ({
        extension: str(m.extension), package: str(m.package) || undefined, enable: m.enable === true || undefined,
    })) : [];
    return {
        state, php_ok: raw.php_ok === true, php_reason: str(raw.php_reason) || undefined, missing,
        doc_root: str(raw.doc_root) || undefined, database_ok: raw.database_ok !== false,
        database_reason: str(raw.database_reason) || undefined, disk_ok: raw.disk_ok !== false,
        disk_reason: str(raw.disk_reason) || undefined, detail: str(raw.detail) || undefined,
        php_line: str(raw.php_line) || undefined,
        install_package: /^php[0-9]+\.[0-9]+-fpm$/.test(str(raw.install_package)) ? str(raw.install_package) : undefined,
        reason: raw.reason === 'busy' || raw.reason === 'refused' ? raw.reason : undefined,
    };
}

/** GET /api/v1/apps: the catalog (every entry for the administrator). */
export function decodeCatalog(raw: unknown): CatalogApp[] {
    if (!isRecord(raw) || (!Array.isArray(raw.apps) && raw.apps !== null)) throw new Error('shape');
    return ((raw.apps ?? []) as unknown[]).map(decodeApp);
}

/** GET /api/v1/domains/{id}/apps. */
export function decodeDomainApps(raw: unknown): DomainApps {
    if (!isRecord(raw) || (!Array.isArray(raw.apps) && raw.apps !== null)) throw new Error('shape');
    const apps = ((raw.apps ?? []) as unknown[]).map((entry) => {
        const app = decodeApp(entry) as DomainApp;
        const record = entry as Record<string, unknown>;
        app.available = record.available === true;
        app.refused_by = str(record.refused_by) || undefined;
        app.grantor = str(record.grantor) || undefined;
        app.requirements = decodeRequirements(record.requirements);
        return app;
    });
    const installed = (Array.isArray(raw.installed) ? raw.installed : []).filter(isRecord).map((entry) => ({
        operation_id: str(entry.operation_id), app_id: str(entry.app_id), name: str(entry.name) || str(entry.app_id),
        version: str(entry.version), status: str(entry.status), site_url: str(entry.site_url),
        admin_url: str(entry.admin_url) || undefined, admin_user: str(entry.admin_user) || undefined,
        database_name: str(entry.database_name) || undefined, data_dir: str(entry.data_dir) || undefined,
        error_kind: str(entry.error_kind) || undefined, last_error: str(entry.last_error) || undefined,
        credentials_pending: entry.credentials_pending === true, can_retry: entry.can_retry === true,
        can_remove: entry.can_remove === true, legacy: entry.legacy === true,
    }));
    const site = isRecord(raw.site) ? raw.site : {};
    return { apps, installed, occupied: site.occupied === true, php_version: str(site.php_version), administrator: site.administrator === true };
}

/** Steps of an install that is still running (the screen moves while it waits). */
export const RUNNING_STATES = ['reserved', 'database_creating', 'database_ready', 'files_installing', 'files_ready', 'setting_up', 'removing'];
