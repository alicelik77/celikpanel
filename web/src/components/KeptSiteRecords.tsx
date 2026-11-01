import { Link } from '../router';
import { useI18n } from '../i18n';
import type { SiteRecordsOutcome } from './AddDomainModal';

// The names a new site found already pointing elsewhere, as the create
// answered (alpha.115 site records): what happened, who acts, the next action
// and how it resumes (D-024). Shown wherever a site is added over an existing
// zone: the end of server setup, Domains and the infrastructure zone's page.
// Nothing is stored; it is this view's answer only.
// Yeni sitenin baska yere yonelmis buldugu adlar: ne oldu, kim yapar, sonraki
// adim ve nasil devam eder. Site eklenen her yerde gosterilir.
export function KeptSiteRecords({ domain, outcome, onDismiss }: { domain: string; outcome: SiteRecordsOutcome; onDismiss?: () => void }) {
    const { t } = useI18n();
    if (outcome.kept.length === 0) return null;
    return (
        <section role="status" className="mb-4 space-y-2 rounded-md border border-warning-mark/50 bg-warning-mark/10 p-4 text-sm leading-6" data-kept-site-records={domain}>
            <h2 className="font-semibold">{t('dnsov.site.keptTitle')}</h2>
            <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)]">
                <dt className="text-fg-muted">{t('setup.v2.why')}</dt>
                <dd className="min-w-0 break-words">{t('dnsov.site.keptWhy', { names: outcome.kept.map((row) => `${row.name} ${row.type} ${row.content}`).join(', ') })}</dd>
                <dt className="text-fg-muted">{t('setup.v2.who')}</dt>
                <dd>{t('dnsov.site.keptWho')}</dd>
                <dt className="text-fg-muted">{t('setup.v2.next')}</dt>
                <dd className="min-w-0 break-words">{t('dnsov.site.keptNext', { ip: outcome.ipv4 })}</dd>
                <dt className="text-fg-muted">{t('setup.v2.then')}</dt>
                <dd>{t('dnsov.site.keptThen')}</dd>
            </dl>
            <div className="flex flex-wrap items-center gap-4">
                <Link to={`/domains/${encodeURIComponent(domain)}?tab=dns`} className="inline-flex font-medium text-primary underline underline-offset-4">{t('dnsov.own.open')}</Link>
                {onDismiss && (
                    <button type="button" onClick={onDismiss} className="text-fg-muted underline underline-offset-4 hover:text-fg">
                        {t('domains.kept.dismiss')}
                    </button>
                )}
            </div>
        </section>
    );
}
