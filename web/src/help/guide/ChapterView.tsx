import { createElement, Fragment, type ReactNode } from 'react';
import { AlertTriangle, Info } from 'lucide-react';
import { useI18n } from '../../i18n';
import type { GuideBlock, GuideChapter, GuideInline } from './types';

// One chapter, drawn from the generated blocks with React elements only (no
// HTML strings): the content is the reviewed subset of docs/guide/FORMAT.md.
// Section headings sit one level under the chapter title the container
// draws. A guide link calls onGuideLink; an outside link opens a new tab.
// Bir bölüm, üretilmiş bloklardan yalnız React öğeleriyle çizilir.

export const sectionId = (chapter: string, anchor: string) => `guide-${chapter}-${anchor}`;

interface Props {
    chapter: GuideChapter;
    /** The level of the chapter's own title; sections are drawn below it. */
    titleLevel: 1 | 2;
    onGuideLink: (chapter: string, anchor: string) => void;
}

export function ChapterView({ chapter, titleLevel, onGuideLink }: Props) {
    const { t } = useI18n();

    const inline = (value: GuideInline, key?: number): ReactNode => {
        if (typeof value === 'string') return value;
        return createElement(Fragment, { key }, ...value.map((part, index) => {
            if (typeof part === 'string') return part;
            switch (part[0]) {
                case 'b':
                    return <strong key={index} className="font-semibold text-fg">{inline(part[1])}</strong>;
                case 'c':
                    return <code key={index} className="rounded border border-border bg-surface-2 px-1 py-px font-mono text-[0.85em] text-fg [overflow-wrap:anywhere]">{part[1]}</code>;
                case 'g': {
                    const [, target, anchor, label] = part;
                    return (
                        <a
                            key={index}
                            href={`/help/${target}${anchor ? `#${anchor}` : ''}`}
                            onClick={(event) => {
                                if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
                                event.preventDefault();
                                onGuideLink(target, anchor);
                            }}
                            className="font-medium text-primary underline underline-offset-2 hover:text-primary-hover"
                        >
                            {inline(label)}
                        </a>
                    );
                }
                default:
                    return (
                        <a key={index} href={part[1]} target="_blank" rel="noopener noreferrer" className="font-medium text-primary underline underline-offset-2 hover:text-primary-hover">
                            {inline(part[2])}
                            <span className="sr-only"> {t('guide.external')}</span>
                        </a>
                    );
            }
        }));
    };

    let heading = chapter.title;
    const block = (b: GuideBlock, index: number): ReactNode => {
        switch (b[0]) {
            case 'h': {
                const [, level, anchor, text] = b;
                heading = typeof text === 'string' ? text : heading;
                const tag = `h${level + titleLevel - 1}` as 'h2' | 'h3' | 'h4';
                return createElement(tag, {
                    key: index,
                    id: sectionId(chapter.id, anchor),
                    tabIndex: -1,
                    className: `${level === 2 ? 'mt-7 text-base' : 'mt-5 text-sm'} scroll-mt-4 font-semibold text-fg focus:outline-none`,
                }, inline(text));
            }
            case 'p':
                return <p key={index} className="mt-3 text-sm leading-relaxed text-fg">{inline(b[1])}</p>;
            case 'ul':
            case 'ol': {
                const List = b[0];
                return (
                    <List key={index} className={`mt-3 space-y-1.5 pl-5 text-sm leading-relaxed text-fg ${List === 'ul' ? 'list-disc' : 'list-decimal'}`}>
                        {b[1].map((item, i) => <li key={i}>{inline(item)}</li>)}
                    </List>
                );
            }
            case 't':
                return (
                    <div key={index} className="mt-3 overflow-x-auto rounded-lg border border-border" tabIndex={0} role="region" aria-label={t('guide.table', { name: heading })}>
                        <table className="w-full min-w-[28rem] text-left text-sm">
                            <thead className="bg-surface-2 text-xs text-fg-muted">
                                <tr>{b[1].map((cell, i) => <th key={i} scope="col" className="px-3 py-2 font-semibold">{inline(cell)}</th>)}</tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {b[2].map((row, r) => (
                                    <tr key={r}>{row.map((cell, i) => <td key={i} className="px-3 py-2 align-top leading-relaxed text-fg">{inline(cell)}</td>)}</tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                );
            case 'pre':
                return (
                    <pre key={index} className="mt-3 overflow-x-auto rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs leading-relaxed text-fg" dir="ltr">
                        <code>{b[1]}</code>
                    </pre>
                );
            default: {
                // A note is the neutral surface; a warning the attention one
                // (never the failure red: it warns, nothing has failed).
                const warning = b[0] === 'warn';
                const Icon = warning ? AlertTriangle : Info;
                return (
                    <aside
                        key={index}
                        className={`mt-4 flex gap-2.5 rounded-lg border p-3 ${warning ? 'border-warning-mark/50 bg-warning-mark/20' : 'border-border-strong bg-surface-2'}`}
                    >
                        <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${warning ? 'text-warning' : 'text-fg-muted'}`} aria-hidden="true" />
                        <div className="min-w-0 [&>:first-child]:mt-0">
                            <p className="text-xs font-semibold uppercase tracking-wide text-fg-muted">{t(warning ? 'guide.warning' : 'guide.note')}</p>
                            {b[1].map(block)}
                        </div>
                    </aside>
                );
            }
        }
    };

    return <div data-guide-chapter={chapter.id}>{chapter.blocks.map(block)}</div>;
}
