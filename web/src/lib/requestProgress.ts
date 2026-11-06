import type { TranslationKey } from '../i18n/en';
import { readApiError, refusalText } from './apiError';
import { followOperation, type OperationEnd, type OperationReport, type OperationStep } from './confirmAction';
import { REQUEST_ID_HEADER, newRequestId } from './requestIdentity';

// The real steps of a change, read from the server while it runs (alpha.127,
// owner's firm order of 6 Nov 2026: "say, like the update screen does, doing
// this, doing that"). The change is sent with an identity made here; while
// its request runs, GET /api/v1/operation-progress/{identity} is read every
// 1.5 s (reads only: the server's guard never runs a change twice) and the
// progress window shows the server's plan with the step it is at. When the
// answer arrives the step it ended at is in its X-CelikPanel-Operation-Step
// header: a refusal marks that step failed, and a refusal at a check that
// runs before anything is changed is an unmet prerequisite (D-024). A lost
// answer leaves the step "not known".
//
// The identity is kept in this tab's session while the change runs, so a
// reload reopens the window at the step the server is at now
// (resumeRequestProgress) and says how it ended; it is forgotten when the
// change ends.
//
// Bir değişikliğin gerçek adımları: değişiklik burada üretilen kimlikle
// gönderilir, istek sürerken adımı her 1,5 sn'de okunur (yalnız okuma). Yanıt
// gelince bittiği adım başlıktadır. Kimlik oturumda tutulur; sayfa yeniden
// yüklenirse pencere o anki adımda yeniden açılır.

export const OPERATION_STEP_HEADER = 'X-CelikPanel-Operation-Step';
const PROGRESS_URL = '/api/v1/operation-progress/';
const POLL_MS = 1500;
const STORAGE_PREFIX = 'celikpanel.op.';
// After this long without a readable answer the window says the result is not known.
const UNREAD_LIMIT_MS = 60_000;

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** One step a flow can show, in the server's order. */
export interface StepLabel {
    /** The server's step code. */
    id: string;
    label: string;
    /** Said under the step while it runs. */
    note?: string;
    /** A check before anything is changed: a refusal here is an unmet prerequisite. */
    check?: boolean;
}

export type ProgressRead =
    | { state: 'running'; plan?: string[]; step?: string; since?: number }
    | { state: 'done'; plan?: string[]; step?: string; status: number; answer?: unknown }
    | { state: 'unknown'; plan?: string[]; step?: string }
    | { state: 'gone' }
    | { state: 'unread' };

const strings = (value: unknown): string[] | undefined =>
    Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : undefined;

/** Reads what the server knows about one change of this person. Reads only. */
export async function readRequestProgress(id: string, send: typeof fetch = fetch): Promise<ProgressRead> {
    try {
        const res = await send(PROGRESS_URL + id, { cache: 'no-store' });
        if (res.status === 404) return { state: 'gone' };
        if (!res.ok) return { state: 'unread' };
        const body = await res.json() as Record<string, unknown>;
        const plan = strings(body.plan);
        const step = typeof body.step === 'string' ? body.step : undefined;
        if (body.state === 'running') {
            const since = typeof body.step_started_at === 'string' ? Date.parse(body.step_started_at) : NaN;
            return { state: 'running', plan, step, since: Number.isFinite(since) ? since : undefined };
        }
        if (body.state === 'done' && typeof body.response_status === 'number') {
            return { state: 'done', plan, step, status: body.response_status, answer: body.answer };
        }
        if (body.state === 'unknown') return { state: 'unknown', plan, step };
        return { state: 'unread' };
    } catch {
        return { state: 'unread' };
    }
}

/** How the run ended at its step: still running, done, refused there, or not known. */
export type StepsAt = 'running' | 'done' | 'failed' | 'unknown';

/**
 * The window's steps: the server's plan (only the steps this flow has words
 * for, in its order) with the current one marked. Before the server has named
 * a step the first one runs.
 */
