// What a domain's mail screens read about the server's mail service
// (`GET /api/v1/domains/{id}/mail/setup`), and its one decoder (see
// lib/remote.ts: every reader of an address shares the decoder and the
// request). The client settings card and the webmail card both read it.

import { webmailLinkOrNull } from './webmailAddress';

// A read that failed is unknown. It is not "webmail is not available on this
// server", which is what the card said before 9 Oct 2026.
//
// Bir alan adının posta ekranlarının sunucunun posta hizmeti hakkında okuduğu
// şey ve onun tek çözücüsü. Başarısız okuma bilinmeyendir; "bu sunucuda webmail
// yok" değildir.

export interface MailProtocol {
    host: string;
    port: number;
    security: string;
    auth_required?: boolean;
}

export interface MailSetup {
    mail_host: string;
    imap: MailProtocol;
    pop3: MailProtocol;
    smtp: MailProtocol;
    username_is_full_email: boolean;
    // Only webmail's own address (https://<host>/, exactly) may become a link.
    // Anything unavailable, malformed or any other URL fails closed to null:
    // no link (alpha.110).
    // Yalnız web postasının kendi adresi (https://<ad>/) bağlantı olabilir.
    webmail: string | null;
    // Why there is no webmail link (alpha.127): Roundcube is not installed,
    // has no address of its own, or does not answer there. null when webmail
    // is available or an older Panel did not say.
    // Web postası bağlantısının neden olmadığı.
    webmail_reason: WebmailReason | null;
    // Whether mail programs set themselves up (alpha.130); null when an older
    // Panel did not say or the answer did not read as one.
    // Posta programlarının kendini ayarlayıp ayarlamadığı.
    autoconfig: MailAutoconfig | null;
}

export interface MailAutoconfigRecord {
    name: string;
    type: string;
    content: string;
    prio?: number;
    state: 'present' | 'missing' | 'different' | 'unknown';
}

export interface MailAutoconfig {
    state: 'on' | 'partial' | 'off' | 'unavailable';
    reason: string;
    names: string[];
    mail_host: string;
    served: 'yes' | 'no' | 'unknown';
    secure: 'yes' | 'no' | 'unknown';
    dns: 'local' | 'remote' | 'external' | '';
    records: MailAutoconfigRecord[];
    can_fix: boolean;
}

const oneOf = <T extends string>(value: unknown, allowed: readonly T[], fallback: T): T =>
    typeof value === 'string' && (allowed as readonly string[]).includes(value) ? value as T : fallback;

export function decodeMailAutoconfig(raw: unknown): MailAutoconfig | null {
    const value = raw as Record<string, unknown> | null;
    if (!value || typeof value !== 'object' || typeof value.state !== 'string') return null;
    const state = oneOf(value.state, ['on', 'partial', 'off', 'unavailable'] as const, 'unavailable');
    const records = Array.isArray(value.records) ? value.records.flatMap((item): MailAutoconfigRecord[] => {
        const record = item as Record<string, unknown> | null;
        if (!record || typeof record.name !== 'string' || typeof record.type !== 'string' || typeof record.content !== 'string') return [];
        return [{
            name: record.name, type: record.type, content: record.content,
            ...(typeof record.prio === 'number' ? { prio: record.prio } : {}),
            state: oneOf(record.state, ['present', 'missing', 'different', 'unknown'] as const, 'unknown'),
        }];
    }) : [];
    return {
        state,
        reason: typeof value.reason === 'string' ? value.reason : '',
        names: Array.isArray(value.names) ? value.names.filter((n): n is string => typeof n === 'string') : [],
        mail_host: typeof value.mail_host === 'string' ? value.mail_host : '',
        served: oneOf(value.served, ['yes', 'no', 'unknown'] as const, 'unknown'),
        secure: oneOf(value.secure, ['yes', 'no', 'unknown'] as const, 'unknown'),
        dns: oneOf(value.dns, ['local', 'remote', 'external', ''] as const, ''),
        records,
        can_fix: value.can_fix === true,
    };
}

export type WebmailReason = 'not_installed' | 'no_address' | 'not_answering';
const WEBMAIL_REASONS: readonly string[] = ['not_installed', 'no_address', 'not_answering'];

export const mailSetupURL = (domainId: number) => `/api/v1/domains/${domainId}/mail/setup`;

function protocol(raw: unknown): MailProtocol {
    const value = raw as Partial<MailProtocol> | null;
    if (!value || typeof value.host !== 'string' || typeof value.port !== 'number' || typeof value.security !== 'string') {
        throw new Error('mail setup');
    }
    return value as MailProtocol;
}

export function decodeMailSetup(raw: unknown): MailSetup {
    const setup = raw as Record<string, unknown> | null;
    if (!setup || typeof setup !== 'object' || typeof setup.webmail_available !== 'boolean') throw new Error('mail setup');
    return {
        mail_host: typeof setup.mail_host === 'string' ? setup.mail_host : '',
        imap: protocol(setup.imap),
        pop3: protocol(setup.pop3),
        smtp: protocol(setup.smtp),
        username_is_full_email: setup.username_is_full_email === true,
        webmail: setup.webmail_available === true ? webmailLinkOrNull(setup.webmail_url) : null,
        webmail_reason: typeof setup.webmail_reason === 'string' && WEBMAIL_REASONS.includes(setup.webmail_reason)
            ? setup.webmail_reason as WebmailReason : null,
        autoconfig: decodeMailAutoconfig(setup.autoconfig),
    };
}
