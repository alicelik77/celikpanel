// Service apps (alpha.118, docs/AI-APPS.md): the shapes the domain's
// "AI & automation" tab and the administrator's offering read, and the
// decoders that refuse an answer of another shape.
//
// Hizmet uygulamaları: alan adının "Yapay zekâ ve otomasyon" sekmesinin ve
// yöneticinin sunma ekranının okuduğu biçimler.

export interface ServiceAppRisk {
    can_do: string[];
    disabled: string[];
    network: string[];
    remains: string[];
}

export interface ServiceAppCatalogEntry {
    id: string;
    name: string;
    vendor: string;
    homepage: string;
    source: string;
    licence: string;
    licence_note?: string;
    runtime: string;
    version: string;
    deferred?: string;
    customer_offerable: boolean;
    /** alpha.118: closed to resellers and their customers by its licence (a code). */
    reseller_closed?: string;
    defaults: { memory_mb: number; cpu_percent: number; tasks_max: number; disk_mb: number };
    risk: ServiceAppRisk;
    secrets: { key: string; label: string }[] | null;
    owner_login: boolean;
    available: boolean;
    reason?: string;
    detail?: string;
    held: boolean;
}

export interface DomainServiceAppEntry extends ServiceAppCatalogEntry {
    offered: boolean;
    refused_by?: string;
    grantor?: string;
}

export interface ServiceAppOperation {
    id: string;
    install_id: number;
    kind: 'install' | 'update' | 'uninstall';
    state: 'running' | 'succeeded' | 'failed';
    step: string;
    class?: string;
    reason?: string;
    message?: string;
    remove_data?: boolean;
}

export interface ServiceAppInstall {
    id: number;
    app_id: string;
    name: string;
    version: string;
    target_version: string;
    state: string;
    url: string;
    update_available?: string;
    held: boolean;
    active: boolean;
    enabled: boolean;
    active_state?: string;
    memory_mb?: number;
    serving: boolean;
    exposed?: string[];
    unit_file?: string;
    status_unknown?: boolean;
    unit: string;
    folder: string;
}

export interface DomainServiceAppsView {
    domain: string;
    suspended: boolean;
    has_site: boolean;
    project_type: string;
    certificate: boolean;
    permission_granted: boolean;
    permission_refused_by?: string;
    permission_grantor?: string;
    availability_known: boolean;
    apps: DomainServiceAppEntry[];
    install: ServiceAppInstall | null;
    operation: ServiceAppOperation | null;
}

export interface AdminServiceAppEntry extends ServiceAppCatalogEntry {
    risk_digest: string;
    offered: boolean;
    offer_stale: boolean;
    held_reason?: string;
    limits: { memory_mb: number; cpu_percent: number; tasks_max: number };
    runtime_source?: string;
    installs: number;
    running: number;
}

const isObject = (raw: unknown): raw is Record<string, unknown> => !!raw && typeof raw === 'object' && !Array.isArray(raw);

export function decodeDomainServiceApps(raw: unknown): DomainServiceAppsView {
    if (!isObject(raw) || !Array.isArray(raw.apps) || typeof raw.permission_granted !== 'boolean' || typeof raw.domain !== 'string') {
        throw new Error('shape');
    }
    return raw as unknown as DomainServiceAppsView;
}

export function decodeAdminServiceApps(raw: unknown): AdminServiceAppEntry[] {
    if (!isObject(raw) || !Array.isArray(raw.apps)) throw new Error('shape');
    return raw.apps as AdminServiceAppEntry[];
}

/** A new operation identity; the server answers a repeat with the same operation. */
export function newOperationID(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return 'op-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}
