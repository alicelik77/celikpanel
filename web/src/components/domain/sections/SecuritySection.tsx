// Güvenlik (alpha.132): the certificate (the old Hosting › SSL/TLS), the
// protected folders and the malware scan, one below the other.
// Güvenlik: sertifika, korumalı klasörler ve zararlı yazılım taraması.
import { lazy } from 'react';
import { DomainSSLSettings } from '../../DomainSSLSettings';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

const DomainSiteProtect = lazy(() => import('../../DomainSiteFeatures').then((module) => ({ default: module.DomainSiteProtect })));
const DomainMalwareScan = lazy(() => import('../../DomainMalwareScan').then((module) => ({ default: module.DomainMalwareScan })));

export function SecuritySection({ domain, parts, part, isTeamMember, canView, open, mailAvailable, go, onCertificateChange }: DomainSectionProps) {
    const id = domain.id;
    const name = domain.domain_name;
    return (
        <SectionParts section="security" parts={parts} part={part} go={go} render={({ id: partId, readOnly }) => {
            switch (partId) {
                case 'certificate':
                    return (
                        <DomainSSLSettings
                            domainId={id}
                            domainName={name}
                            mailAvailable={isTeamMember ? canView('mail') : mailAvailable}
                            onCertificateChange={onCertificateChange}
                            readOnly={readOnly || (!open('ssl_acme') && !open('ssl_custom'))}
                            acmeOpen={open('ssl_acme')}
                            customOpen={open('ssl_custom')}
                        />
                    );
                case 'protect':
                    return <DomainSiteProtect domainId={id} domainName={name} readOnly={readOnly} />;
                case 'malware':
                    return <DomainMalwareScan domainId={id} />;
                default:
                    return null;
            }
        }} />
    );
}
