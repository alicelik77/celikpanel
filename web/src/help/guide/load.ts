import { useCallback, useEffect, useState } from 'react';
import type { Role } from '../../auth/AuthContext';
import type { GuideChapter, GuideChapterEntry, GuideIndex, GuideLocale, GuideRole } from './types';

// The guide arrives in two steps: the index of the reader's role and
// language (chapter list, topics, search) when the guide is first opened,
// then each chapter when it is opened. A chapter's chunk is shared by every
// role that reads it. The role only chooses relevance: every chapter is
// public, and the server sends nothing for it. A team member (additional
// user) reads the customer guide; only the administrator may look at
// another role's guide.
// Kılavuz iki adımda gelir: önce rolün dizini, sonra açılan bölüm.

const indexes: Record<`${GuideRole}.${GuideLocale}`, () => Promise<{ default: GuideIndex }>> = {
    'admin.en': () => import('./gen/admin.en'),
    'admin.tr': () => import('./gen/admin.tr'),
    'reseller.en': () => import('./gen/reseller.en'),
    'reseller.tr': () => import('./gen/reseller.tr'),
    'customer.en': () => import('./gen/customer.en'),
    'customer.tr': () => import('./gen/customer.tr'),
};

// A failed fetch is not remembered, so Retry fetches again.
// Başarısız getirme hatırlanmaz; Yeniden dene yeniden getirir.
const cache = new Map<string, Promise<unknown>>();
function cached<T>(key: string, load: () => Promise<T>): Promise<T> {
    let found = cache.get(key) as Promise<T> | undefined;
    if (!found) {
        found = load();
        found.catch(() => cache.delete(key));
        cache.set(key, found);
    }
    return found;
}

export function loadGuideIndex(role: GuideRole, locale: GuideLocale): Promise<GuideIndex> {
    return cached(`index:${role}.${locale}`, () => indexes[`${role}.${locale}`]().then((module) => module.default));
}

export function loadGuideChapter(locale: GuideLocale, entry: GuideChapterEntry): Promise<GuideChapter> {
    return cached(`chapter:${locale}:${entry.id}`, () => entry.load().then((module) => module.default));
}

export function guideRoleFor(role: Role): GuideRole {
    return role === 'admin' ? 'admin' : role === 'reseller' ? 'reseller' : 'customer';
}

/** The guide an account reads: its own, or for the administrator the one chosen. */
export function readableGuide(role: Role, requested: string | null): GuideRole {
    const own = guideRoleFor(role);
    if (own !== 'admin') return own;
    return requested === 'reseller' || requested === 'customer' ? requested : 'admin';
}

export type Loaded<T> = { state: 'loading' } | { state: 'known'; value: T } | { state: 'unknown' };

function useLoaded<T>(load: (() => Promise<T>) | null, key: string): { loaded: Loaded<T>; retry: () => void } {
    const [loaded, setLoaded] = useState<Loaded<T>>({ state: 'loading' });
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        if (!load) return;
        let current = true;
        setLoaded({ state: 'loading' });
        load().then(
            (value) => current && setLoaded({ state: 'known', value }),
            () => current && setLoaded({ state: 'unknown' }),
        );
        return () => {
            current = false;
        };
        // The key names what load() fetches; the function itself is new each render.
    }, [key, attempt]);
    const retry = useCallback(() => setAttempt((n) => n + 1), []);
    return { loaded, retry };
}

export function useGuideIndex(role: GuideRole, locale: GuideLocale) {
    return useLoaded(() => loadGuideIndex(role, locale), `${role}.${locale}`);
}

/** The opened chapter's blocks; nothing is fetched while no chapter is open. */
export function useGuideChapter(locale: GuideLocale, entry: GuideChapterEntry | undefined) {
    return useLoaded(entry ? () => loadGuideChapter(locale, entry) : null, entry ? `${locale}:${entry.id}` : '');
}
