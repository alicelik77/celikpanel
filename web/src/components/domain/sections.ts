// The nine sections of a domain's page (alpha.132) and what each holds.
//
// This file decides, from the viewer and the domain alone, which sections and
// which parts of them are drawn; it has no React and no copy, so the decision
// can be read and tested on its own. The shell (DomainDetail.tsx) draws the
// row from it, and each section's own lazy file draws the parts it is given.
// The conditions are the ones the eleven tabs had before alpha.132, moved
// unchanged: nothing a viewer could open before is lost, and nothing a team
// member was not granted appears.
//
// The place on the page is in the address (?section=…&part=…). Every older
// link (?tab=…, and the health checks' {tab, sub}) keeps working through
// legacyPlace below; stored notices and server-built links still carry them.
//
// Alan adı sayfasının dokuz bölümü ve her birinin içeriği. Hangi bölümün ve
// parçanın çizileceği yalnız izleyiciye ve alan adına göre burada belirlenir;
// koşullar eski on bir sekmenin koşullarıdır. Sayfadaki yer adreste durur;
// eski ?tab= bağlantıları legacyPlace ile çalışmayı sürdürür.
import type { TranslationKey } from '../../i18n/en';
import type { DomainAccessMode, DomainCapability } from '../../auth/domainAccess';

export type SectionId =
    | 'overview' | 'site' | 'files' | 'mail' | 'databases' | 'dns' | 'security' | 'backups' | 'advanced';

export type PartId =
    | 'overview'
    | 'type' | 'apps' | 'ai' | 'programs' | 'php' | 'redirects'
    | 'manager' | 'access'
    | 'mail'
    | 'databases'
    | 'connection' | 'names' | 'records'
    | 'certificate' | 'protect' | 'malware'
    | 'backups'
    | 'cron' | 'logs' | 'git' | 'copy' | 'web' | 'config';

export const SECTION_ORDER: readonly SectionId[] = [
    'overview', 'site', 'files', 'mail', 'databases', 'dns', 'security', 'backups', 'advanced',
];

export const SECTION_LABELS: Record<SectionId, TranslationKey> = {
    overview: 'domain.section.overview',
    site: 'domain.section.site',
    files: 'domain.section.files',
    mail: 'domain.section.mail',
    databases: 'domain.section.databases',
    dns: 'domain.section.dns',
    security: 'domain.section.security',
    backups: 'domain.section.backups',
    advanced: 'domain.section.advanced',
};

// The words of a part in the section's "On this page" index. A section with
// one part has no index; its part is named by the section.
export const PART_LABELS: Record<PartId, TranslationKey> = {
    overview: 'domain.section.overview',
    type: 'domain.part.type',
    apps: 'domain.part.apps',
    ai: 'svcapps.tab',
    programs: 'runtime.tab',
    php: 'domain.part.php',
    redirects: 'siteFeatures.redirect.title',
    manager: 'domain.part.fileManager',
    access: 'domain.part.access',
    mail: 'domain.section.mail',
    databases: 'domain.section.databases',
    connection: 'domain.part.connection',
    names: 'domain.part.names',
    records: 'domain.part.records',
    certificate: 'domain.part.certificate',
    protect: 'siteFeatures.protect.title',
    malware: 'mscan.title',
    backups: 'domain.section.backups',
    cron: 'domain.part.cron',
    logs: 'domain.part.logs',
    git: 'siteFeatures.git.title',
    copy: 'siteFeatures.copy.title',
    web: 'siteFeatures.web.title',
    config: 'siteConfig.tab',
};

// The guide's "?" opens the chapter that claims the topic on screen: the
// part's own topic where a chapter is about that part, otherwise the
// section's (docs/guide/*/ front matter; tests/domain-sections-alpha132).
export const SECTION_TOPICS: Record<SectionId, string> = {
    overview: 'domain.overview',
    site: 'domain.site',
    files: 'domain.files',
    mail: 'domain.mail',
    databases: 'domain.databases',
    dns: 'domain.dns',
    security: 'domain.security',
    backups: 'domain.backups',
    advanced: 'domain.advanced',
};
export const PART_TOPICS: Partial<Record<PartId, string>> = {
    apps: 'domain.site.apps',
    ai: 'domain.site.ai',
    access: 'domain.files.access',
    malware: 'domain.security.malware',
};

export function guideTopic(section: SectionId, part: PartId | null): string {
    return (part && PART_TOPICS[part]) || SECTION_TOPICS[section];
}

