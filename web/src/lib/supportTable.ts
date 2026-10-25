// The dated support table (alpha.107, D-038 §11): GET /api/v1/support-table
// says, for every version the repository's table knows, whether it still
// receives security fixes on this server (from the distribution release the
// server runs, or from its upstream), and until when. The status is the
// server's answer; a version the table does not know is "unknown", never
// guessed. Read once and shared by every screen that marks a version.
// Tarihli destek tablosu: her sürüm bu sunucuda güvenlik düzeltmesi alıyor mu,
// ne zamana dek. Bilinmeyen "bilinmiyor"dur.
import { useRemote } from './remote';

export type SupportStatus = 'supported' | 'ends_soon' | 'unsupported' | 'unknown';
export type SupportComponent = 'php' | 'postgresql' | 'mariadb' | 'node';

export interface SupportResult {
    status: SupportStatus;
    until?: string;
    provider?: 'distribution' | 'upstream' | string;
    note?: string;
}

export interface SupportTable {
    checked: string;
    sources: Record<string, string>;
    distro?: string;
    distro_name?: string;
    statuses: Record<string, Record<string, SupportResult>>;
    /** alpha.108 (D-038 §15): version lines tested with this release here. */
    tested?: Record<string, string[]>;
    tested_release?: string;
}

const STATUSES: ReadonlySet<string> = new Set(['supported', 'ends_soon', 'unsupported', 'unknown']);

export function decodeSupportTable(raw: unknown): SupportTable {
    const body = raw as Partial<SupportTable> | null;
    if (!body || typeof body !== 'object' || typeof body.checked !== 'string' || !body.statuses || typeof body.statuses !== 'object') throw new Error('shape');
    for (const versions of Object.values(body.statuses)) {
        for (const result of Object.values(versions ?? {})) {
            if (!result || !STATUSES.has(result.status)) throw new Error('shape');
        }
    }
    return body as SupportTable;
}

// The table's key for a version, as the server computes it: PHP and MariaDB
// "8.4"/"11.8", PostgreSQL and Node.js the major.
export function supportLine(component: SupportComponent, version: string): string {
    const text = version.trim().replace(/^v/, '');
    if (component === 'php' || component === 'mariadb') return /^([0-9]+\.[0-9]+)/.exec(text)?.[1] ?? '';
    return /^([0-9]+)/.exec(text)?.[1] ?? '';
}

export function supportFor(table: SupportTable | null, component: SupportComponent, version: string): SupportResult {
    const line = supportLine(component, version);
    return (line && table?.statuses?.[component]?.[line]) || { status: 'unknown' };
}

export const SUPPORT_TABLE_URL = '/api/v1/support-table';

/** The table, read once per half hour and shared; null until it is known. */
export function useSupportTable(): SupportTable | null {
    const handle = useRemote(SUPPORT_TABLE_URL, decodeSupportTable, { freshFor: 30 * 60_000 });
    return handle.remote.state === 'known' ? handle.remote.value : null;
}
