// Imported with their extensions so that the web tests can load this module as it is.
import { enSiteLimitsLane, type SiteLimitsLaneKey } from './siteLimitsLane/en.ts';
import { trSiteLimitsLane } from './siteLimitsLane/tr.ts';
import type { TranslationKey } from './en';
import { unitText } from '../lib/settingLimits.ts';

// Route-scoped copy of the site limits lane (alpha.149, like ./panelSessions.ts):
// its keys are resolved here first, every other key by the screen catalogue.
// Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: önce buradaki anahtarlar, diğerleri ekran kataloğundan.
export function siteLimitsLaneTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enSiteLimitsLane)) return null;
    let value: string = (locale === 'tr' ? trSiteLimitsLane : enSiteLimitsLane)[key as SiteLimitsLaneKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

type Values = Record<string, string | number>;
/** A `t` that also knows this lane's keys. */
export type SiteLimitsLaneT = (key: TranslationKey | SiteLimitsLaneKey, values?: Values) => string;

/** The screen's `t` with this lane's keys first. */
export function withSiteLimitsLane(locale: string | undefined, t: (key: TranslationKey, values?: Values) => string): SiteLimitsLaneT {
    return (key, values) => siteLimitsLaneTranslation(locale, key, values) ?? t(key as TranslationKey, values);
}

export type { SiteLimitsLaneKey };

// The sites a lowered limit stops, in the page's own words (alpha.149, the
// alpha.148 lab's CHECK 2: the Turkish window carried the server's English
// "memory 215 MB in use, new limit 128 MB", and "The programs of 1 sites").
// The server sends the values (sites_stopped_items); its English lines stay
// what the confirmation's token names. Shared by the plan and account
// editors and Problems now.
//
// Düşürülen sınırın durduracağı siteler, sayfanın kendi dilinde.

export interface SiteStoppedItem {
    domain: string;
    memory_mb?: number;
    memory_limit_mb?: number;
    tasks?: number;
    tasks_limit?: number;
}

/** The list's heading, in the singular for one site. */
export function sitesStoppedTitle(n: number, t: SiteLimitsLaneT): string {
    return n === 1 ? t('limits.preview.sitesStopped.one') : t('limits.preview.sitesStopped', { n });
}

/** Reads the server's items (an answer of another shape gives none). */
function decodeSiteStoppedItems(raw: unknown): SiteStoppedItem[] | undefined {
    if (!Array.isArray(raw)) return undefined;
    return raw.filter((item): item is SiteStoppedItem => !!item && typeof item === 'object' && typeof (item as SiteStoppedItem).domain === 'string');
}

/** One line per site; the count of the sites not listed as "+n". */
export function siteStoppedLines(
    preview: { sites_stopped?: string[]; sites_stopped_count?: number; sites_stopped_items?: SiteStoppedItem[] },
    t: SiteLimitsLaneT,
): string[] {
    const items = decodeSiteStoppedItems(preview.sites_stopped_items);
    if (!items) return preview.sites_stopped ?? [];
    const lines = items.map((item) => {
        const parts: string[] = [];
        if (item.memory_limit_mb) parts.push(t('limits.preview.part.memory', { used: unitText(t, 'mb', item.memory_mb ?? 0), limit: unitText(t, 'mb', item.memory_limit_mb) }));
        if (item.tasks_limit) parts.push(t('limits.preview.part.tasks', { used: item.tasks ?? 0, limit: item.tasks_limit }));
        return `${item.domain}: ${parts.join('; ')}`;
    });
    const more = (preview.sites_stopped_count ?? items.length) - items.length;
    if (more > 0) lines.push(`+${more}`);
    return lines;
}
