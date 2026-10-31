import { useState } from 'react';
import { Check, Copy, Link2, MailPlus, Send, XCircle } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
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

/** The list's one subscription; a page that creates an invitation reads it again with retry(). */
export const usePendingInvitations = () => useRemote(INVITATIONS_URL, decodePendingInvitations);

/** The open invitations the signed-in account manages; nothing when there are none. */
export function PendingInvitations() {
    const { t, locale } = useI18n();
    const list = usePendingInvitations();
    const { report, dialog } = useInvitationOutcome();
    const [busy, setBusy] = useState<number | null>(null);

    if (list.remote.state === 'unknown') {
        return <p className="mt-4 text-sm text-fg-muted">{t('acct.invite.readFailed')}</p>;
    }
    const items = list.remote.state === 'known' ? list.remote.value : [];
    if (items.length === 0) return dialog;

    const act = async (item: PendingInvitation, delivery: 'email' | 'link' | 'revoke') => {
        if (delivery === 'revoke' && !window.confirm(t('acct.invite.revokeConfirm', { name: item.username }))) return;
        setBusy(item.id);
        try {
            const res = delivery === 'revoke'
                ? await fetch(`${INVITATIONS_URL}/${item.id}`, { method: 'DELETE' })
                : await postJSON(`${INVITATIONS_URL}/${item.id}/resend`, { delivery, lang: locale });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
            } else if (delivery === 'revoke') {
                showToast('success', t('acct.invite.revoked'));
            } else {
                report(((await res.json()) as { invitation?: unknown }).invitation);
            }
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setBusy(null);
            void list.retry();
        }
    };

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
