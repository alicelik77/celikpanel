import { AlertTriangle } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, SudoNote, mentionsSudo } from './ui';

// The one way to show what stops a screen's main action (owner feedback
// 2026-10-31; D-024). The problems stand at the TOP of what they block, not
// under a long list; each says what is wrong in one plain sentence, what to do
// in one concrete action and, where the panel can, offers the fix as a button.
// The code stays under "Technical details". Next to the disabled main button,
// BlockedReason says why it cannot be pressed and brings the problems into
// view; the button names it with aria-describedby.
//
// Ekranin ana eylemini durduran sorunlarin tek gosterimi: engelledigi yerin
// en ustunde; her biri ne yanlis (tek sade cumle), ne yapilmali (tek somut
// eylem) ve mumkunse duzeltme dugmesi. Kod "Teknik ayrintilar" altinda kalir.
// Devre disi ana dugmenin yaninda BlockedReason nedenini soyler ve sorunlari
// gorunur alana getirir; dugme onu aria-describedby ile adlandirir.

export interface BlockingProblem {
    /** Stable code, shown under Technical details only. */
    code: string;
    /** What is wrong: one plain sentence, no internal terms. */
    what: string;
    /** What to do: one concrete action. */
    todo?: string;
    /** A further technical line (the server's own words), with the code. */
    detail?: string;
    /** The fix, when the panel can perform it or go where it is made. */
    fix?: { label: string; onFix: () => void; busy?: boolean; disabled?: boolean };
}

export function BlockingProblems({ id, title, problems, className }: {
    id: string;
    /** The heading; by default "One thing needs fixing first" / "{count} things…". */
    title?: string;
    problems: BlockingProblem[];
    className?: string;
}) {
    const { t } = useI18n();
    if (problems.length === 0) return null;
    return (
        <section
            id={id}
            tabIndex={-1}
            role="alert"
            aria-labelledby={`${id}-title`}
            data-blocking-problems={problems.length}
            className={`scroll-mt-6 rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary ${className ?? ''}`}
        >
            <h3 id={`${id}-title`} className="flex items-start gap-2 text-base font-semibold">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span>{title ?? t(problems.length === 1 ? 'problem.title.one' : 'problem.title.many', { count: problems.length })}</span>
            </h3>
            <ol className="mt-3 space-y-4">
                {problems.map(problem => (
                    <li key={problem.code} data-problem={problem.code} className="min-w-0 border-t border-warning-mark/30 pt-3 first:border-t-0 first:pt-0">
                        <p className="max-w-[75ch] break-words font-medium">{problem.what}</p>
                        {problem.todo && <p className="mt-1 max-w-[75ch] break-words"><span className="font-semibold">{t('problem.todo')}</span> {problem.todo}</p>}
                        {problem.todo && mentionsSudo(problem.todo) && <SudoNote />}
                        {problem.fix && (
                            <Button type="button" variant="primary" className="mt-2 max-sm:min-h-11" data-problem-fix={problem.code}
                                loading={problem.fix.busy} disabled={problem.fix.disabled} onClick={problem.fix.onFix}>
                                {problem.fix.label}
                            </Button>
                        )}
                        <details className="mt-2 text-xs text-fg-muted">
                            <summary className="cursor-pointer">{t('problem.details')}</summary>
                            <code className="mt-1 block break-words">{problem.code}</code>
                            {problem.detail && <p className="mt-1 break-words">{problem.detail}</p>}
                        </details>
                    </li>
                ))}
            </ol>
        </section>
    );
}

/**
 * Beside a disabled main button: why it cannot be pressed, and a "Show" that
 * scrolls to and focuses `target` (the problems, or the box to tick). Give the
 * button aria-describedby={id}.
 */
export function BlockedReason({ id, target, text, className }: { id: string; target: string; text: string; className?: string }) {
    const { t } = useI18n();
    const show = () => {
        const element = typeof document === 'undefined' ? null : document.getElementById(target);
        element?.scrollIntoView?.({ block: 'start', behavior: 'smooth' });
        element?.focus?.({ preventScroll: true });
    };
    return (
        <p id={id} data-blocked-reason={target} className={`flex flex-wrap items-center gap-x-2 text-sm leading-6 text-fg ${className ?? ''}`}>
            <span>{text}</span>
            <button type="button" onClick={show} className="min-h-11 font-medium text-primary underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary sm:min-h-0">
                {t('problem.show')}
            </button>
        </p>
    );
}
