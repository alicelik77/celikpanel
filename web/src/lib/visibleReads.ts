// Automatic reads that run only while the page is visible (D-033 rule 6,
// D-032: no timer that runs when nobody is looking). Used by the component
// install dialog's readiness re-read and the setup page's operation reads,
// the same rule the update card and the spreading-domain checks follow.
//
// - While the tab is hidden no read runs and the interval is stopped.
// - When the tab is shown again, one read runs at once, then the interval
//   resumes.
// - Never two reads at once: a tick that finds a read in flight is skipped.
// - Reads only. The caller's read must be a GET; nothing here starts or
//   admits a change.
// - Without a document (server-side or a test without one) the page counts as
//   visible, so behaviour is unchanged there.
//
// Gorunur sayfada otomatik okumalar: gizliyken okuma yok; yeniden
// gorununce bir okuma hemen, sonra aralik surer. Ayni anda iki okuma yok.

export type VisibleReadTimers = {
    setInterval: (callback: () => void, ms: number) => number;
    clearInterval: (id: number) => void;
};

type VisibilitySource = Pick<Document, 'visibilityState' | 'addEventListener' | 'removeEventListener'>;

function visibilitySource(): VisibilitySource | null {
    try {
        if (typeof document === 'undefined' || !document || typeof document.addEventListener !== 'function') return null;
        return document;
    } catch {
        return null;
    }
}

export function startVisibleReads(
    read: () => unknown,
    everyMs: number,
    timers: VisibleReadTimers,
    options: { immediate?: boolean } = {},
): () => void {
    const source = visibilitySource();
    const visible = () => !source || source.visibilityState !== 'hidden';
    let stopped = false;
    let timer: number | undefined;
    let inFlight = false;
    const runRead = () => {
        if (stopped || inFlight) return;
        let result: unknown;
        try {
            result = read();
        } catch {
            return;
        }
        if (result && typeof (result as Promise<unknown>).then === 'function') {
            inFlight = true;
            (result as Promise<unknown>).then(
                () => { inFlight = false; },
                () => { inFlight = false; },
            );
        }
    };
    const startTimer = () => {
        if (stopped || timer !== undefined) return;
        timer = timers.setInterval(() => { if (visible()) runRead(); }, everyMs);
    };
    const stopTimer = () => {
        if (timer === undefined) return;
        timers.clearInterval(timer);
        timer = undefined;
    };
    const onVisibility = () => {
        if (stopped) return;
        if (visible()) {
            runRead();
            startTimer();
            return;
        }
        stopTimer();
    };
    source?.addEventListener('visibilitychange', onVisibility);
    if (options.immediate) runRead();
    if (visible()) startTimer();
    return () => {
        stopped = true;
        stopTimer();
        source?.removeEventListener('visibilitychange', onVisibility);
    };
}
