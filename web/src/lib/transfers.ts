import type { TranslationKey } from '../i18n/en';
import { componentName } from './componentName';
import { readApiError, type ApiError } from './apiError';

// Subscription transfer (alpha.117, D-042 section 8): the server's answers
// and how an item of a rights set is named. The server decides everything;
// the screen only shows the preview and sends the confirmed call with a
// request id that stays the same until the answer arrives (D-029).
//
// Abonelik devri: sunucunun yanıtları ve bir hak öğesinin adı. Her şeye
// sunucu karar verir; ekran önizlemeyi gösterir ve onaylı çağrıyı gönderir.

export const TRANSFER_REQUESTS_URL = '/api/v1/transfer-requests';

export const transferURL = (subscriptionID: number, part: '' | '/preview' | '/receivers' = '') =>
    `/api/v1/subscriptions/${subscriptionID}/transfer${part}`;
export const transferRequestURL = (subscriptionID: number) => `/api/v1/subscriptions/${subscriptionID}/transfer-request`;
export const transferRequestActionURL = (id: number, action: 'preview' | 'approve' | 'reject' | 'cancel' | 'seen' | 'rotation-retry') =>
    `${TRANSFER_REQUESTS_URL}/${id}/${action}`;

export interface TransferParty {
    id: number;
    username: string;
    role?: string;
}

export interface TransferReceiver {
    id: number;
    username: string;
    role: string;
    reseller?: string;
}

export interface TransferPoolLine {
    kind: string;
    used: number;
    adds: number;
    limit: number;
    fits: boolean;
}

export interface TransferGrantLine {
    user_id: number;
    username: string;
    subscription_grants: number;
    domain_grants: number;
}

export interface TransferInUse {
    item: string;
    count: number;
    what: string;
}

/** The credentials a transfer changes (owner decision, 2 November 2026). */
export interface TransferRotationPreview {
    site_accounts: string[];
    vpn_devices: string[];
    mailboxes: string[];
    database_users: { username: string; app_domain?: string; host?: string; legacy?: boolean }[];
    protected_users: { domain: string; users: number }[];
    legacy_ftp_accounts: number;
    total: number;
    /** What the new owner does after: mail_clients, database_apps, file_access, vpn_devices, protected_folders. */
    after: string[];
    /** What CelikPanel does not hold: app_admin_accounts, own_app_secrets, legacy_ftp_accounts. */
    not_covered: string[];
}

export interface RotationGuidance {
    result: 'in_progress' | 'done' | 'prerequisite' | 'verified_failure' | 'unknown' | string;
    reason: string;
    who_acts: string;
    next_action: string;
    resumes: string;
}

export interface RotationStep {
    kind: 'site_account' | 'vpn' | 'mailbox' | 'database_user' | 'protected_dir' | string;
    target: string;
    state: 'pending' | 'running' | 'done' | 'failed' | 'manual' | 'replaced' | string;
    detail?: string;
}

/** The change of passwords and keys after a completed transfer. */
export interface TransferRotation {
    id: string;
    state: 'running' | 'completed' | 'failed' | string;
    attempts: number;
    created_at: string;
    finished_at?: string;
    guidance: RotationGuidance;
    counts: { total: number; done: number; pending: number; failed: number; manual: number; replaced: number };
    effects: RotationStep[];
}

export interface TransferPreview {
    subscription_id: number;
    subscription_name: string;
    from: TransferParty;
    to: TransferParty;
    from_reseller: TransferParty | null;
    to_reseller: TransferParty | null;
    same_tree: boolean;
    rights_from: string;
    rights_from_plan_name?: string;
    rights_to: string;
    rights_to_plan_name?: string;
    plan_converted: boolean;
    capped: string[];
    capped_in_use: TransferInUse[];
    widened: string[];
    missing: string[];
    pool: TransferPoolLine[];
    pool_exceeded: string[];
    grants_removed: TransferGrantLine[];
    moves: {
        domains: number;
        sites: number;
        mailboxes: number;
        databases: number;
        database_users: number;
        certificates: number;
        vpn_devices: number;
        pending_vpn_devices: number;
        backup_schedules: number;
        add_ons: number;
        disk_used_bytes: number;
    };
    from_left_without_subscription: boolean;
    receiver_subscriptions_after: number;
    host_changes: string;
    installed_unknown?: boolean;
    rotate_credentials: boolean;
    rotation: TransferRotationPreview;
}

