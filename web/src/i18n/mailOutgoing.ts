import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enMailOutgoing, type MailOutgoingKey } from './mailOutgoing/en';
import { trMailOutgoing } from './mailOutgoing/tr';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): outgoing mail
// limits. It is bundled with the screens that use it, never with the boot.
// Sayfaya bağlı metin: önyükleme paketine girmez.
export function mailOutgoingTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enMailOutgoing)) return null;
    let value: string = (locale === 'tr' ? trMailOutgoing : enMailOutgoing)[key as MailOutgoingKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

/** useI18n whose t also knows the outgoing mail copy. */
export function useMailOutgoingWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => mailOutgoingTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}
