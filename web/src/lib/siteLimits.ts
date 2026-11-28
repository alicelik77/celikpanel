// The limits of a site's programs together (S4 R2a): the answer of
// GET /api/v1/domains/{id}/site-limits and what the card says of it. Pure,
// so the card and the tests read the same rules.
//
// Bir sitenin programlarının birlikte sınırları: sunucunun yanıtı ve kartın
// ondan söyledikleri.

import type { TasksPlacement } from './siteTasks';

export type SiteLimitKey = 'site.memory_mb' | 'site.tasks' | 'site.cpu_percent';

export interface SiteLimitValue {
    key: SiteLimitKey;
    value: number;
    set_by: string;
    set_by_name?: string;
}

export interface SiteLimitEvent {
    /**
     * memory, tasks: the site's own limit; *_other: a program of the site, not proved at the site's limit;
     * tasks_peak (alpha.149): the slice's own peak reached its process limit (a count is not known).
     */
    kind: 'memory' | 'tasks' | 'tasks_peak' | 'cpu' | 'memory_other' | 'tasks_other';
    count: number;
    first_at: string;
    last_at: string;
    repeated?: boolean;
    /** alpha.149: the limit that held then (absent: not known; never today's). */
    limit?: number;
}

export interface SiteLimitUnit {
    unit: string;
    kind: string;
    label: string;
    /** restarting (alpha.149): not running; systemd is starting it again by itself. */
    placement: 'in_slice' | 'outside' | 'stopped' | 'stopped_outside' | 'elsewhere' | 'absent' | 'restarting';
    own_memory_mb?: number;
    lower?: boolean;
    /** Why the move cannot take it: edited, older, owner_slice. */
    blocked?: 'edited' | 'older' | 'owner_slice';
}

export interface SiteLimitsView {
    known: boolean;
    read_error?: string;
    domain: string;
    admin: boolean;
    reseller?: boolean;
    limits: SiteLimitValue[];
    /**
     * yes, outside, no_file, unsupported (unsupported names what the server lacks); pending (written, the
     * kernel does not hold them yet) and unknown (not read) for the administrator; not_yet is what a customer
     * or reseller reads for every state they cannot act on.
     */
    applies?: 'yes' | 'outside' | 'no_file' | 'unsupported' | 'pending' | 'unknown' | 'not_yet';
    unsupported?: string;
    drop_ins?: string[];
    /** kept: the put-back did not complete; no site is moved until it is settled ("Try again"). */
    /** tasks* (S4 R2b, alpha.150): the kept record is of the site's scheduled tasks. */
    move_interrupted?: { at: string; units: Record<string, string>; kept?: boolean; tasks?: string; tasks_step?: string; tasks_site?: number; tasks_account?: string; tasks_running?: string };
    /**
     * Database users' values the panel did not write (alpha.149): origin `owner` for a firm limit only an
     * administrator account can set (temporary files, the connection limit); `on_user` for a default changed on
     * the database user itself (statement time, idle transaction time), by the server owner or the user's own
     * session. An item without a known origin is read as the owner's.
     */
    db_owner?: DbOwnerSetting[];
    apply_done?: number;
    apply_total?: number;
    live?: { memory_mb: number; tasks: number; cpu_percent: number };
    memory_current_mb?: number;
    tasks_current?: number;
    events: SiteLimitEvent[];
    slice_file?: string;
    slice?: string;
    units?: SiteLimitUnit[];
    needs_move?: number;
    busy?: boolean;
    /** S4 R2b (administrator): where the site's scheduled tasks run. */
    tasks?: TasksPlacement;
    /** alpha.150 (D-048, administrator): the panel moving sites by itself; the card's one move action. */
    auto_move?: AutoMove;
}

/**
 * alpha.150 (D-048): every site goes inside its limits by itself. done/total: the run's progress (the calm
 * line); offer: "retry" next to a move that did not bring this site inside (state, reason), "move" for a site
 * whose scheduled tasks were put back into cron on purpose. A healthy site's card offers nothing.
 */
export interface AutoMove {
    done?: number;
    total?: number;
    /** Fix round G5: the run waits instead of moving, and why. */
    waiting?: string;
    offer?: 'retry' | 'move';
    state?: string;
    reason?: string;
    /** Fix round G5: which part ended the move, and whether it put the site back. */
    part?: string;
    put_back?: boolean;
}

