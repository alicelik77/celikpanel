import { useEffect, useState } from 'react';
import { BadgeCheck, Eye, EyeOff } from 'lucide-react';
import { useI18n } from '../i18n';
import { useNavigate, useSearchParams } from '../router';
import { Button, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { decodeLicenseCheck, licenseCheckGuidance, licenseCheckVars } from '../lib/licenseOffline';

type LicenseStatus = { state: 'active' | 'missing' | 'invalid' | 'expired' | 'verification_unavailable' | 'status_unavailable'; can_provision: boolean; expires_at?: number; license_id?: string };
type ShownStatus = LicenseStatus & ReturnType<typeof decodeLicenseCheck>;
const states = ['active', 'missing', 'invalid', 'expired', 'verification_unavailable', 'status_unavailable'] as const;
// The answer is the contract or it is not shown.
const contract = (result: LicenseStatus) => states.includes(result.state) && typeof result.can_provision === 'boolean' && !(result.can_provision && result.state !== 'active')
    && (result.expires_at === undefined || (Number.isSafeInteger(result.expires_at) && result.expires_at > 0));
const shown = (result: LicenseStatus): ShownStatus => ({ ...result, ...decodeLicenseCheck(result as unknown as Record<string, unknown>) });

// A failure this page has its own sentence for, with what the server said
// (a code or its words) kept for Technical details.
class LicenseFailure extends Error {
    constructor(message: string, readonly detail: string) { super(message); }
}

export function LicensePanel({ locked = false, onContinue }: { locked?: boolean; onContinue?: () => void }) {
    const { t, locale } = useI18n();
    const [params] = useSearchParams();
    const navigate = useNavigate();
    const setup = locked || params.get('setup') === '1';
    const [status, setStatus] = useState<ShownStatus | null>(null);
    const [key, setKey] = useState('');
    const [showKey, setShowKey] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [errorDetail, setErrorDetail] = useState('');
    const [keyError, setKeyError] = useState('');
    async function request(action?: 'activate' | 'refresh', signal?: AbortSignal, reread = false) {
        const normalizedKey = key.trim();
        if (action === 'activate' && !/^CPK-[a-f0-9]{64}$/.test(normalizedKey)) {
            setKeyError(t('license.keyInvalid'));
            return;
        }
        setBusy(true); if (!reread) { setError(''); setErrorDetail(''); } setKeyError('');
        try {
            const response = await fetch('/api/v1/panel/license', action ? {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action, ...(action === 'activate' ? { key: normalizedKey } : {}) }), signal,
            } : { signal, cache: 'no-store' });
            if (!response.ok) {
                const problem = await readApiError(response);
                if (action) {
                    const message = problem.message;
                    const reason = message === 'license: license_in_use' ? 'license.inUse'
                        : message === 'license: invalid_license' || message === 'invalid license key' ? 'license.unknownKey'
                        : message === 'license: license_expired' ? 'license.expiredHelp'
                        : message === 'license: rate_limited' ? 'license.tryLater'
                        : problem.code === 'license_unavailable' || message === 'license service unavailable; existing license retained' ? 'license.unavailable'
                        : 'license.actionFailed';
                    throw new LicenseFailure(t(reason), problem.code || problem.message);
                }
                // The page's own sentence; the server's words under Technical details.
                const text = apiErrorText(problem, t, 'license.loadFailed');
                throw new LicenseFailure(problem.code && text !== problem.message ? text : t('license.loadFailed'), problem.code || problem.message);
            }
            const result = await response.json() as LicenseStatus;
            if (signal?.aborted) return;
            if (!contract(result)) throw new Error(t('license.loadFailed'));
            setStatus(shown(result));
            if (action === 'activate') { setKey(''); setShowKey(false); }
            // Activation must immediately recheck the shared access gate. Only
            // that server-verified decision can mount the management pages.
            if (setup && result.state === 'active' && result.can_provision) {
                if (onContinue) onContinue();
                else navigate('/', { replace: true });
            }
        } catch (cause) {
            // A re-read after a failed check keeps that check's error.
            // Only this page's own sentences are shown; anything else (a reply
            // that is not JSON…) is the could-not-load sentence, with what was
            // seen under Technical details (owner feedback 2026-10-31).
            if (!signal?.aborted && !reread) {
                setError(cause instanceof TypeError ? t('license.lockError') : cause instanceof LicenseFailure ? cause.message : t('license.loadFailed'));
                setErrorDetail(cause instanceof LicenseFailure ? cause.detail : cause instanceof Error && !(cause instanceof TypeError) && cause.name !== 'Error' ? cause.name : '');
            }
            // A failed "Check again" leaves the receipt as it was; read the status
            // again so the page shows how long management stays open and why.
            if (action === 'refresh') await request(undefined, undefined, true);
        } finally { if (!signal?.aborted) setBusy(false); }
    }
    useEffect(() => { const controller = new AbortController(); void request(undefined, controller.signal); return () => controller.abort(); }, []);
    const Heading = locked ? 'h1' : 'h2';
    const unavailable = status?.state === 'verification_unavailable' || status?.state === 'status_unavailable';
    // The license check itself (D-039): an unreachable service inside the offline
    // window, its last 48 hours, a wrong clock, or a lapse with its cause.
    const guidance = status ? licenseCheckGuidance(status) : null;
    const vars = status ? licenseCheckVars(status, locale) : {};
    const lapsed = unavailable && guidance?.tone === 'blocked' ? guidance : null;
    return <section className="rounded-xl border border-border bg-surface p-5 sm:p-6" aria-labelledby="license-heading">
        <Heading id="license-heading" className="mb-3 flex items-center gap-2 text-lg font-semibold text-fg"><BadgeCheck className="h-5 w-5 shrink-0" />{lapsed ? t(lapsed.title) : t(unavailable ? 'recovery.licenseTitle' : setup && status ? status.can_provision ? 'license.readyTitle' : status?.state === 'expired' ? 'license.state.expired' : 'license.setupTitle' : 'license.title')}</Heading>
        {status && <p className="mb-5 max-w-prose text-sm text-fg-muted">{lapsed ? t(lapsed.help, vars) : t(unavailable ? 'recovery.licenseHelp' : status?.state === 'expired' ? 'license.expiredHelp' : setup && !status?.can_provision ? 'license.setupIntro' : 'license.description')}</p>}
        {guidance && !lapsed && <div role={guidance.tone === 'warning' ? 'alert' : 'status'} data-license-check={status?.check}
            className={`mb-5 max-w-prose rounded-lg border p-3 text-sm text-fg ${guidance.tone === 'warning' ? 'border-warning-mark/60 bg-warning-mark/20' : 'border-border bg-bg'}`}>
            <p className="font-semibold">{t(guidance.title, vars)}</p>
            <p className="mt-1">{t(guidance.help, vars)}</p>
        </div>}
        {error && <div role="alert" className="mb-4 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm text-danger"><p>{error}</p>{errorDetail && <details className="mt-1 text-xs text-fg-muted"><summary className="cursor-pointer">{t('common.technicalDetails')}</summary><code className="break-words">{errorDetail}</code></details>}</div>}
        {status ? <div className="mb-6 space-y-2 text-sm" role="status">
            {!unavailable && (!setup || (!status.can_provision && status.state !== 'missing' && status.state !== 'expired')) && <p className="font-semibold">{t(`license.state.${status.state as Exclude<LicenseStatus['state'], 'status_unavailable'>}`)}</p>}
            {status.expires_at && <p>{t('license.expires')}: {new Date(status.expires_at * 1000).toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-US')}</p>}
            {status.state === 'active' && vars.since && <p className="text-fg-muted">{t('license.lastCheck', vars)}</p>}
            {!status.can_provision && !unavailable && <p className="max-w-prose text-fg-muted">{t('license.restricted')}</p>}
            {setup && status.can_provision && <p>{t('license.setupSuccess')}</p>}
        </div> : <p role="status" className="mb-4 text-sm">{busy ? t('common.loading') : t('license.loadFailed')}</p>}
        {status && !unavailable && !(setup && status.can_provision) && <form noValidate onSubmit={event => { event.preventDefault(); void request('activate'); }} className="space-y-3">
            <label htmlFor="license-key" className="block text-sm font-medium">{t('license.key')}</label>
            <div className="flex items-stretch gap-2">
                <input id="license-key" name="license_key" type={showKey ? 'text' : 'password'} autoComplete="off" autoCapitalize="off" spellCheck={false} value={key} onChange={event => { setKey(event.target.value); setKeyError(''); }} className={`${inputClass} min-w-0 flex-1`} aria-invalid={!!keyError} aria-describedby={keyError ? 'license-key-help license-key-error' : 'license-key-help'} disabled={busy} />
                <Button type="button" variant="secondary" disabled={busy} aria-label={t(showKey ? 'license.hideKey' : 'license.showKey')} aria-pressed={showKey} onClick={() => setShowKey(value => !value)}>{showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</Button>
            </div>
            <p id="license-key-help" className="text-sm text-fg-muted">{t('license.keyHelp')}</p>
            {keyError && <p id="license-key-error" role="alert" className="text-sm text-danger">{keyError}</p>}
            <div className="flex flex-wrap items-center gap-4 pt-2">
                <Button type="submit" disabled={busy || !key.trim()}>{t(busy ? 'common.loading' : 'license.activate')}</Button>
                <a href="https://celikpanel.net/account/" target="_blank" rel="noopener noreferrer" className="text-sm text-primary underline underline-offset-4">{t(status?.state === 'missing' ? 'license.getKey' : 'license.manage')}</a>
            </div>
        </form>}
        {!(setup && status?.can_provision) && (!status || status.state !== 'missing') && <div className="mt-6 flex flex-wrap items-center gap-4 border-t border-border pt-5">
            {(!status || status.state !== 'missing') && <Button variant="secondary" disabled={busy} onClick={() => void request(status ? 'refresh' : undefined)}>{t('license.refresh')}</Button>}
        </div>}
    </section>;
}
