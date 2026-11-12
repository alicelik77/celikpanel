// Veritabanları (alpha.132): the domain's databases, as the Databases tab was.
// Veritabanları: alan adının veritabanları, eski sekme gibi.
import { DomainDatabaseManager } from '../../DomainDatabaseManager';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

export function DatabasesSection({ domain, parts, part, isTeamMember, go }: DomainSectionProps) {
    return (
        <SectionParts section="databases" parts={parts} part={part} go={go} render={({ readOnly }) => (
            <DomainDatabaseManager domainId={domain.id} domainName={domain.domain_name} readOnly={readOnly} isAdditionalUser={isTeamMember} />
        )} />
    );
}
