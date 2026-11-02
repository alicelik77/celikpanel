// Does every name a certificate request would carry point to this server?
//
// The SSL/TLS screen reads GET /api/v1/domains/<id>/connection?names=certificate
// (the Domain Overview's live DNS check, plus where each name of the request
// resolves) before it offers "Get certificate". The owner's rule (2 Nov 2026):
// "If the domain is not on the server, the certificate cannot be obtained
// either." Three answers, never two: the names point here, a name is known not
// to, or the check could not tell. Only a known negative blocks; unknown allows
// the attempt with a warning (D-024), because a failed lookup is not evidence.
//
// Bir sertifika isteğinin taşıyacağı her ad bu sunucuyu gösteriyor mu? Üç yanıt:
// adlar buraya bakıyor, bir ad bakmıyor (bilinen olumsuz) ya da kontrol
// söyleyemedi. Yalnız bilinen olumsuz engeller; bilinmeyen uyarıyla denemeye
// izin verir.

export type CertificateNameState = 'here' | 'elsewhere' | 'unknown';

export interface CertificateNameCheck {
    name: string;
    kind: 'site' | 'mail';
    state: CertificateNameState;
    ips: string[];
}

export interface CertificateReach {
    server_ip: string;
    nameservers: string[];
    live_nameservers: string[];
    certificate_names: CertificateNameCheck[];
}

const states = ['here', 'elsewhere', 'unknown'];

function strings(value: unknown): string[] {
    if (value === null || value === undefined) return [];
    if (!Array.isArray(value)) throw new Error('list');
    return value.filter((item): item is string => typeof item === 'string');
}

// The answer as the screen uses it. An answer that names no check for the
// domain is not "everything points here": it is not the contract, so the read
// is unknown.
export function decodeCertificateReach(raw: unknown): CertificateReach {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (typeof body.server_ip !== 'string' || !Array.isArray(body.certificate_names)) throw new Error('field');
    const checks = body.certificate_names.map((item): CertificateNameCheck => {
        const entry = item as Record<string, unknown> | null;
        if (
            !entry || typeof entry.name !== 'string' ||
            (entry.kind !== 'site' && entry.kind !== 'mail') ||
            typeof entry.state !== 'string' || !states.includes(entry.state)
        ) {
            throw new Error('name');
        }
        return { name: entry.name, kind: entry.kind, state: entry.state as CertificateNameState, ips: strings(entry.ips) };
    });
    return {
        server_ip: body.server_ip,
        nameservers: strings(body.nameservers),
        live_nameservers: strings(body.live_nameservers),
        certificate_names: checks,
    };
}

export type CertificateReachGate =
    /** Not read yet: the request waits for the answer. */
    | { kind: 'checking' }
    /** Every name points here. */
    | { kind: 'open' }
    /** The check could not tell for these names; the attempt is allowed. */
    | { kind: 'unknown'; names: string[] }
    /** These names are known not to point here; the request is not offered. */
    | {
          kind: 'blocked';
          names: string[];
          /** Only mail.<domain> fails: the request can go ahead without it. */
          mailOnly: boolean;
          /** The domain itself is among them (so its nameservers matter). */
          domainFails: boolean;
          serverIp: string;
          /** This server's nameservers, only where the panel serves the zone. */
          nameservers: string[];
          /** The nameservers the world sees now, when they are not this server's. */
          liveNameservers: string[];
      };

const sameSet = (a: string[], b: string[]) => {
    const left = new Set(a.map((n) => n.toLowerCase().replace(/\.$/, '')));
    const right = new Set(b.map((n) => n.toLowerCase().replace(/\.$/, '')));
    return left.size === right.size && [...left].every((n) => right.has(n));
};

/**
 * The gate for one request. `reach` is null while the read has not answered;
 * `readFailed` is a read that could not be made (unknown, not blocked).
 * Mail names count only when the request includes mail.
 */
export function certificateReachGate(
    reach: CertificateReach | null,
    readFailed: boolean,
    domainName: string,
    includeMail: boolean,
): CertificateReachGate {
    if (readFailed) return { kind: 'unknown', names: [domainName] };
    if (!reach) return { kind: 'checking' };
    const named = reach.certificate_names.filter((check) => check.kind === 'site' || includeMail);
    const failing = named.filter((check) => check.state === 'elsewhere');
    if (failing.length > 0) {
        const domainFails = failing.some((check) => check.name.toLowerCase() === domainName.toLowerCase());
        const live = reach.live_nameservers;
        return {
            kind: 'blocked',
            names: failing.map((check) => check.name),
            mailOnly: failing.every((check) => check.kind === 'mail'),
            domainFails,
            serverIp: reach.server_ip,
            nameservers: reach.nameservers,
            liveNameservers: reach.nameservers.length > 0 && sameSet(live, reach.nameservers) ? [] : live,
        };
    }
    const unknown = named.filter((check) => check.state === 'unknown');
    if (unknown.length > 0 || named.length === 0) {
        return { kind: 'unknown', names: unknown.length > 0 ? unknown.map((check) => check.name) : [domainName] };
    }
    return { kind: 'open' };
}
