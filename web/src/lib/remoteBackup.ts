// Remote backups (alpha.118, W4): what the screens read from the Panel and the
// few decisions they make themselves. The server enforces every rule; these
// helpers only decide what a screen shows or offers (D-024, D-042).
//
// cmd/panel/remote_backup_handlers.go is the contract: no answer carries a
// secret or the passphrase, so nothing here ever holds one that came from the
// server. A passphrase a person types or generates stays in the form until it
// is sent once, on creation.
//
// Uzak yedekler: ekranların Panel'den okuduğu ve kendi verdiği birkaç karar.
// Kuralları sunucu uygular; bunlar yalnız ekranın ne göstereceğini belirler.
// Hiçbir yanıt gizli bilgi ya da parola cümlesi taşımaz.

export const REMOTE_DESTINATIONS_URL = '/api/v1/remote-backup/destinations';
export const REMOTE_DESTINATION_TEST_URL = '/api/v1/remote-backup/destinations/test';
export const REMOTE_POLICIES_URL = '/api/v1/remote-backup/policies';
export const siteRemoteURL = (domainId: number) => `/api/v1/domains/${domainId}/backups/remote`;

/** guidance.result: what is known about an operation (D-024). */
export type GuidanceResult = 'waiting' | 'in_progress' | 'retrying' | 'done' | 'prerequisite' | 'verified_failure' | 'unknown';
/** The server's five; 'you' is the screen's own, for a form the person corrects. */
export type WhoActs = 'none' | 'panel' | 'destination_owner' | 'administrator' | 'account_owner' | 'you';

export interface RemoteGuidance {
    result: GuidanceResult;
    code: string;
    reason: string;
    whoActs: WhoActs;
    nextAction: string;
    resumes: string;
}

export interface RemoteDestinationConfig {
    endpoint?: string;
    region?: string;
    bucket?: string;
    prefix?: string;
    access_key_id?: string;
    virtual_hosted?: boolean;
    host?: string;
    port?: number;
    user?: string;
    path?: string;
}

export interface RemoteDestination {
    id: number;
    name: string;
    kind: 's3' | 'sftp' | string;
    scope: 'server' | 'own' | 'reseller' | string;
    manageable: boolean;
    config?: RemoteDestinationConfig;
    hasSecret: boolean;
    passphraseCheck: string;
    hostKeyFingerprint: string;
    offeredToAccounts: boolean;
    lastTestAt: string;
    lastTestResult: string;
    location: string;
}

export interface DestinationsAnswer {
    destinations: RemoteDestination[];
    canAdd: boolean;
    /** '' when the caller may add; else the layer that refuses: administrator | reseller | subscription | role. */
    addRefusedBy: string;
    addScope: 'server' | 'own' | string;
    minPassphrase: number;
}

export interface RemotePolicy {
    id: number;
    destinationId: number;
    destinationName: string;
    everySite: boolean;
    frequency: 'daily' | 'weekly';
    backupType: 'files' | 'full';
    keepDaily: number;
    keepWeekly: number;
    enabled: boolean;
    editable: boolean;
}

export interface RemoteOperation {
    id: string;
    kind: 'upload' | 'restore' | string;
    destinationId: number;
    backupName: string;
    backupType: string;
    objectKey: string;
    scheduled: boolean;
    state: 'queued' | 'running' | 'succeeded' | 'failed' | string;
    phase: string;
    bytesDone: number;
    bytesTotal: number;
    attempts: number;
    nextAttemptAt: string;
    createdAt: string;
    finishedAt: string;
    guidance: RemoteGuidance;
}

export interface SiteRemoteOverview {
    destinations: RemoteDestination[];
    policies: RemotePolicy[];
    operations: RemoteOperation[];
    canAddDestination: boolean;
    addRefusedBy: string;
}

export interface RemoteObject {
    objectKey: string;
    backupName: string;
    type: string;
    group: 'manual' | 'schedule' | string;
    createdAt: string;
    size: number;
    hasSidecar: boolean;
    otherServer: boolean;
}

export interface RemoteTestStep {
    step: 'connect' | 'write' | 'read' | 'delete' | string;
    ok: boolean;
}

export interface RemoteHostKey {
    key: string;
    fingerprint: string;
}

export interface RemoteTestResult {
    ok: boolean;
    steps: RemoteTestStep[];
    hostKey: RemoteHostKey | null;
    guidance: RemoteGuidance | null;
}

// ----- decoders: an answer that is not the contract is "unknown" -----

type Raw = Record<string, unknown>;

