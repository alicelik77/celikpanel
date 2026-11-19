// Programs (alpha.138): what runs on the domain — a program or a Python app
// the customer uploads, and the AI and automation apps — in its own place in
// the row, right after Site (owner, 19 Nov 2026). The two cards are the ones
// Site held.
//
// Which cards are drawn is decided in sections.ts: from the plan, the site's
// type and what the domain holds (the domain list's holds_program and
// holds_app), so that nothing is offered whose press the server would refuse,
// and anything that keeps the domain from being deleted is drawn where it can
// be removed, whatever the plan or the type say. Each card offers only what
// the server accepts on this domain (newRefused mirrors the server's rules):
//   - offerNew: the plan includes it and the server takes a new one here;
//   - deployRefused / republishRefused: a record of the other kind is on the
//     domain (or the domain is the other kind's), so the server refuses to
//     publish this one again (a program's deploy; an app's update or retry);
//   - notServing: the domain's type is another kind's, so it is not opened at
//     the domain's address as this one.
//
// Programlar: alan adında çalışan program, Python uygulaması ve yapay zekâ ve
// otomasyon uygulamaları. Her kart yalnız sunucunun bu alan adında kabul
// ettiğini sunar; alan adında duran her şey kaldırılabilsin diye çizilir.
import { lazy } from 'react';
import { newRefused, type Holds } from '../sections';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each is fetched when the section opens.
// Her biri bölüm açılınca getirilir.
const DomainRuntimeApps = lazy(() => import('../../DomainRuntimeApps').then((module) => ({ default: module.DomainRuntimeApps })));
const DomainServiceApps = lazy(() => import('../../DomainServiceApps').then((module) => ({ default: module.DomainServiceApps })));

export function ProgramsSection({ domain, parts, part, go, open }: DomainSectionProps) {
    const type = domain.project_type || 'php';
    const holds: Holds = { program: domain.holds_program === true, app: domain.holds_app === true };
    return (
        <SectionParts section="programs" parts={parts} part={part} go={go} render={({ id: partId }) => {
            switch (partId) {
                case 'programs':
                    return (
                        <DomainRuntimeApps
                            domainId={domain.id}
                            domainName={domain.domain_name}
                            offerNew={(open('program_apps') || open('python_apps')) && !newRefused('programs', type, holds)}
                            deployRefused={holds.app || type === 'service_app'}
                            notServing={type !== 'runtime_app'}
                        />
                    );
                case 'ai':
                    return (
                        <DomainServiceApps
                            domainId={domain.id}
                            domainName={domain.domain_name}
                            offerNew={open('service_apps') && !newRefused('ai', type, holds)}
                            republishRefused={holds.program || type === 'runtime_app'}
                            notServing={type !== 'service_app'}
                        />
                    );
                default:
                    return null;
            }
        }} />
    );
}
