// The update's steps, as the update page draws them: one bar per step (owner
// design 2026-10-16, after updating the second installed server from alpha.85
// to alpha.87 and seeing a warning and a corner notice while nothing was
// wrong). The server declares the steps and reports each one's state with its
// start and status answers (cmd/panel system_update_plan.go); this module
// decodes that plan and decides what is drawn now.
//
// What the page adds is only what it alone can see: when it loses the Panel
// after the server said the update is active, the update has reached its
// restart (the updater stops the Panel when it begins installing; installing
// the files and restarting cannot be told apart from outside, so they are one
// step). That step's bar is then time-based, against the length the server
// expects from its last measured restart, and it never reaches the end by
// itself. After the restart window (lib/systemUpdateTracking.ts, 6 minutes from
// the start) the motion stops and the overdue wording appears; never before.
// Every moment here is this browser's clock; the server's elapsed seconds are
// measured on its own clock (D-032).
//
// Guncellemenin adimlari: sunucu plani bildirir; sayfa yalniz kendi gorebildigini
// ekler: Panel'e ulasamadiginda guncelleme yeniden baslama adimindadir. Bu
// adimin cubugu beklenen sureye gore ilerler ve kendiliginden sona ulasmaz.

import type { SystemUpdateTrackingPhase } from './systemUpdateTracking';

export const SYSTEM_UPDATE_STEP_IDS = ['download', 'verify', 'prepare', 'restart', 'confirm'] as const;
export type SystemUpdateStepID = typeof SYSTEM_UPDATE_STEP_IDS[number];
export type SystemUpdateStepState = 'pending' | 'running' | 'done' | 'failed';

export type SystemUpdatePlanStep = {
    id: SystemUpdateStepID;
    state: SystemUpdateStepState;
    // 0..1 when the server measured it (the download's bytes); absent otherwise.
    progress?: number;
};

export type SystemUpdatePlan = {
    steps: SystemUpdatePlanStep[];
    expectedRestartSeconds: number;
};

// Without a measured restart on this server (cmd/panel: 90 s, the floor of the
// expected length; the eighteenth native record measured the Panel unanswered
// for at most 32 s).
export const SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS = 90;
const SYSTEM_UPDATE_RESTART_BOUND_SECONDS = 360;

// The plan of a start or status answer, or null when it is absent or not
// exactly the five known steps in order (an older or a garbled answer keeps
// the page's previous wording).
export function decodeSystemUpdatePlan(value: unknown): SystemUpdatePlan | null {
    if (!value || typeof value !== 'object') return null;
    const raw = value as Record<string, unknown>;
    if (!Array.isArray(raw.steps) || raw.steps.length !== SYSTEM_UPDATE_STEP_IDS.length) return null;
    const steps: SystemUpdatePlanStep[] = [];
    for (let index = 0; index < SYSTEM_UPDATE_STEP_IDS.length; index += 1) {
        const step = raw.steps[index] as Record<string, unknown> | null;
        if (!step || typeof step !== 'object' || step.id !== SYSTEM_UPDATE_STEP_IDS[index]) return null;
        if (step.state !== 'pending' && step.state !== 'running' && step.state !== 'done' && step.state !== 'failed') return null;
        const decoded: SystemUpdatePlanStep = { id: SYSTEM_UPDATE_STEP_IDS[index], state: step.state };
        if (typeof step.progress === 'number' && Number.isFinite(step.progress) && step.progress >= 0 && step.progress <= 1) {
            decoded.progress = step.progress;
        }
        steps.push(decoded);
    }
    const expected = raw.expected_restart_seconds;
    const expectedRestartSeconds = typeof expected === 'number' && Number.isInteger(expected)
        && expected >= 1 && expected <= SYSTEM_UPDATE_RESTART_BOUND_SECONDS
        ? expected : SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS;
    return { steps, expectedRestartSeconds };
}

// The restart bar's fill after elapsedMs against an expected length: linear to
// 90 % at the expected length, then slower and slower towards 99 %. It never
// reaches the end by itself; only the reconnection finishes the step.
export function systemUpdateRestartProgress(elapsedMs: number, expectedSeconds: number): number {
    const expected = Number.isFinite(expectedSeconds) && expectedSeconds > 0
        ? expectedSeconds * 1000 : SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS * 1000;
    const elapsed = Number.isFinite(elapsedMs) ? Math.max(0, elapsedMs) : 0;
    if (elapsed <= expected) return 0.9 * (elapsed / expected);
    return Math.min(0.99, 0.9 + 0.09 * (1 - Math.exp(-(elapsed - expected) / expected)));
}

