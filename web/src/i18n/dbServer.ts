import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enDbServer, type DbServerKey } from './dbServer/en';
import { trDbServer } from './dbServer/tr';

// Route-scoped copy (alpha.133, like ./dbRemote.ts): Security › Open to the
// outside, and CelikPanel's own account on the MariaDB and PostgreSQL pages.
// Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.

type Values = Record<string, string | number>;
export function dbServerTranslation(locale: string | undefined, key: string, values?: Values): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enDbServer)) return null;
    let value: string = (locale === 'tr' ? trDbServer : enDbServer)[key as DbServerKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type DbServerWords = (key: DbServerKey | TranslationKey, values?: Values) => string;

/** The I18nProvider's words with this copy in front. */
export function useDbServerWords(): DbServerWords {
    const i18n = useI18n();
    return (key, values) => dbServerTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values);
}
