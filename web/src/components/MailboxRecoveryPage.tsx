import { useEffect, useRef, useState, type FormEvent } from 'react';
import { CheckCircle2, KeyRound, LifeBuoy, MailCheck } from 'lucide-react';
import { useLocation, useNavigate } from '../router';
import { apiErrorText, readApiError } from '../lib/apiError';
import { accountLinkToken, postJSON } from '../lib/accountAccess';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { useMailboxRecoveryWords } from '../i18n/mailboxRecovery';
import { BrandMark } from './BrandMark';
import { LanguageSwitcher } from './LanguageSwitcher';
import { ThemeSwitcher } from './ThemeSwitcher';
import { Spinner, inputClass } from './ui';
import { PasswordInput } from './PasswordInput';

// The pages a mailbox's recovery links open (alpha.130;
// docs/MAILBOX-RECOVERY-REVIEW.md). No session: the link's token is the
// credential, read from the fragment and removed from the address bar at once.
//
//   /account-link/mailbox-recovery  "forgot the mailbox password": always the
//                                   same answer, whatever the address
//   /account-link/mailbox-confirm   confirms the recovery address (one request,
//                                   sent once even when React runs effects twice)
//   /account-link/mailbox-reset     a new mailbox password (nothing changes until
//                                   Save)
//
// Posta kutusu kurtarma bağlantılarının sayfaları: oturum gerekmez; jeton
// adres çubuğundan hemen silinir.

export const MAILBOX_RECOVERY_REQUEST_URL = '/api/v1/auth/mailbox-recovery/request';
export const MAILBOX_RECOVERY_CONFIRM_URL = '/api/v1/auth/mailbox-recovery/confirm';
export const MAILBOX_RECOVERY_LINK_URL = '/api/v1/auth/mailbox-recovery/link';
export const MAILBOX_RECOVERY_RESET_URL = '/api/v1/auth/mailbox-recovery/reset';

export type MailboxRecoveryPageKind = 'request' | 'confirm' | 'reset';

/** Which mailbox recovery page a path is, or null for any other path. */
export function mailboxRecoveryPage(pathname: string): MailboxRecoveryPageKind | null {
    switch (pathname.replace(/\/+$/, '')) {
        case '/account-link/mailbox-recovery':
            return 'request';
        case '/account-link/mailbox-confirm':
            return 'confirm';
        case '/account-link/mailbox-reset':
            return 'reset';
        default:
            return null;
    }
}

type Phase =
    | { kind: 'form'; mailbox: string }
    | { kind: 'checking' }
    | { kind: 'saving'; mailbox: string }
    | { kind: 'done'; mailbox: string; notes: string[] }
    | { kind: 'refused'; text: string }
    | { kind: 'unknown' }
    | { kind: 'missing' };

