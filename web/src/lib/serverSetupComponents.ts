import type { ServerSetupDraft, SetupPurpose } from './serverSetup';

// What the wizard does with a catalogue component (alpha.111): the owner
// ticks a "choice"; "required" is in every setup; "automatic" is prepared only
// when needed (cron); "dns" is chosen in the DNS step; "unavailable" has no
// CelikPanel integration yet; "components" installs from the Components page.
export type SetupComponentWizard = 'choice' | 'required' | 'automatic' | 'dns' | 'unavailable' | 'components';
export interface SetupComponentVersion {
    package: string;
    version?: string;
    source: 'distribution' | 'vendor';
    available: boolean;
    reason?: string;
}
export interface SetupComponent {
    id: string;
    name: string;
    category: string;
    dependencies: string[];
    conflicts: string[];
    supported: boolean;
    installed: boolean;
    reason?: string;
    // Additive (alpha.111); an older catalogue has none of them.
    wizard?: SetupComponentWizard;
    installed_version?: string;
    candidate_version?: string;
    repo?: { id: string; name: string; required?: boolean; state: 'enabled' | 'disabled' | 'unknown' };
    versions?: SetupComponentVersion[];
}
export interface SetupDNSEngineChoice {
    id: 'bind' | 'pdns';
    role: 'main' | 'backup';
    offered: boolean;
    reason?: string;
    installed_version?: string;
    candidate_version?: string;
}
export interface SetupComponentCatalog {
    version: 1;
    inventory_state: 'ready' | 'unknown';
    presets: Record<SetupPurpose, string[]>;
    required_components: string[];
    components: SetupComponent[];
    dns_engines?: SetupDNSEngineChoice[];
    host?: { distro_id?: string; version_id?: string };
}
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const ids = (value: unknown): value is string[] => Array.isArray(value) && value.length <= 80
    && value.every(id => typeof id === 'string' && /^[a-z][a-z0-9-]{0,63}$/.test(id)) && new Set(value).size === value.length;

export function decodeSetupComponentCatalog(value: unknown): SetupComponentCatalog | null {
    if (!isRecord(value) || value.version !== 1 || !['ready', 'unknown'].includes(String(value.inventory_state))
        || !isRecord(value.presets) || !ids(value.required_components) || !Array.isArray(value.components)
        || value.components.length > 80 || value.components.length === 0) return null;
    for (const item of value.components) {
        if (!isRecord(item) || !ids([item.id]) || typeof item.name !== 'string' || !item.name.trim() || item.name.length > 120
            || typeof item.category !== 'string' || item.category.length > 40 || !ids(item.dependencies) || !ids(item.conflicts)
            || typeof item.supported !== 'boolean' || typeof item.installed !== 'boolean'
            || (item.reason !== undefined && (typeof item.reason !== 'string' || item.reason.length > 300))) return null;
        if (item.wizard !== undefined && !['choice', 'required', 'automatic', 'dns', 'unavailable', 'components'].includes(String(item.wizard))) return null;
        for (const key of ['installed_version', 'candidate_version'] as const) {
            if (item[key] !== undefined && (typeof item[key] !== 'string' || (item[key] as string).length > 64)) return null;
        }
        const repo = item.repo;
        if (repo !== undefined && (!isRecord(repo) || typeof repo.id !== 'string' || typeof repo.name !== 'string' || repo.name.length > 120
            || !['enabled', 'disabled', 'unknown'].includes(String(repo.state)))) return null;
        if (item.versions !== undefined && (!Array.isArray(item.versions) || item.versions.length > 24 || item.versions.some(version => !isRecord(version)
            || typeof version.package !== 'string' || version.package.length > 64 || !['distribution', 'vendor'].includes(String(version.source))
            || typeof version.available !== 'boolean' || (version.version !== undefined && typeof version.version !== 'string')
            || (version.reason !== undefined && typeof version.reason !== 'string')))) return null;
    }
    if (value.dns_engines !== undefined && (!Array.isArray(value.dns_engines) || value.dns_engines.length > 8 || value.dns_engines.some(engine => !isRecord(engine)
        || !['bind', 'pdns'].includes(String(engine.id)) || !['main', 'backup'].includes(String(engine.role)) || typeof engine.offered !== 'boolean'
        || (engine.reason !== undefined && typeof engine.reason !== 'string')))) return null;
    const rows = value.components as SetupComponent[];
    const known = new Set(rows.map(row => row.id));
    if (known.size !== rows.length || value.required_components.some(id => !known.has(id))) return null;
    if (rows.some(row => row.dependencies.some(id => !known.has(id)))) return null;
    // Rows written before alpha.111 carry no wizard role: a required one is
    // required, every other one was a choice.
    for (const row of rows) row.wizard ??= value.required_components.includes(row.id) ? 'required' : 'choice';
    for (const purpose of ['web', 'web_mail', 'application', 'dns', 'custom']) {
        const preset = value.presets[purpose];
        if (!ids(preset) || preset.some(id => !known.has(id))) return null;
    }
    return value as unknown as SetupComponentCatalog;
}

export function setupComponentClosure(requested: string[], catalog: SetupComponentCatalog): Set<string> {
    const rows = new Map(catalog.components.map(row => [row.id, row]));
    const found = new Set<string>();
    const visit = (id: string) => {
        if (found.has(id)) return;
        found.add(id);
        rows.get(id)?.dependencies.forEach(visit);
    };
    requested.forEach(visit);
    return found;
}

