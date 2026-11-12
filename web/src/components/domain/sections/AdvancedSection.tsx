// Gelişmiş (alpha.132): the less visited tools, one below the other with an
// index at the top — scheduled tasks, logs, Git deploy, copying the site, the
// web server settings and, for the administrator, the configuration file.
// Gelişmiş: seyrek kullanılan araçlar, üstte dizinle alt alta.
import { lazy } from 'react';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each is fetched when the section opens, as when they were sub-tabs.
// Her biri bölüm açılınca getirilir.
const DomainCronManager = lazy(() => import('../../DomainCronManager').then((module) => ({ default: module.DomainCronManager })));
const DomainLogsViewer = lazy(() => import('../../DomainLogsViewer').then((module) => ({ default: module.DomainLogsViewer })));
const LogRetentionCard = lazy(() => import('../../LogRetentionCard').then((module) => ({ default: module.LogRetentionCard })));
const GitDeployPanel = lazy(() => import('../../GitDeployPanel').then((module) => ({ default: module.GitDeployPanel })));
const SiteCopyPanel = lazy(() => import('../../SiteCopyPanel').then((module) => ({ default: module.SiteCopyPanel })));
const DomainWebSettings = lazy(() => import('../../DomainWebSettings').then((module) => ({ default: module.DomainWebSettings })));
const DomainSiteConfig = lazy(() => import('../../DomainSiteConfig').then((module) => ({ default: module.DomainSiteConfig })));

export function AdvancedSection({ domain, parts, part, isTeamMember, go, onReasonEnded }: DomainSectionProps) {
    const id = domain.id;
    const name = domain.domain_name;
    return (
        <SectionParts section="advanced" parts={parts} part={part} go={go} render={({ id: partId, readOnly }) => {
            switch (partId) {
                case 'cron':
                    return <DomainCronManager domainId={id} domainName={name} readOnly={readOnly} />;
                case 'logs':
                    return (
                        <div className="space-y-4">
                            {!isTeamMember && <LogRetentionCard domainId={id} />}
                            <DomainLogsViewer domainId={id} domainName={name} readOnly={readOnly} />
                        </div>
                    );
                case 'git':
                    return <GitDeployPanel domainId={id} domainName={name} />;
                case 'copy':
                    return <SiteCopyPanel domainId={id} domainName={name} />;
                case 'web':
                    return <DomainWebSettings domainId={id} domainName={name} readOnly={readOnly} />;
                case 'config':
                    return <DomainSiteConfig domainId={id} domainName={name} onReasonEnded={onReasonEnded} />;
                default:
                    return null;
            }
        }} />
    );
}
