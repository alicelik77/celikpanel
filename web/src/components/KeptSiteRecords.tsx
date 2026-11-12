import { Link } from '../router';
import { useI18n } from '../i18n';
import type { SiteRecordsOutcome } from './AddDomainModal';
import { Guidance } from './kit/rows/Guidance';

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
        <Guidance
            tone="waiting"
            titleAs="h2"
            className="mb-4"
            data-kept-site-records={domain}
            title={t('dnsov.site.keptTitle')}
            why={t('dnsov.site.keptWhy', { names: outcome.kept.map((row) => `${row.name} ${row.type} ${row.content}`).join(', ') })}
            who={t('dnsov.site.keptWho')}
            next={t('dnsov.site.keptNext', { ip: outcome.ipv4 })}
            then={t('dnsov.site.keptThen')}
            action={<>
                <Link to={`/domains/${encodeURIComponent(domain)}?tab=dns`} className="inline-flex font-medium text-primary underline underline-offset-4">{t('dnsov.own.open')}</Link>
                {onDismiss && (
                    <button type="button" onClick={onDismiss} className="text-fg-muted underline underline-offset-4 hover:text-fg">
                        {t('domains.kept.dismiss')}
                    </button>
                )}
            </>}
        />
    );
}