export interface TransferRefusal {
    code: string;
    message: string;
    details?: string[];
}

export interface TransferPreviewAnswer {
    preview: TransferPreview;
    canTransfer: boolean;
    refusal: TransferRefusal | null;
}

export interface TransferRow {
    id: number;
    subscription_id: number;
    subscription_name: string;
    status: 'requested' | 'completed' | 'rejected' | 'cancelled';
    from: TransferParty;
    to: TransferParty | null;
    requested_to: string;
    note: string;
    approver: 'administrator' | 'reseller';
    approver_reseller_id?: number;
    decision_reason: string;
    created_at: string;
    decided_at?: string;
    receiver_seen: boolean;
    requester_seen: boolean;
    /** Made by a customer's request, not a direct transfer. */
    is_request: boolean;
    /** For the caller of the list. */
    to_you?: boolean;
    from_you?: boolean;
    can_decide?: boolean;
    /** Only for those who act on it: the administrator, the new owner and its provider. */
    rotation?: TransferRotation;
    /** The stored preview of the transfer (what it changed). */
    result?: unknown;
}

export interface TransferList {
    rows: TransferRow[];
    unseen: { received: number; decided: number };
    waiting: number;
}

const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const list = <T,>(v: unknown): T[] => (Array.isArray(v) ? (v as T[]) : []);

const previewShape = (raw: unknown): raw is Record<string, unknown> =>
    isRecord(raw) && typeof raw.subscription_id === 'number' && isRecord(raw.from) && isRecord(raw.to) && isRecord(raw.moves);

export function decodeRotationPreview(raw: unknown): TransferRotationPreview {
    const r = isRecord(raw) ? raw : {};
    return {
        site_accounts: list<string>(r.site_accounts),
        vpn_devices: list<string>(r.vpn_devices),
        mailboxes: list<string>(r.mailboxes),
        database_users: list<TransferRotationPreview['database_users'][number]>(r.database_users),
        protected_users: list<TransferRotationPreview['protected_users'][number]>(r.protected_users),
        legacy_ftp_accounts: typeof r.legacy_ftp_accounts === 'number' ? r.legacy_ftp_accounts : 0,
        total: typeof r.total === 'number' ? r.total : 0,
        after: list<string>(r.after),
        not_covered: list<string>(r.not_covered),
    };
}

/** The rotation a row carries, or null when it has none or it is not the contract. */
export function decodeRotation(raw: unknown): TransferRotation | null {
    if (!isRecord(raw) || typeof raw.id !== 'string' || typeof raw.state !== 'string' || !isRecord(raw.guidance) || !isRecord(raw.counts)) return null;
    return { ...(raw as unknown as TransferRotation), effects: list<RotationStep>(raw.effects) };
}

/** The credential list the stored transfer result names (for the new owner's guidance). */
export function rowRotationPreview(row: TransferRow): TransferRotationPreview | null {
    return isRecord(row.result) && isRecord(row.result.rotation) && row.result.rotate_credentials === true
        ? decodeRotationPreview(row.result.rotation)
        : null;
}

function decodePreview(raw: unknown): TransferPreview {
    if (!previewShape(raw)) throw new Error('shape');
    return {
        ...(raw as unknown as TransferPreview),
        capped: list<string>(raw.capped),
        capped_in_use: list<TransferInUse>(raw.capped_in_use),
        widened: list<string>(raw.widened),
        missing: list<string>(raw.missing),
        pool: list<TransferPoolLine>(raw.pool),
        pool_exceeded: list<string>(raw.pool_exceeded),
        grants_removed: list<TransferGrantLine>(raw.grants_removed),
        // An older server sends no rotation: nothing is changed then.
        rotate_credentials: raw.rotate_credentials === true,
        rotation: decodeRotationPreview(raw.rotation),
    };
}

