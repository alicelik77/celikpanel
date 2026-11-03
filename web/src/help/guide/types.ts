// The user guide's data, as web/scripts/build-guide.mjs writes it: one small
// index per role and language (gen/<role>.<locale>.ts: the chapter list,
// topics and search index) and one lazy chunk per chapter and language
// (gen/chapters/<id>.<locale>.ts), shared by every role that reads it, so no
// chapter is carried twice. The content is public and carries no server
// data; the role only chooses relevance.
// Kılavuzun verisi: rol ve dil başına küçük bir dizin, bölüm ve dil başına
// bir parça; bölümler roller arasında paylaşılır.

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

/** A chapter as the index lists it; its blocks arrive with load(). */
export interface GuideChapterEntry {
    id: string;
    title: string;
    load: () => Promise<{ default: GuideChapter }>;
}

export interface GuideIndex {
    v: 2;
    role: GuideRole;
    locale: GuideLocale;
    chapters: GuideChapterEntry[];
    /** topic -> [chapter id, anchor or ''] */
    topics: Record<string, [string, string]>;
    search: GuideSearchEntry[];
}
