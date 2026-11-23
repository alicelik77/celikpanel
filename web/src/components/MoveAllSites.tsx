import { useState } from 'react';
import { ArrowRightLeft } from 'lucide-react';
import { useI18n } from '../i18n';
import { Link } from '../router';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd, type OperationStep } from '../lib/confirmAction';
import { newMoveRequestID } from '../lib/siteLimits';
import { lineList, movedTaskCount, moveSitesInTurn, readOutsideSites, readTasksPreview, type OutsideSite, type TasksAnswer } from '../lib/siteTasks';
import { Button } from './ui';

// "Move all" (S4 R2b, fix round): the sites whose programs or scheduled tasks
// still run outside their limits, moved one after another with each site's
// own move (its tasks, then its running programs). Before anything runs, the
// panel's own window lists per site what will move and what will stay and
// why (each site's preview is read first). A site that cannot move is
// skipped with its reason and the run goes on; it stops only when a move
// that started fails (or its result is unknown). A site whose tasks must
// stay in cron but whose programs could move is skipped unless the
// administrator ticks the one choice for such sites. At the end: how many
// moved, and which did not and why, each with a link to its page.
//
// "Hepsini taşı": taşınamayan site nedeniyle atlanır, iş yalnız başlamış bir
// taşıma başarısız olursa durur; sonunda taşınmayanlar bağlantılarıyla listelenir.

interface Planned {
    site: OutsideSite;
    /** What the site's move would do (programs, tasks). */
    what: string;
    /** Why its tasks stay in cron ('' when they move or have nothing to move). */
    stay: string;
}

interface NotMoved {
    domain: string;
    reason: string;
}

