import { useI18n } from './index';
import type { TranslationKey } from './en';
import { enMailboxRecovery, type MailboxRecoveryKey } from './mailboxRecovery/en';
import { trMailboxRecovery } from './mailboxRecovery/tr';

// Route-scoped copy (alpha.130, like ./panelSessions.ts): mailbox recovery.
// It is bundled with the screens that use it, never with the boot.
// Sayfaya bağlı metin: önyükleme paketine girmez.
export function mailboxRecoveryTranslation(locale: string | undefined, key: string, values?: Record<string, string | number>): string | null {
    if ((locale !== 'en' && locale !== 'tr') || !(key in enMailboxRecovery)) return null;
    let value: string = (locale === 'tr' ? trMailboxRecovery : enMailboxRecovery)[key as MailboxRecoveryKey];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export type MailboxRecoveryWordKey = TranslationKey | MailboxRecoveryKey;

/** useI18n whose t also knows the mailbox recovery copy. */
export function useMailboxRecoveryWords() {
    const i18n = useI18n();
    return {
        ...i18n,
        t: (key: MailboxRecoveryWordKey, values?: Record<string, string | number>) =>
            mailboxRecoveryTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values),
    };
}

/** A stored time in the reader's language (TR: Turkish format). */
export function mailboxRecoveryTime(value: string | undefined, locale: string | undefined): string {
    if (!value) return '';
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return value;
    return parsed.toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' });
}

export type { MailboxRecoveryKey };
