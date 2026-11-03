import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { ServiceAppCatalogEntry } from '../lib/serviceApps';

// The risk summary of one AI or automation app, the same for the
// administrator who offers it and the customer who installs it: what it can
// do, what CelikPanel turns off, what it connects to, what remains a risk,
// its licence and its resource needs (docs/AI-APPS.md).
//
// Bir uygulamanın risk özeti; sunan yönetici ve kuran müşteri aynısını görür.

const sections: { field: keyof ServiceAppCatalogEntry['risk']; title: TranslationKey; tone: string }[] = [
    { field: 'can_do', title: 'svcapps.risk.canDo', tone: 'text-fg' },
    { field: 'disabled', title: 'svcapps.risk.disabled', tone: 'text-success' },
    { field: 'network', title: 'svcapps.risk.network', tone: 'text-fg' },
    { field: 'remains', title: 'svcapps.risk.remains', tone: 'text-warning' },
];

export function ServiceAppRisk({ app }: { app: ServiceAppCatalogEntry }) {
    const { t } = useI18n();
    return (
        <div className="space-y-3 text-sm">
            <div className="grid gap-3 sm:grid-cols-2">
                {sections.map((section) => {
                    const codes = app.risk?.[section.field] ?? [];
                    if (codes.length === 0) return null;
                    return (
                        <div key={section.field}>
                            <div className={`mb-1 text-xs font-semibold uppercase tracking-wide ${section.tone}`}>{t(section.title)}</div>
                            <ul className="list-disc space-y-0.5 pl-5 text-fg-muted">
                                {codes.map((code) => (
                                    <li key={code}>{t(('svcapps.risk.' + code) as TranslationKey)}</li>
                                ))}
                            </ul>
                        </div>
                    );
                })}
            </div>
            <p className="text-xs text-fg-subtle">
                {t('svcapps.version', { version: app.version })} · {t('svcapps.licence', { licence: app.licence })} ·{' '}
                <a className="underline" href={app.source} target="_blank" rel="noopener noreferrer">{t('svcapps.source')}</a>
            </p>
            {app.licence_note && <p className="text-xs text-warning">{t(('svcapps.licence.' + app.licence_note) as TranslationKey)}</p>}
            {/* alpha.118: closed to resellers and their customers by its licence. */}
            {app.reseller_closed && <p className="text-xs text-warning" data-reseller-closed="">{t('svcapps.licence.resellerClosed')}</p>}
            {app.defaults && (
                <p className="text-xs text-fg-subtle">
                    {t('svcapps.resources', { memory: app.defaults.memory_mb, cpu: app.defaults.cpu_percent, disk: app.defaults.disk_mb })}
                </p>
            )}
        </div>
    );
}
