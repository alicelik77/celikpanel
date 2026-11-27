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

// alpha.150 (D-048): the words of a move that did not bring a site inside its limits. The panel's
// worker, or the administrator's Try again, marked it with a state and a reason code (the card's
// auto_move, Problems now's site_auto_move_failed); never the agent's English (its journal holds it).
// Here, beside the lane's words, so that Problems now loads nothing more for it.
const AUTO_TASK_WHYS: readonly string[] = ['cron_allow_owner', 'crontab_changed', 'limits', 'processes_unread', 'no_cron'];
const AUTO_WAITED: readonly string[] = ['site_busy', 'another_move', 'update_running', 'update_unreadable', 'build', 'host_busy', 'marks_unreadable', 'mark_failed'];

/** The catalogue key of why a site was not moved into its limits by itself. */
export function autoMoveReasonKey(a: { state?: string; reason?: string }): string {
    const reason = a.reason ?? '';
    switch (a.state) {
        case 'time_up': return 'slim.move.timeUp';
        case 'unknown': return 'slim.auto.why.unknown';
        case 'kept': return 'slim.auto.why.kept';
    }
    if (reason === 'stayed') return 'slim.auto.why.stayed';
    if (reason === 'tasks_put_back') return 'slim.tasks.why.put_back';
    if (reason === 'limits_unread') return 'slim.auto.why.limits_unread';
    if (AUTO_TASK_WHYS.includes(reason)) return `slim.tasks.why.${reason}`;
    // A reason that clears by itself and did not clear for hours (the worker then stopped waiting).
    if (AUTO_WAITED.includes(reason)) return 'slim.auto.why.waited';
    if (reason === 'kept_earlier' || reason === 'unsupported') return `slim.why.${reason}`;
    return 'slim.auto.why.failed';
}

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
    if (more > 0) lines.push(t('limits.preview.more', { n: more }));
    return lines;
}

// The unapplied-limits line of Problems now (alpha.149 fix round, F9): the
// server names each reason with its sites ("kernel_differs:2,unchecked:1"; an
// earlier record one code for all). Only the sites whose limits file was not
// written or whose kernel still holds the earlier values are said to have the
// earlier limits; for every other reason it is not known.
const APPLY_LACKING: readonly string[] = ['kernel_differs', 'write_failed', 'refused', 'out_of_range'];
const APPLY_REASONS: readonly string[] = [...APPLY_LACKING, 'unchecked', 'agent_unreachable', 'agent_refused'];

export function unappliedCounts(name: string | undefined, total: number): { lacking: number; unknown: number; lackingCodes: string[]; unknownCodes: string[] } {
    const raw = name ?? '';
    const parts: [string, number][] = raw.includes(':') ? raw.split(',').map((p) => [p.split(':')[0], Number(p.split(':')[1])]) : [[raw, total]];
    const out = { lacking: 0, unknown: 0, lackingCodes: [] as string[], unknownCodes: [] as string[] };
    for (const [code, n] of parts) {
        if (!(n > 0)) continue;
        if (APPLY_LACKING.includes(code)) {
            out.lacking += n;
            out.lackingCodes.push(code);
        } else {
            out.unknown += n;
            out.unknownCodes.push(code);
        }
    }
    return out;
}

/** The line's title or body key by what is known of its sites. */
export function unappliedKey(problem: { name?: string; value?: number }, part: 'title' | 'body'): string {
    const c = unappliedCounts(problem.name, problem.value ?? 0);
    if (c.lacking === 0) return part === 'title' ? 'pnow.site_limits_unapplied.title_unchecked' : 'pnow.site_limits_unapplied.body_unknown';
    if (c.unknown > 0) return `pnow.site_limits_unapplied.${part}_mixed`;
    return c.lacking === 1 ? `pnow.site_limits_unapplied.${part}_one` : `pnow.site_limits_unapplied.${part}`;
}

/** The line's values: each group's sites and reasons in the page's words. */
export function unappliedVars(problem: { name?: string; value: number }, t: (key: string, values?: Values) => string): Values {
    const c = unappliedCounts(problem.name, problem.value);
    const words = (codes: string[]) =>
        [...new Set(codes.map((code) => t(`slim.applyReason.${APPLY_REASONS.includes(code) ? code : 'other'}`)))].join('; ');
    const sites = (n: number) => (n === 1 ? t('slim.sites.one') : t('slim.sites.many', { n }));
    return { other: words(c.lackingCodes), why: words(c.unknownCodes), lacking: sites(c.lacking), unknown: sites(c.unknown), value: c.unknown === 0 ? c.lacking : problem.value };
}
