import { useCallback, useState } from 'react';
import type { ApiError } from './apiError';
import type { TranslationKey } from '../i18n/en';

// A refused change, said once (owner rule: "the same warning is never
// repeated"; Boston 6 Nov 2026, a refused database create said the same
// sentence twice, in a notice and in the banner above the list). The banner
// (ErrorBanner with the screen's sentence) is where a refusal stands, with its
// fix and its code, until the next change succeeds; it brings itself into
// view, so no notice repeats it. A screen that shows its refusal in a banner
// keeps it here and sends no error notice for the same refusal.
//
// Reddedilen değişiklik bir kez söylenir: ret, düzeltmesi ve koduyla bir
// sonraki başarılı değişikliğe kadar bannerda durur ve kendini görünür kılar;
// aynı cümleyi bir bildirim tekrar etmez.
export type Refusal = { problem: ApiError; sentence: TranslationKey };

export function useRefusal() {
    const [refusal, setRefusal] = useState<Refusal | null>(null);
    const refuse = useCallback((problem: ApiError, sentence: TranslationKey) => setRefusal({ problem, sentence }), []);
    const clear = useCallback(() => setRefusal(null), []);
    return { refusal, refuse, clear, setRefusal };
}
