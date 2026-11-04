// The server-side "remove together" operation (alpha.123, D-045), followed by
// the removal window. The Panel answers 202 with a durable operation; this
// reads it by its id until it ends. Reading never starts anything: the
// removal goes on on the server whether this page stays open or not.
// Sunucudaki birlikte kaldırma işlemi; pencere yalnız kimliğiyle okur.

export function newRemovalRequestID(): string {
    const bytes = new Uint8Array(16);
    globalThis.crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export interface RemovalOperation { id: string; status: string; phase: string; removed: string[]; error?: { code?: string; message?: string } }

const isStrings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === 'string');

export function decodeRemovalOperation(value: unknown): RemovalOperation | null {
    const op = (value as { operation?: Record<string, unknown> } | null)?.operation;
    if (!op || typeof op.id !== 'string' || typeof op.status !== 'string') return null;
    const result = op.result as { removed?: unknown } | undefined;
    const error = op.error as { code?: unknown; message?: unknown } | undefined;
    return {
        id: op.id, status: op.status, phase: typeof op.phase === 'string' ? op.phase : '',
        removed: isStrings(result?.removed) ? result.removed : [],
        ...(error && typeof error === 'object' ? { error: { code: typeof error.code === 'string' ? error.code : undefined, message: typeof error.message === 'string' ? error.message : undefined } } : {}),
    };
}

export const REMOVAL_POLL_MS = 1500;
// After this many unread answers in a row the page stops following and says
// what it last knew; the operation itself goes on and Components shows it.
const REMOVAL_POLL_MISSES = 200;

// followRemovalOperation reads the operation until it ends. A read that fails
// is tried again after the same pause.
export async function followRemovalOperation(first: unknown, onUpdate: (op: RemovalOperation) => void, wait = (ms: number) => new Promise((r) => setTimeout(r, ms))): Promise<RemovalOperation | null> {
    const firstOp = decodeRemovalOperation(first);
    if (!firstOp) return null;
    let op: RemovalOperation = firstOp;
    onUpdate(op);
    let misses = 0;
    while (op.status === 'queued' || op.status === 'running') {
        await wait(REMOVAL_POLL_MS);
        let next: RemovalOperation | null = null;
        try {
            const res: Response = await fetch(`/api/v1/service/operation?id=${encodeURIComponent(op.id)}`, { cache: 'no-store' });
            if (res.ok) {
                next = decodeRemovalOperation(await res.json());
            } else {
                // An answer that is not the operation is a miss, read again.
                next = null;
            }
        } catch (error) {
            next = null;
            void error;
        }
        if (next === null) {
            misses += 1;
            if (misses > REMOVAL_POLL_MISSES) return op;
            continue;
        }
        misses = 0;
        op = next;
        onUpdate(op);
    }
    return op;
}

type Say = (key: 'services.operation.phase.removing' | 'services.operation.phase.removeSetting' | 'services.operation.phase.syncing' | 'services.operation.starting', vars?: Record<string, string>) => string;

// The phase a running removal is in, said plainly.
export function removalPhaseText(t: Say, phase: string, nameOf: (id: string) => string): string {
    if (phase.startsWith('removing:')) return t('services.operation.phase.removing', { name: nameOf(phase.slice(9)) });
    if (phase === 'setting:malware_schedule') return t('services.operation.phase.removeSetting');
    if (phase.startsWith('firewall')) return t('services.operation.phase.syncing');
    return t('services.operation.starting');
}
