import type { TranslationKey } from '../i18n/en';

// What deletions left, kept 30 days (alpha.135; cmd/panel/deletion_holding.go):
// the list's address, its decoder and the words a row uses for what it is.
// Silmelerden kalanlar: listenin adresi, çözücüsü ve satırın adı.

export const DELETION_HOLDING_URL = '/api/v1/maintenance/deletion-holding';

export interface HeldItem {
    id: string;
    kind: 'mail' | 'backups';
    domain: string;
    domainId: number;
    mailbox: string;
    /** -1: not known. */
    bytes: number;
    files: number;
    deletedAt: string;
    removeAfter: string;
    adopted: boolean;
    path: string;
    state: 'kept' | 'waiting' | 'failed' | 'unknown';
    failureLine: string;
    nextTryAt: string;
}

export interface DeletionHolding {
    items: HeldItem[];
    summary: { count: number; bytes: number; earliest: string };
}

const STATES = new Set(['kept', 'waiting', 'failed', 'unknown']);
const text = (value: unknown) => (typeof value === 'string' ? value : '');
const number = (value: unknown, fallback = 0) => (typeof value === 'number' && Number.isFinite(value) ? value : fallback);

/** The list as the server answers it; a body that is not this shape is unknown. */
export function decodeDeletionHolding(raw: unknown): DeletionHolding {
    const body = raw as { items?: unknown; summary?: Record<string, unknown> } | null;
    if (!body || !Array.isArray(body.items) || !body.summary || typeof body.summary !== 'object') throw new Error('shape');
    const items = body.items.map((value): HeldItem => {
        const item = (value ?? {}) as Record<string, unknown>;
        const kind = item.kind === 'mail' || item.kind === 'backups' ? item.kind : null;
        const state = STATES.has(text(item.state)) ? text(item.state) as HeldItem['state'] : null;
        if (!kind || !state || !text(item.id) || Number.isNaN(Date.parse(text(item.remove_after)))) throw new Error('shape');
        const failure = item.failure as Record<string, unknown> | undefined;
        return {
            id: text(item.id), kind, domain: text(item.domain), domainId: number(item.domain_id), mailbox: text(item.mailbox),
            bytes: number(item.bytes, -1), files: number(item.files), deletedAt: text(item.deleted_at), removeAfter: text(item.remove_after),
            adopted: item.adopted === true, path: text(item.path), state,
            failureLine: failure ? text(failure.line) : '', nextTryAt: text(item.next_try_at),
        };
    });
    return {
        items,
        summary: { count: number(body.summary.count), bytes: number(body.summary.bytes), earliest: text(body.summary.earliest_remove_after) },
    };
}

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** What a row is: "Mail of example.com", "Mail of info@example.com", "Backups of example.com (3 files)". */
export function holdingItemWhat(item: Pick<HeldItem, 'kind' | 'domain' | 'domainId' | 'mailbox' | 'files'>, t: Translate): string {
    const domain = item.domain || t('holding.domainUnknown', { id: item.domainId || '?' });
    if (item.kind === 'backups') return t('holding.what.backups', { domain, n: item.files });
    return item.mailbox ? t('holding.what.mailbox', { mailbox: item.mailbox }) : t('holding.what.mail', { domain });
}

/** A held backups item's files (name and size); a body that is not this shape is unknown. */
export function decodeHeldBackupFiles(raw: unknown): { name: string; size: number }[] {
    const body = raw as { backups?: unknown } | null;
    if (!body || !Array.isArray(body.backups)) throw new Error('shape');
    return body.backups.map((value) => {
        const file = (value ?? {}) as Record<string, unknown>;
        if (typeof file.name !== 'string' || typeof file.size !== 'number') throw new Error('shape');
        return { name: file.name, size: file.size };
    });
}
