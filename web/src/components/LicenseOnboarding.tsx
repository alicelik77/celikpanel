import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from '../router';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { Button, Spinner } from './ui';
import { parseAccessObservation, type AccessObservation } from '../lib/accessObservation';
import { readApiError } from '../lib/apiError';
import { useQuietRead } from '../lib/quietRead';
import { systemUpdateAccessGraceEnd, useSystemUpdateRestart } from '../lib/systemUpdateRestartSignal';
import { RecoveryAccess } from './RecoveryAccess';
import { AccessHold } from './AccessHold';

const LicenseLockScreen = lazy(() => import('./LicenseLockScreen').then(module => ({ default: module.LicenseLockScreen })));

/** While access is unknown the server's answer is read again this often in a visible tab. */
const UNKNOWN_RECHECK_MS = 5000;

/**
 * Why the last access read did not confirm access, as that read showed it. The
 * license is named only when the Panel answered and its license result was what
 * could not be read (seventh native record, cell 1: during an update's planned
 * restart the layer said "could not read the license result" while the Panel
 * itself was not answering).
 * - license: the Panel answered; the license result was unavailable, or a
 *   management request was refused for want of a license decision.
 * - starting: the Panel answered that it is still starting.
 * - availability: no answer arrived from the Panel (a dropped or refused
 *   connection, a timeout, or an answer from something in front of it).
 *
 * Son erisim okumasinin erisimi neden dogrulamadigi, o okumanin gosterdigi
 * bicimiyle. Lisans yalnizca Panel yanit verdiginde ve okunamayan sey lisans
 * sonucu oldugunda anilir.
 */
export type AccessReadCause = 'license' | 'starting' | 'availability';

/**
 * Never mount management pages before a positive, server-verified decision.
 *
 * Once they are mounted, only a KNOWN negative decision (license missing,
 * expired or invalid) takes them away. A decision that ran out, or a read that
 * failed, leaves access unknown: the pages stay mounted and unreachable behind
 * AccessHold until the server confirms access again. The decision itself, its
 * validity and what the server refuses without it are unchanged.
 *
 * Yonetim sayfalari, sunucunun dogruladigi olumlu karardan once baglanmaz.
 * Baglandiktan sonra onlari yalnizca BILINEN olumsuz karar kaldirir. Suresi dolan
 * karar ya da basarisiz okuma erisimi bilinmez kilar: sayfalar bagli kalir ve
 * sunucu erisimi yeniden dogrulayana kadar AccessHold arkasinda erisilmezdir.
 */
