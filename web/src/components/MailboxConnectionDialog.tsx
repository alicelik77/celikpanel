import { useEffect, useRef, useState } from 'react';
import { KeyRound, Server } from 'lucide-react';
import { useMailboxConnectionWords, type MailboxConnectionWordKey } from '../i18n/mailboxConnection';
import { decodeMailSetup, mailSetupURL, webmailSignInURL, type MailProtocol, type MailSetup } from '../lib/mailSetup';
import { useRemote } from '../lib/remote';
import { Button, Dialog, RemoteGate } from './ui';
import { MailAutoconfigNotice } from './MailAutoconfigNotice';
import { MailHostProtection } from './domain/MailHostProtection';
import { decodeCoverageLine } from './domain/coverageLines';

// One mailbox's connection settings (alpha.139; owner, 19 Nov 2026: "her email
// için de bir ayar bilgisi göstersek fena olmaz"). Whoever makes a mailbox
// usually hands its settings to the person who will use it: here they are for
// this one address, each ready to copy, and once more as one plain text for a
// message. The values are the "Settings" tab's (MailSettingsPanel): the same
// read of GET /api/v1/domains/{id}/mail/setup with the same decoder, shared by
// every reader of that address (lib/remote.ts). The password is never in it.
//
// Bir posta kutusunun bağlantı bilgileri: Ayarlar sekmesinin okuduğu aynı
// yanıttan, bu adres için; her değer kopyalanabilir, hepsi bir arada da.
// Parola hiçbir zaman yoktur.

type Say = (key: MailboxConnectionWordKey, values?: Record<string, string | number>) => string;

/**
 * The one "not ready" line the Settings tab already shows, if the domain's
 * mail is not ready: no mail service on this server or no mail name set
 * (the tab's autoconfig line), else the mail name not protected by a
 * trusted certificate or not pointing here (the tab's certificate line).
 * No new check: both come in the same answer.
 */
export function notReadyLine(setup: MailSetup): 'mail' | 'host' | null {
    const reason = setup.autoconfig?.state === 'unavailable' ? setup.autoconfig.reason : '';
    if (reason === 'mail_not_here' || reason === 'mail_host_unknown') return 'mail';
    if (setup.mail_host && decodeCoverageLine(setup.host_protection)?.state === 'not_protected') return 'host';
    return null;
}

/** The plain text "Copy all" gives, in the page's language, without the password. */
export function connectionText(t: Say, address: string, setup: MailSetup): string {
    const block = (title: MailboxConnectionWordKey, name: string, proto: MailProtocol) => [
        `${t(title)} (${name})`,
        `  ${t('mail.setup.server')}: ${proto.host}`,
        `  ${t('mail.setup.port')}: ${proto.port}`,
        `  ${t('mail.setup.security')}: ${proto.security}`,
    ];
    const webmail = webmailSignInURL(setup.webmail, address);
    return [
        t('mconn.text.title', { address }),
        '',
        `${t('mail.setup.username')}: ${address}`,
        t('mconn.text.password'),
        '',
        ...block('mconn.incoming', 'IMAP', setup.imap),
        '',
        ...block('mconn.incomingAlt', 'POP3', setup.pop3),
        '',
        ...block('mconn.outgoing', 'SMTP', setup.smtp),
        ...(webmail ? ['', `${t('mail.row.webmail')}: ${webmail}`] : []),
    ].join('\n');
}

type Said = 'copied' | 'copiedAll' | 'failed' | null;

