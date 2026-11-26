// A site's scheduled tasks as timers inside its own limits (S4 R2b,
// docs/SITE-SANDBOX.md section 9): the answers the screens read, decoded in
// one place, and what one site's move means for "Move all".
//
// Sitenin zamanlanmış görevleri kendi sınırları içinde; ekranların okuduğu
// yanıtlar tek yerde çözülür.

/** A task's last run as the server reads it. */
export interface TaskRun {
    /** skipped (second fix round): systemd skipped the run because the site's limits file was missing. */
    state: 'never' | 'running' | 'succeeded' | 'failed' | 'unknown' | 'skipped';
    started_at?: string;
    ended_at?: string;
    seconds?: number;
    exit_status?: number;
    result?: string;
    output?: string[];
    next_at?: string;
}

export const TASK_STATES = ['scheduled', 'off', 'at_boot', 'kept', 'waiting', 'not_run', 'not_on_yet'] as const;
export type TaskState = (typeof TASK_STATES)[number];

const RUN_STATES: readonly string[] = ['never', 'running', 'succeeded', 'failed', 'unknown', 'skipped'];

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

export const TASK_REASONS = ['schedule', 'mailto', 'home', 'shell', 'env_value', 'carriage', 'empty', 'invalid_env', 'too_long', 'not_made_here'] as const;

export function decodeTaskLines(raw: unknown): TaskLine[] {
    if (!Array.isArray(raw)) return [];
    return raw.filter((l): l is TaskLine => !!l && typeof l.line === 'number' && typeof l.text === 'string' && typeof l.reason === 'string');
}

/** The administrator's line about the site's tasks on the Overview card. */
export interface TasksPlacement {
    moved: boolean;
    reappeared?: number;
    /** Fix round: the account could have installed that crontab itself. */
    by_site?: boolean;
    /** Whether sites may reach this server's own submission services ('ours'), or why not. */
    mail_submission?: string;
    recovery?: string;
    unknown?: boolean;
}

/** The server owner's two steps by hand for a kept tasks move record that Try again cannot settle (timers off first). */
export function unresolvedSteps(u: { tasks_site?: number; tasks_account?: string }): string {
    if (!u.tasks_account || !u.tasks_site) return '';
    return `sudo systemctl disable --now 'celikcron-${u.tasks_site}-*.timer' ; sudo crontab -u ${u.tasks_account} /etc/celikpanel-cron/${u.tasks_site}.crontab`;
}

export function decodeTasksPlacement(raw: unknown): TasksPlacement | undefined {
    const body = raw as Partial<TasksPlacement> | null;
    if (!body || typeof body !== 'object' || typeof body.moved !== 'boolean') return undefined;
    return {
        moved: body.moved,
        reappeared: typeof body.reappeared === 'number' ? body.reappeared : undefined,
        by_site: body.by_site === true,
        mail_submission: typeof body.mail_submission === 'string' ? body.mail_submission : undefined,
        recovery: typeof body.recovery === 'string' ? body.recovery : undefined,
        unknown: body.unknown === true,
    };
}

/** What a tasks move did or would do (the preview, the move, the undo). */
export interface TasksAnswer {
    outcome: 'moved' | 'already' | 'stayed' | 'put_back' | 'preview' | 'undone' | 'refused' | 'no_cron' | 'unknown' | 'crontab_back';
    rows: { line: number; text: string; unit?: string; at_boot?: boolean; paused?: boolean; setting?: boolean; privileged?: boolean }[];
    blocking: TaskLine[];
    mail: boolean;
    mail_submission?: string;
    error?: string;
    refusal?: string;
}

const OUTCOMES: readonly string[] = ['moved', 'already', 'stayed', 'put_back', 'preview', 'undone', 'refused', 'no_cron', 'unknown', 'crontab_back'];

