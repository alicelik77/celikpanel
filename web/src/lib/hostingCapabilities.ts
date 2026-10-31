import { useRemote, type Remote, type RemoteHandle } from './remote';
import type { TranslationKey } from '../i18n/en';

// What this server can host right now: GET /api/v1/hosting/capabilities.
//
// This is the ONE reader of that address. Before 9 Oct 2026 eight screens read
// it on their own and each gave a failed or unfinished read its own meaning:
// "no DNS engine", "PHP is installed", "no database engine", an empty version
// list. An owner opening "Add domain" on a server that has DNS was told to
// choose a DNS engine for as long as the read took.
//
// Every reader now gets the same three states from the same request. Nothing
// about the server is shown as missing until the server has said so.
//
// Bu sunucunun şu anda neyi barındırabildiği. Bu adresin TEK okuyucusu budur.
// 9 Eki 2026'dan önce sekiz ekran onu ayrı ayrı okuyor ve bitmemiş ya da
// başarısız okumaya her biri kendi anlamını veriyordu. Artık her okuyan aynı
// istekten aynı üç durumu alır; sunucu söylemeden hiçbir şey eksik gösterilmez.
export interface HostingCapabilities {
    /** "nginx", or "" when no supported web server is installed. */
    web_server: string;
    /** Installed PHP-FPM versions, newest first. */
    php_versions: string[];
    /** The proven active engine ("bind", "pdns"), or "". */
    dns_server: string;
    /**
     * Why dns_server is "" (owner report 2026-10-31): a primary waiting for
     * its backup still serves every zone. "" when the server did not say
     * (an older server, or the read failed): then nothing is claimed.
     */
    dns_server_state: DNSServerState;
    /** The active engine whenever one is active, also when not ready. */
    dns_server_engine: string;
    /** The other server of a DNS pair (its nameserver name or address). */
    dns_server_peer: string;
    /** The nameserver pair and operating mode are saved. */
    dns_identity_ready: boolean;
    dns_management_mode: 'local' | 'external' | 'existing' | '';
    dns_management_ready: boolean;
    mail_server: boolean;
    database_servers: string[];
    db_tools: string[];
}

export type DNSServerState = 'none' | 'switching' | 'port53_conflict' | 'not_running' | 'engine_conflict' | 'secondary' | 'pair_pending' | 'ready' | '';
const dnsServerStates: DNSServerState[] = ['none', 'switching', 'port53_conflict', 'not_running', 'engine_conflict', 'secondary', 'pair_pending', 'ready'];

export const HOSTING_CAPABILITIES_URL = '/api/v1/hosting/capabilities';

function strings(value: unknown): string[] {
    if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) throw new Error('list');
    return value as string[];
}

// The answer is the contract or it is unknown. A missing or mistyped field is
// not read as "nothing installed": that is how an unreadable answer once became
// "DNS is missing" on screen.
// Yanıt ya sözleşmedir ya da bilinmeyendir. Eksik ya da yanlış türde bir alan
// "hiçbir şey kurulu değil" diye okunmaz.
export function decodeHostingCapabilities(raw: unknown): HostingCapabilities {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (
        typeof body.web_server !== 'string' ||
        typeof body.dns_server !== 'string' ||
        typeof body.dns_identity_ready !== 'boolean' ||
        typeof body.mail_server !== 'boolean'
    ) {
        throw new Error('field');
    }
    const mode = body.dns_management_mode;
    return {
        web_server: body.web_server,
        php_versions: strings(body.php_versions),
        dns_server: body.dns_server,
        dns_server_state: dnsServerStates.includes(body.dns_server_state as DNSServerState) ? body.dns_server_state as DNSServerState : '',
        dns_server_engine: typeof body.dns_server_engine === 'string' ? body.dns_server_engine : '',
        dns_server_peer: typeof body.dns_server_peer === 'string' ? body.dns_server_peer : '',
        dns_identity_ready: body.dns_identity_ready,
        dns_management_mode: mode === 'local' || mode === 'external' || mode === 'existing' ? mode : '',
        dns_management_ready: body.dns_management_ready === true,
        mail_server: body.mail_server,
        database_servers: strings(body.database_servers),
        db_tools: strings(body.db_tools),
    };
}

// A dialog or panel that mounts within this time of the page's own read uses
// the page's answer and sends nothing. Later than that it still shows the
// page's answer at once and refreshes it behind the screen.
// Sayfanın okumasından bu süre içinde açılan pencere ya da bölüm sayfanın
// yanıtını kullanır ve istek göndermez.
const FRESH_FOR_MS = 30_000;

