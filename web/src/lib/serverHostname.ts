// Does this server's own host name resolve in public DNS? (alpha.121,
// Frankfurt 4 Nov: frankfurt.celikhost.com had no address record.) The
// Panel's answer is read-only and cached; when the name is missing and this
// panel serves its zone, it proposes the exact records and the existing
// record endpoint that adds them after the owner's review.
// Sunucunun kendi adı genel DNS'te çözülüyor mu; salt-okur yanıt.

export const SERVER_HOSTNAME_DNS_URL = '/api/v1/dns/server-hostname';

export type ServerHostnameState = 'resolves' | 'missing' | 'other_address' | 'unknown' | 'not_a_domain_name';

export interface ServerHostnameRecord {
    name: string;
    type: 'A' | 'AAAA';
    content: string;
    ttl: number;
}

export interface ServerHostnameZone {
    name: string;
    kind: 'domain' | 'infrastructure';
    recordsURL: string;
    editorPath: string;
    missing: ServerHostnameRecord[];
    present: string[];
}

export interface ServerHostnameDNS {
    hostname: string;
    state: ServerHostnameState;
    addresses: string[];
    ipv4: string;
    ipv6: string;
    zone: ServerHostnameZone | null;
}

const STATES: readonly string[] = ['resolves', 'missing', 'other_address', 'unknown', 'not_a_domain_name'];
// Only the panel's own record endpoints are followed (as for webmail's address).
const RECORDS_URL = /^\/api\/v1\/(?:domains\/\d+\/dns\/records|dns\/infrastructure-zones\/[a-z0-9.-]+\/records)$/;
const EDITOR_PATH = /^\/domains(?:\/infrastructure)?(?:\/[a-z0-9.-]+)?$/;
const NAME = /^[a-z0-9.-]{1,253}$/;
const ADDRESS = /^[0-9a-f.:]{2,45}$/i;

const text = (value: unknown): string => (typeof value === 'string' ? value : '');

export function decodeServerHostnameDNS(raw: unknown): ServerHostnameDNS {
    const body = raw as Record<string, unknown> | null;
    if (!body || typeof body !== 'object' || typeof body.state !== 'string' || !STATES.includes(body.state) || typeof body.hostname !== 'string') {
        throw new Error('shape');
    }
    const addresses = Array.isArray(body.addresses) ? body.addresses.filter((value): value is string => typeof value === 'string' && ADDRESS.test(value)).slice(0, 8) : [];
    let zone: ServerHostnameZone | null = null;
    const z = body.zone as Record<string, unknown> | null | undefined;
    if (z && typeof z === 'object' && typeof z.name === 'string' && NAME.test(z.name)
        && typeof z.records_path === 'string' && RECORDS_URL.test(z.records_path)
        && typeof z.editor_path === 'string' && EDITOR_PATH.test(z.editor_path)) {
        const missing: ServerHostnameRecord[] = [];
        for (const item of Array.isArray(z.missing) ? z.missing as Record<string, unknown>[] : []) {
            if (item && (item.type === 'A' || item.type === 'AAAA') && typeof item.name === 'string' && NAME.test(item.name)
                && typeof item.content === 'string' && ADDRESS.test(item.content) && typeof item.ttl === 'number') {
                missing.push({ name: item.name, type: item.type, content: item.content, ttl: item.ttl });
            }
        }
        const present = Array.isArray(z.present) ? z.present.filter((value): value is string => typeof value === 'string').slice(0, 8) : [];
        zone = {
            name: z.name,
            kind: z.kind === 'infrastructure' ? 'infrastructure' : 'domain',
            recordsURL: z.records_path,
            editorPath: z.editor_path,
            missing,
            present,
        };
    }
    return {
        hostname: body.hostname,
        state: body.state as ServerHostnameState,
        addresses,
        ipv4: text(body.ipv4),
        ipv6: text(body.ipv6),
        zone,
    };
}

// The record as a zone file line, as the review shows it.
export function serverHostnameRecordText(record: ServerHostnameRecord): string {
    return `${record.name}. ${record.ttl} IN ${record.type} ${record.content}`;
}

// The domain part of a host name ("frankfurt.celikhost.com" → "celikhost.com").
export function hostnameDomain(hostname: string): string {
    const labels = hostname.split('.');
    return labels.length > 2 ? labels.slice(1).join('.') : hostname;
}
