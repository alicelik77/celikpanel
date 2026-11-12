// Yedekler (alpha.132): the domain's backups, out of the old Advanced tab.
// Yedekler: alan adının yedekleri; eski Gelişmiş sekmesinden çıktı.
import { DomainBackupManager } from '../../DomainBackupManager';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

export function BackupsSection({ domain, parts, part, go }: DomainSectionProps) {
    return (
        <SectionParts section="backups" parts={parts} part={part} go={go} render={({ readOnly }) => (
            <DomainBackupManager domainId={domain.id} domainName={domain.domain_name} readOnly={readOnly} />
        )} />
    );
}
