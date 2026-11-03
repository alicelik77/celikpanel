import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { BookOpen, X } from 'lucide-react';
import { useAuth } from '../../auth/AuthContext';
import { useI18n } from '../../i18n';
import { Link } from '../../router';
import { Checking, CouldNotCheck } from '../../components/ui';
import { ChapterView, sectionId } from './ChapterView';
import { useFocusTrap } from './focusTrap';
import { readableGuide, useGuideChapter, useGuideIndex } from './load';
import { placeFor } from './routes';
import { ViewAsSwitch } from './ViewAsSwitch';
import type { GuideRole } from './types';

// The guide beside the page: opened by "?" (the page's own topic) or by a
// GuideLink (the topic it names). It shows one chapter at the section that
// explains the page, and keeps the page where it was. Informational, so the
// backdrop and Escape close it; the focus stays inside while it is open.
// Sayfanın yanındaki kılavuz; sayfayı anlatan bölümü açar, sayfa yerinde kalır.

interface Props {
    /** Topics to look for, most specific first. */
    topics: Array<string | null>;
    onClose: () => void;
}

export function GuideDrawer({ topics, onClose }: Props) {
    const { t, locale } = useI18n();
    const { role } = useAuth();
    const [viewAs, setViewAs] = useState<GuideRole>(() => readableGuide(role, null));
    const { loaded: indexLoad, retry } = useGuideIndex(viewAs, locale);
    const [place, setPlace] = useState<{ chapter: string; anchor: string; fallback: boolean } | null>(null);
    const panel = useRef<HTMLElement>(null);
    const body = useRef<HTMLDivElement>(null);
    const closeButton = useRef<HTMLButtonElement>(null);
    useFocusTrap(panel, onClose, closeButton);

    // Where the drawer opens follows the topics and the guide being read.
    // topics is a fresh array from the caller each render; its words are the key.
    const index = indexLoad.state === 'known' ? indexLoad.value : null;
    const topicKey = topics.join(' ');
    useEffect(() => {
        if (index) setPlace(placeFor(index, topicKey.split(' ')));
    }, [index, topicKey]);

    // The index names the chapter; its blocks are fetched when it is opened.
    const chapter = index && place ? index.chapters.find((item) => item.id === place.chapter) : undefined;
    const { loaded: chapterLoad, retry: retryChapter } = useGuideChapter(locale, chapter);
    const blocks = chapterLoad.state === 'known' && chapterLoad.value.id === chapter?.id ? chapterLoad.value : undefined;
    const known = useMemo(() => new Set(index?.chapters.map((item) => item.id) ?? []), [index]);

    useLayoutEffect(() => {
        if (!place || !body.current || !blocks) return;
        const target = place.anchor ? body.current.querySelector<HTMLElement>(`#${CSS.escape(sectionId(place.chapter, place.anchor))}`) : null;
        if (target) target.scrollIntoView({ block: 'start' });
        else body.current.scrollTop = 0;
    }, [place, blocks]);

    const full = chapter ? `/help/${chapter.id}${viewAs !== readableGuide(role, null) ? `?as=${viewAs}` : ''}${place?.anchor ? `#${place.anchor}` : ''}` : '/help';

    return (
        <div className="fixed inset-0 z-50" data-guide-drawer="">
            <div className="absolute inset-0 bg-scrim/70" onClick={onClose} aria-hidden="true" />
            <aside
                ref={panel}
                role="dialog"
                aria-modal="true"
                aria-labelledby="guide-drawer-title"
                className="absolute inset-y-0 right-0 flex w-full max-w-lg flex-col border-l border-border bg-surface shadow-2xl"
            >
                <header className="flex items-start gap-2.5 border-b border-border px-5 py-4">
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                        <BookOpen className="h-4 w-4" aria-hidden="true" />
                    </span>
                    <div className="min-w-0 flex-1">
                        <p className="text-xs text-fg-subtle">{t('guide.title')}</p>
                        <h2 id="guide-drawer-title" className="break-words text-sm font-semibold text-fg">{chapter?.title ?? t('guide.title')}</h2>
                    </div>
                    <button
                        ref={closeButton}
                        type="button"
                        onClick={onClose}
                        className="rounded-lg p-1.5 text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg max-sm:min-h-11 max-sm:min-w-11"
                        aria-label={t('guide.close')}
                    >
                        <X className="h-4 w-4" aria-hidden="true" />
                    </button>
                </header>
                {role === 'admin' && (
                    <div className="border-b border-border px-5 py-2.5">
                        <ViewAsSwitch value={viewAs} onChange={setViewAs} />
                    </div>
                )}

                <div ref={body} className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
                    {indexLoad.state === 'loading' && <Checking label={t('guide.loading')} />}
                    {indexLoad.state === 'unknown' && <CouldNotCheck text={t('guide.unknown')} onRetry={retry} />}
                    {place?.fallback && <p className="mb-3 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg-muted">{t('guide.noTopic')}</p>}
                    {chapter && chapterLoad.state === 'unknown' && <CouldNotCheck text={t('guide.unknown')} onRetry={retryChapter} />}
                    {chapter && !blocks && chapterLoad.state !== 'unknown' && <Checking label={t('guide.loading')} />}
                    {blocks && (
                        <ChapterView
                            chapter={blocks}
                            known={known}
                            titleLevel={2}
                            onGuideLink={(target, anchor) => setPlace({ chapter: target, anchor, fallback: false })}
                        />
                    )}
                </div>

                <footer className="flex items-center justify-between gap-3 border-t border-border px-5 py-3">
                    <Link to={full} onClick={onClose} className="inline-flex min-h-9 items-center text-sm font-medium text-primary underline underline-offset-2 hover:text-primary-hover max-sm:min-h-11">
                        {t('guide.openFull')}
                    </Link>
                </footer>
            </aside>
        </div>
    );
}
