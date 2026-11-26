import { lazy, Suspense, useCallback, useEffect, useState } from 'react';
import { AlertTriangle, Bell, CheckCircle2, XCircle } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { readServiceActionNote } from '../lib/serviceActionNote';
import { componentName } from '../lib/componentName';
import { sizeWord } from './kit/words';
import { invalidate, readRemote, useInvalidation, useRemote } from '../lib/remote';
import { startVisibleReads } from '../lib/visibleReads';
import { Button } from './ui';
import { isServiceActionOutcome, ServiceActionNotice } from './ServiceActionNotice';
import { showToast } from './Toast';
import { confirmAction, followOperation } from '../lib/confirmAction';
import { siteStoppedLines, sitesStoppedTitle, unappliedKey, unappliedVars, withSiteLimitsLane, type SiteStoppedItem } from '../i18n/siteLimitsLane';
import { useDbLimitsWords, type DbLimitsSay } from '../i18n/dbLimits';
import type { DbLimitsWordKey } from '../i18n/dbLimitsWords/en';

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
    | 'security_updates'
    | 'site_down'
    | 'site_certificate'
    | 'service_restarting'
    | 'name_overlap'
    | 'name_server_reserved'
    | 'node_port_reserved'
    | 'tool_account_failed'
    | 'tool_account_owner_config'
    | 'stock_pool_in_use'
    | 'stock_pool_owner_changed'
    | 'site_crontab_outside'
    | 'site_at_jobs'
    | 'at_allow_lists_sites'
    | 'at_directory_accounts'
    | 'at_not_closed'
    | 'tool_down'
    | 'stock_pool_not_put_back'
    | 'stock_pool_failed'
    | 'stock_pool_unknown'
    | 'stock_pool_owner_restored'
    | 'at_owner_removed'
    | 'site_crontab_unreadable'
    | 'postgresql_databases_open'
    | 'mail_signing'
    | 'mail_autoresponder_foreign'
    | 'postgresql_databases_unreadable'
    | 'webmail_address_kept'
    | 'webmail_address_refused'
    | 'webmail_cannot_send'
    | 'service_operation_unsettled'
    | 'site_memory_repeated'
    | 'site_tasks_repeated'
    | 'disk_limits_counted'
    | 'site_programs_outside'
    | 'site_limits_unsupported'
    | 'site_limits_unapplied'
    | 'site_memory_default'
    | 'site_move_kept'
    | 'database_limits_unapplied'
    | 'site_limits_outside'
    | 'cron_allow_lists_sites'
    | 'cron_not_closed'
    | 'cron_owner_removed'
    | 'site_crontab_returned'
    | 'site_crontab_by_site'
    | 'site_tasks_held'
    | 'site_lingering'
    | 'linger_owner_removed'
    | 'linger_not_closed'
    | 'site_user_manager_open';

// S4 R1 second fix round: the administrator's "close it again" runs on the
// panel; the window follows it with reads only.
const RECLOSE_URL = '/api/v1/metrics/side-doors/reclose';
// Third fix round: the panel keeps the last result with its time, also over
// a restart; one a restart cut is "unknown".
type RecloseStatus = { target?: string; version?: string; running?: boolean; closed?: boolean; unknown?: boolean; error_kind?: string; error?: string; at?: string };
const RECLOSE_POLL_MS = 2000;
const RECLOSE_FOLLOW_MS = 25 * 60 * 1000;

