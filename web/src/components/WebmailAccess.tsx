import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { decodeMailSetup, mailSetupURL, type WebmailReason } from '../lib/mailSetup';
import { useRemote } from '../lib/remote';
import { Link } from '../router';
import { CouldNotCheck } from './ui';

// The webmail card of a domain's mail screen. Whether webmail is available is
// read from the tenant-scoped setup endpoint and is one of three things
// (9 Oct 2026): being checked, could not be checked, or known. "Webmail is not
// available on this server" is said only for an answer the server gave; a read
// that failed used to say it too.
//
// Since alpha.110 webmail opens on its own address (https://<host>/), served
// by the server's own web server and never by the panel. Since alpha.135
// (owner, 13 Nov 2026: "webmail her emaile özgü link olsa") each mailbox row
// opens it with its own address filled in (DomainMailManager.tsx), so this
// card is only a problem with its fix: it is drawn when webmail cannot be
// opened, or when that could not be checked, and is absent when it works.
// The administrator gets the one action that fixes it, anyone else is told
// that the provider acts.
//
// Bir alan adının posta ekranındaki webmail kartı. alpha.135'ten beri her
// posta kutusu satırı web postasını kendi adresiyle açar; bu kart yalnız
// sorun ve çözümüdür: web postası açılamıyorsa ya da bu denetlenemediyse
// görünür, çalışıyorsa yoktur.
export default function WebmailAccess({ domainId }: { domainId: number }) {
    const { t } = useI18n();
    const { role } = useAuth();
    const { remote, reading, retry } = useRemote(mailSetupURL(domainId), decodeMailSetup);
    const url = remote.state === 'known' ? remote.value.webmail : null;
    const isAdmin = role === 'admin';
    // While it is read, and once it works, there is nothing to say here.
    if (remote.state === 'loading' || url) return null;

    return (
        <div className='mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border p-3' data-webmail-access=''>
            <div className='min-w-0 flex-1'>
                <div className='text-sm font-medium text-fg'>{t('mail.webmail.title')}</div>
                {remote.state === 'unknown' ? (
                    <CouldNotCheck className='mt-2' text={t('mail.webmail.unknown')} onRetry={() => void retry()} busy={reading} />
                ) : (
                    <WebmailMissing reason={remote.value.webmail_reason} isAdmin={isAdmin} />
                )}
            </div>
        </div>
    );
}

// Why there is no webmail, who acts and the one next step (alpha.127, owner on
// Boston: "Webmail is not available on this server yet" said none of these).
// The administrator gets the action itself: install the Webmail setup (its own
// install dialog on Components, confirmed there), give Roundcube its address,
// or open Roundcube when it does not answer. Everyone else is told what is
// missing and that their provider acts; mail apps keep working meanwhile.
// Web postası neden yok, kim ne yapar: yönetici tek eylemi alır, diğerleri
// sağlayıcısına sorar.
const ADMIN_STEP: Record<WebmailReason, { to: string; label: 'adminNext.webmail.install' | 'mail.webmail.setAddress' | 'adminNext.webmail.openRoundcube' }> = {
    not_installed: { to: '/components?bundle=mail.webmail', label: 'adminNext.webmail.install' },
    no_address: { to: '/components/roundcube', label: 'mail.webmail.setAddress' },
    not_answering: { to: '/components/roundcube', label: 'adminNext.webmail.openRoundcube' },
};

function WebmailMissing({ reason, isAdmin }: { reason: WebmailReason | null; isAdmin: boolean }) {
    const { t } = useI18n();
    if (!isAdmin) {
        return (
            <p className='text-xs text-fg-muted' data-webmail-missing={reason ?? 'unknown'}>
                {t('mail.webmail.unavailable')} {t(reason ? `mail.webmail.tenant.${reason}` : 'mail.webmail.tenant.other')}
            </p>
        );
    }
    const step = reason ? ADMIN_STEP[reason] : { to: '/components/roundcube', label: 'mail.webmail.setAddress' as const };
    return (
        <>
            <p className='text-xs text-fg-muted' data-webmail-missing={reason ?? 'unknown'}>
                {t(reason ? `adminNext.webmail.${reason}` : 'mail.webmail.noAddressAdmin')}
            </p>
            <Link to={step.to} className='mt-1 inline-flex min-h-[2.75rem] items-center text-xs font-medium text-primary underline underline-offset-2 sm:min-h-0' data-webmail-set-address='' data-webmail-action={reason ?? 'no_address'}>
                {t(step.label)}
            </Link>
        </>
    );
}
