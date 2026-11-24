// The limits of a site's programs together (S4 R2a): the answer of
// GET /api/v1/domains/{id}/site-limits and what the card says of it. Pure,
// so the card and the tests read the same rules.
//
// Bir sitenin programlarının birlikte sınırları: sunucunun yanıtı ve kartın
// ondan söyledikleri.

export type SiteLimitKey = 'site.memory_mb' | 'site.tasks' | 'site.cpu_percent';

export interface SiteLimitValue {
    key: SiteLimitKey;
    value: number;
    set_by: string;
    set_by_name?: string;
}

export interface SiteLimitEvent {
    /** memory, tasks: the site's own limit; *_other: a program of the site, not proved at the site's limit. */
    kind: 'memory' | 'tasks' | 'cpu' | 'memory_other' | 'tasks_other';
    count: number;
    first_at: string;
    last_at: string;
    repeated?: boolean;
}

export interface SiteLimitUnit {
    unit: string;
    kind: string;
    label: string;
    placement: 'in_slice' | 'outside' | 'stopped' | 'stopped_outside' | 'elsewhere' | 'absent';
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
    move_interrupted?: { at: string; units: Record<string, string>; kept?: boolean };
    db_owner?: { user: string; key: string; value: string }[];
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
}

const KEYS: readonly string[] = ['site.memory_mb', 'site.tasks', 'site.cpu_percent'];
const EVENTS: readonly string[] = ['memory', 'tasks', 'cpu', 'memory_other', 'tasks_other'];
const PLACES: readonly string[] = ['in_slice', 'outside', 'stopped', 'stopped_outside', 'elsewhere', 'absent'];

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

/** What happened to one unit of a move, for the sentence that lists the ones not moved. */
export interface MoveOutcome {
    unit: string;
    outcome: string;
    step?: string;
    put_back?: boolean;
    detail?: string;
}

export interface MoveAnswer {
    done: boolean;
    rolled_back?: boolean;
    /** The move's time ran out: the programs it moved stay moved; the next move continues. */
    time_up?: boolean;
    /** The put-back did not complete: the record is kept and no site is moved until it is settled. */
    kept?: boolean;
    units: MoveOutcome[];
    error?: string;
}

export function decodeMoveAnswer(raw: unknown): MoveAnswer {
    const body = raw as Partial<MoveAnswer> | null;
    if (!body || typeof body !== 'object' || typeof body.done !== 'boolean' || !Array.isArray(body.units)) throw new Error('shape');
    return {
        done: body.done, rolled_back: body.rolled_back === true, time_up: body.time_up === true, kept: body.kept === true,
        units: body.units.filter((u) => !!u && typeof u.unit === 'string'), error: body.error,
    };
}

/**
 * After a failed move, the programs confirmed back where they ran and the ones whose put-back did not
 * complete: "put back" is said only of the first (review W1).
 */
export function putBackSplit(answer: MoveAnswer): { back: string[]; left: string[] } {
    const back: string[] = [];
    const left: string[] = [];
    for (const u of answer.units) {
        if (u.outcome !== 'failed' && u.outcome !== 'put_back') continue;
        (u.put_back ? back : left).push(u.unit);
    }
    return { back, left };
}

/** The programs a kept move record left, with their state: failed (stopped), unknown, starting, waiting (not restarted yet). */
export function keptUnits(view: SiteLimitsView): { unit: string; state: 'failed' | 'unknown' | 'starting' | 'waiting' }[] {
    const record = view.move_interrupted;
    if (!record?.kept) return [];
    return Object.entries(record.units ?? {})
        .filter((e): e is [string, 'failed' | 'unknown' | 'starting' | 'waiting'] => ['failed', 'unknown', 'starting', 'waiting'].includes(e[1]))
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
export function moveSteps(answer: MoveAnswer, units: SiteLimitUnit[], words: { write: string; restart: string; check: string }): MoveStep[] {
    const label = (unit: string) => units.find((u) => u.unit === unit)?.label ?? unit;
    const wrote = answer.done || answer.units.length > 0;
    const steps: MoveStep[] = [{ id: 'write', label: words.write, state: wrote ? 'done' : 'failed', note: wrote ? undefined : answer.error }];
    for (const u of answer.units) {
        if (u.outcome === 'absent' || u.outcome === 'refused') continue;
        let state: MoveStep['state'] = 'done';
        if (u.outcome === 'failed' || u.outcome === 'put_back') state = 'failed';
        else if (u.outcome === 'not_tried') state = 'pending';
        else if (notMovedReason(u.outcome)) state = 'waiting';
        steps.push({ id: u.unit, label: `${words.restart}: ${label(u.unit)}`, state, note: state === 'failed' ? u.detail : undefined });
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