const KINDS: readonly ProblemKind[] = [
    'service_stopped', 'disk_full', 'certificate_expired', 'certificate_renewal_failed', 'certificate_expiring',
    'memory_sustained', 'cpu_sustained', 'mail_queue_deferred', 'security_updates',
    // alpha.127: website checks and a service that keeps restarting.
    'site_down', 'site_certificate', 'service_restarting',
    // Security release (20 November 2026): names and Node.js ports held
    // before the one name rule.
    'name_overlap', 'name_server_reserved', 'node_port_reserved',
    // S4 R1: the tools' own accounts and the distribution's PHP-FPM pool.
    'tool_account_failed', 'tool_account_owner_config', 'stock_pool_in_use', 'stock_pool_owner_changed',
    // S4 R1: jobs a site's own code can make outside CelikPanel.
    'site_crontab_outside', 'site_at_jobs', 'at_allow_lists_sites', 'at_directory_accounts', 'at_not_closed',
    // S4 R1 fix round: a tool that does not answer, the stock pool's other
    // states, the server owner's undo and an unreadable crontab.
    'tool_down', 'stock_pool_not_put_back', 'stock_pool_failed', 'stock_pool_unknown', 'stock_pool_owner_restored',
    'at_owner_removed', 'site_crontab_unreadable',
    // 20 Nov 2026 (S1b): PostgreSQL databases every account can enter.
    'postgresql_databases_open',
    // 20 Nov 2026: mail signed only for the sender's own domain.
    'mail_signing',
    // 22 Nov 2026: an out-of-office reply still answering from an address
    // that is no longer the mailbox's own.
    'mail_autoresponder_foreign',
    // 22 Nov 2026: PostgreSQL databases the hourly checks could not read.
    'postgresql_databases_unreadable',
    // alpha.144: webmail's address still in its earlier form (owner's file,
    // or a rewrite nginx's test put back).
    'webmail_address_kept', 'webmail_address_refused',
    // alpha.145 lab, D1: a server change whose end the panel could not record.
    'service_operation_unsettled',
    // S4 R2a: a site that keeps reaching its limits; disk limits counted, not enforced.
    'site_memory_repeated', 'site_tasks_repeated', 'disk_limits_counted',
    // S4 R2a/R2b: sites whose programs or scheduled tasks still run outside
    // their limits (a to-do; its action opens "Move all").
    'site_limits_outside',
    // S4 R2a: this server cannot enforce a site's limits at all.
    'site_limits_unsupported',
    // S4 R2a second fix round: a change of limits that did not reach every
    // site; a default memory limit fixed for the server's earlier memory.
    'site_limits_unapplied', 'site_memory_default',
    // S4 R2a third fix round: a move whose put-back did not complete.
    'site_move_kept',
    // alpha.149: database users the database limits did not reach.
    'database_limits_unapplied',
    // alpha.145: webmail's configuration still sends without encryption.
    'webmail_cannot_send',
    // S4 R2b: cron.allow, a crontab that came back, lingering.
    'cron_allow_lists_sites', 'cron_not_closed', 'cron_owner_removed', 'site_crontab_returned',
    // S4 R2b fix round: a crontab the site's own code can have made; tasks
    // held for want of the site's limits file.
    'site_crontab_by_site', 'site_tasks_held',
    'site_lingering', 'linger_owner_removed', 'linger_not_closed',
    // S4 R2b second fix round: site accounts whose own service manager can
    // start (a move that could not be settled is site_move_kept, alpha.150).
    'site_user_manager_open',
];
// S4 R2b: "Move all" is fetched only when its line is shown.
const MoveAllSites = lazy(() => import('./MoveAllSites').then((module) => ({ default: module.MoveAllSites })));
// alpha.149: Apply again for those users; the panel runs it apart from the page.
const DB_LIMITS_URL = '/api/v1/metrics/database-limits/apply';
// The outcomes the panel tells apart (review of PR #370, finding 7).
const DB_LIMITS_STATES = ['agent', 'prerequisite', 'connection', 'refused', 'failed', 'unknown'];
// The reason codes a line words (finding 15); the journal holds the detail.
const DB_LIMITS_REASONS = ['agent_unreachable', 'server_not_running', 'connection_refused', 'refused_privileged', 'refused_member', 'refused_not_recorded',
    'refused_absent', 'engine_refused', 'result_unknown', 'not_known_yet', 'server_record_unreadable', 'record_unreadable'];

