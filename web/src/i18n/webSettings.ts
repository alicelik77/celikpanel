import { useI18n } from '.';
import type { TranslationKey } from './en';
import { enWebSettings, type WebSettingsKey } from './webSettings/en';
import { trWebSettings } from './webSettings/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): the Web server
// screen of a domain. Its keys are not part of the shell or screen
// catalogue, so nothing of it reaches the boot bundle; changing language
// still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export type { WebSettingsKey };
export type WebWords = (key: TranslationKey | WebSettingsKey, values?: Record<string, string | number>) => string;

export function webSettingsTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enWebSettings)) return null;
    let value: string = (locale === 'tr' ? trWebSettings : enWebSettings)[key as WebSettingsKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

/** The I18nProvider's words with this screen's own copy in front (no boot cost). */
export function useWebWords() {
    const i18n = useI18n();
    const t: WebWords = (key, values) => webSettingsTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values);
    return { ...i18n, t };
}
