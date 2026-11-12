import { useState } from 'react';
import { LifeBuoy } from 'lucide-react';
import { showToast } from './Toast';
import { useAuth } from '../auth/AuthContext';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';
import { invalidate, useRemote } from '../lib/remote';
import { mailboxRecoveryTime, useMailboxRecoveryWords, type MailboxRecoveryWordKey } from '../i18n/mailboxRecovery';
import { Button, Dialog, RemoteGate, inputClass } from './ui';
import { CopyCommand } from './CopyCommand';

// A mailbox's recovery address (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md).
// Whoever may change the mailbox's password sets or removes it here. The
// address works only after the link mailed to it was opened; the dialog says
// which state it is in, who acts next, and the page where a forgotten mailbox
// password is asked for.
//
// Posta kutusunun kurtarma adresi: girilen adres, ona gönderilen bağlantı
// açılınca çalışır. Pencere durumu, sıradaki adımı kimin atacağını ve
// unutulan parola sayfasını söyler.

export interface MailboxRecoveryView {
    mailbox: string;
    state: 'none' | 'pending' | 'confirmed' | 'void';
    void_reason?: string;
    address?: string;
    set_at?: string;
    confirmed_at?: string;
    mail: { available: boolean; reason?: string };
    request_page?: string;
}

export function decodeMailboxRecovery(raw: unknown): MailboxRecoveryView {
    const view = raw as MailboxRecoveryView | null;
    if (!view || typeof view !== 'object' || typeof view.mailbox !== 'string' ||
        !['none', 'pending', 'confirmed', 'void'].includes(view.state) ||
        !view.mail || typeof view.mail.available !== 'boolean') {
        throw new Error('recovery answer outside the contract');
    }
    return view;
}

export function mailboxRecoveryUrl(domainId: number, accountId: number): string {
    return `/api/v1/domains/${domainId}/mail/accounts/recovery?id=${accountId}`;
}

/** The state sentence for a recovery address as the server reads it. */
export function recoveryStateKey(view: MailboxRecoveryView): MailboxRecoveryWordKey {
    if (view.state === 'void') {
        return view.void_reason === 'local_domain_changed' ? 'mrec.state.void.local_domain_changed' : 'mrec.state.void.transferred';
    }
    return `mrec.state.${view.state}` as MailboxRecoveryWordKey;
}

