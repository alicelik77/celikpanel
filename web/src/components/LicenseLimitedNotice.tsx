import { BadgeCheck } from 'lucide-react';
import { Link, useLocation } from '../router';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { LicenseLapseReason, LicenseState } from '../lib/accessObservation';

/**
 * The notice on top of every page while the licence does not allow full use
 * (D-047, stage 1): why, what still works, what is closed, who acts and how it
 * continues (D-024). Only the administrator is told about the licence and gets
 * the license page; resellers and customers get one sentence each that never
 * names the licence or a date, because they cannot act on it.
 *
 * Lisans tam kullanıma izin vermezken her sayfanın üstündeki bildirim: neden,
 * ne çalışıyor, ne kapalı, kim ne yapar. Lisans yalnız yöneticiye anlatılır;
 * bayi ve müşteri lisansın adını ya da bir tarihi görmez.
 */
export function licenseLimitedWords(role: string, state: LicenseState, reason?: LicenseLapseReason): { title: TranslationKey; help: TranslationKey } {
    if (role === 'reseller') return { title: 'license.limited.tenantTitle', help: 'license.limited.reseller' };
    if (role !== 'admin') return { title: 'license.limited.tenantTitle', help: 'license.limited.customer' };
    if (state === 'verification_unavailable' && reason === 'offline_ended') return { title: 'license.offlineEnded.title', help: 'license.offlineEnded.help' };
    if (state === 'verification_unavailable' && reason === 'clock_behind') return { title: 'license.clockBehind.title', help: 'license.clockBehind.help' };
    if (state === 'missing') return { title: 'license.limited.missingTitle', help: 'license.limited.missing' };
    if (state === 'expired') return { title: 'license.limited.expiredTitle', help: 'license.limited.expired' };
    if (state === 'invalid') return { title: 'license.limited.invalidTitle', help: 'license.limited.invalid' };
    return { title: 'license.limited.unreadTitle', help: 'license.limited.unread' };
}

export function LicenseLimitedNotice({ state, reason }: { state: LicenseState; reason?: LicenseLapseReason }) {
    const { role } = useAuth();
    const { t } = useI18n();
    const location = useLocation();
    const onLicensePage = location.pathname === '/settings' && new URLSearchParams(location.search).get('section') === 'license';
    const words = licenseLimitedWords(role, state, reason);
    return <section role="status" aria-labelledby="license-limited-title" data-license-limited={state}
        className="border-b border-warning-mark/60 bg-warning-mark/20 px-4 py-3 text-sm text-fg sm:px-6">
        <div className="flex flex-wrap items-start gap-x-6 gap-y-2">
            <div className="flex min-w-0 flex-1 basis-80 items-start gap-3">
                <BadgeCheck className="mt-0.5 h-5 w-5 shrink-0 text-fg-muted" aria-hidden="true" />
                <div className="min-w-0">
                    <h2 id="license-limited-title" className="font-semibold">{t(words.title)}</h2>
                    <p className="mt-1 max-w-[110ch]">{t(words.help)}</p>
                </div>
            </div>
            {/* The license page itself needs no link to itself. */}
            {role === 'admin' && !onLicensePage && <Link to="/settings?section=license" className="shrink-0 font-medium text-primary underline underline-offset-4 sm:mt-0.5">
                {t('license.noticeOpen')}
            </Link>}
        </div>
    </section>;
}