export type SystemUpdateStepView = {
    id: SystemUpdateStepID;
    state: SystemUpdateStepState;
    // For the running step: 0..1, or null when the bar is indeterminate.
    progress: number | null;
    // The running restart step that the page measures by time; it carries
    // the step's one sentence.
    restarting: boolean;
    // True when the bar must stop moving (past the restart window).
    still: boolean;
};

export type SystemUpdateStepsView = {
    steps: SystemUpdateStepView[];
    // Every step done: the update succeeded.
    completed: boolean;
};

export type SystemUpdateStepsInput = {
    plan: SystemUpdatePlan | null;
    // What the last read established (lib/systemUpdateTracking.ts); 'restarting'
    // only inside the restart window.
    phase: SystemUpdateTrackingPhase;
    // True when the restart outlasted its window (the overdue wording is shown).
    overdue: boolean;
    // The first lost read of the current outage, this browser's clock.
    lostSince: number | null;
    now: number;
    // The update's outcome when it succeeded (then every step is done).
    succeeded: boolean;
};

function stepsFrom(states: SystemUpdateStepState[], plan: SystemUpdatePlan | null): SystemUpdateStepView[] {
    return SYSTEM_UPDATE_STEP_IDS.map((id, index) => {
        const progress = plan?.steps[index]?.progress;
        return {
            id,
            state: states[index],
            progress: states[index] === 'running' && typeof progress === 'number' ? progress : null,
            restarting: false,
            still: false,
        };
    });
}

// What is drawn now, or null when the steps say nothing true (no plan yet and
// nothing the page saw itself, or a failure, which keeps its own wording).
export function systemUpdateStepsView(input: SystemUpdateStepsInput): SystemUpdateStepsView | null {
    const { plan } = input;
    if (input.succeeded) {
        return { steps: stepsFrom(SYSTEM_UPDATE_STEP_IDS.map(() => 'done'), plan), completed: true };
    }
    if (plan?.steps.some((step) => step.state === 'failed')) return null;
    const restartIndex = SYSTEM_UPDATE_STEP_IDS.indexOf('restart');
    if (input.phase === 'restarting' || input.overdue) {
        // The Panel cannot be reached after the server said the update is
        // active: the restart, unless the server already said the restart is
        // done (a later step keeps its own state).
        const planned = plan ? plan.steps.map((step) => step.state) : null;
        const restartDone = planned?.[restartIndex] === 'done';
        if (restartDone && planned) {
            return { steps: stepsFrom(planned, plan), completed: false };
        }
        const states = SYSTEM_UPDATE_STEP_IDS.map((_, index): SystemUpdateStepState =>
            index < restartIndex ? 'done' : index === restartIndex ? 'running' : 'pending');
        const steps = stepsFrom(states, null);
        const expected = plan?.expectedRestartSeconds ?? SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS;
        const since = input.lostSince ?? input.now;
        steps[restartIndex] = {
            ...steps[restartIndex],
            progress: systemUpdateRestartProgress(input.now - since, expected),
            restarting: true,
            still: input.overdue,
        };
        return { steps, completed: false };
    }
    if (input.phase === 'verifying') {
        // The new Panel answers and the record is still running: everything
        // before the confirmation is done, whatever an older plan said.
        const states = SYSTEM_UPDATE_STEP_IDS.map((id): SystemUpdateStepState => (id === 'confirm' ? 'running' : 'done'));
        return { steps: stepsFrom(states, plan), completed: false };
    }
    if ((input.phase === 'queued' || input.phase === 'applying') && plan) {
        return { steps: stepsFrom(plan.steps.map((step) => step.state), plan), completed: false };
    }
    return null;
}

// The step the page names: the running one, else none.
export function systemUpdateCurrentStep(view: SystemUpdateStepsView | null): SystemUpdateStepView | null {
    return view?.steps.find((step) => step.state === 'running') ?? null;
}

// The steps the start answer declared, or null. The start was accepted
// whatever this returns: an unreadable body only means the steps arrive with
// the first status read instead (1.5 s later).
// Baslangic yanitinin bildirdigi adimlar; okunamazsa ilk durum okumasi getirir.
export async function admittedSystemUpdatePlan(response: { json: () => Promise<unknown> }): Promise<SystemUpdatePlan | null> {
    let body: unknown;
    try {
        body = await response.json();
    } catch {
        return null;
    }
    return body && typeof body === 'object' ? decodeSystemUpdatePlan((body as Record<string, unknown>).plan) : null;
}
