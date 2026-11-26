import { useEffect, useState } from 'react';
import { useI18n } from './index';
import type { DbLimitsWordKey } from './dbLimitsWords/en';

// The database limits' words (alpha.149, review of PR #370 finding 18) load
// WITH the screens that use them - Problems now when it has such a line, and
// a plan's editor when it saves - as their own chunk per language, never in
// the screen part every screen fetches. Until they arrive a screen does not
// draw what needs them. Changing language reads the same I18nProvider locale.
//
// Veritabanı sınırlarının metinleri yalnız onları gösteren ekranlarla gelir.

type Words = Record<DbLimitsWordKey, string>;
type Values = Record<string, string | number>;

const loaded: Partial<Record<'en' | 'tr', Words>> = {};
const loading: Partial<Record<'en' | 'tr', Promise<Words>>> = {};

function localeOf(locale: string | undefined): 'en' | 'tr' {
    return locale === 'tr' ? 'tr' : 'en';
}

/** Fetches the words of a language once. */
export function loadDbLimitsWords(locale: string | undefined): Promise<Words> {
    const language = localeOf(locale);
    const ready = loaded[language];
    if (ready) return Promise.resolve(ready);
    const pending = loading[language];
    if (pending) return pending;
    const started = (language === 'tr'
        ? import('./dbLimitsWords/tr').then((module) => module.trDbLimitsWords)
        : import('./dbLimitsWords/en').then((module) => module.enDbLimitsWords)
    ).then((words) => {
        loaded[language] = words;
        return words;
    }, (error: unknown) => {
        delete loading[language];
        throw error;
    });
    loading[language] = started;
    return started;
}

/** Puts the values into a text. */
export function dbLimitsText(words: Words, key: DbLimitsWordKey, values?: Values): string {
    let value: string = words[key] ?? key;
    for (const [name, replacement] of Object.entries(values ?? {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type DbLimitsSay = (key: DbLimitsWordKey, values?: Values) => string;

/**
 * The words, fetched while `wanted` is true: `ready` tells whether they are
 * here; `say` answers a key (the key itself until they are).
 */
export function useDbLimitsWords(wanted: boolean): { ready: boolean; say: DbLimitsSay } {
    const { locale } = useI18n();
    const language = localeOf(locale);
    const [words, setWords] = useState<Words | undefined>(() => loaded[language]);
    useEffect(() => {
        if (!wanted) return;
        if (loaded[language]) {
            setWords(loaded[language]);
            return;
        }
        let live = true;
        loadDbLimitsWords(language).then((fetched) => { if (live) setWords(fetched); }, () => { /* the screen keeps the line out */ });
        return () => { live = false; };
    }, [wanted, language]);
    const current = words && words === loaded[language] ? words : loaded[language];
    return {
        ready: !!current,
        say: (key, values) => (current ? dbLimitsText(current, key, values) : key),
    };
}
