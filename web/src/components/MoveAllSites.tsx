import { useState } from 'react';
import { ArrowRightLeft } from 'lucide-react';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd, type OperationStep } from '../lib/confirmAction';
import { newMoveRequestID } from '../lib/siteLimits';
import { readOutsideSites, siteMoved, type OutsideSite } from '../lib/siteTasks';
import { Button } from './ui';

// "Move all" (S4 R2b): the sites whose programs or scheduled tasks still run
// outside their limits, moved one after another with each site's own move
// (its tasks, then its running programs). The panel's own window shows a
// step per site; the work stops at the first site that does not move and
// says which and why; the sites after it are not touched. Pressing again
// later reads the list anew, so a site already moved is not in it.
//
// "Hepsini taşı": sınırlarının dışındaki siteler sırayla, her biri kendi
// taşımasıyla taşınır; taşınamayan ilk sitede durulur ve nedeni söylenir.

export function MoveAllSites({ onDone }: { onDone: () => void }) {
    const { t } = useI18n();
    const [working, setWorking] = useState(false);

    const run = async () => {
        setWorking(true);
        try {
            let sites: OutsideSite[] = [];
            let unread = false;
            try {
                sites = await readOutsideSites();
            } catch {
                unread = true;
            }
            const step = (site: OutsideSite, state: OperationStep['state'], note?: string): OperationStep => ({
                id: String(site.domain_id), label: site.domain, state, note, since: Date.now(),
            });
            await confirmAction({
                title: sites.length > 0 ? t('moveall.confirm', { n: sites.length }) : t('moveall.title'),
                message: unread ? t('moveall.unread') : sites.length > 0 ? t('moveall.body') : t('moveall.none'),
                confirmLabel: t('moveall.go'),
                cancelLabel: t('moveall.cancel'),
                progressTitle: t('moveall.title'),
                action: async (report): Promise<OperationEnd> => {
                    const steps = sites.map((site) => step(site, 'pending'));
                    report.steps(steps);
                    for (let i = 0; i < sites.length; i++) {
                        const site = sites[i];
                        steps[i] = step(site, 'running');
                        report.steps([...steps]);
                        let sent: Response;
                        try {
                            sent = await fetch(`/api/v1/domains/${site.domain_id}/site-limits/move`, {
                                method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
                                body: JSON.stringify({ request_id: newMoveRequestID() }),
                            });
                        } catch {
                            steps[i] = step(site, 'failed');
                            report.steps([...steps]);
                            return { state: 'unknown', reason: t('moveall.unknown', { domain: site.domain }) };
                        }
                        let reason = '';
                        let ok = false;
                        if (sent.ok) {
                            try {
                                ({ ok, reason } = siteMoved(await sent.json()));
                            } catch {
                                steps[i] = step(site, 'failed');
                                report.steps([...steps]);
                                return { state: 'unknown', reason: t('moveall.unknown', { domain: site.domain }) };
                            }
                        } else {
                            reason = apiErrorText(await readApiError(sent), t);
                        }
                        if (!ok) {
                            steps[i] = step(site, 'failed', reason);
                            report.steps([...steps]);
                            return { state: 'failed', reason: t('moveall.failed', { domain: site.domain, reason: reason || t('slim.move.unknown') }) };
                        }
                        steps[i] = step(site, 'done');
                        report.steps([...steps]);
                    }
                    return { state: 'done', message: sites.length > 0 ? t('moveall.done', { n: sites.length }) : t('moveall.none') };
                },
            });
        } finally {
            setWorking(false);
            onDone();
        }
    };

    return (
        <Button variant="primary" icon={ArrowRightLeft} className="shrink-0 self-start" loading={working} disabled={working} onClick={() => void run()} data-problem-move-all="">
            {t('moveall.button')}
        </Button>
    );
}