/** A place on the page: a section, and optionally one part of it. */
export interface Place {
    section: SectionId;
    part?: PartId;
}

export interface PartView {
    id: PartId;
    /** A team member who may only view what this part shows. */
    readOnly: boolean;
    /**
     * Present only in a long section (Site, Advanced): the part is a card that
     * opens and closes, and this is whether it starts open. A closed part
     * loads nothing until it is opened.
     */
    startsOpen?: boolean;
}

export interface SectionView {
    id: SectionId;
    /** Drawn and opened as usual. */
    parts: PartView[];
    /**
     * Closed by the plan (D-042): drawn faded in its own place in the row,
     * and opening a page that says what it is and whom to ask. The rights
     * that open it.
     */
    notInPlan?: string[];
}

export interface SectionInput {
    role: string;
    /** The domain's hosting type; an absent one is a PHP site. */
    projectType: string;
    /** The rights the domain's subscription closes (rights_closed). */
    closedRights: readonly string[];
    /** A team member's grants on this domain; absent for everyone else. */
    access?: Partial<Record<DomainCapability, DomainAccessMode>>;
    /**
     * The server's mail and database engines as last known; null while that
     * is not known, which keeps their sections (nothing hidden on a guess).
     * Never asked for a team member.
     */
    server: { mail: boolean; databases: boolean } | null;
}

interface PartRule {
    id: PartId;
    capabilities: DomainCapability[];
    shown: (v: Viewer) => boolean;
    /**
     * Site and Advanced only (owner review, 12 Nov 2026: with real content
     * they were about 5,000 px long): whether the part starts open.
     */
    startsOpen?: (v: Viewer) => boolean;
}

interface Viewer {
    teamMember: boolean;
    admin: boolean;
    type: string;
    dnsOnly: boolean;
    view: (capability: DomainCapability) => boolean;
    open: (right: string) => boolean;
    server: SectionInput['server'];
}

const php = (v: Viewer) => v.type === 'php';
// A DNS-only domain has no website: no files, PHP, certificate or web server.
const site = (v: Viewer) => !v.dnsOnly;

// Every part, in the order its section draws them. Each condition is the one
// its tab or sub-tab had before alpha.132.
// Her parça, bölümünün çizdiği sırayla; koşul eski sekmesininkidir.
const RULES: Record<SectionId, PartRule[]> = {
    overview: [
        { id: 'overview', capabilities: [], shown: (v) => !v.teamMember },
    ],
    site: [
        { id: 'type', capabilities: ['files'], shown: (v) => site(v) && !v.teamMember && v.view('files'), startsOpen: () => true },
        {
            id: 'apps', capabilities: ['files', 'php'],
            shown: (v) => !v.teamMember && php(v) && v.view('files') && v.view('php')
                && (v.open('app_installer') || v.admin || v.open('wordpress_tools')),
            // Ready apps and WordPress tools are for PHP sites, the only kind this part is shown for.
            startsOpen: php,
        },
        // An app or a program that runs stays manageable after its right
        // closed (D-042 P5).
        {
            id: 'ai', capabilities: [], shown: (v) => !v.teamMember && site(v) && (v.open('service_apps') || v.type === 'service_app'),
            startsOpen: (v) => v.type === 'service_app',
        },
        {
            id: 'programs', capabilities: [],
            shown: (v) => !v.teamMember && site(v) && (v.open('program_apps') || v.open('python_apps') || v.type === 'runtime_app'),
            startsOpen: (v) => v.type === 'runtime_app',
        },
        { id: 'php', capabilities: ['php'], shown: (v) => site(v) && php(v) && v.view('php'), startsOpen: () => true },
        {
            id: 'redirects', capabilities: ['files'],
            shown: (v) => site(v) && v.view('files') && v.open('site_protect_redirects') && v.type !== 'forwarding',
            startsOpen: () => true,
        },
    ],
    files: [
        { id: 'manager', capabilities: ['files'], shown: (v) => site(v) && v.view('files') && v.open('file_manager') },
        { id: 'access', capabilities: [], shown: (v) => !v.teamMember && site(v) && (v.open('file_access') || v.open('ssh')) },
    ],
    mail: [
        {
            id: 'mail', capabilities: ['mail'],
            shown: (v) => v.view('mail') && v.open('mail') && (v.teamMember || !v.server || v.server.mail),
        },
    ],
    databases: [
        {
            id: 'databases', capabilities: ['databases'],
            shown: (v) => v.view('databases') && v.open('databases') && (v.teamMember || !v.server || v.server.databases),
        },
    ],
    dns: [
        // Whether the domain points here: before alpha.132 on the Overview
        // only, which a team member does not have.
        { id: 'connection', capabilities: [], shown: (v) => !v.teamMember },
        // The www redirect and the aliases (the old Hosting › General).
        { id: 'names', capabilities: ['files'], shown: (v) => site(v) && !v.teamMember && v.view('files') },
        { id: 'records', capabilities: ['dns'], shown: (v) => v.view('dns') },
    ],
    security: [
        { id: 'certificate', capabilities: ['ssl'], shown: (v) => site(v) && v.view('ssl') },
        {
            id: 'protect', capabilities: ['files'],
            shown: (v) => site(v) && v.view('files') && v.open('site_protect_redirects') && (v.type === 'php' || v.type === 'static'),
        },
        { id: 'malware', capabilities: [], shown: (v) => !v.teamMember && site(v) && (v.admin || v.open('malware_scan')) },
    ],
    backups: [
        { id: 'backups', capabilities: ['backups'], shown: (v) => site(v) && v.view('backups') && v.open('backups') },
    ],
    advanced: [
        { id: 'cron', capabilities: ['cron'], shown: (v) => site(v) && v.view('cron') && v.open('cron'), startsOpen: () => true },
        { id: 'logs', capabilities: ['statistics'], shown: (v) => site(v) && v.view('statistics'), startsOpen: () => false },
        {
            id: 'git', capabilities: [],
            shown: (v) => !v.teamMember && (v.type === 'php' || v.type === 'static' || v.type === 'node') && v.open('file_manager'),
            startsOpen: () => false,
        },
        { id: 'copy', capabilities: [], shown: (v) => !v.teamMember && site(v) && v.type !== 'forwarding' && v.open('file_manager'), startsOpen: () => false },
        { id: 'web', capabilities: ['files'], shown: (v) => site(v) && v.view('files') && v.type !== 'forwarding', startsOpen: () => false },
        { id: 'config', capabilities: ['files'], shown: (v) => site(v) && v.admin, startsOpen: () => false },
    ],
};

