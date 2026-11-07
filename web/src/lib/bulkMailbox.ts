import type { OperationStep } from './confirmAction';

// Bulk mailbox actions (alpha.128, gap G20): one confirmed operation, one
// real server call per mailbox, one step per mailbox in the progress window.
// A mailbox the server refused is said with its reason and the next ones go
// on (each mailbox is its own change); a call whose answer never arrived
// stops the run there, because whether it happened is not known and nothing
// is sent blindly after it (D-024).
//
// Toplu posta kutusu işlemleri: her posta kutusu için bir sunucu çağrısı ve
// bir adım. Reddedilen kutu nedeniyle söylenir, sonrakiler sürer; yanıtı
// gelmeyen çağrıda iş durur.

export interface BulkTarget {
    id: number;
    label: string;
}

export type BulkOutcome = { ok: true } | { ok: false; reason: string } | { unknown: true };

export interface BulkResult {
    done: number[];
    refused: { id: number; reason: string }[];
    unknown: number | null;
    notStarted: number[];
}

export interface BulkWords {
    stepLabel: (target: BulkTarget) => string;
    unknownNote: string;
    notStartedNote: string;
}

/** Runs `run` for each target in order, reporting a step per target. */
export async function runBulk(
    targets: BulkTarget[],
    run: (target: BulkTarget) => Promise<BulkOutcome>,
    report: (steps: OperationStep[]) => void,
    words: BulkWords,
    now: () => number = Date.now,
): Promise<BulkResult> {
    const steps: OperationStep[] = targets.map((target) => ({ id: String(target.id), label: words.stepLabel(target), state: 'pending' }));
    const result: BulkResult = { done: [], refused: [], unknown: null, notStarted: [] };
    const show = () => report(steps.map((step) => ({ ...step })));
    for (let index = 0; index < targets.length; index++) {
        const target = targets[index];
        steps[index] = { ...steps[index], state: 'running', since: now() };
        show();
        let outcome: BulkOutcome;
        try {
            outcome = await run(target);
        } catch {
            outcome = { unknown: true };
        }
        if ('unknown' in outcome) {
            steps[index] = { ...steps[index], state: 'waiting', note: words.unknownNote };
            result.unknown = target.id;
            for (let rest = index + 1; rest < targets.length; rest++) {
                steps[rest] = { ...steps[rest], state: 'pending', note: words.notStartedNote };
                result.notStarted.push(targets[rest].id);
            }
            show();
            return result;
        }
        if (outcome.ok) {
            steps[index] = { ...steps[index], state: 'done' };
            result.done.push(target.id);
        } else {
            steps[index] = { ...steps[index], state: 'failed', note: outcome.reason };
            result.refused.push({ id: target.id, reason: outcome.reason });
        }
        show();
    }
    return result;
}
