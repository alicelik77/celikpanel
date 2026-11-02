import { decodeListIn, lastKnown, useRemote } from './remote';
export type { SuspendedAccount } from './api';

// Subscription suspension (owner, 2 Nov 2026): when a subscription ends or is
// suspended, its sites show "This site is suspended" and no service works. The
// server runs the work in the background; these are the shapes it answers with
// and the one decoder per address, for every screen that reads them. An answer
// that is not the contract is unknown, never "not suspended".
//
// Abonelik askıya alma: abonelik bittiğinde ya da askıya alındığında siteler
// "Bu site askıya alındı" gösterir ve hiçbir hizmet çalışmaz. İşi sunucu arka
// planda yürütür. Sözleşme olmayan yanıt bilinmeyendir, "askıda değil" değildir.

export type SuspensionEffectKind = 'web' | 'site_account' | 'mail' | 'database' | 'vpn';
export type SuspensionEffectState = 'pending' | 'captured' | 'done' | 'kept' | 'failed' | 'manual';
export interface SuspensionEffect { kind: SuspensionEffectKind; target: string; state: SuspensionEffectState; detail?: string }

export type SuspensionResult = 'in_progress' | 'done' | 'verified_failure' | 'prerequisite' | 'unknown';
export type SuspensionWhoActs = 'none' | 'panel' | 'administrator' | 'reseller';
export interface SuspensionGuidance {
    result: SuspensionResult;
    reason: string;
    who_acts: SuspensionWhoActs;
    next_action: string;
    resumes: string;
}

export interface SuspensionOperation {
    id: string;
    kind: 'suspend' | 'resume';
    trigger: 'manual' | 'reseller' | 'expired';
    state: 'running' | 'completed' | 'failed';
    attempts: number;
    created_at: string;
    finished_at?: string;
    guidance: SuspensionGuidance;
    effects: SuspensionEffect[];
}

export interface SuspensionPreview {
    action: 'suspend' | 'resume';
    sites: string[];
    mail_domains: string[];
    mailboxes: number;
    database_users: string[];
    vpn_peers: number;
    site_accounts: string[];
    notes: string[];
}

export interface SuspensionCauses { manual: boolean; reseller_id?: number; reseller?: string; expired: boolean }

export interface SubscriptionSuspensionView {
    subscription_id: number;
    subscription_name: string;
    owner_id: number;
    owner: string;
    status: 'active' | 'suspended' | 'expired';
    causes: SuspensionCauses;
    reason?: string;
    suspended_at?: string;
    ends_at?: string | null;
    operation?: SuspensionOperation | null;
    preview: SuspensionPreview;
    can_suspend: boolean;
    can_resume: boolean;
    resume_blocked?: string;
}

export interface AccountSuspensionView {
    user_id: number;
    username: string;
    role: 'customer' | 'reseller';
    reseller_suspended: boolean;
    reason?: string;
    suspended_at?: string;
    subscriptions: SubscriptionSuspensionView[];
}

export interface DeletableEntry {
    subscription_id: number;
    subscription_name: string;
    owner_id: number;
    owner: string;
    reseller?: string;
    causes: SuspensionCauses;
    reason?: string;
    suspended_at?: string;
    contents: {
        domains: string[];
        sites: number;
        mailboxes: number;
        databases: number;
        database_users: number;
        vpn_peers: number;
        backup_schedules: number;
    };
    delete_hint: string;
}

export const DELETABLE_URL = '/api/v1/suspensions/deletable';
export const accountSuspensionURL = (userID: number) => `/api/v1/users/${userID}/suspension`;
export const subscriptionSuspensionURL = (subscriptionID: number) => `/api/v1/subscriptions/${subscriptionID}/suspension`;

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);
const list = (value: unknown): string[] => (Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []);
const count = (value: unknown): number => (typeof value === 'number' && Number.isFinite(value) ? value : 0);

function decodeCauses(raw: unknown): SuspensionCauses {
    const causes = isRecord(raw) ? raw : {};
    return {
        manual: causes.manual === true,
        reseller_id: typeof causes.reseller_id === 'number' ? causes.reseller_id : undefined,
        reseller: typeof causes.reseller === 'string' ? causes.reseller : undefined,
        expired: causes.expired === true,
    };
}

function decodeSubscription(raw: unknown): SubscriptionSuspensionView {
    if (!isRecord(raw) || typeof raw.subscription_id !== 'number'
        || (raw.status !== 'active' && raw.status !== 'suspended' && raw.status !== 'expired')) {
        throw new Error('shape');
    }
    const preview = isRecord(raw.preview) ? raw.preview : {};
    const operation = isRecord(raw.operation) && typeof raw.operation.id === 'string' ? raw.operation : null;
    return {
        subscription_id: raw.subscription_id,
        subscription_name: typeof raw.subscription_name === 'string' ? raw.subscription_name : String(raw.subscription_id),
        owner_id: count(raw.owner_id),
        owner: typeof raw.owner === 'string' ? raw.owner : '',
        status: raw.status,
        causes: decodeCauses(raw.causes),
        reason: typeof raw.reason === 'string' ? raw.reason : undefined,
        suspended_at: typeof raw.suspended_at === 'string' ? raw.suspended_at : undefined,
        ends_at: typeof raw.ends_at === 'string' ? raw.ends_at : null,
        operation: operation as unknown as SuspensionOperation | null,
        preview: {
            action: preview.action === 'resume' ? 'resume' : 'suspend',
            sites: list(preview.sites),
            mail_domains: list(preview.mail_domains),
            mailboxes: count(preview.mailboxes),
            database_users: list(preview.database_users),
            vpn_peers: count(preview.vpn_peers),
            site_accounts: list(preview.site_accounts),
            notes: list(preview.notes),
        },
        can_suspend: raw.can_suspend === true,
        can_resume: raw.can_resume === true,
        resume_blocked: typeof raw.resume_blocked === 'string' && raw.resume_blocked ? raw.resume_blocked : undefined,
    };
}

