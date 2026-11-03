// The user guide's data, as web/scripts/build-guide.mjs writes it into
// gen/<role>.<locale>.ts (one lazy chunk per role and language). The content
// is public and carries no server data; the role only chooses relevance.
// Kılavuzun verisi; build-guide.mjs her rol ve dil için bir parça yazar.

export type GuideRole = 'admin' | 'reseller' | 'customer';
export type GuideLocale = 'en' | 'tr';

/** Inline text: a plain string, or a run of strings and marks. */
export type GuideInline = string | Array<string | GuideMark>;
export type GuideMark =
    | ['b', GuideInline]
    | ['c', string]
    /** A link to a chapter (and a section) of the guide. */
    | ['g', string, string, GuideInline]
    /** A link to an outside https page. */
    | ['x', string, GuideInline];

export type GuideBlock =
    | ['h', 2 | 3, string, GuideInline]
    | ['p', GuideInline]
    | ['ul', GuideInline[]]
    | ['ol', GuideInline[]]
    | ['t', GuideInline[], GuideInline[][]]
    | ['pre', string]
    | ['note', GuideBlock[]]
    | ['warn', GuideBlock[]];

export interface GuideChapter {
    id: string;
    title: string;
    blocks: GuideBlock[];
}

/** [chapter index, section anchor ('' for the chapter), heading, folded words]. */
export type GuideSearchEntry = [number, string, string, string];

export interface GuideChunk {
    v: 1;
    role: GuideRole;
    locale: GuideLocale;
    chapters: GuideChapter[];
    /** topic -> [chapter id, anchor or ''] */
    topics: Record<string, [string, string]>;
    search: GuideSearchEntry[];
}
