// The webmail update check (alpha.107, D-038 §9): GET
// /api/v1/service/roundcube/update. Read by the webmail page's card and the
// Components row alike, through the same address. Since alpha.109 it also
// carries the last webmail update operation (last_update), so a screen
// re-attaches to a running update and says the outcome of the last one.
// Web postası güncelleme denetimi; kart ve satır aynı adresi okur; son
// güncelleme işlemi de gelir.
export const ROUNDCUBE_UPDATE_URL = '/api/v1/service/roundcube/update';

// WEBMAIL_LINK_OFFERED (alpha.109 review): no screen links to webmail while it
// is served only on the panel's own origin, where the panel's script policy
// (which protects the administrator's session) keeps Roundcube's interface
// from starting. It turns on when webmail has its own address.
// Web postası kendi adresine kavuşana dek hiçbir ekran ona bağlantı vermez.
export const WEBMAIL_LINK_OFFERED = false;

export interface WebmailUpdateOperation {
    id: string;
    request_id?: string;
    kind?: string;
    service_id?: string;
    package_name?: string;
    status: 'queued' | 'running' | 'succeeded' | 'failed';
    phase?: string;
    started_at?: string;
    finished_at?: string;
    result?: Record<string, unknown>;
    error?: { code?: string; message?: string } | null;
}

export interface RoundcubeUpdateCheck {
    installed: boolean;
    version?: string;
    line?: string;
    latest_on_line?: string;
    latest_overall?: string;
    update_available: boolean;
    checked_at?: string;
    error?: string;
    /** alpha.109: the newest webmail update operation, or null. */
    last_update?: WebmailUpdateOperation | null;
    /** alpha.109: webmail answers at the panel's /webmail/ address. */
    webmail_ready?: boolean;
    /** alpha.109 review: the check could not be read, but the last update is known. */
    check_unknown?: boolean;
}

const OPERATION_ID = /^[a-f0-9]{32}$/;
const STATUSES: readonly string[] = ['queued', 'running', 'succeeded', 'failed'];

export function decodeWebmailUpdateOperation(raw: unknown): WebmailUpdateOperation | null {
    const value = raw as Record<string, unknown> | null;
    if (!value || typeof value !== 'object' || typeof value.id !== 'string' || !OPERATION_ID.test(value.id) || typeof value.status !== 'string' || !STATUSES.includes(value.status)) return null;
    if (value.kind !== undefined && value.kind !== 'webmail_update') return null;
    return value as unknown as WebmailUpdateOperation;
}

export function decodeRoundcubeUpdate(raw: unknown): RoundcubeUpdateCheck {
    const body = raw as { state?: string; check?: RoundcubeUpdateCheck; last_update?: unknown; webmail_ready?: unknown } | null;
    // The release check could not be read, but the last update is still said
    // (a running one is followed) rather than lost with the check.
    // Denetim okunamasa da son güncelleme söylenir.
    const lastUpdate = body ? decodeWebmailUpdateOperation(body.last_update) : null;
    if (body && body.state === 'unknown' && lastUpdate) {
        return { installed: false, update_available: false, check_unknown: true, last_update: lastUpdate, webmail_ready: body.webmail_ready === true };
    }
    if (!body || body.state !== 'read' || !body.check || typeof body.check.installed !== 'boolean' || typeof body.check.update_available !== 'boolean') {
        throw new Error('shape');
    }
    return { ...body.check, last_update: lastUpdate, webmail_ready: body.webmail_ready === true };
}

export type WebmailUpdateOutcomeKind = 'updated' | 'rolled_back' | 'not_changed' | 'rollback_failed' | 'unknown' | 'interrupted';

export interface WebmailUpdateOutcome {
    outcome: WebmailUpdateOutcomeKind;
    from: string;
    to: string;
    backup: string;
    reason: string;
    code: string;
    previousAnswers?: boolean;
}

const OUTCOMES: readonly string[] = ['updated', 'rolled_back', 'not_changed', 'rollback_failed', 'unknown', 'interrupted'];
const text = (value: unknown) => (typeof value === 'string' ? value : '');

// webmailUpdateOutcome reads a finished operation's result: what happened,
// from and to which release, the backup and the cause. A row without an
// outcome (an older record, a failure before the Agent answered) is said
// from its status: succeeded is updated, failed is not known.
export function webmailUpdateOutcome(op: WebmailUpdateOperation): WebmailUpdateOutcome {
    const result = op.result ?? {};
    let outcome = text(result.outcome);
    if (!OUTCOMES.includes(outcome)) outcome = op.status === 'succeeded' ? 'updated' : 'unknown';
    return {
        outcome: outcome as WebmailUpdateOutcomeKind,
        from: text(result.from),
        to: text(result.to) || op.package_name || '',
        backup: text(result.backup),
        reason: text(result.reason) || (op.error?.message ?? ''),
        code: text(result.code),
        ...(typeof result.previous_answers === 'boolean' ? { previousAnswers: result.previous_answers } : {}),
    };
}

const OPERATION_URL = '/api/v1/service/operation';

// readWebmailUpdateOperation reads one operation by id or request id
// (alpha.109): null when there is none, 'error' when the read failed. A read
// only; it never sends a change.
// Bir işlemi kimliğiyle okur; hiçbir değişiklik göndermez.
export async function readWebmailUpdateOperation(query: string): Promise<WebmailUpdateOperation | null | 'error'> {
    try {
        const res = await fetch(`${OPERATION_URL}?${query}`, { cache: 'no-store' });
        if (res.status === 404) return null;
        if (!res.ok) return 'error';
        const body = (await res.json()) as { operation?: WebmailUpdateOperation | null };
        return body.operation && typeof body.operation.id === 'string' ? body.operation : null;
    } catch {
        return 'error';
    }
}
