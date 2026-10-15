// A signed self-update worker has its own durable systemd lifecycle. The
// browser must not hold the whole application hostage while that independent
// recovery path is proving a result. This deadline releases navigation only;
// the canonical update marker remains authoritative and continues to disable
// every competing update action.
export const SYSTEM_UPDATE_BLOCKING_DEADLINE_MS = 2 * 60_000;

// The update's own restart (native record set17b, finding F1, 2026-10-15):
// the lock ended by the two-minute lease at T+120 s in the middle of a Panel
// outage that lasted until T+194 to 203 s. When the update tracking lost its
// read of the followed update before the lease ended, and the server had said
// the update is active, the lock is kept until the tracking reaches the Panel
// again or the update's restart window ends (lib/systemUpdateTracking.ts:
// 6 minutes from the start, at least 2 minutes from the first lost read),
// whichever comes first. This limit is the hard bound of any lock: the same
// 6 minutes as SYSTEM_UPDATE_EXPECTED_RESTART_MS (a test keeps them equal);
// this module imports nothing so that it stays usable on its own.
//
// Guncellemenin kendi yeniden baslamasi (set17b F1): izleme, kira bitmeden
// once okumasini kaybettiyse kilit, Panel yeniden yanit verene ya da yeniden
// baslama penceresi bitene kadar surer; hangisi once gelirse. Ust sinir
// baslangictan 6 dakikadir.
export const SYSTEM_UPDATE_RESTART_NAVIGATION_LIMIT_MS = 6 * 60_000;

// The followed update's restart as the navigation lease needs it: the first
// lost read of the current outage and the end of the restart window, both on
// this browser's clock (D-032).
export type SystemUpdateNavigationRestart = { lostSince: number; until: number };

// When the navigation lock of an update created at createdAt ends. Without a
// restart it is the two-minute lease, unchanged. A restart extends it only when
// its first lost read falls inside the lease (a restart that begins after the
// lease never brings a released lock back, and a lease whose timer was late
// is not stretched by it), and never past the 6-minute limit.
export function systemUpdateNavigationDeadline(
    createdAt: number,
    restart: SystemUpdateNavigationRestart | null,
): number {
    const lease = createdAt + SYSTEM_UPDATE_BLOCKING_DEADLINE_MS;
    if (!restart || !Number.isFinite(lease)
        || !Number.isFinite(restart.lostSince) || !Number.isFinite(restart.until)) return lease;
    if (restart.lostSince < createdAt || restart.lostSince >= lease || restart.until <= lease) return lease;
    return Math.min(restart.until, createdAt + SYSTEM_UPDATE_RESTART_NAVIGATION_LIMIT_MS);
}

export const SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY = 'celikpanel.system-update-navigation-release.v1';

export type SystemUpdateNavigationReleaseStorage = Pick<Storage, 'getItem' | 'setItem'>;

type SystemUpdatePageLockApplication = Pick<HTMLElement, 'inert' | 'setAttribute' | 'removeAttribute'>;
type SystemUpdatePageLockWindow = {
    addEventListener: (type: 'beforeunload', listener: (event: BeforeUnloadEvent) => void) => void;
    removeEventListener: (type: 'beforeunload', listener: (event: BeforeUnloadEvent) => void) => void;
};
type SystemUpdatePageLockDocument = { body: { style: { overflow: string } } };

export function acquireSystemUpdatePageLock(
    application: SystemUpdatePageLockApplication | null,
    programmaticReload: () => boolean,
    targetWindow: SystemUpdatePageLockWindow = window,
    targetDocument: SystemUpdatePageLockDocument = document,
): () => void {
    const previousOverflow = targetDocument.body.style.overflow;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
        if (programmaticReload()) return;
        event.preventDefault();
        event.returnValue = '';
    };
    if (application) {
        application.inert = true;
        application.setAttribute('aria-busy', 'true');
    }
    targetDocument.body.style.overflow = 'hidden';
    targetWindow.addEventListener('beforeunload', onBeforeUnload);
    return () => {
        targetWindow.removeEventListener('beforeunload', onBeforeUnload);
        if (application) {
            application.inert = false;
            application.removeAttribute('aria-busy');
        }
        targetDocument.body.style.overflow = previousOverflow;
    };
}

export function systemUpdateNavigationBlocked(
    createdAt: number,
    now: number,
): boolean {
    // An invalid or future wall-clock value must never create an unbounded
    // navigation lock. Returning background is fail-safe because callers keep
    // the exact operation marker and mutation gate intact.
    return Number.isFinite(createdAt) && createdAt > 0
        && Number.isFinite(now) && now >= createdAt
        && now - createdAt < SYSTEM_UPDATE_BLOCKING_DEADLINE_MS;
}

export function systemUpdateGlobalNavigationBlocked(
    operationBlocks: boolean,
    navigationLeaseBlocked: boolean,
): boolean {
    // Canonical uncertainty is deliberately absent here. Callers keep update
    // mutations fail-closed through their independent `occupied` gate, while
    // this short-lived lease controls only whole-application navigation.
    return operationBlocks && navigationLeaseBlocked;
}

export function systemUpdateMutationLocked(
    canonicalReady: boolean,
    operationPresent: boolean,
): boolean {
    // Navigation is a bounded convenience lease; mutation authority is not.
    // Unknown canonical state and every exact in-flight/terminal record remain
    // fail-closed until the canonical record is proven and acknowledged.
    return !canonicalReady || operationPresent;
}

export function systemUpdateExactTrackingAllowed(
    operationPresent: boolean,
    canonicalReady: boolean,
    navigationReleased: boolean,
): boolean {
    return operationPresent && (canonicalReady || navigationReleased);
}

export function systemUpdateExactStatusPath(requestID: string): string {
    return `/api/v1/panel/update/status?request_id=${encodeURIComponent(requestID)}`;
}

export function systemUpdateNavigationReleasedInStorage(
    storage: SystemUpdateNavigationReleaseStorage | null,
    identity: string,
): boolean {
    if (!storage || !identity) return false;
    try {
        return storage.getItem(SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY) === identity;
    } catch {
        return false;
    }
}

export function rememberSystemUpdateNavigationRelease(
    storage: SystemUpdateNavigationReleaseStorage | null,
    identity: string,
): void {
    if (!storage || !identity) return;
    try {
        storage.setItem(SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY, identity);
    } catch {
        // Navigation release is a bounded browser-side usability lease. A
        // storage failure must never change the canonical mutation lock.
    }
}
