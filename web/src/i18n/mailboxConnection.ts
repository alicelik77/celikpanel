import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enMailboxConnection, type MailboxConnectionKey } from './mailboxConnection/en';
import { trMailboxConnection } from './mailboxConnection/tr';

// Route-scoped copy (alpha.138, like ./mailboxRecovery.ts): one mailbox's
// connection settings window. It is bundled with that window, never with the
// boot.
// Sayfaya bağlı metin: önyükleme paketine girmez.
export function mailboxConnectionTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enMailboxConnection)) return null;
    let value: string = (locale === 'tr' ? trMailboxConnection : enMailboxConnection)[key as MailboxConnectionKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type MailboxConnectionWordKey = TranslationKey | MailboxConnectionKey;

/** useI18n whose t also knows the connection settings window's copy. */
export function useMailboxConnectionWords() {
    const i18n = useI18n();
    return {
        ...i18n,
        t: (key: MailboxConnectionWordKey, values?: Record<string, string | number>) =>
            mailboxConnectionTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values),
    };
}
