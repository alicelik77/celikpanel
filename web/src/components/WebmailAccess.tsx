import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { decodeMailSetup, mailSetupURL, type WebmailReason } from '../lib/mailSetup';
import { useRemote } from '../lib/remote';
import { Link } from '../router';
import { Checking, CouldNotCheck } from './ui';

// The webmail card of a domain's mail screen. Whether webmail is available is
// read from the tenant-scoped setup endpoint and is one of three things
// (9 Oct 2026): being checked, could not be checked, or known. "Webmail is not
// available on this server" is said only for an answer the server gave; a read
// that failed used to say it too.
//
// Since alpha.110 webmail opens on its own address (https://<host>/), served
// by the server's own web server and never by the panel. Only that exact
// address may become a link (see lib/mailSetup.ts: anything else decodes to no
// link). When there is none, the card says why (alpha.127): the administrator
// gets the one action that fixes it, anyone else is told that the provider acts.
//
// Bir alan adının posta ekranındaki webmail kartı. Webmail'in kullanılabilir
// olup olmadığı kiracıya özgü kurulum uç noktasından okunur ve üç şeyden
// biridir: kontrol ediliyor, kontrol edilemedi ya da biliniyor. alpha.110'dan
// beri web postası kendi adresinde (https://<ad>/) açılır; yalnız o tam adres
// bağlantı olabilir. Adres yoksa yönetici nerede ayarlayacağını görür.
export default function WebmailAccess({ domainId }: { domainId: number }) {
    const { t } = useI18n();
    const { role } = useAuth();
    const { remote, reading, retry } = useRemote(mailSetupURL(domainId), decodeMailSetup);
    const url = remote.state === 'known' ? remote.value.webmail : null;
    const isAdmin = role === 'admin';

    return (
        <div className='mb-4 flex min-h-[4.25rem] flex-wrap items-center justify-between gap-3 rounded-lg border border-border p-3' data-webmail-access=''>
            <div className='min-w-0 flex-1'>
                <div className='text-sm font-medium text-fg'>{t('mail.webmail.title')}</div>
                {remote.state === 'loading' ? (
                    <Checking label={t('mail.webmail.checking')} className='mt-0.5' />
                ) : remote.state === 'unknown' ? (
                    <CouldNotCheck className='mt-2' text={t('mail.webmail.unknown')} onRetry={() => void retry()} busy={reading} />
                ) : url ? (
                    <>
                        <p className='text-xs text-fg-muted'>{t('mail.webmail.available')}</p>
                        <p className='break-all text-xs text-fg-muted'>{t('mail.webmail.signIn', { url })}</p>
                    </>
                ) : (
                    <WebmailMissing reason={remote.value.webmail_reason} isAdmin={isAdmin} />
                )}
            </div>
            {url && (
                <a href={url} target='_blank' rel='noopener noreferrer' className='inline-flex min-h-[2.75rem] items-center rounded-lg border border-border-strong px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2'>
                    {t('mail.webmail.open')}
                </a>
            )}
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
const ADMIN_STEP: Record<WebmailReason, { to: string; label: 'mail.webmail.installAction' | 'mail.webmail.setAddress' | 'mail.webmail.openRoundcube' }> = {
    not_installed: { to: '/components?bundle=mail.webmail', label: 'mail.webmail.installAction' },
    no_address: { to: '/components/roundcube', label: 'mail.webmail.setAddress' },
    not_answering: { to: '/components/roundcube', label: 'mail.webmail.openRoundcube' },
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
                {t(reason ? `mail.webmail.admin.${reason}` : 'mail.webmail.noAddressAdmin')}
            </p>
            <Link to={step.to} className='mt-1 inline-flex min-h-[2.75rem] items-center text-xs font-medium text-primary underline underline-offset-2 sm:min-h-0' data-webmail-set-address='' data-webmail-action={reason ?? 'no_address'}>
                {t(step.label)}
            </Link>
        </>
    );
}
