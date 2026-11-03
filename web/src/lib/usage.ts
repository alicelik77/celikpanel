// Plan limits made real (alpha.118, D-044): website traffic per calendar
// month, the subscription's real disk use, and the notices at 80% and 100%.
// One decoder per address; an answer that is not the contract is unknown,
// never "no notices" or "nothing used".
//
// Plan sınırları gerçek olur: aylık web trafiği, aboneliğin gerçek disk
// kullanımı ve %80/%100 bildirimleri. Sözleşme dışı yanıt bilinmeyendir.

export const USAGE_NOTICES_URL = '/api/v1/usage-notices';
export const USAGE_STATUS_URL = '/api/v1/usage/status';

export type UsageNoticeKind =
    | 'traffic_80' | 'traffic_100' | 'pool_traffic_80' | 'pool_traffic_100'
    | 'disk_80' | 'disk_100' | 'disk_countdown' | 'disk_suspended';

export interface UsageNotice {
    id: number;
    kind: UsageNoticeKind;
    audience: 'customer' | 'reseller' | 'administrator';
    subscription_id?: number;
    created_at: string;
    body: {
        subscription?: string;
        owner?: string;
        reseller?: string;
        used_bytes: number;
        limit_bytes: number;
        period: string;
        cause?: 'subscription' | 'pool';
        suspend_on?: string;
    };
}

const KINDS: readonly string[] = ['traffic_80', 'traffic_100', 'pool_traffic_80', 'pool_traffic_100', 'disk_80', 'disk_100', 'disk_countdown', 'disk_suspended'];
const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const str = (v: unknown): string | undefined => (typeof v === 'string' && v !== '' ? v : undefined);

export function decodeUsageNotices(raw: unknown): UsageNotice[] {
    if (!isRecord(raw) || !Array.isArray(raw.notices)) throw new Error('shape');
    return raw.notices.flatMap((entry): UsageNotice[] => {
        if (!isRecord(entry) || typeof entry.id !== 'number' || typeof entry.kind !== 'string' || !KINDS.includes(entry.kind) || !isRecord(entry.body)) return [];
        const audience = entry.audience === 'reseller' || entry.audience === 'administrator' ? entry.audience : 'customer';
        const b = entry.body;
        return [{
            id: entry.id,
            kind: entry.kind as UsageNoticeKind,
            audience,
            subscription_id: typeof entry.subscription_id === 'number' && entry.subscription_id > 0 ? entry.subscription_id : undefined,
            created_at: typeof entry.created_at === 'string' ? entry.created_at : '',
            body: {
                subscription: str(b.subscription), owner: str(b.owner), reseller: str(b.reseller),
                used_bytes: num(b.used_bytes), limit_bytes: num(b.limit_bytes), period: typeof b.period === 'string' ? b.period : '',
                cause: b.cause === 'pool' ? 'pool' : b.cause === 'subscription' ? 'subscription' : undefined,
                suspend_on: str(b.suspend_on),
            },
        }];
    });
}

/** The measurement state of the administrator's status answer. */
export interface UsageStatus {
    traffic: { last_collected?: string; error?: string; logrotate?: string; unknown: Record<string, number>; measuring: number; interval_minutes: number };
    disk: { measured: number; unknown: number; not_measured: number; low_space: boolean; interval_hours: number };
    mail_quota: { state: string; dialect?: string; detail?: string; error?: string };
}

export function decodeUsageStatus(raw: unknown): UsageStatus {
    if (!isRecord(raw) || !isRecord(raw.traffic) || !isRecord(raw.disk) || !isRecord(raw.mail_quota) || typeof raw.mail_quota.state !== 'string') throw new Error('shape');
    const traffic = raw.traffic;
    const unknown: Record<string, number> = {};
    if (isRecord(traffic.unknown)) {
        for (const [reason, n] of Object.entries(traffic.unknown)) unknown[reason] = num(n);
    }
    return {
        traffic: {
            last_collected: str(traffic.last_collected), error: str(traffic.error), logrotate: str(traffic.logrotate),
            unknown, measuring: num(traffic.measuring), interval_minutes: num(traffic.interval_minutes) || 5,
        },
        disk: {
            measured: num(raw.disk.measured), unknown: num(raw.disk.unknown), not_measured: num(raw.disk.not_measured),
            low_space: raw.disk.low_space === true, interval_hours: num(raw.disk.interval_hours) || 6,
        },
        mail_quota: { state: raw.mail_quota.state, dialect: str(raw.mail_quota.dialect), detail: str(raw.mail_quota.detail), error: str(raw.mail_quota.error) },
    };
}

/** Bytes for a sentence: MB below one GB, GB with one decimal above. */
export function formatBytes(bytes: number): string {
    const mb = bytes / (1024 * 1024);
    if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
    return `${Math.round(mb)} MB`;
}

/** What needs the administrator's attention in the status, as reason codes. */
export function usageStatusProblems(status: UsageStatus): string[] {
    const problems: string[] = [];
    if (status.traffic.error) problems.push('traffic_collect');
    if (Object.values(status.traffic.unknown).some((n) => n > 0)) problems.push('traffic_unknown');
    if (status.traffic.logrotate === 'missing_tool' || status.traffic.logrotate === 'kept' || status.traffic.logrotate === 'failed') problems.push('logrotate');
    if (status.disk.unknown > 0) problems.push('disk_unknown');
    if (status.disk.low_space) problems.push('disk_low');
    if (!['enforced', 'no_mail', 'not_checked_yet'].includes(status.mail_quota.state) || status.mail_quota.error) problems.push('mail_quota');
    return problems;
}