export function decodePreviewAnswer(raw: unknown): TransferPreviewAnswer {
    if (!isRecord(raw)) throw new Error('shape');
    const refusal = isRecord(raw.refusal) && typeof raw.refusal.code === 'string'
        ? { code: raw.refusal.code, message: String(raw.refusal.message ?? ''), details: list<string>(raw.refusal.details) }
        : null;
    return { preview: decodePreview(raw.preview), canTransfer: raw.can_transfer === true && refusal === null, refusal };
}

/**
 * A refused confirmed call: its coded error, and the fresh preview it
 * carries when it has one. A body that is not the contract has no preview;
 * the error says what happened either way.
 */
export async function readTransferRefusal(res: Response): Promise<{ error: ApiError; preview: TransferPreview | null }> {
    const copy = res.clone();
    const error = await readApiError(res);
    const preview = await copy.json().then(
        (raw: unknown) => (isRecord(raw) && previewShape(raw.preview) ? decodePreview(raw.preview) : null),
        // Not the contract: the refusal is shown without a preview.
        (): TransferPreview | null => null,
    );
    return { error, preview };
}

export function decodeReceivers(raw: unknown): TransferReceiver[] {
    if (!isRecord(raw) || !Array.isArray(raw.receivers)) throw new Error('shape');
    return raw.receivers.filter((r): r is TransferReceiver => isRecord(r) && typeof r.id === 'number' && typeof r.username === 'string');
}

export function decodeTransferList(raw: unknown): TransferList {
    if (!isRecord(raw) || !Array.isArray(raw.requests) || !isRecord(raw.unseen)) throw new Error('shape');
    const rows = raw.requests
        .filter((r): r is TransferRow => isRecord(r) && typeof r.id === 'number' && isRecord(r.from))
        .map((r) => ({ ...r, rotation: decodeRotation(r.rotation) ?? undefined }));
    const received = typeof raw.unseen.received === 'number' ? raw.unseen.received : 0;
    const decided = typeof raw.unseen.decided === 'number' ? raw.unseen.decided : 0;
    return { rows, unseen: { received, decided }, waiting: typeof raw.waiting === 'number' ? raw.waiting : 0 };
}

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

// How an item of a rights set reads: a permission by its words, a PHP or
// Node.js line by name and version, any other component by its catalogue
// name. An item this screen has no words for keeps its id.
export function rightsItemLabel(item: string, t: Translate): string {
    if (item.startsWith('permission:')) {
        const key = `transfer.permission.${item.slice('permission:'.length)}` as TranslationKey;
        const said = t(key);
        return said === key ? item : said;
    }
    const parts = item.split(':');
    if (parts[0] === 'component' && parts.length >= 2) {
        const name = parts[1] === 'php-fpm' ? 'PHP' : parts[1] === 'node' ? 'Node.js' : componentName(parts[1], t);
        return parts.length === 3 ? `${name} ${parts[2]}` : name;
    }
    if (parts[0] === 'integration' && parts[1] === 'acme' && parts[2]) {
        return t('transfer.item.acme', { provider: parts[2] });
    }
    return item;
}

/** The refusal codes of a transfer that have their own sentence and way out. */
export const TRANSFER_CODES = [
    'TRANSFER_NOT_ALLOWED', 'TRANSFER_REQUEST_ONLY', 'TRANSFER_NEEDS_ADMINISTRATOR', 'TRANSFER_RECEIVER_INVALID',
    'TRANSFER_SAME_OWNER', 'TRANSFER_ADMIN_WORKSPACE', 'TRANSFER_RIGHTS_OUTSIDE_RECEIVER', 'TRANSFER_POOL_EXCEEDED',
    'TRANSFER_STALE', 'TRANSFER_REQUEST_ID_REUSED', 'TRANSFER_REQUEST_OPEN', 'TRANSFER_REQUEST_CLOSED',
    'TRANSFER_ROTATION_PENDING', 'TRANSFER_ROTATION_RUNNING', 'TRANSFER_ROTATION_NOTHING_TO_RETRY',
] as const;