// A section the plan closes as a whole, and the right that opens it again.
// Only these four can be closed whole; in every other section something
// stays. A section the viewer was not granted, or the server cannot offer
// (no mail or database engine installed), is not "not in the plan": it is
// not drawn at all, as before.
// Planın bütünüyle kapattığı bölüm ve onu yeniden açan hak.
function notInPlan(id: SectionId, v: Viewer): string[] | undefined {
    switch (id) {
        case 'mail':
            return v.view('mail') && !v.open('mail') && (v.teamMember || !v.server || v.server.mail) ? ['mail'] : undefined;
        case 'databases':
            return v.view('databases') && !v.open('databases') && (v.teamMember || !v.server || v.server.databases) ? ['databases'] : undefined;
        case 'backups':
            return site(v) && v.view('backups') && !v.open('backups') ? ['backups'] : undefined;
        case 'files':
            return site(v) && v.view('files') && !v.open('file_manager') && (v.teamMember || (!v.open('file_access') && !v.open('ssh')))
                ? ['file_manager'] : undefined;
        default:
            return undefined;
    }
}

const ORDER: Record<DomainAccessMode, number> = { none: 0, view: 1, manage: 2 };

// The part of Site that IS the site for a program or a service app: it comes
// first in Site, before "Site type" (owner, 12 Nov 2026: after alpha.132 he
// could not find his running Go program in the Site section).
// Program ya da hizmet uygulaması olan sitede Site bölümünün ilk parçası.
const SITE_LEAD: Partial<Record<string, PartId>> = { runtime_app: 'programs', service_app: 'ai' };

/** The Site part that holds what runs on a site of this kind, if the kind has one. */
export function siteLeadPart(projectType: string | null | undefined): PartId | null {
    return SITE_LEAD[projectType || 'php'] ?? null;
}

/**
 * The sections this viewer sees on this domain, always in SECTION_ORDER: a
 * section the plan closes keeps its own place, faded. A section with nothing
 * to show and no plan reason is absent; the others close up around it.
 */
