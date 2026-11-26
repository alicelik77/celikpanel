import { isFinishingStep, isPreflightStop, preflightStopCodes, recoveryFailureCodes, recoveryFailureGuidanceKey, retryingCauseKey, type PausedCauseCode, type RecoveryFailureCode, type RecoveryObservation } from './recoveryObservation';
import { systemUpdatePackageManagerBusy, systemUpdatePreflightStop, systemUpdateRecoveryPreparationUnconfirmed, systemUpdateStoppedUnchangedSummary, withoutInternalTokens } from './systemUpdateFailure';

/** Preflight steps with reviewed owner wording; any other step uses the generic line. */
export const preflightSteps = ['mode', 'owner', 'compatibility', 'panel_database_check', 'agent_ledger_check', 'owner_metadata_probe', 'runtime_selection',
    'runtime_revalidation', 'release_boundary', 'material_support', 'database_support', 'database_metadata',
    'idle_probe', 'agent_idle', 'bind_compatibility', 'application_compatibility', 'bootstrap_state'] as const;
type PreflightStep = typeof preflightSteps[number];
/** Reason classes of a refused update check whose own wording replaces the server's line. */
export const preflightReasonClasses = ['concurrent_write', 'operation_active'] as const;
type PreflightReasonClass = typeof preflightReasonClasses[number];

/**
 * Owner guidance for a finished-with-failure update notice. The primary text
 * comes from typed data (the exact request's recovery observation and its
 * optional typed cause), never from the worker's raw summary. The summary stays
 * available as a secondary "server reported" line only.
 *
 * Başarısız biten güncelleme bildiriminin yönlendirmesi tipli veriden gelir;
 * sunucunun ham özeti yalnız ikincil satırdır.
 */
export type OutcomeKey =
    | 'panelUpdate.failed'
    | 'panelUpdate.outcome.rolledBackTitle'
    | 'panelUpdate.outcome.rolledBack'
    | 'panelUpdate.outcome.cause.candidate_panel_startup_check_failed'
    | 'panelUpdate.outcome.cause.panel_start_unverified'
    | 'panelUpdate.outcome.cause.update_finishing_scheduler'
    | 'panelUpdate.finishing.title'
    | 'panelUpdate.finishing.body'
    | 'panelUpdate.outcome.cause.recovery_runtime_preflight_failed'
    | 'panelUpdate.outcome.stoppedTitle'
    | 'panelUpdate.outcome.stopped'
    | `panelUpdate.outcome.preflightStep.${PreflightStep}`
    | 'panelUpdate.outcome.preflightStep.generic'
    | `panelUpdate.outcome.preflightClass.${PreflightReasonClass}`
    | 'panelUpdate.outcome.serverMessage'
    | 'panelUpdate.outcome.packageManagerBusy'
    | 'panelUpdate.packageManagerBusy'
    | 'panelUpdate.outcome.packageManagerBusyNext'
    | 'panelUpdate.outcome.preparationUnconfirmedStopped'
    | 'panelUpdate.outcome.preparationUnconfirmed'
    | 'panelUpdate.outcome.preparationUnconfirmedNext'
    | 'panelUpdate.outcome.preparationUnconfirmedCommandNote'
    | 'panelUpdate.outcome.preparationUnconfirmedAnswers'
    | 'panelUpdate.outcome.stoppedNext'
    | 'panelUpdate.outcome.stoppedResume'
    | 'panelUpdate.outcome.preflightNext.application_compatibility'
    | 'panelUpdate.outcome.preflightResume.application_compatibility'
    | 'panelUpdate.outcome.cause.generic'
    | 'panelUpdate.outcome.rolledBackNext'
    | 'panelUpdate.outcome.rolledBackResume'
    | 'panelUpdate.outcome.followsRecovery'
    | 'panelUpdate.outcome.unknownResult'
    | 'panelUpdate.outcome.checking'
    | 'panelUpdate.outcome.checkingTitle'
    | `recovery.phase.${NonNullable<RecoveryObservation['phase']>}`
    | `recovery.next.${NonNullable<RecoveryObservation['phase']>}`
    | `recovery.wait.${NonNullable<RecoveryObservation['waiting_for']>}`
    | 'recovery.wait.next'
    | 'recovery.automatic.pausedTitle'
    | `recovery.automatic.cause.${PausedCauseCode}`
    | 'recovery.automatic.pausedHelp'
    | 'recovery.automatic.renewal'
    | 'recovery.automatic.renewalOff'
    | 'recovery.automatic.pausingTitle'
    | 'recovery.automatic.pausingHelp'
    | 'recovery.automatic.retryTitle'
    | 'recovery.automatic.retryHelp'
    | 'recovery.automatic.inspect'
    | 'recovery.automatic.resume'
    | NonNullable<ReturnType<typeof recoveryFailureGuidanceKey>>;

