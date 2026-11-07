import { enNotifications, type NotificationsKey } from './notifications/en';
import { trNotifications } from './notifications/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): the
// notifications list, the email choice and the administrator's settings.
// Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function notificationsTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enNotifications)) return null;
    let value: string = (locale === 'tr' ? trNotifications : enNotifications)[key as NotificationsKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type { NotificationsKey };
