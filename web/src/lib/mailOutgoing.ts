// A domain's mailboxes and their hourly sending limits as the server answers
// them (`GET /api/v1/domains/{id}/mail/outgoing`; cmd/panel/mail_outgoing.go),
// and the one decoder every reader of that address shares (lib/remote.ts).
// The mail list reads it to know whether limits are really applied on this
// server; the limit lines, the problem line and the limit dialog read it too.
//
// Bir alan adının posta kutuları ve saatlik gönderim sınırları; bu adresin
// bütün okuyucularının paylaştığı tek çözücü.

export interface OutgoingPaused {
    paused_at: string;
    reason: 'spike' | 'refusals' | 'manual' | string;
    detail?: string;
}

export interface OutgoingEffective {
    /** The limit in force now, recipients per hour; 0 = no limit. */
    limit: number;
    mode: 'default' | 'custom' | 'unlimited';
    custom?: number;
    default: number;
    ceiling: number;
    /** While a new mailbox's first hours run, the time they end. */
    new_until?: string;
}

export interface OutgoingMailbox {
    id: number;
    address: string;
    effective: OutgoingEffective;
    used_hour: number;
    sent_7d: number;
    refused_7d: number;
    paused?: OutgoingPaused;
}

export interface DomainOutgoing {
    enforced: boolean;
    state: 'off' | 'pending' | 'not_enforced' | 'on';
    reason?: string;
    domain_limit: number;
    domain_used_hour: number;
    subscription_limit: number;
    subscription_used_hour: number;
    domain_refused_7d: number;
    subscription_refused_7d: number;
    ceiling: number;
    default: number;
    may_exceed_ceiling: boolean;
    mailboxes: OutgoingMailbox[];
}

export const domainOutgoingURL = (domainId: number) => `/api/v1/domains/${domainId}/mail/outgoing`;

const count = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0);

export function decodeDomainOutgoing(raw: unknown): DomainOutgoing {
    const value = raw as Partial<DomainOutgoing> | null;
    if (!value || typeof value.state !== 'string' || !Array.isArray(value.mailboxes) || typeof value.ceiling !== 'number') {
        throw new Error('outgoing');
    }
    // An older Panel did not send the account's hour and the refusals: none known.
    return {
        ...value as DomainOutgoing,
        subscription_used_hour: count(value.subscription_used_hour),
        domain_refused_7d: count(value.domain_refused_7d),
        subscription_refused_7d: count(value.subscription_refused_7d),
    };
}

/** The limits are really applied on this server (on, and the server proved it counts). */
export const limitsApplied = (view: DomainOutgoing | null | undefined): boolean => !!view && view.state === 'on' && view.enforced;