/**
 * fallback: a boot-catalogue key shown while the screen catalogue that holds
 * `key` has not arrived (the notice is mounted above the routes).
 */
export type OutcomeText = { key: OutcomeKey; vars?: Record<string, string>; fallback?: OutcomeKey } | { text: string };

/**
 * How the outcome is drawn (owner direction 2026-10-16, D-024/D-033): nothing
 * alarming for what the server is handling by itself. progress: recovery owns
 * the request (or the first read is still due); success: verified update;
 * warning: verified return to the previous version; neutral: stopped before
 * changing anything; danger: the owner must act (recovery_required, a paused
 * recovery) or the outcome stayed unreadable past its limit.
 */
export type SystemUpdateOutcomeTone = 'progress' | 'success' | 'warning' | 'neutral' | 'danger';

export type FailedUpdateGuidance = {
    /**
     * rolled_back: verified; unchanged: stopped in the read-only preflight, final;
     * recovery: the server's recovery is the source; unknown: no usable observation yet.
     */
    state: 'rolled_back' | 'unchanged' | 'recovery' | 'succeeded' | 'unknown';
    tone: SystemUpdateOutcomeTone;
    /**
     * The new version is installed and the server repeats the update's last
     * step by itself (Frankfurt 2026-10-16): the page draws the steps with the
     * confirmation retrying, and the attempt when it is known.
     */
    finishing?: { attempt: { current: number; max: number } | null };
    title: OutcomeText;
    lines: OutcomeText[];
    /** A fixed owner command to show as code, never server-supplied text. */
    command?: string;
    /** One sentence under the command (what it does), then what each answer means. */
    commandNote?: OutcomeText;
    afterCommand?: OutcomeText[];
    /**
     * The worker's summary without internal tokens, shown only as a secondary
     * line and only when no translated product summary says the same.
     */
    serverMessage?: string;
};

export const RECOVERY_LOG_COMMAND = 'sudo journalctl -u celikpanel-release-recovery.service --no-pager -n 50';
/**
 * Read-only: says whether a recovery kit transition is left and what to do,
 * in the page's language (the tool answers in English without --lang; it has
 * had --lang since alpha.81). Sayfanın dilinde yanıt verir.
 */
export function recoveryRuntimeStatusCommand(locale: 'en' | 'tr' = 'en'): string {
    return `sudo /usr/libexec/celikpanel/recovery runtime-status --lang ${locale}`;
}

type Input = {
    targetVersion: string;
    previousVersion: string;
    /** The stored terminal message: either the server's summary or product text. */
    message: string;
    /** Product texts this browser may have stored instead of a server summary. */
    productMessages: readonly string[];
    /** Reviewed typed guidance derived from a summary (for example package_manager_busy). */
    typedMessage?: string;
    /** True until the first observation read for this request has settled. */
    reading: boolean;
    /**
     * True while an unreadable outcome is still inside its limit (the page's
     * own clock since the failure was recorded); afterwards it is said as a
     * failure the owner looks at.
     */
    withinLimit?: boolean;
    /** The page's language, for an owner command that answers in it. */
    locale?: 'en' | 'tr';
};

/** How long an unreadable outcome is still said as "being checked". */
export const SYSTEM_UPDATE_UNKNOWN_OUTCOME_LIMIT_MS = 2 * 60 * 1000;

/**
 * Input.withinLimit for a failed request: true while this browser's own record
 * of the failure (completedAt, kept in its stored record, so a reload or a
 * second tab does not restart it) is younger than the limit. A record without
 * that time is past it.
 */
export function systemUpdateWithinLimit(completedAt: number | undefined, now: number): boolean {
    return completedAt !== undefined && now - completedAt < SYSTEM_UPDATE_UNKNOWN_OUTCOME_LIMIT_MS;
}

/**
 * A failure notice the owner reads and closes (red, amber or neutral); a
 * progress notice is the server's to finish and has no close button.
 */
