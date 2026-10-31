import { useEffect, useRef, useState, type FormEvent } from 'react';
import { CheckCircle2, KeyRound, MailCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import { useLocation, useNavigate } from '../router';
import { apiErrorText, readApiError } from '../lib/apiError';
import {
    RECOVERY_ACCEPT_URL,
    RECOVERY_LINK_URL,
    RECOVERY_RESET_URL,
    RECOVERY_VERIFY_URL,
    accountLinkPurpose,
    accountLinkToken,
    postJSON,
} from '../lib/accountAccess';
import { BrandMark } from './BrandMark';
import { LanguageSwitcher } from './LanguageSwitcher';
import { ThemeSwitcher } from './ThemeSwitcher';
import { Spinner } from './ui';

// The page a reset, invitation or confirmation link opens (v0.1.0-alpha.114,
// P6). It needs no session. The token is read from the fragment and removed
// from the address bar at once; nothing is changed until the person presses
// Save (a reset or invitation) or, for a confirmation, until the page itself
// sends the one request, which it does once even when React runs effects
// twice. A link scanner that only fetches the address changes nothing.
//
// Sıfırlama, davet ya da onay bağlantısının açtığı sayfa. Oturum gerekmez.
// Jeton parça kısmından okunur ve adres çubuğundan hemen silinir; kişi Kaydet'e
// basana (ya da onayda sayfa tek isteğini gönderene) dek hiçbir şey değişmez.

type Phase =
    | { kind: 'checking' }
    | { kind: 'form'; username: string }
    | { kind: 'saving'; username: string }
    | { kind: 'done' }
    | { kind: 'refused'; text: string }
    | { kind: 'unknown' }
    | { kind: 'missing' };

const fieldClass = 'w-full rounded-lg border border-border bg-surface-2 px-3 py-2 text-fg outline-none transition-shadow focus:border-primary';

export function AccountLinkPage() {
    const { t, screensReady } = useI18n();
    const location = useLocation();
    const navigate = useNavigate();
    const purpose = accountLinkPurpose(location.pathname);
    // Read once: the fragment is removed from the address bar below.
    const [token] = useState(() => accountLinkToken(window.location.hash));
    const [phase, setPhase] = useState<Phase>(() => (purpose && token ? { kind: 'checking' } : { kind: 'missing' }));
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
        if (!purpose || !token || sent.current) return;
        sent.current = true;
        const refuse = async (res: Response) => setPhase({ kind: 'refused', text: apiErrorText(await readApiError(res), t) });
        if (purpose === 'email_verify') {
            postJSON(RECOVERY_VERIFY_URL, { token })
                .then(async (res) => (res.ok ? setPhase({ kind: 'done' }) : refuse(res)))
                .catch(() => setPhase({ kind: 'unknown' }));
            return;
        }
        postJSON(RECOVERY_LINK_URL, { token, purpose })
            .then(async (res) => {
                if (!res.ok) return refuse(res);
                const body = (await res.json()) as { username?: unknown };
                setPhase({ kind: 'form', username: typeof body.username === 'string' ? body.username : '' });
            })
            .catch(() => setPhase({ kind: 'refused', text: t('common.resultUnknown') }));
        // The link is read once per page load.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        if (phase.kind !== 'form' || password.length < 8 || password !== repeat || !token) return;
        const username = phase.username;
        setFormProblem('');
        setPhase({ kind: 'saving', username });
        try {
            const res = await postJSON(purpose === 'invitation' ? RECOVERY_ACCEPT_URL : RECOVERY_RESET_URL, { token, new_password: password });
            if (res.ok) {
                setPassword('');
                setRepeat('');
                setPhase({ kind: 'done' });
                return;
            }
            const problem = await readApiError(res);
            // A too-short or too-long password leaves the link usable.
            if (problem.code === 'account_password_too_short' || problem.code === 'account_password_too_long') {
                setFormProblem(apiErrorText(problem, t));
                setPhase({ kind: 'form', username });
                return;
            }
            setPhase({ kind: 'refused', text: apiErrorText(problem, t) });
        } catch {
            setPhase({ kind: 'unknown' });
        }
    };

    const title = purpose === 'invitation' ? t('acct.link.invitationTitle') : purpose === 'email_verify' ? t('acct.link.verifyTitle') : t('acct.link.resetTitle');
    const doneText = purpose === 'invitation' ? t('acct.link.invitationDone') : purpose === 'email_verify' ? t('acct.link.verifyDone') : t('acct.link.resetDone');
    const signIn = (
        <button
            type="button"
            onClick={() => navigate('/', { replace: true })}
            className="w-full rounded-lg bg-primary px-3 py-2.5 font-medium text-primary-fg transition-colors hover:bg-primary-hover"
        >
            {t('acct.link.signIn')}
        </button>
    );
    const mismatch = repeat !== '' && password !== repeat;
    const Icon = purpose === 'email_verify' ? MailCheck : KeyRound;

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
                <div className="space-y-4 rounded-2xl border border-border bg-surface p-6" data-account-link={purpose ?? ''} data-phase={phase.kind}>
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
                                    {purpose === 'email_verify' ? t('acct.link.verifying') : t('acct.link.checking')}
                                </p>
                            )}
                            {phase.kind === 'missing' && (
                                <>
                                    <p className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm">{t('acct.link.missing')}</p>
                                    {signIn}
                                </>
                            )}
                            {(phase.kind === 'form' || phase.kind === 'saving') && (
                                <form onSubmit={(event) => void save(event)} className="space-y-4" noValidate>
                                    {phase.username && (
                                        <p className="text-sm text-fg-muted">{t('acct.link.account', { username: phase.username })}</p>
                                    )}
                                    {/* A hidden user name lets a password manager store the new password for the right account. */}
                                    <input type="text" name="username" autoComplete="username" value={phase.username} readOnly hidden />
                                    <label className="block">
                                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('acct.link.password')}</span>
                                        <input type="password" autoComplete="new-password" autoFocus value={password} maxLength={1024}
                                            disabled={phase.kind === 'saving'} onChange={(e) => setPassword(e.target.value)} className={fieldClass} />
                                    </label>
                                    <label className="block">
                                        <span className="mb-1.5 block text-sm font-medium text-fg-muted">{t('acct.link.repeat')}</span>
                                        <input type="password" autoComplete="new-password" value={repeat} maxLength={1024}
                                            disabled={phase.kind === 'saving'} onChange={(e) => setRepeat(e.target.value)} className={fieldClass} />
                                    </label>
                                    {mismatch && <p role="alert" className="text-sm text-danger">{t('acct.link.mismatch')}</p>}
                                    {formProblem && <p role="alert" className="text-sm text-danger">{formProblem}</p>}
                                    <p className="text-xs text-fg-subtle">{t('acct.link.hint')}</p>
                                    <button
                                        type="submit"
                                        disabled={phase.kind === 'saving' || password.length < 8 || password !== repeat}
                                        className="w-full rounded-lg bg-primary px-3 py-2.5 font-medium text-primary-fg transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
                                    >
                                        {phase.kind === 'saving' ? t('acct.link.saving') : t('acct.link.save')}
                                    </button>
                                </form>
                            )}
                            {phase.kind === 'done' && (
                                <>
                                    <p role="status" className="flex items-start gap-2 rounded-lg border border-success/40 bg-success/10 px-3 py-2.5 text-sm">
                                        <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                                        <span>{doneText}</span>
                                    </p>
                                    {signIn}
                                </>
                            )}
                            {phase.kind === 'refused' && (
                                <>
                                    <p role="alert" className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2.5 text-sm text-danger">{phase.text}</p>
                                    {signIn}
                                </>
                            )}
                            {phase.kind === 'unknown' && (
                                <>
                                    <p role="status" className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm">{t('acct.link.unknown')}</p>
                                    {signIn}
                                </>
                            )}
                        </>
                    )}
                </div>
            </div>
        </div>
    );
}
