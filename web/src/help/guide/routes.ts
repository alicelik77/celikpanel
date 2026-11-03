import { matchRoute } from '../../router-core';
import type { GuideChunk } from './types';

// Which part of the guide a page opens. A page names its topic here by its
// route; a page that shows several things (a domain's tabs) names the one on
// screen with publishGuideTopic (./context.ts), and that wins. A chapter claims a
// topic in its front matter (docs/guide/FORMAT.md). A topic no chapter claims
// yet opens the first chapter, and the drawer says so.
// Bir sayfanın kılavuzda neyi açtığı; bölüm, konuyu ön bilgisinde sahiplenir.

export interface GuideRoute {
    pattern: string;
    topic: string;
}

// Longer, more specific patterns first: the first match wins.
export const guideRoutes: GuideRoute[] = [
    { pattern: '/', topic: 'dashboard' },
    { pattern: '/domains', topic: 'domains' },
    { pattern: '/domains/infrastructure/:zone', topic: 'dns.infrastructure' },
    { pattern: '/domains/:domainName', topic: 'domains' },
    { pattern: '/databases', topic: 'databases' },
    { pattern: '/users', topic: 'users' },
    { pattern: '/addons', topic: 'store' },
    { pattern: '/vpn', topic: 'vpn' },
    { pattern: '/services', topic: 'components' },
    { pattern: '/services/:serviceId', topic: 'components' },
    { pattern: '/server-setup', topic: 'server-setup' },
    { pattern: '/dns-infrastructure', topic: 'dns.servers' },
    { pattern: '/monitoring', topic: 'monitoring' },
    { pattern: '/security', topic: 'security' },
    { pattern: '/maintenance', topic: 'maintenance' },
    { pattern: '/import', topic: 'import' },
    { pattern: '/audit', topic: 'audit' },
    { pattern: '/settings', topic: 'settings' },
    { pattern: '/account', topic: 'account' },
    { pattern: '/help', topic: 'help' },
    { pattern: '/help/:chapter', topic: 'help' },
];

/** The route's topic, or null for an address the guide does not know. */
export function topicForPath(pathname: string): string | null {
    const path = pathname.replace(/\/+$/, '') || '/';
    return guideRoutes.find((route) => matchRoute(route.pattern, path) !== null)?.topic ?? null;
}

export interface GuidePlace {
    chapter: string;
    anchor: string;
    /** No chapter claims any of the topics: this is the guide's first chapter. */
    fallback: boolean;
}

/**
 * Where the guide opens for these topics, most specific first. A topic that
 * the chunk does not hold is skipped; with none, the first chapter.
 */
export function placeFor(chunk: GuideChunk, topics: Array<string | null | undefined>): GuidePlace | null {
    for (const topic of topics) {
        const found = topic ? chunk.topics[topic] : undefined;
        if (found) return { chapter: found[0], anchor: found[1], fallback: false };
    }
    const first = chunk.chapters[0];
    return first ? { chapter: first.id, anchor: '', fallback: true } : null;
}
