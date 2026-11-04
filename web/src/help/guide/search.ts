import type { GuideBlock, GuideChapter, GuideIndex, GuideInline } from './types';

// The guide's search, shared by the panel and by web/scripts/build-guide.mjs,
// which writes the index with these same functions (Node strips the types).
// Keep this file free of runtime imports so both can load it.
//
// Words are folded so that Turkish and English are typed the same way on any
// keyboard: "Şifre", "sifre" and "ŞİFRE" all match. A term matches the start of
// a word, so "dosya" finds "dosyaları".
// Kılavuz araması; panel ve build-guide.mjs aynı işlevleri kullanır. Sözcükler
// katlanır, terim sözcüğün başıyla eşleşir.

const FOLD: Record<string, string> = {
    ı: 'i', ş: 's', ğ: 'g', ü: 'u', ö: 'o', ç: 'c', â: 'a', î: 'i', û: 'u', é: 'e', è: 'e', ä: 'a',
};

export function fold(text: string): string {
    return text
        .replace(/İ/g, 'i')
        .replace(/I/g, 'i')
        .toLowerCase()
        .replace(/[ışğüöçâîûéèä]/g, (c) => FOLD[c] ?? c)
        .replace(/[^a-z0-9]+/g, ' ')
        .trim();
}

// Words that would match almost every section in either language.
const STOP = new Set([
    'the', 'and', 'for', 'you', 'your', 'with', 'that', 'this', 'are', 'not', 'from', 'can', 'has', 'have',
    'will', 'its', 'all', 'any', 'but', 'when', 'then', 'than', 'into', 'only', 'what', 'who', 'how', 'was',
    'bir', 'icin', 'gibi', 'olan', 'daha', 'her', 'cok', 'sonra', 'once', 'veya', 'yalniz', 'ancak', 'nasil',
]);

/** The distinct folded words of a text worth indexing, in first-seen order. */
export function wordsOf(text: string): string[] {
    const seen = new Set<string>();
    for (const word of fold(text).split(' ')) {
        if (word.length >= 3 && !STOP.has(word)) seen.add(word);
    }
    return [...seen];
}


// The index carries only what finds a chapter quickly: titles, headings and
// keywords. The words of the chapters' text come from the chapters
// themselves, fetched when someone searches (GuidePage), so no text is kept
// twice. chapterBodies gives a fetched chapter's words per section.
// Dizin yalnız başlık, ara başlık ve anahtar sözcük taşır; metnin sözcükleri
// arama yapılınca bölümlerin kendisinden gelir.

function inlineText(value: GuideInline): string {
    if (typeof value === 'string') return value;
    return value.map((part) => {
        if (typeof part === 'string') return part;
        if (part[0] === 'c') return part[1];
        if (part[0] === 'b') return inlineText(part[1]);
        if (part[0] === 'g') return inlineText(part[3]);
        return inlineText(part[2]);
    }).join(' ');
}

function blockText(block: GuideBlock): string {
    switch (block[0]) {
        case 'h': return inlineText(block[3]);
        case 'p': return inlineText(block[1]);
        case 'ul': case 'ol': return block[1].map(inlineText).join(' ');
        case 't': return [...block[1], ...block[2].flat()].map(inlineText).join(' ');
        case 'pre': return block[1];
        default: return block[1].map(blockText).join(' ');
    }
}

/** A chapter's words per section: anchor ('' for the opening text) -> words. */
export function chapterBodies(chapter: GuideChapter): Record<string, string> {
    const texts: Record<string, string> = { '': '' };
    let anchor = '';
    for (const block of chapter.blocks) {
        if (block[0] === 'h') anchor = block[2];
        texts[anchor] = `${texts[anchor] ?? ''} ${blockText(block)}`;
    }
    return Object.fromEntries(Object.entries(texts).map(([key, text]) => [key, wordsOf(text).join(' ')]));
}

export interface GuideHit {
    chapter: number;
    anchor: string;
    heading: string;
    score: number;
}

/**
 * Sections whose heading, keywords or (when given) text start with every term
 * of the query. bodies: chapter id -> chapterBodies of that chapter.
 */
export function searchGuide(index: GuideIndex, query: string, bodies: Record<string, Record<string, string>> = {}, limit = 20): GuideHit[] {
    const terms = [...new Set(fold(query).split(' ').filter((term) => term.length >= 2))];
    if (terms.length === 0) return [];
    const hits: GuideHit[] = [];
    index.search.forEach(([chapter, anchor, heading, words], order) => {
        const head = ` ${fold(heading)} ${words} `;
        const body = ` ${bodies[index.chapters[chapter]?.id ?? '']?.[anchor] ?? ''} `;
        let score = 0;
        for (const term of terms) {
            if (head.includes(` ${term}`)) score += 3;
            else if (body.includes(` ${term}`)) score += 1;
            else return;
        }
        // The chapter's own entry wins a tie with one of its sections, and
        // earlier entries come first among equals.
        hits.push({ chapter, anchor, heading, score: score + (anchor === '' ? 0.5 : 0) - order / 1e6 });
    });
    return hits.sort((a, b) => b.score - a.score).slice(0, limit);
}
