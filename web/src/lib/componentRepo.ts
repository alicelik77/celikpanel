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
