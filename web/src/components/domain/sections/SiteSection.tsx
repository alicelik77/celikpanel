// Site (alpha.132): what runs on the domain and how — the old Hosting type,
// Applications, AI & automation, Programs, PHP and Redirects, one below the
// other. The insides are unchanged; the section's own redesign (D4, D5) comes
// in a later release.
// Site: alan adında ne çalıştığı; eski barındırma tipi, uygulamalar, yapay
// zekâ, programlar, PHP ve yönlendirmeler alt alta.
import { lazy } from 'react';
import { HostingTypePanel } from '../../HostingTypePanel';
import { DomainPHPSettings } from '../../DomainPHPSettings';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each of these is fetched when the section opens, as when it was a tab.
// Her biri bölüm açılınca getirilir.
const DomainAppsPanel = lazy(() => import('../../DomainAppsPanel').then((module) => ({ default: module.DomainAppsPanel })));
const WordPressToolsPanel = lazy(() => import('../../WordPressToolsPanel').then((module) => ({ default: module.WordPressToolsPanel })));
const DomainServiceApps = lazy(() => import('../../DomainServiceApps').then((module) => ({ default: module.DomainServiceApps })));
const DomainRuntimeApps = lazy(() => import('../../DomainRuntimeApps').then((module) => ({ default: module.DomainRuntimeApps })));
const DomainSiteRedirects = lazy(() => import('../../DomainSiteFeatures').then((module) => ({ default: module.DomainSiteRedirects })));

export function SiteSection({ domain, parts, part, role, isTeamMember, open, go, onPhpVersionChange }: DomainSectionProps) {
    const id = domain.id;
    const name = domain.domain_name;
    return (
        <SectionParts section="site" parts={parts} part={part} go={go} render={({ id: partId, readOnly }) => {
            switch (partId) {
                case 'type':
                    return <HostingTypePanel domainId={id} domainName={name} />;
                case 'apps':
                    return (
                        <>
                            {open('app_installer') && <DomainAppsPanel domainId={id} domainName={name} />}
                            {(role === 'admin' || open('wordpress_tools')) && <WordPressToolsPanel domainId={id} />}
                        </>
                    );
                case 'ai':
                    return <DomainServiceApps domainId={id} domainName={name} />;
                case 'programs':
                    return <DomainRuntimeApps domainId={id} domainName={name} />;
                case 'php':
                    return (
                        <DomainPHPSettings
                            domainId={id}
                            domainName={name}
                            currentVersion={domain.php_version ?? ''}
                            onVersionChange={onPhpVersionChange}
                            readOnly={readOnly}
                            isAdditionalUser={isTeamMember}
                            isAdmin={role === 'admin'}
                        />
                    );
                case 'redirects':
                    return <DomainSiteRedirects domainId={id} domainName={name} readOnly={readOnly} />;
                default:
                    return null;
            }
        }} />
    );
}
