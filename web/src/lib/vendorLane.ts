// The vendor lane (v0.1.0-alpha.108, D-038 amendment "two lanes"). By default
// a server uses its operating system's own packages. A vendor repository (Sury
// for PHP, PGDG for PostgreSQL) is the administrator's informed choice and is
// enabled only as one shown-and-confirmed operation: the repository, its
// confinement and its origin among the operating system's automatic updates,
// together. GET /api/v1/repo/lane?service_id= is the plan; POST with the
// plan's SHA-256 writes it.
//
// Satıcı yolu: dış depo yalnız deposu, sınırlaması ve otomatik güncellemesiyle
// birlikte, gösterilip onaylanan tek işlemle açılır.

export type LaneStepName = 'repository' | 'confinement' | 'updates';
export type LaneStepState = 'done' | 'owner' | 'todo' | 'blocked';

export interface LaneStep {
    step: LaneStepName | string;
    state: LaneStepState | string;
    reason?: string;
    path?: string;
    text?: string;
    sha256?: string;
    detail?: string;
}

export interface LanePlan {
    repo_id: string;
    name?: string;
    origin?: string;
    enabled: boolean;
    steps: LaneStep[];
    sha256?: string;
    ready: boolean;
    kept?: string[];
}

export type LaneRead = { state: 'none' } | { state: 'read'; plan: LanePlan };

export const laneURL = (serviceId: string): string => `/api/v1/repo/lane?service_id=${encodeURIComponent(serviceId)}`;

export function decodeLaneRead(raw: unknown): LaneRead {
    const body = raw as { state?: string; plan?: LanePlan; error?: string[] } | null;
    if (body?.state === 'none') return { state: 'none' };
    if (!body || body.state !== 'read' || !body.plan || !Array.isArray(body.plan.steps) || typeof body.plan.ready !== 'boolean') {
        throw new Error(body?.state === 'failed' ? (body.error ?? []).join(' ') || 'failed' : 'shape');
    }
    return { state: 'read', plan: body.plan };
}

/** The steps still to be written or blocked: what the card names as missing. */
export function missingSteps(plan: LanePlan): LaneStep[] {
    return plan.steps.filter((step) => step.state === 'todo' || step.state === 'blocked');
}

// The component a vendor lane belongs to, for the security updates card's
// direct action (owner finding: "ne ekleyeyim").
export const LANE_SERVICE: Record<string, string> = { sury: 'php-fpm', pgdg: 'postgresql' };

// Blocking reasons the dialog explains; anything else reads as "unknown".
export const LANE_REASONS: ReadonlySet<string> = new Set([
    'automatic_off', 'automatic_not_installed', 'drop_in_edited', 'origin_mismatch', 'apt_config_unreadable', 'lists_unreadable',
]);

// Refusal codes the dialog says in words.
export const LANE_CODES: ReadonlySet<string> = new Set([
    'LANE_PLAN_CHANGED', 'LANE_BLOCKED', 'LANE_KEY_UNTRUSTED', 'LANE_ENABLE_FAILED', 'LANE_PARTIAL', 'LANE_INVALID', 'LANE_UNSUPPORTED', 'LANE_RESULT_UNKNOWN',
]);

// Who provides a version (D-038 §11): the distribution or a vendor lane.
// The support table's provider says it: "distribution" is the operating
// system's own package; "upstream" needs the vendor repository.
export const LANE_FOR_COMPONENT: Record<string, { repo: string; name: string }> = {
    'php-fpm': { repo: 'sury', name: 'Sury PHP' },
    postgresql: { repo: 'pgdg', name: 'PGDG' },
};
