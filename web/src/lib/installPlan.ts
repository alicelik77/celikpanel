// What a component needs, at component level (alpha.124, D-045 follow-up).
// GET /api/v1/service/install-plan answers from the component dependency map
// against what is installed here: each requirement is installed, installed
// together with it (one operation on the server), unavailable here (with the
// reason) or unknown. Low-level packages are never part of it.
// Bir bileşenin bileşen düzeyinde gerektirdikleri.

export type InstallNeedState = 'installed' | 'together' | 'unavailable' | 'unknown';
export type InstallNeedReason = 'mail_setup' | 'not_offered' | 'conflict' | 'elsewhere';

export interface InstallNeed {
    /** The requirement as the map says it: a component id or a role ("web-server"). */
    need: string;
    /** The component that fills it (installed, to install, or that would). */
    component: string;
    state: InstallNeedState;
    reason?: InstallNeedReason;
    /** For a conflict: the installed component that holds the role. */
    holder?: string;
    /** The component that needs it, when it is not the one being installed. */
    for?: string;
}

export interface InstallPlan {
    target: string;
    requires: InstallNeed[];
    /** Installed first, in this order, then the component. */
    together: string[];
    blocked: boolean;
}

const ID = /^[a-z0-9][a-z0-9-]{0,63}$/;
const STATES: readonly string[] = ['installed', 'together', 'unavailable', 'unknown'];
const REASONS: readonly string[] = ['mail_setup', 'not_offered', 'conflict', 'elsewhere'];
const id = (value: unknown) => (typeof value === 'string' && ID.test(value) ? value : '');

// decodeInstallPlan throws on an answer that is not the contract; the window
// then says the requirements could not be checked.
export function decodeInstallPlan(raw: unknown): InstallPlan {
    const plan = (raw as { plan?: Record<string, unknown> } | null)?.plan;
    if (!plan || typeof plan !== 'object' || !id(plan.target) || !Array.isArray(plan.requires) || !Array.isArray(plan.together) || typeof plan.blocked !== 'boolean') {
        throw new Error('shape');
    }
    const requires: InstallNeed[] = [];
    for (const item of plan.requires) {
        const value = item as Record<string, unknown> | null;
        if (!value || !id(value.need) || typeof value.state !== 'string' || !STATES.includes(value.state)) throw new Error('shape');
        const reason = typeof value.reason === 'string' && REASONS.includes(value.reason) ? value.reason as InstallNeedReason : undefined;
        requires.push({
            need: id(value.need),
            component: id(value.component),
            state: value.state as InstallNeedState,
            ...(reason ? { reason } : {}),
            ...(id(value.holder) ? { holder: id(value.holder) } : {}),
            ...(id(value.for) ? { for: id(value.for) } : {}),
        });
    }
    const together = plan.together.map(id);
    if (together.some((item) => !item)) throw new Error('shape');
    return { target: id(plan.target), requires, together, blocked: plan.blocked };
}

export const installPlanURL = (serviceId: string) => `/api/v1/service/install-plan?id=${encodeURIComponent(serviceId)}`;

// The canonical member list, as the server's operation names it.
export const togetherQualifier = (members: readonly string[]) => [...new Set(members)].sort().join(',');

// The PHP tools whose PHP version is chosen at their install (alpha.124).
export const PHP_VERSION_TOOLS: readonly string[] = ['phpmyadmin', 'phppgadmin', 'roundcube'];

// "8.4" from a PHP version string; '' when it is not one.
const phpLine = (value: string) => /^([0-9]{1,2}\.[0-9]{1,2})/.exec(value)?.[1] ?? '';

// installedPHPVersions: the installed PHP-FPM versions ("8.3", "8.4"),
// newest first.
export function installedPHPVersions(versions: readonly string[] | undefined): string[] {
    const lines = [...new Set((versions ?? []).map(phpLine).filter(Boolean))];
    return lines.sort((a, b) => {
        const [am, an] = a.split('.').map(Number);
        const [bm, bn] = b.split('.').map(Number);
        return bm - am || bn - an;
    });
}

// defaultPHPVersion: the distribution's own version when it is installed,
// otherwise the newest installed one; '' when none is.
export function defaultPHPVersion(installed: readonly string[], distribution: string): string {
    const own = phpLine(distribution);
    if (own && installed.includes(own)) return own;
    return installed[0] ?? '';
}
