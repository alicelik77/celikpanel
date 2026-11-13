// Genel Bakış (alpha.132): the certificate card and one tile per section of
// the row, as the Overview was before (one tile per tab). The real Overview
// with facts (D2, D3) needs a summary read and comes in a later release.
// Genel Bakış: sertifika kartı ve satırdaki her bölüm için bir kutucuk.
import { useI18n } from '../../../i18n';
import { domainSectionIcons } from '../../../lib/domainSectionIcons';
import { DomainSSLOverviewCard } from '../../DomainSSLOverviewCard';
import { siteKindWord } from '../../kit/words';
import { SECTION_LABELS, siteLeadPart } from '../sections';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

export function OverviewSection({ domain, parts, part, sections, go, onCertificateChange }: DomainSectionProps) {
    const { t } = useI18n();
    const tiles = sections.filter((section) => section.id !== 'overview' && !section.notInPlan);
    const certificate = sections.some((section) => section.id === 'security' && section.parts.some((item) => item.id === 'certificate'));
    // A program or a service app: the Site tile says what runs there and goes
    // straight to it (the part leads the Site section). From the domain's
    // own facts; nothing more is read.
    const lead = siteLeadPart(domain.project_type);
    const leadShown = lead !== null && sections.some((section) => section.id === 'site' && section.parts.some((item) => item.id === lead));
    return (
        <SectionParts section="overview" parts={parts} part={part} go={go} render={() => (
            <div>
                {certificate && (
                    <DomainSSLOverviewCard
                        domainId={domain.id}
                        onOpen={() => go({ section: 'security', part: 'certificate' })}
                        onCertificateChange={onCertificateChange}
                    />
                )}
                <p className="mb-4 text-sm text-fg-muted">{t('domain.overview.hint')}</p>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    {tiles.map((section) => {
                        const Icon = domainSectionIcons[section.id];
                        const runs = section.id === 'site' && leadShown;
                        return (
                            <button
                                key={section.id}
                                type="button"
                                onClick={() => go(runs && lead ? { section: 'site', part: lead } : { section: section.id })}
                                className="group flex items-center gap-3 rounded-xl border border-border bg-surface p-4 text-left transition-colors hover:border-primary/40 hover:bg-surface-2"
                                data-overview-tile={section.id}
                            >
                                <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-surface-2 text-fg-muted transition-colors group-hover:bg-primary group-hover:text-primary-fg">
                                    <Icon className="h-5 w-5" aria-hidden="true" />
                                </span>
                                <span className="min-w-0">
                                    <span className="block text-sm font-semibold text-fg">{t(SECTION_LABELS[section.id])}</span>
                                    {runs && (
                                        <span className="block text-xs text-fg-muted" data-overview-runs="">
                                            {t('domain.overview.siteRuns', { kind: siteKindWord(t, domain.project_type) })}
                                        </span>
                                    )}
                                </span>
                            </button>
                        );
                    })}
                </div>
            </div>
        )} />
    );
}
