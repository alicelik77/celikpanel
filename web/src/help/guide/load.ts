import { useCallback, useEffect, useState } from 'react';
import type { Role } from '../../auth/AuthContext';
import type { GuideChunk, GuideLocale, GuideRole } from './types';

// One chunk per role and language, fetched when the guide is first opened.
// The role only chooses relevance: every chapter is public, and the server
// sends nothing for it. A team member (additional user) reads the customer
// guide; only the administrator may look at another role's guide.
// Her rol ve dil için bir parça, kılavuz ilk açıldığında getirilir.

const loaders: Record<`${GuideRole}.${GuideLocale}`, () => Promise<{ default: GuideChunk }>> = {
    'admin.en': () => import('./gen/admin.en'),
    'admin.tr': () => import('./gen/admin.tr'),
    'reseller.en': () => import('./gen/reseller.en'),
    'reseller.tr': () => import('./gen/reseller.tr'),
    'customer.en': () => import('./gen/customer.en'),
    'customer.tr': () => import('./gen/customer.tr'),
};

const cache = new Map<string, Promise<GuideChunk>>();

export function loadGuide(role: GuideRole, locale: GuideLocale): Promise<GuideChunk> {
    const key = `${role}.${locale}` as const;
    let chunk = cache.get(key);
    if (!chunk) {
        chunk = loaders[key]().then((module) => module.default);
        // A failed fetch is not remembered, so Retry fetches again.
        chunk.catch(() => cache.delete(key));
        cache.set(key, chunk);
    }
    return chunk;
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

export type GuideLoad = { state: 'loading' } | { state: 'known'; chunk: GuideChunk } | { state: 'unknown' };

export function useGuideChunk(role: GuideRole, locale: GuideLocale): { load: GuideLoad; retry: () => void } {
    const [load, setLoad] = useState<GuideLoad>({ state: 'loading' });
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        let current = true;
        setLoad({ state: 'loading' });
        loadGuide(role, locale).then(
            (chunk) => current && setLoad({ state: 'known', chunk }),
            () => current && setLoad({ state: 'unknown' }),
        );
        return () => {
            current = false;
        };
    }, [role, locale, attempt]);
    const retry = useCallback(() => setAttempt((n) => n + 1), []);
    return { load, retry };
}
