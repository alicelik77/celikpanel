// A site's scheduled tasks as timers inside its own limits (S4 R2b,
// docs/SITE-SANDBOX.md section 9): the answers the screens read, decoded in
// one place, and what one site's move means for "Move all".
//
// Sitenin zamanlanmış görevleri kendi sınırları içinde; ekranların okuduğu
// yanıtlar tek yerde çözülür.

/** A task's last run as the server reads it. */
export interface TaskRun {
    state: 'never' | 'running' | 'succeeded' | 'failed' | 'unknown';
    started_at?: string;
    ended_at?: string;
    seconds?: number;
    exit_status?: number;
    result?: string;
    output?: string[];
    next_at?: string;
}

export const TASK_STATES = ['scheduled', 'off', 'at_boot', 'kept', 'waiting', 'not_run'] as const;
export type TaskState = (typeof TASK_STATES)[number];

const RUN_STATES: readonly string[] = ['never', 'running', 'succeeded', 'failed', 'unknown'];

/** Reads one task's run; anything of another shape is no run at all. */
export function decodeTaskRun(raw: unknown): TaskRun | undefined {
    const run = raw as Partial<TaskRun> | null;
    if (!run || typeof run !== 'object' || typeof run.state !== 'string' || !RUN_STATES.includes(run.state)) return undefined;
    return {
        state: run.state,
        started_at: typeof run.started_at === 'string' ? run.started_at : undefined,
        ended_at: typeof run.ended_at === 'string' ? run.ended_at : undefined,
        seconds: typeof run.seconds === 'number' ? run.seconds : undefined,
        exit_status: typeof run.exit_status === 'number' ? run.exit_status : undefined,
        result: typeof run.result === 'string' ? run.result : undefined,
        output: Array.isArray(run.output) ? run.output.filter((line): line is string => typeof line === 'string').slice(-10) : undefined,
        next_at: typeof run.next_at === 'string' ? run.next_at : undefined,
    };
}

/** A line of a moved site's task file that does not run. */
export interface TaskLine {
    line: number;
    text: string;
    reason: string;
}

export const TASK_REASONS = ['schedule', 'mailto', 'home', 'shell', 'env_value', 'carriage', 'empty', 'invalid_env', 'not_made_here'] as const;

export function decodeTaskLines(raw: unknown): TaskLine[] {
    if (!Array.isArray(raw)) return [];
    return raw.filter((l): l is TaskLine => !!l && typeof l.line === 'number' && typeof l.text === 'string' && typeof l.reason === 'string');
}

/** The administrator's line about the site's tasks on the Overview card. */
export interface TasksPlacement {
    moved: boolean;
    reappeared?: number;
    recovery?: string;
    unknown?: boolean;
}

export function decodeTasksPlacement(raw: unknown): TasksPlacement | undefined {
    const body = raw as Partial<TasksPlacement> | null;
    if (!body || typeof body !== 'object' || typeof body.moved !== 'boolean') return undefined;
    return {
        moved: body.moved,
        reappeared: typeof body.reappeared === 'number' ? body.reappeared : undefined,
        recovery: typeof body.recovery === 'string' ? body.recovery : undefined,
        unknown: body.unknown === true,
    };
}

/** What a tasks move did or would do (the preview, the move, the undo). */
export interface TasksAnswer {
    outcome: 'moved' | 'already' | 'stayed' | 'put_back' | 'preview' | 'undone' | 'refused' | 'no_cron' | 'unknown';
    rows: { line: number; text: string; unit?: string; at_boot?: boolean; paused?: boolean; setting?: boolean }[];
    blocking: TaskLine[];
    mail: boolean;
    error?: string;
    refusal?: string;
}

const OUTCOMES: readonly string[] = ['moved', 'already', 'stayed', 'put_back', 'preview', 'undone', 'refused', 'no_cron', 'unknown'];

export function decodeTasksAnswer(raw: unknown): TasksAnswer {
    const body = raw as Partial<TasksAnswer> | null;
    if (!body || typeof body !== 'object' || typeof body.outcome !== 'string' || !OUTCOMES.includes(body.outcome)) throw new Error('shape');
    return {
        outcome: body.outcome,
        rows: Array.isArray(body.rows) ? body.rows.filter((r) => !!r && typeof r.line === 'number' && typeof r.text === 'string') : [],
        blocking: decodeTaskLines(body.blocking),
        mail: body.mail === true,
        error: typeof body.error === 'string' ? body.error : undefined,
        refusal: typeof body.refusal === 'string' ? body.refusal : undefined,
    };
}

/** The tasks that become timers in a preview (not settings, not paused ones). */
export function movedTaskCount(answer: TasksAnswer): number {
    return answer.rows.filter((r) => !!r.unit).length;
}

/** "Line 6: <text>" for a list of lines. */
export function lineList(lines: TaskLine[]): string {
    return lines.map((l) => `${l.line}: ${l.text}`).join('; ');
}

/** Whether one site's move ended well for "Move all" (it then goes on). */
export function siteMoved(answer: { done?: boolean; rolled_back?: boolean; error?: string; tasks?: unknown }): { ok: boolean; reason: string } {
    let tasks: TasksAnswer | undefined;
    try {
        tasks = answer.tasks ? decodeTasksAnswer(answer.tasks) : undefined;
    } catch {
        tasks = undefined;
    }
    if (answer.rolled_back || answer.done === false) return { ok: false, reason: answer.error ?? '' };
    if (!tasks) return { ok: false, reason: '' };
    if (tasks.outcome === 'stayed') return { ok: false, reason: lineList(tasks.blocking) };
    if (tasks.outcome !== 'moved' && tasks.outcome !== 'already') return { ok: false, reason: tasks.error ?? tasks.outcome };
    return { ok: true, reason: '' };
}

/** A site still outside its limits (GET /api/v1/site-limits/outside). */
export interface OutsideSite {
    domain_id: number;
    domain: string;
    programs?: number;
    tasks?: boolean;
}

export function decodeOutsideSites(raw: unknown): OutsideSite[] {
    const body = raw as { sites?: unknown } | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.sites)) throw new Error('shape');
    return body.sites.filter((s): s is OutsideSite => !!s && typeof s.domain_id === 'number' && typeof s.domain === 'string');
}

/** What the move would make of a site's crontab (read-only); throws when it cannot be read. */
export async function readTasksPreview(domainId: number): Promise<TasksAnswer> {
    const sent = await fetch(`/api/v1/domains/${domainId}/site-limits/preview`, { credentials: 'same-origin' });
    if (!sent.ok) throw new Error(`preview ${sent.status}`);
    return decodeTasksAnswer(await sent.json());
}

/** The sites still outside their limits; throws when the list cannot be read. */
export async function readOutsideSites(): Promise<OutsideSite[]> {
    const sent = await fetch('/api/v1/site-limits/outside', { credentials: 'same-origin' });
    if (!sent.ok) throw new Error(`outside ${sent.status}`);
    return decodeOutsideSites(await sent.json());
}
