// The installed package version of a component (v0.1.0-alpha.97, D-036).
//
// The Panel sends, for an installed component that is not a multi-version
// runtime, the version the system package database printed (package_version,
// epoch and revision kept), its upstream part for the row
// (package_version_short), whether its packages report different versions
// (package_versions_differ), the packages themselves (package_versions), and
// whether the last scan found the package manager busy and kept an earlier
// reading (package_version_kept, with package_version_read_at). Everything is
// additive: an older Panel sends none of it.
//
// A version shown is the package database's own answer; unknown stays
// unknown. Anything that is not the contract reads as unknown here, so the
// row says nothing rather than a guess, and the page itself is never refused
// for it.
//
// Bileşenin kurulu paket sürümü: paket veritabanının kendi yanıtı. Sözleşme
// dışı her şey bilinmeyen okunur; satır tahmin değil, hiçbir şey söylemez.

export interface InstalledPackageVersion {
    package: string;
    version: string;
}

export interface PackageVersionFacts {
    /** The full version as printed; empty when the packages differ. */
    full: string;
    /** The upstream part for the row; empty when the packages differ. */
    short: string;
    /** The component's packages report different versions. */
    several: boolean;
    packages: InstalledPackageVersion[];
    /** The package manager was busy at the last scan; this is an earlier reading. */
    kept: boolean;
    /** When the versions were read (RFC 3339), or ''. */
    readAt: string;
    /** The kept reading is more than a day old: the page adds a short line. */
    keptLong: boolean;
}

const versionText = /^[0-9A-Za-z][0-9A-Za-z.+~:_-]{0,63}$/;
const packageText = /^[0-9A-Za-z][0-9A-Za-z.+_-]{0,99}$/;
const instant = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/;

export function packageVersionFacts(service: unknown): PackageVersionFacts | null {
    if (!service || typeof service !== 'object') return null;
    const value = service as Record<string, unknown>;
    if (value.is_installed !== true || value.kind === 'runtime') return null;
    const raw = value.package_versions;
    if (!Array.isArray(raw) || raw.length === 0 || raw.length > 16) return null;
    const packages: InstalledPackageVersion[] = [];
    for (const item of raw) {
        if (!item || typeof item !== 'object') return null;
        const { package: name, version } = item as Record<string, unknown>;
        if (typeof name !== 'string' || !packageText.test(name) || typeof version !== 'string' || !versionText.test(version)) return null;
        packages.push({ package: name, version });
    }
    const readAt = typeof value.package_version_read_at === 'string' && instant.test(value.package_version_read_at)
        ? value.package_version_read_at : '';
    const kept = value.package_version_kept === true;
    const keptLong = kept && value.package_version_kept_long === true;
    if (value.package_versions_differ === true) {
        if (new Set(packages.map((pkg) => pkg.version)).size < 2) return null;
        return { full: '', short: '', several: true, packages, kept, readAt, keptLong };
    }
    const full = value.package_version;
    const short = value.package_version_short;
    if (typeof full !== 'string' || !versionText.test(full) || typeof short !== 'string' || !versionText.test(short)) return null;
    if (packages.some((pkg) => pkg.version !== full)) return null;
    return { full, short, several: false, packages, kept, readAt, keptLong };
}
