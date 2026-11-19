// Programs (alpha.139): what runs on the domain — a program or a Python app
// the customer uploads, and the AI and automation apps — in its own place in
// the row, right after Site (owner, 19 Nov 2026). The two cards are the ones
// Site held, unchanged.
// Programlar: alan adında çalışan program, Python uygulaması ve yapay zekâ ve
// otomasyon uygulamaları; Site'taki kartların aynısı.
import { lazy } from 'react';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each is fetched when the section opens.
// Her biri bölüm açılınca getirilir.
const DomainRuntimeApps = lazy(() => import('../../DomainRuntimeApps').then((module) => ({ default: module.DomainRuntimeApps })));
const DomainServiceApps = lazy(() => import('../../DomainServiceApps').then((module) => ({ default: module.DomainServiceApps })));

export function ProgramsSection({ domain, parts, part, go }: DomainSectionProps) {
    return (
        <SectionParts section="programs" parts={parts} part={part} go={go} render={({ id: partId }) => {
            switch (partId) {
                case 'programs':
                    return <DomainRuntimeApps domainId={domain.id} domainName={domain.domain_name} />;
                case 'ai':
                    return <DomainServiceApps domainId={domain.id} domainName={domain.domain_name} />;
                default:
                    return null;
            }
        }} />
    );
}
