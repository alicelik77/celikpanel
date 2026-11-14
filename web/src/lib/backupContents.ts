import type { TranslationKey } from '../i18n/en';
import { sizeWord } from '../components/kit/words';

// What a backup of a site holds (docs/SITE-BACKUP-CONTENTS-DESIGN.md): the
// site's files and, since the release after alpha.133, the site-home areas
// (the program's folder "app", each service app's "apps/<app>"), with what an
// area leaves out and why. The same words are used before a backup is made
// (GET …/backups/contents, measured with the archive's own rules) and on each
// backup in the list (its manifest's areas).
//
// Bir yedeğin neleri içerdiği: site dosyaları ve sitenin program alanları;
// dışarıda kalanlar gerekçesiyle. Aynı sözler yedekten önce ve listede.

type Translate = (key: TranslationKey, values?: Record<string, string | number>) => string;

export interface BackupArea {
    name: string;
    bytes: number;
    files: number;
}

export interface LeftOutItem {
    path: string;
    reason: string;
    bytes?: number;
}

export interface BackupContents {
    siteBytes: number;
    areas: BackupArea[];
    leftOut: LeftOutItem[];
    leftOutMore: number;
    /** Free space on the server's backup disk; null when it could not be read. */
    freeBytes: number | null;
    /** False when the measurement stopped at its time limit: the sizes are "at least". */
    complete: boolean;
    /** The site's program record, when it has one. */
    program: { kind: string; deployed: boolean } | null;
}

/** A backup of this many bytes or more asks before it is made. */
export const LARGE_BACKUP_BYTES = 1024 * 1024 * 1024;

/** The program's area (a program or a Python app). */
export const PROGRAM_AREA = 'app';

const num = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) ? value : 0);

function decodeAreas(raw: unknown): BackupArea[] {
    if (!Array.isArray(raw)) return [];
    return raw.filter((item) => item && typeof item === 'object' && typeof item.name === 'string')
        .map((item) => ({ name: item.name as string, bytes: num(item.bytes), files: num(item.files) }));
}

function decodeLeftOut(raw: unknown): LeftOutItem[] {
    if (!Array.isArray(raw)) return [];
    return raw.filter((item) => item && typeof item === 'object' && typeof item.path === 'string' && typeof item.reason === 'string')
        .map((item) => ({ path: item.path as string, reason: item.reason as string, bytes: num(item.bytes) }));
}

/** The contents answer; anything else is not the contract (the read is "unknown"). */
export function decodeBackupContents(raw: unknown): BackupContents {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (typeof body.site_bytes !== 'number' || !Array.isArray(body.areas) || typeof body.complete !== 'boolean') throw new Error('shape');
    if (typeof body.error === 'string' && body.error) throw new Error('measurement');
    const program = body.program && typeof body.program === 'object'
        ? { kind: String((body.program as Record<string, unknown>).kind ?? ''), deployed: (body.program as Record<string, unknown>).deployed === true }
        : null;
    const free = typeof body.backup_free_bytes === 'number' && body.backup_free_bytes >= 0 ? body.backup_free_bytes : null;
    return {
        siteBytes: body.site_bytes, areas: decodeAreas(body.areas), leftOut: decodeLeftOut(body.left_out),
        leftOutMore: num(body.left_out_more), freeBytes: free, complete: body.complete, program,
    };
}

/** The decoded areas and left-out items of one backup in the list. */
export const backupAreas = (raw: unknown) => decodeAreas(raw);
export const backupLeftOut = (raw: unknown) => decodeLeftOut(raw);

/** The bytes a backup of these contents archives (site files and areas). */
export const contentsBytes = (contents: BackupContents) =>
    contents.siteBytes + contents.areas.reduce((sum, area) => sum + area.bytes, 0);

export const holdsProgram = (areas: BackupArea[]) => areas.some((area) => area.name === PROGRAM_AREA);

/** "Program dosyaları ve verisi (1,1 GB)", "n8n verisi (3 MB)". */
export function areaWord(t: Translate, area: BackupArea, locale: string): string {
    const size = sizeWord(area.bytes, locale);
    if (area.name === PROGRAM_AREA) return t('bkc.area.program', { size });
    return t('bkc.area.app', { app: serviceAppName(area.name), size });
}

/** A service app's name from its area ("apps/open-webui" -> "Open WebUI"). */
export function serviceAppName(area: string): string {
    const id = area.replace(/^apps\//, '');
    if (id === 'n8n') return 'n8n';
    if (id === 'open-webui') return 'Open WebUI';
    return id;
}

/**
 * The left-out items as the screen says them: grouped by why, with a count and
 * the bytes. Caches and build leftovers are one group.
 */
export function leftOutWords(t: Translate, items: LeftOutItem[], more: number, locale: string): string[] {
    const groups = new Map<string, { count: number; bytes: number }>();
    for (const item of items) {
        const group = item.reason === 'build_temp' ? 'cache' : item.reason;
        const now = groups.get(group) ?? { count: 0, bytes: 0 };
        groups.set(group, { count: now.count + 1, bytes: now.bytes + (item.bytes ?? 0) });
    }
    const words: string[] = [];
    for (const [group, { count, bytes }] of groups) {
        const size = sizeWord(bytes, locale);
        switch (group) {
            case 'cache': words.push(t('bkc.left.cache', { size })); break;
            case 'reinstalled': words.push(t('bkc.left.reinstalled', { size })); break;
            case 'special': words.push(t('bkc.left.special', { count })); break;
            case 'not_site_owned': words.push(t('bkc.left.foreign', { count, paths: pathsOf(items, 'not_site_owned') })); break;
            default: words.push(t('bkc.left.other', { count, paths: pathsOf(items, group) })); break;
        }
    }
    if (more > 0) words.push(t('bkc.left.more', { count: more }));
    return words;
}

const pathsOf = (items: LeftOutItem[], reason: string) =>
    items.filter((item) => item.reason === reason).slice(0, 3).map((item) => item.path).join(', ');