const isObject = (value: unknown): value is Raw => !!value && typeof value === 'object' && !Array.isArray(value);
const str = (value: unknown): string => (typeof value === 'string' ? value : '');
const num = (value: unknown): number => (typeof value === 'number' && Number.isFinite(value) ? value : 0);
const list = (value: unknown): unknown[] => {
    if (value === null || value === undefined) return [];
    if (!Array.isArray(value)) throw new Error('shape');
    return value;
};

const GUIDANCE_RESULTS: readonly GuidanceResult[] = ['waiting', 'in_progress', 'retrying', 'done', 'prerequisite', 'verified_failure', 'unknown'];
const WHO: readonly string[] = ['none', 'panel', 'destination_owner', 'administrator', 'account_owner'];

export function decodeGuidance(raw: unknown): RemoteGuidance | null {
    if (!isObject(raw)) return null;
    const result = GUIDANCE_RESULTS.includes(raw.result as GuidanceResult) ? raw.result as GuidanceResult : 'unknown';
    const who = typeof raw.who_acts === 'string' && WHO.includes(raw.who_acts) ? raw.who_acts as WhoActs : 'administrator';
    return {
        result, code: str(raw.code), reason: str(raw.reason), whoActs: who,
        nextAction: str(raw.next_action), resumes: str(raw.resumes),
    };
}

function decodeDestination(raw: unknown): RemoteDestination {
    if (!isObject(raw) || typeof raw.id !== 'number' || typeof raw.name !== 'string' || typeof raw.kind !== 'string') {
        throw new Error('destination');
    }
    return {
        id: raw.id, name: raw.name, kind: raw.kind, scope: str(raw.scope), manageable: raw.manageable === true,
        config: isObject(raw.config) ? raw.config as RemoteDestinationConfig : undefined,
        hasSecret: raw.has_secret === true, passphraseCheck: str(raw.passphrase_check),
        hostKeyFingerprint: str(raw.host_key_fingerprint), offeredToAccounts: raw.offered_to_accounts === true,
        lastTestAt: str(raw.last_test_at), lastTestResult: str(raw.last_test_result), location: str(raw.location),
    };
}

export function decodeDestinations(raw: unknown): DestinationsAnswer {
    if (!isObject(raw) || !('destinations' in raw) || typeof raw.can_add !== 'boolean') throw new Error('shape');
    return {
        destinations: list(raw.destinations).map(decodeDestination),
        canAdd: raw.can_add, addRefusedBy: str(raw.add_refused_by), addScope: str(raw.add_scope),
        minPassphrase: num(raw.min_passphrase) || 12,
    };
}

function decodePolicy(raw: unknown): RemotePolicy {
    if (!isObject(raw) || typeof raw.destination_id !== 'number') throw new Error('policy');
    return {
        id: num(raw.id), destinationId: raw.destination_id, destinationName: str(raw.destination_name),
        everySite: raw.every_site === true, frequency: raw.frequency === 'weekly' ? 'weekly' : 'daily',
        backupType: raw.backup_type === 'full' ? 'full' : 'files', keepDaily: num(raw.keep_daily),
        keepWeekly: num(raw.keep_weekly), enabled: raw.enabled === true, editable: raw.editable === true,
    };
}

export function decodePolicies(raw: unknown): RemotePolicy[] {
    if (!isObject(raw) || !('policies' in raw)) throw new Error('shape');
    return list(raw.policies).map(decodePolicy);
}

export function decodeOperation(raw: unknown): RemoteOperation {
    if (!isObject(raw) || typeof raw.id !== 'string' || typeof raw.state !== 'string') throw new Error('operation');
    return {
        id: raw.id, kind: str(raw.kind), destinationId: num(raw.destination_id), backupName: str(raw.backup_name),
        backupType: str(raw.backup_type), objectKey: str(raw.object_key), scheduled: raw.scheduled === true,
        state: raw.state, phase: str(raw.phase), bytesDone: num(raw.bytes_done), bytesTotal: num(raw.bytes_total),
        attempts: num(raw.attempts), nextAttemptAt: str(raw.next_attempt_at), createdAt: str(raw.created_at),
        finishedAt: str(raw.finished_at),
        guidance: decodeGuidance(raw.guidance) ?? { result: 'unknown', code: '', reason: '', whoActs: 'administrator', nextAction: '', resumes: '' },
    };
}

export function decodeSiteOverview(raw: unknown): SiteRemoteOverview {
    if (!isObject(raw) || !('destinations' in raw) || !('operations' in raw) || typeof raw.can_add_destination !== 'boolean') {
        throw new Error('shape');
    }
    return {
        destinations: list(raw.destinations).map(decodeDestination),
        policies: list(raw.policies).map(decodePolicy),
        operations: list(raw.operations).map(decodeOperation),
        canAddDestination: raw.can_add_destination, addRefusedBy: str(raw.add_refused_by),
    };
}

