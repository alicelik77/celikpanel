// The infrastructure DNS zones of this server (2026-10-16): the zone server
// setup created on the primary for the panels' own names, listed beside the
// domains and edited with the domain DNS editor. Administrator only; the
// decoder refuses an answer that is not the contract (lib/remote.ts).
// Sunucunun altyapi DNS bolgeleri: kurulumun panellerin kendi adlari icin
// olusturdugu bolge; alan adlarinin yaninda listelenir.

import type { Remote } from './remote';

export const INFRASTRUCTURE_ZONES_URL = '/api/v1/dns/infrastructure-zones';

export interface InfrastructureZone {
    zone: string;
    /** A website added later for the same name; it is then listed as that website. */
    domainId: number | null;
    records: number;
    peerRecords: number;
}

export function decodeInfrastructureZones(raw: unknown): InfrastructureZone[] {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const zones = (raw as Record<string, unknown>).zones;
    if (!Array.isArray(zones)) throw new Error('list');
    return zones.map((item) => {
        if (!item || typeof item !== 'object' || Array.isArray(item)) throw new Error('item');
        const zone = item as Record<string, unknown>;
        if (typeof zone.zone !== 'string' || zone.zone === '' || zone.kind !== 'infrastructure'
            || typeof zone.records !== 'number' || typeof zone.peer_records !== 'number'
            || (zone.domain_id !== undefined && (typeof zone.domain_id !== 'number' || zone.domain_id <= 0))) throw new Error('field');
        return { zone: zone.zone, domainId: typeof zone.domain_id === 'number' ? zone.domain_id : null, records: zone.records, peerRecords: zone.peer_records };
    });
}

export const infrastructureZonePath = (zone: string) => `/domains/infrastructure/${encodeURIComponent(zone)}`;

// The Domains count the Dashboard tile and the sidebar badge show: the
// domains plus the DNS-only infrastructure zones the Domains page lists
// beside them (a zone that became a website is that website). It is known
// only when both reads are; while either is read it is loading, and when one
// could not be read it is unknown, carrying the earlier count only when both
// had one. A count is never "domains + 0" for a zone list nobody read
// (alpha.95 review). Without the zone read (not an administrator) it is the
// domains alone.
// Pano kutucuğu ile kenar çubuğu rozetinin Alan adları sayısı: alan adları ve
// Alan adları sayfasının listelediği yalnız-DNS altyapı bölgeleri. İki okuma
// da bilindiğinde bilinir.
export function domainsCount(domains: Remote<unknown[]>, zones: Remote<InfrastructureZone[]> | null): Remote<number> {
    const standalone = (list: InfrastructureZone[]) => list.filter((zone) => zone.domainId === null).length;
    if (zones === null) {
        if (domains.state === 'loading') return domains;
        if (domains.state === 'known') return { state: 'known', value: domains.value.length, observedAt: domains.observedAt };
        const { previous, ...rest } = domains;
        return previous ? { ...rest, previous: { value: previous.value.length, observedAt: previous.observedAt } } : rest;
    }
    if (domains.state === 'known' && zones.state === 'known') {
        return { state: 'known', value: domains.value.length + standalone(zones.value), observedAt: Math.min(domains.observedAt, zones.observedAt) };
    }
    const failed = domains.state === 'unknown' ? domains : zones.state === 'unknown' ? zones : null;
    if (failed === null) return { state: 'loading' };
    const earlierDomains = domains.state === 'known' ? { value: domains.value, observedAt: domains.observedAt } : domains.state === 'unknown' ? domains.previous : undefined;
    const earlierZones = zones.state === 'known' ? { value: zones.value, observedAt: zones.observedAt } : zones.state === 'unknown' ? zones.previous : undefined;
    const previous = earlierDomains && earlierZones
        ? { value: earlierDomains.value.length + standalone(earlierZones.value), observedAt: Math.min(earlierDomains.observedAt, earlierZones.observedAt) }
        : undefined;
    return { state: 'unknown', reason: failed.reason, ...(failed.status !== undefined ? { status: failed.status } : {}), ...(previous ? { previous } : {}) };
}
