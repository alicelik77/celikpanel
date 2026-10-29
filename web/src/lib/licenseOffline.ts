/**
 * What the license page and the dashboard say about the license check itself
 * (D-024, D-039): the license service may be unreachable while the last
 * successful check still keeps management open for up to seven days.
 *
 * The server states durations measured on its own clock at the moment it
 * answered (`offline_for_seconds`, `verified_seconds_ago`); the page adds them
 * to the moment it received the answer, so it never compares its clock with
 * the server's (D-032).
 *
 * Lisans sayfasının ve panonun lisans kontrolü hakkında söyledikleri: lisans
 * hizmetine ulaşılamazken son başarılı kontrol yönetimi en çok yedi gün açık
 * tutar. Süreler sunucunun saatiyle gelir; sayfa onları yanıtı aldığı ana ekler.
 */
import type { TranslationKey } from '../i18n/en';

export const CHECKS = ['unreachable', 'rate_limited', 'invalid_answer', 'clock_behind'] as const;
export const REASONS = ['offline_ended', 'clock_behind'] as const;
export type LicenseCheck = (typeof CHECKS)[number];
export type LicenseReason = (typeof REASONS)[number];

/** In the last 48 hours of the offline window the owner is told to act. */
export const OFFLINE_WARNING_SECONDS = 48 * 3600;

export type LicenseCheckFacts = {
    state: string;
    can_provision: boolean;
    check?: LicenseCheck;
    reason?: LicenseReason;
    offline_for_seconds?: number;
    verified_seconds_ago?: number;
    /** Date.now() when this answer arrived. */
    receivedAt: number;
};

export type LicenseGuidance = {
    tone: 'info' | 'warning' | 'blocked';
    title: TranslationKey;
    help: TranslationKey;
    /** Dashboard sentence; the license page shows `help`. */
    notice: TranslationKey;
};

const seconds = (value: unknown) => typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : undefined;

/** The optional check fields of a license status answer; unknown values are dropped, not guessed. */
export function decodeLicenseCheck(raw: Record<string, unknown>, receivedAt = Date.now()) {
    return {
        check: CHECKS.includes(raw.check as LicenseCheck) ? raw.check as LicenseCheck : undefined,
        reason: REASONS.includes(raw.reason as LicenseReason) ? raw.reason as LicenseReason : undefined,
        offline_for_seconds: seconds(raw.offline_for_seconds),
        verified_seconds_ago: seconds(raw.verified_seconds_ago),
        receivedAt,
    };
}

/**
 * Null when there is nothing to say beyond the license state: the last check
 * succeeded, or the state is one the existing copy already explains.
 */
export function licenseCheckGuidance(facts: LicenseCheckFacts): LicenseGuidance | null {
    if (facts.state === 'verification_unavailable') {
        if (facts.reason === 'clock_behind') return { tone: 'blocked', title: 'license.clockBehind.title', help: 'license.clockBehind.help', notice: 'license.clockBehind.help' };
        if (facts.reason === 'offline_ended') return { tone: 'blocked', title: 'license.offlineEnded.title', help: 'license.offlineEnded.help', notice: 'license.offlineEnded.help' };
        return null;
    }
    if (facts.state !== 'active' || !facts.can_provision || !facts.check) return null;
    if (facts.check === 'clock_behind') return { tone: 'warning', title: 'license.clock.title', help: 'license.clock.help', notice: 'license.clock.help' };
    if ((facts.offline_for_seconds ?? 0) <= OFFLINE_WARNING_SECONDS) {
        return { tone: 'warning', title: 'license.offline.soonTitle', help: 'license.offline.soonHelp', notice: 'license.noticeOfflineSoon' };
    }
    if (facts.check === 'rate_limited') return { tone: 'info', title: 'license.offline.waitTitle', help: 'license.offline.waitHelp', notice: 'license.noticeOffline' };
    if (facts.check === 'invalid_answer') return { tone: 'info', title: 'license.offline.answerTitle', help: 'license.offline.answerHelp', notice: 'license.noticeOffline' };
    return { tone: 'info', title: 'license.offline.title', help: 'license.offline.help', notice: 'license.noticeOffline' };
}

/** {until}: end of the offline window; {since}: the last successful check. */
export function licenseCheckVars(facts: LicenseCheckFacts, locale: string): Record<string, string> {
    const format = (at: number) => new Date(at).toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-US', { dateStyle: 'medium', timeStyle: 'short' });
    const vars: Record<string, string> = {};
    if (facts.offline_for_seconds !== undefined) vars.until = format(facts.receivedAt + facts.offline_for_seconds * 1000);
    if (facts.verified_seconds_ago !== undefined) vars.since = format(facts.receivedAt - facts.verified_seconds_ago * 1000);
    return vars;
}
