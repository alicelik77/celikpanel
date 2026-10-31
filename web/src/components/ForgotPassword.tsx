import { useState, type FormEvent } from 'react';
import { useRemote } from '../lib/remote';
import { MailCheck, ArrowLeft } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import {
    RECOVERY_MAIL_STATUS_URL,
    RECOVERY_REQUEST_URL,
    decodeAccountMailStatus,
    postJSON,
    type AccountMailStatus,
} from '../lib/accountAccess';
import { Spinner } from './ui';

// "Forgot password?" on the sign-in card (v0.1.0-alpha.114, P6). Before the
// person types anything it asks whether this server can email a reset link
// at all; when it cannot, it says why and who can help (D-024: reason, who
// acts, next step) instead of taking a request that can never arrive. The
// answer to a request is the same whatever was typed: the page never says
// whether an account exists.
//
// Giriş kartındaki "Parolamı unuttum". Önce sunucunun e-posta gönderip
// gönderemediğine bakar; gönderemiyorsa nedenini ve kimin yardım edeceğini
// söyler. İsteğin yanıtı yazılan ne olursa olsun aynıdır.

type MailState = { kind: 'checking' } | { kind: 'known'; status: AccountMailStatus } | { kind: 'unknown' };

const box = 'rounded-lg border px-3 py-2.5 text-sm';

export function ForgotPassword({ onBack }: { onBack: () => void }) {
    const { t, locale, screensReady } = useI18n();
    const status = useRemote(RECOVERY_MAIL_STATUS_URL, decodeAccountMailStatus);
    // A refusal of the request itself names the same server-wide fact.
    const [refused, setRefused] = useState<AccountMailStatus | null>(null);
    const mail: MailState = refused
        ? { kind: 'known', status: refused }
        : status.remote.state === 'known'
          ? { kind: 'known', status: status.remote.value }
          : status.remote.state === 'unknown'
            ? { kind: 'unknown' }
            : { kind: 'checking' };
    const [identifier, setIdentifier] = useState('');
    const [sending, setSending] = useState(false);
    const [done, setDone] = useState(false);
    const [problem, setProblem] = useState('');

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        if (!identifier.trim() || sending) return;
        setSending(true);
        setProblem('');
        try {
            const res = await postJSON(RECOVERY_REQUEST_URL, { identifier: identifier.trim(), lang: locale });
            if (res.status === 202) {
                setDone(true);
                return;
            }
            const error = await readApiError(res);
            if (error.code === 'account_mail_unavailable') {
                setRefused({ available: false, reason: error.reason });
                return;
            }
            setProblem(apiErrorText(error, t));
        } catch {
            setProblem(t('common.resultUnknown'));
        } finally {
            setSending(false);
        }
    };

    const back = (
        <button
            type="button"
            onClick={onBack}
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
        >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            {t('acct.forgot.back')}
        </button>
    );

    if (!screensReady || mail.kind === 'checking') {
        return (
            <div className="space-y-4 rounded-2xl border border-border bg-surface p-6">
                <p className="flex items-center gap-2 text-sm text-fg-muted">
                    <Spinner size="xs" tone="current" label={screensReady ? t('acct.forgot.checking') : undefined} />
                    <span aria-hidden="true">{screensReady ? t('acct.forgot.checking') : ''}</span>
                </p>
                {screensReady && back}
            </div>
        );
    }

    const unavailable = mail.kind === 'known' && !mail.status.available;
    const reasonKey = ('acct.mail.' + (mail.kind === 'known' ? mail.status.reason ?? 'no_mail_server' : '')) as TranslationKey;
    const reasonText = t(reasonKey) === reasonKey ? t('acct.mail.no_mail_server') : t(reasonKey);

    return (
        <div className="space-y-4 rounded-2xl border border-border bg-surface p-6" data-forgot-password="">
            <h2 className="text-lg font-semibold text-fg">{t('acct.forgot.title')}</h2>
            {unavailable ? (
                <div className={`${box} border-warning-mark/60 bg-warning-mark/20 text-fg`} role="status" data-forgot-unavailable={mail.status.reason ?? ''}>
                    <p className="mb-1 font-medium">{t('acct.mail.unavailableTitle')}</p>
                    <p>{reasonText}</p>
                </div>
            ) : done ? (
                <div className={`${box} border-success/40 bg-success/10 text-fg`} role="status" data-forgot-done="">
                    <p className="mb-1 flex items-center gap-2 font-medium">
                        <MailCheck className="h-4 w-4" aria-hidden="true" />
                        {t('acct.forgot.doneTitle')}
                    </p>
                    <p>{t('acct.forgot.done')}</p>
                </div>
            ) : (
                <form onSubmit={(event) => void submit(event)} className="space-y-4" noValidate>
                    <p className="text-sm text-fg-muted">{t('acct.forgot.help')}</p>
                    {mail.kind === 'unknown' && <p className={`${box} border-border bg-surface-2 text-fg-muted`}>{t('acct.mail.unknown')}</p>}
                    <label className="block">
                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('acct.forgot.identifier')}</span>
                        <input
                            type="text"
                            autoFocus
                            autoComplete="username"
                            spellCheck={false}
                            maxLength={254}
                            value={identifier}
                            onChange={(e) => setIdentifier(e.target.value)}
                            disabled={sending}
                            className="w-full rounded-lg border border-border bg-surface-2 px-3 py-2 text-fg outline-none transition-shadow focus:border-primary"
                        />
                    </label>
                    {problem && (
                        <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
                            {problem}
                        </p>
                    )}
                    <button
                        type="submit"
                        disabled={sending || !identifier.trim()}
                        className="w-full rounded-lg bg-primary px-3 py-2.5 font-medium text-primary-fg transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
                    >
                        {sending ? t('acct.forgot.sending') : t('acct.forgot.submit')}
                    </button>
                </form>
            )}
            <p className="text-xs text-fg-subtle">{t('acct.forgot.owner')}</p>
            {back}
        </div>
    );
}
