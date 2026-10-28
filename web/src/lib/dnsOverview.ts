// The DNS page's overview (v0.1.0-alpha.115): GET /api/v1/dns/overview, read
// only. The decoder refuses an answer outside the contract (lib/remote.ts
// then shows "could not be checked", never a guess). The helpers here are
// pure: what the sentence says, how a server is named, and which MX record
// "Move mail to …" may change.
// DNS sayfasinin ozeti: sozlesme disi yanit reddedilir; yardimcilar saf.

export const DNS_OVERVIEW_URL = '/api/v1/dns/overview';

export type DNSOverviewState = 'ok' | 'waiting' | 'prerequisite' | 'failed' | 'unknown';
export type DNSOverviewRowState = 'ok' | 'missing' | 'different' | 'not_yet' | 'unknown' | 'kept' | 'pending';

export interface DNSOverviewServer {
    who: 'this' | 'peer';
    role: 'main' | 'backup' | 'standalone' | 'none';
    short_name?: string;
    panel_name?: string;
    software?: string;
    ipv4?: string;
    answers: 'ok' | 'not_serving' | 'unknown' | 'not_joined' | 'not_applicable';
    serial?: number;
}

export interface DNSOverviewName {
    name: string;
    kind: 'panel' | 'mail' | 'ns' | 'nameservers' | 'glue';
    type: string;
    server?: 'this' | 'peer';
    expected: string;
    seen: string[];
    state: DNSOverviewRowState;
    where: 'zone' | 'main' | 'provider' | 'remote' | 'registrar';
    optional?: boolean;
    reason?: string;
}

export interface DNSOverviewMail {
    kind: 'mx' | 'spf' | 'dkim' | 'dmarc';
    name: string;
    type: string;
    expected: string;
    seen: string[];
    state: DNSOverviewRowState;
}

export interface DNSOverview {
    domain: string;
    home: 'own' | 'provider' | 'remote' | 'none';
    setup_ready: boolean;
    sentence: { state: DNSOverviewState; code: string; reason: string; who: string; next: string; then: string; vars?: Record<string, string> };
    servers: DNSOverviewServer[];
    names: DNSOverviewName[];
    mail: DNSOverviewMail[];
    mail_receiver: { server: 'this' | 'peer' | 'none'; host?: string; short_name?: string };
    mx_move?: { zone: string; to_host: string; to: 'this' | 'peer' };
    own_records_link: { where: 'here' | 'main' | 'provider' | 'remote' | 'none'; path?: string; api?: string; panel_name?: string };
    join: { state: 'waiting' | 'bound' | 'unavailable'; reason?: string };
    checked_at: string;
    next_check_after: number;
}

// The sentence codes the page words itself; any other code is shown with the
// server's own English sentence (D-024: never a bare code).
export const DNS_OVERVIEW_CODES = [
    'setup_not_done', 'setup_running', 'join_waiting', 'main_not_serving', 'backup_not_serving', 'peer_not_joined',
    'registrar_not_pointing', 'domain_not_registered', 'glue_missing', 'provider_records', 'remote_records',
    'provider_mail', 'remote_mail', 'server_unknown', 'some_unknown', 'spreading', 'mail_spreading',
    'ok_mail_kept', 'ok', 'check_running',
] as const;

const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const text = (value: unknown) => typeof value === 'string';
const optionalText = (value: unknown) => value === undefined || typeof value === 'string';
const strings = (value: unknown) => Array.isArray(value) && value.length <= 32 && value.every(text);
const oneOf = (value: unknown, allowed: readonly string[]) => typeof value === 'string' && allowed.includes(value);
const rowStates = ['ok', 'missing', 'different', 'not_yet', 'unknown', 'kept', 'pending'] as const;

export function decodeDNSOverview(raw: unknown): DNSOverview {
    const invalid = (what: string) => new Error(`DNS overview outside the contract: ${what}`);
    if (!record(raw) || !text(raw.domain) || !oneOf(raw.home, ['own', 'provider', 'remote', 'none']) || typeof raw.setup_ready !== 'boolean'
        || !text(raw.checked_at) || typeof raw.next_check_after !== 'number') throw invalid('shape');
    const sentence = raw.sentence;
    if (!record(sentence) || !oneOf(sentence.state, ['ok', 'waiting', 'prerequisite', 'failed', 'unknown']) || !text(sentence.code)
        || !text(sentence.reason) || !text(sentence.who) || !text(sentence.next) || !text(sentence.then)
        || (sentence.vars !== undefined && (!record(sentence.vars) || !Object.values(sentence.vars).every(text)))) throw invalid('sentence');
    if (!Array.isArray(raw.servers) || raw.servers.length > 2) throw invalid('servers');
    for (const server of raw.servers) {
        if (!record(server) || !oneOf(server.who, ['this', 'peer']) || !oneOf(server.role, ['main', 'backup', 'standalone', 'none'])
            || !oneOf(server.answers, ['ok', 'not_serving', 'unknown', 'not_joined', 'not_applicable'])
            || !optionalText(server.short_name) || !optionalText(server.panel_name) || !optionalText(server.software) || !optionalText(server.ipv4)) throw invalid('server');
    }
    if (!Array.isArray(raw.names) || raw.names.length > 32) throw invalid('names');
    for (const name of raw.names) {
        if (!record(name) || !text(name.name) || !oneOf(name.kind, ['panel', 'mail', 'ns', 'nameservers', 'glue']) || !text(name.type) || !text(name.expected)
            || !strings(name.seen) || !oneOf(name.state, rowStates) || !oneOf(name.where, ['zone', 'main', 'provider', 'remote', 'registrar'])
            || !optionalText(name.server) || !optionalText(name.reason)) throw invalid('name');
    }
    if (!Array.isArray(raw.mail) || raw.mail.length > 4) throw invalid('mail');
    for (const mail of raw.mail) {
        if (!record(mail) || !oneOf(mail.kind, ['mx', 'spf', 'dkim', 'dmarc']) || !text(mail.name) || !text(mail.type) || !text(mail.expected)
            || !strings(mail.seen) || !oneOf(mail.state, rowStates)) throw invalid('mail record');
    }
    const receiver = raw.mail_receiver;
    if (!record(receiver) || !oneOf(receiver.server, ['this', 'peer', 'none']) || !optionalText(receiver.host) || !optionalText(receiver.short_name)) throw invalid('receiver');
    const move = raw.mx_move;
    if (move !== undefined && (!record(move) || !text(move.zone) || !text(move.to_host) || !oneOf(move.to, ['this', 'peer']))) throw invalid('mx move');
    const own = raw.own_records_link;
    if (!record(own) || !oneOf(own.where, ['here', 'main', 'provider', 'remote', 'none']) || !optionalText(own.path) || !optionalText(own.api) || !optionalText(own.panel_name)) throw invalid('own records');
    // The editor is opened inside the panel only.
    if (own.path !== undefined && !String(own.path).startsWith('/domains/')) throw invalid('own records path');
    if (own.api !== undefined && !String(own.api).startsWith('/api/v1/')) throw invalid('own records api');
    const join = raw.join;
    if (!record(join) || !oneOf(join.state, ['waiting', 'bound', 'unavailable']) || !optionalText(join.reason)) throw invalid('join');
    return raw as unknown as DNSOverview;
}

