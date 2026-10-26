import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { decodeMailSetup, mailSetupURL } from '../lib/mailSetup';
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
// link). When there is none, an administrator is told where to set it; anyone
// else is told it is not available yet.
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
                ) : isAdmin ? (
                    <>
                        <p className='text-xs text-fg-muted'>{t('mail.webmail.noAddressAdmin')}</p>
                        <Link to='/services/roundcube' className='mt-1 inline-block text-xs font-medium text-primary underline underline-offset-2' data-webmail-set-address=''>
                            {t('mail.webmail.setAddress')}
                        </Link>
                    </>
                ) : (
                    <p className='text-xs text-fg-muted'>{t('mail.webmail.unavailable')}</p>
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