// `enabled: false` is for a signed-in team member: the server-wide inventory
// is not theirs to read, so nothing is requested and the caller must not draw
// from the handle.
// `enabled: false` ekip üyesi içindir: sunucu geneli envanter ona ait değildir.
export function useHostingCapabilities(options: { enabled?: boolean } = {}): RemoteHandle<HostingCapabilities> {
    return useRemote(
        options.enabled === false ? null : HOSTING_CAPABILITIES_URL,
        decodeHostingCapabilities,
        { freshFor: FRESH_FOR_MS },
    );
}

// --- DNS readiness for adding a domain --------------------------------------
//
// Explicit external or remote DNS permits websites without a local publisher.
// A DNS-only domain still requires a proven local authoritative service.
// Açık harici ya da uzak DNS, yerel yayıncı olmadan web sitesine izin verir.
// Yalnız-DNS alan adı ise kanıtlanmış yerel bir yetkili hizmet ister.
export type DomainPurpose = 'website' | 'dnsonly';

/** Which half is missing: no active engine, or an engine without its identity. */
export type DNSBlocker = 'engine' | 'identity';

export function localDNSReady(caps: HostingCapabilities): boolean {
    return caps.dns_server !== '' && caps.dns_identity_ready === true;
}

export function hostingDNSReady(caps: HostingCapabilities): boolean {
    const delegatedElsewhere =
        (caps.dns_management_mode === 'external' || caps.dns_management_mode === 'existing') &&
        caps.dns_management_ready === true;
    return localDNSReady(caps) || delegatedElsewhere;
}

// null when a domain of this purpose can be added. The blocker is computed
// from a known answer only; see gateOn in ./remote.
// Bu amaçla alan adı eklenebiliyorsa null. Engel yalnız bilinen yanıttan
// hesaplanır.
export function dnsBlocker(caps: HostingCapabilities, purpose: DomainPurpose): DNSBlocker | null {
    const ready = purpose === 'dnsonly' ? localDNSReady(caps) : hostingDNSReady(caps);
    if (ready) return null;
    return caps.dns_server === '' ? 'engine' : 'identity';
}

export type CapabilitiesRemote = Remote<HostingCapabilities>;

// --- Why the local DNS takes no new change ------------------------------------
//
// dns_server "" used to be drawn as "NOT PUBLISHED: no active DNS engine",
// also on a main DNS server whose backup had not yet confirmed the zone list
// while both answered publicly (owner report 2026-10-31, Frankfurt). The
// explanation follows dns_server_state; what is allowed does not change.
// dns_server "" artık durumuna göre açıklanır; izin verilen değişmez.
export type DNSStateExplained = Exclude<DNSServerState, 'ready' | ''>;

export interface DNSStateExplanation {
    state: DNSStateExplained;
    /** The records are still answered (a backup, or a main server waiting for it). */
    served: boolean;
    what: TranslationKey;
    todo: TranslationKey;
    title: TranslationKey;
    vars: { engine: string; peer: string };
}

const engineNames: Record<string, string> = { pdns: 'PowerDNS', bind: 'BIND' };

/** null when the local DNS takes changes, or when nothing is known. */
export function dnsStateExplanation(caps: HostingCapabilities, t: (key: TranslationKey) => string): DNSStateExplanation | null {
    if (caps.dns_server !== '') return null;
    const state: DNSStateExplained = caps.dns_server_state === '' || caps.dns_server_state === 'ready' ? 'none' : caps.dns_server_state;
    const served = state === 'pair_pending' || state === 'secondary';
    const peer = caps.dns_server_peer || t(state === 'secondary' ? 'dnsstate.mainServer' : 'dnsstate.backupServer');
    const engine = engineNames[caps.dns_server_engine] ?? (caps.dns_server_engine || 'DNS');
    return {
        state,
        served,
        what: state === 'none' ? 'dns.notServed' : `dnsstate.${state}.what` as TranslationKey,
        todo: `dnsstate.${state}.todo` as TranslationKey,
        title: state === 'pair_pending' || state === 'switching' ? 'dnsstate.title.waiting' : state === 'secondary' ? 'dnsstate.title.backup' : state === 'none' ? 'dnsstate.title.notServed' : 'dnsstate.title.blocked',
        vars: { engine, peer },
    };
}

/** The sentence an Add domain screen says when the engine half is missing. */
export function dnsEngineBlockerText(caps: HostingCapabilities, t: (key: TranslationKey, vars?: Record<string, string>) => string): string {
    const explained = dnsStateExplanation(caps, t);
    if (!explained || explained.state === 'none') return t('domains.add.needsDns');
    return `${t(explained.what, explained.vars)} ${t(explained.todo, explained.vars)}`;
}
