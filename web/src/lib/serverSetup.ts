export const setupPurposes = ['web', 'web_mail', 'application', 'dns', 'custom'] as const;
export type SetupPurpose = typeof setupPurposes[number];
export type SetupDNSMode = 'local' | 'existing' | 'external';
export interface ServerSetupDraft {
    purpose: SetupPurpose;
    // versions (alpha.111): component id -> the exact vendor package chosen
    // instead of the distribution's version; absent means the distribution's.
    customization?: { components: string[]; versions?: Record<string, string> };
    infrastructure_dns?: { zone: string; peer_panel_domain?: string; peer_mail_hostname?: string };
    panel_domain: string;
    mail_hostname: string;
    dns_mode: SetupDNSMode;
    remote_dns_connection_id: string;
    dns_publisher_endpoint?: string;
    dns_hosting_management?: 'manual' | 'panel';
    dns_engine: 'bind' | 'pdns';
    dns_role: 'primary' | 'secondary';
    ns1: string;
    ns2: string;
    local_ip: string;
    peer_ip: string;
    peer_ns: string;
    node_version: string;
    database: string;
    // Setup wizard v2 answers (alpha.111), all optional: a draft saved before
    // them has none and keeps its reviewed plan.
    main_domain?: string;
    server_count?: 1 | 2;
    short_name?: string;
    // 'this' or, with two servers, 'peer': the second server is the main DNS
    // server (alpha.114, Option A); this server is then the backup.
    main_dns_server?: 'this' | 'peer';
    // alpha.112: the main domain's public records the owner reviewed, each
    // imported or skipped, and the mail DNS choice.
    imported_records?: SetupImportedRecord[];
    mail_dns?: { mx?: 'now' | 'later' | ''; dmarc?: 'keep' | 'add' | '' };
    // alpha.114: the other server's main-domain mail, taken from the join.
    peer_mail?: { hostname: string; mx?: string; dmarc?: string };
}
export interface SetupImportedRecord {
    name: string;
    type: 'A' | 'AAAA' | 'CNAME' | 'MX' | 'TXT' | 'CAA';
    content: string;
    priority?: number;
    decision: 'import' | 'skip';
}
export interface ServerSetupCheck {
    id: string;
    state: 'ready' | 'action_required' | 'unknown';
    code: string;
    // Additive (2026-10-10): the typed cause and the observed values it names.
    reason?: string;
    vars?: Record<string, string>;
    // alpha.112: an open task setup does not wait for (reverse DNS at the
    // server provider). / Kurulumun beklemedigi acik is.
    advisory?: boolean;
}
export interface ServerSetupSnapshot {
    version: number;
    revision: number;
    origin: 'fresh' | 'legacy';
    status: 'new' | 'legacy' | 'draft' | 'running' | 'waiting' | 'failed' | 'ready';
    required: boolean;
    guidance?: 'undecided' | 'guided' | 'manual';
    draft: ServerSetupDraft;
    checks: ServerSetupCheck[];
    server_ip?: string;
    // Additive (2026-10-16): a completed setup being extended keeps its
    // completion record; extension_base is the draft as it was completed.
    extension?: boolean;
    extension_base?: ServerSetupDraft;
    // alpha.111: the machine host name's first label when it reads like a
    // name a person chose; absent otherwise. Never a guess of the panel's.
    short_name_suggestion?: string;
    // alpha.112: the server's public IPv6 address, when it has one.
    server_ipv6?: string;
}
const draftStrings = ['remote_dns_connection_id', 'panel_domain', 'mail_hostname', 'ns1', 'ns2', 'local_ip', 'peer_ip', 'peer_ns', 'node_version', 'database'] as const;
function record(value: unknown): value is Record<string, unknown> {
    return !!value && typeof value === 'object' && !Array.isArray(value);
}
function validDraft(draft: unknown): draft is Record<string, unknown> {
    if (!record(draft)) return false;
    if (!setupPurposes.includes(draft.purpose as SetupPurpose)
        || !['local', 'existing', 'external'].includes(String(draft.dns_mode))
        || !['bind', 'pdns'].includes(String(draft.dns_engine))
        || !['primary', 'secondary'].includes(String(draft.dns_role))
        || draftStrings.some(key => typeof draft[key] !== 'string')) return false;
    const infrastructure = draft.infrastructure_dns;
    if (infrastructure !== undefined && (!record(infrastructure)
        || typeof infrastructure.zone !== 'string' || infrastructure.zone.length > 253
        || (infrastructure.peer_panel_domain !== undefined && (typeof infrastructure.peer_panel_domain !== 'string' || infrastructure.peer_panel_domain.length > 253))
        || (infrastructure.peer_mail_hostname !== undefined && (typeof infrastructure.peer_mail_hostname !== 'string' || infrastructure.peer_mail_hostname.length > 253)))) return false;
    if (draft.dns_hosting_management !== undefined && !['manual', 'panel'].includes(String(draft.dns_hosting_management))) return false;
    if (draft.dns_publisher_endpoint !== undefined && typeof draft.dns_publisher_endpoint !== 'string') return false;
    const customization = draft.customization;
    if (customization !== undefined && (!record(customization) || !Array.isArray(customization.components)
        || customization.components.length > 80 || customization.components.some(id => typeof id !== 'string' || !/^[a-z][a-z0-9-]{0,63}$/.test(id))
        || new Set(customization.components).size !== customization.components.length)) return false;
    const versions = record(customization) ? customization.versions : undefined;
    if (versions !== undefined && (!record(versions) || Object.keys(versions).length > 8
        || Object.entries(versions).some(([id, pkg]) => !/^[a-z][a-z0-9-]{0,63}$/.test(id) || typeof pkg !== 'string' || pkg.length > 64))) return false;
    if (draft.main_domain !== undefined && (typeof draft.main_domain !== 'string' || draft.main_domain.length > 253)) return false;
    if (draft.server_count !== undefined && draft.server_count !== 1 && draft.server_count !== 2) return false;
    if (draft.short_name !== undefined && (typeof draft.short_name !== 'string' || draft.short_name.length > 63)) return false;
    if (draft.main_dns_server !== undefined && draft.main_dns_server !== 'this' && draft.main_dns_server !== 'peer') return false;
    if (draft.peer_mail !== undefined && (!record(draft.peer_mail) || typeof draft.peer_mail.hostname !== 'string' || draft.peer_mail.hostname.length > 253)) return false;
    const imported = draft.imported_records;
    if (imported !== undefined && (!Array.isArray(imported) || imported.length > 64 || imported.some(item => !record(item)
        || typeof item.name !== 'string' || !['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'CAA'].includes(String(item.type))
        || typeof item.content !== 'string' || item.content.length > 2048 || !['import', 'skip'].includes(String(item.decision))))) return false;
    if (draft.mail_dns !== undefined && (!record(draft.mail_dns) || !['', 'now', 'later', undefined].includes(draft.mail_dns.mx as string)
        || !['', 'keep', 'add', undefined].includes(draft.mail_dns.dmarc as string))) return false;
    return true;
}
export function decodeServerSetup(value: unknown): ServerSetupSnapshot | null {
    if (!record(value) || value.version !== 1 || !Number.isSafeInteger(value.revision) || (value.revision as number) < 0
        || !['fresh', 'legacy'].includes(String(value.origin))
        || !['new', 'legacy', 'draft', 'running', 'waiting', 'failed', 'ready'].includes(String(value.status))
        || typeof value.required !== 'boolean' || !Array.isArray(value.checks)) return null;
    if (!validDraft(value.draft)) return null;
    if (value.extension !== undefined && typeof value.extension !== 'boolean') return null;
    if (value.extension_base !== undefined && (value.extension !== true || !validDraft(value.extension_base))) return null;
    if (value.checks.some(check => !record(check) || typeof check.id !== 'string'
        || !['ready', 'action_required', 'unknown'].includes(String(check.state)) || typeof check.code !== 'string')) return null;
    if (value.server_ip !== undefined && typeof value.server_ip !== 'string') return null;
    if (value.short_name_suggestion !== undefined && (typeof value.short_name_suggestion !== 'string' || value.short_name_suggestion.length > 63)) return null;
    if (value.status === 'ready' && value.required) return null;
    if (value.extension === true && (value.status === 'ready' || value.required)) return null;
    if (value.guidance !== undefined && !['undecided', 'guided', 'manual'].includes(String(value.guidance))) return null;
    return value as unknown as ServerSetupSnapshot;
}
// Pages that stay open while setup is required, so the owner can reach what
// fixes it. The four pages that were Settings sections until v0.1.0-alpha.98
// (D-037), and Server setup since v0.1.0-alpha.100, stay reachable as they
// were under /settings.
// Kurulum gerekirken açık kalan sayfalar. alpha.98'e dek Ayarlar bölümü olan
// dört sayfa ve alpha.100'den beri Sunucu kurulumu, /settings altındayken
// olduğu gibi ulaşılabilir kalır.
const SETUP_RECOVERY_PAGES = new Set(['/settings', '/account', '/security', '/dns-infrastructure', '/maintenance', '/server-setup', '/services']);
export function isSetupRecoveryPath(pathname: string): boolean {
    return SETUP_RECOVERY_PAGES.has(pathname) || pathname.startsWith('/services/');
}
export function shouldOpenServerSetup(snapshot: ServerSetupSnapshot, pathname: string): boolean {
    // Extending a completed setup never routes the owner into setup: the
    // server stays prepared and the Dashboard notice links to the extension.
    if (snapshot.extension) return false;
    const active = snapshot.status === 'running' || snapshot.status === 'waiting';
    const needsChoice = snapshot.guidance === 'undecided' && snapshot.status !== 'ready';
    return (active || needsChoice || (snapshot.required && snapshot.guidance !== 'manual'))
        && pathname !== '/setup' && !isSetupRecoveryPath(pathname);
}
export function setupNextPath(purpose: SetupPurpose, selected?: ReadonlySet<string>): string {
    if (selected) {
        if (['nginx', 'node', 'phpmyadmin', 'phppgadmin', 'roundcube'].some(id => selected.has(id))) return '/domains';
        if (selected.size === 0) return '/dns-infrastructure';
        return '/services';
    }
    return purpose === 'dns' ? '/dns-infrastructure' : purpose === 'custom' ? '/services' : '/domains';
}
export function chooseSetupPurpose(draft: ServerSetupDraft, purpose: SetupPurpose): ServerSetupDraft {
    return {
        ...draft, purpose,
        customization: purpose === 'custom' ? { components: [] } : undefined,
        dns_mode: purpose === 'dns' ? 'local' : draft.dns_mode,
        database: purpose === 'dns' || purpose === 'custom' ? '' : draft.database || 'mariadb',
    };
}

// Slot names are the setup protocol's primary/secondary order, not hostname prefixes.
export function setupDNSNames(draft: ServerSetupDraft) {
    const localKey = draft.dns_role === 'primary' ? 'ns1' : 'ns2';
    const peerKey = localKey === 'ns1' ? 'ns2' : 'ns1';
    const canonical = (value: string) => value.trim().toLowerCase().replace(/\.$/, '');
    return { localKey, peerKey, mismatch: canonical(draft.peer_ns) !== canonical(draft[peerKey]) } as const;
}
export function changeSetupDNSRole(draft: ServerSetupDraft, role: ServerSetupDraft['dns_role']): ServerSetupDraft {
    // Changing roles keeps the name and IP attached to the same physical server.
    if (role === draft.dns_role) return draft;
    const next = { ...draft, dns_role: role, ns1: draft.ns2, ns2: draft.ns1 };
    if (role === 'secondary') delete next.infrastructure_dns;
    return next;
}
// An editable suggestion only: common suffixes never authorize zone creation.
export function setupInfrastructureZoneCandidate(draft: ServerSetupDraft): string {
    const names = [draft.panel_domain, draft.ns1, draft.ns2].map(value => value.trim().toLowerCase().replace(/\.$/, '').split('.'));
    if (names.some(labels => labels.length < 2 || labels.some(label => !/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(label)))) return '';
    const suffix: string[] = [];
    for (let index = 1; index < Math.min(...names.map(labels => labels.length)); index++) {
        const label = names[0][names[0].length - index];
        if (!names.every(labels => labels[labels.length - index] === label)) break;
        suffix.unshift(label);
    }
    return suffix.length >= 2 ? suffix.join('.') : '';
}
export function setupDetectedIPv4(value?: string): string {
    if (!value || !/^(?:0|[1-9]\d{0,2})(?:\.(?:0|[1-9]\d{0,2})){3}$/.test(value)) return '';
    const [a, b, ...rest] = value.split('.').map(Number);
    if ([a, b, ...rest].some(n => n > 255) || a === 0 || a === 10 || a === 127 || a >= 224
        || (a === 169 && b === 254) || (a === 172 && b >= 16 && b <= 31)
        || (a === 192 && b === 168) || (a === 100 && b >= 64 && b <= 127)) return '';
    return value;
}

// Tab-local editing state only. Server revisions and operation admission remain
// authoritative; neither a reviewed plan nor permission to start is stored here.
// Yalniz sekmenin duzenleme durumu tutulur; sunucu surumu ve islem kabul
// denetimleri gecerliligini korur. Plan veya baslatma izni burada saklanmaz.
// The wizard's editing steps (alpha.111, setup wizard v2). "components" is
// shown only when the owner customizes the components.
export const setupWizardSteps = ['domain', 'count', 'name', 'roles', 'components', 'names', 'dnshome', 'records', 'mail', 'review'] as const;
export type SetupEditorStep = typeof setupWizardSteps[number];
export const setupEditorKey = (username: string) => `celikpanel.setup.editor.${username}`;
export interface SetupEditorCheckpoint {
    version: 1;
    revision: number;
    step: SetupEditorStep;
    draft: ServerSetupDraft;
}
export function decodeSetupEditorCheckpoint(raw: string | null, snapshot: ServerSetupSnapshot): SetupEditorCheckpoint | null {
    try {
        if (!['new', 'legacy', 'draft'].includes(snapshot.status)) return null;
        const value: unknown = JSON.parse(raw || 'null');
        if (!record(value) || value.version !== 1 || value.revision !== snapshot.revision) return null;
        // A checkpoint written before alpha.111 names the old steps: the
        // purpose step reopens at the start, the access step at the DNS step.
        // Eski adimlar yeni adimlara eslenir.
        const legacy: Record<string, SetupEditorStep> = { purpose: 'domain', access: 'dnshome' };
        const step = legacy[String(value.step)] ?? String(value.step);
        if (!(setupWizardSteps as readonly string[]).includes(step)) return null;
        const restored = decodeServerSetup({ ...snapshot, draft: value.draft });
        if (!restored || (step === 'components' && !restored.draft.customization && restored.draft.purpose !== 'custom')) return null;
        // Review can only be recovered from exactly the server-saved inputs.
        // Inceleme yalniz sunucuda kayitli girdilerle ayniysa geri yuklenir.
        if (step === 'review' && JSON.stringify(restored.draft) !== JSON.stringify(snapshot.draft)) return null;
        return { version: 1, revision: snapshot.revision, step: step as SetupEditorStep, draft: restored.draft };
    } catch { return null; }
}

// What an extension changes, against the draft it started from: the settings
// the owner sees on the access step. Empty without a base.
// Genisletmenin tabana gore degistirdigi ayarlar.
export type SetupExtensionChangeKey = 'purpose' | 'panel_domain' | 'mail_hostname' | 'dns_mode' | 'node_version' | 'database' | 'dns_hosting_management';
export interface SetupExtensionChange { key: SetupExtensionChangeKey; before: string; after: string }
export function setupExtensionChanges(base: ServerSetupDraft | undefined, draft: ServerSetupDraft): SetupExtensionChange[] {
    if (!base) return [];
    const purpose = (value: ServerSetupDraft) => value.customization ? 'custom' : value.purpose;
    const keys: SetupExtensionChangeKey[] = ['purpose', 'panel_domain', 'mail_hostname', 'dns_mode', 'node_version', 'database', 'dns_hosting_management'];
    const read = (value: ServerSetupDraft, key: SetupExtensionChangeKey) => key === 'purpose' ? purpose(value) : String(value[key] ?? '');
    return keys.map(key => ({ key, before: read(base, key), after: read(draft, key) }))
        .filter(change => change.before !== change.after);
}

// Setup wizard v2 (alpha.111): the main domain is asked once and first; every
// other name is proposed from it and stays editable.
// Ana alan adi bir kez ve ilk sorulur; diger adlar ondan onerilir.
const dnsLabel = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
export function normalizeSetupDomain(value: string): string {
    return value.trim().toLowerCase().replace(/^[a-z][a-z0-9+.-]*:\/\//, '').replace(/[/?#].*$/, '').replace(/:\d+$/, '').replace(/^www\./, '').replace(/\.$/, '');
}
export function validSetupDomain(value: string): boolean {
    const labels = value.split('.');
    return value.length <= 253 && labels.length >= 2 && labels.every(item => dnsLabel.test(item)) && !/^\d+$/.test(labels[labels.length - 1]);
}
export function validSetupShortName(value: string): boolean {
    return value === '' || dnsLabel.test(value);
}
export function setupNameWithin(name: string, domain: string): boolean {
    const value = name.trim().toLowerCase().replace(/\.$/, '');
    return !!domain && (value === domain || value.endsWith(`.${domain}`));
}
// An extended or reopened setup takes its main domain from the zone it
// prepared; otherwise the owner is asked (empty).
export function setupInitialMainDomain(draft: ServerSetupDraft): string {
    return draft.main_domain || draft.infrastructure_dns?.zone || '';
}
export interface SetupProposedNames { panel: string; mail: string; ns1: string; ns2: string }
export function setupProposedNames(mainDomain: string, shortName: string): SetupProposedNames {
    if (!mainDomain) return { panel: '', mail: '', ns1: '', ns2: '' };
    return { panel: `${shortName || 'panel'}.${mainDomain}`, mail: `mail.${mainDomain}`, ns1: `ns1.${mainDomain}`, ns2: `ns2.${mainDomain}` };
}
// Fills each name the owner has not typed: an empty one, or one still equal to
// the previous proposal. A name the owner changed is never replaced. The
// nameserver slots follow the setup protocol (ns1 = main DNS server).
// Sahibin yazmadigi adlar doldurulur; degistirdigi ad hic degistirilmez.
export function setupApplyProposedNames(draft: ServerSetupDraft, proposal: SetupProposedNames, previous: SetupProposedNames | null, withMail: boolean): ServerSetupDraft {
    const fill = (current: string, next: string, before: string | undefined) => !next ? current : (!current.trim() || current === before) ? next : current;
    const next = { ...draft, panel_domain: fill(draft.panel_domain, proposal.panel, previous?.panel) };
    if (withMail) next.mail_hostname = fill(draft.mail_hostname, proposal.mail, previous?.mail);
    next.ns1 = fill(draft.ns1, proposal.ns1, previous?.ns1);
    next.ns2 = fill(draft.ns2, proposal.ns2, previous?.ns2);
    const peerKey = next.dns_role === 'primary' ? 'ns2' : 'ns1';
    if (!draft.peer_ns.trim() || draft.peer_ns === draft[peerKey] || draft.peer_ns === previous?.[peerKey]) next.peer_ns = next[peerKey];
    return next;
}
// The DNS role in the wizard: proposed names keep ns1 on the main DNS server
// whichever server this is; names the owner typed stay with their server
// (changeSetupDNSRole).
export function changeSetupWizardDNSRole(draft: ServerSetupDraft, role: ServerSetupDraft['dns_role'], proposal: SetupProposedNames): ServerSetupDraft {
    if (role === draft.dns_role) return draft;
    const proposed = !!proposal.ns1 && [draft.ns1, draft.ns2].sort().join() === [proposal.ns1, proposal.ns2].sort().join();
    const next: ServerSetupDraft = proposed ? { ...draft, dns_role: role, ns1: proposal.ns1, ns2: proposal.ns2 } : changeSetupDNSRole(draft, role);
    if (role === 'secondary') delete next.infrastructure_dns;
    next.peer_ns = next[role === 'primary' ? 'ns2' : 'ns1'];
    if (role === 'primary' && next.dns_mode === 'local') next.main_dns_server = 'this';
    else if (role === 'secondary' && next.dns_mode === 'local' && next.server_count === 2) next.main_dns_server = 'peer';
    else delete next.main_dns_server;
    return next;
}
// With two servers and the owner's own DNS the second server joins by code
// (alpha.114): its address is never typed here. With one server the DNS is
// this server's alone (standalone): both names point here, no peer.
// Iki sunucuda ikinci sunucu kodla katilir; tek sunucuda DNS yalniz buradadir.
export function setupJoinPendingDraft(draft: ServerSetupDraft): ServerSetupDraft {
    if (draft.dns_mode !== 'local') return draft;
    if (draft.server_count === 1) return { ...draft, dns_role: 'primary', main_dns_server: 'this', peer_ip: '', peer_ns: '' };
    return { ...draft, peer_ip: '', peer_ns: draft.dns_role === 'primary' ? draft.ns2 : draft.ns1 };
}

// SPF is merged, never replaced (alpha.112, cmd/panel mergeServerSetupSPF):
// this mail server is added before the final "all" or "redirect".
export function mergeSetupSPF(old: string, mailHost: string): string {
    const mechanism = `a:${mailHost}`;
    const fields = old.trim().split(/\s+/).filter(Boolean);
    if (fields.length === 0) return `v=spf1 mx ${mechanism} ~all`;
    if (fields.some(field => field.toLowerCase() === mechanism.toLowerCase())) return fields.join(' ');
    let at = fields.findIndex(field => { const lower = field.toLowerCase(); return (lower.endsWith('all') && lower.length <= 4) || lower.startsWith('redirect='); });
    if (at < 0) at = fields.length;
    return [...fields.slice(0, at), mechanism, ...fields.slice(at)].join(' ');
}
export const setupImportedSPF = (draft: ServerSetupDraft, domain: string) =>
    draft.imported_records?.find(item => item.decision === 'import' && item.type === 'TXT' && item.name === domain && item.content.toLowerCase().startsWith('v=spf1'))?.content || '';
export const setupImportedMX = (draft: ServerSetupDraft, domain: string) =>
    draft.imported_records?.filter(item => item.decision === 'import' && item.type === 'MX' && item.name === domain) || [];
export const setupImportedDMARC = (draft: ServerSetupDraft, domain: string) =>
    draft.imported_records?.find(item => item.decision === 'import' && item.type === 'TXT' && item.name === `_dmarc.${domain}` && item.content.toUpperCase().startsWith('V=DMARC1'));