function decodeAutoMove(raw: unknown): AutoMove | undefined {
    const body = raw as AutoMove | null;
    if (!body || typeof body !== 'object') return undefined;
    const num = (n: unknown) => (typeof n === 'number' && n >= 0 ? n : undefined);
    return {
        done: num(body.done), total: num(body.total),
        offer: body.offer === 'retry' || body.offer === 'move' ? body.offer : undefined,
        state: typeof body.state === 'string' ? body.state : undefined,
        reason: typeof body.reason === 'string' ? body.reason : undefined,
        waiting: typeof body.waiting === 'string' ? body.waiting : undefined,
        part: typeof body.part === 'string' ? body.part : undefined,
        put_back: body.put_back === true,
    };
}

/** Fix round G5: the card follows the run by itself while the worker has work or the site is outside its limits. */
export function followsAutoMove(view: SiteLimitsView | undefined): boolean {
    if (!view?.admin) return false;
    const tasksOutside = !!view.tasks && !view.tasks.unknown && !view.tasks.moved;
    return (view.auto_move?.total ?? 0) > 0 || (view.needs_move ?? 0) > 0 || view.applies === 'outside' || tasksOutside;
}

export interface DbOwnerSetting {
    user: string;
    key: string;
    value: string;
    origin?: 'owner' | 'on_user';
}

/** The card's two sentences: the server owner's firm limits, and defaults changed on the database user itself. */
export function dbOwnerByOrigin(view: SiteLimitsView): { owner: DbOwnerSetting[]; onUser: DbOwnerSetting[] } {
    const items = (view.db_owner ?? []).filter((s): s is DbOwnerSetting => !!s && typeof s.user === 'string' && typeof s.key === 'string');
    return { owner: items.filter((s) => s.origin !== 'on_user'), onUser: items.filter((s) => s.origin === 'on_user') };
}

const KEYS: readonly string[] = ['site.memory_mb', 'site.tasks', 'site.cpu_percent'];
const EVENTS: readonly string[] = ['memory', 'tasks', 'tasks_peak', 'cpu', 'memory_other', 'tasks_other'];
const PLACES: readonly string[] = ['in_slice', 'outside', 'stopped', 'stopped_outside', 'elsewhere', 'absent', 'restarting'];

/** S4 R2b: the site's tasks placement (a type-only import keeps this module standalone). */
function decodeTasksPlacement(raw: unknown): TasksPlacement | undefined {
    const body = raw as Partial<TasksPlacement> | null;
    if (!body || typeof body !== 'object' || typeof body.moved !== 'boolean') return undefined;
    return { moved: body.moved, reappeared: typeof body.reappeared === 'number' ? body.reappeared : undefined, by_site: body.by_site === true,
        mail_submission: typeof body.mail_submission === 'string' ? body.mail_submission : undefined,
        recovery: typeof body.recovery === 'string' ? body.recovery : undefined, unknown: body.unknown === true };
}

/** Reads the answer; an answer of another shape is not a card. */
export function decodeSiteLimits(raw: unknown): SiteLimitsView {
    const body = raw as Partial<SiteLimitsView> | null;
    if (!body || typeof body !== 'object' || typeof body.known !== 'boolean' || !Array.isArray(body.limits) || !Array.isArray(body.events)) {
        throw new Error('shape');
    }
    return {
        ...body,
        known: body.known,
        domain: typeof body.domain === 'string' ? body.domain : '',
        admin: body.admin === true,
        limits: body.limits.filter((l): l is SiteLimitValue => !!l && KEYS.includes(l.key) && typeof l.value === 'number' && typeof l.set_by === 'string'),
        events: body.events.filter((e): e is SiteLimitEvent => !!e && EVENTS.includes(e.kind) && typeof e.count === 'number' && typeof e.last_at === 'string'),
        units: Array.isArray(body.units) ? body.units.filter((u): u is SiteLimitUnit => !!u && typeof u.unit === 'string' && PLACES.includes(u.placement)) : undefined,
        tasks: decodeTasksPlacement(body.tasks),
        auto_move: decodeAutoMove(body.auto_move),
    };
}

/** The limit of one key (0: not in the answer). */
export function limitOf(view: SiteLimitsView, key: SiteLimitKey): SiteLimitValue | undefined {
    return view.limits.find((l) => l.key === key);
}

/**
 * The value systemd applies for one key when it is not the plan's (an owner's
 * drop-in, an edited file, or a change of limits that has not reached the
 * site yet): undefined when it is the plan's or not known; 0 means no limit.
 */
