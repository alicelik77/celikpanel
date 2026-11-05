import { useState } from 'react';
import { AlertTriangle, Copy, KeyRound, RefreshCw } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, Checking, Dialog } from './ui';
import { showToast } from './Toast';
import { apiErrorText, readApiError } from '../lib/apiError';
import { readRemote, useRemote } from '../lib/remote';
import { decodeMailSetup, mailSetupURL, type MailSetup } from '../lib/mailSetup';

// Passwordless mail, phase 1 (alpha.124; owner decision of 4 Nov 2026). The
// next release closes mail sent from this server without a mailbox password:
// PHP mail() and the sendmail command, and SMTP to port 25 on this server
// without a password. This notice says so first, then which sites used it in
// the last 30 days (read from Postfix's own log), and gives each site one
// button: create a sending mailbox and show the settings to use. The password
// is made in this browser, sent once to create the mailbox and shown once.
//
// Parolasız posta, 1. aşama: bir sonraki sürüm kapatır. Bildirim önce bunu,
// sonra son 30 günde kullanan siteleri söyler; her site için tek düğme: bir
// gönderim posta kutusu oluştur ve ayarları göster. Parola bu tarayıcıda
// üretilir ve bir kez gösterilir.

export const PASSWORDLESS_MAIL_URL = '/api/v1/mail/passwordless';

interface PasswordlessSite {
    domain_id: number;
    domain: string;
    ways: string[];
    messages: number;
    refused: number;
    last?: string;
    sender_mailbox?: string;
}

interface PasswordlessOther {
    label: string;
    way: string;
    messages: number;
    refused: number;
    last?: string;
}

interface PasswordlessView {
    state: 'unknown' | 'none' | 'found';
    reason?: string;
    window_days: number;
    covered_since?: string;
    incomplete?: boolean;
    sites: PasswordlessSite[];
    others: PasswordlessOther[];
}

function decodePasswordless(raw: unknown): PasswordlessView {
    const value = raw as Partial<PasswordlessView> | null;
    if (!value || !['unknown', 'none', 'found'].includes(value.state as string) || !Array.isArray(value.sites) || !Array.isArray(value.others)) {
        throw new Error('passwordless mail');
    }
    return value as PasswordlessView;
}

// 18 random bytes, base64url: 24 characters.
function newMailboxPassword(): string {
    const bytes = new Uint8Array(18);
    crypto.getRandomValues(bytes);
    return btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_');
}

const day = (iso?: string) => (iso ? new Date(iso).toLocaleDateString() : '');

interface Created {
    domainId: number;
    address: string;
    password?: string;
    setup?: MailSetup;
}

export function PasswordlessMailNotice() {
    const { t } = useI18n();
    const report = useRemote(PASSWORDLESS_MAIL_URL, decodePasswordless);
    const [busy, setBusy] = useState<number | null>(null);
    const [shown, setShown] = useState<Created | null>(null);

    // A settings read that fails leaves the host named as unknown in the
    // dialogue; the mailbox exists either way.
    const withSetup = async (created: Created) => {
        const setup = await readRemote(mailSetupURL(created.domainId), decodeMailSetup);
        setShown({ ...created, setup: setup.state === 'known' ? setup.value : undefined });
    };

    const create = async (site: PasswordlessSite) => {
        const address = `noreply@${site.domain}`;
        const password = newMailboxPassword();
        setBusy(site.domain_id);
        try {
            const res = await fetch(`/api/v1/domains/${site.domain_id}/mail/accounts`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ address, password, quota_mb: 1024 }),
            });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t, 'pwmail.createFailed'));
                return;
            }
            await withSetup({ domainId: site.domain_id, address, password });
        } catch {
            showToast('error', t('pwmail.createFailed'));
        } finally {
            setBusy(null);
            // The list shows the new mailbox by itself.
            void report.retry();
        }
    };

    const remote = report.remote;
    if (remote.state === 'loading') return <Checking className="mb-4" label={t('pwmail.checking')} />;
    if (remote.state === 'unknown') {
        return (
            <Box>
                <p>{t('pwmail.unreadPanel')}</p>
                <Retry onClick={() => void report.retry()} loading={report.reading} />
            </Box>
        );
    }
    const view = remote.value;
    const windowStart = Date.now() - view.window_days * 86_400_000;
    const shortLog = view.covered_since && new Date(view.covered_since).getTime() > windowStart + 86_400_000;

    return (
        <Box>
            <p className="font-medium">{t('pwmail.title')}</p>
            <p className="mt-1">{t('pwmail.why')}</p>
            {view.state === 'unknown' && (
                <>
                    <p className="mt-2">{t('pwmail.unread', { reason: view.reason ?? '' })}</p>
                    <Retry onClick={() => void report.retry()} loading={report.reading} />
                </>
            )}
            {view.state === 'none' && <p className="mt-2">{t('pwmail.none', { days: view.window_days })}</p>}
            {view.state === 'found' && (
                <>
                    <p className="mt-2">{t('pwmail.found', { days: view.window_days })}</p>
                    <ul className="mt-2 divide-y divide-border rounded-lg border border-border bg-surface text-fg">
                        {view.sites.map((site) => (
                            <li key={site.domain_id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                                <div className="min-w-0">
                                    <p className="font-medium break-words">{site.domain}</p>
                                    <p className="text-xs text-fg-muted">
                                        {site.ways.map((way) => t(way === 'smtp' ? 'pwmail.way.smtp' : 'pwmail.way.sendmail')).join(' · ')}
                                        {' · '}
                                        {t('pwmail.count', { count: site.messages + site.refused, last: day(site.last) })}
                                    </p>
                                    {site.sender_mailbox && (
                                        <p className="text-xs text-fg-muted">{t('pwmail.hasMailbox', { address: site.sender_mailbox })}</p>
                                    )}
                                </div>
                                {site.sender_mailbox ? (
                                    <Button icon={KeyRound} onClick={() => void withSetup({ domainId: site.domain_id, address: site.sender_mailbox as string })}>
                                        {t('pwmail.showSettings')}
                                    </Button>
                                ) : (
                                    <Button variant="primary" icon={KeyRound} loading={busy === site.domain_id} disabled={busy !== null} onClick={() => void create(site)}>
                                        {t('pwmail.create')}
                                    </Button>
                                )}
                            </li>
                        ))}
                    </ul>
                    {view.others.length > 0 && (
                        <>
                            <p className="mt-3">{t('pwmail.others')}</p>
                            <ul className="mt-1 list-disc pl-5">
                                {view.others.map((other) => (
                                    <li key={`${other.way}:${other.label}`} className="break-words">
                                        {other.label} · {t(other.way === 'smtp' ? 'pwmail.way.smtp' : 'pwmail.way.sendmail')} ·{' '}
                                        {t('pwmail.count', { count: other.messages + other.refused, last: day(other.last) })}
                                    </li>
                                ))}
                            </ul>
                        </>
                    )}
                </>
            )}
            {view.state !== 'unknown' && (shortLog || view.incomplete) && (
                <p className="mt-2 text-xs">{t(view.incomplete ? 'pwmail.incomplete' : 'pwmail.shortLog', { since: day(view.covered_since) })}</p>
            )}
            {shown && <SettingsDialog created={shown} onClose={() => setShown(null)} />}
        </Box>
    );
}