export function buildSections(input: SectionInput): SectionView[] {
    const teamMember = input.role === 'additional_user';
    const closed = new Set(input.closedRights);
    const type = input.projectType || 'php';
    const mode = (capability: DomainCapability): DomainAccessMode => (
        teamMember ? input.access?.[capability] ?? 'none' : 'manage'
    );
    const viewer: Viewer = {
        teamMember,
        admin: input.role === 'admin',
        type,
        dnsOnly: type === 'dnsonly',
        view: (capability) => mode(capability) !== 'none',
        open: (right) => !closed.has(right),
        server: teamMember ? null : input.server,
    };
    const sections: SectionView[] = [];
    for (const id of SECTION_ORDER) {
        const parts = RULES[id].filter((rule) => rule.shown(viewer)).map((rule) => {
            // The weakest grant among the part's capabilities decides.
            const weakest = rule.capabilities.reduce<DomainAccessMode>(
                (low, capability) => (ORDER[mode(capability)] < ORDER[low] ? mode(capability) : low),
                'manage',
            );
            const view: PartView = { id: rule.id, readOnly: teamMember && weakest === 'view' };
            if (rule.startsOpen) view.startsOpen = rule.startsOpen(viewer);
            return view;
        });
        const lead = id === 'site' ? siteLeadPart(type) : null;
        const at = lead ? parts.findIndex((part) => part.id === lead) : -1;
        if (at > 0) parts.unshift(...parts.splice(at, 1));
        if (parts.length > 0) {
            sections.push({ id, parts });
            continue;
        }
        const rights = notInPlan(id, viewer);
        if (rights) sections.push({ id, parts: [], notInPlan: rights });
    }
    return sections;
}

// --- The address --------------------------------------------------------------

const SECTION_IDS = new Set<string>(SECTION_ORDER);
const PART_IDS = new Set<string>(Object.keys(PART_LABELS));

// Every value an older link can carry in ?tab= (and the health checks in
// {tab, sub}), and where it is now. A sub-tab name wins over its tab
// ("hosting"+"ssl" is the certificate). Kept for good: notices stored in
// account_notices.fix_path and links the server builds still use them.
// Eski bağlantıların ?tab= değerleri ve bugünkü yerleri; kalıcıdır.
const LEGACY: Record<string, Place> = {
    overview: { section: 'overview' },
    hosting: { section: 'site' },
    type: { section: 'site', part: 'type' },
    apps: { section: 'site', part: 'apps' },
    ai: { section: 'site', part: 'ai' },
    programs: { section: 'site', part: 'programs' },
    php: { section: 'site', part: 'php' },
    redirects: { section: 'site', part: 'redirects' },
    files: { section: 'files', part: 'manager' },
    access: { section: 'files', part: 'access' },
    mail: { section: 'mail' },
    databases: { section: 'databases' },
    dns: { section: 'dns', part: 'records' },
    general: { section: 'dns', part: 'names' },
    ssl: { section: 'security', part: 'certificate' },
    protect: { section: 'security', part: 'protect' },
    malware: { section: 'security', part: 'malware' },
    backups: { section: 'backups' },
    advanced: { section: 'advanced' },
    cron: { section: 'advanced', part: 'cron' },
    logs: { section: 'advanced', part: 'logs' },
    git: { section: 'advanced', part: 'git' },
    copy: { section: 'advanced', part: 'copy' },
    web: { section: 'advanced', part: 'web' },
    config: { section: 'advanced', part: 'config' },
};

/** Where an older ?tab= link (or a health fix's {tab, sub}) leads now. */
export function legacyPlace(tab: string | null | undefined, sub?: string | null): Place | null {
    if (sub && Object.prototype.hasOwnProperty.call(LEGACY, sub)) return LEGACY[sub];
    if (tab && Object.prototype.hasOwnProperty.call(LEGACY, tab)) return LEGACY[tab];
    return null;
}

/** The place the address asks for: ?section=…&part=…, else an older ?tab=. */
export function requestedPlace(params: URLSearchParams): Place | null {
    const section = params.get('section');
    if (section && SECTION_IDS.has(section)) {
        const part = params.get('part');
        return part && PART_IDS.has(part) ? { section: section as SectionId, part: part as PartId } : { section: section as SectionId };
    }
    return legacyPlace(params.get('tab'));
}

/** The address of a place: the other parameters kept, an older ?tab= gone. */
export function withPlace(params: URLSearchParams, place: Place): URLSearchParams {
    const next = new URLSearchParams(params);
    next.delete('tab');
    next.set('section', place.section);
    if (place.part) next.set('part', place.part);
    else next.delete('part');
    return next;
}

/** Whether a place can be opened as it is: its section is drawn and open, and its part is in it. */
export function isOpenPlace(sections: readonly SectionView[], place: Place): boolean {
    const section = sections.find((item) => item.id === place.section);
    if (!section || section.notInPlan) return false;
    return !place.part || section.parts.some((part) => part.id === place.part);
}
