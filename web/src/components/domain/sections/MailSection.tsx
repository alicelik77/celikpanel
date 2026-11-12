// E-posta (alpha.132): the domain's mail, as the Mail tab was.
// E-posta: alan adının postası, eski Posta sekmesi gibi.
import { DomainMailManager } from '../../DomainMailManager';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

export function MailSection({ domain, parts, part, go }: DomainSectionProps) {
    return (
        <SectionParts section="mail" parts={parts} part={part} go={go} render={({ readOnly }) => (
            <DomainMailManager domainId={domain.id} domainName={domain.domain_name} readOnly={readOnly} />
        )} />
    );
}
