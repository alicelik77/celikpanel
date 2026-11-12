import { useMemo, useState } from 'react';
import { AlertTriangle, Bell, Check, ExternalLink, Info, Mail } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { notificationsTranslation, type NotificationsKey } from '../i18n/notifications';
import { showToast } from './Toast';
import { Button, CouldNotCheck, RemoteGate, inputClass } from './ui';
import { refusalText } from '../lib/apiError';
import { invalidate, lastKnown, useRemote } from '../lib/remote';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';
import { sendRights } from '../lib/rights';
import { Link, useLocation, useNavigate } from '../router';
import { useAuth } from '../auth/AuthContext';
import { usageNoticeText } from './UsageNotices';
import { unitText, whoText } from '../lib/settingLimits';
import {
    NOTIFICATIONS_URL, NOTIFICATION_PREFERENCES_URL, NOTIFICATION_SETTINGS_URL, NOTIFY_GROUPS,
    decodeNotificationPreferences, decodeNotificationSettings, decodeNotifications, groupNotifications,
    notificationSeenPath,
    type NotificationItem, type NotificationMode, type NotifyGroup,
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

// The units of the limit keys a lowered limit can bring down (PHP values).
const LIMIT_UNITS: Record<string, string> = {
    'php.memory_limit_mb': 'mb', 'php.max_execution_time_s': 's', 'php.upload_max_mb': 'mb', 'php.max_input_vars': 'count', 'php.max_children': 'count',
};

function itemText(item: NotificationItem, locale: string | undefined, say: (key: TranslationKey, vars?: Record<string, string | number>) => string): { title: string; body: string } {
    if (item.source === 'account' || item.source === 'mail_outgoing') {
        const tr = locale === 'tr';
        return { title: tr ? item.words.title_tr : item.words.title_en, body: tr ? item.words.text_tr : item.words.text_en };
    }
    if (item.source === 'site_limit') {
        // The limits words (screen part "limits"): what changed, that the site
        // keeps running, who acts.
        const c = item.clamp;
        const unit = LIMIT_UNITS[c.key] ?? 'count';
        const who = whoText(say, c.set_by, c.set_by_name);
        const line = say('limits.notice.line', { domain: c.domain, key: say(`limits.key.${c.key}` as TranslationKey), value: unitText(say, unit, c.value), limit: unitText(say, unit, c.limit), who });
        const todo = say(item.audience === 'customer' ? 'limits.notice.todoCustomer' : 'limits.notice.todoProvider', { who });
        return { title: say('limits.notice.title'), body: `${line} ${todo}` };
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
                const result = await sendRights(notificationSeenPath(item), 'POST');
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

// An extra address is empty or a whole address; the server checks it again.
const emailOK = (value: string) => value.trim() === '' || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());

/** My account: which notices are also emailed, and one extra address. */
export function NotificationPreferencesCard() {
    const { t } = useNotificationWords();
    const say = useSaveWords();
    const { user } = useAuth();
    const read = useRemote(NOTIFICATION_PREFERENCES_URL, decodeNotificationPreferences);
    const answer = lastKnown(read.remote)?.value;
    // alpha.132: a draft over the answer; only Save writes (owner, 8 Nov 2026).
    const known = useMemo(() => (answer ? { mode: answer.mode, extra: answer.extra_email } : null), [answer]);
    const draft = useDraft(known);
    const impersonating = user.impersonating === true;
    const save = () => void draft.save(async (_changes, values) => {
        const result = await sendRights(NOTIFICATION_PREFERENCES_URL, 'PUT', { mode: values.mode, extra_email: values.extra.trim() });
        if (result.kind === 'lost') return 'unknown';
        if (result.kind === 'refused') {
            showToast('error', refusalText(result.error, t, 'notif.prefs.failed' as TranslationKey));
            return 'refused';
        }
        showToast('success', t('notif.prefs.saved'));
        await read.retry();
        return 'saved';
    });
    return (
        <section id="notifications" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="notifications">
            <div className="mb-1 flex items-center gap-2">
                <Mail className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('notif.prefs.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('notif.prefs.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('notif.prefs.checking')} failed={t('notif.prefs.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value, stale }) => {
                    const mode = draft.values?.mode ?? value.mode;
                    const extra = draft.values?.extra ?? value.extra_email;
                    const extraBad = draft.isChanged('extra') && !emailOK(extra);
                    return (
                        <div className="grid gap-3">
                            {!value.mail_available && <p className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-3 text-sm text-fg" data-notification-mail="unavailable">{t('notif.prefs.noMail')}</p>}
                            {value.mail_available && !value.own_email_set && <p className="text-sm text-fg-muted" data-notification-own-email="missing">{t('notif.prefs.noOwnEmail')}</p>}
                            {impersonating && <p className="text-sm text-fg-muted" data-notification-impersonating>{t('notif.prefs.impersonating')}</p>}
                            <fieldset className="grid gap-2" disabled={impersonating || draft.saving}>
                                {MODES.map((choice) => {
                                    const marked = draft.isChanged('mode') && mode === choice;
                                    return (
                                        <label key={choice} className={`flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 max-sm:min-h-11 ${mode === choice ? 'border-primary bg-primary/5' : 'border-border'} ${marked ? changedRowClass : ''}`} data-notification-mode={choice}>
                                            <input type="radio" name="notification-mode" className="mt-1" checked={mode === choice} onChange={() => draft.set('mode', choice)} />
                                            <span>
                                                <span className="block text-sm font-semibold text-fg">{t(MODE_WORDS[choice][0])}{marked && <ChangedMark />}</span>
                                                <span className="block text-xs text-fg-muted">{t(MODE_WORDS[choice][1])}</span>
                                            </span>
                                        </label>
                                    );
                                })}
                                {draft.isMoved('mode') && draft.base && <ServerMovedNote value={t(MODE_WORDS[draft.base.mode][0])} onTake={() => draft.take('mode')} />}
                                <label className={`mt-1 grid gap-1 text-sm ${draft.isChanged('extra') ? `-mx-2 rounded-r px-2 ${changedRowClass}` : ''}`}>
                                    <span className="font-semibold text-fg">{t('notif.prefs.extra')}{draft.isChanged('extra') && <ChangedMark />}</span>
                                    <input type="email" className={inputClass} value={extra} onChange={(event) => draft.set('extra', event.target.value)} maxLength={254} autoComplete="email" aria-invalid={extraBad || undefined} data-notification-extra />
                                    <span className={`text-xs ${extraBad ? 'text-danger' : 'text-fg-muted'}`}>{extraBad ? t('notif.badEmail') : t('notif.prefs.extraHint')}</span>
                                </label>
                                {draft.isMoved('extra') && draft.base && <ServerMovedNote value={draft.base.extra || '—'} onTake={() => draft.take('extra')} />}
                            </fieldset>
                            {!impersonating && (
                                <SaveBar
                                    className="-mx-5 px-5 pb-0"
                                    count={draft.count}
                                    saving={draft.saving}
                                    blockedBy={stale ? say('stale') : extraBad ? t('notif.badEmail') : undefined}
                                    unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                                    onSave={save}
                                    onDiscard={draft.reset}
                                />
                            )}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

// The administrator's form: the expiry warning days and one address per kind of event.
type AdminForm = { expiry: number } & Record<NotifyGroup, string>;

/** The administrator's server-wide settings: expiry warning days and one extra address per kind of event. */
export function NotificationSettingsCard() {
    const { t } = useNotificationWords();
    const say = useSaveWords();
    const read = useRemote(NOTIFICATION_SETTINGS_URL, decodeNotificationSettings);
    const answer = lastKnown(read.remote)?.value;
    // alpha.132: a draft over the answer; only Save writes (owner, 8 Nov 2026).
    const known = useMemo<AdminForm | null>(() => (answer ? { expiry: answer.expiry_days_before, ...answer.extra } : null), [answer]);
    const draft = useDraft(known);
    const save = () => void draft.save(async (_changes, values) => {
        const extra = Object.fromEntries(NOTIFY_GROUPS.map((group) => [group, values[group].trim()]));
        const result = await sendRights(NOTIFICATION_SETTINGS_URL, 'PUT', { expiry_days_before: values.expiry, extra });
        if (result.kind === 'lost') return 'unknown';
        if (result.kind === 'refused') {
            showToast('error', refusalText(result.error, t, 'notif.admin.failed' as TranslationKey));
            return 'refused';
        }
        showToast('success', t('notif.admin.saved'));
        await read.retry();
        return 'saved';
    });
    const changedClass = (field: keyof AdminForm) => (draft.isChanged(field) ? `-mx-2 rounded-r px-2 ${changedRowClass}` : '');
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-account-section="notification-settings">
            <div className="mb-1 flex items-center gap-2">
                <Bell className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('notif.admin.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('notif.admin.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('notif.admin.checking')} failed={t('notif.admin.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value, stale }) => {
                    const form: AdminForm = draft.values ?? { expiry: value.expiry_days_before, ...value.extra };
                    const bad = NOTIFY_GROUPS.filter((group) => draft.isChanged(group) && !emailOK(form[group]));
                    return (
                        <div>
                            <fieldset className="grid gap-4" disabled={draft.saving}>
                                <div className={changedClass('expiry')}>
                                    <label className="grid gap-1 text-sm">
                                        <span className="font-semibold text-fg">{t('notif.admin.expiry')}{draft.isChanged('expiry') && <ChangedMark />}</span>
                                        <select className={inputClass} value={form.expiry} onChange={(event) => draft.set('expiry', Number(event.target.value))} data-notification-expiry-days>
                                            {value.expiry_choices.map((n) => <option key={n} value={n}>{t('notif.admin.days', { n })}</option>)}
                                        </select>
                                        <span className="text-xs text-fg-muted">{t('notif.admin.expiryHint')}</span>
                                    </label>
                                    {draft.isMoved('expiry') && draft.base && <ServerMovedNote value={t('notif.admin.days', { n: draft.base.expiry })} onTake={() => draft.take('expiry')} />}
                                </div>
                                <div className="grid gap-2">
                                    <p className="text-sm font-semibold text-fg">{t('notif.admin.extraTitle')}</p>
                                    <p className="text-xs text-fg-muted">{t('notif.admin.extraHint')}</p>
                                    {NOTIFY_GROUPS.map((group) => (
                                        <div key={group} className={changedClass(group)}>
                                            <label className="grid gap-1 text-sm">
                                                <span className="text-fg">{t(`notif.admin.group.${group}` as NotificationsKey)}{draft.isChanged(group) && <ChangedMark />}</span>
                                                <input type="email" className={inputClass} value={form[group]} onChange={(event) => draft.set(group, event.target.value)} maxLength={254}
                                                    aria-invalid={bad.includes(group) || undefined} data-notification-group={group} />
                                                {bad.includes(group) && <span className="text-xs text-danger">{t('notif.badEmail')}</span>}
                                            </label>
                                            {draft.isMoved(group) && draft.base && <ServerMovedNote value={draft.base[group] || '—'} onTake={() => draft.take(group)} />}
                                        </div>
                                    ))}
                                </div>
                            </fieldset>
                            <SaveBar
                                className="-mx-5 px-5 pb-0"
                                count={draft.count}
                                saving={draft.saving}
                                blockedBy={stale ? say('stale') : bad.length > 0 ? t('notif.badEmail') : undefined}
                                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                                onSave={save}
                                onDiscard={draft.reset}
                            />
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
