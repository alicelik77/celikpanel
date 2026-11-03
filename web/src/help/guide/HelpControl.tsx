import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react';
import { BookOpen, HelpCircle, Map as MapIcon } from 'lucide-react';
import { useAuth } from '../../auth/AuthContext';
import { useI18n } from '../../i18n';
import { Link, useLocation } from '../../router';
import { showToast } from '../../components/Toast';
import { Spinner } from '../../components/ui';
import { GUIDE_OPEN_EVENT, publishedGuideTopic } from './context';
import { topicForPath } from './routes';
import { readGuideState, saveGuideState } from '../../lib/guideState';
import { tourDecision, TOUR_VERSION, type TourState } from './tourState';

// The "?" in the page header (alpha.119): the help menu - help for this page,
// the whole guide and the tour - and the host of the guide drawer and of the
// first-login tour. The shell loads it once its screen copy is here. It
// reads one row (the tour state) once per page load and writes it only when
// the person finishes or skips the tour; nothing else of the guide touches
// the server.
// Sayfa başlığındaki "?": yardım menüsü, kılavuz çekmecesi ve tur.

const GuideDrawer = lazy(() => import('./GuideDrawer').then((module) => ({ default: module.GuideDrawer })));
const Tour = lazy(() => import('./Tour').then((module) => ({ default: module.Tour })));
const TourOffer = lazy(() => import('./Tour').then((module) => ({ default: module.TourOffer })));

// One read per page load and person: the help control is mounted again with
// every layout, and an answer must not bring the tour back after a skip that
// could not be saved.
// Sayfa yüklemesi ve kişi başına tek okuma.
const decided = new Map<string, Promise<ReturnType<typeof tourDecision>>>();

function readDecision(username: string, impersonating: boolean): Promise<ReturnType<typeof tourDecision>> {
    let decision = decided.get(username);
    if (!decision) {
        decision = readGuideState().then((state) => tourDecision(state, impersonating));
        decided.set(username, decision);
    }
    return decision;
}

export function HelpControl() {
    const { t } = useI18n();
    const { user } = useAuth();
    const location = useLocation();
    const [open, setOpen] = useState(false);
    const [drawer, setDrawer] = useState<Array<string | null> | null>(null);
    const [tour, setTour] = useState<'tour' | 'offer' | null>(null);
    const box = useRef<HTMLDivElement>(null);
    const button = useRef<HTMLButtonElement>(null);
    const impersonating = Boolean(user.impersonating);

    useEffect(() => {
        let current = true;
        readDecision(user.username, impersonating).then((decision) => {
            if (!current) return;
            if (decision === 'show') setTour('tour');
            else if (decision === 'offer') setTour('offer');
        });
        return () => {
            current = false;
        };
    }, [user.username, impersonating]);

    // A screen asks for a chapter (GuideLink).
    useEffect(() => {
        const onOpen = (event: Event) => {
            const topic = (event as CustomEvent<{ topic?: string }>).detail?.topic;
            if (typeof topic === 'string') setDrawer([topic, publishedGuideTopic(), topicForPath(location.pathname)]);
        };
        window.addEventListener(GUIDE_OPEN_EVENT, onOpen);
        return () => window.removeEventListener(GUIDE_OPEN_EVENT, onOpen);
    }, [location.pathname]);

    // The menu is a disclosure like the personal menu: Escape closes it and
    // gives the focus back, and it closes when the focus leaves it.
    useEffect(() => {
        if (!open) return;
        const onDown = (event: MouseEvent) => {
            if (box.current && !box.current.contains(event.target as Node)) setOpen(false);
        };
        const onKey = (event: KeyboardEvent) => {
            if (event.key !== 'Escape') return;
            setOpen(false);
            button.current?.focus();
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
        };
    }, [open]);

    const save = useCallback((state: TourState) => {
        // The decision is made for this page load whatever the answer: a
        // failed save may offer the tour again at the next sign-in, and says so.
        decided.set(user.username, Promise.resolve('none'));
        setTour(null);
        if (impersonating) return;
        void saveGuideState(TOUR_VERSION, state).then((saved) => {
            if (!saved) showToast('warning', t('guide.tour.saveFailed'));
        });
    }, [impersonating, t, user.username]);

    const itemClass = 'flex w-full items-center gap-2 px-4 py-2.5 text-sm text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg max-sm:min-h-11';

    return (
        <div
            ref={box}
            className="relative"
            data-tour="help"
            onBlur={(event) => {
                if (open && !event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false);
            }}
        >
            <button
                ref={button}
                type="button"
                aria-expanded={open}
                aria-controls={open ? 'help-menu' : undefined}
                aria-label={t('guide.button')}
                title={t('guide.button')}
                onClick={() => setOpen((value) => !value)}
                className="flex h-9 w-9 items-center justify-center rounded-lg text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary max-sm:h-11 max-sm:w-11"
                data-help-button=""
            >
                <HelpCircle className="h-5 w-5" aria-hidden="true" />
            </button>
            {open && (
                <div id="help-menu" data-help-menu="" className="absolute right-0 z-20 mt-2 w-60 overflow-hidden rounded-xl border border-border bg-surface py-1 shadow-lg">
                    <button
                        type="button"
                        className={itemClass}
                        onClick={() => {
                            setOpen(false);
                            setDrawer([publishedGuideTopic(), topicForPath(location.pathname)]);
                        }}
                    >
                        <HelpCircle className="h-4 w-4" aria-hidden="true" />
                        {t('guide.menu.page')}
                    </button>
                    <Link to="/help" onClick={() => setOpen(false)} className={itemClass}>
                        <BookOpen className="h-4 w-4" aria-hidden="true" />
                        {t('guide.menu.guide')}
                    </Link>
                    <button
                        type="button"
                        className={itemClass}
                        onClick={() => {
                            setOpen(false);
                            setTour('tour');
                        }}
                    >
                        <MapIcon className="h-4 w-4" aria-hidden="true" />
                        {t('guide.menu.tour')}
                    </button>
                </div>
            )}
            {/* The drawer's part is fetched when it is first opened; until it
                arrives, a spinner stands where the drawer will be. */}
            <Suspense fallback={drawer ? <span className="fixed right-4 top-16 z-50"><Spinner /></span> : null}>
                {drawer && (
                    <GuideDrawer
                        topics={drawer}
                        onClose={() => {
                            setDrawer(null);
                            button.current?.focus();
                        }}
                    />
                )}
                {tour === 'tour' && <Tour onFinish={() => save('completed')} onSkip={() => save('skipped')} />}
                {tour === 'offer' && <TourOffer onTake={() => setTour('tour')} onDismiss={() => save('skipped')} />}
            </Suspense>
        </div>
    );
}
