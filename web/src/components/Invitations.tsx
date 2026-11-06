import { useState } from 'react';
import { Check, Copy, Link2, MailPlus, Send, XCircle } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import type { AccountInvitationState } from '../lib/api';
import {
    INVITATIONS_URL,
    decodeInvitationOutcome,
    decodePendingInvitations,
    invitationLink,
    postJSON,
    shownDate,
    type InvitationOutcome,
    type PendingInvitation,
} from '../lib/accountAccess';
import { showToast } from './Toast';
import { Button, Dialog, inputClass } from './ui';
import { confirmAction } from '../lib/confirmAction';

// Invitations on the Users page (v0.1.0-alpha.114, P6). After an account is
// created with "send an invitation", or an invitation is sent again, the
// inviter sees what happened: handed to the mail system, or - when this server
// cannot send it - the one copy of the link with the reason, to pass on
// themselves. The link is shown only in that moment; the server keeps only its
// hash. Pending invitations can be emailed again, replaced by a new link to
// copy, or cancelled.
//
// Kullanıcılar sayfasındaki davetler. Davet edilen kişiye ne olduğu söylenir:
// posta sistemine verildi ya da sunucu gönderemiyorsa bağlantının tek kopyası
// nedeniyle birlikte gösterilir. Bekleyen davetler yeniden gönderilir, yeni
// kopyalanacak bağlantıyla değiştirilir ya da iptal edilir.

/** Shows what one send did; returns a link dialog when there is a link to pass on. */
export function useInvitationOutcome() {
    const { t } = useI18n();
    const [shown, setShown] = useState<InvitationOutcome | null>(null);
    const report = (raw: unknown) => {
        const outcome = decodeInvitationOutcome(raw);
        if (!outcome || outcome.delivery === 'none') {
            showToast('error', t('acct.invite.none'));
            return;
        }
        if (outcome.delivery === 'sent') {
            showToast('success', t('acct.invite.sent'));
            return;
        }
        setShown(outcome);
    };
    const dialog = shown ? <InvitationLinkDialog outcome={shown} onClose={() => setShown(null)} /> : null;
    return { report, dialog };
}

export function InvitationLinkDialog({ outcome, onClose }: { outcome: InvitationOutcome; onClose: () => void }) {
    const { t, locale } = useI18n();
    const [copied, setCopied] = useState(false);
    const link = invitationLink(outcome, window.location.origin);
    const reasonKey = ('acct.invite.reason.' + (outcome.reason ?? '')) as TranslationKey;
    const reason = t(reasonKey) === reasonKey ? '' : t(reasonKey);
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(link);
            setCopied(true);
        } catch {
            // The field stays selectable; the person copies it by hand.
            setCopied(false);
        }
    };
    return (
        <Dialog
            id="invitation-link"
            icon={Link2}
            width="md"
            title={t('acct.invite.copyTitle')}
            onDismiss={onClose}
            actions={
                <>
                    <Button type="button" icon={copied ? Check : Copy} onClick={() => void copy()}>
                        {copied ? t('acct.invite.copied') : t('acct.invite.copy')}
                    </Button>
                    <Button type="button" variant="primary" onClick={onClose}>{t('acct.invite.done')}</Button>
                </>
            }
        >
            <div className="space-y-3 text-sm" data-invitation-link={outcome.reason ?? ''}>
                {reason && <p className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-fg">{reason}</p>}
                <p className="text-fg-muted">{t('acct.invite.copyHelp', { date: shownDate(outcome.expires_at, locale) })}</p>
                <input
                    readOnly
                    value={link}
                    onFocus={(e) => e.currentTarget.select()}
                    className={`${inputClass} font-mono text-xs`}
                    aria-label={t('acct.invite.copyTitle')}
                />
            </div>
        </Dialog>
    );
}

/**
 * The status of an account that was invited and has not chosen a password
 * yet, with what to do next. `cancelledHint` is the page's own way forward
 * after a cancelled invitation.
 * Davet edilmiş ama henüz parola belirlememiş hesabın durumu ve sonraki adım.
 */
export function InvitedStatus({ invitation, cancelledHint }: { invitation: AccountInvitationState; cancelledHint: string }) {
    const { t, locale } = useI18n();
    const label = invitation.state === 'pending'
        ? t('acct.invite.status.pending')
        : invitation.state === 'expired' ? t('acct.invite.status.expired') : t('acct.invite.status.cancelled');
    const hint = invitation.state === 'pending'
        ? t('acct.invite.status.pendingHint', { date: shownDate(invitation.expires_at, locale) })
        : invitation.state === 'expired' ? t('acct.invite.status.expiredHint') : cancelledHint;
    return (
        <span className="inline-flex flex-col gap-0.5" data-invited-status={invitation.state}>
            <span className="inline-flex items-center gap-1.5 text-fg-muted">
                <MailPlus className="h-3.5 w-3.5 text-warning" aria-hidden="true" />
                {label}
            </span>
            <span className="max-w-xs text-xs text-fg-subtle">{hint}</span>
        </span>
    );
}

