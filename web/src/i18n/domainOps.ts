import { enDomainOps, type DomainOpsKey } from './domainOps/en';
import { trDomainOps } from './domainOps/tr';
import { useI18n } from './index';

// Route-scoped copy (alpha.128, like ./monitoringAlerts.ts): the domain
// health strip and the action log. Changing language still reads the same
// I18nProvider locale; the shell catalogue does not grow.
// Sayfaya bağlı metin: dil değişince aynı I18nProvider yerel ayarı okunur.
export function domainOpsTranslation(locale: string | undefined, key: DomainOpsKey, values?: Record<string, string | number>): string {
    let value: string = (locale === 'tr' ? trDomainOps : enDomainOps)[key] ?? enDomainOps[key] ?? key;
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

export function isDomainOpsKey(key: string): key is DomainOpsKey {
    return key in enDomainOps;
}

/** The I18nProvider's locale with this copy (no boot cost). */
export function useDomainOpsWords() {
    const i18n = useI18n();
    return { ...i18n, say: (key: DomainOpsKey, values?: Record<string, string | number>) => domainOpsTranslation(i18n.locale, key, values) };
}