/** The database limits line's title, body and reason (their own words, ../i18n/dbLimits.ts). */
export function dbLimitsLine(problem: { name?: string; reason?: string; value: number; subject?: string }, say: DbLimitsSay): { title: string; body: string; why: string } {
    const state = DB_LIMITS_STATES.includes(problem.name ?? '') ? problem.name : 'failed';
    const reason = DB_LIMITS_REASONS.includes(problem.reason ?? '') ? problem.reason : '';
    return {
        title: problem.reason === 'record_unreadable' ? say('pnow.database_limits_unapplied.title_record')
            : say('pnow.database_limits_unapplied.title', { value: problem.value, domain: problem.subject ?? '' }),
        body: say(`pnow.database_limits_unapplied.${state}` as DbLimitsWordKey),
        why: reason ? say('pnow.database_limits_unapplied.why', { reason: say(`pnow.database_limits_unapplied.reason.${reason}` as DbLimitsWordKey) }) : '',
    };
}
// The doors the administrator may close again after the server owner undid them.
// The action of a site's default memory limit fixed for the server's earlier memory (S4 R2a).
const SITE_MEMORY_DEFAULT_URL = '/api/v1/rights/site-memory-default';
const RECLOSE = ['at', 'stock_pool', 'cron', 'linger'];
const SOURCES = ['services', 'metrics', 'disk', 'certificates', 'mail_queue', 'security_updates', 'sites', 'names'] as const;

/** The words of a problem's title and body; alerts use the same (alpha.127). */
export function problemVars(
    problem: { kind: string; subject?: string; name?: string; value: number },
    t: ReturnType<typeof useI18n>['t'],
): Record<string, string | number> {
    const vars: Record<string, string | number> = {
        name: componentName({ id: problem.subject ?? '', name: problem.name }, t),
        domain: problem.subject ?? '',
        other: problem.name ?? '',
        days: Math.max(problem.value, 0),
        value: problem.kind === 'disk_full' || problem.kind.endsWith('_sustained') ? t('monitoring.pct', { n: Math.round(problem.value) }) : problem.value,
    };
    // alpha.149 (lab O3): a change of limits that did not reach every site names
    // its reason by code, in the page's words; a reason this page has no words for
    // (an earlier release's text) is the general sentence pointing to the journal.
    // Fix round (F9): each site by its own reason.
    if (problem.kind === 'site_limits_unapplied') Object.assign(vars, unappliedVars(problem, t as (key: string, values?: Record<string, string | number>) => string));
    return vars;
}

// 21 Nov 2026: mail signing is worded per state, each with its concrete step
// (cmd/panel/mail_signing_problem.go); a state this screen does not know reads
// as "unknown", never as fine.
// 22 Nov 2026 (B4): a main.cf filter list CelikPanel cannot read entry by
// entry has two states of its own, never the hand-changed port-25 words.
const MAIL_SIGNING_STATES = ['conf_kept', 'port25_kept', 'port25_variable', 'port25_variable_own', 'submit_open', 'unsupported', 'failed', 'unknown'];

