import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { useI18n } from '../../i18n';
import { useLocation, useNavigate, useParams } from '../../router';
import { PageHeader } from '../../components/PageHeader';
import { Checking, CouldNotCheck } from '../../components/ui';
import { ChapterView, sectionId } from './ChapterView';
import { readableGuide, useGuideChunk } from './load';
import { searchGuide } from './search';
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
    const { load, retry } = useGuideChunk(guideRole, locale);
    const [query, setQuery] = useState('');
    const article = useRef<HTMLDivElement>(null);

    const chunk = load.state === 'known' ? load.chunk : null;
    const chapter = chunk ? (chunk.chapters.find((item) => item.id === requested) ?? chunk.chapters[0]) : undefined;
    const missing = Boolean(chunk && requested && chapter?.id !== requested);
    const anchor = decodeURIComponent(location.hash.replace(/^#/, ''));
    const hits = useMemo(() => (chunk && query.trim() !== '' ? searchGuide(chunk, query) : null), [chunk, query]);

    const own = readableGuide(role, null);
    const address = (id: string, section = '', viewAs: GuideRole = guideRole) =>
        `/help/${id}${viewAs !== own ? `?as=${viewAs}` : ''}${section ? `#${section}` : ''}`;

    // A chapter or section that was asked for is scrolled into view and gets
    // the focus, so a keyboard or screen reader lands where the link pointed.
    useLayoutEffect(() => {
        if (!chapter || !article.current) return;
        const target = anchor ? article.current.querySelector<HTMLElement>(`#${CSS.escape(sectionId(chapter.id, anchor))}`) : null;
        if (target) {
            target.scrollIntoView({ block: 'start' });
            target.focus({ preventScroll: true });
        }
    }, [chapter, anchor]);

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

            {load.state === 'loading' && <Checking label={t('guide.loading')} />}
            {load.state === 'unknown' && <CouldNotCheck text={t('guide.unknown')} onRetry={retry} />}
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
                        <ChapterView chapter={chapter} titleLevel={2} onGuideLink={open} />
                    </article>
                </div>
            )}
        </div>
    );
}