export function decodeTasksAnswer(raw: unknown): TasksAnswer {
    const body = raw as Partial<TasksAnswer> | null;
    if (!body || typeof body !== 'object' || typeof body.outcome !== 'string' || !OUTCOMES.includes(body.outcome)) throw new Error('shape');
    return {
        outcome: body.outcome,
        rows: Array.isArray(body.rows) ? body.rows.filter((r) => !!r && typeof r.line === 'number' && typeof r.text === 'string') : [],
        blocking: decodeTaskLines(body.blocking),
        mail: body.mail === true,
        mail_submission: typeof body.mail_submission === 'string' ? body.mail_submission : undefined,
        error: typeof body.error === 'string' ? body.error : undefined,
        refusal: typeof body.refusal === 'string' ? body.refusal : undefined,
    };
}

/** The claim's refusals a tasks move, undo or rebuild shares with the programs' move (main's codes). */
const MOVE_WHYS: readonly string[] = ['kept_earlier', 'update_running', 'update_unreadable', 'another_move', 'site_busy'];
/** The tasks' own refusal codes (transport.SiteTasksRefusal*). */
const TASK_WHYS: readonly string[] = ['cron_allow_owner', 'source_owner', 'crontab_changed', 'limits'];

/**
 * The words of a tasks answer that did not end as asked (alpha.150, main's
 * rule: a code worded by the page, never the agent's English; the agent's
 * journal holds the detail). undo: the answer of "Put the tasks back into cron".
 */
export function tasksAnswerKey(answer: TasksAnswer, undo: boolean): string {
    const refusal = answer.refusal ?? '';
    if (MOVE_WHYS.includes(refusal)) return `slim.why.${refusal}`;
    if (TASK_WHYS.includes(refusal)) return `slim.tasks.${undo ? 'undoWhy' : 'why'}.${refusal}`;
    switch (answer.outcome) {
        case 'put_back': return 'slim.tasks.why.put_back';
        case 'unknown': return undo ? 'slim.tasks.undoWhy.unknown' : 'slim.tasks.why.unknown';
        case 'no_cron': return 'slim.tasks.why.no_cron';
        case 'crontab_back': return 'slim.tasks.done.back';
    }
    return undo ? 'slim.tasks.undoWhy.other' : 'slim.tasks.why.other';
}

/** How a kept tasks record's tasks run now (SiteSliceMoveRecovery.tasks_running), or '' when it is not known. */
export function keptTasksRunningKey(running: string | undefined): string {
    return running === 'none' || running === 'timers' || running === 'cron' ? `slim.tasks.running.${running}` : '';
}

/** The tasks that become timers in a preview (not settings, not paused ones). */
export function movedTaskCount(answer: TasksAnswer): number {
    return answer.rows.filter((r) => !!r.unit).length;
}

/** "Line 6: <text>" for a list of lines. */
export function lineList(lines: TaskLine[]): string {
    return lines.map((l) => `${l.line}: ${l.text}`).join('; ');
}

/** The tasks that call a program needing extra rights (set-user-id), as "line: text". */
export function privilegedList(answer: TasksAnswer): string {
    return answer.rows.filter((r) => r.privileged).map((r) => `${r.line}: ${r.text}`).join('; ');
}

/**
 * How one site's move ended for "Move all" (fix round): moved; a site where
 * nothing changed (skipped, with why: the run goes on); a site whose programs
 * moved while its tasks stayed (goes on); or a move that started and failed,
 * or whose result is unknown (the run stops).
 */
export type SiteMoveEnd = { kind: 'moved'; note: string } | { kind: 'skipped' | 'partial' | 'stop'; reason: string; unresolved?: boolean };

/** The refusal every move and undo answers while a move record is kept (alpha.150: one record for programs and tasks). */
export const KEPT_MOVE = 'kept_earlier';