export function appliedOf(view: SiteLimitsView, key: SiteLimitKey): number | undefined {
    const limit = limitOf(view, key);
    if (!view.live || !limit) return undefined;
    const live = key === 'site.memory_mb' ? view.live.memory_mb : key === 'site.tasks' ? view.live.tasks : view.live.cpu_percent;
    return live === limit.value ? undefined : live;
}

/** The limit an event is about. */
export function eventKey(kind: SiteLimitEvent['kind']): SiteLimitKey {
    return kind.startsWith('memory') ? 'site.memory_mb' : kind.startsWith('tasks') ? 'site.tasks' : 'site.cpu_percent';
}

/** Whether an event is at the site's own limit (only then the plan is named and who can raise it). */
export function eventAtSiteLimit(kind: SiteLimitEvent['kind']): boolean {
    return !kind.endsWith('_other');
}

/** Why the move did not take a unit, as a blocked reason (kept: edited). */
export function notMovedReason(outcome: string): 'edited' | 'older' | 'owner_slice' | 'changed' | undefined {
    if (outcome === 'kept') return 'edited';
    if (outcome === 'older' || outcome === 'owner_slice' || outcome === 'changed') return outcome;
    return undefined;
}

/** The running programs a move would restart that may get less memory than now. */
export function lowerUnits(view: SiteLimitsView): SiteLimitUnit[] {
    return (view.units ?? []).filter((u) => u.placement === 'outside' && u.lower === true);
}

/** The code of a move's failure with its values (alpha.149): the page words it in its own language. */
export interface MoveReason {
    code: string;
    state?: string;
    seconds?: number;
    ports?: number[];
}

/** What happened to one unit of a move, for the sentence that lists the ones not moved. */
export interface MoveOutcome {
    unit: string;
    outcome: string;
    step?: string;
    put_back?: boolean;
    /** The agent's English words: kept for its journal and the audit record, never shown. */
    detail?: string;
    reason?: MoveReason;
    /** Why its put-back did not complete. */
    back?: MoveReason;
    /** Fix round W1: it ran when the move began (put back and running again), else put back as it was. */
    ran?: boolean;
}

const REASONS: readonly string[] = ['not_running', 'still_starting', 'not_in_slice', 'not_listening', 'no_port', 'restart_failed', 'write_failed', 'record_failed', 'unread', 'ports_unread'];

/**
 * A step's reason as a catalogue key and its values (alpha.149, lab D7): never the agent's English
 * text; a code this page does not know is the general sentence that points to the agent's journal.
 * Ports by count (fix round W4): one port "on port 3000"; several "on all of its ports" (the move's
 * step) or "on none of its ports" (a put-back, F10).
 */
export function moveReasonWords(reason: MoveReason | undefined): { key: string; values: Record<string, string | number> } {
    const ports = reason?.ports ?? [];
    const values = { seconds: reason?.seconds ?? 0, ports: ports.join(', ') };
    if (!reason || !REASONS.includes(reason.code)) return { key: 'slim.reason.other', values };
    if (reason.code === 'not_running' && (reason.state ?? '').includes('auto-restart')) return { key: 'slim.reason.restarting', values };
    if (reason.code === 'not_listening' || reason.code === 'no_port') {
        if (ports.length <= 1) return { key: 'slim.reason.not_listening', values };
        return { key: reason.code === 'no_port' ? 'slim.reason.no_port_many' : 'slim.reason.not_listening_many', values };
    }
    return { key: `slim.reason.${reason.code}`, values };
}

const WHYS: readonly string[] = ['another_move', 'site_busy', 'kept_earlier', 'unsupported', 'update_running', 'update_unreadable', 'update_finishing', 'build'];

/** Why a whole move did not run, as a catalogue key (the general sentence for every other code). */
export function moveWhyKey(reason: MoveReason | undefined): string {
    return reason && WHYS.includes(reason.code) ? `slim.why.${reason.code}` : 'slim.why.other';
}

export interface MoveAnswer {
    done: boolean;
    rolled_back?: boolean;
    /** The move's time ran out: the programs it moved stay moved; the next move continues. */
    time_up?: boolean;
    /** The put-back did not complete: the record is kept and no site is moved until it is settled. */
    kept?: boolean;
    units: MoveOutcome[];
    /** The agent's English words (journal and audit record only). */
    error?: string;
    reason?: MoveReason;
    /** Second fix round (R8): put back by Try again, running where they ran, listening on no port. */
    runs_no_port?: string[];
}

