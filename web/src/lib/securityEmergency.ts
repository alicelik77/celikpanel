// The security emergency's shared contract (D-043): the list endpoint, its
// decoding, and which Components row and page belong to each emergency
// component. Kept apart from the section itself (components/SecurityEmergency)
// so the Components list and the Security page can show a short badge or a
// link without carrying the whole section (alpha.120, owner's Boston screens).
//
// Güvenlik acil durumunun ortak sözleşmesi: liste ucu, çözümleme ve her acil
// durum bileşeninin Bileşenler satırı ile sayfası. Bölümün kendisinden ayrıdır;
// liste ve Güvenlik sayfası bütün bölümü taşımadan rozet ya da bağlantı gösterir.

export const SECURITY_EMERGENCIES_URL = '/api/v1/security-emergencies';

export type EmergencyComponent = 'php' | 'node' | 'mariadb' | 'postgresql';

export interface EmergencySite {
    domain_id: number;
    domain: string;
    customer: string;
    reseller?: string;
    maintenance: boolean;
    moved_to?: string;
}

export type EmergencyStep = 'stop' | 'move' | 'remove' | 'reenable' | 'cancel' | 'continue';

export interface EmergencyDetail {
    emergency: { id: string; component: EmergencyComponent; version: string; name: string; reason: string; stage: string; withdrawn: boolean; recommended?: string };
    guidance: { result: string; reason: string; who_acts: string; next_action: string; resumes: string };
    operation: null | { id: string; kind: string; state: string; done: number; total: number; last_error?: string; effects: { kind: string; target: string; state: string; detail?: string }[] };
    preview: { sites: EmergencySite[]; customers: { id: number; name: string; sites: number }[]; resellers: { id: number; name: string; sites: number }[]; offered_by: string[]; notes: string[] };
    notices: { total: number; seen: number; mail_sent: number; mail_pending: number; mail_failed: number; no_address: number; mail_not_available: number };
    confirm_name: string;
    alternatives: string[];
    can: Partial<Record<EmergencyStep, boolean>>;
}

export function decodeEmergencyDetail(raw: unknown): EmergencyDetail {
    const body = raw as EmergencyDetail | null;
    if (!body || typeof body !== 'object' || !body.emergency || typeof body.emergency.id !== 'string' || !body.guidance || !body.preview || !Array.isArray(body.preview.sites)) {
        throw new Error('shape');
    }
    return { ...body, alternatives: Array.isArray(body.alternatives) ? body.alternatives : [], can: body.can ?? {} };
}

export function decodeEmergencyList(raw: unknown): EmergencyDetail[] {
    const body = raw as { emergencies?: unknown[] } | null;
    if (!body || !Array.isArray(body.emergencies)) throw new Error('shape');
    return body.emergencies.map(decodeEmergencyDetail);
}

/** The open emergencies of one component. */
export function openEmergenciesFor(list: EmergencyDetail[], component: EmergencyComponent): EmergencyDetail[] {
    return list.filter((item) => item.emergency.component === component && item.emergency.stage !== 'resolved');
}

/**
 * The four components the emergency covers, in the order the Security page
 * lists them, with the Components row (catalogue id) that carries each one.
 * They are the programs customer sites run on directly.
 * Acil durumun kapsadığı dört bileşen ve her birinin Bileşenler satırı.
 */
export const EMERGENCY_COMPONENTS: { component: EmergencyComponent; serviceId: string; name: string }[] = [
    { component: 'php', serviceId: 'php-fpm', name: 'PHP' },
    { component: 'node', serviceId: 'node', name: 'Node.js' },
    { component: 'mariadb', serviceId: 'mariadb', name: 'MariaDB' },
    { component: 'postgresql', serviceId: 'postgresql', name: 'PostgreSQL' },
];

/** The anchor of the section on a component's own page. */
export const EMERGENCY_ANCHOR = 'security-emergency';

/** Where a component's emergency section is: its own page, at the section. */
export function emergencyPath(serviceId: string): string {
    return `/components/${serviceId}#${EMERGENCY_ANCHOR}`;
}

/** The catalogue ids whose row has an open emergency. */
export function rowsWithOpenEmergency(list: EmergencyDetail[]): Set<string> {
    const rows = new Set<string>();
    for (const entry of EMERGENCY_COMPONENTS) {
        if (openEmergenciesFor(list, entry.component).length > 0) rows.add(entry.serviceId);
    }
    return rows;
}
