// The sources unattended-upgrades installs from, in plain words (alpha.130,
// owner finding of 8 Nov 2026: the Security page showed
// "origin=Debian,codename=${distro_codename}-security,label=Debian-Security").
// apt reports each source as a pattern, either Origins-Pattern
// ("origin=Debian,codename=${distro_codename}-security") or Allowed-Origins
// ("${distro_id}:${distro_codename}-security"). Each is read into who
// publishes it and what kind of updates it carries; a pattern this code does
// not understand is kept as "another source" (the exact text stays behind the
// card's details disclosure).
//
// unattended-upgrades kaynakları düz dille: kim yayımlıyor, ne tür güncelleme.

export type OriginKind = 'security' | 'release' | 'updates' | 'backports' | 'proposed' | 'esm' | 'vendor' | 'other';

export interface DescribedOrigin {
    /** Who publishes it ("Debian", "Ubuntu", a vendor), or null for "the distribution". */
    origin: string | null;
    kind: OriginKind;
}

const DISTRO_VARIABLE = /^\$\{distro_id\}/;

function kindOfSuite(suite: string, label: string): OriginKind {
    const lower = suite.toLowerCase();
    if (/-security$/.test(lower) || /security/i.test(label)) return 'security';
    if (/-proposed(-updates)?$/.test(lower)) return 'proposed';
    if (/-backports$/.test(lower)) return 'backports';
    if (/-updates$/.test(lower)) return 'updates';
    if (lower === '' && label === '') return 'other';
    return 'release';
}

export function describeOrigin(raw: string): DescribedOrigin {
    const text = raw.trim();
    if (text.includes('=')) {
        // Origins-Pattern: comma-separated key=value; o/a/n/l are short forms.
        const fields: Record<string, string> = {};
        for (const part of text.split(',')) {
            const at = part.indexOf('=');
            if (at < 0) continue;
            const key = part.slice(0, at).trim().toLowerCase();
            const value = part.slice(at + 1).trim();
            const long = { o: 'origin', a: 'archive', n: 'codename', l: 'label', c: 'component', site: 'site' }[key] ?? key;
            fields[long] = value;
        }
        const originText = fields.origin ?? '';
        const origin = originText === '' || DISTRO_VARIABLE.test(originText) ? null : originText;
        const suite = fields.codename ?? fields.archive ?? '';
        const label = fields.label ?? '';
        if (/ESM/i.test(originText) || /ESM/i.test(label)) return { origin: origin?.replace(/ESM.*$/i, '') || null, kind: 'esm' };
        if (origin && !/^(debian|ubuntu)$/i.test(origin)) return { origin, kind: 'vendor' };
        return { origin, kind: kindOfSuite(suite, label) };
    }
    // Allowed-Origins: "origin:suite".
    const at = text.lastIndexOf(':');
    if (at <= 0) return { origin: null, kind: 'other' };
    const originText = text.slice(0, at);
    const suite = text.slice(at + 1);
    if (/ESM/i.test(originText)) return { origin: DISTRO_VARIABLE.test(originText) ? null : originText.replace(/ESM.*$/i, ''), kind: 'esm' };
    const origin = DISTRO_VARIABLE.test(originText) ? null : originText;
    if (origin && !/^(debian|ubuntu)$/i.test(origin)) return { origin, kind: 'vendor' };
    return { origin, kind: kindOfSuite(suite, '') };
}

const ORDER: OriginKind[] = ['security', 'esm', 'release', 'updates', 'backports', 'proposed', 'vendor', 'other'];

/**
 * Each distinct source once, security first. `distro` names "the
 * distribution" when a pattern says ${distro_id} and another pattern names it.
 */
export function describeOrigins(covers: readonly string[]): DescribedOrigin[] {
    const described = covers.map(describeOrigin);
    const named = described.find((item) => item.origin && /^(debian|ubuntu)$/i.test(item.origin))?.origin ?? null;
    const seen = new Set<string>();
    const out: DescribedOrigin[] = [];
    for (const item of described) {
        const origin = item.origin ?? (item.kind === 'vendor' || item.kind === 'other' ? null : named);
        const key = `${item.kind}|${origin ?? ''}`;
        if (seen.has(key)) continue;
        seen.add(key);
        out.push({ origin, kind: item.kind });
    }
    return out.sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind));
}
