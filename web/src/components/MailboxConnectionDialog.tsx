import { useEffect, useRef, useState } from 'react';
import { KeyRound, Server } from 'lucide-react';
import { useMailboxConnectionWords, type MailboxConnectionWordKey } from '../i18n/mailboxConnection';
import { decodeMailSetup, mailSetupURL, webmailSignInURL, type MailProtocol, type MailSetup } from '../lib/mailSetup';
import { useRemote } from '../lib/remote';
import { useAuth } from '../auth/AuthContext';
import { Button, Dialog, RemoteGate } from './ui';
import { Guidance } from './kit/rows/Guidance';
import { whoWord } from './kit/words';
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
 * Whether these settings lead to this mailbox, from the read alone (no new
 * check). The server names one host for IMAP, POP3 and SMTP
 * (cmd/panel/domain_mail_extra_handlers.go handleMailClientSetupWith):
 * the name mail programs are told (mail_autoconfig.go mailClientHost: the
 * server's own mail name, or mail.<domain> for a domain that chose it with a
 * certificate serving mail) whenever the auto-setup decision has one; in
 * every other case the bare convention mail.<domain>, which the server did
 * not choose and never looked up. The read says nothing about the public DNS
 * of the name (certificate_coverage.go mailProgramsCoverage reads without
 * it), so a name "pointing elsewhere" can never be stated here.
 *
 *   notHere      mail is not installed on this server: verified, nothing reaches it
 *   nameUnset    the server's mail name is not set: the name is a stand-in
 *   unconfirmed  any other answer without the told name: a stand-in too
 *   certificate  the told name, without a trusted certificate (the tab's line,
 *                only when it is about the name shown)
 *
 * Ayarların bu posta kutusuna ulaşıp ulaşmadığı, yalnız okunan yanıttan.
 */
export type ConnectionProblem = 'notHere' | 'nameUnset' | 'unconfirmed' | 'certificate' | null;
const sameName = (a: string, b: string) => a.trim().toLowerCase().replace(/\.$/, '') === b.trim().toLowerCase().replace(/\.$/, '');
export function connectionProblem(setup: MailSetup): ConnectionProblem {
    const auto = setup.autoconfig;
    if (auto?.state === 'unavailable' && auto.reason === 'mail_not_here') return 'notHere';
    if (!auto || !auto.mail_host || !sameName(auto.mail_host, setup.mail_host)) {
        return auto?.state === 'unavailable' && auto.reason === 'mail_host_unknown' ? 'nameUnset' : 'unconfirmed';
    }
    const line = decodeCoverageLine(setup.host_protection);
    return line?.state === 'not_protected' && sameName(line.name, setup.mail_host) ? 'certificate' : null;
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
    const problem = connectionProblem(setup);
    return [
        // A name this server did not confirm is never handed on as ready.
        ...(problem === 'nameUnset' || problem === 'unconfirmed' ? [t('mconn.text.unconfirmed'), ''] : []),
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
        ...(setup.smtp.auth_required === true ? [t('mconn.text.smtpAuth')] : []),
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
    const { role } = useAuth();
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
    const notHere = !!known && connectionProblem(known) === 'notHere';
    const admin = role === 'admin';
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
                    {/* Mail not on this server: nothing in the text would work, so
                        there is nothing to hand on; the line above says why. */}
                    <Button type="button" variant="primary" disabled={!known || notHere} onClick={() => known && !notHere && void copy(connectionText(t, address, known), 'copiedAll')}
                        aria-describedby={notHere ? 'mailbox-connection-problem' : undefined} data-mailbox-copy-all="">
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
                    const problem = connectionProblem(settings);
                    const webmail = webmailSignInURL(settings.webmail, address);
                    const reach = problem === 'notHere' || problem === 'nameUnset' || problem === 'unconfirmed' ? problem : null;
                    return (
                        <div className="space-y-4" data-mailbox-connection={address}>
                            {/* Whether these settings reach this mailbox, once, above the values (D-024). */}
                            {reach && (
                                <div id="mailbox-connection-problem"><Guidance
                                    tone={reach === 'notHere' ? 'blocked' : 'unknown'}
                                    data-mailbox-connection-problem={reach}
                                    title={t(reach === 'notHere' ? 'mconn.reach.notHere.title' : 'mconn.reach.maybe.title')}
                                    why={t(`mconn.reach.${reach}.why` as MailboxConnectionWordKey, { name: settings.mail_host })}
                                    who={whoWord(t, admin ? 'you' : 'administrator')}
                                    next={t(`mconn.reach.${reach}.next.${admin ? 'admin' : 'other'}` as MailboxConnectionWordKey)}
                                    then={reach === 'unconfirmed' ? undefined : t(`mconn.reach.${reach}.then` as MailboxConnectionWordKey)}
                                /></div>
                            )}
                            {/* The Settings tab's certificate line, only about the name shown. */}
                            {problem === 'certificate' && (
                                <div className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 px-3 py-2 [&>p]:my-0" data-mailbox-connection-problem="certificate">
                                    <MailHostProtection host={settings.mail_host} protection={settings.host_protection} domainName={domainName} />
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
                                    {settings.smtp.auth_required === true && (
                                        <tr className="max-sm:block" data-mailbox-smtp-auth="">
                                            <td className="max-sm:hidden" />
                                            <td colSpan={3} className="pb-2 text-sm text-fg-muted max-sm:block">{t('mconn.smtpAuth')}</td>
                                        </tr>
                                    )}
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
