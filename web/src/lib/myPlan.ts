import type { SubscriptionUsage } from './subscriptions';

// GET /api/v1/my/subscription (alpha.114): the signed-in account's plan, its
// limits and the live usage the quota checks use, whom to ask for a larger
// plan, and a reseller's pool. Every limit of 0 is unlimited. An answer
// without the subscription list or the contact is not "no plan"; it is
// unknown.
//
// Oturumdaki hesabın planı, sınırları ve kota denetimlerinin kullandığı canlı
// kullanım; daha büyük plan için kime başvurulacağı; bayinin havuzu. Her 0
// sınırı sınırsızdır.
export const MY_SUBSCRIPTION_URL = '/api/v1/my/subscription';

export interface MyPlanSubscription {
    id: number;
    name: string;
    status: string;
    plan: { id: number; name: string } | null;
    usage: SubscriptionUsage;
    over_limit: string[];
}

export interface MyPlanContact {
    kind: 'reseller' | 'administrator' | 'none';
    name?: string;
    email?: string;
}

export interface PoolLimits {
    max_customers: number;
    max_domains: number;
    max_databases: number;
    max_email_accounts: number;
    disk_quota_mb: number;
}

export interface PoolAllocation {
    customers: number;
    domains: number;
    databases: number;
    mailboxes: number;
    disk_mb: number;
    unlimited_domains?: boolean;
    unlimited_databases?: boolean;
    unlimited_mailboxes?: boolean;
    unlimited_disk?: boolean;
}

export interface MyPlan {
    subscriptions: MyPlanSubscription[];
    contact: MyPlanContact;
    bandwidthMeasured: boolean;
    pool?: { limits: PoolLimits | null; allocated: PoolAllocation; over_limit: string[] };
}

const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);

export function decodeMyPlan(raw: unknown): MyPlan {
    if (!isRecord(raw) || !Array.isArray(raw.subscriptions) || !isRecord(raw.contact)) throw new Error('shape');
    const kind = raw.contact.kind;
    if (kind !== 'reseller' && kind !== 'administrator' && kind !== 'none') throw new Error('shape');
    const subscriptions = raw.subscriptions.map((entry) => {
        if (!isRecord(entry) || typeof entry.id !== 'number' || !isRecord(entry.usage)) throw new Error('shape');
        return {
            ...(entry as unknown as MyPlanSubscription),
            over_limit: Array.isArray(entry.over_limit) ? (entry.over_limit as string[]) : [],
        };
    });
    const bandwidth = isRecord(raw.bandwidth) ? raw.bandwidth.measured === true : false;
    const pool = isRecord(raw.pool) && isRecord(raw.pool.allocated)
        ? {
            limits: isRecord(raw.pool.limits) ? (raw.pool.limits as unknown as PoolLimits) : null,
            allocated: raw.pool.allocated as unknown as PoolAllocation,
            over_limit: Array.isArray(raw.pool.over_limit) ? (raw.pool.over_limit as string[]) : [],
        }
        : undefined;
    return { subscriptions, contact: raw.contact as unknown as MyPlanContact, bandwidthMeasured: bandwidth, pool };
}

/** Usage against a limit as a percentage; null when the limit is unlimited (0). */
export function usagePercent(used: number, limit: number): number | null {
    if (limit <= 0) return null;
    return Math.min(100, (used / limit) * 100);
}
