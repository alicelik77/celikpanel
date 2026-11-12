// The body every section of a domain's page shares (alpha.132): its parts one
// below the other on one page, each in its own card, and, when there is more
// than one, a short "On this page" index whose links scroll to them (the
// second level of DESIGN-DIRECTION P1 and D12: no sub-tab pills). The part an
// address names (?part=…) is scrolled to when the section opens.
// A team member who may only view a part is told so above it.
//
// Bir bölümün gövdesi: parçaları tek sayfada alt alta, her biri kendi
// kartında; birden çok parça varsa "Bu sayfada" dizini onlara kaydırır.
// Adresin adlandırdığı parça bölüm açılınca görünür kılınır.
import { Suspense, useEffect, useRef, type ReactNode } from 'react';
import { Eye } from 'lucide-react';
import { useI18n } from '../../i18n';
import type { DomainCapability } from '../../auth/domainAccess';
import type { SSLRuntimeSummary } from '../DomainSSLSettings';
import { Spinner } from '../ui';
import { PART_LABELS, type PartId, type PartView, type Place, type SectionId, type SectionView } from './sections';

export interface DomainFacts {
    id: number;
    domain_name: string;
    php_version?: string;
    project_type?: string;
}

/** What the shell gives every section's lazy file. */
export interface DomainSectionProps {
    domain: DomainFacts;
    /** This section's parts, in order, as the registry decided them. */
    parts: PartView[];
    /** The part the address names, if any. */
    part: PartId | null;
    /** Every section of the row (the Overview's tiles follow them). */
    sections: readonly SectionView[];
    role: string;
    isTeamMember: boolean;
    open: (right: string) => boolean;
    canView: (capability: DomainCapability) => boolean;
    /** Whether the server has mail, for the certificate's mail names; null while not known. */
    mailAvailable: boolean | null;
    go: (place: Place, options?: { replace?: boolean }) => void;
    onCertificateChange: (status: SSLRuntimeSummary) => void;
    onPhpVersionChange: (version: string) => void;
    /** The configuration file ended a reason the domain list carries. */
    onReasonEnded: () => void;
}

export const partElementId = (id: PartId) => `domain-part-${id}`;

function scrollToPart(id: PartId, smooth: boolean) {
    document.getElementById(partElementId(id))?.scrollIntoView({ block: 'start', behavior: smooth ? 'smooth' : 'auto' });
}

export function SectionParts({ section, parts, part, go, render, unframed = [] }: {
    section: SectionId;
    parts: PartView[];
    part: PartId | null;
    go: DomainSectionProps['go'];
    render: (part: PartView) => ReactNode;
    /** Parts that draw their own card (the connection card). */
    unframed?: readonly PartId[];
}) {
    const { t } = useI18n();
    const named = part && parts.some((item) => item.id === part) ? part : null;
    const body = useRef<HTMLDivElement | null>(null);
    // A part chosen from the index glides into view; one an address names
    // (a link, a reload, Back) is shown at once.
    const fromIndex = useRef(false);

    // The part the address names is brought into view when the section is
    // drawn. The parts above it load after it and grow, which would push it
    // down again (a browser does not hold its place at the top of the page),
    // so it is kept in view while they load: until the person moves the page
    // themselves, or for a few seconds.
    // Adresin adlandırdığı parça görünür kılınır ve üstündeki parçalar
    // yüklenirken, kişi sayfayı kendisi oynatana dek görünür tutulur.
    useEffect(() => {
        if (!named) return undefined;
        scrollToPart(named, fromIndex.current);
        fromIndex.current = false;
        const element = body.current;
        if (!element || typeof ResizeObserver === 'undefined') return undefined;
        const observer = new ResizeObserver(() => scrollToPart(named, false));
        const moves = ['wheel', 'touchstart', 'keydown', 'mousedown'] as const;
        const stop = () => {
            observer.disconnect();
            window.clearTimeout(timer);
            moves.forEach((name) => window.removeEventListener(name, stop));
        };
        const timer = window.setTimeout(stop, 4000);
        moves.forEach((name) => window.addEventListener(name, stop, { passive: true }));
        observer.observe(element);
        return stop;
    }, [named]);

    return (
        <div ref={body} className="space-y-4" data-section-body={section}>
            {parts.length > 1 && (
                <nav aria-label={t('domain.onThisPage')} className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm" data-section-index="">
                    <span className="text-fg-subtle">{t('domain.onThisPage')}</span>
                    {parts.map((item) => (
                        <a
                            key={item.id}
                            href={`#${partElementId(item.id)}`}
                            onClick={(event) => {
                                event.preventDefault();
                                if (item.id === named) {
                                    scrollToPart(item.id, true);
                                    return;
                                }
                                fromIndex.current = true;
                                void go({ section, part: item.id }, { replace: true });
                            }}
                            className="rounded font-medium text-primary underline underline-offset-4 hover:text-primary-hover"
                        >
                            {t(PART_LABELS[item.id])}
                        </a>
                    ))}
                </nav>
            )}
            {parts.map((item) => (
                <section
                    key={item.id}
                    id={partElementId(item.id)}
                    aria-label={t(PART_LABELS[item.id])}
                    className={unframed.includes(item.id) ? 'scroll-mt-4' : 'scroll-mt-4 rounded-xl border border-border bg-surface p-5'}
                    data-part={item.id}
                >
                    {item.readOnly && (
                        <p className="mb-4 flex items-start gap-2 rounded-lg border border-info/30 bg-info/10 px-3 py-2 text-sm text-fg" data-read-only="">
                            <Eye className="mt-0.5 h-4 w-4 shrink-0 text-info" aria-hidden="true" />
                            {t('domain.readOnly')}
                        </p>
                    )}
                    <Suspense fallback={<Spinner />}>{render(item)}</Suspense>
                </section>
            ))}
        </div>
    );
}
