import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { useI18n } from '../../i18n';
import { useLocation, useNavigate, useParams } from '../../router';
import { PageHeader } from '../../components/PageHeader';
import { Checking, CouldNotCheck } from '../../components/ui';
import { ChapterView, sectionId } from './ChapterView';
import { loadGuideChapter, readableGuide, useGuideChapter, useGuideIndex } from './load';
import { chapterBodies, searchGuide } from './search';
import { ViewAsSwitch } from './ViewAsSwitch';
import type { GuideRole } from './types';

// /help and /help/<chapter>#<section>: the whole guide for the reader's role,
// with search. The administrator may read another role's guide (?as=). The
// guide is public content; this page reads nothing from the server.
// /help: okuyucunun rolüne göre kılavuzun tamamı ve arama.

export function GuidePage() {
    const { t, locale } = useI18n();
    const { role } = useAuth();
    const { chapter: requested } = useParams();
    const location = useLocation();
    const navigate = useNavigate();
    const as = new URLSearchParams(location.search).get('as');
    const guideRole = readableGuide(role, as);
    const { loaded: indexLoad, retry } = useGuideIndex(guideRole, locale);
    const [query, setQuery] = useState('');
    const article = useRef<HTMLDivElement>(null);

    const chunk = indexLoad.state === 'known' ? indexLoad.value : null;
    const chapter = chunk ? (chunk.chapters.find((item) => item.id === requested) ?? chunk.chapters[0]) : undefined;
    const missing = Boolean(chunk && requested && chapter?.id !== requested);
    const anchor = decodeURIComponent(location.hash.replace(/^#/, ''));
    // Titles, headings and keywords are searched at once; the text of every
    // chapter is fetched on the first search (each chapter once) and joins
    // the results when it arrives. A chapter that could not be fetched is
    // searched by its title and headings only, and the list says so.
    const searching = Boolean(chunk && query.trim() !== '');
    const [bodies, setBodies] = useState<{ index: unknown; texts: Record<string, Record<string, string>>; state: 'loading' | 'known' | 'partial' } | null>(null);
    // The index whose texts are fetched or on their way; cleared after a
    // partial fetch, so the next search fetches the missing chapters again.
    const fetched = useRef<unknown>(null);
    useEffect(() => {
        if (!searching || !chunk || fetched.current === chunk) return;
        fetched.current = chunk;
        setBodies({ index: chunk, texts: {}, state: 'loading' });
        void Promise.allSettled(chunk.chapters.map((entry) => loadGuideChapter(locale, entry))).then((results) => {
            if (fetched.current !== chunk) return;
            const texts: Record<string, Record<string, string>> = {};
            for (const result of results) if (result.status === 'fulfilled') texts[result.value.id] = chapterBodies(result.value);
            const complete = results.every((result) => result.status === 'fulfilled');
            if (!complete) fetched.current = null;
            setBodies({ index: chunk, texts, state: complete ? 'known' : 'partial' });
        });
    }, [searching, chunk, locale, query]);
    const texts = bodies?.index === chunk ? bodies.texts : undefined;
    const hits = useMemo(() => (chunk && searching ? searchGuide(chunk, query, texts) : null), [chunk, query, searching, texts]);
    // The index lists the chapters; the one on screen is fetched on its own.
    const { loaded: chapterLoad, retry: retryChapter } = useGuideChapter(locale, chapter);
    const blocks = chapterLoad.state === 'known' && chapterLoad.value.id === chapter?.id ? chapterLoad.value : undefined;
    const known = useMemo(() => new Set(chunk?.chapters.map((item) => item.id) ?? []), [chunk]);

    const own = readableGuide(role, null);
    const address = (id: string, section = '', viewAs: GuideRole = guideRole) =>
        `/help/${id}${viewAs !== own ? `?as=${viewAs}` : ''}${section ? `#${section}` : ''}`;

    // A chapter or section that was asked for is scrolled into view and gets
    // the focus, so a keyboard or screen reader lands where the link pointed.
    useLayoutEffect(() => {
        if (!blocks || !article.current) return;
        const target = anchor ? article.current.querySelector<HTMLElement>(`#${CSS.escape(sectionId(blocks.id, anchor))}`) : null;
        if (target) {
            target.scrollIntoView({ block: 'start' });
            target.focus({ preventScroll: true });
        }
    }, [blocks, anchor]);

    const open = (id: string, section = '') => {
        setQuery('');
        navigate(address(id, section));
        if (!section) document.getElementById('celikpanel-main-content')?.scrollTo({ top: 0 });
    };

    return (
        <div className="p-4 sm:p-6 md:p-8" data-guide-page="">
            <PageHeader
                title={t('guide.title')}
                subtitle={t('guide.subtitle')}
                breadcrumb={[t('common.home'), t('guide.title')]}
                // Another role's guide opens at its start: the chapter on screen
                // may not be part of it.
                actions={role === 'admin' ? <ViewAsSwitch value={guideRole} onChange={(next) => navigate(next === own ? '/help' : `/help?as=${next}`)} /> : undefined}
            />
            {role === 'admin' && guideRole !== 'admin' && <p className="mb-4 text-sm text-fg-muted">{t('guide.viewAsHint')}</p>}

            {indexLoad.state === 'loading' && <Checking label={t('guide.loading')} />}
            {indexLoad.state === 'unknown' && <CouldNotCheck text={t('guide.unknown')} onRetry={retry} />}
            {chunk && chapter && (
                <div className="grid gap-6 lg:grid-cols-[16rem_minmax(0,1fr)]">
                    <nav aria-label={t('guide.chapters')} className="min-w-0 lg:sticky lg:top-0 lg:self-start">
                        <label htmlFor="guide-search" className="mb-1 block text-xs font-medium text-fg-muted">{t('guide.search.label')}</label>
                        <input
                            id="guide-search"
                            type="search"
                            value={query}
                            onChange={(event) => setQuery(event.target.value)}
                            placeholder={t('guide.search.placeholder')}
                            className="w-full rounded-lg border border-border-strong bg-surface px-3 py-1.5 text-sm text-fg outline-none placeholder:text-fg-subtle focus:border-primary max-sm:min-h-11"
                        />
                        {hits ? (
                            <div className="mt-3" aria-live="polite">
                                <p className="mb-1.5 text-xs text-fg-muted">
                                    {hits.length === 0 ? t('guide.search.none', { query: query.trim() }) : t('guide.search.count', { count: hits.length })}
                                </p>
                                {bodies?.index === chunk && bodies.state === 'loading' && <Checking className="mb-1.5" label={t('guide.search.text')} />}
                                {bodies?.index === chunk && bodies.state === 'partial' && <p className="mb-1.5 text-xs text-fg-muted">{t('guide.search.partial')}</p>}
                                <ul className="space-y-0.5">
                                    {hits.map((hit) => {
                                        const target = chunk.chapters[hit.chapter];
                                        return (
                                            <li key={`${target.id}#${hit.anchor}`}>
                                                <button type="button" onClick={() => open(target.id, hit.anchor)} className="w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-2 max-sm:min-h-11">
                                                    <span className="block font-medium text-fg">{hit.heading}</span>
                                                    {hit.anchor && <span className="block text-xs text-fg-muted">{target.title}</span>}
                                                </button>
                                            </li>
                                        );
                                    })}
                                </ul>
                            </div>
                        ) : (
                            <>
                                <p className="mb-1.5 mt-4 text-xs font-semibold uppercase tracking-wide text-fg-subtle">{t('guide.chapters')}</p>
                                <ol className="space-y-0.5">
                                    {chunk.chapters.map((item) => (
                                        <li key={item.id}>
                                            <a
                                                href={address(item.id)}
                                                aria-current={item.id === chapter.id ? 'page' : undefined}
                                                onClick={(event) => {
                                                    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
                                                    event.preventDefault();
                                                    open(item.id);
                                                }}
                                                className={`flex w-full rounded-md px-2 py-1.5 text-sm max-sm:min-h-11 max-sm:items-center ${
                                                    item.id === chapter.id ? 'bg-surface-2 font-semibold text-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg'
                                                }`}
                                            >
                                                {item.title}
                                            </a>
                                        </li>
                                    ))}
                                </ol>
                            </>
                        )}
                    </nav>
                    <article ref={article} className="min-w-0 max-w-3xl rounded-xl border border-border bg-surface p-5 md:p-6" aria-labelledby="guide-chapter-title">
                        {missing && <p className="mb-4 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg-muted">{t('guide.notInGuide')}</p>}
                        <h2 id="guide-chapter-title" className="text-xl font-bold tracking-tight text-fg">{chapter.title}</h2>
                        {chapterLoad.state === 'unknown' && <CouldNotCheck className="mt-4" text={t('guide.unknown')} onRetry={retryChapter} />}
                        {!blocks && chapterLoad.state !== 'unknown' && <Checking className="mt-4" label={t('guide.loading')} />}
                        {blocks && <ChapterView chapter={blocks} known={known} titleLevel={2} onGuideLink={open} />}
                    </article>
                </div>
            )}
        </div>
    );
}
