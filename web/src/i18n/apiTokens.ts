import { enApiTokens, type ApiTokensKey } from './apiTokens/en';
import { trApiTokens } from './apiTokens/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): the API tokens
// card and the remote API switch. Changing language still reads the same
// I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function apiTokensTranslation(locale: string | undefined, key: ApiTokensKey, values?: Record<string, string | number>): string {
    let value: string = (locale === 'tr' ? trApiTokens : enApiTokens)[key];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}