export function progressSteps(labels: StepLabel[], t: T, plan: string[] | undefined, step: string | undefined, since: number, at: StepsAt, status?: number): OperationStep[] {
    const known = new Map(labels.map((label) => [label.id, label]));
    const planned = plan?.filter((id) => known.has(id));
    const order = planned && planned.length > 0 ? planned : labels.map((label) => label.id);
    const index = step && order.includes(step) ? order.indexOf(step) : 0;
    return order.map((id, position) => {
        const label = known.get(id)!;
        const base = { id, label: label.label };
        if (at === 'done' || position < index) return { ...base, state: 'done' as const };
        if (position > index) return { ...base, state: 'pending' as const };
        if (at === 'failed') {
            // A refusal at a check (4xx) changed nothing: an unmet prerequisite.
            const prerequisite = !!label.check && status !== undefined && status >= 400 && status < 500;
            return { ...base, state: 'failed' as const, prerequisite, note: t(prerequisite ? 'opstep.stoppedBeforeChange' : 'opstep.stoppedHere') };
        }
        if (at === 'unknown') return { ...base, state: 'waiting' as const, note: t('opstep.stepUnknown') };
        return { ...base, state: 'running' as const, since, note: label.note };
    });
}

/** Plain words the window needs again after a reload (a version, a name); never a secret. */
export type ProgressContext = Record<string, string>;

/** Keeps a running change's identity in this tab's session (for a reload). */
export function remember(key: string, id: string, context: ProgressContext = {}) {
    try {
        window.sessionStorage.setItem(STORAGE_PREFIX + key, JSON.stringify({ id, at: Date.now(), context }));
    } catch {
        // Without storage a reload cannot reopen the window; the change still runs.
    }
}

export function forget(key: string) {
    try {
        window.sessionStorage.removeItem(STORAGE_PREFIX + key);
    } catch {
        // Nothing to forget.
    }
}

export function remembered(key: string): { id: string; context: ProgressContext } | null {
    try {
        const raw = window.sessionStorage.getItem(STORAGE_PREFIX + key);
        if (!raw) return null;
        const value = JSON.parse(raw) as { id?: unknown; at?: unknown; context?: unknown };
        // A day later the server has forgotten the change too.
        if (typeof value.id !== 'string' || typeof value.at !== 'number' || Date.now() - value.at > 86_400_000) {
            forget(key);
            return null;
        }
        const context: ProgressContext = {};
        if (value.context && typeof value.context === 'object') {
            for (const [name, item] of Object.entries(value.context)) if (typeof item === 'string') context[name] = item;
        }
        return { id: value.id, context };
    } catch {
        return null;
    }
}

const pause = (ms: number) => new Promise<void>((resolve) => { setTimeout(resolve, ms); });

export interface RequestProgress {
    /** The identity header to send with the change. */
    headers: Record<string, string>;
    /** The change's answer arrived (or null: it was lost); marks the steps and stops reading. */
    settle(res: Response | null): void;
}

/**
 * Follows one change sent with the returned headers. `key` names it in this
 * tab's session (flow and target, e.g. "ssl.12") so a reload can reopen it.
 */
export function followRequestProgress(report: OperationReport, labels: StepLabel[], t: T, key: string, context: ProgressContext = {}): RequestProgress {
    const id = newRequestId();
    const startedAt = Date.now();
    let plan: string[] | undefined;
    let step: string | undefined;
    let since = startedAt;
    let settled = false;
    remember(key, id, context);
    report.steps(progressSteps(labels, t, undefined, undefined, startedAt, 'running'));
    void (async () => {
        while (!settled) {
            await pause(POLL_MS);
            if (settled) return;
            const read = await readRequestProgress(id);
            if (settled || read.state !== 'running') continue;
            plan = read.plan ?? plan;
            if (read.step && read.step !== step) {
                step = read.step;
                since = read.since ?? Date.now();
            }
            report.steps(progressSteps(labels, t, plan, step, since, 'running'));
        }
    })();
    return {
        headers: { [REQUEST_ID_HEADER]: id },
        settle(res) {
            if (settled) return;
            settled = true;
            forget(key);
            if (!res) {
                report.steps(progressSteps(labels, t, plan, step, since, 'unknown'));
                return;
            }
            const at = res.headers.get(OPERATION_STEP_HEADER) || step;
            report.steps(progressSteps(labels, t, plan, at, since, res.ok ? 'done' : 'failed', res.status));
        },
    };
}

/**
 * After a reload: when this tab sent the change named `key` and it may still
 * run, the locked window reopens at the step the server is at and stays until
 * it ends, then says how it ended. `ended` runs once it has (the page reads
 * its state again). Reads only.
 */
