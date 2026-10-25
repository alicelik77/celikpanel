// PostgreSQL majors side by side (v0.1.0-alpha.108): GET
// /api/v1/service/postgresql/clusters and its shapes. Every PostgreSQL major
// runs as its own cluster on its own port; another major is installed beside
// the old one, a cluster's port is changed with proof, a database is copied
// with per-table row counts as proof and the panel's record switched only as
// its own step afterwards.
// PostgreSQL ana sürümleri yan yana: kümeler, kayıtlar ve taşımalar.
export const PG_CLUSTERS_URL = '/api/v1/service/postgresql/clusters';
export const PG_MOVES_URL = '/api/v1/service/postgresql/moves';

export interface PGCluster {
    version: string;
    name: string;
    port: number;
    configured_port: number;
    status: string;
    online: boolean;
    data_directory: string;
    config_path: string;
    databases: string[];
    databases_known: boolean;
    databases_error?: string;
}

export interface PGOffer {
    major: string;
    package: string;
    candidate?: string;
    origin?: '' | 'distribution' | 'pgdg';
    needs_repository: boolean;
    port: number;
    kept_cluster?: boolean;
}

export interface PGRecord {
    database_id: number;
    name: string;
    domain?: string;
    subscription_id: number;
    server_id: number;
    host: string;
    port: number;
    users: string[];
    legacy?: boolean;
}

export interface PGTableCount {
    schema: string;
    table: string;
    source: number;
    target: number;
}

export interface PGProof {
    equal: boolean;
    tables_total: number;
    tables: PGTableCount[];
    mismatches: number;
    source_version?: string;
    target_version?: string;
    roles_created: string[];
    roles_kept: string[];
    owner?: string;
    size_bytes?: number;
    seconds?: number;
}

export type PGMoveState = 'copied' | 'switched' | 'switched_back' | 'source_dropped' | 'copy_dropped';

export interface PGMove {
    id: number;
    database_id: number | null;
    database: string;
    source_cluster: string;
    target_cluster: string;
    source_port: number;
    target_port: number;
    state: PGMoveState;
    proof: PGProof;
    created_at: string;
    updated_at: string;
}

export interface PGMismatch {
    cluster: string;
    record_port: number;
    cluster_port: number;
    records: number;
}

export interface PGClusters {
    supported: boolean;
    clusters: PGCluster[];
    offers: PGOffer[];
    next_port: number;
    used_ports: number[];
    creates_cluster: boolean;
    repository_on: boolean;
    records: Record<string, PGRecord[]>;
    unplaced: PGRecord[];
    mismatches: PGMismatch[];
    moves: PGMove[];
}

const isArray = (value: unknown): value is unknown[] => Array.isArray(value);

export function decodePGClusters(raw: unknown): PGClusters {
    const body = raw as Record<string, unknown> | null;
    if (!body || body.state !== 'read' || typeof body.supported !== 'boolean' || !isArray(body.clusters) || !isArray(body.offers) || !isArray(body.moves)) {
        throw new Error('shape');
    }
    for (const cluster of body.clusters as Record<string, unknown>[]) {
        if (!cluster || typeof cluster.version !== 'string' || typeof cluster.name !== 'string' || typeof cluster.port !== 'number' || !isArray(cluster.databases)) {
            throw new Error('shape');
        }
    }
    return {
        supported: body.supported,
        clusters: body.clusters as PGCluster[],
        offers: body.offers as PGOffer[],
        next_port: typeof body.next_port === 'number' ? body.next_port : 0,
        used_ports: isArray(body.used_ports) ? (body.used_ports as number[]) : [],
        creates_cluster: body.creates_cluster !== false,
        repository_on: body.repository_on === true,
        records: body.records && typeof body.records === 'object' ? (body.records as Record<string, PGRecord[]>) : {},
        unplaced: isArray(body.unplaced) ? (body.unplaced as PGRecord[]) : [],
        mismatches: isArray(body.mismatches) ? (body.mismatches as PGMismatch[]) : [],
        moves: body.moves as PGMove[],
    };
}

export const pgRef = (cluster: Pick<PGCluster, 'version' | 'name'>) => `${cluster.version}/${cluster.name}`;

export interface PGSession {
    database: string;
    user: string;
    client: string;
    application?: string;
    count: number;
}

export interface PGPortPreview {
    preview: {
        config_path: string;
        config_sha256: string;
        line_before: string;
        line_after: string;
        from: number;
        to: number;
        sessions: PGSession[];
        sessions_known: boolean;
        phppgadmin_port?: number;
        refusal_code?: string;
        refusal?: string;
    };
    records: PGRecord[];
}

export function decodePGPortPreview(raw: unknown): PGPortPreview {
    const body = raw as Record<string, unknown> | null;
    const preview = body?.preview as Record<string, unknown> | undefined;
    if (!body || body.state !== 'read' || !preview || typeof preview.from !== 'number' || !isArray(preview.sessions)) throw new Error('shape');
    return { preview: preview as PGPortPreview['preview'], records: isArray(body.records) ? (body.records as PGRecord[]) : [] };
}

export function pgPortPreviewURL(cluster: Pick<PGCluster, 'version' | 'name'>, port: number): string {
    return `${PG_CLUSTERS_URL}/port-preview?version=${encodeURIComponent(cluster.version)}&name=${encodeURIComponent(cluster.name)}&port=${port}`;
}

// The answer of a POST, read once: the body, and the error sentence and code.
export async function pgPost(url: string, body: unknown): Promise<{ ok: boolean; body: Record<string, unknown> }> {
    const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const raw = await res.text();
    let parsed: Record<string, unknown>;
    try {
        parsed = JSON.parse(raw) as Record<string, unknown>;
    } catch {
        parsed = { error: raw.slice(0, 200) };
    }
    return { ok: res.ok, body: parsed };
}

// A port a person typed: a whole number from 1024 to 65535.
export function pgPortValue(text: string): number | null {
    if (!/^[0-9]{4,5}$/.test(text.trim())) return null;
    const port = Number(text.trim());
    return port >= 1024 && port <= 65535 ? port : null;
}
