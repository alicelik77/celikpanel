import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enServerMaintenance, type ServerMaintenanceKey } from './serverMaintenance/en';
import { trServerMaintenance } from './serverMaintenance/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): Server →
// Maintenance's clock, processes, restart and certificates. Changing
// language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function serverMaintenanceTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enServerMaintenance)) return null;
    let value: string = (locale === 'tr' ? trServerMaintenance : enServerMaintenance)[key as ServerMaintenanceKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type MaintenanceWordKey = ServerMaintenanceKey | TranslationKey;

/** The I18nProvider's words with this page's own copy in front (no boot cost). */
export function useMaintenanceWords() {
    const i18n = useI18n();
    const t = (key: MaintenanceWordKey, values?: Record<string, string | number>) =>
        serverMaintenanceTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values);
    return { ...i18n, t };
}