export function MailboxRecoveryPage() {
    const { t, locale, screensReady } = useMailboxRecoveryWords();
    const location = useLocation();
    const navigate = useNavigate();
    const page = mailboxRecoveryPage(location.pathname) ?? 'request';
    const [token] = useState(() => accountLinkToken(window.location.hash));
    const [phase, setPhase] = useState<Phase>(() =>
        page === 'request' ? { kind: 'form', mailbox: '' } : token ? { kind: 'checking' } : { kind: 'missing' });
    const [mailbox, setMailbox] = useState('');
    const [password, setPassword] = useState('');
    const [repeat, setRepeat] = useState('');
    const [formProblem, setFormProblem] = useState('');
    const sent = useRef(false);

    useEffect(() => {
        if (window.location.hash) {
            window.history.replaceState(window.history.state, '', window.location.pathname);
        }
    }, []);

    useEffect(() => {
        if (page === 'request' || !token || sent.current) return;
        sent.current = true;
        const refuse = async (res: Response) => setPhase({ kind: 'refused', text: apiErrorText(await readApiError(res), t) });
        const url = page === 'confirm' ? MAILBOX_RECOVERY_CONFIRM_URL : MAILBOX_RECOVERY_LINK_URL;
        postJSON(url, { token })
            .then(async (res) => {
                if (!res.ok) return refuse(res);
                const body = (await res.json()) as { mailbox?: unknown };
                const name = typeof body.mailbox === 'string' ? body.mailbox : '';
                setPhase(page === 'confirm' ? { kind: 'done', mailbox: name, notes: [] } : { kind: 'form', mailbox: name });
            })
            .catch(() => setPhase(page === 'confirm' ? { kind: 'unknown' } : { kind: 'refused', text: t('common.resultUnknown') }));
        // The link is read once per page load.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const askForLink = async (event: FormEvent) => {
        event.preventDefault();
        const typed = mailbox.trim();
        if (phase.kind !== 'form' || !typed) return;
        setFormProblem('');
        setPhase({ kind: 'saving', mailbox: typed });
        try {
            const res = await postJSON(MAILBOX_RECOVERY_REQUEST_URL, { address: typed, lang: locale });
            if (res.ok) {
                setPhase({ kind: 'done', mailbox: typed, notes: [] });
                return;
            }
            setFormProblem(apiErrorText(await readApiError(res), t));
            setPhase({ kind: 'form', mailbox: typed });
        } catch {
            setFormProblem(t('common.resultUnknown'));
            setPhase({ kind: 'form', mailbox: typed });
        }
    };

    const save = async (event: FormEvent) => {
        event.preventDefault();
        if (phase.kind !== 'form' || !token || !passwordAcceptable(password, 'mailbox') || password !== repeat) return;
        const name = phase.mailbox;
        setFormProblem('');
        setPhase({ kind: 'saving', mailbox: name });
        try {
            const res = await postJSON(MAILBOX_RECOVERY_RESET_URL, { token, new_password: password });
            if (res.ok) {
                const body = (await res.json()) as Record<string, unknown>;
                const notes: string[] = [];
                if (body.sessions_not_ended === true) notes.push(t('mrec.page.sessionsNotEnded'));
                if (body.sending_paused === true) notes.push(t('mrec.page.sendingPaused'));
                if (body.sending_resumed === true) notes.push(t('mrec.page.sendingResumed'));
                if (body.sending_resume_pending === true) notes.push(t('mrec.page.sendingResumePending'));
                setPassword('');
                setRepeat('');
                setPhase({ kind: 'done', mailbox: name, notes });
                return;
            }
            const problem = await readApiError(res);
            // A password the rule refuses, or a busy server, leaves the link usable.
            if (problem.code === 'PASSWORD_WEAK' || problem.code === 'mailbox_recovery_busy') {
                setFormProblem(apiErrorText(problem, t));
                setPhase({ kind: 'form', mailbox: name });
                return;
            }
            setPhase({ kind: 'refused', text: apiErrorText(problem, t) });
        } catch {
            setPhase({ kind: 'unknown' });
        }
    };

    const title = page === 'confirm' ? t('mrec.page.confirmTitle') : page === 'reset' ? t('mrec.page.resetTitle') : t('mrec.page.requestTitle');
    const Icon = page === 'confirm' ? MailCheck : page === 'reset' ? KeyRound : LifeBuoy;
    const doneText = phase.kind !== 'done' ? '' : page === 'confirm' ? t('mrec.page.confirmDone', { mailbox: phase.mailbox })
        : page === 'reset' ? t('mrec.page.resetDone') : t('mrec.page.requestDone');
    const newLink = page !== 'request' && (
        <button
            type="button"
            onClick={() => navigate('/account-link/mailbox-recovery', { replace: true })}
            className="w-full rounded-lg border border-border px-3 py-2.5 font-medium transition-colors hover:bg-surface-2"
        >
            {t('mrec.page.newLink')}
        </button>
    );
    const mismatch = repeat !== '' && password !== repeat;
    const busy = phase.kind === 'saving';

    return (
        <div className="relative flex min-h-screen items-center justify-center bg-bg px-4 text-fg">
            <div className="absolute right-4 top-4 flex items-center gap-2">
                <LanguageSwitcher />
                <ThemeSwitcher />
            </div>
            <div className="w-full max-w-sm">
                <div className="mb-8 flex flex-col items-center text-center">
                    <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary text-primary-fg shadow-lg">
                        <BrandMark className="h-7 w-7" />
                    </div>
                    <h1 className="text-2xl font-bold tracking-tight">{t('app.name')}</h1>
                </div>
                <div className="space-y-4 rounded-2xl border border-border bg-surface p-6" data-mailbox-recovery={page} data-phase={phase.kind}>
                    {!screensReady ? (
                        <Spinner size="sm" />
                    ) : (
                        <>
                            <h2 className="flex items-center gap-2 text-lg font-semibold">
                                <Icon className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                {title}
                            </h2>
                            {phase.kind === 'checking' && (
                                <p role="status" className="flex items-center gap-2 text-sm text-fg-muted">
                                    <Spinner size="xs" tone="current" />
                                    {page === 'confirm' ? t('mrec.page.confirming') : t('acct.link.checking')}
                                </p>
                            )}
                            {phase.kind === 'missing' && (
                                <>
                                    <p className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm">{t('mrec.page.missing')}</p>
                                    {newLink}
                                </>
                            )}
                            {page === 'request' && (phase.kind === 'form' || phase.kind === 'saving') && (
                                <form onSubmit={(event) => void askForLink(event)} className="space-y-4" noValidate>
                                    <p className="text-sm text-fg-muted">{t('mrec.page.requestBody')}</p>
                                    <label className="block">
                                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('mrec.page.mailbox')}</span>
                                        <input
                                            type="email"
                                            autoComplete="email"
                                            autoFocus
                                            maxLength={254}
                                            value={mailbox}
                                            disabled={busy}
                                            onChange={(event) => setMailbox(event.target.value)}
                                            className={inputClass}
                                        />
                                    </label>
                                    {formProblem && <p role="alert" className="text-sm text-danger">{formProblem}</p>}
                                    <button
                                        type="submit"
                                        disabled={busy || mailbox.trim() === ''}
                                        className="w-full rounded-lg bg-primary px-3 py-2.5 font-medium text-primary-fg transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
                                    >
                                        {busy ? t('mrec.page.requestSending') : t('mrec.page.requestSend')}
                                    </button>
                                </form>
                            )}
                            {page === 'reset' && (phase.kind === 'form' || phase.kind === 'saving') && (
                                <form onSubmit={(event) => void save(event)} className="space-y-4" noValidate>
                                    <p className="text-sm text-fg-muted">{t('mrec.page.resetFor', { mailbox: phase.mailbox })}</p>
                                    {/* A hidden user name lets a password manager store the new password for the right mailbox. */}
                                    <input type="text" name="username" autoComplete="username" value={phase.mailbox} readOnly hidden />
                                    <label className="block">
                                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('mrec.page.password')}</span>
                                        <PasswordInput field="mailbox" autoComplete="new-password" autoFocus value={password} maxLength={1024}
                                            disabled={busy} onChange={setPassword} onGenerated={setRepeat} />
                                    </label>
                                    <label className="block">
                                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('mrec.page.repeat')}</span>
                                        <PasswordInput autoComplete="new-password" value={repeat} maxLength={1024}
                                            disabled={busy} onChange={setRepeat} />
                                    </label>
                                    {mismatch && <p role="alert" className="text-sm text-danger">{t('mrec.page.mismatch')}</p>}
                                    {formProblem && <p role="alert" className="text-sm text-danger">{formProblem}</p>}
                                    <p className="text-xs text-fg-subtle">{t('mrec.page.resetHint')}</p>
                                    <button
                                        type="submit"
                                        disabled={busy || !passwordAcceptable(password, 'mailbox') || password !== repeat}
                                        className="w-full rounded-lg bg-primary px-3 py-2.5 font-medium text-primary-fg transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
                                    >
                                        {busy ? t('mrec.page.saving') : t('mrec.page.save')}
                                    </button>
                                </form>
                            )}
                            {phase.kind === 'done' && (
                                <>
                                    <p role="status" className="flex items-start gap-2 rounded-lg border border-success/40 bg-success/10 px-3 py-2.5 text-sm">
                                        <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                                        <span>{doneText}</span>
                                    </p>
                                    {phase.notes.map((note) => (
                                        <p key={note} className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm">{note}</p>
                                    ))}
                                </>
                            )}
                            {phase.kind === 'refused' && (
                                <>
                                    <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2.5 text-sm text-danger">{phase.text}</p>
                                    {newLink}
                                </>
                            )}
                            {phase.kind === 'unknown' && (
                                <>
                                    <p role="status" className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm">{t('mrec.page.unknown')}</p>
                                    {newLink}
                                </>
                            )}
                        </>
                    )}
                </div>
            </div>
        </div>
    );
}

export default MailboxRecoveryPage;