/** The catalogue key of a problem's body. */
export function problemBodyKey(problem: { kind: string; name?: string; subject?: string; value?: number; reason?: string }): TranslationKey {
    if (problem.kind === 'mail_signing') {
        return `pnow.mail_signing.${MAIL_SIGNING_STATES.includes(problem.name ?? '') ? problem.name : 'unknown'}` as TranslationKey;
    }
    // 22 Nov 2026 (B3): worded per step; an unknown step reads as the Agent's failure.
    if (problem.kind === 'mail_autoresponder_foreign') {
        const step = ['agent_unreachable', 'agent_build', 'agent_failed'].includes(problem.subject ?? '') ? problem.subject : 'agent_failed';
        return `pnow.mail_autoresponder_foreign.${step}` as TranslationKey;
    }
    // alpha.144 (alpha.143 lab): one database is said in the singular.
    if ((problem.kind === 'postgresql_databases_open' || problem.kind === 'postgresql_databases_unreadable') && problem.value === 1) {
        return `pnow.${problem.kind}.body_one` as TranslationKey;
    }
    // alpha.144: nginx's test named no file.
    // alpha.145 (security): no address: webmail's local server.
    if (problem.kind === 'webmail_address_refused' && !problem.subject) return (problem.name ? 'pnow.webmail_local_server.refused' : 'pnow.webmail_local_server.refused_unnamed') as TranslationKey;
    if (problem.kind === 'webmail_address_kept' && !problem.subject) return ((problem.name ?? '').endsWith('.celikpanel-pending') ? 'pnow.webmail_local_server.kept' : 'pnow.webmail_local_server.kept_no_text') as TranslationKey;
    if (problem.kind === 'webmail_address_refused' && !problem.name) return 'pnow.webmail_address_refused.body_unnamed' as TranslationKey;
    // CelikPanel's text is named only when it lies beside the owner's file.
    if (problem.kind === 'webmail_address_kept' && !(problem.name ?? '').endsWith('.celikpanel-pending')) return 'pnow.webmail_address_kept.body_no_text' as TranslationKey;
    // alpha.145 (review of PR #357, finding 2): CelikPanel's own earlier text
    // that it could not rewrite: the journal and a restart, never a hand edit.
    if (problem.kind === 'webmail_cannot_send' && problem.subject === 'rewrite_failed') return 'pnow.webmail_cannot_send.body_rewrite_failed' as TranslationKey;
    // alpha.145: CelikPanel's text is named only when it lies beside the configuration.
    if (problem.kind === 'webmail_cannot_send' && !(problem.name ?? '').endsWith('.celikpanel-pending')) return 'pnow.webmail_cannot_send.body_no_text' as TranslationKey;
    // alpha.149 (lab D6): a program put back with its earlier file that does not run is named.
    if (problem.kind === 'site_move_kept' && problem.name) return 'pnow.site_move_kept.body_stopped' as TranslationKey;
    // alpha.150: a kept record of a site's scheduled tasks says how they run now, when the agent knows it.
    if (problem.kind === 'site_move_kept' && ['tasks_none', 'tasks_timers', 'tasks_cron'].includes(problem.reason ?? '')) return `pnow.site_move_kept.body_${problem.reason}` as TranslationKey;
    // alpha.149 (lab O3; fix round F9): each site by its own reason; "not known" never "earlier limits"; one site in the singular.
    if (problem.kind === 'site_limits_unapplied') return unappliedKey(problem, 'body') as TranslationKey;
    return `pnow.${problem.kind}.body` as TranslationKey;
}

/** The catalogue key of a problem's title (22 Nov 2026: one signing state signs nothing). */
export function problemTitleKey(problem: { kind: string; name?: string; subject?: string; value?: number }): TranslationKey {
    // alpha.145 (security): no address: webmail's local server.
    if ((problem.kind === 'webmail_address_kept' || problem.kind === 'webmail_address_refused') && !problem.subject) return 'pnow.webmail_local_server.title' as TranslationKey;
    if (problem.kind === 'mail_signing' && problem.name === 'port25_variable_own') return 'pnow.mail_signing.port25_variable_own_title' as TranslationKey;
    // alpha.149 (lab O3; fix round F9).
    if (problem.kind === 'site_limits_unapplied') return unappliedKey(problem, 'title') as TranslationKey;
    // alpha.144 (alpha.143 lab): "1 PostgreSQL databases" was one database.
    if ((problem.kind === 'postgresql_databases_open' || problem.kind === 'postgresql_databases_unreadable') && problem.value === 1) {
        return `pnow.${problem.kind}.title_one` as TranslationKey;
    }
    return `pnow.${problem.kind}.title` as TranslationKey;
}

