// Webmail on its own address (alpha.110). Roundcube is never served on the
// panel's own origin: a script running in it could call the panel's API with
// the signed-in administrator's session. The administrator gives webmail a
// host name of its own (for example mail.example.com); the server's own web
// server answers there with its own certificate. Every screen links to that
// address and to nothing else.
//
// The decoders here fail closed: an answer that is not exactly the contract
// is unknown, never a link and never "no address". isWebmailAddressURL is the
// one test of a link: https, a valid lower-case host name, the root path, and
// nothing more (no port, no user name, no query, no fragment).
//
// Web postası kendi adresinde: panelin origin'inde Roundcube sunulmaz. Her ekran
// yalnız bu adrese bağlantı verir; çözücüler kapalı başarısız olur.

export const WEBMAIL_ADDRESS_URL = '/api/v1/service/roundcube/address';
export const WEBMAIL_ADDRESS_CHECK_URL = `${WEBMAIL_ADDRESS_URL}/check`;
export const WEBMAIL_ADDRESS_REMOVE_URL = `${WEBMAIL_ADDRESS_URL}/remove`;

// A host name as the server stores it: two or more lower-case labels of
// letters, digits and inner hyphens, each at most 63 characters, 253 in all,
// and a last label that is not only digits (so an IP address is never one).
const LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

export function isWebmailHostName(value: unknown): value is string {
    if (typeof value !== 'string' || value.length === 0 || value.length > 253) return false;
    const labels = value.split('.');
    if (labels.length < 2) return false;
    if (!labels.every((label) => LABEL.test(label))) return false;
    return /[a-z-]/.test(labels[labels.length - 1]);
}

// Only https://<host>/ exactly. Anything else (a path such as /webmail/, http,
// a port, user information, a query, a fragment, another scheme) is not a link.
export function isWebmailAddressURL(value: unknown): value is string {
    if (typeof value !== 'string' || !value.startsWith('https://') || !value.endsWith('/')) return false;
    return isWebmailHostName(value.slice('https://'.length, -1));
}

export const webmailAddressURLFor = (host: string): string => `https://${host}/`;

// What a screen holds of the address link: the valid URL, or null.
export function webmailLinkOrNull(value: unknown): string | null {
    return isWebmailAddressURL(value) ? value : null;
}

const text = (value: unknown) => (typeof value === 'string' ? value : '');
const textList = (value: unknown): string[] | null => (Array.isArray(value) && value.every((item) => typeof item === 'string') ? (value as string[]) : null);

// --- The durable operation (kinds webmail_address and webmail_address_remove) ---

export type WebmailAddressOperationKind = 'webmail_address' | 'webmail_address_remove';

export interface WebmailAddressOperation {
    id: string;
    request_id?: string;
    kind: WebmailAddressOperationKind;
    service_id?: string;
    package_name?: string;
    status: 'queued' | 'running' | 'succeeded' | 'failed';
    phase?: string;
    started_at?: string;
    finished_at?: string;
    result?: Record<string, unknown>;
    error?: { code?: string; message?: string } | null;
}

const OPERATION_ID = /^[a-f0-9]{32}$/;
const STATUSES: readonly string[] = ['queued', 'running', 'succeeded', 'failed'];
const KINDS: readonly string[] = ['webmail_address', 'webmail_address_remove'];

export function decodeWebmailAddressOperation(raw: unknown): WebmailAddressOperation | null {
    const value = raw as Record<string, unknown> | null;
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    if (typeof value.id !== 'string' || !OPERATION_ID.test(value.id)) return null;
    if (typeof value.status !== 'string' || !STATUSES.includes(value.status)) return null;
    if (typeof value.kind !== 'string' || !KINDS.includes(value.kind)) return null;
    return value as unknown as WebmailAddressOperation;
}

// --- GET /api/v1/service/roundcube/address ---

export interface WebmailAddressState {
    host: string;
    url: string;
    enabledAt: string;
}

export interface WebmailAddressStatus {
    present: boolean;
    managed: boolean;
    certificateExpires: string;
    // proved: the server asked webmail over HTTPS on its address and it answered.
    // The server leaves the field out when it did not (and when no address is
    // set, so it asked nothing); a screen reads it only for a set address.
    proved: boolean;
    proofError: string;
    error: string;
}

