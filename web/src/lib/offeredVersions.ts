// Versions offered to customers (alpha.108, D-038 §14): GET
// /api/v1/offered-versions says which version lines the administrator hides
// from customers (PHP "8.4", Node.js "22"), and whether the person reading is
// held to them. Nothing hidden is the default: everything installed is
// offered. A site already on a hidden version keeps it; its page says so.
// Müşteriye sunulan sürümler: yöneticinin müşterilerden gizlediği sürüm hatları.
import { useRemote } from './remote';

export type OfferedComponent = 'php' | 'node';

export interface OfferedVersions {
    hidden: Record<OfferedComponent, string[]>;
    restricted: boolean;
}

export const OFFERED_VERSIONS_URL = '/api/v1/offered-versions';

const isLines = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === 'string');

export function decodeOfferedVersions(raw: unknown): OfferedVersions {
    const body = raw as { hidden?: Record<string, unknown>; restricted?: unknown } | null;
    if (!body || typeof body !== 'object' || !body.hidden || typeof body.hidden !== 'object' || typeof body.restricted !== 'boolean') throw new Error('shape');
    const php = body.hidden.php ?? [];
    const node = body.hidden.node ?? [];
    if (!isLines(php) || !isLines(node)) throw new Error('shape');
    return { hidden: { php, node }, restricted: body.restricted };
}

/** The line a version belongs to, as the server computes it. */
export function offeredLine(component: OfferedComponent, version: string): string {
    const text = version.trim().replace(/^v/, '');
    if (component === 'php') return /^([0-9]{1,2}\.[0-9]{1,2})/.exec(text)?.[1] ?? '';
    return /^([0-9]{1,3})/.exec(text)?.[1] ?? '';
}

/** Whether a version is offered to customers; true while it is not known. */
export function isOffered(offered: OfferedVersions | null, component: OfferedComponent, version: string): boolean {
    if (!offered || !version) return true;
    return !offered.hidden[component].includes(offeredLine(component, version));
}

/** The answer, read once and shared; null until it is known. */
export function useOfferedVersions(): OfferedVersions | null {
    const handle = useRemote(OFFERED_VERSIONS_URL, decodeOfferedVersions, { freshFor: 60_000 });
    return handle.remote.state === 'known' ? handle.remote.value : null;
}
