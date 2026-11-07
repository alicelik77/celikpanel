import { enPanelSessions, type PanelSessionsKey } from './panelSessions/en';
import { trPanelSessions } from './panelSessions/tr';

// Route-scoped copy (alpha.128, G4, like ./notifications.ts): panel sessions
// and panel access. Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function panelSessionsTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enPanelSessions)) return null;
    let value: string = (locale === 'tr' ? trPanelSessions : enPanelSessions)[key as PanelSessionsKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type { PanelSessionsKey };