export interface WebmailAddressRead {
    address: WebmailAddressState | null;
    suggestion: string;
    expected: string[];
    // The server's own state could not be read (the agent did not answer).
    statusUnknown: boolean;
    status: WebmailAddressStatus | null;
    lastOperation: WebmailAddressOperation | null;
}

function decodeStatus(raw: unknown): WebmailAddressStatus | null {
    const value = raw as Record<string, unknown> | null;
    if (!value || typeof value !== 'object' || Array.isArray(value) || typeof value.present !== 'boolean') return null;
    return {
        present: value.present,
        managed: value.managed === true,
        certificateExpires: text(value.certificate_expires),
        proved: value.proved === true,
        proofError: text(value.proof_error),
        error: text(value.error),
    };
}

// Throws on anything that is not the contract: the read is then "could not be
// checked", and no link, no form that depends on it, is drawn.
export function decodeWebmailAddress(raw: unknown): WebmailAddressRead {
    const body = raw as Record<string, unknown> | null;
    if (!body || typeof body !== 'object' || Array.isArray(body) || !('address' in body)) throw new Error('webmail address');
    let address: WebmailAddressState | null = null;
    if (body.address !== null) {
        const value = body.address as Record<string, unknown>;
        if (!value || typeof value !== 'object' || !isWebmailHostName(value.host) || value.url !== webmailAddressURLFor(value.host)) {
            throw new Error('webmail address');
        }
        address = { host: value.host, url: value.url as string, enabledAt: text(value.enabled_at) };
    }
    const expected = textList(body.expected);
    if (!expected) throw new Error('webmail address');
    const suggestion = isWebmailHostName(body.suggestion) ? body.suggestion : '';
    const lastRaw = body.last_operation ?? null;
    const lastOperation = lastRaw === null ? null : decodeWebmailAddressOperation(lastRaw);
    if (lastRaw !== null && lastOperation === null) throw new Error('webmail address');
    const statusUnknown = body.status_unknown === true;
    const status = statusUnknown ? null : decodeStatus(body.status);
    return { address, suggestion, expected, statusUnknown: statusUnknown || (address !== null && status === null), status, lastOperation };
}

// --- POST /api/v1/service/roundcube/address/check ---

export type WebmailCheckState = 'ready' | 'not_found' | 'elsewhere' | 'unknown' | 'refused';
const CHECK_STATES: readonly string[] = ['ready', 'not_found', 'elsewhere', 'unknown', 'refused'];

export interface WebmailAddressRecord {
    type: 'A' | 'AAAA';
    name: string;
    content: string;
    ttl: number;
    text: string;
}

export type WebmailFirewallState = 'open' | 'closed' | 'off' | 'unknown';

export interface WebmailAddressCheck {
    host: string;
    state: WebmailCheckState;
    code: string;
    message: string;
    expected: string[];
    answered: string[];
    peer: { name: string; address: string } | null;
    records: WebmailAddressRecord[];
    // Present when this panel hosts the DNS zone and can add the records after a confirmation.
    zone: { name: string; recordsURL: string } | null;
    firewall: { state: WebmailFirewallState; closed: number[] };
    current: string;
    ready: boolean;
    warnings: string[];
    ownerAction: { who: string; next: string } | null;
}

// The only record endpoints the panel's own zones are edited through; the
// check's zone link is followed only when it is one of these.
const RECORDS_URL = /^\/api\/v1\/(?:domains\/\d+\/dns\/records|dns\/infrastructure-zones\/[a-z0-9.-]+\/records)$/;