export function LicenseOnboarding({ children, onAccessChange, onRecoveryChange, suspended = false }: {
    children?: ReactNode; onAccessChange?: (allowed: boolean) => void; onRecoveryChange?: (recovering: boolean) => void;
    /** The session or panel readiness is itself unknown and already explained by an outer hold. */
    suspended?: boolean;
}) {
    const { user } = useAuth();
    const navigate = useNavigate();
    const location = useLocation();
    const { t, screensReady, screensFailed } = useI18n();
    const [access, setAccess] = useState<(AccessObservation & { owner: string }) | null>(null);
    const [failed, setFailed] = useState(false);
    const [pending, setPending] = useState(false);
    const [cause, setCause] = useState<AccessReadCause>('license');
    const controller = useRef<AbortController | null>(null);
    const mountedFor = useRef<string | null>(null);
    const latest = useRef(access);
    latest.current = access;
    const lastRead = useRef(0);
    // The update this browser follows, and the gate's own first lost read of the
    // current outage (set17 D1). While the update's planned restart lasts, a
    // read that got no answer from the Panel does not end a positive decision:
    // the update's own screen owns the page, and nothing more is drawn here
    // (lib/systemUpdateRestartSignal.ts). An answer from the Panel is applied
    // as before.
    // Guncellemenin planli yeniden baslamasi surerken yanitsiz okuma olumlu
    // karari bitirmez; Panel'in yaniti her zamanki gibi uygulanir.
    const published = useSystemUpdateRestart();
    const [lostSince, setLostSince] = useState<number | null>(null);
    const lostSinceRef = useRef<number | null>(null);
    // The update's last signal stays the evidence while the gate itself has not
    // been answered again: the update's read that ends its restart can also be
    // the one that ends its tracking (an outcome read), and the decision must
    // not lapse in the moment before the gate's own read answers.
    const lastRestart = useRef(published);
    if (published) lastRestart.current = published;
    else if (lostSince === null) lastRestart.current = null;
    const restart = published ?? lastRestart.current;
    const restartRef = useRef(restart);
    restartRef.current = restart;
    const graceEnd = useCallback((until: number) => systemUpdateAccessGraceEnd(restartRef.current, until * 1000, lostSinceRef.current), []);
    const check = useCallback(async () => {
        // Focus and periodic checks share the pending request.
        // Odak ve zamanlayıcı kontrolleri sürmekte olan isteği paylaşır.
        if (controller.current) return;
        const request = new AbortController();
        controller.current = request;
        lastRead.current = Date.now();
        // The deadline is dated from the moment of asking (accessObservation).
        // Son an sorulan andan itibaren hesaplanır (accessObservation).
        const askedAt = lastRead.current;
        setPending(true);
        const timeout = window.setTimeout(() => request.abort(), 15000);
        // Until an answer arrives, nothing came back from the Panel.
        let observed: AccessReadCause = 'availability';
        try {
            const response = await fetch('/api/v1/license/access', { cache: 'no-store', signal: request.signal });
            if (!response.ok) {
                // The Panel's own access route answers 200, also while it starts; it refuses
                // with PANEL_STARTING only for what is not yet served. Any other status
                // came from in front of the Panel, not from its license check.
                const problem = await readApiError(response);
                observed = problem.code === 'PANEL_STARTING' || problem.code === 'panel_starting' ? 'starting' : 'availability';
                throw new Error('access unavailable');
            }
            observed = 'license';
            const result = parseAccessObservation(await response.json(), Date.now() / 1000, askedAt / 1000);
            if (!request.signal.aborted && controller.current === request) {
                lostSinceRef.current = null;
                setLostSince(null);
                setAccess({ owner: user.username, ...result });
                setFailed(result.allowed === null);
                if (result.allowed === null) setCause('license');
            }
        } catch {
            if (controller.current === request) {
                setCause(observed);
                // No answer from the Panel (or "still starting"): the first such read
                // of this outage dates the gate's side of the update's restart.
                const unanswered = observed !== 'license';
                if (unanswered && lostSinceRef.current === null) {
                    lostSinceRef.current = askedAt;
                    setLostSince(askedAt);
                }
                // A failed request is not a license rejection. Keep only an
                // unexpired decision for this identity; never extend its deadline,
                // except through the followed update's restart window.
                // Bağlantı hatası lisans reddi değildir; bu kimliğin geçerli kararı
                // yalnızca sunucunun belirlediği süre (ya da izlenen güncellemenin
                // yeniden başlama penceresi) dolana kadar korunur.
                setAccess(previous => previous?.owner === user.username
                    && (previous.allowed === false || (previous.allowed && (previous.until * 1000 > Date.now()
                        || (unanswered && (graceEnd(previous.until) ?? 0) > Date.now()))))
                    ? previous : { owner: user.username, allowed: null, until: 0, state: 'status_unavailable' });
                setFailed(true);
            }
        } finally {
            window.clearTimeout(timeout);
            if (controller.current === request) { controller.current = null; setPending(false); }
        }
    }, [graceEnd, user.username]);

    useEffect(() => {
        void check();
        // Only a mounted authenticated browser checks; the server has no idle timer.
        const interval = window.setInterval(() => { if (document.visibilityState === 'visible') void check(); }, 60000);
        // A browser may delay the timers of a hidden tab. On return the deadline is
        // applied by the clock before anything is read, so a decision that has run
        // out is never used while its replacement is in flight.
        // Tarayici gizli sekmenin zamanlayicilarini geciktirebilir. Donuste sure
        // saate gore uygulanir; suresi dolan karar yenisi okunurken kullanilmaz.
        const returned = () => {
            void check();
            setAccess(previous => previous?.allowed && previous.until * 1000 <= Date.now()
                && !((graceEnd(previous.until) ?? 0) > Date.now()) ? { ...previous, allowed: null } : previous);
        };
        const shown = () => { if (document.visibilityState === 'visible') returned(); };
        const locked = () => {
            // The server refused a management request for want of a current
            // decision. Pages that stay mounted report this with every poll; one
            // read answers all of them, so a read that began after access became
            // unknown is left to finish. Only typed status can require activation.
            const known = latest.current?.allowed !== null;
            if (known) { controller.current?.abort(); controller.current = null; }
            // The read is in flight before access is marked unknown, so no render
            // shows an unknown state that nothing is reading. Only the report that
            // makes access unknown reads at once. While it stays unknown the regular
            // re-check reads, and further refusals cannot read more often than that:
            // a refusal answered by another refusal would otherwise read without pause.
            // Yalnizca erisimi bilinmez kilan bildirim hemen okur; bilinmezken gelen
            // retler olagan yeniden kontrol araligindan sik okuma baslatamaz.
            if (known || Date.now() - lastRead.current >= UNKNOWN_RECHECK_MS) void check();
            setFailed(true);
            // The Panel answered that request with a license refusal.
            setCause('license');
            setAccess({ owner: user.username, allowed: null, until: 0, state: 'status_unavailable' });
        };
        window.addEventListener('focus', returned);
        document.addEventListener('visibilitychange', shown);
        window.addEventListener('celikpanel:license-locked', locked);
        return () => {
            controller.current?.abort(); controller.current = null;
            window.clearInterval(interval);
            window.removeEventListener('focus', returned);
            document.removeEventListener('visibilitychange', shown);
            window.removeEventListener('celikpanel:license-locked', locked);
        };
    }, [check, graceEnd, user.username]);

    useEffect(() => {
        if (!access?.allowed) return;
        // Refresh shortly before the server's hard deadline so normal renewals
        // preserve the current page and form state.
        const refreshDelay = access.until * 1000 - Date.now() - 15000;
        const refreshTimer = refreshDelay > 0 ? window.setTimeout(() => {
            if (document.visibilityState === 'visible') void check();
        }, Math.min(2147483647, refreshDelay)) : undefined;
        // At the deadline the decision is no longer used. That is not a failed
        // check: a hidden tab is deliberately not refreshed, and it reads again
        // when the owner returns to it. While the followed update restarts the
        // Panel, the decision lasts to the end of its restart window instead;
        // the access is still read at its own deadline (and then every few
        // seconds, below), so an answer ends the wait as soon as there is one.
        // Izlenen guncelleme Paneli yeniden baslatirken karar, yeniden baslama
        // penceresinin sonuna kadar surer; erisim yine kendi son aninda okunur.
        const until = access.until * 1000;
        const end = Math.max(until, systemUpdateAccessGraceEnd(restart, until, lostSince) ?? 0);
        const lapse = () => {
            if (document.visibilityState === 'visible') void check();
            setAccess(previous => previous ? { ...previous, allowed: null } : null);
        };
        const delay = (at: number) => Math.min(2147483647, Math.max(0, at - Date.now()));
        // Inside the window with the deadline already past, a gate that has not
        // failed yet reads once now; one that has failed reads on its re-check.
        const readAtUntil = end > until && (until > Date.now() || !failed);
        const timer = end > until
            ? (readAtUntil ? window.setTimeout(() => { if (document.visibilityState === 'visible') void check(); }, delay(until)) : undefined)
            : window.setTimeout(lapse, delay(until));
        const graceTimer = end > until ? window.setTimeout(lapse, delay(end)) : undefined;
        return () => { window.clearTimeout(timer); window.clearTimeout(graceTimer); window.clearTimeout(refreshTimer); };
    }, [access, check, restart, lostSince, failed]);

    const known = access?.owner === user.username ? access : null;
    const allowed = known?.allowed === true;
    const denied = known?.allowed === false;
    // Management was mounted for this identity and no known negative has arrived since.
    if (allowed) mountedFor.current = user.username;
    else if (denied || mountedFor.current !== user.username) mountedFor.current = null;
    const held = !allowed && mountedFor.current === user.username;
    const unknown = !allowed && !denied && access !== null;
    // The first read of this identity has not answered yet (nothing is mounted).
    const quiet = useQuietRead(!failed && !known);
    // A positive decision kept through the update's restart: nothing is drawn
    // here, and the access is read again quietly until the Panel answers.
    const restartQuiet = allowed && failed && restart !== null && (restart.lostSince !== null || lostSince !== null);

    useEffect(() => {
        if (!unknown && !restartQuiet) return;
        const timer = window.setInterval(() => { if (document.visibilityState === 'visible') void check(); }, UNKNOWN_RECHECK_MS);
        return () => window.clearInterval(timer);
    }, [unknown, restartQuiet, check]);
    // The update tracking reached the Panel again: read the access at once, so the
    // decision is renewed before the restart window would end it.
    const trackingLost = published?.lostSince ?? null;
    const trackingLostBefore = useRef(trackingLost);
    useEffect(() => {
        const before = trackingLostBefore.current;
        trackingLostBefore.current = trackingLost;
        if (before !== null && trackingLost === null && failed && document.visibilityState === 'visible') void check();
    }, [trackingLost, failed, check]);

    // Pause the root update overlay while activation owns the screen. This
    // changes browser tracking only; the server's running operation continues.
    useLayoutEffect(() => { onAccessChange?.(!!allowed); }, [allowed, onAccessChange]);
    useLayoutEffect(() => {
        onRecoveryChange?.(!!screensFailed || (!!failed && !allowed));
        return () => onRecoveryChange?.(false);
    }, [screensFailed, failed, allowed, onRecoveryChange]);
    useEffect(() => {
        if (!access || access.owner !== user.username) return;
        if (access.allowed === false && location.pathname !== '/activate') navigate('/activate', { replace: true });
        if (allowed && location.pathname === '/activate') navigate('/', { replace: true });
    }, [access, allowed, location.pathname, navigate, user.username]);
    if (allowed || held) return <AccessHold active={held} silent={suspended} cause={cause} checking={pending} user={user} onRetry={() => void check()}>
        {allowed && failed && !restartQuiet && <div role="status" className="flex flex-wrap items-center justify-center gap-3 border-b border-border bg-surface px-4 py-3 text-sm text-fg">
            <p className="max-w-prose">{t('license.connectionRetry')}</p>
            <Button variant="secondary" onClick={() => void check()}>{t('license.refresh')}</Button>
            <Button variant="secondary" onClick={() => window.location.reload()}>{t('common.reloadPage')}</Button>
        </div>}
        {children}
    </AccessHold>;
    const loading = <div className="min-h-screen flex items-center justify-center bg-bg"><Spinner /></div>;
    if (screensFailed) return <RecoveryAccess user={user} cause="bundle" onRetry={() => void check()} />;
    if (failed && !allowed) return <RecoveryAccess user={user} cause={cause} onRetry={() => void check()} />;
    // Before the quiet time a first read that has not answered is not a page: only the background.
    if (quiet) return <div className="min-h-screen bg-bg" aria-busy="true" data-access-quiet="" />;
    if (!screensReady) return loading;
    return <Suspense fallback={loading}><LicenseLockScreen checking={!access || access.owner !== user.username || access.allowed === null} failed={failed} onCheck={() => void check()} /></Suspense>;
}
