import { enDataTools, type DataToolsKey } from './dataTools/en';
import { trDataTools } from './dataTools/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): the data tools.
// Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function dataToolsTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enDataTools)) return null;
    let value: string = (locale === 'tr' ? trDataTools : enDataTools)[key as DataToolsKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}