export function decodeMoveAnswer(raw: unknown): MoveAnswer {
    const body = raw as Partial<MoveAnswer> | null;
    if (!body || typeof body !== 'object' || typeof body.done !== 'boolean' || !Array.isArray(body.units)) throw new Error('shape');
    return {
        done: body.done, rolled_back: body.rolled_back === true, time_up: body.time_up === true, kept: body.kept === true,
        units: body.units.filter((u) => !!u && typeof u.unit === 'string'), error: body.error,
        reason: body.reason && typeof body.reason.code === 'string' ? body.reason : undefined,
        runs_no_port: Array.isArray(body.runs_no_port) ? body.runs_no_port.filter((u): u is string => typeof u === 'string') : [],
    };
}

/**
 * After a failed move, the programs confirmed back where they ran and running again, the ones put back
 * as they were (they did not run before the move; fix round W1), and the ones whose put-back did not
 * complete: "put back" is said only of the first two (review W1).
 */
export function putBackSplit(answer: MoveAnswer): { back: string[]; asWere: string[]; left: string[] } {
    const back: string[] = [];
    const asWere: string[] = [];
    const left: string[] = [];
    for (const u of answer.units) {
        if (u.outcome !== 'failed' && u.outcome !== 'put_back') continue;
        (!u.put_back ? left : u.ran === true ? back : asWere).push(u.unit);
    }
    return { back, asWere, left };
}

/** A kept record's state of one program (alpha.149: unverified, not_answering). */
export type KeptState = 'failed' | 'unknown' | 'starting' | 'waiting' | 'unverified' | 'not_answering';
const KEPT: readonly string[] = ['failed', 'unknown', 'starting', 'waiting', 'unverified', 'not_answering'];

/** The programs a kept move record left, with their state: failed (put back, not running), unknown, starting, waiting (not restarted yet), unverified, not_answering. */
export function keptUnits(view: SiteLimitsView): { unit: string; state: KeptState }[] {
    const record = view.move_interrupted;
    if (!record?.kept) return [];
    return Object.entries(record.units ?? {})
        .filter((e): e is [string, KeptState] => KEPT.includes(e[1]))
        .map(([unit, state]) => ({ unit, state }))
        .sort((a, b) => a.unit.localeCompare(b.unit));
}

/** How long the move waits for each program at most (systemd's start timeout of these units). */
export const MOVE_WAIT_SECONDS = 90;

/** One step of the move's progress window, before it is worded. */
export interface MoveStep {
    id: string;
    label: string;
    state: 'done' | 'pending' | 'failed' | 'waiting';
    note?: string;
}

/**
 * The move's steps once the server answered: the limits written, one step per
 * program that ran outside them, and the check. A program put back after a
 * later one failed is a failed step; one not tried stays pending.
 */
export function moveSteps(answer: MoveAnswer, units: SiteLimitUnit[], words: { write: string; restart: string; check: string; reason: (r: MoveReason) => string; why: string }): MoveStep[] {
    const label = (unit: string) => units.find((u) => u.unit === unit)?.label ?? unit;
    const wrote = answer.done || answer.units.length > 0;
    const steps: MoveStep[] = [{ id: 'write', label: words.write, state: wrote ? 'done' : 'failed', note: wrote ? undefined : words.why }];
    for (const u of answer.units) {
        if (u.outcome === 'absent' || u.outcome === 'refused') continue;
        let state: MoveStep['state'] = 'done';
        if (u.outcome === 'failed' || u.outcome === 'put_back') state = 'failed';
        else if (u.outcome === 'not_tried') state = 'pending';
        else if (notMovedReason(u.outcome)) state = 'waiting';
        // alpha.149 (lab D7): the reason in the page's words, never the agent's English detail.
        steps.push({ id: u.unit, label: `${words.restart}: ${label(u.unit)}`, state, note: state === 'failed' && u.reason ? words.reason(u.reason) : undefined });
    }
    steps.push({ id: 'check', label: words.check, state: answer.done ? 'done' : 'failed' });
    return steps;
}

/** A request id the server accepts (8 to 64 letters, digits and dashes). */
export function newMoveRequestID(): string {
    const bytes = new Uint8Array(12);
    crypto.getRandomValues(bytes);
    return 'move-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}
