import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, DownloadCloud, Info, Loader2, RefreshCw } from 'lucide-react';
import { Button } from './ui';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import {
    fetchHostMutationReadiness,
    runHostMutationAdmission,
    unverifiedHostMutationReadiness,
    type HostMutationReadiness,
} from '../lib/panelUpdateAdmission';
import {
    createSystemUpdateRequestID,
    systemUpdateResponseHint,
    useSystemUpdateOperation,
    validUpdateTarget,
    validUpdateVersion,
    type UpdateMarker,
    type UpdateTarget,
} from './SystemUpdateOperation';
import { decodePreviousUpdateAttempt, previousAttemptStopped, type PreviousUpdateAttempt } from '../lib/systemUpdateOutcome';
import { SystemUpdateSteps, systemUpdateStepsAnnouncement } from './SystemUpdateSteps';
import { BuildCommit } from './BuildCommit';
import {
    PANEL_UPDATE_DISCOVERY_URL,
    clockTime,
    decodePanelUpdateDiscovery,
    publishPanelUpdateDiscovery,
    refreshPanelUpdateDiscovery,
    relativeAge,
    usePanelUpdateDiscovery,
    type PanelUpdateDiscovery,
} from '../lib/panelUpdateDiscovery';

type Translate = ReturnType<typeof useI18n>['t'];

type UpdateCheck = {
    supported: boolean;
    available: boolean;
    current_version: string;
    current_commit: string;
    target?: UpdateTarget;
    /** Additive: the last failed or rolled-back attempt to this exact target on this server. */
    previous_attempt?: PreviousUpdateAttempt;
};

// The bottom message is kept for what the owner's own press could not do: a
// check or a start that failed (warning mark). What the panel knows about
// updates is the one status line at the top (alpha.96, D-035), so a found
// update or the latest version is never said a second time.
// Alttaki ileti yalniz basarisiz kontrol ya da baslatma icindir; bilinen durum
// ustteki tek durum satiridir.
type MessageTone = 'warning';

type PanelBuild = {
    version: string;
    commit: string;
};

export const PANEL_UPDATE_CHECK_TIMEOUT_MS = 8000;
// While the card says the server is busy it reads readiness again by itself,
// as the Components page does (owner rule 2026-10-14, audit row 25): GET only,
// one read at a time, only while the tab is visible. It never starts or admits
// an update; Start still makes its own fresh admission check.
// Kart sunucu mesgul derken hazirligi kendisi yeniden okur: yalniz GET, ayni
// anda tek okuma, yalniz sekme gorunurken. Guncellemeyi asla baslatmaz.
export const PANEL_UPDATE_READINESS_POLL_MS = 5000;

export type PanelUpdateCheckRuntime = {
    fetch: typeof fetch;
    setTimeout: (callback: () => void, delay: number) => ReturnType<typeof globalThis.setTimeout>;
    clearTimeout: (timer: ReturnType<typeof globalThis.setTimeout>) => void;
};

const commitPattern = /^[a-f0-9]{40}$/;

function decodeUpdateCheck(payload: unknown): UpdateCheck | null {
    if (!payload || typeof payload !== 'object') return null;
    const value = payload as Record<string, unknown>;
    if (value.supported !== true || typeof value.available !== 'boolean'
        || !validUpdateVersion(value.current_version)
        || typeof value.current_commit !== 'string' || !commitPattern.test(value.current_commit)) return null;
    if (value.available) {
        if (!validUpdateTarget(value.target)) return null;
        const previousAttempt = decodePreviousUpdateAttempt(value.previous_attempt);
        return {
            supported: true, available: true,
            current_version: value.current_version, current_commit: value.current_commit,
            target: value.target,
            ...(previousAttempt ? { previous_attempt: previousAttempt } : {}),
        };
    }
    if (value.target !== undefined) return null;
    return {
        supported: true, available: false,
        current_version: value.current_version, current_commit: value.current_commit,
    };
}

async function codedResponseHint(response: Response, t: Translate): Promise<{ code?: string; message: string }> {
    const apiError = await readApiError(response);
    return {
        ...(apiError.code ? { code: apiError.code } : {}),
        message: apiError.code ? apiErrorText(apiError, t) : systemUpdateResponseHint(response.status, t),
    };
}

