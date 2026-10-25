// The two reads a component's install dialogue makes (moved here in
// alpha.105 so the bundle dialogue reads the same addresses with the same
// decoders): the version the package manager would install now, and the
// component's managed repository with the versions it offers.
// Bir bileşenin kurulum diyaloğunun iki okuması; paket penceresi de aynı
// adresleri aynı çözücülerle okur.
export interface RepoInfo {
    available: boolean;
    enabled: boolean;
    repairable?: boolean;
    id?: string;
    name?: string;
    detail?: string;
    required?: boolean;
    packages?: string[];
    error_code?: string;
    // The repository card (alpha.107, D-038 §12); absent from an older Panel
    // or Agent.
    confinement?: {
        state: 'confined' | 'edited' | 'owner_rule' | 'not_written' | 'unknown' | string;
        pin_path?: string;
        owner_files?: string[];
        kept?: string[];
        kept_total?: number;
        written_at?: string;
        written_by?: string;
        reason?: string;
        detail?: string;
    };
    moving_packages?: { package: string; exact?: string }[];
    key?: { fingerprint: string; created_at?: string; expires_at?: string; state: 'valid' | 'expiring' | 'expired' | 'unknown' | string };
    lists?: { state: 'fresh' | 'missing' | 'expired' | 'unknown' | string; date?: string; valid_until?: string; origin?: string; suite?: string };
    enabled_at?: string;
    enabled_by?: string;
    apt_errors?: string[];
    apt_errors_at?: string;
    // alpha.108 (D-038 amendment "two lanes"): a vendor lane is enabled only
    // with its confinement and automatic updates; with the repository
    // disabled, what is still installed from it is no longer updated.
    lane?: boolean;
    origin?: string;
    no_longer_updated?: string[];
    disabled_at?: string;
}

// The two reads of the install dialogue. An answer that does not say whether a
// repository exists for the component is not "no repository": that reading is
// what hid a required repository and left Install enabled.
// Kurulum diyaloğunun iki okuması. Bileşen için depo olup olmadığını söylemeyen
// yanıt "depo yok" değildir: zorunlu depoyu gizleyip Kur'u açık bırakan buydu.
export function decodeRepoInfo(raw: unknown): RepoInfo {
    const body = raw as Partial<RepoInfo> | null;
    if (!body || typeof body !== 'object' || typeof body.available !== 'boolean' || typeof body.enabled !== 'boolean') {
        throw new Error('shape');
    }
    return body as RepoInfo;
}

export function decodeCandidateVersion(raw: unknown): string {
    const version = (raw as { version?: unknown } | null)?.version;
    if (typeof version !== 'string') throw new Error('shape');
    return version;
}
