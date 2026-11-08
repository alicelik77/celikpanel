import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enMailConfig, type MailConfigKey } from './mailConfig/en';
import { trMailConfig } from './mailConfig/tr';

// Route-scoped copy (alpha.130, like ./mailOutgoing.ts): mail programs that
// set themselves up and the mail relay. Bundled with the screens that use
// it, never with the boot.
// Sayfaya bağlı metin: önyükleme paketine girmez.
export function mailConfigTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enMailConfig)) return null;
    let value: string = (locale === 'tr' ? trMailConfig : enMailConfig)[key as MailConfigKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

/** useI18n whose t also knows the mail configuration copy. */
export function useMailConfigWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => mailConfigTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}
