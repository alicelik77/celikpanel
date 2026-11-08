import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enSSHPassword, type SSHPasswordKey } from './sshPassword/en';
import { trSSHPassword } from './sshPassword/tr';

// Route-scoped copy (alpha.130, like ./serverMaintenance.ts): Security → SSH
// password sign-in. Changing language still reads the same I18nProvider locale.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function sshPasswordTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enSSHPassword)) return null;
    let value: string = (locale === 'tr' ? trSSHPassword : enSSHPassword)[key as SSHPasswordKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type SSHPasswordWordKey = SSHPasswordKey | TranslationKey;

/** The I18nProvider's words with this card's own copy in front (no boot cost). */
export function useSSHPasswordWords() {
    const i18n = useI18n();
    const t = (key: SSHPasswordWordKey, values?: Record<string, string | number>) =>
        sshPasswordTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values);
    return { ...i18n, t };
}
