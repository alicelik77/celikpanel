// Account access (v0.1.0-alpha.114, P6): forgot password, the pages a reset,
// invitation or confirmation link opens, the own-address confirmation and
// invitations. Server side: cmd/panel/account_recovery_handlers.go and
// account_invitation_handlers.go; docs/ACCOUNT-ACCESS.md.
//
// A link carries its token in the fragment (#t=…), so the token never reaches
// a server log or a Referer header; the page reads it and sends it in a POST
// body. The fragment is removed from the address bar as soon as it is read.
//
// Hesap erişimi: parolamı unuttum, bağlantı sayfaları, adres onayı ve davetler.
// Jeton bağlantının parça kısmında (#t=…) taşınır; sunucu günlüğüne ya da
// Referer başlığına gitmez.

export const RECOVERY_MAIL_STATUS_URL = '/api/v1/auth/recovery/mail-status';
export const RECOVERY_REQUEST_URL = '/api/v1/auth/recovery/request';
export const RECOVERY_LINK_URL = '/api/v1/auth/recovery/link';
export const RECOVERY_RESET_URL = '/api/v1/auth/recovery/reset';
export const RECOVERY_ACCEPT_URL = '/api/v1/auth/recovery/accept-invitation';
export const RECOVERY_VERIFY_URL = '/api/v1/auth/recovery/verify-email';
export const OWN_EMAIL_VERIFICATION_URL = '/api/v1/auth/email/verification';
export const INVITATIONS_URL = '/api/v1/invitations';

export const ACCOUNT_LINK_PREFIX = '/account-link/';

export type AccountLinkPurpose = 'password_reset' | 'invitation' | 'email_verify';

/** The page an account link opens, from its path; null for any other path. */
export function accountLinkPurpose(pathname: string): AccountLinkPurpose | null {
    switch (pathname.replace(/\/+$/, '')) {
        case '/account-link/reset':
            return 'password_reset';
        case '/account-link/invitation':
            return 'invitation';
        case '/account-link/verify':
            return 'email_verify';
        default:
            return null;
    }
}

/** The token of a link's fragment (#t=<43 URL-safe characters>), or null. */
export function accountLinkToken(hash: string): string | null {
    const match = /^#t=([A-Za-z0-9_-]{43})$/.exec(hash);
    return match ? match[1] : null;
}

export interface AccountMailStatus {
    available: boolean;
    reason?: string;
}

export function decodeAccountMailStatus(raw: unknown): AccountMailStatus {
    if (!raw || typeof raw !== 'object' || typeof (raw as { available?: unknown }).available !== 'boolean') {
        throw new Error('malformed account mail status');
    }
    const { available, reason } = raw as { available: boolean; reason?: unknown };
    return { available, reason: typeof reason === 'string' ? reason : undefined };
}

export interface OwnEmailVerification {
    email_set: boolean;
    email_verified: boolean;
    mail: AccountMailStatus;
}

export function decodeOwnEmailVerification(raw: unknown): OwnEmailVerification {
    if (!raw || typeof raw !== 'object') throw new Error('malformed verification state');
    const value = raw as Record<string, unknown>;
    if (typeof value.email_set !== 'boolean' || typeof value.email_verified !== 'boolean') {
        throw new Error('malformed verification state');
    }
    return { email_set: value.email_set, email_verified: value.email_verified, mail: decodeAccountMailStatus(value.mail) };
}

/** What the server answered about one invitation send. */
export interface InvitationOutcome {
    id?: number;
    delivery: 'sent' | 'link' | 'none';
    reason?: string;
    link_path?: string;
    expires_at?: string;
}

export function decodeInvitationOutcome(raw: unknown): InvitationOutcome | null {
    if (!raw || typeof raw !== 'object') return null;
    const value = raw as Record<string, unknown>;
    if (value.delivery !== 'sent' && value.delivery !== 'link' && value.delivery !== 'none') return null;
    return {
        id: typeof value.id === 'number' ? value.id : undefined,
        delivery: value.delivery,
        reason: typeof value.reason === 'string' ? value.reason : undefined,
        link_path: typeof value.link_path === 'string' && value.link_path.startsWith(ACCOUNT_LINK_PREFIX) ? value.link_path : undefined,
        expires_at: typeof value.expires_at === 'string' ? value.expires_at : undefined,
    };
}

/** The full link to copy: the inviter's own panel address plus the path. */
export function invitationLink(outcome: InvitationOutcome, origin: string): string {
    return outcome.link_path ? origin + outcome.link_path : '';
}

export interface PendingInvitation {
    id: number;
    user_id: number;
    username: string;
    email: string;
    account_type: string;
    role: string;
    state: 'pending' | 'expired' | 'superseded';
    delivery: string;
    created_at: string;
    expires_at: string;
}

export function decodePendingInvitations(raw: unknown): PendingInvitation[] {
    const list = raw && typeof raw === 'object' ? (raw as { invitations?: unknown }).invitations : undefined;
    if (list === null) return [];
    if (!Array.isArray(list)) throw new Error('malformed invitation list');
    return list.filter((item): item is PendingInvitation =>
        !!item && typeof item === 'object'
        && typeof (item as PendingInvitation).id === 'number'
        && typeof (item as PendingInvitation).username === 'string'
        && ['pending', 'expired', 'superseded'].includes((item as PendingInvitation).state));
}

/** A date for the screen, in the interface language, without the time zone noise. */
export function shownDate(iso: string | undefined, locale: string): string {
    if (!iso) return '';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return iso;
    return date.toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' });
}

export function postJSON(url: string, body: unknown, method = 'POST'): Promise<Response> {
    return fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        cache: 'no-store',
        credentials: 'same-origin',
    });
}
