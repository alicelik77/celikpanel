import { useState } from 'react';
import { AlertTriangle, Check } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button } from './ui';
import { refusalText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { sendRights } from '../lib/rights';
import { USAGE_NOTICES_URL, USAGE_STATUS_URL, decodeUsageNotices, decodeUsageStatus, formatBytes, usageStatusProblems, type UsageNotice } from '../lib/usage';

// Traffic and disk notices (alpha.118, D-044): what happened, what keeps
// working, who acts and how it resumes, before anything else. A notice stays
// until it is marked as seen; it was also e-mailed when the panel's mail is
// set up. The administrator also sees whether measuring itself works.
//
// Trafik ve disk bildirimleri: ne oldu, ne çalışmaya devam ediyor, kim ne
// yapar, nasıl düzelir. Yönetici ölçümün kendisinin çalışıp çalışmadığını da görür.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** One notice in words: the reason first, then the next step. */
export function usageNoticeText(notice: UsageNotice, t: Say): { title: string; body: string } {
    const b = notice.body;
    const vars = {
        what: b.subscription ?? '',
        owner: b.owner ?? '',
        reseller: b.reseller ?? '',
        used: formatBytes(b.used_bytes),
        limit: formatBytes(b.limit_bytes),
        date: b.suspend_on ?? '',
    };
    const forOthers = notice.audience !== 'customer';
    switch (notice.kind) {
        case 'traffic_80':
            return { title: t('usage.notice.traffic80Title', vars), body: t('usage.notice.traffic80', vars) };
        case 'traffic_100':
            return {
                title: t('usage.notice.traffic100Title', vars),
                body: t(b.cause === 'pool' ? 'usage.notice.traffic100Pool' : forOthers ? 'usage.notice.traffic100Others' : 'usage.notice.traffic100', vars),
            };
        case 'pool_traffic_80':
            return { title: t('usage.notice.pool80Title', vars), body: t('usage.notice.pool80', vars) };
        case 'pool_traffic_100':
            return { title: t('usage.notice.pool100Title', vars), body: t('usage.notice.pool100', vars) };
        case 'disk_80':
            return { title: t('usage.notice.disk80Title', vars), body: t('usage.notice.disk80', vars) };
        case 'disk_100':
            return { title: t('usage.notice.disk100Title', vars), body: t('usage.notice.disk100', vars) };
        case 'disk_countdown':
            return { title: t('usage.notice.countdownTitle', vars), body: t('usage.notice.countdown', vars) };
        case 'disk_suspended':
            return { title: t('usage.notice.suspendedTitle', vars), body: t('usage.notice.suspended', vars) };
    }
}

/** The signed-in person's unseen traffic and disk notices. */
export function UsageNotices() {
    const { t } = useI18n();
    const read = useRemote(USAGE_NOTICES_URL, decodeUsageNotices);
    const [busy, setBusy] = useState(false);
    if (read.remote.state !== 'known' || read.remote.value.length === 0) return null;
    const notices = read.remote.value;
    const seen = async (ids: number[]) => {
        if (busy) return;
        setBusy(true);
        try {
            for (const id of ids) {
                const result = await sendRights(`${USAGE_NOTICES_URL}/${id}/seen`, 'POST');
                if (result.kind === 'lost') {
                    showToast('error', t('common.resultUnknown'));
                    break;
                }
                if (result.kind === 'refused') {
                    showToast('error', refusalText(result.error, t, 'rights.notice.ackFailed'));
                    break;
                }
            }
        } finally {
            invalidate('rights');
            setBusy(false);
        }
    };
    return (
        <section className="grid gap-2" data-usage-notices={notices.length} aria-label={t('usage.notice.listTitle')}>
            {notices.map((notice) => {
                const text = usageNoticeText(notice, t);
                const full = notice.kind.endsWith('100') || notice.kind === 'disk_suspended' || notice.kind === 'disk_countdown';
                return (
                    <div
                        key={notice.id}
                        role="status"
                        data-usage-notice={notice.kind}
                        className={`rounded-lg border p-3 text-sm leading-6 text-fg ${full ? 'border-danger/40 bg-danger/10' : 'border-warning-mark/50 bg-warning-mark/15'}`}
                    >
                        <p className="flex items-start gap-2 font-semibold">
                            <AlertTriangle className={`mt-1 h-4 w-4 shrink-0 ${full ? 'text-danger' : 'text-warning'}`} aria-hidden="true" />
                            <span>{text.title}</span>
                        </p>
                        <p className="mt-1 max-w-[75ch]">{text.body}</p>
                        <Button type="button" icon={Check} className="mt-2 max-sm:min-h-11" onClick={() => void seen([notice.id])} disabled={busy}>{t('rights.notice.seen')}</Button>
                    </div>
                );
            })}
            {notices.length > 1 && (
                <Button type="button" onClick={() => void seen(notices.map((notice) => notice.id))} disabled={busy}>{t('usage.notice.seenAll')}</Button>
            )}
        </section>
    );
}

/** The administrator's view of whether measuring works; shown only when something needs a look. */
export function UsageStatusCard() {
    const { t } = useI18n();
    const read = useRemote(USAGE_STATUS_URL, decodeUsageStatus, { freshFor: 60_000 });
    if (read.remote.state !== 'known') return null;
    const status = read.remote.value;
    const problems = usageStatusProblems(status);
    if (problems.length === 0) return null;
    const unknownSites = Object.values(status.traffic.unknown).reduce((sum, n) => sum + n, 0);
    const lines: string[] = problems.map((problem) => {
        switch (problem) {
            case 'traffic_collect':
                return t('usage.status.trafficCollect', { minutes: status.traffic.interval_minutes });
            case 'traffic_unknown':
                return t('usage.status.trafficUnknown', { n: unknownSites });
            case 'logrotate':
                return t(status.traffic.logrotate === 'missing_tool' ? 'usage.status.logrotateMissing' : 'usage.status.logrotateKept');
            case 'disk_unknown':
                return t('usage.status.diskUnknown', { n: status.disk.unknown, hours: status.disk.interval_hours });
            case 'disk_low':
                return t('usage.status.diskLow');
            default:
                return t('usage.status.mailQuota', { detail: status.mail_quota.detail || status.mail_quota.error || status.mail_quota.state });
        }
    });
    return (
        <section className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg" role="status" data-usage-status={problems.join(' ')}>
            <h3 className="flex items-start gap-2 text-base font-semibold">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span>{t('usage.status.title')}</span>
            </h3>
            <ul className="mt-1 list-disc pl-5">
                {lines.map((line) => <li key={line} className="max-w-[75ch]">{line}</li>)}
            </ul>
            <p className="mt-2 text-xs text-fg-muted">{t('usage.status.resumes')}</p>
        </section>
    );
}