// The effects of an operation are a list the server may write as null.
function decodeOperation(view: SubscriptionSuspensionView): SubscriptionSuspensionView {
    const operation = view.operation;
    if (!operation) return { ...view, operation: null };
    const guidance = isRecord(operation.guidance) ? operation.guidance : ({} as Partial<SuspensionGuidance>);
    return {
        ...view,
        operation: {
            ...operation,
            attempts: count(operation.attempts),
            guidance: {
                result: (guidance.result ?? 'unknown') as SuspensionResult,
                reason: typeof guidance.reason === 'string' ? guidance.reason : '',
                who_acts: (guidance.who_acts ?? 'none') as SuspensionWhoActs,
                next_action: typeof guidance.next_action === 'string' ? guidance.next_action : '',
                resumes: typeof guidance.resumes === 'string' ? guidance.resumes : '',
            },
            effects: Array.isArray(operation.effects) ? operation.effects.filter(isRecord) as unknown as SuspensionEffect[] : [],
        },
    };
}

export const decodeSubscriptionSuspension = (raw: unknown): SubscriptionSuspensionView => decodeOperation(decodeSubscription(raw));

export function decodeAccountSuspension(raw: unknown): AccountSuspensionView {
    if (!isRecord(raw) || typeof raw.user_id !== 'number' || !('subscriptions' in raw)) throw new Error('shape');
    const subscriptions = raw.subscriptions === null ? [] : raw.subscriptions;
    if (!Array.isArray(subscriptions)) throw new Error('shape');
    return {
        user_id: raw.user_id,
        username: typeof raw.username === 'string' ? raw.username : '',
        role: raw.role === 'reseller' ? 'reseller' : 'customer',
        reseller_suspended: raw.reseller_suspended === true,
        reason: typeof raw.reason === 'string' ? raw.reason : undefined,
        suspended_at: typeof raw.suspended_at === 'string' ? raw.suspended_at : undefined,
        subscriptions: subscriptions.map(decodeSubscriptionSuspension),
    };
}

// A subscription's own read is shown by the same dialogue as an account's: it
// is an account view with that one subscription in it.
export function decodeSubscriptionAsAccount(raw: unknown): AccountSuspensionView {
    const view = decodeSubscriptionSuspension(raw);
    return {
        user_id: view.owner_id,
        username: view.owner,
        role: 'customer',
        reseller_suspended: false,
        reason: view.reason,
        suspended_at: view.suspended_at,
        subscriptions: [view],
    };
}

export function decodeDeletable(raw: unknown): DeletableEntry[] {
    return decodeListIn<Record<string, unknown>>(raw, 'subscriptions').map((row) => {
        if (!isRecord(row) || typeof row.subscription_id !== 'number') throw new Error('shape');
        const contents = isRecord(row.contents) ? row.contents : {};
        return {
            subscription_id: row.subscription_id,
            subscription_name: typeof row.subscription_name === 'string' ? row.subscription_name : String(row.subscription_id),
            owner_id: count(row.owner_id),
            owner: typeof row.owner === 'string' ? row.owner : '',
            reseller: typeof row.reseller === 'string' && row.reseller ? row.reseller : undefined,
            causes: decodeCauses(row.causes),
            reason: typeof row.reason === 'string' && row.reason ? row.reason : undefined,
            suspended_at: typeof row.suspended_at === 'string' ? row.suspended_at : undefined,
            contents: {
                domains: list(contents.domains),
                sites: count(contents.sites),
                mailboxes: count(contents.mailboxes),
                databases: count(contents.databases),
                database_users: count(contents.database_users),
                vpn_peers: count(contents.vpn_peers),
                backup_schedules: count(contents.backup_schedules),
            },
            delete_hint: typeof row.delete_hint === 'string' ? row.delete_hint : '',
        };
    });
}

/** The operation is still being carried out on the server: poll, never send. */
export const operationRunning = (view: SubscriptionSuspensionView): boolean => view.operation?.state === 'running';

/** Which of the next actions this account's subscriptions allow. */
export function availableActions(view: AccountSuspensionView): { suspend: boolean; resume: boolean } {
    return {
        suspend: view.subscriptions.some((s) => s.can_suspend) || (view.role === 'reseller' && !view.reseller_suspended),
        resume: view.subscriptions.some((s) => s.can_resume) || view.reseller_suspended,
    };
}

/** Why a subscription is suspended, as the keys of its causes. */
export function causeKeys(causes: SuspensionCauses): Array<'manual' | 'reseller' | 'expired'> {
    return [
        ...(causes.manual ? (['manual'] as const) : []),
        ...(causes.reseller_id !== undefined || causes.reseller ? (['reseller'] as const) : []),
        ...(causes.expired ? (['expired'] as const) : []),
    ];
}

/**
 * The accounts that own a suspended subscription, from the deletable list (the
 * Accounts list marks them). `known` is false until that list has been read:
 * an unread list marks nobody and claims nothing.
 */
export function useSuspendedOwners(): { known: boolean; owners: ReadonlySet<number> } {
    const { remote } = useRemote(DELETABLE_URL, decodeDeletable);
    const read = lastKnown(remote);
    return { known: read !== undefined, owners: new Set(read ? read.value.map((entry) => entry.owner_id) : []) };
}