/** Whether a kind is one this screen has words for. */
export function isProblemKind(kind: unknown): kind is ProblemKind {
    return typeof kind === 'string' && (KINDS as readonly string[]).includes(kind);
}
const REASONS = ['checking', 'unreadable', 'not_scanned', 'stale'] as const;

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
    action: { kind: 'start_service' | 'open' | 'reclose' | 'apply_default' | 'apply_db_limits' | 'move_all'; to?: string };
    /** alpha.149 (database_limits_unapplied): the stored reason as a code. */
    reason?: string;
    /** alpha.135 (disk_full): what deletions left, which can be deleted now. */
    holding_count?: number;
    holding_bytes?: number;
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
        && !!p.action && (p.action.kind === 'open' || p.action.kind === 'move_all' ? typeof p.action.to === 'string' && p.action.to.startsWith('/')
            : p.action.kind === 'reclose' ? RECLOSE.includes(p.action.to ?? '') : p.action.kind === 'apply_default' ? p.kind === 'site_memory_default'
                : p.action.kind === 'apply_db_limits' ? p.kind === 'database_limits_unapplied'
                    : p.action.kind === 'start_service' && typeof p.unit === 'string')
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
    const { t: screenT, locale } = useI18n();
    // alpha.149: the site limits lane's words (route-scoped) before the screen catalogue.
    const t = withSiteLimitsLane(locale, screenT);
    const handle = useRemote(PROBLEMS_URL, decodeProblems);
    const reread = handle.retry;
    useEffect(() => {
        // alpha.125 rule: no read while the tab is hidden; one at once when
        // shown again, never two at once. Reads only.
        // Gizli sekmede okuma yok; görününce bir kez okur. Yalnız okur.
        return startVisibleReads(reread, REFRESH_MS, { setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number, clearInterval: (id) => clearInterval(id) });
    }, [reread]);
    useInvalidation(['components', 'domains'], () => void handle.retry());
    const [starting, setStarting] = useState<string | null>(null);
    const [outcome, setOutcome] = useState<ApiError | null>(null);
    const [failed, setFailed] = useState<{ key: string; text: string } | null>(null);
    // The last close-again's result when it did not close the door (failed,
    // or cut by a panel restart): read once when a line offers it (reads only).
    const [lastReclose, setLastReclose] = useState<RecloseStatus | null>(null);
    const listed = handle.remote.state === 'known' ? handle.remote.value : handle.remote.state === 'unknown' ? handle.remote.previous?.value : undefined;
    const offersReclose = (listed?.problems ?? []).some((p: Problem) => p.action.kind === 'reclose');
    // alpha.149 (finding 18): the database limits' words come with such a line.
    const dbWords = useDbLimitsWords((listed?.problems ?? []).some((p: Problem) => p.kind === 'database_limits_unapplied'));
    const readLastReclose = useCallback(async () => {
        const read = await readRemote(RECLOSE_URL, (raw) => raw as RecloseStatus);
        if (read.state === 'known') setLastReclose(read.value);
    }, []);
    useEffect(() => {
        if (offersReclose) void readLastReclose();
    }, [offersReclose, readLastReclose]);

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

    // S4 R1 fix round: close again what the server owner undid, after the
    // panel's own question; the window shows the work, then the list reads
    // itself again. Nothing is sent twice. Second fix round: the panel runs it
    // apart from this page (202), so leaving the page neither cancels nor
    // half-does it; this window follows it until it ends, and a second press
    // while it runs follows the same run.
    const reclose = async (problem: Problem) => {
        const vars = problemVars(problem, t);
        const target = problem.action.to ?? '';
        const version = target === 'stock_pool' ? problem.name ?? '' : '';
        await confirmAction({
            title: t(`pnow.${problem.kind}.title` as TranslationKey, vars),
            message: t(`pnow.${problem.kind}.confirm` as TranslationKey, vars),
            confirmLabel: t(`pnow.${problem.kind}.action` as TranslationKey),
            progressTitle: t('pnow.reclose.progress'),
            action: async (report) => {
                report.steps([{ id: 'reclose', label: t('pnow.reclose.step'), state: 'running', since: Date.now() }]);
                const unknown = { state: 'unknown' as const, reason: t('pnow.unknownResult', { name: vars.name ?? '' }) };
                try {
                    const response = await fetch(RECLOSE_URL, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ target, version }),
                    });
                    if (!response.ok) {
                        return { state: 'failed', title: t('pnow.reclose.failed'), reason: apiErrorText(await readApiError(response), t, 'pnow.reclose.failed') };
                    }
                    let status: RecloseStatus;
                    try {
                        status = await response.json() as RecloseStatus;
                    } catch {
                        return unknown;
                    }
                    // Follow the run (reads only) until it ends; the answer
                    // stays on the panel if this page is left. A read that
                    // fails is tried again; when the run's end cannot be read
                    // the result is said as not known.
                    const until = Date.now() + RECLOSE_FOLLOW_MS;
                    let lastRead: 'known' | 'unknown' = 'known';
                    while (status?.running && Date.now() < until) {
                        await new Promise((resolve) => setTimeout(resolve, RECLOSE_POLL_MS));
                        const again = await readRemote(RECLOSE_URL, (raw) => raw as RecloseStatus);
                        lastRead = again.state;
                        if (again.state === 'known') status = again.value;
                    }
                    if (lastRead === 'unknown' || status?.running || status?.unknown || status?.target !== target || (status?.version ?? '') !== version) return unknown;
                    if (status.error) return { state: 'failed', title: t('pnow.reclose.failed'), reason: t('pnow.reclose.notRun', { reason: status.error }) };
                    return status.closed ? { state: 'done', message: t('pnow.reclose.done') } : { state: 'blocked', reason: t('pnow.reclose.still') };
                } catch {
                    return unknown;
                } finally {
                    void handle.retry();
                    void readLastReclose();
                }
            },
        });
    };

    // S4 R2a second fix round: a site's default memory limit fixed for the
    // server's earlier memory. The server's preview first (the sites whose
    // programs use more than the new default, as for any lowered limit), the
    // panel's own question with that list, then the apply with the list's
    // token: a list that changed meanwhile is refused and asked again.
    const applyDefault = async (problem: Problem) => {
        const vars = problemVars(problem, t);
        const send = (body: Record<string, unknown>) => fetch(SITE_MEMORY_DEFAULT_URL, {
            method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
        });
        let preview: { sites_stopped?: string[]; sites_stopped_count?: number; stopped_token?: string; sites_stopped_items?: SiteStoppedItem[] } = {};
        try {
            const response = await send({ action: 'preview' });
            if (!response.ok) {
                setFailed({ key: problem.key, text: apiErrorText(await readApiError(response), t, 'pnow.site_memory_default.failed') });
                return;
            }
            preview = ((await response.json()) as { preview?: typeof preview }).preview ?? {};
        } catch {
            showToast('warning', t('pnow.unknownResult', { name: vars.name ?? '' }));
            return;
        }
        const stopped = preview.sites_stopped_count ?? 0;
        await confirmAction({
            title: t('pnow.site_memory_default.title', vars),
            // alpha.149 (lab CHECK 2): each site's line in the page's own words.
            message: [t('pnow.site_memory_default.confirm', vars), stopped > 0 ? sitesStoppedTitle(stopped, t) : '', ...siteStoppedLines(preview, t)].filter(Boolean).join('\n'),
            confirmLabel: t('pnow.site_memory_default.action'),
            progressTitle: t('pnow.site_memory_default.step'),
            action: async (report) => {
                report.steps([{ id: 'apply', label: t('pnow.site_memory_default.step'), state: 'running', since: Date.now() }]);
                try {
                    const response = await send({ action: 'apply', confirm_stopped: preview.stopped_token ?? '' });
                    if (!response.ok) {
                        return { state: 'failed', title: t('pnow.site_memory_default.failed'), reason: apiErrorText(await readApiError(response), t, 'pnow.site_memory_default.failed') };
                    }
                    return { state: 'done', message: t('pnow.site_memory_default.done') };
                } catch {
                    return { state: 'unknown', reason: t('pnow.unknownResult', { name: vars.name ?? '' }) };
                } finally {
                    void handle.retry();
                }
            },
        });
    };

    // alpha.149: Apply again for database users the limits did not reach. The
    // panel runs it apart from this page (202); this window follows it with
    // reads only, and a second press while it runs follows the same run.
    const applyDbLimits = async (problem: Problem) => {
        const say = dbWords.say;
        const title = dbLimitsLine(problem, say).title;
        await followOperation({
            progressTitle: say('pnow.database_limits_unapplied.action'),
            action: async (report) => {
                report.steps([{ id: 'apply', label: say('pnow.database_limits_unapplied.step'), state: 'running', since: Date.now() }]);
                const unknown = { state: 'unknown' as const, reason: say('pnow.database_limits_unapplied.unknown') };
                type Run = { id?: string; running?: boolean; not_applied?: number };
                try {
                    const response = await fetch(DB_LIMITS_URL, { method: 'POST' });
                    if (!response.ok) return { state: 'failed', title, reason: apiErrorText(await readApiError(response), t, 'pnow.reclose.failed') };
                    let run = await response.json() as Run;
                    const started = run.id ?? '';
                    const until = Date.now() + RECLOSE_FOLLOW_MS;
                    while (run.running && Date.now() < until) {
                        await new Promise((resolve) => setTimeout(resolve, RECLOSE_POLL_MS));
                        const again = await readRemote(DB_LIMITS_URL, (raw) => raw as Run);
                        if (again.state === 'known') run = again.value;
                    }
                    // A run this window no longer sees (the panel restarted,
                    // finding 6) is never said to be done: not known yet.
                    if (!started || (run.id ?? '') !== started) return { state: 'unknown', reason: say('pnow.database_limits_unapplied.notKnownYet') };
                    if (run.running) return unknown;
                    return (run.not_applied ?? 0) === 0 ? { state: 'done', message: say('pnow.database_limits_unapplied.done') }
                        : { state: 'failed', title, reason: say('pnow.database_limits_unapplied.still', { n: run.not_applied ?? 0 }) };
                } catch {
                    return unknown;
                } finally {
                    void handle.retry();
                }
            },
        });
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
    // The database limits line waits for its own words (finding 18).
    const problems = report.problems.filter((p) => !omit.includes(p.kind) && (p.kind !== 'database_limits_unapplied' || dbWords.ready));
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
                            const vars = problemVars(problem, t);
                            const dbLine = problem.kind === 'database_limits_unapplied' ? dbLimitsLine(problem, dbWords.say) : null;
                            const when = problem.since
                                ? t('pnow.since', { time: clock(problem.since) })
                                : problem.kind === 'service_stopped' && problem.observed_at ? t('pnow.seen', { time: clock(problem.observed_at) }) : '';
                            // alpha.145: webmail's local-server line (no address) offers no link: Roundcube's page has nothing for it.
                            const localWebmailLine = (problem.kind === 'webmail_address_kept' || problem.kind === 'webmail_address_refused') && !problem.subject;
                            const showLink = problem.action.kind === 'open' && problem.action.to !== here && !localWebmailLine;
                            return (
                                <li key={problem.key} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start" data-problem={problem.kind} data-severity={problem.severity}>
                                    <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${critical ? 'text-danger' : 'text-warning'}`} aria-hidden="true" />
                                    <div className="min-w-0 flex-1">
                                        <p className="text-sm font-semibold text-fg">
                                            {dbLine ? dbLine.title : t(problemTitleKey(problem), vars)}
                                            {when && <span className="ml-2 text-xs font-normal text-fg-subtle">{when}</span>}
                                        </p>
                                        <p className="mt-1 text-sm leading-6 text-fg-muted">{dbLine ? dbLine.body : t(problemBodyKey(problem), vars)}</p>
                                        {dbLine?.why && <p className="mt-1 text-sm leading-6 text-fg" data-problem-db-limits-why="">{dbLine.why}</p>}
                                        {problem.kind === 'disk_full' && (problem.holding_bytes ?? 0) > 0 && (
                                            <p className="mt-1 text-sm leading-6 text-fg" data-problem-holding="">{t('holding.disk', { size: sizeWord(problem.holding_bytes ?? 0, locale) })}</p>
                                        )}
                                        {failed?.key === problem.key && <p className="mt-1 text-sm text-danger" role="alert">{failed.text}</p>}
                                        {problem.action.kind === 'reclose' && lastReclose && !lastReclose.running && (lastReclose.error || lastReclose.unknown)
                                            && lastReclose.target === problem.action.to
                                            && (lastReclose.version ?? '') === (problem.action.to === 'stock_pool' ? problem.name ?? '' : '') && (
                                            <p className="mt-1 text-sm text-fg" data-problem-reclose-last="">
                                                {lastReclose.error
                                                    ? `${t('pnow.reclose.lastFailed', { time: clock(lastReclose.at) })} ${t('pnow.reclose.notRun', { reason: lastReclose.error })}`
                                                    : t('pnow.reclose.unknown', { time: clock(lastReclose.at) })}
                                            </p>
                                        )}
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
                                    ) : problem.action.kind === 'move_all' ? (
                                        <Suspense fallback={null}><MoveAllSites onDone={() => void handle.retry()} /></Suspense>
                                    ) : problem.action.kind === 'apply_default' ? (
                                        <Button className="shrink-0 self-start" onClick={() => void applyDefault(problem)} data-problem-apply-default="">
                                            {t('pnow.site_memory_default.action')}
                                        </Button>
                                    ) : problem.action.kind === 'apply_db_limits' ? (
                                        <Button className="shrink-0 self-start" onClick={() => void applyDbLimits(problem)} data-problem-apply-db-limits="">
                                            {dbWords.say('pnow.database_limits_unapplied.action')}
                                        </Button>
                                    ) : problem.action.kind === 'reclose' ? (
                                        <Button className="shrink-0 self-start" onClick={() => void reclose(problem)} data-problem-reclose="">
                                            {t(`pnow.${problem.kind}.action` as TranslationKey)}
                                        </Button>
                                    ) : showLink ? (
                                        <Link to={problem.action.to as string} className="shrink-0 self-start text-sm font-semibold text-primary max-sm:min-h-11" data-problem-link="">
                                            {problem.kind === 'disk_full' && (problem.holding_bytes ?? 0) > 0 ? t('holding.diskAction') : t(`pnow.${problem.kind}.action` as TranslationKey)}
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

// alpha.127: one quiet line on the Dashboard while open alerts are unseen
// (Monitoring lists them). It lives here, beside the list the Dashboard
// already loads, so the Dashboard does not pull in the Monitoring page.
// Gösterge Paneli'nde görülmemiş açık uyarı varken tek sakin satır.
export const ALERTS_URL = '/api/v1/metrics/alerts';

export function decodeUnseenOpenAlerts(raw: unknown): number {
    const body = raw as { alerts?: unknown } | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.alerts)) throw new Error('shape');
    return body.alerts.filter((a) => !!a && typeof a === 'object'
        && (a as { state?: unknown }).state === 'open' && (a as { seen?: unknown }).seen === false
        && isProblemKind((a as { kind?: unknown }).kind)).length;
}

export function AlertsNotice() {
    const { t } = useI18n();
    const handle = useRemote(ALERTS_URL, decodeUnseenOpenAlerts);
    const reread = handle.retry;
    useEffect(() => startVisibleReads(reread, REFRESH_MS, { setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number, clearInterval: (id) => clearInterval(id) }), [reread]);
    const remote = handle.remote;
    const open = remote.state === 'known' ? remote.value : 0;
    if (open === 0) return null;
    return (
        <p className="mb-4 flex flex-wrap items-center gap-2 rounded-xl border border-border-strong bg-surface p-3 text-sm text-fg" data-alerts-notice={open}>
            <Bell className="h-4 w-4 text-warning" aria-hidden="true" />
            <span className="font-semibold">{t('pnow.alertsNew', { n: open })}</span>
            <Link to="/monitoring" className="ml-auto font-semibold text-primary max-sm:min-h-11">{t('pnow.cpu_sustained.action')}</Link>
        </p>
    );
}
