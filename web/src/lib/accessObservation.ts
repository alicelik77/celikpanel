export type LicenseState = 'active' | 'missing' | 'expired' | 'invalid' | 'verification_unavailable' | 'status_unavailable';
/** Why verification is unavailable, when the server named it (D-039). */
export type LicenseLapseReason = 'offline_ended' | 'clock_behind';
/**
 * `until` is a deadline on THIS browser's clock, in seconds. `limited`: the panel
 * is usable while the licence does not allow full use (D-047, stage 1): creating
 * new things and changing settings are refused, everything else works, and there
 * is no deadline. `termEndsAt` (administrators only) is when the licence term
 * ends, on this browser's clock, in seconds.
 */
export type AccessObservation = {
    allowed: boolean | null; until: number; state: LicenseState; reason?: LicenseLapseReason;
    limited?: true; termEndsAt?: number;
};

/**
 * A negative boolean alone is not evidence of a missing or invalid license.
 *
 * The deadline follows the server's clock rule (D-032): the server states the
 * time left (`valid_for_seconds`, measured on its own clock when it answered)
 * and the browser adds it to `observedAt`, the moment on its own clock when it
 * asked. That moment is not later than the server's measurement, so the
 * deadline is never later than the server allows. The browser never compares
 * its clock with the server's: a server clock behind the browser's no longer
 * makes every positive answer look expired (set14b D2: 418 s behind, no page
 * loaded), and a server clock ahead no longer makes access look valid longer
 * than the server says. An older server without the field, or a value that is
 * not a finite number, keeps the absolute `valid_until` rule.
 *
 * Son an sunucunun saat kuralını izler (D-032): sunucu kalan süreyi kendi
 * saatiyle söyler (`valid_for_seconds`), tarayıcı bunu kendi saatiyle sorduğu
 * ana (`observedAt`) ekler. O an sunucunun ölçümünden sonra olamaz; son an
 * sunucunun izninden geç olmaz. Tarayıcı kendi saatini sunucununkiyle
 * karşılaştırmaz: geride kalan sunucu saati olumlu yanıtı süresi dolmuş
 * göstermez, ileride olan da erişimi sunucunun söylediğinden uzun göstermez.
 * Alanı göndermeyen eski sunucuda ya da sonlu sayı olmayan değerde mutlak
 * `valid_until` kuralı sürer.
 *
 * An answer without full use is not a lock (D-047): with `level: "limited"` the
 * pages mount and the server refuses only what needs full use. An answer that is
 * neither is unknown, never a verdict.
 */
export function parseAccessObservation(value: unknown, now = Date.now() / 1000, observedAt = now): AccessObservation {
    if (!value || typeof value !== 'object') throw new Error('invalid access observation');
    const result = value as Record<string, unknown>;
    if (typeof result.can_use_panel !== 'boolean' || !Number.isSafeInteger(result.valid_until)) throw new Error('invalid access observation');
    const validFor = result.valid_for_seconds;
    const until = typeof validFor === 'number' && Number.isFinite(validFor) ? observedAt + validFor : result.valid_until as number;
    const termLeft = result.term_ends_in_seconds;
    const term = typeof termLeft === 'number' && Number.isFinite(termLeft) && termLeft >= 0 ? { termEndsAt: observedAt + termLeft } : {};
    if (result.can_use_panel) {
        if (until <= now || (result.state !== undefined && result.state !== 'active')
            || (result.observation !== undefined && result.observation !== 'known')
            || (result.level !== undefined && result.level !== 'full')) throw new Error('invalid positive access');
        return { allowed: true, until, state: 'active', ...term };
    }
    // The panel answered with a licence state that does not allow full use:
    // its pages are usable at the limited level, without a deadline.
    // Panel, tam kullanıma izin vermeyen bir lisans durumuyla yanıt verdi:
    // sayfalar sınırlı düzeyde, süresiz kullanılır.
    const known = ['missing', 'expired', 'invalid'].includes(String(result.state)) && result.observation === 'known';
    const unknown = ['verification_unavailable', 'status_unavailable'].includes(String(result.state)) && result.observation === 'unavailable';
    if (result.level === 'limited' && (known || unknown)) {
        const reason: { reason?: LicenseLapseReason } = result.state === 'verification_unavailable' && (result.reason === 'offline_ended' || result.reason === 'clock_behind') ? { reason: result.reason } : {};
        return { allowed: true, until: Number.POSITIVE_INFINITY, state: result.state as LicenseState, limited: true, ...reason, ...term };
    }
    // Not the contract: unknown, never a licence verdict.
    return { allowed: null, until: 0, state: result.state === 'verification_unavailable' ? 'verification_unavailable' : 'status_unavailable' };
}
