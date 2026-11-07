// Notifications by recipient (alpha.128, G18): the signed-in person's own
// notices in one list, their email choice, and the administrator's
// server-wide settings. One decoder per address; an answer that is not the
// contract is unknown, never "no notices".
//
// Alıcıya göre bildirimler: kişinin kendi bildirimleri tek listede, e-posta
// seçimi ve yöneticinin sunucu geneli ayarları. Sözleşme dışı yanıt bilinmeyendir.

import { decodeUsageNotices, type UsageNotice } from './usage';

export const NOTIFICATIONS_URL = '/api/v1/notifications';
export const NOTIFICATION_PREFERENCES_URL = '/api/v1/notification-preferences';
export const NOTIFICATION_SETTINGS_URL = '/api/v1/panel/notification-settings';
export const notificationSeenURL = (id: number) => `${NOTIFICATIONS_URL}/${id}/seen`;
export const usageNoticeSeenURL = (id: number) => `/api/v1/usage-notices/${id}/seen`;
// alpha.128 integration: two stores the list reads but never copies; "seen"
// goes back to the store that owns the notice.
export const siteLimitNoticeSeenURL = (id: number) => `/api/v1/rights/notices/${id}/acknowledge`;
export const mailOutgoingNoticeSeenURL = (id: number) => `/api/v1/mail-outgoing-notices/${id}/seen`;

export interface AccountNoticeWords { title_en: string; text_en: string; title_tr: string; text_tr: string }

interface ItemBase {
    id: number;
    kind: string;
    audience: 'customer' | 'reseller' | 'administrator';
    needs_action: boolean;
    /** A local panel path ("" = none). */
    fix_path: string;
    created_at: string;
}
/** A lowered limit that brought a site's own PHP value down (limits). */
export interface ClampedSite { domain: string; key: string; value: number; limit: number; set_by: string; set_by_name: string }

export type NotificationItem =
    | (ItemBase & { source: 'account'; words: AccountNoticeWords })
    | (ItemBase & { source: 'mail_outgoing'; words: AccountNoticeWords })
    | (ItemBase & { source: 'usage'; usage: UsageNotice })
    | (ItemBase & { source: 'site_limit'; clamp: ClampedSite });

/** Where "seen" goes for one item: the store that owns it. */
export function notificationSeenPath(item: NotificationItem): string {
    switch (item.source) {
        case 'account': return notificationSeenURL(item.id);
        case 'usage': return usageNoticeSeenURL(item.id);
        case 'site_limit': return siteLimitNoticeSeenURL(item.id);
        case 'mail_outgoing': return mailOutgoingNoticeSeenURL(item.id);
    }
}

export interface NotificationsReport {
    items: NotificationItem[];
    /** The administrator's open monitoring alerts; absent for everyone else. */
    monitoring?: { open: number; unseen: number };
}

const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const text = (v: unknown): string => (typeof v === 'string' ? v : '');
const count = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.floor(v) : 0);

/** Only a local panel path is ever followed. */
export function localFixPath(raw: unknown): string {
    const path = text(raw);
    return path.startsWith('/') && !path.startsWith('//') && !path.startsWith('/\\') && path.length <= 300 ? path : '';
}

export function decodeNotifications(raw: unknown): NotificationsReport {
    if (!isRecord(raw) || !Array.isArray(raw.items)) throw new Error('shape');
    const items = raw.items.flatMap((entry): NotificationItem[] => {
        if (!isRecord(entry) || typeof entry.id !== 'number' || typeof entry.kind !== 'string') return [];
        const audience = entry.audience === 'reseller' || entry.audience === 'administrator' ? entry.audience : 'customer';
        const base: ItemBase = {
            id: entry.id, kind: entry.kind, audience, needs_action: entry.needs_action === true,
            fix_path: localFixPath(entry.fix_path), created_at: text(entry.created_at),
        };
        if ((entry.source === 'account' || entry.source === 'mail_outgoing') && isRecord(entry.words)) {
            const w = entry.words;
            const words = { title_en: text(w.title_en), text_en: text(w.text_en), title_tr: text(w.title_tr), text_tr: text(w.text_tr) };
            if (!words.title_en || !words.title_tr) return [];
            return [{ ...base, source: entry.source, words }];
        }
        if (entry.source === 'site_limit' && isRecord(entry.clamp)) {
            const c = entry.clamp;
            if (typeof c.value !== 'number' || typeof c.limit !== 'number' || !text(c.domain) || !text(c.key)) return [];
            return [{ ...base, source: 'site_limit', clamp: { domain: text(c.domain), key: text(c.key), value: c.value, limit: c.limit, set_by: text(c.set_by), set_by_name: text(c.set_by_name) } }];
        }
        if (entry.source === 'usage' && isRecord(entry.usage)) {
            // The usage notice's own decoder: the same contract as /usage-notices.
            const [usage] = decodeUsageNotices({ notices: [{ id: entry.id, kind: entry.kind, audience, subscription_id: entry.subscription_id, created_at: entry.created_at, body: entry.usage }] });
            return usage ? [{ ...base, source: 'usage', usage }] : [];
        }
        return [];
    });
    const report: NotificationsReport = { items };
    if (isRecord(raw.monitoring)) report.monitoring = { open: count(raw.monitoring.open), unseen: count(raw.monitoring.unseen) };
    return report;
}

/** "Needs your action" first, then "For your information"; newest first in each. */
export function groupNotifications(items: NotificationItem[]): { action: NotificationItem[]; info: NotificationItem[] } {
    const newest = (a: NotificationItem, b: NotificationItem) => (a.created_at < b.created_at ? 1 : a.created_at > b.created_at ? -1 : b.id - a.id);
    return {
        action: items.filter((item) => item.needs_action).sort(newest),
        info: items.filter((item) => !item.needs_action).sort(newest),
    };
}

export type NotificationMode = 'all' | 'action' | 'none';
export interface NotificationPreferences { mode: NotificationMode; extra_email: string; own_email_set: boolean; mail_available: boolean }

export function decodeNotificationPreferences(raw: unknown): NotificationPreferences {
    if (!isRecord(raw) || (raw.mode !== 'all' && raw.mode !== 'action' && raw.mode !== 'none')) throw new Error('shape');
    return { mode: raw.mode, extra_email: text(raw.extra_email), own_email_set: raw.own_email_set === true, mail_available: raw.mail_available === true };
}

export const NOTIFY_GROUPS = ['monitoring', 'account', 'security'] as const;
export type NotifyGroup = typeof NOTIFY_GROUPS[number];
export interface NotificationSettings { expiry_days_before: number; expiry_choices: number[]; extra: Record<NotifyGroup, string> }

export function decodeNotificationSettings(raw: unknown): NotificationSettings {
    if (!isRecord(raw) || typeof raw.expiry_days_before !== 'number' || !Array.isArray(raw.expiry_choices) || !isRecord(raw.extra)) throw new Error('shape');
    const extra = raw.extra;
    return {
        expiry_days_before: raw.expiry_days_before,
        expiry_choices: raw.expiry_choices.filter((n): n is number => typeof n === 'number' && Number.isFinite(n)),
        extra: { monitoring: text(extra.monitoring), account: text(extra.account), security: text(extra.security) },
    };
}
