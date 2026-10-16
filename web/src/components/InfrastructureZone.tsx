import { ArrowLeft, Globe, Settings } from 'lucide-react';
import { Link, useNavigate } from '../router';
import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { DomainDNSManager } from './DomainDNSManager';
import { Button, Checking, CouldNotCheck, KnownEmpty } from './ui';
import { lastKnown, useRemote } from '../lib/remote';
import { INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones } from '../lib/infrastructureZones';

// The infrastructure zone's page (2026-10-16, the owner's decision 2): the
// zone server setup created on the primary opens the same DNS record editor as
// a domain. The page first says what the zone is, which records setup keeps
// (and where they are changed) and that every other record is the owner's;
// then the editor. No hosting action is offered: the zone has no website until
// one is added for its name in Domains.
// Altyapi bolgesinin sayfasi: once bolgenin ne oldugu ve kayitlarin kimde
// oldugu, sonra ayni DNS duzenleyicisi. Barindirma eylemi sunulmaz.
export function InfrastructureZonePage({ zone }: { zone: string }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const zones = useRemote(INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones);
    const shown = lastKnown(zones.remote);
    const entry = shown?.value.find((item) => item.zone === zone);

    const header = (
        <PageHeader
            title={t('domains.infrastructure.title', { zone })}
            subtitle={t('domains.infrastructure.subtitle')}
            breadcrumb={[t('common.home'), t('nav.domains'), zone]}
            actions={
                <Button variant="secondary" icon={ArrowLeft} onClick={() => navigate('/domains')}>
                    {t('domains.infrastructure.back')}
                </Button>
            }
        />
    );

    if (zones.remote.state === 'loading') {
        return <div className="p-6 md:p-8">{header}<Checking label={t('domains.infrastructure.checking')} /></div>;
    }
    if (!shown) {
        return (
            <div className="p-6 md:p-8">
                {header}
                <CouldNotCheck text={t('domains.infrastructure.unknown')} onRetry={() => void zones.retry()} busy={zones.reading} />
            </div>
        );
    }
    if (!entry) {
        return (
            <div className="p-6 md:p-8">
                {header}
                <KnownEmpty of={shown} icon={Globe} title={t('domains.infrastructure.missing', { zone })} />
            </div>
        );
    }

    return (
        <div className="p-6 md:p-8">
            {header}
            {zones.remote.state === 'unknown' && (
                <CouldNotCheck className="mb-4" text={t('domains.infrastructure.unknown')} onRetry={() => void zones.retry()} busy={zones.reading} />
            )}
            <section aria-labelledby="infrastructure-zone-about" className="mb-6 max-w-3xl space-y-3 text-sm leading-6 text-fg">
                <h2 id="infrastructure-zone-about" className="sr-only">{t('domains.infrastructure.badge')}</h2>
                <p>{t('domains.infrastructure.what')}</p>
                <p>{t('domains.infrastructure.managed')}{' '}
                    <Link to="/settings?section=setup" className="inline-flex items-center gap-1 font-medium text-primary underline underline-offset-4">
                        <Settings className="h-3.5 w-3.5" aria-hidden="true" />
                        {t('domains.infrastructure.openSetup')}
                    </Link>
                </p>
                <p className="text-fg-muted">{t('domains.infrastructure.yours')}</p>
                {entry.peerRecords > 0 && <p className="text-fg-muted">{t('domains.infrastructure.peer')}</p>}
                {entry.domainId !== null && (
                    <p role="note" className="rounded-lg border border-border bg-surface-2 p-3">
                        {t('domains.infrastructure.site')}{' '}
                        <Link to={`/domains/${encodeURIComponent(zone)}`} className="font-medium text-primary underline underline-offset-4">
                            {t('domains.infrastructure.openSite')}
                        </Link>
                    </p>
                )}
            </section>
            <DomainDNSManager domainId={0} domainName={zone} infrastructureZone />
        </div>
    );
}
