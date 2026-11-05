import { useState } from 'react';
import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { readServiceActionNote } from '../lib/serviceActionNote';
import { componentName } from '../lib/componentName';
import { invalidate, useInvalidation, useRefreshEvery, useRemote } from '../lib/remote';
import { Button } from './ui';
import { isServiceActionOutcome, ServiceActionNotice } from './ServiceActionNotice';
import { showToast } from './Toast';

// "Problems now" (alpha.126, Monitoring stage 1; D-024). One list at the top
// of Monitoring and on the Dashboard, computed on the server only from what
// the panel already reads (cmd/panel/monitoring_problems.go). Each item says
// what is wrong, who acts and how it clears, with the one next action beside
// it: Start for a stopped service (the same request and the same outcome
// notice as the component's own page), otherwise a link to the screen where
// it is done. What could not be checked is said in one quiet line, never
// drawn as "no problem". The list reads again every minute and whenever a
// component or a domain changes; reading it never changes anything.
//
// "Şu anki sorunlar": İzleme'nin üstünde ve Gösterge Paneli'nde tek liste.
// Her kalem neyin yanlış olduğunu, kimin işlem yapacağını ve nasıl
// kalkacağını söyler; yanında tek sonraki eylem durur. Bakılamayan şey "sorun
// yok" diye çizilmez. Liste her dakika ve bir bileşen ya da alan adı
// değişince yeniden okunur; okumak hiçbir şeyi değiştirmez.

export const PROBLEMS_URL = '/api/v1/metrics/problems';
const REFRESH_MS = 60_000;

export type ProblemKind =
    | 'service_stopped'
    | 'disk_full'
    | 'certificate_expired'
    | 'certificate_renewal_failed'
    | 'certificate_expiring'
    | 'memory_sustained'
    | 'cpu_sustained'
    | 'mail_queue_deferred'
    | 'security_updates';

const KINDS: readonly ProblemKind[] = [
    'service_stopped', 'disk_full', 'certificate_expired', 'certificate_renewal_failed', 'certificate_expiring',
    'memory_sustained', 'cpu_sustained', 'mail_queue_deferred', 'security_updates',
];
const SOURCES = ['services', 'metrics', 'disk', 'certificates', 'mail_queue', 'security_updates'] as const;
const REASONS = ['checking', 'unreadable', 'not_scanned'] as const;

export interface Problem {
    key: string;
    kind: ProblemKind;
    severity: 'warning' | 'critical';
    actor: string;
    subject?: string;
    name?: string;
    unit?: string;
    value: number;
    since?: string;
    observed_at?: string;
    action: { kind: 'start_service' | 'open'; to?: string };
}

export interface ProblemsReport {
    checked_at: string;
    problems: Problem[];
    unknown: { source: (typeof SOURCES)[number]; reason: (typeof REASONS)[number] }[];
}

// A problem of a kind this screen has no words for is left out rather than
// shown without its reason; an answer of another shape is not a list.
export function decodeProblems(raw: unknown): ProblemsReport {
    const body = raw as Partial<ProblemsReport> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.problems) || !Array.isArray(body.unknown)) throw new Error('shape');
    const problems = body.problems.filter((p): p is Problem => (
        !!p && typeof p === 'object' && KINDS.includes(p.kind) && typeof p.key === 'string'
        && (p.severity === 'warning' || p.severity === 'critical') && typeof p.value === 'number'
        && !!p.action && (p.action.kind === 'open' ? typeof p.action.to === 'string' && p.action.to.startsWith('/') : p.action.kind === 'start_service' && typeof p.unit === 'string')
    ));
    const unknown = body.unknown.filter((u): u is ProblemsReport['unknown'][number] => (
        !!u && SOURCES.includes(u.source) && REASONS.includes(u.reason)
    ));
    return { checked_at: typeof body.checked_at === 'string' ? body.checked_at : '', problems, unknown };
}