export function siteMoveEnd(answer: { done?: boolean; rolled_back?: boolean; error?: string; kept?: boolean; reason?: { code?: string }; units?: unknown[]; tasks?: unknown }): SiteMoveEnd {
    let tasks: TasksAnswer | undefined;
    try {
        tasks = answer.tasks ? decodeTasksAnswer(answer.tasks) : undefined;
    } catch {
        tasks = undefined;
    }
    // Second fix round: an earlier move that could not be resolved stops the
    // run (nothing else is moved until it is).
    if (answer.kept === true || answer.reason?.code === KEPT_MOVE || tasks?.refusal === KEPT_MOVE) return { kind: 'stop', reason: tasks?.error ?? answer.error ?? '', unresolved: true };
    if (answer.rolled_back || answer.done === false) return { kind: 'stop', reason: answer.error ?? '' };
    if (!tasks || tasks.outcome === 'put_back' || tasks.outcome === 'unknown') return { kind: 'stop', reason: tasks?.error ?? '' };
    if (tasks.outcome === 'moved' || tasks.outcome === 'already') return { kind: 'moved', note: '' };
    if (tasks.outcome === 'crontab_back') return { kind: 'moved', note: tasks.error ?? '' };
    // Nothing of the tasks changed (stayed, refused, no cron).
    const reason = tasks.outcome === 'stayed' ? lineList(tasks.blocking) : tasks.error ?? tasks.outcome;
    return { kind: Array.isArray(answer.units) && answer.units.length > 0 ? 'partial' : 'skipped', reason };
}

/** How "Move all" ended (fix round). */
export interface MoveAllOutcome {
    moved: number;
    /** Sites that did not (fully) move, with why; partial: its programs moved, its tasks stayed. */
    notMoved: { domain: string; reason: string; partial?: boolean }[];
    /** The move that started and failed (or whose result is unknown): the run stopped there. */
    stop?: { domain: string; reason: string; unknown: boolean; unresolved?: boolean };
}

/**
 * Refusals that changed nothing (fix round 2, review N5): a bad request, no
 * right, no such site, a move already running or a conflict. Every other
 * answer that is not a success - a server error, a gateway timeout, an
 * answer without a readable body - is "result unknown": the run stops.
 */
const SKIP_STATUSES: readonly number[] = [400, 403, 404, 409];

/** One site's move as "Move all" sends it: the HTTP status and answer, or null when nothing readable came back. */
export type SiteMoveCall = (site: OutsideSite) => Promise<{ status: number; body?: unknown; refusal?: string } | null>;

/**
 * "Move all"'s run (fix round): each site in turn; a site refused before
 * anything changed, or whose move changed nothing, is skipped with why and
 * the run goes on; a move that started and failed, or whose result is not
 * known, stops it. onStep says each site's state as it goes.
 */
export async function moveSitesInTurn(
    sites: OutsideSite[], move: SiteMoveCall,
    onStep: (index: number, state: 'running' | 'done' | 'failed', note?: string) => void,
): Promise<MoveAllOutcome> {
    const out: MoveAllOutcome = { moved: 0, notMoved: [] };
    for (let i = 0; i < sites.length; i++) {
        const site = sites[i];
        onStep(i, 'running');
        const sent = await move(site);
        if (!sent) {
            onStep(i, 'failed');
            out.stop = { domain: site.domain, reason: '', unknown: true };
            return out;
        }
        if (sent.status < 200 || sent.status > 299) {
            if (!SKIP_STATUSES.includes(sent.status)) {
                onStep(i, 'failed', sent.refusal);
                out.stop = { domain: site.domain, reason: sent.refusal ?? '', unknown: true };
                return out;
            }
            onStep(i, 'failed', sent.refusal);
            out.notMoved.push({ domain: site.domain, reason: sent.refusal ?? '' });
            continue;
        }
        if (!sent.body || typeof sent.body !== 'object') {
            onStep(i, 'failed');
            out.stop = { domain: site.domain, reason: '', unknown: true };
            return out;
        }
        const end = siteMoveEnd(sent.body as Parameters<typeof siteMoveEnd>[0]);
        switch (end.kind) {
            case 'stop':
                onStep(i, 'failed', end.reason);
                out.stop = end.unresolved ? { domain: site.domain, reason: end.reason, unknown: false, unresolved: true }
                    : { domain: site.domain, reason: end.reason, unknown: false };
                return out;
            case 'moved':
                out.moved++;
                onStep(i, 'done', end.note || undefined);
                break;
            case 'partial':
                onStep(i, 'done', end.reason);
                out.notMoved.push({ domain: site.domain, reason: end.reason, partial: true });
                break;
            default:
                onStep(i, 'failed', end.reason);
                out.notMoved.push({ domain: site.domain, reason: end.reason });
        }
    }
    return out;
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
