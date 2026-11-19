// Programs (alpha.138): what runs on the domain — a program or a Python app
// the customer uploads, and the AI and automation apps — in its own place in
// the row, right after Site (owner, 19 Nov 2026). The two cards are the ones
// Site held.
//
// Which cards are drawn is decided in sections.ts: from the plan and the
// site's type, so that nothing is offered whose press the server would
// refuse, and from what the domain holds (the domain list's holds_program
// and holds_app), so that anything that keeps the domain from being deleted
// is drawn where it can be removed, whatever the plan or the type say. Such
// a card offers what manages and removes it, never a create or install form:
//   - offerNew: the plan includes it and the type lets a new one be added;
//   - elsewhere: the domain serves the other kind (a domain that holds both,
//     from before alpha.138), so nothing that would publish this one again
//     is offered.
//
// Programlar: alan adında çalışan program, Python uygulaması ve yapay zekâ ve
// otomasyon uygulamaları. Alan adında duran her şey, kaldırılabilsin diye
// kartıyla çizilir; oluşturma formu yalnız plan ve tür izin verirse gösterilir.
import { lazy } from 'react';
import { NEW_REFUSED_ON } from '../sections';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each is fetched when the section opens.
// Her biri bölüm açılınca getirilir.
const DomainRuntimeApps = lazy(() => import('../../DomainRuntimeApps').then((module) => ({ default: module.DomainRuntimeApps })));
const DomainServiceApps = lazy(() => import('../../DomainServiceApps').then((module) => ({ default: module.DomainServiceApps })));

export function ProgramsSection({ domain, parts, part, go, open }: DomainSectionProps) {
    const type = domain.project_type || 'php';
    const refusesProgram = NEW_REFUSED_ON.programs.includes(type);
    const refusesApp = NEW_REFUSED_ON.ai.includes(type);
    return (
        <SectionParts section="programs" parts={parts} part={part} go={go} render={({ id: partId }) => {
            switch (partId) {
                case 'programs':
                    return (
                        <DomainRuntimeApps
                            domainId={domain.id}
                            domainName={domain.domain_name}
                            offerNew={(open('program_apps') || open('python_apps')) && !refusesProgram}
                            elsewhere={refusesProgram}
                        />
                    );
                case 'ai':
                    return (
                        <DomainServiceApps
                            domainId={domain.id}
                            domainName={domain.domain_name}
                            offerNew={open('service_apps') && !refusesApp}
                            elsewhere={refusesApp && type !== 'service_app'}
                        />
                    );
                default:
                    return null;
            }
        }} />
    );
}