export function ProblemsNow({
    omit = [],
    here,
    quiet = false,
}: {
    /** Kinds another card on the same screen already shows. */
    omit?: readonly ProblemKind[];
    /** The screen this list is on: a link to it is not drawn. */
    here?: string;
    /** Dashboard: nothing while reading, when clear or when the list cannot be read. */
    quiet?: boolean;
}) {
    const { t, locale } = useI18n();
    const handle = useRemote(PROBLEMS_URL, decodeProblems);
    useRefreshEvery(handle, REFRESH_MS);
    useInvalidation(['components', 'domains'], () => void handle.retry());
    const [starting, setStarting] = useState<string | null>(null);
    const [outcome, setOutcome] = useState<ApiError | null>(null);
    const [failed, setFailed] = useState<{ key: string; text: string } | null>(null);

    const pct = (n: number) => t('monitoring.pct', { n: Math.round(n) });
    const clock = (iso?: string) => {
        if (!iso) return '';
        const at = new Date(iso);
        if (Number.isNaN(at.getTime())) return '';
        const sameDay = at.toDateString() === new Date().toDateString();
        return at.toLocaleString(locale, sameDay ? { hour: '2-digit', minute: '2-digit' } : { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
    };

    // Start, as on the component's own page: one request, a known outcome
    // stays on screen, an unknown one is said and the state is read again.
    // Nothing is sent twice: the button is busy until the answer is in.
    const start = async (problem: Problem) => {
        const name = componentName({ id: problem.subject ?? problem.unit ?? '', name: problem.name }, t);
        setOutcome(null);
        setFailed(null);
        setStarting(problem.key);
        try {
            let response: Response;
            try {
                response = await fetch('/api/v1/service/action', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: problem.unit, action: 'start' }),
                });
            } catch {
                showToast('warning', t('pnow.unknownResult', { name }));
                return;
            }
            if (!response.ok) {
                const error = await readApiError(response);
                if (isServiceActionOutcome(error)) setOutcome(error);
                else setFailed({ key: problem.key, text: apiErrorText(error, t, 'pnow.startFailed') });
                return;
            }
            let answer: unknown;
            try {
                answer = await response.json();
            } catch {
                // The answer could not be read: what the start came to is not
                // known. The state is read again below; nothing is sent twice.
                showToast('warning', t('pnow.unknownResult', { name }));
                return;
            }
            const note = readServiceActionNote(answer);
            if (note) setOutcome(note);
        } finally {
            setStarting(null);
            // Whatever came of it, every screen that shows components reads
            // again, this list included (it listens for "components").
            invalidate('components');
        }
    };

    const remote = handle.remote;
    if (remote.state === 'loading') {
        return quiet ? null : <p className="mb-4 text-sm text-fg-muted" data-problems-now="checking">{t('pnow.checking')}</p>;
    }
    const report = remote.state === 'known' ? remote.value : remote.previous?.value;
    if (!report) {
        if (quiet) return null;
        return (
            <div className="mb-4 flex flex-wrap items-center gap-3 rounded-xl border border-border bg-surface p-4 text-sm text-fg" data-problems-now="unknown" role="status">
                <AlertTriangle className="h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span className="min-w-0 flex-1">{t('pnow.failed')}</span>
                <Button onClick={() => void handle.retry()} loading={handle.reading}>{t('pnow.retry')}</Button>
            </div>
        );
    }
    const problems = report.problems.filter((p) => !omit.includes(p.kind));
    const unknown = report.unknown.filter((u) => !(omit.includes('service_stopped') && u.source === 'services'));
    if (problems.length === 0 && unknown.length === 0 && !outcome) {
        if (quiet) return null;
        return (
            <p className="mb-4 flex items-center gap-2 text-sm text-fg-muted" data-problems-now="none">
                <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                {t('pnow.none')}
            </p>
        );
    }
    if (problems.length === 0 && quiet && !outcome) return null;

    return (
        <section className="mb-4" aria-labelledby="problems-now-title" data-problems-now={problems.length > 0 ? 'problems' : 'clear'}>
            {problems.length > 0 && (
                <>
                    <h2 id="problems-now-title" className="mb-2 text-sm font-semibold text-fg">{t('pnow.title')}</h2>
                    <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border-strong bg-surface">
                        {problems.map((problem) => {
                            const critical = problem.severity === 'critical';
                            const Icon = critical ? XCircle : AlertTriangle;
                            const vars = {
                                name: componentName({ id: problem.subject ?? '', name: problem.name }, t),
                                domain: problem.subject ?? '',
                                days: Math.max(problem.value, 0),
                                value: problem.kind === 'disk_full' || problem.kind.endsWith('_sustained') ? pct(problem.value) : problem.value,
                            };
                            const when = problem.since
                                ? t('pnow.since', { time: clock(problem.since) })
                                : problem.kind === 'service_stopped' && problem.observed_at ? t('pnow.seen', { time: clock(problem.observed_at) }) : '';
                            const showLink = problem.action.kind === 'open' && problem.action.to !== here;
                            return (
                                <li key={problem.key} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start" data-problem={problem.kind} data-severity={problem.severity}>
                                    <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${critical ? 'text-danger' : 'text-warning'}`} aria-hidden="true" />
                                    <div className="min-w-0 flex-1">
                                        <p className="text-sm font-semibold text-fg">
                                            {t(`pnow.${problem.kind}.title` as TranslationKey, vars)}
                                            {when && <span className="ml-2 text-xs font-normal text-fg-subtle">{when}</span>}
                                        </p>
                                        <p className="mt-1 text-sm leading-6 text-fg-muted">{t(`pnow.${problem.kind}.body` as TranslationKey, vars)}</p>
                                        {failed?.key === problem.key && <p className="mt-1 text-sm text-danger" role="alert">{failed.text}</p>}
                                    </div>
                                    {problem.action.kind === 'start_service' ? (
                                        <Button
                                            variant="primary"
                                            className="shrink-0 self-start"
                                            loading={starting === problem.key}
                                            disabled={starting !== null}
                                            onClick={() => void start(problem)}
                                        >
                                            {t('pnow.service_stopped.action')}
                                        </Button>
                                    ) : showLink ? (
                                        <Link to={problem.action.to as string} className="shrink-0 self-start text-sm font-semibold text-primary max-sm:min-h-11" data-problem-link="">
                                            {t(`pnow.${problem.kind}.action` as TranslationKey)}
                                        </Link>
                                    ) : null}
                                </li>
                            );
                        })}
                    </ul>
                </>
            )}
            <ServiceActionNotice outcome={outcome} onClose={() => setOutcome(null)} className="mt-3" />
            {unknown.length > 0 && !quiet && (
                <p className="mt-2 text-xs text-fg-subtle" data-problems-unknown="">
                    {t('pnow.notChecked', {
                        list: unknown.map((u) => `${t(`pnow.source.${u.source}` as TranslationKey)} (${t(`pnow.reason.${u.reason}` as TranslationKey)})`).join(', '),
                    })}
                </p>
            )}
        </section>
    );
}
