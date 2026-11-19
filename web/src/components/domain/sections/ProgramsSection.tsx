// Programs (alpha.139): what runs on the domain — a program or a Python app
// the customer uploads, and the AI and automation apps — in its own place in
// the row, right after Site (owner, 19 Nov 2026). The two cards are the ones
// Site held.
//
// Which cards are drawn is decided in sections.ts from the domain's type and
// plan, so that nothing is offered whose press the server would refuse. A
// domain can still hold something its type no longer shows: an AI app
// installed over a program before alpha.139 made the server refuse that (the
// type then says "AI app", while the program's record stays), or an install
// that failed on a program's site. Such a domain cannot be deleted until it
// is removed, so the section reads, where the card itself reads, whether the
// kind its type refuses is there; when it is, its card is drawn with only
// what can be managed and removed, never a create or install form. The list
// of domains does not carry this; one read per opening of this section, only
// on a program's or an app's site.
//
// Programlar: alan adında çalışan program, Python uygulaması ve yapay zekâ ve
// otomasyon uygulamaları. Türün artık göstermediği ama alan adında duran şey,
// kaldırılabilsin diye kartıyla çizilir; oluşturma formu gösterilmez.
import { lazy } from 'react';
import { decodeRuntimeApps } from '../../../lib/runtimeApps';
import { decodeDomainServiceApps } from '../../../lib/serviceApps';
import { lastKnown, useInvalidation, useRemote, type Remote } from '../../../lib/remote';
import { NEW_REFUSED_ON, type PartView } from '../sections';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

// Each is fetched when the section opens.
// Her biri bölüm açılınca getirilir.
const DomainRuntimeApps = lazy(() => import('../../DomainRuntimeApps').then((module) => ({ default: module.DomainRuntimeApps })));
const DomainServiceApps = lazy(() => import('../../DomainServiceApps').then((module) => ({ default: module.DomainServiceApps })));

// Whether a read says the domain holds one: what it last said; a read that
// could not be made at all counts as "maybe", and the card, which says so
// itself and offers the read again, is drawn rather than hidden.
function holds<T>(remote: Remote<T>, has: (value: T) => boolean): boolean {
    const known = lastKnown(remote);
    if (known) return has(known.value);
    return remote.state === 'unknown';
}

export function ProgramsSection({ domain, parts, part, go }: DomainSectionProps) {
    const type = domain.project_type || 'php';
    const refusesProgram = NEW_REFUSED_ON.programs.includes(type);
    const refusesApp = NEW_REFUSED_ON.ai.includes(type);
    const drawn = new Set(parts.map((item) => item.id));
    const probeProgram = refusesProgram && !drawn.has('programs');
    const probeApp = refusesApp && !drawn.has('ai');
    const program = useRemote(probeProgram ? `/api/v1/domains/${domain.id}/runtime-apps` : null, decodeRuntimeApps);
    const app = useRemote(probeApp ? `/api/v1/domains/${domain.id}/service-apps` : null, decodeDomainServiceApps);
    // A removal that finished reads the domain list again; so does this.
    useInvalidation(['domains'], () => {
        if (probeProgram) void program.retry();
        if (probeApp) void app.retry();
    });

    const shown: PartView[] = [...parts];
    if (probeProgram && holds(program.remote, (view) => view.app != null)) shown.push({ id: 'programs', readOnly: false });
    if (probeApp && holds(app.remote, (view) => view.install != null)) shown.push({ id: 'ai', readOnly: false });

    return (
        <SectionParts section="programs" parts={shown} part={part} go={go} render={({ id: partId }) => {
            switch (partId) {
                // A card drawn only because the domain still holds it is
                // "elsewhere": the domain serves the other kind, so nothing
                // that would publish this one again is offered.
                case 'programs':
                    return <DomainRuntimeApps domainId={domain.id} domainName={domain.domain_name} offerNew={!refusesProgram} elsewhere={probeProgram} />;
                case 'ai':
                    return <DomainServiceApps domainId={domain.id} domainName={domain.domain_name} offerNew={!refusesApp} elsewhere={probeApp} />;
                default:
                    return null;
            }
        }} />
    );
}
