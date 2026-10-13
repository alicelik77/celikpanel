export type LicenseState = 'active' | 'missing' | 'expired' | 'invalid' | 'verification_unavailable' | 'status_unavailable';
/** `until` is a deadline on THIS browser's clock, in seconds. */
export type AccessObservation = { allowed: boolean | null; until: number; state: LicenseState };

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
 */
export function parseAccessObservation(value: unknown, now = Date.now() / 1000, observedAt = now): AccessObservation {
    if (!value || typeof value !== 'object') throw new Error('invalid access observation');
    const result = value as Record<string, unknown>;
    if (typeof result.can_use_panel !== 'boolean' || !Number.isSafeInteger(result.valid_until)) throw new Error('invalid access observation');
    const validFor = result.valid_for_seconds;
    const until = typeof validFor === 'number' && Number.isFinite(validFor) ? observedAt + validFor : result.valid_until as number;
    if (result.can_use_panel) {
        if (until <= now || (result.state !== undefined && result.state !== 'active')
            || (result.observation !== undefined && result.observation !== 'known')) throw new Error('invalid positive access');
        return { allowed: true, until, state: 'active' };
    }
    if (result.observation === 'known' && ['missing', 'expired', 'invalid'].includes(String(result.state))) {
        return { allowed: false, until: 0, state: result.state as LicenseState };
    }
    return { allowed: null, until: 0, state: result.state === 'verification_unavailable' ? 'verification_unavailable' : 'status_unavailable' };
}