export default function MailboxConnectionDialog({ domainId, domainName, address, readOnly = false, onClose }: {
    domainId: number;
    domainName: string;
    address: string;
    /** A team member who only reads: the password line names who can set one. */
    readOnly?: boolean;
    onClose: () => void;
}) {
    const { t } = useMailboxConnectionWords();
    const setup = useRemote(mailSetupURL(domainId), decodeMailSetup);
    const [said, setSaid] = useState<Said>(null);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);

    // Copying is a convenience: when the browser refuses, the values stay
    // selectable and the line under them says so.
    const copy = async (text: string, done: Exclude<Said, 'failed' | null>) => {
        window.clearTimeout(timer.current);
        try {
            if (!navigator.clipboard) throw new Error('clipboard');
            await navigator.clipboard.writeText(text);
            setSaid(done);
            timer.current = window.setTimeout(() => setSaid(null), 4000);
        } catch {
            setSaid('failed');
        }
    };

    const known = setup.remote.state === 'known' ? setup.remote.value : null;
    const copyButton = (value: string) => (
        <button type="button" onClick={() => void copy(value, 'copied')} data-mailbox-copy={value}
            className="shrink-0 rounded-md border border-border-strong px-2.5 py-1 text-xs font-medium text-fg hover:bg-surface-2 max-sm:min-h-11">
            {t('common.copy')}
        </button>
    );
    // A value as wide as itself, its Copy right beside it.
    const copyable = (text: string) => (
        <span className="flex min-w-0 max-w-full items-center gap-2">
            <span className="min-w-0 [overflow-wrap:anywhere] font-mono text-sm text-fg">{text}</span>
            {copyButton(text)}
        </span>
    );
    const value = (label: string, text: string) => (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <dt className="w-36 shrink-0 text-sm text-fg-muted max-sm:w-full">{label}</dt>
            <dd className="min-w-0 max-w-full">{copyable(text)}</dd>
        </div>
    );
    // One row per protocol: what it is, its server, port and security. On a
    // phone the row is a card with each value named (no sideways scrolling).
    // Her protokol bir satır; telefonda her değer adıyla bir kart.
    const muted = 'text-fg-muted sm:hidden';
    const protocol = (title: MailboxConnectionWordKey, name: string, proto: MailProtocol) => (
        <tr className="border-t border-border align-middle max-sm:flex max-sm:flex-wrap max-sm:items-center max-sm:gap-x-4 max-sm:gap-y-1 max-sm:py-2" data-mailbox-protocol={name}>
            <th scope="row" className="w-36 py-2 pr-3 text-left text-sm font-medium text-fg max-sm:w-full max-sm:py-0">
                {t(title)} <span className="whitespace-nowrap font-normal text-fg-muted">({name})</span>
            </th>
            <td className="py-2 pr-3 max-sm:w-full max-sm:py-0">{copyable(proto.host)}</td>
            <td className="whitespace-nowrap py-2 pr-3 font-mono text-sm text-fg max-sm:py-0"><span className={`${muted} mr-1.5 font-sans`}>{t('mail.setup.port')}</span>{proto.port}</td>
            <td className="whitespace-nowrap py-2 text-sm text-fg max-sm:py-0"><span className={`${muted} mr-1.5`}>{t('mail.setup.security')}</span>{proto.security}</td>
        </tr>
    );

    return (
        <Dialog
            id="mailbox-connection-dialog"
            icon={Server}
            width="xl"
            onDismiss={onClose}
            title={<span className="break-all">{address}</span>}
            description={t('mconn.lead')}
            footerLead={said && (
                <p role="status" className="text-sm text-fg-muted" data-mailbox-copy-said={said}>
                    {t(said === 'copied' ? 'mconn.copied' : said === 'copiedAll' ? 'mconn.copiedAll' : 'mconn.copyFailed')}
                </p>
            )}
            actions={
                <>
                    <Button type="button" onClick={onClose}>{t('common.close')}</Button>
                    <Button type="button" variant="primary" disabled={!known} onClick={() => known && void copy(connectionText(t, address, known), 'copiedAll')}
                        data-mailbox-copy-all="">
                        {t('mconn.copyAll')}
                    </Button>
                </>
            }
        >
            <RemoteGate
                remote={setup.remote}
                checking={t('mail.setup.checking')}
                failed={t('mail.setup.unknown')}
                onRetry={() => void setup.retry()}
                busy={setup.reading}
            >
                {({ value: settings }) => {
                    const problem = notReadyLine(settings);
                    const webmail = webmailSignInURL(settings.webmail, address);
                    return (
                        <div className="space-y-4" data-mailbox-connection={address}>
                            {/* The Settings tab's own line, once, above the values. */}
                            {problem && (
                                <div className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 px-3 py-2 [&>p]:my-0" data-mailbox-connection-problem={problem}>
                                    {problem === 'mail' && settings.autoconfig
                                        ? <MailAutoconfigNotice domainId={domainId} autoconfig={settings.autoconfig} readOnly />
                                        : <MailHostProtection host={settings.mail_host} protection={settings.host_protection} domainName={domainName} />}
                                </div>
                            )}
                            <dl>{value(t('mail.setup.username'), address)}</dl>
                            <table className="w-full border-b border-border max-sm:block">
                                <thead className="max-sm:hidden">
                                    <tr className="text-left text-xs font-semibold text-fg-muted">
                                        <td className="pb-1.5" />
                                        <th scope="col" className="pb-1.5 pr-3">{t('mail.setup.server')}</th>
                                        <th scope="col" className="pb-1.5 pr-3">{t('mail.setup.port')}</th>
                                        <th scope="col" className="pb-1.5">{t('mail.setup.security')}</th>
                                    </tr>
                                </thead>
                                <tbody className="max-sm:block">
                                    {protocol('mconn.incoming', 'IMAP', settings.imap)}
                                    {protocol('mconn.incomingAlt', 'POP3', settings.pop3)}
                                    {protocol('mconn.outgoing', 'SMTP', settings.smtp)}
                                </tbody>
                            </table>
                            {webmail && <dl>{value(t('mail.row.webmail'), webmail)}</dl>}
                            <p className="flex items-start gap-2 text-sm text-fg-muted" data-mailbox-password-line="">
                                <KeyRound className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                                <span>{t(readOnly ? 'mconn.passwordReadOnly' : 'mconn.password')}</span>
                            </p>
                        </div>
                    );
                }}
            </RemoteGate>
        </Dialog>
    );
}
