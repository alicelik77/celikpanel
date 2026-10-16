// The infrastructure DNS zones of this server (2026-10-16): the zone server
// setup created on the primary for the panels' own names, listed beside the
// domains and edited with the domain DNS editor. Administrator only; the
// decoder refuses an answer that is not the contract (lib/remote.ts).
// Sunucunun altyapi DNS bolgeleri: kurulumun panellerin kendi adlari icin
// olusturdugu bolge; alan adlarinin yaninda listelenir.

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
