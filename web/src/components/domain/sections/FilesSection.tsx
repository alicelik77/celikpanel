// Dosyalar (alpha.132): the file manager and the ways to reach the files from
// outside (FTP, SFTP, SSH and the other logins), one below the other.
// Dosyalar: dosya yöneticisi ve dosyalara dışarıdan ulaşma yolları.
import { lazy } from 'react';
import { DomainFileManager } from '../../DomainFileManager';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

const DomainFileAccess = lazy(() => import('../../DomainFileAccess').then((module) => ({ default: module.DomainFileAccess })));
// The site's other FTP/SFTP logins, each closed into one folder (alpha.128).
const FTPLogins = lazy(() => import('../../FTPLogins').then((module) => ({ default: module.FTPLogins })));

export function FilesSection({ domain, parts, part, role, go }: DomainSectionProps) {
    return (
        <SectionParts section="files" parts={parts} part={part} go={go} render={({ id: partId, readOnly }) => {
            if (partId === 'manager') return <DomainFileManager domainId={domain.id} domainName={domain.domain_name} readOnly={readOnly} />;
            if (partId === 'access') {
                return (
                    <div className="space-y-6">
                        <DomainFileAccess domainId={domain.id} domainName={domain.domain_name} isAdmin={role === 'admin'} />
                        <FTPLogins domainId={domain.id} isAdmin={role === 'admin'} />
                    </div>
                );
            }
            return null;
        }} />
    );
}
