import { useEffect, useState } from 'react';
import { MailCheck, Send } from 'lucide-react';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { OWN_EMAIL_VERIFICATION_URL, decodeOwnEmailVerification, postJSON } from '../lib/accountAccess';
import { Button, RemoteGate } from './ui';

// Whether the signed-in person's own address is confirmed, and a way to
// confirm it (v0.1.0-alpha.114, P6). A password reset is sent only to a
// confirmed address; any change of the address makes it unconfirmed again.
// `address` is the address as last read; a new one reads the state again.
//
// Oturum açan kişinin adresi onaylı mı; değilse onaylama yolu. Parola
// sıfırlama yalnız onaylı adrese gider; adres değişince onay düşer.
export function EmailVerificationCard({ address }: { address: string | null }) {
    const { t, locale } = useI18n();
    const state = useRemote(OWN_EMAIL_VERIFICATION_URL, decodeOwnEmailVerification);
    const [sending, setSending] = useState(false);
    const [note, setNote] = useState<{ tone: 'ok' | 'problem' | 'unknown'; text: string } | null>(null);

    useEffect(() => {
        if (address !== null) void state.retry();
        setNote(null);
        // Only a new address reads the state again.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [address]);

    const send = async () => {
        setSending(true);
        setNote(null);
        try {
            const res = await postJSON(OWN_EMAIL_VERIFICATION_URL, { lang: locale });
            if (res.ok) {
                setNote({ tone: 'ok', text: t('acct.verify.sent') });
            } else {
                setNote({ tone: 'problem', text: apiErrorText(await readApiError(res), t) });
            }
        } catch {
            setNote({ tone: 'unknown', text: t('acct.verify.unknown') });
        } finally {
            setSending(false);
        }
    };

    const tones = {
        ok: 'border-success/40 bg-success/10',
        problem: 'border-danger/30 bg-danger/10 text-danger',
        unknown: 'border-warning-mark/60 bg-warning-mark/20',
    };

    return (
        <div className="mb-4 rounded-lg border border-border bg-surface-2/50 p-3" data-email-verification="">
            <p className="mb-1 flex items-center gap-2 text-sm font-medium text-fg">
                <MailCheck className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                {t('acct.verify.title')}
            </p>
            <RemoteGate
                remote={state.remote}
                checking={t('acct.link.checking')}
                failed={t('acct.verify.readFailed')}
                onRetry={() => void state.retry()}
                busy={state.reading}
            >
                {({ value }) => (
                    <div className="space-y-2 text-sm">
                        {!value.email_set ? (
                            <p className="text-fg-muted">{t('acct.verify.none')}</p>
                        ) : value.email_verified ? (
                            <p className="text-fg-muted" data-email-verified="">{t('acct.verify.verified')}</p>
                        ) : (
                            <>
                                <p className="text-fg-muted" data-email-unverified="">{t('acct.verify.unverified')}</p>
                                {!value.mail.available && (
                                    <p className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2 text-fg">
                                        {t('acct.verify.noMail')}
                                    </p>
                                )}
                                <Button icon={Send} loading={sending} disabled={sending || !value.mail.available} onClick={() => void send()}>
                                    {sending ? t('acct.verify.sending') : t('acct.verify.send')}
                                </Button>
                            </>
                        )}
                        {note && <p role="status" className={`rounded-lg border px-3 py-2 ${tones[note.tone]}`}>{note.text}</p>}
                    </div>
                )}
            </RemoteGate>
        </div>
    );
}
