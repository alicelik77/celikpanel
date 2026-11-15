/**
 * The warning before the licence term ends (D-047, stage 1): 30, 7 and 1 day
 * before, for the administrator only. The term's end comes from the access
 * answer as time left on the server's clock, dated on this browser's clock when
 * it was asked (accessObservation), so the two clocks are never compared
 * (D-032).
 *
 * A closed notice stays closed until the next threshold; this browser remembers
 * that (a per-viewer convenience: if the browser forgets, the notice shows
 * again, nothing else changes). After a renewal the term ends more than 30 days
 * away and the notice is gone by itself.
 *
 * Lisans süresi bitmeden önceki uyarı: 30, 7 ve 1 gün kala, yalnız yöneticiye.
 * Kapatılan uyarı bir sonraki eşiğe kadar kapalı kalır; yenilemeden sonra
 * kendiliğinden kaybolur.
 */
export type ExpiryThreshold = 30 | 7 | 1;

const DAY = 86400;

/** The threshold reached now; null while more than 30 days are left or after the end. */
export function licenseExpiryThreshold(termEndsAt: number | undefined, now = Date.now() / 1000): ExpiryThreshold | null {
    if (termEndsAt === undefined) return null;
    const left = termEndsAt - now;
    if (left <= 0) return null;
    if (left <= DAY) return 1;
    if (left <= 7 * DAY) return 7;
    if (left <= 30 * DAY) return 30;
    return null;
}

/** {date}: the end in the reader's language; {days}: whole days left, rounded up. */
export function licenseExpiryVars(termEndsAt: number, locale: string, now = Date.now() / 1000): { date: string; days: string } {
    const date = new Date(termEndsAt * 1000).toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-US', { dateStyle: 'long', timeStyle: 'short' });
    return { date, days: String(Math.max(1, Math.ceil((termEndsAt - now) / DAY))) };
}

const DISMISSED_KEY = 'celikpanel.license-expiry-notice.v1';
/** A closed notice older than this belongs to an earlier term. */
const DISMISSAL_LIFETIME_MS = 40 * DAY * 1000;

export function licenseExpiryDismissed(threshold: ExpiryThreshold, now = Date.now()): boolean {
    try {
        const raw = localStorage.getItem(DISMISSED_KEY);
        if (!raw) return false;
        const stored = JSON.parse(raw) as { threshold?: unknown; at?: unknown };
        return stored.threshold === threshold && typeof stored.at === 'number' && now - stored.at >= 0 && now - stored.at < DISMISSAL_LIFETIME_MS;
    } catch {
        return false;
    }
}

export function dismissLicenseExpiry(threshold: ExpiryThreshold, now = Date.now()): void {
    try {
        localStorage.setItem(DISMISSED_KEY, JSON.stringify({ threshold, at: now }));
    } catch {
        // Not remembered: the notice shows again on the next visit.
    }
}
