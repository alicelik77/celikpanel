import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { useI18n } from '../../i18n';
import { Button } from '../../components/ui';
import { useFocusTrap } from './focusTrap';
import { tourSteps, visibleSteps } from './tourState';

// The first-login tour (alpha.119): a few steps that point at the menu and the
// header (elements marked data-tour in Layout). Skippable at every step (the
// button or Escape); Finish and Skip are both saved by the caller. Steps whose
// element is not on screen (a menu item this account does not have, the rail
// on a phone) are left out. Motion only where the person allows it.
// İlk giriş turu: menüyü ve başlığı gösteren birkaç adım; her adımda geçilebilir.

const MARGIN = 16;
const GAP = 12;

function findAnchor(anchor: string): HTMLElement | null {
    for (const element of document.querySelectorAll<HTMLElement>(`[data-tour="${anchor}"]`)) {
        if (element.getClientRects().length > 0) return element;
    }
    return null;
}

interface Box {
    top: number;
    left: number;
    width: number;
    height: number;
}

export function Tour({ onFinish, onSkip }: { onFinish: () => void; onSkip: () => void }) {
    const { t } = useI18n();
    const { role } = useAuth();
    const steps = useMemo(() => visibleSteps(tourSteps(role), (anchor) => findAnchor(anchor) !== null), [role]);
    const [index, setIndex] = useState(0);
    const [target, setTarget] = useState<Box | null>(null);
    const [size, setSize] = useState({ width: 0, height: 0 });
    const dialog = useRef<HTMLDivElement>(null);
    const primary = useRef<HTMLButtonElement>(null);
    useFocusTrap(dialog, onSkip, primary);

    const step = steps[index];
    const last = index === steps.length - 1;

    // Measure the element the step points at, again when the page moves.
    const measure = useCallback(() => {
        const element = step?.anchor ? findAnchor(step.anchor) : null;
        const box = element?.getBoundingClientRect();
        setTarget(box ? { top: box.top, left: box.left, width: box.width, height: box.height } : null);
        if (dialog.current) setSize({ width: dialog.current.offsetWidth, height: dialog.current.offsetHeight });
    }, [step]);
    useLayoutEffect(() => {
        const element = step?.anchor ? findAnchor(step.anchor) : null;
        element?.scrollIntoView({ block: 'nearest' });
        measure();
        primary.current?.focus();
    }, [measure, step]);
    useEffect(() => {
        window.addEventListener('resize', measure);
        window.addEventListener('scroll', measure, true);
        return () => {
            window.removeEventListener('resize', measure);
            window.removeEventListener('scroll', measure, true);
        };
    }, [measure]);

    if (!step) return null;

    // Beside the element when there is room, else under or over it; centred
    // when the step points at nothing.
    let position: React.CSSProperties = { top: '50%', left: '50%', transform: 'translate(-50%, -50%)' };
    if (target) {
        const width = size.width || 352;
        const height = size.height || 200;
        const clampLeft = (left: number) => Math.max(MARGIN, Math.min(left, window.innerWidth - width - MARGIN));
        const clampTop = (top: number) => Math.max(MARGIN, Math.min(top, window.innerHeight - height - MARGIN));
        if (target.left + target.width + GAP + width + MARGIN <= window.innerWidth && target.left < window.innerWidth / 2) {
            position = { top: clampTop(target.top), left: target.left + target.width + GAP };
        } else if (target.top + target.height + GAP + height + MARGIN <= window.innerHeight) {
            position = { top: target.top + target.height + GAP, left: clampLeft(target.left + target.width - width) };
        } else {
            position = { top: clampTop(target.top - height - GAP), left: clampLeft(target.left + target.width - width) };
        }
    }

    return (
        <div className="fixed inset-0 z-[70]" data-tour-overlay="">
            {target ? (
                <div
                    aria-hidden="true"
                    className="pointer-events-none fixed rounded-lg ring-2 ring-primary motion-safe:transition-all motion-safe:duration-200"
                    style={{
                        top: target.top - 4,
                        left: target.left - 4,
                        width: target.width + 8,
                        height: target.height + 8,
                        boxShadow: '0 0 0 9999px rgb(var(--scrim) / 0.6)',
                    }}
                />
            ) : (
                <div aria-hidden="true" className="absolute inset-0 bg-scrim/60" />
            )}
            <div
                ref={dialog}
                role="dialog"
                aria-modal="true"
                aria-label={t('guide.tour.label')}
                aria-describedby="guide-tour-text"
                className="fixed w-[min(22rem,calc(100vw-2rem))] rounded-xl border border-border-strong bg-surface p-4 text-fg shadow-xl motion-safe:transition-[top,left] motion-safe:duration-200"
                style={position}
                data-tour-step={step.anchor ?? 'welcome'}
            >
                <div aria-live="polite">
                    <p className="text-xs text-fg-muted">{t('guide.tour.progress', { step: index + 1, total: steps.length })}</p>
                    <h2 className="mt-1 text-base font-semibold">{t(step.title)}</h2>
                    <p id="guide-tour-text" className="mt-1.5 text-sm leading-relaxed text-fg-muted">{t(step.text)}</p>
                </div>
                <div className="mt-4 flex flex-wrap items-center gap-2 max-sm:[&>*]:min-h-11">
                    <button type="button" onClick={onSkip} className="mr-auto rounded-md px-1 py-1.5 text-sm font-medium text-fg-muted underline underline-offset-2 hover:text-fg">
                        {t('guide.tour.skip')}
                    </button>
                    {index > 0 && (
                        <Button type="button" onClick={() => setIndex(index - 1)}>
                            {t('guide.tour.back')}
                        </Button>
                    )}
                    {/* The shared Button does not take a ref; this is its primary look. */}
                    <button
                        ref={primary}
                        type="button"
                        onClick={() => (last ? onFinish() : setIndex(index + 1))}
                        className="inline-flex items-center gap-1.5 rounded-md border border-transparent bg-primary px-3 py-1.5 text-sm font-medium text-primary-fg transition-colors hover:bg-primary-hover"
                    >
                        {t(last ? 'guide.tour.finish' : 'guide.tour.next')}
                    </button>
                </div>
            </div>
        </div>
    );
}

// After TOUR_VERSION is raised the tour is offered, never started: a quiet
// card that does not take the focus or cover the page.
// Sürüm artınca tur yalnız önerilir: odağı almayan, sayfayı örtmeyen bir kart.
export function TourOffer({ onTake, onDismiss }: { onTake: () => void; onDismiss: () => void }) {
    const { t } = useI18n();
    return (
        <section
            aria-labelledby="guide-tour-offer-title"
            className="fixed bottom-4 right-4 z-40 w-[min(22rem,calc(100vw-2rem))] rounded-xl border border-border-strong bg-surface p-4 text-fg shadow-lg"
            data-tour-offer=""
        >
            <h2 id="guide-tour-offer-title" className="text-sm font-semibold">{t('guide.tour.offer.title')}</h2>
            <p className="mt-1 text-sm leading-relaxed text-fg-muted">{t('guide.tour.offer.text')}</p>
            <div className="mt-3 flex flex-wrap justify-end gap-2 max-sm:[&>*]:min-h-11">
                <Button type="button" onClick={onDismiss}>{t('guide.tour.offer.dismiss')}</Button>
                <Button type="button" variant="primary" onClick={onTake}>{t('guide.tour.offer.take')}</Button>
            </div>
        </section>
    );
}