export function decodeObjects(raw: unknown): RemoteObject[] {
    if (!isObject(raw) || !('objects' in raw)) throw new Error('shape');
    return list(raw.objects).map((item) => {
        if (!isObject(item) || typeof item.object_key !== 'string') throw new Error('object');
        return {
            objectKey: item.object_key, backupName: str(item.backup_name), type: str(item.type), group: str(item.group),
            createdAt: str(item.created_at), size: num(item.size), hasSidecar: item.has_sidecar === true,
            otherServer: item.other_server === true,
        };
    });
}

// The host key travels as remotebackup.SFTPHostKey, a Go struct without JSON
// tags, so its fields arrive as "Key" and "Fingerprint"; the lower-case names
// are read too, should the server add tags later.
// Sunucu anahtarı etiketsiz bir Go yapısıdır; alanlar büyük harfle gelir.
function decodeHostKey(raw: unknown): RemoteHostKey | null {
    if (!isObject(raw)) return null;
    const fingerprint = str(raw.fingerprint) || str(raw.Fingerprint);
    if (!fingerprint) return null;
    return { key: str(raw.key) || str(raw.Key), fingerprint };
}

export function decodeTestResult(raw: unknown): RemoteTestResult {
    if (!isObject(raw) || typeof raw.ok !== 'boolean') throw new Error('test');
    return {
        ok: raw.ok,
        steps: list(raw.steps).filter(isObject).map((step) => ({ step: str(step.step), ok: step.ok === true })),
        hostKey: decodeHostKey(raw.host_key),
        guidance: decodeGuidance(raw.guidance),
    };
}

// ----- guidance codes -----

// Every code the remote backup code can give a screen, with a sentence of
// its own in both languages (rbak.code.<CODE>.what / .todo).
// Uzak yedeklerin ekrana verebileceği her kod; iki dilde kendi cümlesiyle.
export const REMOTE_GUIDANCE_CODES = [
    'REMOTE_CREDENTIALS', 'REMOTE_PERMISSION', 'REMOTE_NOT_FOUND', 'REMOTE_HOST_KEY', 'REMOTE_HOST_KEY_CONFIRM',
    'REMOTE_FULL', 'SERVER_CLOCK', 'REMOTE_SETTINGS', 'REMOTE_UNREACHABLE', 'REMOTE_TIMED_OUT', 'REMOTE_INTEGRITY',
    'DECRYPT_FAILED', 'PANEL_RESTARTED', 'LOCAL_BACKUP_FAILED', 'LOCAL_READ_FAILED', 'LOCAL_BACKUP_UNREADABLE',
    'LOCAL_BACKUP_MISSING', 'LOCAL_IMPORT_FAILED', 'LOCAL_IMPORT_REFUSED', 'LEGACY_BACKUP', 'DISK_QUOTA',
    'DATABASE_GONE', 'DESTINATION_REMOVED', 'DESTINATION_NOT_AVAILABLE', 'SITE_REMOVED', 'SUBSCRIPTION_SUSPENDED',
    'RESTORE_IN_PROGRESS', 'RESTORE_FAILED', 'RESTORE_RESULT_UNKNOWN', 'NOT_RESTORABLE', 'WRONG_SITE', 'REMOTE_KEY',
    'REMOTE_PASSPHRASE', 'REMOTE_PASSPHRASE_FIXED', 'REMOTE_SCHEDULE', 'REMOTE_SCHEDULE_ADMINISTRATOR',
    'REMOTE_DESTINATION_BUSY', 'CONFIRM_REQUIRED',
] as const;
export type RemoteGuidanceCode = (typeof REMOTE_GUIDANCE_CODES)[number];

export function isRemoteGuidanceCode(code: string | undefined): code is RemoteGuidanceCode {
    return !!code && (REMOTE_GUIDANCE_CODES as readonly string[]).includes(code);
}

/** The catalogue keys of a code's sentences; null for a code without sentences of its own. */
export function guidanceCodeKeys(code: string | undefined): { what: string; todo: string } | null {
    if (!isRemoteGuidanceCode(code)) return null;
    return { what: `rbak.code.${code}.what`, todo: `rbak.code.${code}.todo` };
}

/**
 * How a result is drawn. Three failure classes stay apart (D-024): a verified
 * failure, an unmet prerequisite and a result that is not known.
 */
export type GuidanceTone = 'progress' | 'done' | 'prerequisite' | 'verified_failure' | 'unknown';

export function guidanceTone(result: GuidanceResult): GuidanceTone {
    switch (result) {
        case 'waiting':
        case 'in_progress':
        case 'retrying':
            return 'progress';
        case 'done':
        case 'prerequisite':
        case 'verified_failure':
            return result;
        default:
            return 'unknown';
    }
}

