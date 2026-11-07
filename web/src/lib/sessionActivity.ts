// The "the person is here" signal of the idle timeout (alpha.128, G4).
//
// The server ends a session that was not used for the administrator's idle
// timeout. Reads that screens make by themselves (lists refreshing, alert
// counts) must not keep a forgotten browser signed in, so the server counts
// only changes and this signal. It is sent at most once a minute, and only
// after the person typed, clicked, scrolled or touched the page. It carries
// nothing and changes nothing; a lost signal is not retried.
//
// "Kişi burada" işareti: yalnız kişi yazınca, tıklayınca ya da dokununca,
// dakikada en çok bir kez gönderilir. Kendiliğinden yenilenen okumalar
// oturumu açık tutmaz.

export const SESSION_ACTIVITY_URL = '/api/v1/auth/activity';
const INTERVAL_MS = 60_000;
const EVENTS = ['pointerdown', 'keydown', 'wheel', 'touchstart'] as const;

let installed = false;
let lastSent = 0;

/** Starts listening once per page; later calls do nothing. */
export function startSessionActivity(): void {
    if (installed || typeof window === 'undefined' || typeof window.addEventListener !== 'function') return;
    installed = true;
    // A lost signal is not retried: the next interaction sends another.
    const settled = () => undefined;
    const note = () => {
        const now = Date.now();
        if (now - lastSent < INTERVAL_MS || typeof fetch !== 'function') return;
        lastSent = now;
        fetch(SESSION_ACTIVITY_URL, { method: 'POST', credentials: 'same-origin', keepalive: true }).then(settled, settled);
    };
    for (const name of EVENTS) window.addEventListener(name, note, { passive: true, capture: true });
}
