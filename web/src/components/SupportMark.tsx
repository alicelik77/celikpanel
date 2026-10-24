// A version's security support, marked where the version appears (alpha.107,
// D-038 §5, §11, §14): a small badge beside a version, or one sentence under a
// picker. The status is the server's answer from the dated support table; a
// version the table does not know says "not known". Marked, never blocked.
// Bir sürümün güvenlik desteği, sürümün göründüğü yerde işaretlenir.
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { supportFor, type SupportComponent, type SupportResult, type SupportTable } from '../lib/supportTable';

const BADGE_CLASS: Record<string, string> = {
    supported: 'bg-success/10 text-success',
    ends_soon: 'bg-warning/15 text-warning',
    unsupported: 'bg-danger/10 text-danger',
    unknown: 'bg-surface-2 text-fg-subtle',
};

export const SUPPORT_NAMES: Record<SupportComponent, string> = { php: 'PHP', postgresql: 'PostgreSQL', mariadb: 'MariaDB', node: 'Node.js' };

function shortDate(value: string | undefined, locale: string): string {
    if (!value) return '';
    // A month as the source gives it ("2027-05") is said as a month.
    if (/^\d{4}-\d{2}$/.test(value)) {
        const month = new Date(value + '-01T00:00:00Z');
        return Number.isNaN(month.getTime()) ? value : month.toLocaleDateString(locale, { year: 'numeric', month: 'short', timeZone: 'UTC' });
    }
    const date = new Date(value + 'T00:00:00Z');
    return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric', timeZone: 'UTC' });
}

export function SupportBadge({ table, component, version }: { table: SupportTable | null; component: SupportComponent; version: string }) {
    const { t, locale } = useI18n();
    if (!table) return null;
    const result = supportFor(table, component, version);
    return (
        <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-semibold ${BADGE_CLASS[result.status] ?? BADGE_CLASS.unknown}`} data-support={result.status} title={supportSentence(t, locale, component, version, result, table)}>
            {t(`life.support.badge.${result.status}` as TranslationKey, { until: shortDate(result.until, locale) })}
        </span>
    );
}

export function supportSentence(
    t: (key: TranslationKey, vars?: Record<string, string | number>) => string,
    locale: string,
    component: SupportComponent,
    version: string,
    result: SupportResult,
    table: SupportTable | null,
): string {
    const from = result.provider === 'distribution'
        ? (table?.distro_name || t('life.support.fromDistribution'))
        : t('life.support.fromUpstream');
    if (result.status === 'unknown') return t('life.support.line.unknown', { name: SUPPORT_NAMES[component], version });
    return t(`life.support.line.${result.status}` as TranslationKey, { name: SUPPORT_NAMES[component], version, until: shortDate(result.until, locale), from });
}

export function SupportLine({ table, component, version }: { table: SupportTable | null; component: SupportComponent; version: string }) {
    const { t, locale } = useI18n();
    if (!table || !version) return null;
    const result = supportFor(table, component, version);
    const tone = result.status === 'unsupported' ? 'text-danger' : result.status === 'ends_soon' ? 'text-warning' : 'text-fg-muted';
    return (
        <p className={`mt-1.5 text-sm leading-6 ${tone}`} data-support-line={result.status}>
            {supportSentence(t, locale, component, version, result, table)}
        </p>
    );
}

/** The confirmation an unsupported version asks for, or '' when it asks none. */
export function supportConfirmation(
    t: (key: TranslationKey, vars?: Record<string, string | number>) => string,
    locale: string,
    table: SupportTable | null,
    component: SupportComponent,
    version: string,
    kind: 'install' | 'choose',
): string {
    const result = supportFor(table, component, version);
    if (result.status !== 'unsupported') return '';
    return t(kind === 'install' ? 'life.support.confirm.install' : 'life.support.confirm.choose', {
        name: SUPPORT_NAMES[component], version, until: shortDate(result.until, locale),
    });
}