/** The list's one subscription; a page that creates an invitation reads it again with retry(). */
export const usePendingInvitations = () => useRemote(INVITATIONS_URL, decodePendingInvitations);

export type InvitationAction = 'email' | 'link' | 'revoke';

/**
 * Resend (by email or as a new link to copy) or cancel one open invitation,
 * by its id. Each click sends one request; afterwards every list that shows
 * accounts or invitations reads again (users, team members, pending
 * invitations), so nothing needs a page refresh. An answer that does not
 * arrive is reported as unknown and nothing is sent again.
 * Davet yeniden gönderilir ya da iptal edilir; ardından hesapları ve davetleri
 * gösteren her liste yeniden okunur.
 */
export function useInvitationActions() {
    const { t, locale } = useI18n();
    const { report, dialog } = useInvitationOutcome();
    const [busy, setBusy] = useState<number | null>(null);
    const act = async (id: number, username: string, action: InvitationAction) => {
        if (busy !== null) return;
        if (action === 'revoke' && !(await confirmAction({ message: t('acct.invite.revokeConfirm', { name: username }), tone: 'danger' }))) return;
        setBusy(id);
        try {
            const res = action === 'revoke'
                ? await fetch(`${INVITATIONS_URL}/${id}`, { method: 'DELETE' })
                : await postJSON(`${INVITATIONS_URL}/${id}/resend`, { delivery: action, lang: locale });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
            } else if (action === 'revoke') {
                showToast('success', t('acct.invite.revoked'));
            } else {
                report(((await res.json()) as { invitation?: unknown }).invitation);
            }
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setBusy(null);
            invalidate('users', 'team');
        }
    };
    return { act, busy, dialog };
}

/** The open invitations the signed-in account manages; nothing when there are none. */
export function PendingInvitations() {
    const { t, locale } = useI18n();
    const list = usePendingInvitations();
    const { act: send, busy, dialog } = useInvitationActions();

    if (list.remote.state === 'unknown') {
        return <p className="mt-4 text-sm text-fg-muted">{t('acct.invite.readFailed')}</p>;
    }
    const items = list.remote.state === 'known' ? list.remote.value : [];
    if (items.length === 0) return dialog;

    const act = (item: PendingInvitation, action: InvitationAction) => send(item.id, item.username, action);

    return (
        <section className="mt-6 rounded-xl border border-border bg-surface p-4" data-pending-invitations="">
            <h3 className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg">
                <MailPlus className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                {t('acct.invite.pendingTitle')}
            </h3>
            <p className="mb-3 text-xs text-fg-muted">{t('acct.invite.pendingHelp')}</p>
            <ul className="divide-y divide-border">
                {items.map((item) => (
                    <li key={item.id} className="flex flex-col gap-2 py-2.5 sm:flex-row sm:items-center sm:justify-between" data-invitation-state={item.state}>
                        <div className="min-w-0">
                            <p className="truncate text-sm font-medium text-fg">{item.username}</p>
                            <p className="truncate text-xs text-fg-muted">{item.email}</p>
                            <p className="text-xs text-fg-subtle">
                                {item.state === 'pending'
                                    ? t('acct.invite.state.pending', { date: shownDate(item.expires_at, locale) })
                                    : t(`acct.invite.state.${item.state}` as TranslationKey)}
                            </p>
                        </div>
                        <div className="flex flex-wrap gap-2">
                            {item.state !== 'superseded' && (
                                <>
                                    <Button className="text-xs" icon={Send} disabled={busy !== null} onClick={() => void act(item, 'email')}>
                                        {t('acct.invite.resend')}
                                    </Button>
                                    <Button className="text-xs" icon={Link2} disabled={busy !== null} onClick={() => void act(item, 'link')}>
                                        {t('acct.invite.copyNew')}
                                    </Button>
                                </>
                            )}
                            <Button className="text-xs" icon={XCircle} disabled={busy !== null} onClick={() => void act(item, 'revoke')}>
                                {t('acct.invite.revoke')}
                            </Button>
                        </div>
                    </li>
                ))}
            </ul>
            {dialog}
        </section>
    );
}