export function decodeWebmailAddressCheck(raw: unknown): WebmailAddressCheck | null {
    const body = raw as Record<string, unknown> | null;
    if (!body || typeof body !== 'object' || Array.isArray(body)) return null;
    const value = body.check as Record<string, unknown> | null | undefined;
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    if (typeof value.host !== 'string' || typeof value.state !== 'string' || !CHECK_STATES.includes(value.state) || typeof value.ready !== 'boolean') return null;
    const expected = textList(value.expected ?? []);
    const answered = textList(value.answered ?? []);
    const warnings = textList(value.warnings ?? []);
    if (!expected || !answered || !warnings) return null;
    // A check that says "ready" for anything but a ready state is not trusted.
    if (value.ready && value.state !== 'ready') return null;
    const records: WebmailAddressRecord[] = [];
    if (value.records !== undefined) {
        if (!Array.isArray(value.records)) return null;
        for (const item of value.records as Record<string, unknown>[]) {
            if (!item || (item.type !== 'A' && item.type !== 'AAAA') || typeof item.name !== 'string' || typeof item.content !== 'string' || typeof item.ttl !== 'number' || typeof item.text !== 'string') return null;
            records.push({ type: item.type, name: item.name, content: item.content, ttl: item.ttl, text: item.text });
        }
    }
    let zone: WebmailAddressCheck['zone'] = null;
    if (value.zone !== undefined && value.zone !== null) {
        const z = value.zone as Record<string, unknown>;
        // A zone the panel cannot be trusted to name leaves out the shortcut; the record text stays.
        if (typeof z.name === 'string' && typeof z.records_url === 'string' && RECORDS_URL.test(z.records_url)) zone = { name: z.name, recordsURL: z.records_url };
    }
    const fw = (value.firewall ?? {}) as Record<string, unknown>;
    const fwState: WebmailFirewallState = fw.state === 'open' || fw.state === 'closed' || fw.state === 'off' ? fw.state : 'unknown';
    const closed = Array.isArray(fw.closed) ? (fw.closed as unknown[]).filter((port): port is number => typeof port === 'number') : [];
    const peerRaw = value.peer as Record<string, unknown> | null | undefined;
    const peer = peerRaw && typeof peerRaw.address === 'string' ? { name: text(peerRaw.name), address: peerRaw.address } : null;
    const ownerRaw = value.owner_action as Record<string, unknown> | null | undefined;
    const ownerAction = ownerRaw && typeof ownerRaw === 'object' ? { who: text(ownerRaw.who), next: text(ownerRaw.next) } : null;
    return {
        host: value.host,
        state: value.state as WebmailCheckState,
        code: text(value.code),
        message: text(value.message),
        expected,
        answered,
        peer,
        records,
        zone,
        firewall: { state: fwState, closed },
        current: text(value.current),
        ready: value.ready,
        warnings,
        ownerAction,
    };
}

// --- What an enable or a remove operation ended as ---

export type WebmailAddressOutcomeKind = 'enabled' | 'removed' | 'not_enabled' | 'not_removed' | 'rollback_failed' | 'not_recorded' | 'unknown' | 'interrupted';

export interface WebmailAddressOutcome {
    remove: boolean;
    outcome: WebmailAddressOutcomeKind;
    host: string;
    url: string;
    certificateExpires: string;
    certificateRemoved: boolean;
    warning: string;
    roundcubeSettings: string;
    code: string;
    reason: string;
    rolledBack: boolean;
    errorCode: string;
}

const ENABLE_OUTCOMES: readonly string[] = ['enabled', 'not_enabled', 'rollback_failed', 'unknown', 'not_recorded', 'interrupted'];
const REMOVE_OUTCOMES: readonly string[] = ['removed', 'not_removed', 'unknown', 'interrupted'];

// webmailAddressOutcome reads a finished operation. An unreadable result is
// unknown: success is claimed only for a succeeded operation whose own result
// says so.
export function webmailAddressOutcome(op: WebmailAddressOperation): WebmailAddressOutcome {
    const result = op.result ?? {};
    const remove = op.kind === 'webmail_address_remove';
    let outcome = text(result.outcome);
    if (!(remove ? REMOVE_OUTCOMES : ENABLE_OUTCOMES).includes(outcome)) outcome = 'unknown';
    if (op.status === 'succeeded' && result.success !== true) outcome = 'unknown';
    if (op.status === 'failed' && (outcome === 'enabled' || outcome === 'removed')) outcome = 'unknown';
    if (op.status === 'failed' && op.error?.code === 'webmail_address_interrupted') outcome = 'interrupted';
    const host = text(result.host) || op.package_name || '';
    const url = text(result.url);
    return {
        remove,
        outcome: outcome as WebmailAddressOutcomeKind,
        host,
        url: isWebmailAddressURL(url) ? url : '',
        certificateExpires: text(result.certificate_expires),
        certificateRemoved: result.certificate_removed === true,
        warning: text(result.warning),
        roundcubeSettings: text(result.roundcube_settings),
        code: text(result.code),
        reason: text(result.reason) || (op.error?.message ?? ''),
        rolledBack: result.rolled_back === true,
        errorCode: op.error?.code ?? '',
    };
}