export function resumeRequestProgress(options: {
    key: string;
    /** The steps, from the words the change was sent with. */
    labels: (context: ProgressContext) => StepLabel[];
    t: T;
    progressTitle: (context: ProgressContext) => string;
    /** The screen's sentence for a refusal of this change. */
    refusedSentence: TranslationKey;
    ended?: () => void;
}): void {
    const kept = remembered(options.key);
    if (!kept) return;
    const { id, context } = kept;
    const { t } = options;
    const labels = options.labels(context);
    void followOperation({
        progressTitle: options.progressTitle(context),
        action: async (report: OperationReport): Promise<OperationEnd | void> => {
            let since = Date.now();
            let step: string | undefined;
            let unreadSince: number | null = null;
            try {
                for (;;) {
                    const read = await readRequestProgress(id);
                    if (read.state === 'unread') {
                        unreadSince ??= Date.now();
                        if (Date.now() - unreadSince > UNREAD_LIMIT_MS) return { state: 'unknown', reason: t('opstep.unread') };
                    } else {
                        unreadSince = null;
                    }
                    if (read.state === 'gone') {
                        // The server keeps no such change for this person: nothing to follow.
                        return { state: 'unknown', reason: t('opstep.gone') };
                    }
                    if (read.state === 'running') {
                        if (read.step && read.step !== step) {
                            step = read.step;
                            since = read.since ?? Date.now();
                        }
                        report.steps(progressSteps(labels, t, read.plan, read.step, since, 'running'));
                    }
                    if (read.state === 'unknown') {
                        report.steps(progressSteps(labels, t, read.plan, read.step, since, 'unknown'));
                        return { state: 'unknown', reason: t('opstep.interrupted') };
                    }
                    if (read.state === 'done') {
                        const ok = read.status >= 200 && read.status < 300;
                        const steps = progressSteps(labels, t, read.plan, read.step, since, ok ? 'done' : 'failed', read.status);
                        report.steps(steps);
                        if (ok) return { state: 'done' };
                        if (read.answer === undefined) return { state: 'failed', reason: t('opstep.refusedNotKept') };
                        const problem = await readApiError(new Response(JSON.stringify(read.answer), { status: read.status }));
                        const reason = refusalText(problem, t, options.refusedSentence);
                        return { state: steps.some((item) => item.prerequisite) ? 'blocked' : 'failed', reason };
                    }
                    await pause(POLL_MS);
                }
            } finally {
                forget(options.key);
                options.ended?.();
            }
        },
    });
}

// The steps of each flow, in the server's order (Go: reportOperationPlan /
// reportOperationStep in the handler named beside each).
// Her akışın adımları, sunucunun sırasıyla.

/** handleIssueLetsEncrypt (domain_ssl_handlers.go). */
export const sslIssueSteps = (t: T, name: string, authority: string): StepLabel[] => [
    { id: 'check', label: t('opstep.ssl.check'), check: true },
    { id: 'dns_check', label: t('opstep.ssl.dns_check', { name }), check: true },
    { id: 'prepare_validation', label: t('opstep.ssl.prepare_validation') },
    { id: 'acme', label: t('opstep.ssl.acme', { authority }), note: t('opstep.ssl.acmeNote') },
    { id: 'verify', label: t('opstep.ssl.verify') },
    { id: 'install', label: t('opstep.ssl.install') },
    { id: 'reload', label: t('opstep.ssl.reload') },
    { id: 'mail', label: t('opstep.ssl.mail') },
];

/** handleRestoreBackup (domain_backup_handlers.go). */
export const backupRestoreSteps = (t: T): StepLabel[] => [
    { id: 'check', label: t('opstep.restore.check'), check: true },
    { id: 'inspect', label: t('opstep.restore.inspect'), check: true },
    { id: 'restore', label: t('opstep.restore.restore'), note: t('opstep.restore.restoreNote') },
    { id: 'finish', label: t('opstep.restore.finish') },
];

/** handleCreateBackup (domain_backup_handlers.go). */
export const backupCreateSteps = (t: T): StepLabel[] => [
    { id: 'check', label: t('opstep.backup.check'), check: true },
    { id: 'create', label: t('opstep.backup.create'), note: t('opstep.backup.createNote') },
    { id: 'finish', label: t('opstep.backup.finish') },
];

/** handleDomainPHPSettings POST (domain_php_handlers.go). */
export const phpVersionSteps = (t: T, version: string): StepLabel[] => [
    { id: 'check', label: t('opstep.php.check', { version }), check: true },
    { id: 'pool', label: t('opstep.php.pool', { version }) },
    { id: 'save', label: t('opstep.php.save') },
    { id: 'web_server', label: t('opstep.php.web_server', { version }) },
];
