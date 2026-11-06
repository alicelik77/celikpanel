import { enMonitoringAlerts, type MonitoringAlertsKey } from './monitoringAlerts/en';
import { trMonitoringAlerts } from './monitoringAlerts/tr';

// Route-scoped copy (alpha.127, like ./setupDNS.ts): the Monitoring page's
// alerts, services, website checks and machine detail. Changing language
// still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function monitoringAlertsTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enMonitoringAlerts)) return null;
    let value: string = (locale === 'tr' ? trMonitoringAlerts : enMonitoringAlerts)[key as MonitoringAlertsKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}
