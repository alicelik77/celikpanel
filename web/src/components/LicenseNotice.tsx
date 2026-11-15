import { useState } from 'react';
import { BadgeCheck } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import { useRemote } from '../lib/remote';
import { useLicenseAccess } from '../lib/licenseAccess';
import { decodeLicenseCheck, licenseCheckGuidance, licenseCheckVars } from '../lib/licenseOffline';
import { dismissLicenseExpiry, licenseExpiryDismissed, licenseExpiryThreshold, licenseExpiryVars } from '../lib/licenseTerm';
import { Button } from './ui';

const STATES = ['active', 'missing', 'invalid', 'expired', 'verification_unavailable', 'status_unavailable'] as const;
type LicenseState = (typeof STATES)[number];

type LicenseStatus = { state: LicenseState; can_provision: boolean } & ReturnType<typeof decodeLicenseCheck>;

const LICENSE_URL = '/api/v1/panel/license';
const NO_STORE: RequestInit = { cache: 'no-store' };

// The answer is the contract or it is unknown. In particular an answer without
// `can_provision`, or with a state this build does not know, is not "a license
// is required".
// Yanıt ya sözleşmedir ya da bilinmeyendir. `can_provision` taşımayan ya da bu
// sürümün tanımadığı bir durum bildiren yanıt "lisans gerekli" değildir.
function decodeLicenseStatus(raw: unknown): LicenseStatus {
    if (!raw || typeof raw !== 'object') throw new Error('shape');
    const { state, can_provision } = raw as { state?: unknown; can_provision?: unknown };
    if (!STATES.includes(state as LicenseState) || typeof can_provision !== 'boolean') throw new Error('field');
    if (can_provision && state !== 'active') throw new Error('contradiction');
    return { state: state as LicenseState, can_provision, ...decodeLicenseCheck(raw as Record<string, unknown>) };
}

const NOTICE = 'mb-5 flex flex-wrap items-center justify-between gap-4 rounded-lg border p-4';

/**
 * Mounted only in the administrator dashboard. No tenant license request.
 *
 * While the license allows full use it says what needs the administrator: the
 * term ends within 30, 7 or 1 day (D-047), or the license service could not be
 * reached at the last check (D-039). A license that does not allow full use is
 * explained by the notice on top of every page (LicenseLimitedNotice), not
 * twice. While the license is being read, and when that read failed, it draws
 * nothing: the access gate around every page reads access on its own.
 *
 * Lisans tam kullanıma izin verirken yöneticinin yapacağını söyler: süre 30, 7
 * ya da 1 gün içinde bitiyor veya son kontrolde lisans hizmetine ulaşılamadı.
 * Tam kullanıma izin vermeyen lisansı her sayfanın üstündeki bildirim anlatır.
 */
export function LicenseNotice() {
    const { t, locale } = useI18n();
    const access = useLicenseAccess();
    const license = useRemote(LICENSE_URL, decodeLicenseStatus, { init: NO_STORE });
    const threshold = access && !access.limited ? licenseExpiryThreshold(access.termEndsAt) : null;
    const [closed, setClosed] = useState<number | null>(null);
    if (threshold !== null && access?.termEndsAt !== undefined && closed !== threshold && !licenseExpiryDismissed(threshold)) {
        const vars = licenseExpiryVars(access.termEndsAt, locale);
        const day = threshold === 1;
        return (
            <section className={`${NOTICE} ${threshold === 30 ? 'border-border bg-surface' : 'border-warning-mark/60 bg-warning-mark/20'}`}
                aria-labelledby="license-expiry-title" role={threshold === 30 ? 'status' : 'alert'} data-license-expiry={threshold}>
                <div className="flex min-w-0 flex-1 basis-80 items-start gap-3">
                    <BadgeCheck className="mt-1 h-5 w-5 shrink-0 text-fg-muted" aria-hidden="true" />
                    <div className="min-w-0">
                        <h2 id="license-expiry-title" className="font-semibold text-fg">{t(day ? 'license.expiring.dayTitle' : 'license.expiring.title', vars)}</h2>
                        <p className="max-w-[75ch] text-sm text-fg-muted">{t(day ? 'license.expiring.dayHelp' : 'license.expiring.help', vars)}</p>
                    </div>
                </div>
                <div className="flex shrink-0 flex-wrap items-center gap-4">
                    <Link to="/settings?section=license" className="text-sm font-medium text-primary underline underline-offset-4">{t('license.noticeOpen')}</Link>
                    <Button variant="secondary" onClick={() => { dismissLicenseExpiry(threshold); setClosed(threshold); }}>{t('license.expiring.dismiss')}</Button>
                </div>
            </section>
        );
    }
    if (license.remote.state !== 'known' || !license.remote.value.can_provision) return null;
    // Full use, but the license service could not be reached at the last check
    // (D-039): say until when, and in the last 48 hours who acts.
    // Tam kullanım, ama son kontrolde lisans hizmetine ulaşılamadı.
    const guidance = licenseCheckGuidance(license.remote.value);
    if (!guidance) return null;
    const vars = licenseCheckVars(license.remote.value, locale);
    return (
        <section
            className={`${NOTICE} ${guidance.tone === 'warning' ? 'border-warning-mark/60 bg-warning-mark/20' : 'border-border bg-surface'}`}
            aria-label={t('license.title')} role={guidance.tone === 'warning' ? 'alert' : 'status'} data-license-check={license.remote.value.check}
        >
            <div className="flex min-w-0 items-start gap-3">
                <BadgeCheck className="mt-1 h-5 w-5 shrink-0 text-fg-muted" aria-hidden="true" />
                <div className="min-w-0">
                    <h2 className="font-semibold text-fg">{t(guidance.title, vars)}</h2>
                    <p className="max-w-[75ch] text-sm text-fg-muted">{t(guidance.notice, vars)}</p>
                </div>
            </div>
            <Link to="/settings?section=license" className="shrink-0 text-sm font-medium text-primary underline underline-offset-4">
                {t('license.noticeOpen')}
            </Link>
        </section>
    );
}
