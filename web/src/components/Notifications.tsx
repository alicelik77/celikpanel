import { useEffect, useState } from 'react';
import { AlertTriangle, Bell, Check, ExternalLink, Info, Mail, Save } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { notificationsTranslation, type NotificationsKey } from '../i18n/notifications';
import { showToast } from './Toast';
import { Button, CouldNotCheck, RemoteGate, inputClass } from './ui';
import { refusalText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { sendRights } from '../lib/rights';
import { Link, useLocation, useNavigate } from '../router';
import { useAuth } from '../auth/AuthContext';
import { usageNoticeText } from './UsageNotices';
import {
    NOTIFICATIONS_URL, NOTIFICATION_PREFERENCES_URL, NOTIFICATION_SETTINGS_URL, NOTIFY_GROUPS,
    decodeNotificationPreferences, decodeNotificationSettings, decodeNotifications, groupNotifications,
    notificationSeenURL, usageNoticeSeenURL,
    type NotificationItem, type NotificationMode, type NotificationSettings, type NotifyGroup,
} from '../lib/notifications';

// Notifications by recipient (alpha.128, G18): the signed-in person's own
// notices in one list — "Needs your action" first, then "For your
// information" — each with the reason in plain words and the button that
// opens where the fix is; the person's email choice; and, for the
// administrator, the server-wide settings (how long before a subscription's
// end date to warn, one extra address per kind of event). Every list
// refreshes itself after a change; there is no browser dialog.
//
// Alıcıya göre bildirimler: kişinin kendi bildirimleri tek listede, önce
// "Sizin yapmanız gerekenler", sonra "Bilginize"; her birinde düz dilde
// neden ve düzeltmenin yapıldığı sayfayı açan düğme. E-posta seçimi ve
// yöneticinin sunucu geneli ayarları.

type Key = TranslationKey | NotificationsKey;

/** The I18nProvider's words with this screen's own copy in front (route-scoped, no boot cost). */
function useNotificationWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: Key, values?: Record<string, string | number>) => notificationsTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values) };
}

function itemText(item: NotificationItem, locale: string | undefined, say: (key: TranslationKey, vars?: Record<string, string | number>) => string): { title: string; body: string } {
    if (item.source === 'account') {
        const tr = locale === 'tr';
        return { title: tr ? item.words.title_tr : item.words.title_en, body: tr ? item.words.text_tr : item.words.text_en };
    }
    return usageNoticeText(item.usage, say);
}

/**
 * The person's own notices. On the Dashboard it shows nothing while there is
 * nothing; a read that failed says so (it is not "no notices"). showAlerts
 * adds the administrator's open monitoring alerts as one line (the
 * Dashboard already has its own alerts line).
 */