/** The i18n key stem of the sentence, or null when the page has no words for its code. */
export function dnsOverviewSentenceKey(code: string): string | null {
    const short = code.startsWith('dns_overview_') ? code.slice('dns_overview_'.length) : '';
    return (DNS_OVERVIEW_CODES as readonly string[]).includes(short) ? `dnsov.s.${short}` : null;
}

/** The sentence's tone: a verified failure, a task, or calm. */
export function dnsOverviewTone(state: DNSOverviewState): 'bad' | 'task' | 'done' | 'neutral' {
    return state === 'failed' ? 'bad' : state === 'prerequisite' ? 'task' : state === 'ok' ? 'done' : 'neutral';
}

type Translate = (key: string, values?: Record<string, string | number>) => string;

/** A server as the screens say it: "This server (name)" / "Second server (name)", never an invented name. */
export function dnsOverviewServerLabel(t: Translate, server: Pick<DNSOverviewServer, 'who' | 'short_name'> | undefined): string {
    if (!server) return t('dnsov.peerServer');
    if (server.who === 'this') return server.short_name ? t('dnsov.thisServerNamed', { name: server.short_name }) : t('dnsov.thisServer');
    return server.short_name ? t('dnsov.peerServerNamed', { name: server.short_name }) : t('dnsov.peerServer');
}

/** The sentence's values with the servers named by the page, not by the server's fallback. */
export function dnsOverviewSentenceValues(t: Translate, overview: DNSOverview): Record<string, string> {
    const vars = { ...(overview.sentence.vars || {}) };
    const byRole = (role: string) => overview.servers.find(server => server.role === role || (role === 'main' && server.role === 'standalone'));
    const main = byRole('main');
    const backup = byRole('backup');
    if (main) vars.main = dnsOverviewServerLabel(t, main);
    if (backup) vars.backup = dnsOverviewServerLabel(t, backup);
    if (vars.ip) {
        const named = overview.servers.find(server => server.ipv4 === vars.ip);
        if (named) vars.server = dnsOverviewServerLabel(t, named);
    }
    if (!vars.server && vars.host) vars.server = dnsOverviewServerLabel(t, overview.servers.find(server => server.who === overview.mx_move?.to));
    for (const key of ['seen', 'names']) if (vars[key] === '') vars[key] = '—';
    return vars;
}

export interface DNSRecordRow {
    id: number;
    name: string;
    type: string;
    content: string;
    ttl: number;
    prio?: number;
    disabled?: boolean;
}

export function decodeDNSRecordRows(raw: unknown): DNSRecordRow[] {
    if (!record(raw) || !Array.isArray(raw.records) || raw.records.length > 4096) throw new Error('records outside the contract');
    return raw.records.map((row) => {
        if (!record(row) || typeof row.id !== 'number' || !text(row.name) || !text(row.type) || !text(row.content) || typeof row.ttl !== 'number') throw new Error('record outside the contract');
        return { id: row.id, name: String(row.name), type: String(row.type), content: String(row.content), ttl: row.ttl, prio: typeof row.prio === 'number' ? row.prio : undefined, disabled: row.disabled === true };
    });
}

const bare = (name: string) => name.trim().toLowerCase().replace(/\.$/, '');

/**
 * The one apex MX record "Move mail to …" changes, or why it changes none:
 * no MX, more than one (the owner chooses in the editor), or it already
 * names the new host. Only looks.
 */
export function dnsOverviewMXTarget(records: DNSRecordRow[], zone: string, toHost: string): { record: DNSRecordRow } | { none: 'none' | 'many' | 'done'; count: number } {
    const apex = records.filter(row => row.type === 'MX' && (bare(row.name) === bare(zone) || row.name === '@') && !row.disabled);
    if (apex.some(row => bare(row.content.split(/\s+/).pop() || '') === bare(toHost))) return { none: 'done', count: apex.length };
    if (apex.length === 0) return { none: 'none', count: 0 };
    if (apex.length > 1) return { none: 'many', count: apex.length };
    return { record: apex[0] };
}
