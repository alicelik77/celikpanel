import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { dataToolsTranslation } from '../i18n/dataTools';

// Shared by the data tools' screens (alpha.128): their route-scoped words
// and a byte size in plain units.
// Veri araçları ekranlarının ortak sözcükleri ve boyut biçimi.

export function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = bytes / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** The I18nProvider's words with the data tools' own copy in front (route-scoped). */
export function useDataToolsWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => dataToolsTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}

