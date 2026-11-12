// The one row of sections on a domain's page (alpha.132, drawings X07, P1).
// Words with icons, the active one underlined; a section the plan does not
// include is drawn faded in its own place, with a lock, and still opens (its
// page says whom to ask). The order of the sections never changes. On a narrow screen the row scrolls sideways inside itself and
// keeps the active section in view; the page never scrolls sideways.
// Keyboard: the arrow keys, Home and End move between sections, Enter or
// Space opens one (a section loads its own screens, so moving does not).
//
// Alan adı sayfasındaki tek bölüm satırı. Planda olmayan bölüm kendi yerinde,
// soluk ve kilitli çizilir; açılınca kime başvurulacağını söyler. Dar ekranda satır
// kendi içinde yana kayar, etkin bölüm görünür kalır.
import { useEffect, useRef } from 'react';
import { Lock } from 'lucide-react';
import { useI18n } from '../../i18n';
import { domainSectionIcons } from '../../lib/domainSectionIcons';
import { nextTabIndex, revealTabInRow } from '../../lib/tablist';
import { SECTION_LABELS, type SectionId, type SectionView } from './sections';

export const SECTION_PANEL_ID = 'domain-section-panel';
export const sectionTabId = (id: SectionId) => `domain-section-${id}-tab`;

export function SectionRow({ sections, active, onSelect }: {
    sections: readonly SectionView[];
    active: SectionId;
    onSelect: (id: SectionId) => void;
}) {
    const { t } = useI18n();
    const tabs = useRef<Partial<Record<SectionId, HTMLButtonElement | null>>>({});

    useEffect(() => revealTabInRow(tabs.current[active]), [active]);

    const move = (key: string, index: number) => {
        const next = nextTabIndex(key, index, sections.length);
        if (next === null) return false;
        const tab = tabs.current[sections[next].id];
        tab?.focus();
        revealTabInRow(tab);
        return true;
    };

    return (
        <div
            role="tablist"
            aria-label={t('domain.sections')}
            className="mb-4 flex gap-1 overflow-x-auto border-b border-border [scrollbar-width:thin]"
            data-domain-sections=""
        >
            {sections.map((section, index) => {
                const Icon = domainSectionIcons[section.id];
                const selected = section.id === active;
                const faded = Boolean(section.notInPlan);
                return (
                    <button
                        key={section.id}
                        ref={(element) => { tabs.current[section.id] = element; }}
                        id={sectionTabId(section.id)}
                        type="button"
                        role="tab"
                        aria-selected={selected}
                        aria-controls={SECTION_PANEL_ID}
                        tabIndex={selected ? 0 : -1}
                        onClick={() => onSelect(section.id)}
                        onKeyDown={(event) => { if (move(event.key, index)) event.preventDefault(); }}
                        data-section={section.id}
                        data-not-in-plan={faded ? '' : undefined}
                        className={`relative -mb-px flex min-h-11 shrink-0 items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary ${
                            selected
                                ? 'border-primary text-primary'
                                : faded
                                  ? 'border-transparent text-fg-subtle hover:text-fg-muted'
                                  : 'border-transparent text-fg-muted hover:text-fg'
                        }`}
                    >
                        <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                        {t(SECTION_LABELS[section.id])}
                        {faded && (
                            <>
                                <Lock className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                                <span className="sr-only">{t('domain.section.notInPlan')}</span>
                            </>
                        )}
                    </button>
                );
            })}
        </div>
    );
}