export function systemUpdateFailureClosable(tone: SystemUpdateOutcomeTone | null): boolean {
    return tone === 'danger' || tone === 'warning' || tone === 'neutral';
}
/** The native recovery timer admits at most three automatic attempts. */
const RECOVERY_MAX_ATTEMPTS = 3;

// A refused update check never rolls back or pauses, so it has no cause text
// of its own there; the generic cause is shown if a record ever says so.
function causeKey(code: RecoveryFailureCode | undefined): OutcomeKey {
    return code && code !== 'update_preflight_refused' && (recoveryFailureCodes as readonly string[]).includes(code)
        ? `panelUpdate.outcome.cause.${code as PausedCauseCode}`
        : 'panelUpdate.outcome.cause.generic';
}

// The refused-check steps and the reason classes live in the server screen
// catalogue (the boot catalogue stays small); until it has arrived the notice
// shows the boot catalogue's generic reason.
function preflightReasonLine(preflight: { step: string; reasonClass?: string } | undefined): { key: OutcomeKey; fallback?: OutcomeKey } {
    const generic: OutcomeKey = 'panelUpdate.outcome.preflightStep.generic';
    if (preflight?.reasonClass && (preflightReasonClasses as readonly string[]).includes(preflight.reasonClass)) {
        return { key: `panelUpdate.outcome.preflightClass.${preflight.reasonClass as PreflightReasonClass}`, fallback: generic };
    }
    return preflight?.step && (preflightSteps as readonly string[]).includes(preflight.step)
        ? { key: `panelUpdate.outcome.preflightStep.${preflight.step as PreflightStep}`, fallback: generic }
        : { key: generic };
}