export function MailboxRecoveryDialog({ domainId, accountId, address, onClose }: {
    domainId: number;
    accountId: number;
    address: string;
    onClose: () => void;
}) {
    const { t, locale } = useMailboxRecoveryWords();
    const { role } = useAuth();
    const url = mailboxRecoveryUrl(domainId, accountId);
    const read = useRemote(url, decodeMailboxRecovery);
    const [draft, setDraft] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);

    const save = async (current: MailboxRecoveryView) => {
        const typed = (draft ?? current.address ?? '').trim();
        if (busy || !typed || !current.mail.available) return;
        setBusy(true);
        setRefusal(null);
        try {
            const res = await fetch(`/api/v1/domains/${domainId}/mail/accounts/recovery`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: accountId, address: typed, lang: locale }),
            });
            if (!res.ok) {
                setRefusal(await readApiError(res));
                return;
            }
            const body = (await res.json()) as { unchanged?: boolean };
            showToast(body.unchanged ? 'info' : 'success', body.unchanged ? t('mrec.unchanged') : t('mrec.sent', { address: typed }));
            setDraft(null);
        } catch {
            setRefusal({ message: t('common.resultUnknown') });
        } finally {
            setBusy(false);
            invalidate('mail');
        }
    };

    const remove = async (current: MailboxRecoveryView) => {
        if (busy || !current.address) return;
        setRefusal(null);
        await confirmAction({
            title: t('mrec.removeConfirmTitle', { address: current.mailbox }),
            message: t('mrec.removeConfirmBody'),
            confirmLabel: t('mrec.remove'),
            tone: 'danger',
            progressTitle: t('mrec.removing'),
            action: async (): Promise<OperationEnd> => {
                setBusy(true);
                try {
                    const res = await fetch(url, { method: 'DELETE' });
                    if (!res.ok) {
                        const problem = await readApiError(res);
                        return { state: 'failed', reason: problem.code ? apiErrorText(problem, t) : `${t('mrec.removeFailed')} ${problem.message}` };
                    }
                    const body = (await res.json()) as { removed?: boolean };
                    setDraft(null);
                    return { state: 'done', message: body.removed ? t('mrec.removed') : t('mrec.nothingToRemove') };
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    setBusy(false);
                    invalidate('mail');
                }
            },
        });
    };

    return (
        <Dialog
            id="mailbox-recovery-dialog"
            icon={LifeBuoy}
            width="md"
            busy={busy}
            onDismiss={onClose}
            title={t('mrec.title')}
            description={<>{t('mrec.mailbox')}: <strong className="break-all font-medium text-fg">{address}</strong></>}
            actions={<Button type="button" onClick={onClose} disabled={busy}>{t('mrec.close')}</Button>}
        >
            <RemoteGate remote={read.remote} checking={t('mrec.checking')} failed={t('mrec.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => {
                    const noMail = !view.mail.available;
                    const value = draft ?? view.address ?? '';
                    const canSend = !noMail && !busy && !stale && value.trim() !== '';
                    const noMailText = t(role === 'admin' ? 'mrec.noMail.admin' : 'mrec.noMail.other');
                    const stateTime = view.state === 'confirmed' ? view.confirmed_at : view.set_at;
                    return (
                        <div className="space-y-4 text-sm">
                            {noMail && (
                                <p role="alert" id="mrec-no-mail" className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5">{noMailText}</p>
                            )}
                            <p className="text-fg-muted">{t('mrec.what')}</p>
                            <p role="status" data-recovery-state={view.state} className={view.state === 'void' ? 'rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5' : 'rounded-lg border border-border bg-surface-2/60 px-3 py-2.5'}>
                                {t(recoveryStateKey(view), { address: view.address ?? '', time: mailboxRecoveryTime(stateTime, locale) })}
                            </p>
                            {refusal && (
                                <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2.5 text-danger">
                                    {refusal.code ? apiErrorText(refusal, t) : `${t('mrec.failed')} ${refusal.message}`}
                                </p>
                            )}
                            <label className="block">
                                <span className="mb-1 block text-xs text-fg-muted">{t('mrec.address')}</span>
                                <input
                                    type="email"
                                    className={`${inputClass} field-lg`}
                                    value={value}
                                    maxLength={254}
                                    autoComplete="off"
                                    disabled={busy}
                                    onChange={(event) => setDraft(event.target.value)}
                                    aria-describedby={noMail ? 'mrec-no-mail mrec-address-hint' : 'mrec-address-hint'}
                                />
                                <span id="mrec-address-hint" className="mt-1 block text-xs text-fg-subtle">{t('mrec.addressHint')}</span>
                            </label>
                            <div className="flex flex-wrap items-center gap-2">
                                <Button type="button" variant="primary" disabled={!canSend} onClick={() => void save(view)}
                                    aria-describedby={noMail ? 'mrec-no-mail' : undefined}>
                                    {busy ? t('mrec.sending') : view.state === 'pending' && draft === null ? t('mrec.resend') : t('mrec.send')}
                                </Button>
                                {view.address && (
                                    <Button type="button" variant="danger" disabled={busy || stale} onClick={() => void remove(view)}>
                                        {t('mrec.remove')}
                                    </Button>
                                )}
                            </div>
                            {view.request_page && (
                                <div className="space-y-1">
                                    <p className="text-xs text-fg-muted">{t('mrec.requestPage')}</p>
                                    <CopyCommand command={view.request_page} label={t('mrec.copyPage')} />
                                </div>
                            )}
                        </div>
                    );
                }}
            </RemoteGate>
        </Dialog>
    );
}

export default MailboxRecoveryDialog;