export function NotificationsCenter({ showAlerts = false }: { showAlerts?: boolean }) {
    const { t, locale } = useNotificationWords();
    const say = useI18n().t;
    const read = useRemote(NOTIFICATIONS_URL, decodeNotifications);
    const navigate = useNavigate();
    const here = useLocation().pathname;
    const [busy, setBusy] = useState(false);
    if (read.remote.state === 'loading') return null;
    if (read.remote.state === 'unknown' && !read.remote.previous) {
        return <CouldNotCheck text={t('notif.readFailed')} onRetry={() => void read.retry()} busy={read.reading} className="mb-6" />;
    }
    const report = read.remote.state === 'known' ? read.remote.value : read.remote.previous!.value;
    const alerts = showAlerts && report.monitoring ? report.monitoring.open : 0;
    if (report.items.length === 0 && alerts === 0) return null;
    const groups = groupNotifications(report.items);

    const seen = async (items: NotificationItem[]) => {
        if (busy || items.length === 0) return;
        setBusy(true);
        try {
            for (const item of items) {
                const result = await sendRights(item.source === 'account' ? notificationSeenURL(item.id) : usageNoticeSeenURL(item.id), 'POST');
                if (result.kind === 'lost') {
                    showToast('error', t('common.resultUnknown'));
                    break;
                }
                if (result.kind === 'refused') {
                    showToast('error', refusalText(result.error, say, 'rights.notice.ackFailed'));
                    break;
                }
            }
        } finally {
            await read.retry();
            invalidate('rights');
            setBusy(false);
        }
    };

    const row = (item: NotificationItem) => {
        const text = itemText(item, locale, say);
        const fix = item.fix_path && item.fix_path.split('?')[0] !== here ? item.fix_path : '';
        return (
            <li
                key={`${item.source}-${item.id}`}
                role="status"
                data-notification={item.kind}
                data-notification-source={item.source}
                data-needs-action={item.needs_action ? 'yes' : 'no'}
                className={`rounded-lg border p-3 text-sm leading-6 text-fg ${item.needs_action ? 'border-warning-mark/50 bg-warning-mark/15' : 'border-border bg-surface'}`}
            >
                <p className="flex items-start gap-2 font-semibold">
                    {item.needs_action
                        ? <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                        : <Info className="mt-1 h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />}
                    <span>{text.title}</span>
                </p>
                <p className="mt-1 max-w-[75ch]">{text.body}</p>
                <div className="mt-2 flex flex-wrap gap-2">
                    {fix && (
                        <Button type="button" variant={item.needs_action ? 'primary' : 'secondary'} icon={ExternalLink} className="max-sm:min-h-11" onClick={() => navigate(fix)} data-notification-fix={fix}>
                            {t(item.needs_action ? 'notif.open' : 'notif.openInfo')}
                        </Button>
                    )}
                    <Button type="button" icon={Check} className="max-sm:min-h-11" onClick={() => void seen([item])} disabled={busy}>{t('notif.seen')}</Button>
                </div>
            </li>
        );
    };

    return (
        <section className="mb-6 rounded-xl border border-border bg-surface p-5" data-notifications={report.items.length} aria-label={t('notif.title')}>
            <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-fg">
                <Bell className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                {t('notif.title')}
            </h2>
            {(groups.action.length > 0 || alerts > 0) && (
                <div className="mb-4" data-notifications-group="action">
                    <h3 className="mb-2 text-sm font-semibold text-fg">{t('notif.needsAction')}</h3>
                    <ul className="grid gap-2">
                        {alerts > 0 && (
                            <li role="status" data-notification="monitoring_alerts" className="flex flex-wrap items-center gap-2 rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-3 text-sm text-fg">
                                <AlertTriangle className="h-4 w-4 text-warning" aria-hidden="true" />
                                <span className="font-semibold">{alerts === 1 ? t('notif.alertsOne') : t('notif.alerts', { n: alerts })}</span>
                                <Link to="/monitoring" className="ml-auto font-semibold text-primary max-sm:min-h-11">{t('notif.alertsOpen')}</Link>
                            </li>
                        )}
                        {groups.action.map(row)}
                    </ul>
                </div>
            )}
            {groups.info.length > 0 && (
                <div data-notifications-group="info">
                    <h3 className="mb-2 text-sm font-semibold text-fg">{t('notif.forInfo')}</h3>
                    <ul className="grid gap-2">{groups.info.map(row)}</ul>
                </div>
            )}
            <div className="mt-3 flex flex-wrap items-center gap-3">
                {report.items.length > 1 && (
                    <Button type="button" onClick={() => void seen(report.items)} disabled={busy}>{t('notif.seenAll')}</Button>
                )}
                {here !== '/account' && <Link to="/account#notifications" className="text-xs text-fg-muted underline underline-offset-2">{t('notif.emailedNote')}</Link>}
            </div>
        </section>
    );
}

const MODES: NotificationMode[] = ['all', 'action', 'none'];
const MODE_WORDS: Record<NotificationMode, [NotificationsKey, NotificationsKey]> = {
    all: ['notif.prefs.all', 'notif.prefs.allHint'],
    action: ['notif.prefs.action', 'notif.prefs.actionHint'],
    none: ['notif.prefs.none', 'notif.prefs.noneHint'],
};

/** My account: which notices are also emailed, and one extra address. */
export function NotificationPreferencesCard() {
    const { t } = useNotificationWords();
    const { user } = useAuth();
    const read = useRemote(NOTIFICATION_PREFERENCES_URL, decodeNotificationPreferences);
    const [mode, setMode] = useState<NotificationMode | null>(null);
    const [extra, setExtra] = useState<string | null>(null);
    const [saving, setSaving] = useState(false);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    useEffect(() => {
        if (known) {
            setMode(known.mode);
            setExtra(known.extra_email);
        }
    }, [known]);
    const impersonating = user.impersonating === true;
    const save = async () => {
        if (!known || saving) return;
        setSaving(true);
        try {
            const result = await sendRights(NOTIFICATION_PREFERENCES_URL, 'PUT', { mode: mode ?? known.mode, extra_email: (extra ?? known.extra_email).trim() });
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            else if (result.kind === 'refused') showToast('error', refusalText(result.error, t, 'notif.prefs.failed' as TranslationKey));
            else showToast('success', t('notif.prefs.saved'));
        } finally {
            await read.retry();
            setSaving(false);
        }
    };
    return (
        <section id="notifications" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="notifications">
            <div className="mb-1 flex items-center gap-2">
                <Mail className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('notif.prefs.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('notif.prefs.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('notif.prefs.checking')} failed={t('notif.prefs.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value }) => (
                    <div className="grid gap-3">
                        {!value.mail_available && <p className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-3 text-sm text-fg" data-notification-mail="unavailable">{t('notif.prefs.noMail')}</p>}
                        {value.mail_available && !value.own_email_set && <p className="text-sm text-fg-muted" data-notification-own-email="missing">{t('notif.prefs.noOwnEmail')}</p>}
                        {impersonating && <p className="text-sm text-fg-muted" data-notification-impersonating>{t('notif.prefs.impersonating')}</p>}
                        <fieldset className="grid gap-2" disabled={impersonating || saving}>
                            {MODES.map((choice) => (
                                <label key={choice} className={`flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 max-sm:min-h-11 ${(mode ?? value.mode) === choice ? 'border-primary bg-primary/5' : 'border-border'}`} data-notification-mode={choice}>
                                    <input type="radio" name="notification-mode" className="mt-1" checked={(mode ?? value.mode) === choice} onChange={() => setMode(choice)} />
                                    <span>
                                        <span className="block text-sm font-semibold text-fg">{t(MODE_WORDS[choice][0])}</span>
                                        <span className="block text-xs text-fg-muted">{t(MODE_WORDS[choice][1])}</span>
                                    </span>
                                </label>
                            ))}
                            <label className="mt-1 grid gap-1 text-sm">
                                <span className="font-semibold text-fg">{t('notif.prefs.extra')}</span>
                                <input type="email" className={inputClass} value={extra ?? value.extra_email} onChange={(event) => setExtra(event.target.value)} maxLength={254} autoComplete="email" data-notification-extra />
                                <span className="text-xs text-fg-muted">{t('notif.prefs.extraHint')}</span>
                            </label>
                        </fieldset>
                        {!impersonating && (
                            <div>
                                <Button type="button" variant="primary" icon={Save} loading={saving} className="max-sm:min-h-11" onClick={() => void save()}>{t('notif.prefs.save')}</Button>
                            </div>
                        )}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}

/** The administrator's server-wide settings: expiry warning days and one extra address per kind of event. */
export function NotificationSettingsCard() {
    const { t } = useNotificationWords();
    const read = useRemote(NOTIFICATION_SETTINGS_URL, decodeNotificationSettings);
    const [draft, setDraft] = useState<NotificationSettings | null>(null);
    const [saving, setSaving] = useState(false);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    useEffect(() => {
        if (known) setDraft(known);
    }, [known]);
    const save = async () => {
        if (!draft || saving) return;
        setSaving(true);
        try {
            const extra = Object.fromEntries(NOTIFY_GROUPS.map((group) => [group, draft.extra[group].trim()]));
            const result = await sendRights(NOTIFICATION_SETTINGS_URL, 'PUT', { expiry_days_before: draft.expiry_days_before, extra });
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            else if (result.kind === 'refused') showToast('error', refusalText(result.error, t, 'notif.admin.failed' as TranslationKey));
            else showToast('success', t('notif.admin.saved'));
        } finally {
            await read.retry();
            setSaving(false);
        }
    };
    const setExtra = (group: NotifyGroup, value: string) => setDraft((current) => (current ? { ...current, extra: { ...current.extra, [group]: value } } : current));
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-account-section="notification-settings">
            <div className="mb-1 flex items-center gap-2">
                <Bell className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('notif.admin.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('notif.admin.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('notif.admin.checking')} failed={t('notif.admin.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value }) => {
                    const shown = draft ?? value;
                    return (
                        <fieldset className="grid gap-4" disabled={saving}>
                            <label className="grid gap-1 text-sm">
                                <span className="font-semibold text-fg">{t('notif.admin.expiry')}</span>
                                <select className={inputClass} value={shown.expiry_days_before} onChange={(event) => setDraft({ ...shown, expiry_days_before: Number(event.target.value) })} data-notification-expiry-days>
                                    {shown.expiry_choices.map((n) => <option key={n} value={n}>{t('notif.admin.days', { n })}</option>)}
                                </select>
                                <span className="text-xs text-fg-muted">{t('notif.admin.expiryHint')}</span>
                            </label>
                            <div className="grid gap-2">
                                <p className="text-sm font-semibold text-fg">{t('notif.admin.extraTitle')}</p>
                                <p className="text-xs text-fg-muted">{t('notif.admin.extraHint')}</p>
                                {NOTIFY_GROUPS.map((group) => (
                                    <label key={group} className="grid gap-1 text-sm">
                                        <span className="text-fg">{t(`notif.admin.group.${group}` as NotificationsKey)}</span>
                                        <input type="email" className={inputClass} value={shown.extra[group]} onChange={(event) => setExtra(group, event.target.value)} maxLength={254} data-notification-group={group} />
                                    </label>
                                ))}
                            </div>
                            <div>
                                <Button type="button" variant="primary" icon={Save} loading={saving} className="max-sm:min-h-11" onClick={() => void save()}>{t('notif.admin.save')}</Button>
                            </div>
                        </fieldset>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