function Box({ children }: { children: React.ReactNode }) {
    return (
        <div role="status" className="mb-4 flex gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2.5 text-sm text-fg">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div className="min-w-0 flex-1">{children}</div>
        </div>
    );
}

function Retry({ onClick, loading }: { onClick: () => void; loading: boolean }) {
    const { t } = useI18n();
    return (
        <Button className="mt-2" icon={RefreshCw} loading={loading} onClick={onClick}>
            {t('pwmail.retry')}
        </Button>
    );
}

function SettingsDialog({ created, onClose }: { created: Created; onClose: () => void }) {
    const { t } = useI18n();
    const host = created.setup?.smtp.host || created.setup?.mail_host || '';
    const copy = (value: string) => {
        void navigator.clipboard?.writeText(value).then(
            () => showToast('success', t('pwmail.copied')),
            () => showToast('error', t('pwmail.copyFailed')),
        );
    };
    const rows: Array<[string, string, boolean]> = [
        [t('pwmail.set.host'), host || t('pwmail.set.hostUnknown'), !!host],
        [t('pwmail.set.port'), t('pwmail.set.portValue'), false],
        [t('pwmail.set.user'), created.address, true],
        [t('pwmail.set.from'), created.address, false],
    ];
    return (
        <Dialog id="pwmail-settings" title={t('pwmail.set.title', { address: created.address })} icon={KeyRound} onDismiss={onClose}
            actions={<Button variant="primary" onClick={onClose}>{t('pwmail.set.done')}</Button>}>
            <p className="text-sm">{t('pwmail.set.intro')}</p>
            <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-sm">
                {rows.map(([label, value, copyable]) => (
                    <div key={label} className="contents">
                        <dt className="text-fg-muted">{label}</dt>
                        <dd className="flex min-w-0 items-center gap-2 break-all">
                            {value}
                            {copyable && (
                                <button type="button" className="text-fg-muted hover:text-fg" aria-label={t('pwmail.copy')} onClick={() => copy(value)}>
                                    <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                                </button>
                            )}
                        </dd>
                    </div>
                ))}
                <dt className="text-fg-muted">{t('pwmail.set.password')}</dt>
                <dd className="flex min-w-0 items-center gap-2 break-all">
                    {created.password ? (
                        <>
                            <code>{created.password}</code>
                            <button type="button" className="text-fg-muted hover:text-fg" aria-label={t('pwmail.copy')} onClick={() => copy(created.password as string)}>
                                <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                            </button>
                        </>
                    ) : (
                        t('pwmail.set.passwordKept')
                    )}
                </dd>
            </dl>
            {created.password && <p className="mt-3 text-sm font-medium">{t('pwmail.set.once')}</p>}
        </Dialog>
    );
}