export function MoveAllSites({ onDone }: { onDone: () => void }) {
    const { t } = useI18n();
    const [working, setWorking] = useState(false);
    const [notMoved, setNotMoved] = useState<NotMoved[]>([]);

    // One site's plan from its preview (read-only).
    const plan = async (site: OutsideSite): Promise<Planned> => {
        const parts: string[] = [];
        let stay = '';
        if (site.tasks) {
            let preview: TasksAnswer | null = null;
            try {
                preview = await readTasksPreview(site.domain_id);
            } catch {
                stay = t('moveall.previewUnknown');
            }
            if (preview) {
                if (preview.outcome === 'preview') {
                    const n = movedTaskCount(preview);
                    if (n > 0) parts.push(t('moveall.tasks', { n }));
                } else if (preview.outcome === 'stayed') {
                    stay = lineList(preview.blocking);
                } else if (preview.outcome !== 'already') {
                    stay = preview.error || preview.outcome;
                }
            }
        }
        if ((site.programs ?? 0) > 0) parts.push(t('moveall.programs', { n: site.programs ?? 0 }));
        if (parts.length === 0) parts.push(t('moveall.tasks', { n: 0 }));
        return { site, what: parts.join('; '), stay };
    };

    const run = async () => {
        setWorking(true);
        setNotMoved([]);
        try {
            let sites: OutsideSite[] = [];
            let unread = false;
            try {
                sites = await readOutsideSites();
            } catch {
                unread = true;
            }
            const plans: Planned[] = [];
            for (const site of sites) plans.push(await plan(site));
            // Moves fully; programs only (tasks stay) on the administrator's
            // tick; nothing to move (skipped with why).
            const full = plans.filter((p) => !p.stay);
            const programsOnly = plans.filter((p) => p.stay && (p.site.programs ?? 0) > 0);
            const skipped = plans.filter((p) => p.stay && !((p.site.programs ?? 0) > 0));
            const skippedRows = [
                ...programsOnly.map((p) => t('moveall.tasksStay', { domain: p.site.domain, reason: p.stay, n: p.site.programs ?? 0 })),
                ...skipped.map((p) => t('moveall.skip', { domain: p.site.domain, reason: p.stay })),
            ];
            const step = (p: Planned, state: OperationStep['state'], note?: string): OperationStep => ({
                id: String(p.site.domain_id), label: p.site.domain, state, note, since: Date.now(),
            });
            const left: NotMoved[] = [];
            await confirmAction({
                title: plans.length > 0 ? t('moveall.confirm', { n: plans.length }) : t('moveall.title'),
                message: unread ? t('moveall.unread') : plans.length > 0 ? t('moveall.body') : t('moveall.none'),
                kept: full.map((p) => `${p.site.domain}: ${p.what}`),
                lost: skippedRows,
                option: programsOnly.length > 0 ? {
                    label: t('moveall.option', { n: programsOnly.length }),
                    kept: programsOnly.map((p) => t('moveall.optionRow', { domain: p.site.domain })),
                } : undefined,
                confirmLabel: t('moveall.go'),
                cancelLabel: t('moveall.cancel'),
                progressTitle: t('moveall.title'),
                action: async (report, answer): Promise<OperationEnd> => {
                    for (const p of skipped) left.push({ domain: p.site.domain, reason: p.stay });
                    const chosen = [...full, ...(answer?.option ? programsOnly : [])];
                    if (!answer?.option) for (const p of programsOnly) left.push({ domain: p.site.domain, reason: p.stay });
                    const steps = chosen.map((p) => step(p, 'pending'));
                    report.steps(steps);
                    const outcome = await moveSitesInTurn(chosen.map((p) => p.site), async (site) => {
                        try {
                            const sent = await fetch(`/api/v1/domains/${site.domain_id}/site-limits/move`, {
                                method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
                                body: JSON.stringify({ request_id: newMoveRequestID() }),
                            });
                            if (!sent.ok) return { status: sent.status, refusal: apiErrorText(await readApiError(sent), t) };
                            let body: unknown;
                            try {
                                body = await sent.json();
                            } catch {
                                // An answer without a readable body: the result is unknown (the run stops).
                                return { status: sent.status };
                            }
                            return { status: sent.status, body };
                        } catch {
                            return null;
                        }
                    }, (i, state, note) => {
                        steps[i] = step(chosen[i], state, note);
                        report.steps([...steps]);
                    });
                    for (const l of outcome.notMoved) left.push({ domain: l.domain, reason: l.partial ? t('moveall.partial', { reason: l.reason }) : l.reason });
                    if (outcome.stop) {
                        const reason = outcome.stop.unknown ? t('moveall.unknown', { domain: outcome.stop.domain })
                            : outcome.stop.unresolved ? t('moveall.unresolved') : outcome.stop.reason || t('slim.move.unknown');
                        setNotMoved([...left, { domain: outcome.stop.domain, reason }]);
                        return outcome.stop.unknown ? { state: 'unknown', reason }
                            : { state: 'failed', reason: t('moveall.failed', { domain: outcome.stop.domain, reason }) };
                    }
                    setNotMoved(left);
                    return {
                        state: 'done',
                        message: plans.length > 0 ? t('moveall.done', { n: outcome.moved }) : t('moveall.none'),
                        kept: left.length > 0 ? { title: t('moveall.notMoved'), rows: left.map((l) => `${l.domain}: ${l.reason}`) } : undefined,
                    };
                },
            });
        } finally {
            setWorking(false);
            onDone();
        }
    };

    return (
        <div className="shrink-0 self-start" data-problem-move-all="">
            <Button variant="primary" icon={ArrowRightLeft} loading={working} disabled={working} onClick={() => void run()}>
                {t('moveall.button')}
            </Button>
            {notMoved.length > 0 && (
                <div className="mt-2 max-w-sm text-sm text-fg-muted" data-move-all-not-moved="">
                    <p className="font-semibold text-fg">{t('moveall.notMoved')}</p>
                    <ul className="mt-1 space-y-1">
                        {notMoved.map((l) => (
                            <li key={l.domain}>
                                <Link to={`/domains/${encodeURIComponent(l.domain)}`} className="font-semibold text-primary max-sm:min-h-11">{l.domain}</Link>: {l.reason}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
        </div>
    );
}