export async function fetchPanelUpdateCheck(
    t: Translate,
    runtime: PanelUpdateCheckRuntime = {
        fetch: (...args) => fetch(...args),
        setTimeout: (callback, delay) => globalThis.setTimeout(callback, delay),
        clearTimeout: (timer) => globalThis.clearTimeout(timer),
    },
    externalSignal?: AbortSignal,
): Promise<UpdateCheck> {
    const controller = new AbortController();
    let rejectDeadline: (reason: Error) => void = () => undefined;
    const deadline = new Promise<never>((_resolve, reject) => { rejectDeadline = reject; });
    const abortAndReject = () => {
        controller.abort();
        rejectDeadline(new Error(t('panelUpdate.checkFailed')));
    };
    const timer = runtime.setTimeout(abortAndReject, PANEL_UPDATE_CHECK_TIMEOUT_MS);
    externalSignal?.addEventListener('abort', abortAndReject, { once: true });
    if (externalSignal?.aborted) abortAndReject();
    const requestAndDecode = (async () => {
        const response = await runtime.fetch('/api/v1/panel/update/check', {
            cache: 'no-store', credentials: 'same-origin', signal: controller.signal,
        });
        if (!response.ok) throw new Error((await codedResponseHint(response, t)).message);
        const payload = decodeUpdateCheck(await response.json());
        if (!payload) throw new Error(t('panelUpdate.unsupported'));
        return payload;
    })();
    try {
        return await Promise.race([requestAndDecode, deadline]);
    } finally {
        runtime.clearTimeout(timer);
        externalSignal?.removeEventListener('abort', abortAndReject);
    }
}