// The updater's own reason from its reviewed summary line, or '' for none or
// for its historical generic text (that one names no cause).
const GENERIC_UPDATER_REASON = 'updater command failed; inspect the update worker log';
function updaterReason(message: string): string {
    const match = /(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=[a-z_]{1,64} state=[a-z_]{1,40} reason=(.*?) detail=/.exec(message);
    const reason = match ? match[1].trim() : '';
    return reason === GENERIC_UPDATER_REASON || !/[A-Za-z0-9]/.test(reason) ? '' : reason;
}

export function failedUpdateGuidance(observation: RecoveryObservation | null, input: Input): FailedUpdateGuidance {
    const product = input.message !== '' && input.productMessages.includes(input.message);
    // upd4 O7: never the raw line; omitted when a reviewed translation says it.
    const serverMessage = !product && !input.typedMessage && input.message !== '' ? withoutInternalTokens(input.message) || undefined : undefined;
    const versions = { target: input.targetVersion, previous: input.previousVersion };
    // A preflight stop changed nothing and nothing follows. The typed record is
    // the source; the updater's own unchanged summary counts only while no
    // other recorded outcome contradicts it.
    const preflight = systemUpdatePreflightStop(input.message);
    const uncontradicted = !observation || observation.observation !== 'known'
        || (observation.phase === 'failed' && observation.terminal_proof === 'none' && !observation.failure_code);
    // The package manager was busy (Frankfurt, 2026-10-17: Debian's automatic
    // package updates were running). A transient, typed stop before anything
    // changed: its own words, no server line, nothing to repair. The updater
    // records such a stop that its recovery tool's check reported as a
    // recovery runtime preflight stop (update.sh publishes that code beside
    // the package_manager_busy summary), so a typed preflight stop is the same
    // request ending unchanged, not a contradiction.
    // Paket yöneticisi meşguldü: geçici, tipli bir duruş; kendi sözleri.
    if (systemUpdatePackageManagerBusy(input.message) && (uncontradicted || isPreflightStop(observation))) {
        return {
            state: 'unchanged',
            tone: 'neutral',
            title: { key: 'panelUpdate.outcome.stoppedTitle' },
            lines: [
                { key: 'panelUpdate.outcome.stopped', vars: versions },
                // Screen copy (settings part); until it arrives, the boot
                // catalogue's own busy and resume sentences.
                { key: 'panelUpdate.outcome.packageManagerBusy', fallback: 'panelUpdate.packageManagerBusy' },
                { key: 'panelUpdate.outcome.packageManagerBusyNext', vars: versions, fallback: 'panelUpdate.outcome.stoppedResume' },
            ],
        };
    }
    // The recovery tool could not confirm its own preparation: the update
    // stopped before any release transaction or service stop, so no recovery
    // follows it (alpha.137 lab: the page waited for one forever and offered
    // no update). A stop the owner can close and start again, with the one
    // read-only command that says whether anything of the preparation is left.
    // Hazırlık doğrulanamadı: kurtarma gelmez; kapatılabilir duruş ve komut.
    if (systemUpdateRecoveryPreparationUnconfirmed(input.message) && uncontradicted) {
        return {
            state: 'unchanged',
            tone: 'neutral',
            title: { key: 'panelUpdate.outcome.stoppedTitle' },
            // D-024: what happened and what is known, who acts and the
            // next action, the command, then how work resumes for each of
            // its answers (the recovery tool's own runtime-status wording).
            // Its own opening: true also when the kit's preparation had
            // written its intent (a137 lab 2), unlike "in its read-only check".
            lines: [
                { key: 'panelUpdate.outcome.preparationUnconfirmedStopped', vars: versions, fallback: 'panelUpdate.outcome.stoppedTitle' },
                { key: 'panelUpdate.outcome.preparationUnconfirmed', fallback: 'panelUpdate.outcome.preflightStep.generic' },
                { key: 'panelUpdate.outcome.preparationUnconfirmedNext', vars: versions, fallback: 'panelUpdate.outcome.stoppedResume' },
            ],
            command: recoveryRuntimeStatusCommand(input.locale),
            commandNote: { key: 'panelUpdate.outcome.preparationUnconfirmedCommandNote', fallback: 'panelUpdate.outcome.stoppedResume' },
            afterCommand: [
                { key: 'panelUpdate.outcome.preparationUnconfirmedAnswers', vars: versions, fallback: 'panelUpdate.outcome.stoppedResume' },
            ],
        };
    }
    if (isPreflightStop(observation) || (preflight && uncontradicted)) {
        const reported = withoutInternalTokens(preflight ? preflight.diagnostic : input.message);
        // A translated reason class says what happened and what to do: the
        // server's line is then not shown (upd4 O7).
        const translated = !!preflight?.reasonClass && (preflightReasonClasses as readonly string[]).includes(preflight.reasonClass);
        const reason = preflightReasonLine(preflight);
        // A step this page has no words for gets only the generic reason, so
        // the server's own first line follows it in view rather than inside
        // the closed Details (alpha.96: Frankfurt's update to alpha.95 stopped
        // here and the cause was hidden). It is already sanitized and short.
        // Sözcüğü olmayan adımda sunucunun ilk satırı nedenin altında görünür.
        // The recovery tool that could not be started or verified
        // ('compatibility', alpha.97) is a broad reason: its line follows too.
        const generic = (reason.key === 'panelUpdate.outcome.preflightStep.generic'
            || reason.key === 'panelUpdate.outcome.preflightStep.compatibility') && !translated;
        // The first line, cut at 240 characters with an ellipsis; when it is not
        // the whole message, the whole message stays behind Details too.
        const lineOne = !product && reported ? reported.split(/\r?\n/)[0] : '';
        const firstLine = lineOne.length > 240 ? `${lineOne.slice(0, 239).trimEnd()}…` : lineOne;
        const inline = generic && firstLine !== '';
        const complete = inline && firstLine === reported;
        // The new version refused this server's recorded mail or DNS state
        // (alpha.114 E7: every PowerDNS main server on alpha.113). Nothing on
        // the server fixes that and the same version stops the same way: say
        // so, and that a corrected version is the next step.
        // Yeni sürüm kayıtlı posta/DNS durumunu kabul etmedi: sunucuda
        // düzeltilecek bir şey yok, aynı sürüm aynı yerde durur.
        const refusedState = !translated && preflight?.step === 'application_compatibility';
        const next: OutcomeText[] = refusedState
            ? [{ key: 'panelUpdate.outcome.preflightNext.application_compatibility', vars: versions },
                { key: 'panelUpdate.outcome.preflightResume.application_compatibility', vars: versions }]
            : [{ key: 'panelUpdate.outcome.stoppedNext' }, { key: 'panelUpdate.outcome.stoppedResume', vars: versions }];
        return {
            state: 'unchanged',
            tone: 'neutral',
            title: { key: 'panelUpdate.outcome.stoppedTitle' },
            lines: [
                { key: 'panelUpdate.outcome.stopped', vars: versions },
                reason,
                ...(inline ? [{ key: 'panelUpdate.outcome.serverMessage' as const, vars: { message: firstLine } }] : []),
                ...next,
            ],
            serverMessage: !product && !translated && !complete && reported ? reported : undefined,
        };
    }
    if (observation?.observation === 'known' && observation.phase === 'recovered'
        && observation.terminal_proof === 'rollback_verified') {
        // The secondary line keeps the server's words without internal tokens.
        // Without a typed cause, the updater's own reason (for example the
        // installer's line naming a panel.env key, alpha.114 lab E-new-5) is
        // the cause line itself, not hidden behind Details.
        const typedCause = causeKey(observation.previous_failure === 'update_failed' ? observation.failure_code : undefined);
        const reported = typedCause === 'panelUpdate.outcome.cause.generic' && observation.previous_failure === 'update_failed'
            ? updaterReason(input.message) : '';
        return {
            state: 'rolled_back',
            tone: 'warning',
            title: { key: 'panelUpdate.outcome.rolledBackTitle' },
            lines: [
                { key: 'panelUpdate.outcome.rolledBack', vars: versions },
                reported ? { key: 'panelUpdate.outcome.serverMessage', vars: { message: reported } } : { key: typedCause },
                { key: 'panelUpdate.outcome.rolledBackNext', vars: versions },
                { key: 'panelUpdate.outcome.rolledBackResume', vars: versions },
            ],
            serverMessage,
        };
    }
    if (isFinishingStep(observation)) {
        // Installed and running; only the last step is being repeated. The
        // server's line stays behind the page's "Details" disclosure.
        const first = observation!.phase === 'recovering' && observation!.previous_failure === 'update_failed';
        return {
            state: 'recovery',
            tone: 'progress',
            title: { key: 'panelUpdate.finishing.title' },
            lines: [{ key: 'panelUpdate.finishing.body', vars: { version: input.targetVersion } }],
            finishing: { attempt: first ? { current: 1, max: RECOVERY_MAX_ATTEMPTS } : null },
            serverMessage,
        };
    }
    if (observation?.observation === 'known' && observation.phase) {
        const phase = observation.phase;
        const succeeded = phase === 'succeeded' && observation.terminal_proof === 'update_verified';
        const lines: OutcomeText[] = [];
        let title: OutcomeText;
        let command: string | undefined;
        const pausing = observation.automatic_recovery === 'pause_pending';
        if (observation.automatic_recovery === 'retry_scheduled' || pausing) {
            // Attempts remain, or the last one is finishing: nothing is asked of
            // the owner before the pause (with its retry command) is recorded.
            title = { key: pausing ? 'recovery.automatic.pausingTitle' : 'recovery.automatic.retryTitle' };
            const cause = retryingCauseKey(observation.first_failure_code);
            if (cause) lines.push({ key: cause });
            lines.push({ key: pausing ? 'recovery.automatic.pausingHelp' : 'recovery.automatic.retryHelp' });
        } else if (observation.automatic_recovery) {
            title = { key: 'recovery.automatic.pausedTitle' };
            if (observation.first_failure_code) lines.push({ key: `recovery.automatic.cause.${observation.first_failure_code}` });
            lines.push({ key: 'recovery.automatic.pausedHelp' },
                { key: observation.renewal_before_update === 'off' ? 'recovery.automatic.renewalOff' : 'recovery.automatic.renewal' },
                { key: 'recovery.automatic.inspect' });
            command = RECOVERY_LOG_COMMAND;
            lines.push({ key: 'recovery.automatic.resume' });
        } else if (observation.waiting_for) {
            title = { key: `recovery.wait.${observation.waiting_for}` };
            lines.push({ key: 'recovery.wait.next' });
        } else {
            title = { key: `recovery.phase.${phase}` };
            lines.push({ key: recoveryFailureGuidanceKey(observation) ?? `recovery.next.${phase}` });
        }
        // upd4 O9: a verified update shows no failure, and the server's
        // failure line of the earlier attempt is not shown.
        if (!succeeded) lines.push({ key: 'panelUpdate.outcome.followsRecovery' });
        // alpha.149 lab O3: the update failed with no typed cause and the
        // server's recovery has not taken it over (an update refused before it
        // started, observed as phase=failed terminal_proof=none). The native
        // recovery timer runs every 30 seconds: past the page's limit no
        // recovery follows, and the owner's next step (recovery.next.failed)
        // is theirs. Said as such, the notice can be closed and updates checked
        // again; closing starts nothing and the server still refuses a second
        // update while a transaction is open.
        // Nedeni yazılmamış ve kurtarmanın üstlenmediği başarısız güncelleme:
        // süre dolunca bildirim kapatılabilir; kapatmak hiçbir şey başlatmaz.
        const unattendedFailure = phase === 'failed' && observation.terminal_proof === 'none'
            && !observation.automatic_recovery && !observation.waiting_for
            && recoveryFailureGuidanceKey(observation) === undefined
            && !input.reading && input.withinLimit !== true;
        // Red only when the owner must act: a paused recovery, a recovery
        // that could not finish, or a failure no recovery took over. Everything
        // the server still handles is progress.
        const tone: SystemUpdateOutcomeTone = succeeded ? 'success'
            : observation.automatic_recovery === 'paused_retry_limit'
                || (phase === 'recovery_required' && !observation.automatic_recovery)
                || unattendedFailure ? 'danger'
                : phase === 'recovered' ? 'warning' : 'progress';
        return { state: succeeded ? 'succeeded' : 'recovery', tone, title, lines, command, serverMessage: succeeded ? undefined : serverMessage };
    }
    const lines: OutcomeText[] = [];
    if (input.typedMessage) lines.push({ text: input.typedMessage });
    else if (product) lines.push({ text: input.message });
    else lines.push({ key: input.reading || input.withinLimit === true ? 'panelUpdate.outcome.checking' : 'panelUpdate.outcome.unknownResult' });
    // The first read, and an unreadable outcome inside its limit, are still
    // being checked; past the limit the owner is told it is not known.
    // A reviewed summary (typed or product text) keeps its failure wording.
    const checking = !input.typedMessage && !product && (input.reading || input.withinLimit === true);
    return {
        state: 'unknown',
        tone: checking ? 'progress' : 'danger',
        title: { key: checking ? 'panelUpdate.outcome.checkingTitle' : 'panelUpdate.failed' },
        lines,
        serverMessage,
    };
}

/** The update check's optional previous_attempt, for the exact offered target. */
export type PreviousUpdateAttempt = {
    request_id: string;
    phase: 'failed' | 'recovered';
    failure_code?: RecoveryFailureCode;
    finished_at: string;
};

/**
 * An earlier attempt's own stored summary says the package manager was busy
 * (alpha.97): the card then names that cause, calmly, for an attempt the
 * recovery record kept without one.
 */
export function previousAttemptPackageManagerBusy(summary: string): boolean {
    return systemUpdatePackageManagerBusy(summary);
}

/**
 * The attempt's own stored summary says it stopped before changing the
 * installed version: a typed unchanged stop, or the recovery tool's
 * unconfirmed preparation (alpha.137 review: the card said "did not
 * complete" for both once the window was closed).
 */
export function previousAttemptSummaryStopped(summary: string): boolean {
    return systemUpdateStoppedUnchangedSummary(summary) || systemUpdateRecoveryPreparationUnconfirmed(summary);
}

/** The attempt's own summary says the recovery tool could not confirm its preparation. */
export function previousAttemptPreparationUnconfirmed(summary: string): boolean {
    return systemUpdateRecoveryPreparationUnconfirmed(summary);
}

/** The attempt stopped before changing anything installed (either typed preflight stop). */
export function previousAttemptStopped(attempt: PreviousUpdateAttempt | undefined): boolean {
    return !!attempt && (preflightStopCodes as readonly string[]).includes(attempt.failure_code ?? '');
}

/**
 * Accepts only the closed shape the Panel sends. Anything else is ignored, as an
 * older Panel's absent field is: the notice is guidance, never a start gate.
 */
export function decodePreviousUpdateAttempt(value: unknown): PreviousUpdateAttempt | undefined {
    if (!value || typeof value !== 'object') return undefined;
    const attempt = value as Record<string, unknown>;
    if (typeof attempt.request_id !== 'string' || !/^[a-f0-9]{32}$/.test(attempt.request_id)
        || (attempt.phase !== 'failed' && attempt.phase !== 'recovered')
        || typeof attempt.finished_at !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(attempt.finished_at)
        || !Number.isFinite(Date.parse(attempt.finished_at))) return undefined;
    const code = (recoveryFailureCodes as readonly unknown[]).includes(attempt.failure_code)
        ? attempt.failure_code as RecoveryFailureCode
        : undefined;
    return {
        request_id: attempt.request_id,
        phase: attempt.phase,
        ...(code ? { failure_code: code } : {}),
        finished_at: attempt.finished_at,
    };
}
