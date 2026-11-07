import { enDbRemote, type DbRemoteKey } from './dbRemote/en';
import { trDbRemote } from './dbRemote/tr';

// Route-scoped copy (alpha.129, like ./dataTools.ts): connections from
// outside to a database. Changing language still reads the same
// I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function dbRemoteTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enDbRemote)) return null;
    let value: string = (locale === 'tr' ? trDbRemote : enDbRemote)[key as DbRemoteKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}
