// Genel Bakış (alpha.132): the certificate card and one tile per section of
// the row, as the Overview was before (one tile per tab). The real Overview
// with facts (D2, D3) needs a summary read and comes in a later release.
// Genel Bakış: sertifika kartı ve satırdaki her bölüm için bir kutucuk.
import { useI18n } from '../../../i18n';
import { domainSectionIcons } from '../../../lib/domainSectionIcons';
import { DomainSSLOverviewCard } from '../../DomainSSLOverviewCard';
import { SECTION_LABELS } from '../sections';
import { SectionParts, type DomainSectionProps } from '../SectionParts';

export function OverviewSection({ domain, parts, part, sections, go, onCertificateChange }: DomainSectionProps) {
    const { t } = useI18n();
    const tiles = sections.filter((section) => section.id !== 'overview' && !section.notInPlan);
    const certificate = sections.some((section) => section.id === 'security' && section.parts.some((item) => item.id === 'certificate'));
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
                        return (
                            <button
                                key={section.id}
                                type="button"
                                onClick={() => go({ section: section.id })}
                                className="group flex items-center gap-3 rounded-xl border border-border bg-surface p-4 text-left transition-colors hover:border-primary/40 hover:bg-surface-2"
                                data-overview-tile={section.id}
                            >
                                <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-surface-2 text-fg-muted transition-colors group-hover:bg-primary group-hover:text-primary-fg">
                                    <Icon className="h-5 w-5" aria-hidden="true" />
                                </span>
                                <span className="text-sm font-semibold text-fg">{t(SECTION_LABELS[section.id])}</span>
                            </button>
                        );
                    })}
                </div>
            </div>
        )} />
    );
}