/**
 * Who acts on a refused change. The server's guidance names it for an
 * operation; a refusal carries only a code, so the screen says it: a form the
 * person filled in is theirs to correct ('you', a screen-only value).
 */
export function refusalWho(code: string): WhoActs {
    switch (code) {
        case 'REMOTE_SETTINGS': case 'REMOTE_PASSPHRASE': case 'REMOTE_PASSPHRASE_FIXED': case 'REMOTE_SCHEDULE':
        case 'CONFIRM_REQUIRED': case 'DESTINATION_NOT_AVAILABLE': case 'REMOTE_HOST_KEY_CONFIRM':
            return 'you';
        case 'REMOTE_DESTINATION_BUSY': case 'RESTORE_IN_PROGRESS':
            return 'none';
        case 'REMOTE_SCHEDULE_ADMINISTRATOR': case 'SERVER_CLOCK': case 'REMOTE_UNREACHABLE': case 'REMOTE_TIMED_OUT':
            return 'administrator';
        case 'DISK_QUOTA': case 'SUBSCRIPTION_SUSPENDED':
            return 'account_owner';
    }
    return 'destination_owner';
}

/** A refused change's class: a store failure names it in `reason`; everything else is a prerequisite. */
export function refusalResult(reason: string | undefined): GuidanceResult {
    return reason === 'verified_failure' || reason === 'unknown' || reason === 'prerequisite' ? reason : 'prerequisite';
}

// ----- operations -----

export const operationActive = (op: Pick<RemoteOperation, 'state'>): boolean => op.state === 'queued' || op.state === 'running';

/** Percent of bytes done, or null while the total is not known. */
export function progressPercent(done: number, total: number): number | null {
    if (!(total > 0)) return null;
    return Math.max(0, Math.min(100, Math.floor((done / total) * 100)));
}

// The names the backup engine generates (remotebackup.ValidBackupName): only
// those can be sent. A backup made before the current format has another name
// and is not offered.
// Yalnız yedekleme motorunun ürettiği adlar gönderilebilir.
const BACKUP_NAME = /^(files|full)-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{16}\.cpbak$|^database-[1-9][0-9]*-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{16}\.cpbak$/;

export const remoteEligibleBackupName = (name: string): boolean => BACKUP_NAME.test(name);

// ----- schedules -----

export const MAX_KEEP_DAILY = 90;
export const MAX_KEEP_WEEKLY = 104;

/** The catalogue key of what is wrong with a schedule's keep counts, or null. */
export function keepCountProblem(daily: number, weekly: number): string | null {
    if (!Number.isInteger(daily) || daily < 0 || daily > MAX_KEEP_DAILY) return 'rbak.sched.keepDailyRange';
    if (!Number.isInteger(weekly) || weekly < 0 || weekly > MAX_KEEP_WEEKLY) return 'rbak.sched.keepWeeklyRange';
    if (daily + weekly === 0) return 'rbak.sched.keepNone';
    return null;
}

// ----- passphrases -----

// 32 letters and digits that are not mistaken for one another when written
// by hand (no i, l, 0, 1), so one byte gives one character without bias.
// Elle yazarken karışmayan 32 karakter.
const PASSPHRASE_ALPHABET = 'abcdefghjkmnopqrstuvwxyz23456789';

/** Six groups of five: 150 bits, easy to write down. Made by the browser's cryptographic generator. */
export function generatePassphrase(groups = 6): string {
    const bytes = new Uint8Array(groups * 5);
    crypto.getRandomValues(bytes);
    const chars = Array.from(bytes, (byte) => PASSPHRASE_ALPHABET[byte & 31]);
    const out: string[] = [];
    for (let i = 0; i < groups; i++) out.push(chars.slice(i * 5, i * 5 + 5).join(''));
    return out.join('-');
}

// ----- times -----

// SQLite's datetime('now') is UTC without a zone ("2026-11-03 10:00:00").
// SQLite zamanı bölgesiz UTC'dir.
export function serverTime(value: string): Date | null {
    if (!value) return null;
    const text = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? value.replace(' ', 'T') + 'Z' : value;
    const date = new Date(text);
    if (Number.isNaN(date.getTime()) || date.getUTCFullYear() < 2000) return null;
    return date;
}

export function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

/** The one-line command that opens a remote copy without CelikPanel. */
export const GPG_OPEN_COMMAND = 'gpg --batch --pinentry-mode loopback --passphrase-file PASSFILE --output BACKUP.cpbak --decrypt BACKUP.cpbak.gpg && tar -xzf BACKUP.cpbak';
