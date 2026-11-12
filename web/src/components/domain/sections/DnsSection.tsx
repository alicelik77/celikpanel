// Alan adı ve DNS (alpha.132): whether the domain points here, its names (the
// www redirect and the aliases, the old Hosting › General) and its DNS
// records, one below the other.
// Alan adı ve DNS: bağlantı, adlar (www ve takma adlar) ve DNS kayıtları.
import { DomainConnection } from '../../DomainConnection';
import { DomainGeneralSettings } from '../../DomainGeneralSettings';
import { DomainDNSManager } from '../../DomainDNSManager';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// The connection card is a card of its own.
const UNFRAMED = ['connection'] as const;

export function DnsSection({ domain, parts, part, isTeamMember, open, go }: DomainSectionProps) {
    const id = domain.id;
    const name = domain.domain_name;
    return (
        <SectionParts section="dns" parts={parts} part={part} go={go} unframed={UNFRAMED} render={({ id: partId, readOnly }) => {
            switch (partId) {
                case 'connection':
                    return <DomainConnection domainId={id} domainName={name} />;
                case 'names':
                    return <DomainGeneralSettings domainId={id} domainName={name} />;
                case 'records':
                    return <DomainDNSManager domainId={id} domainName={name} readOnly={readOnly || !open('dns_edit')} isAdditionalUser={isTeamMember} dnssecHidden={!open('dnssec')} />;
                default:
                    return null;
            }
        }} />
    );
}