// Mutually dependent services form one removable choice. For example, the
// supported mail lifecycle prepares Postfix and Dovecot together; treating
// each as a disabled dependency of the other would trap the user's selection.
export function setupComponentGroups(catalog: SetupComponentCatalog): SetupComponent[][] {
    const reach = new Map(catalog.components.map(row => [row.id, setupComponentClosure([row.id], catalog)]));
    const grouped = new Set<string>();
    return catalog.components.flatMap(row => {
        if (grouped.has(row.id)) return [];
        const group = catalog.components.filter(other => reach.get(row.id)!.has(other.id) && reach.get(other.id)!.has(row.id));
        group.forEach(other => grouped.add(other.id));
        return [group];
    });
}

export function setupPresetComponents(draft: ServerSetupDraft, catalog: SetupComponentCatalog): string[] {
    let selected = [...catalog.presets[draft.purpose]];
    if (draft.purpose === 'application') {
        selected = selected.filter(id => !['mariadb', 'postgresql'].includes(id));
        if (draft.database) selected.push(draft.database);
    }
    return selected;
}

export function setupEffectiveComponents(draft: ServerSetupDraft, catalog: SetupComponentCatalog | null): Set<string> {
    if (draft.customization) return catalog ? setupComponentClosure(draft.customization.components, catalog) : new Set(draft.customization.components);
    if (catalog) return setupComponentClosure(setupPresetComponents(draft, catalog), catalog);
    if (draft.purpose === 'web_mail') return new Set(['nginx', 'php-fpm', 'mariadb', 'postfix', 'dovecot', 'roundcube', 'rspamd']);
    if (draft.purpose === 'application') return new Set(['nginx', 'node', ...(draft.database ? [draft.database] : [])]);
    return new Set(draft.purpose === 'web' ? ['nginx', 'php-fpm', 'mariadb'] : []);
}

// Roles (alpha.111): "Web siteleri", "E-posta" and "DNS" can be combined and
// preselect the components they need. Web is the web preset; mail is what the
// web-and-mail preset adds to it. DNS is not a component: it is local DNS.
// Gorevler birlesebilir ve gereken bilesenleri onceden secer.
export interface SetupRoles { web: boolean; mail: boolean; dns: boolean }
export function setupRolePreset(role: 'web' | 'mail', catalog: SetupComponentCatalog): string[] {
    const web = catalog.presets.web;
    return role === 'web' ? [...web] : catalog.presets.web_mail.filter(id => !web.includes(id));
}
export function setupRolesPreset(roles: SetupRoles, catalog: SetupComponentCatalog): string[] {
    const ids: string[] = [];
    for (const role of ['web', 'mail'] as const) {
        if (roles[role]) for (const id of setupRolePreset(role, catalog)) if (!ids.includes(id)) ids.push(id);
    }
    return ids;
}
export function setupDraftRoles(draft: ServerSetupDraft, catalog: SetupComponentCatalog | null): SetupRoles {
    const selected = setupEffectiveComponents(draft, catalog);
    return { web: selected.has('nginx') && (!!draft.customization || draft.purpose !== 'dns'), mail: selected.has('postfix'), dns: draft.dns_mode === 'local' };
}
const sameIDs = (left: string[], right: string[]) => left.length === right.length && left.every(id => right.includes(id));
// Customized: the owner chose components or a version beyond the roles'
// recommended set.
export function setupCustomized(draft: ServerSetupDraft, catalog: SetupComponentCatalog | null): boolean {
    if (!draft.customization) return false;
    if (Object.keys(draft.customization.versions || {}).length > 0) return true;
    if (!catalog) return true;
    return !sameIDs(draft.customization.components, setupRolesPreset(setupDraftRoles(draft, catalog), catalog));
}
// Applies the owner's roles: the selection is always explicit (purpose
// "custom"). A customized selection gains or loses only the toggled role's
// components. Unticking DNS keeps the list at an external provider.
export function applySetupRoles(draft: ServerSetupDraft, roles: SetupRoles, catalog: SetupComponentCatalog): ServerSetupDraft {
    const before = setupDraftRoles(draft, catalog);
    const next: ServerSetupDraft = { ...draft, purpose: 'custom', dns_mode: roles.dns ? 'local' : draft.dns_mode === 'local' ? 'external' : draft.dns_mode };
    if (!roles.dns) { delete next.infrastructure_dns; delete next.main_dns_server; }
    if (!setupCustomized(draft, catalog) || !draft.customization) {
        next.customization = { components: setupRolesPreset(roles, catalog) };
        return next;
    }
    let components = [...draft.customization.components];
    for (const role of ['web', 'mail'] as const) {
        if (roles[role] && !before[role]) for (const id of setupRolePreset(role, catalog)) if (!components.includes(id)) components.push(id);
        if (!roles[role] && before[role]) {
            const kept = setupRolesPreset({ ...roles, [role]: false }, catalog);
            components = components.filter(id => kept.includes(id) || !setupRolePreset(role, catalog).includes(id));
        }
    }
    const versions = Object.fromEntries(Object.entries(draft.customization.versions || {}).filter(([id]) => setupComponentClosure(components, catalog).has(id)));
    next.customization = { components, ...(Object.keys(versions).length ? { versions } : {}) };
    return next;
}
// The recommended set again, dropping explicit choices and versions.
export function resetSetupCustomization(draft: ServerSetupDraft, catalog: SetupComponentCatalog): ServerSetupDraft {
    return { ...draft, purpose: 'custom', customization: { components: setupRolesPreset(setupDraftRoles(draft, catalog), catalog) } };
}
