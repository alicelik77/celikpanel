// What an install brings (alpha.105, D-034 amendment: recommend, warn,
// never impose). GET /api/v1/service/install-preview carries the package
// manager's own simulation: the packages it would install, the notable ones
// among them (another PHP version, Apache, another catalogue component, a
// database server) that the owner sees by name and confirms first, and a
// chosen PHP version that cannot serve its tool. A failed read is
// "unknown": the install is still checked again on the server, and it stops
// if it would bring anything not confirmed.
// Bir kurulumun getirdikleri: paket yöneticisinin benzetimi, dikkat çeken
// paketler ve aracı çalıştıramayacak PHP sürümü.

export type NotableReason = 'new_php_version' | 'apache' | 'component' | 'database_server' | 'upgrade' | 'vendor_replaces_distribution' | 'vendor_shared';

export interface NotableInstall {
    package: string;
    reason: NotableReason;
    component?: string;
    version?: string;
    /** alpha.107: an installed package's version now and after. */
    from?: string;
    to?: string;
}

export interface InstallRefusal {
    id: string;
    code: string;
    version?: string;
    packages: string[];
}

export interface InstallPreview {
    state: 'known' | 'busy' | 'unsupported' | 'unknown';
    installs: string[];
    notable: NotableInstall[];
    refusals: InstallRefusal[];
    /** alpha.107 (D-038 §8): data an earlier install of a database engine left. */
    existingData?: { component: string; path: string }[];
}

export interface InstallStep {
    id: string;
    package?: string;
    phpVersion?: string;
}

const REASONS: readonly string[] = ['new_php_version', 'apache', 'component', 'database_server', 'upgrade', 'vendor_replaces_distribution', 'vendor_shared'];
const STATES: readonly string[] = ['known', 'busy', 'unsupported', 'unknown'];
const PACKAGE = /^[a-z0-9][a-z0-9+.:-]{0,160}$/;
const strings = (value: unknown): string[] | null =>
    Array.isArray(value) && value.every((item) => typeof item === 'string') ? (value as string[]) : null;

// decodeInstallPreview throws on an answer that is not the contract; the
// screen then says "could not check".
export function decodeInstallPreview(raw: unknown): InstallPreview {
    if (!raw || typeof raw !== 'object') throw new Error('shape');
    const body = raw as Record<string, unknown>;
    const installs = strings(body.installs);
    if (typeof body.state !== 'string' || !STATES.includes(body.state) || !installs || !Array.isArray(body.notable) || !Array.isArray(body.refusals)) {
        throw new Error('shape');
    }
    const notable: NotableInstall[] = [];
    for (const item of body.notable) {
        const value = item as Record<string, unknown> | null;
        if (!value || typeof value.package !== 'string' || !PACKAGE.test(value.package) || typeof value.reason !== 'string') throw new Error('shape');
        // A reason this page has no words for is said as "another component".
        const reason = (REASONS.includes(value.reason) ? value.reason : 'component') as NotableReason;
        notable.push({
            package: value.package,
            reason,
            ...(typeof value.component === 'string' ? { component: value.component } : {}),
            ...(typeof value.version === 'string' ? { version: value.version } : {}),
            ...(typeof value.from === 'string' ? { from: value.from } : {}),
            ...(typeof value.to === 'string' ? { to: value.to } : {}),
        });
    }
    const refusals: InstallRefusal[] = [];
    for (const item of body.refusals) {
        const value = item as Record<string, unknown> | null;
        if (!value || typeof value.id !== 'string' || typeof value.code !== 'string') throw new Error('shape');
        refusals.push({
            id: value.id,
            code: value.code,
            ...(typeof value.version === 'string' ? { version: value.version } : {}),
            packages: strings(value.packages) ?? [],
        });
    }
    const existingData: { component: string; path: string }[] = [];
    if (Array.isArray(body.existing_data)) {
        for (const item of body.existing_data) {
            const value = item as Record<string, unknown> | null;
            if (value && typeof value.component === 'string' && typeof value.path === 'string') existingData.push({ component: value.component, path: value.path });
        }
    }
    return { state: body.state as InstallPreview['state'], installs, notable, refusals, ...(existingData.length ? { existingData } : {}) };
}

export function installPreviewURL(steps: readonly InstallStep[]): string | null {
    if (steps.length === 0) return null;
    const query = steps
        .map((step) => {
            let value = step.id;
            if (step.package || step.phpVersion) value += `:${step.package ?? ''}`;
            if (step.phpVersion) value += `:${step.phpVersion}`;
            return `step=${encodeURIComponent(value)}`;
        })
        .join('&');
    return `/api/v1/service/install-preview?${query}`;
}

// The notable packages grouped as they are said: one line per PHP version,
// per component and per reason.
export interface NotableGroup {
    reason: NotableReason;
    key: string;
    version?: string;
    component?: string;
    packages: string[];
}

export function notableGroups(notable: readonly NotableInstall[]): NotableGroup[] {
    const groups: NotableGroup[] = [];
    for (const item of notable) {
        const key = `${item.reason}:${item.reason === 'new_php_version' ? item.version ?? '' : item.reason === 'apache' ? '' : item.component ?? ''}`;
        let group = groups.find((candidate) => candidate.key === key);
        if (!group) {
            group = {
                reason: item.reason,
                key,
                ...(item.version ? { version: item.version } : {}),
                ...(item.component ? { component: item.component } : {}),
                packages: [],
            };
            groups.push(group);
        }
        // An upgrade says its versions: "libssl3 3.5.1-1 → 3.5.1-1+deb13u1".
        group.packages.push((item.reason === 'upgrade' || item.reason === 'vendor_shared') && item.from && item.to ? `${item.package} ${item.from} → ${item.to}` : item.package);
    }
    return groups;
}

// A vendor repository replacing a distribution package is never confirmed
// (alpha.107, D-038 §12): the server refuses it whatever the owner ticks.
export function hasVendorReplacement(notable: readonly NotableInstall[]): boolean {
    return notable.some((item) => item.reason === 'vendor_replaces_distribution');
}

// The upstream part of a package manager's version ("2:8.4+96" → "8.4",
// "1.26.3-3+deb13u1" → "1.26.3", "4:5.2.2-really+dfsg-1" → "5.2.2").
export function shortCandidate(version: string): string {
    const withoutEpoch = version.replace(/^[0-9]+:/, '');
    const match = /^[0-9]+(?:\.[0-9]+)*/.exec(withoutEpoch);
    return match ? match[0] : '';
}

// "php8.3-fpm" → "8.3"; "postgresql-17" → "17".
export function versionOfPackage(pkg: string): string {
    const php = /^php([0-9]+\.[0-9]+)-/.exec(pkg);
    if (php) return php[1];
    const numbered = /-([0-9]+(?:\.[0-9]+)?)$/.exec(pkg);
    return numbered ? numbered[1] : pkg;
}

// The database tools that come from the distribution and are built for its
// own PHP.
export const DISTRIBUTION_PHP_TOOLS: readonly string[] = ['phpmyadmin', 'phppgadmin'];