export function PanelUpdateCard({ activation = false }: { activation?: boolean }) {
    const { t, locale } = useI18n();
    const systemUpdate = useSystemUpdateOperation();
    const [currentBuild, setCurrentBuild] = useState<PanelBuild | null>(null);
    // The result of this card's own "Check for updates" press, if any.
    const [check, setCheck] = useState<UpdateCheck | null>(null);
    const [checking, setChecking] = useState(false);
    const [readinessChecking, setReadinessChecking] = useState(false);
    const [readiness, setReadiness] = useState<HostMutationReadiness | null>(null);
    const [starting, setStarting] = useState(false);
    const [message, setMessage] = useState('');
    const [messageTone, setMessageTone] = useState<MessageTone>('warning');
    const [savingAuto, setSavingAuto] = useState(false);
    const [autoError, setAutoError] = useState(false);
    const actionInFlight = useRef(false);
    const lifecycleGeneration = useRef(0);
    const readinessAbort = useRef<AbortController | null>(null);
    const updateCheckAbort = useRef<AbortController | null>(null);
    const readinessForTarget = useRef('');

    useEffect(() => () => {
        lifecycleGeneration.current += 1;
        readinessAbort.current?.abort();
        readinessAbort.current = null;
        updateCheckAbort.current?.abort();
        updateCheckAbort.current = null;
    }, []);

    // The stored result of the Panel's last check, manual or automatic
    // (D-035): the card opens already knowing it, with no button press and
    // no call to the release server. Read again once when the card opens.
    // Panelin son kontrolunun saklanan sonucu: kart dugmeye basilmadan bilir.
    const discovery = usePanelUpdateDiscovery(true);
    useEffect(() => { void refreshPanelUpdateDiscovery(); }, []);
    const stored: PanelUpdateDiscovery | null = discovery.value;

    // The update this browser follows (SystemUpdateOperation): the card says
    // what the notice says (owner report 2026-10-10).
    const tracking = systemUpdate.tracking ?? null;
    // The update's steps (owner design 2026-10-16): drawn while the update is
    // followed and once it succeeded; the overdue wording keeps its own row.
    const steps = systemUpdate.steps ?? null;
    const updated = systemUpdate.updated ?? null;
    const verifying = tracking?.phase === 'verifying';
    // This card is the one notice for the followed update while it is on
    // screen; the corner notice does not repeat it (owner report 2026-10-15).
    const registerInlineNotice = systemUpdate.registerInlineNotice;
    useEffect(() => registerInlineNotice?.(), [registerInlineNotice]);
    // Read on mount and again when the followed update is found installed, so
    // the version shown is the one this panel runs now. A GET only.
    useEffect(() => {
        let cancelled = false;
        fetch('/api/v1/panel/version', { cache: 'no-store', credentials: 'same-origin' })
            .then(async (response) => (response.ok ? response.json() as Promise<unknown> : null))
            .then((payload) => {
                if (cancelled || !payload || typeof payload !== 'object') return;
                const value = payload as Record<string, unknown>;
                if (typeof value.version !== 'string' || value.version.length > 80
                    || typeof value.commit !== 'string' || value.commit.length > 80) return;
                setCurrentBuild({ version: value.version, commit: value.commit });
            })
            .catch(() => undefined);
        return () => { cancelled = true; };
    }, [verifying]);

    // What is offered: this card's own check when it ran, otherwise the
    // stored one. The server checks the exact target again before any start.
    // Sunulan: kartin kendi kontrolu, yoksa saklanan sonuc.
    const storedTarget = stored?.target && validUpdateTarget(stored.target) ? stored.target as UpdateTarget : undefined;
    const storedPrevious = stored ? decodePreviousUpdateAttempt(stored.previousAttempt) : undefined;
    const offer: UpdateCheck | null = check
        ? check
        : storedTarget && stored && validUpdateVersion(stored.currentVersion) && commitPattern.test(stored.currentCommit)
            ? {
                supported: true, available: true,
                current_version: stored.currentVersion, current_commit: stored.currentCommit,
                target: storedTarget,
                ...(storedPrevious ? { previous_attempt: storedPrevious } : {}),
            }
            : null;

    async function refreshHostMutationReadiness(): Promise<HostMutationReadiness> {
        const generation = lifecycleGeneration.current;
        readinessAbort.current?.abort();
        const controller = new AbortController();
        readinessAbort.current = controller;
        setReadinessChecking(true);
        try {
            const next = await fetchHostMutationReadiness(undefined, controller.signal);
            if (lifecycleGeneration.current === generation && !controller.signal.aborted) {
                setReadiness(next);
            }
            return next;
        } catch {
            const next = unverifiedHostMutationReadiness();
            if (lifecycleGeneration.current === generation && !controller.signal.aborted) {
                setReadiness(next);
            }
            return next;
        } finally {
            if (readinessAbort.current === controller) readinessAbort.current = null;
            if (lifecycleGeneration.current === generation) setReadinessChecking(false);
        }
    }

    const target = offer?.target;
    const active = systemUpdate.active;
    // A stored update is followed by the readiness read at once, as a found
    // one always was; once per offered target.
    // Saklanan guncelleme icin hazirlik bir kez hemen okunur.
    const offeredKey = target && offer?.available && !active ? `${target.version}:${target.commit}` : '';
    useEffect(() => {
        if (!offeredKey || readinessForTarget.current === offeredKey || actionInFlight.current) return;
        readinessForTarget.current = offeredKey;
        void refreshHostMutationReadiness();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [offeredKey]);

    // The quiet re-read. It does not draw the card's "checking" look: the busy
    // text stays until a read says ready, then the card returns to its normal
    // state. A read that could not verify keeps what is shown.
    // Sessiz yeniden okuma: "denetleniyor" gorunumu cizilmez; hazir denene
    // kadar mesgul metni kalir. Dogrulanamayan okuma gosterileni degistirmez.
    const quietReadinessPoll = Boolean(target && offer?.available) && !active && !starting
        && !checking && !readinessChecking && readiness?.ready === false;
    useEffect(() => {
        if (!quietReadinessPoll) return;
        const generation = lifecycleGeneration.current;
        let stopped = false;
        let timer: number | undefined;
        let controller: AbortController | null = null;
        const visible = () => document.visibilityState === 'visible';
        const schedule = () => {
            if (stopped || timer !== undefined || controller || !visible()) return;
            timer = window.setTimeout(() => { timer = undefined; void read(); }, PANEL_UPDATE_READINESS_POLL_MS);
        };
        const read = async () => {
            if (stopped || !visible()) return;
            // A Retry, a check or a Start owns its own read; this one waits.
            if (actionInFlight.current) { schedule(); return; }
            const own = new AbortController();
            controller = own;
            const next = await fetchHostMutationReadiness(undefined, own.signal);
            if (controller === own) controller = null;
            if (stopped || own.signal.aborted || lifecycleGeneration.current !== generation) return;
            if (next.ready || next.reason !== 'state_unverified') {
                setReadiness(current => current && current.ready === next.ready
                    && (current.ready || (!next.ready && current.reason === next.reason)) ? current : next);
            }
            schedule();
        };
        // A tab shown again reads at once (as lib/visibleReads.ts does), then
        // the delay resumes from that read; a read already in flight is that
        // read (2026-10-15).
        // Yeniden gosterilen sekme hemen bir kez okur; suren okuma o okumadir.
        const onVisibility = () => {
            if (visible()) {
                if (timer !== undefined) window.clearTimeout(timer);
                timer = undefined;
                if (!controller) void read();
                return;
            }
            if (timer !== undefined) window.clearTimeout(timer);
            timer = undefined;
        };
        document.addEventListener('visibilitychange', onVisibility);
        schedule();
        return () => {
            stopped = true;
            if (timer !== undefined) window.clearTimeout(timer);
            controller?.abort();
            document.removeEventListener('visibilitychange', onVisibility);
        };
    }, [quietReadinessPoll]);

    async function retryHostMutationReadiness() {
        if (actionInFlight.current || systemUpdate.active) return;
        actionInFlight.current = true;
        try {
            await refreshHostMutationReadiness();
        } finally {
            actionInFlight.current = false;
        }
    }

    // The owner's press: the only place the browser asks the release server
    // (through the Panel). The Panel keeps its result as the last check; the
    // shared copy is read again so the sidebar and the Dashboard follow.
    // Sahibin basisi: tarayicinin surum sunucusuna sordugu tek yer.
    async function checkForUpdate() {
        if (actionInFlight.current || systemUpdate.active) return;
        const generation = lifecycleGeneration.current;
        updateCheckAbort.current?.abort();
        const controller = new AbortController();
        updateCheckAbort.current = controller;
        actionInFlight.current = true;
        setChecking(true);
        setMessage('');
        setReadiness(null);
        let found: UpdateCheck | null = null;
        try {
            const payload = await fetchPanelUpdateCheck(t, undefined, controller.signal);
            if (lifecycleGeneration.current !== generation || controller.signal.aborted) return;
            setCheck(payload);
            setCurrentBuild({ version: payload.current_version, commit: payload.current_commit });
            found = payload;
        } catch (error) {
            if (lifecycleGeneration.current === generation && !controller.signal.aborted) {
                setCheck(null);
                setMessageTone('warning');
                setMessage(error instanceof Error ? error.message : t('panelUpdate.checkFailed'));
            }
        } finally {
            if (updateCheckAbort.current === controller) updateCheckAbort.current = null;
            if (lifecycleGeneration.current === generation) setChecking(false);
            if (!found) actionInFlight.current = false;
        }
        // The Panel kept this press's result; the shared copy follows it.
        if (lifecycleGeneration.current === generation && !controller.signal.aborted) void refreshPanelUpdateDiscovery();
        if (!found) return;
        try {
            if (found.available && found.target) {
                readinessForTarget.current = `${found.target.version}:${found.target.commit}`;
                await refreshHostMutationReadiness();
            }
        } finally {
            actionInFlight.current = false;
        }
    }

    async function startUpdate() {
        const target = offer?.target;
        if (actionInFlight.current || systemUpdate.active || !offer?.available || !target) return;
        const generation = lifecycleGeneration.current;
        actionInFlight.current = true;
        setStarting(true);
        setMessage('');
        try {
            // This snapshot is advisory and intentionally has a bounded wait.
            // The backend remains the authoritative admission boundary, so a
            // fresh check is required even when the earlier UI snapshot was ready.
            await runHostMutationAdmission(refreshHostMutationReadiness, async () => {
                // Route changes unmount this card. A readiness response from the
                // abandoned generation must never create a marker or send POST.
                if (lifecycleGeneration.current !== generation) return;
                // Do not create a durable browser marker (and therefore do not mount
                // the full-page tracker) until the bounded preflight proves idle.
                const requestID = createSystemUpdateRequestID();
                if (!requestID) {
                    setMessageTone('warning');
                    setMessage(t('panelUpdate.randomFailed'));
                    return;
                }
                const exactMarker: UpdateMarker = {
                    marker_version: 1,
                    request_id: requestID,
                    current_version: offer.current_version,
                    current_commit: offer.current_commit,
                    target,
                    created_at: Date.now(),
                };
                // The provider durably stores the exact request before this component
                // is allowed to send the only start POST. Its overlay is mounted above
                // routing, so navigation cannot unmount the tracker.
                const result = await systemUpdate.start(exactMarker);
                if (lifecycleGeneration.current === generation && result.kind === 'failed') {
                    setMessageTone('warning');
                    setMessage(result.message);
                }
            });
        } finally {
            if (lifecycleGeneration.current === generation) setStarting(false);
            actionInFlight.current = false;
        }
    }

    // The owner's switch for the automatic check. Saving it changes this
    // setting only; the answer is the stored result again.
    // Sahibin otomatik kontrol anahtari; yalniz bu ayari degistirir.
    async function saveAutoCheck(next: boolean) {
        if (savingAuto) return;
        const generation = lifecycleGeneration.current;
        setSavingAuto(true);
        setAutoError(false);
        try {
            const response = await fetch(PANEL_UPDATE_DISCOVERY_URL, {
                method: 'PUT', cache: 'no-store', credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ auto_check: next }),
            });
            if (!response.ok) throw new Error(String(response.status));
            const value = decodePanelUpdateDiscovery(await response.json(), Date.now());
            if (!value) throw new Error('invalid');
            publishPanelUpdateDiscovery(value);
            // The Panel sets the next check right after saving; one read
            // shortly after shows its time.
            if (next) window.setTimeout(() => { void refreshPanelUpdateDiscovery(); }, 1500);
        } catch {
            if (lifecycleGeneration.current === generation) setAutoError(true);
        } finally {
            if (lifecycleGeneration.current === generation) setSavingAuto(false);
        }
    }

    const currentVersion = offer?.current_version ?? currentBuild?.version ?? stored?.currentVersion;
    const currentCommit = offer?.current_commit ?? currentBuild?.commit ?? stored?.currentCommit;
    const readinessReason = readiness?.ready === false
        ? t(`services.mutationReadiness.${readiness.reason}`)
        : '';
    const previousAttempt = target ? offer?.previous_attempt : undefined;
    // Either typed cause means it stopped before changing anything installed.
    const previousStopped = previousAttemptStopped(previousAttempt);
    const readinessTitle = readinessChecking
        ? t('services.mutationReadiness.checking')
        : readiness?.ready === true
            ? t('panelUpdate.ready')
            : t('services.mutationReadiness.title');

    // The one status line (D-024: what is known, since when, and what happens
    // next). A failed or skipped automatic check is information here and
    // nothing anywhere else.
    // Tek durum satiri: bilinen, ne zamandan beri ve sonra ne olacagi.
    const now = Date.now();
    const age = (moment: number) => relativeAge(moment, now, locale, t('panelUpdate.status.justNow'));
    const nextLine = stored?.autoCheck && stored.nextCheckAt !== null
        ? t('panelUpdate.status.next', { time: clockTime(stored.nextCheckAt, now, locale) })
        : stored && !stored.autoCheck ? t('panelUpdate.status.off') : '';
    type StatusLine = { tone: 'reading' | 'latest' | 'available' | 'info' | 'unread'; text: string; detail?: string };
    const last = stored?.last ?? null;
    const reasonText = last?.reason ? t(`panelUpdate.discoveryReason.${last.reason}`) : t('panelUpdate.discoveryReason.unknown');
    const lastProblem = last && (last.outcome === 'failed' || last.outcome === 'skipped')
        ? t(last.outcome === 'skipped' ? 'panelUpdate.status.lastSkipped'
            : last.automatic ? 'panelUpdate.status.lastFailed' : 'panelUpdate.status.lastFailedManual',
        { age: age(last.checkedAt), reason: reasonText })
        : '';
    // A press that failed says so in its own words below; the stored record of
    // that same failure is not said a second time.
    const manualFailureShown = message !== '' && last?.outcome === 'failed' && !last.automatic;
    let status: StatusLine | null;
    if (checking) {
        status = { tone: 'reading', text: t('panelUpdate.status.checkingNow') };
    } else if (check && !check.available) {
        status = { tone: 'latest', text: t('panelUpdate.status.currentManual', { age: last && !last.automatic && last.outcome === 'current' ? age(last.checkedAt) : t('panelUpdate.status.justNow') }) };
    } else if (offer?.available && target) {
        const foundAt = stored?.target && stored.target.commit === target.commit && stored.foundAt !== null ? stored.foundAt : now;
        status = {
            tone: 'available',
            text: t('panelUpdate.status.available', { version: target.version, age: age(foundAt) }),
            ...(!check && lastProblem && !manualFailureShown ? { detail: lastProblem } : {}),
        };
    } else if (!stored) {
        status = discovery.status === 'failed'
            ? { tone: 'unread', text: t('panelUpdate.status.readFailed') }
            : { tone: 'reading', text: t('panelUpdate.status.reading') };
    } else if (!last) {
        status = { tone: 'info', text: t('panelUpdate.status.never') };
    } else if (last.outcome === 'current') {
        status = { tone: 'latest', text: t(last.automatic ? 'panelUpdate.status.current' : 'panelUpdate.status.currentManual', { age: age(last.checkedAt) }) };
    } else {
        status = manualFailureShown ? null : { tone: 'info', text: lastProblem };
    }
    const showNext = !checking && status?.tone !== 'reading' && status?.tone !== 'unread' && nextLine !== '';
    const statusIcon = !status ? null : status.tone === 'reading'
        ? <Loader2 className="mt-0.5 h-5 w-5 shrink-0 motion-safe:animate-spin text-primary" aria-hidden="true" />
        : status.tone === 'latest'
            ? <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-success" aria-hidden="true" />
            : status.tone === 'available'
                ? <DownloadCloud className="mt-0.5 h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                : status.tone === 'unread'
                    ? <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
                    : <Info className="mt-0.5 h-5 w-5 shrink-0 text-fg-muted" aria-hidden="true" />;
    const expectedRestart = stored?.expectedRestartSeconds ?? 90;
    return (
        <section className="rounded-xl border border-border bg-surface p-5" aria-labelledby="panel-update-title">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="flex min-w-0 gap-3">
                    <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                        <DownloadCloud className="h-5 w-5" aria-hidden="true" />
                    </span>
                    <div>
                        <h3 id="panel-update-title" className="font-semibold text-fg">{t('panelUpdate.title')}</h3>
                        <p className="mt-1 text-sm text-fg-muted">{t(activation ? 'license.updateHelp' : 'panelUpdate.description')}</p>
                    </div>
                </div>
                {/* Outlined: the one filled button of this card is the update
                    itself (DESIGN.md, one primary per view). */}
                <Button type="button" variant="secondary" className="shrink-0 max-sm:min-h-11" onClick={() => void checkForUpdate()} disabled={checking || active || starting || readinessChecking}>
                    {checking ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                    {checking ? t('panelUpdate.checking') : t('panelUpdate.check')}
                </Button>
            </div>

            {!active && !tracking && !updated && status && (
                <div className="mt-4 flex items-start gap-2 text-sm text-fg" role="status" aria-live="polite" data-update-status={status.tone}>
                    {statusIcon}
                    <div className="min-w-0">
                        <p className="font-medium">{status.text}</p>
                        {status.detail && <p className="mt-1 text-fg-muted" data-update-status-detail="">{status.detail}</p>}
                        {showNext && <p className="mt-1 text-fg-muted" data-update-next="">{nextLine}</p>}
                    </div>
                </div>
            )}
            {!active && !tracking && !updated && !status && showNext && (
                <p className="mt-4 text-sm text-fg-muted" data-update-next="">{nextLine}</p>
            )}

            {currentVersion && (
                <p className="mt-3 text-sm text-fg-muted" data-update-installed="">
                    {t('panelUpdate.installed')}{' '}
                    <span className="font-mono text-fg">{currentVersion}</span>
                    {currentCommit && <>{' · '}<span className="font-mono" title={t('common.fullCommit', { commit: currentCommit })}><BuildCommit commit={currentCommit} /></span></>}
                    {verifying && tracking?.version === currentVersion && <span className="ml-2 font-sans text-xs font-medium text-fg-muted" data-update-state="verifying">({t('panelUpdate.card.installedVerifying')})</span>}
                </p>
            )}

            {steps && (tracking || updated) && (
                <div className="mt-4 rounded-lg border border-border bg-surface-subtle p-4 text-sm text-fg" data-update-steps-card="">
                    <p className="font-semibold text-fg">
                        {updated
                            ? t('panelUpdate.steps.updated', { version: updated.version })
                            : t('panelUpdate.steps.title', { version: tracking?.version ?? '' })}
                    </p>
                    {updated && (
                        <p className="mt-1 font-mono text-xs text-fg-subtle" data-update-build=""
                            title={t('common.fullCommit', { commit: updated.commit })}>
                            <BuildCommit commit={updated.commit} />
                        </p>
                    )}
                    {updated?.repeated && (
                        <p className="mt-1 text-sm text-fg-muted" data-update-repeated="">{t('panelUpdate.steps.repeated')}</p>
                    )}
                    <div className="mt-4">
                        <SystemUpdateSteps view={steps} />
                    </div>
                    <p className="sr-only" role="status" aria-live="polite">
                        {systemUpdateStepsAnnouncement(steps, t, updated?.version ?? tracking?.version ?? '')}
                    </p>
                    {tracking?.elapsed && tracking.requestID && tracking.phase !== 'unknown' && (
                        <p className="mt-4 flex flex-wrap gap-x-2 font-mono text-xs text-fg-subtle" data-update-elapsed="true">
                            <span>{`T+${tracking.elapsed}`}</span>
                            <span aria-hidden="true">·</span>
                            <span className="break-all">{`ID ${tracking.requestID}`}</span>
                        </p>
                    )}
                </div>
            )}

            {tracking && (!steps || tracking.phase === 'unknown') && (
                <div className="mt-4 flex items-start gap-2 rounded-lg border border-border bg-surface-subtle p-3 text-sm text-fg" role="status" aria-live="polite" data-update-phase={tracking.phase}>
                    {/* The update's own restart is progress, drawn as such;
                        the warning mark is for a state that is not known
                        (after the restart window, or a failed read). */}
                    {tracking.phase === 'unknown'
                        ? <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                        : <Loader2 className="mt-0.5 h-4 w-4 shrink-0 motion-safe:animate-spin text-primary" aria-hidden="true" />}
                    <div className="min-w-0">
                        <p>{t(tracking.phase === 'verifying' ? 'panelUpdate.card.verifying'
                            : tracking.phase === 'unknown' ? 'panelUpdate.card.unknown'
                                : tracking.phase === 'restarting' ? 'panelUpdate.card.restarting'
                                    : tracking.phase === 'reading' ? 'panelUpdate.card.reading' : 'panelUpdate.card.applying', { version: tracking.version })}</p>
                        {tracking.phase === 'unknown' && tracking.overdue && (
                            <p className="mt-1 text-fg-muted" data-update-overdue="true">{t('panelUpdate.tracking.restartOverdue')}</p>
                        )}
                        {tracking.elapsed && tracking.requestID && (
                            <p className="mt-1 flex flex-wrap gap-x-2 font-mono text-xs text-fg-subtle" data-update-elapsed="true">
                                <span>{`T+${tracking.elapsed}`}</span>
                                <span aria-hidden="true">·</span>
                                <span className="break-all">{`ID ${tracking.requestID}`}</span>
                            </p>
                        )}
                    </div>
                </div>
            )}

            {target && offer?.available && !active && (
                <div className="mt-4 space-y-4">
                    {previousAttempt && (
                        // At most one notice for an earlier attempt. One that
                        // stopped before changing anything is information, not
                        // a problem on this server (alpha.96); a rolled-back or
                        // failed attempt keeps the attention mark. Its words
                        // are kept.
                        // Hicbir seyi degistirmeden duran deneme bilgidir;
                        // geri alinan ya da basarisiz olan dikkat isaretini korur.
                        <div
                            className={`rounded-lg border p-3 text-sm text-fg ${previousStopped ? 'border-border bg-surface-subtle' : 'border-warning-mark/60 bg-warning-mark/20'}`}
                            role="note"
                            data-previous-attempt={previousStopped ? 'stopped' : previousAttempt.phase}
                        >
                            <div className="flex items-start gap-2">
                                {previousStopped
                                    ? <Info className="mt-0.5 h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                                    : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />}
                                <div>
                                    {/* The offered version already ended here
                                        (12 Oct 2026). Said before Start, in
                                        this order: that it was tried and what
                                        the server runs now, the cause the
                                        server recorded or that it recorded
                                        none, what starting it again does. The
                                        button below is never blocked by it. */}
                                    <p className="font-semibold text-fg">{t(previousAttempt.phase === 'failed' && previousStopped ? 'panelUpdate.previousAttempt.stoppedTitle'
                                        : previousAttempt.phase === 'recovered' ? 'panelUpdate.previousAttempt.rolledBackTitle'
                                            : 'panelUpdate.previousAttempt.title')}</p>
                                    <p className="mt-1 text-fg-muted">
                                        {t(previousAttempt.phase === 'recovered' ? 'panelUpdate.previousAttempt.recovered'
                                            : previousStopped ? 'panelUpdate.previousAttempt.stopped'
                                                : 'panelUpdate.previousAttempt.failed', {
                                            version: target.version,
                                            current: currentVersion ?? '',
                                            time: new Date(previousAttempt.finished_at).toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-US'),
                                        })}
                                    </p>
                                    {!previousStopped && (
                                        <p className="mt-1 text-fg-muted">
                                            {previousAttempt.failure_code
                                                ? t('panelUpdate.previousAttempt.cause', { cause: t(`recovery.reason.${previousAttempt.failure_code}`) })
                                                : t('panelUpdate.previousAttempt.noCause')}
                                        </p>
                                    )}
                                    {previousAttempt.phase === 'recovered' && (
                                        <p className="mt-1 text-fg-muted">{t('panelUpdate.previousAttempt.again', { version: target.version })}</p>
                                    )}
                                </div>
                            </div>
                        </div>
                    )}
                    {/* "Before you update", from what the panel knows: the
                        measured restart, what keeps running, the alpha note
                        as plain text, that nothing starts by itself. */}
                    <div data-update-before="">
                        <h4 className="text-sm font-semibold text-fg">{t('panelUpdate.before.title')}</h4>
                        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-fg-muted">
                            <li>{t('panelUpdate.before.restart', { seconds: expectedRestart })}</li>
                            <li>{t('panelUpdate.before.services')}</li>
                            <li data-update-alpha-note="">{t('panelUpdate.before.backup')}</li>
                            <li>{t('panelUpdate.before.manual')}</li>
                            <li>{t('panelUpdate.before.follow')}</li>
                        </ul>
                    </div>
                    <div
                        className={`rounded-lg border p-3 text-sm ${readiness?.ready === false && !readinessChecking
                            ? 'border-warning-mark/60 bg-warning-mark/20'
                            : 'border-border bg-surface-subtle'}`}
                        data-readiness-tone={readiness?.ready === true ? 'ready' : readiness?.ready === false && !readinessChecking ? 'warning' : 'neutral'}
                        role="status"
                        aria-live="polite"
                    >
                        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                            <div className="flex items-start gap-2">
                                {readinessChecking
                                    ? <Loader2 className="h-4 w-4 animate-spin text-primary" aria-hidden="true" />
                                    : readiness?.ready === false
                                        ? <AlertTriangle className="h-4 w-4 text-warning" aria-hidden="true" />
                                        : readiness?.ready === true
                                            ? <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                                            : null}
                                <div>
                                    <p className="font-semibold text-fg">{readinessTitle}</p>
                                    {!readinessChecking && readinessReason && <p className="mt-1 text-fg-muted">{readinessReason}</p>}
                                    {!readinessChecking && readiness?.ready === false && (
                                        <p className="mt-1 text-fg-muted" data-readiness-auto-resume="">{t('services.mutationReadiness.autoResume')}</p>
                                    )}
                                </div>
                            </div>
                            {readiness?.ready === false && !readinessChecking && (
                                <Button
                                    type="button"
                                    variant="secondary"
                                    onClick={() => void retryHostMutationReadiness()}
                                    disabled={starting || readinessChecking}
                                >
                                    <RefreshCw className="h-4 w-4" />
                                    {t('common.retry')}
                                </Button>
                            )}
                        </div>
                    </div>
                    <Button
                        id="panel-update-start-button"
                        type="button"
                        variant="primary"
                        className="max-sm:min-h-11"
                        onClick={() => void startUpdate()}
                        disabled={starting || readinessChecking || readiness?.ready !== true}
                    >
                        {starting ? <Loader2 className="h-4 w-4 animate-spin" /> : <DownloadCloud className="h-4 w-4" />}
                        {starting ? t('panelUpdate.starting') : t('panelUpdate.start', { version: target.version })}
                    </Button>
                </div>
            )}

            {message && (
                <div className="mt-4 flex items-start gap-2 rounded-lg border border-border p-3 text-sm text-fg" role="status" aria-live="polite" data-message-tone={messageTone}>
                    <AlertTriangle className="h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
                    <p>{message}</p>
                </div>
            )}

            {currentVersion && currentCommit && (
                <details className="mt-4 text-sm" data-update-technical="">
                    <summary className="cursor-pointer select-none font-medium text-fg-muted hover:text-fg max-sm:min-h-11 max-sm:py-2.5">{t('panelUpdate.technical')}</summary>
                    <dl className="mt-3 grid gap-3 rounded-lg border border-border bg-surface-subtle p-4 sm:grid-cols-2">
                        <div><dt className="text-fg-muted">{t('panelUpdate.currentVersion')}</dt><dd className="font-mono text-fg">{currentVersion}</dd></div>
                        <div><dt className="text-fg-muted">{t('panelUpdate.currentCommit')}</dt><dd className="break-all font-mono text-fg">{currentCommit}</dd></div>
                        {target && offer?.available && <>
                            <div><dt className="text-fg-muted">{t('panelUpdate.targetVersion')}</dt><dd className="font-mono font-semibold text-fg">{target.version}</dd></div>
                            <div><dt className="text-fg-muted">{t('panelUpdate.targetCommit')}</dt><dd className="break-all font-mono text-fg">{target.commit}</dd></div>
                            <div><dt className="text-fg-muted">{t('panelUpdate.targetPlatform')}</dt><dd className="font-mono text-fg">{target.os}/{target.arch}</dd></div>
                            <div><dt className="text-fg-muted">{t('panelUpdate.sequence')}</dt><dd className="font-mono text-fg">{target.sequence}</dd></div>
                            <div><dt className="text-fg-muted">{t('panelUpdate.archiveSize')}</dt><dd className="font-mono text-fg">{t('panelUpdate.bytes', { size: target.archive_size })}</dd></div>
                            <div className="sm:col-span-2"><dt className="text-fg-muted">{t('panelUpdate.sha256')}</dt><dd className="break-all font-mono text-xs text-fg">{target.archive_sha256}</dd></div>
                        </>}
                    </dl>
                </details>
            )}

            <div className="mt-5 border-t border-border pt-4" data-update-auto="">
                <label className="flex cursor-pointer items-start gap-3">
                    <input
                        type="checkbox"
                        role="switch"
                        className="mt-1 h-4 w-4 shrink-0 accent-primary"
                        checked={stored?.autoCheck ?? true}
                        disabled={!stored || savingAuto}
                        aria-checked={stored?.autoCheck ?? true}
                        onChange={(event) => void saveAutoCheck(event.currentTarget.checked)}
                    />
                    <span className="min-w-0">
                        <span className="block text-sm font-medium text-fg">{t('panelUpdate.auto.label')}</span>
                        <span className="mt-0.5 block text-sm text-fg-muted">{t('panelUpdate.auto.help')}</span>
                    </span>
                </label>
                {savingAuto && <p className="mt-2 flex items-center gap-2 text-sm text-fg-muted" role="status"><Loader2 className="h-4 w-4 motion-safe:animate-spin" aria-hidden="true" />{t('panelUpdate.auto.saving')}</p>}
                {autoError && !savingAuto && <p className="mt-2 text-sm text-fg" role="alert" data-update-auto-error="">{t('panelUpdate.auto.failed')}</p>}
            </div>
        </section>
    );
}
