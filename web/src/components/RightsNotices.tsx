import { useState } from 'react';
import { AlertTriangle, Check } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button } from './ui';
import { refusalText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { MIGRATION_NOTICE, POOL_NOTICES, RIGHTS_NOTICES_URL, TRAFFIC_MEASURED_NOTICE, decodeNotices, sendRights, type RightsNotice } from '../lib/rights';
import { UsageNotices, UsageStatusCard } from './UsageNotices';

// What the rights want someone to see (D-042 5 and 9): after the update, the
// customers that had no subscription and are now Unlimited (the
// administrator checks each one and sets its rights); and a reseller's pool
// at 80% and at 100% of real use (the reseller and the administrator). Each
// notice says who acts and what to do, and stays until it is marked as seen.
//
// Hakların birinin görmesini istedikleri: güncellemeden sonra Sınırsız olan
// abonesiz müşteriler ve bayi havuzunun gerçek kullanımda %80 ve %100'ü.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

const KIND_WORDS: Record<string, TranslationKey> = {
    domains: 'rights.qty.domains',
    databases: 'rights.qty.databases',
    'email accounts': 'rights.qty.mailboxes',
    mailboxes: 'rights.qty.mailboxes',
    disk: 'rights.qty.disk',
    customers: 'rights.qty.customers',
};

/** One pool notice in words. */
export function poolNoticeText(notice: RightsNotice, t: Say, forAdministrator: boolean): string {
    const what = KIND_WORDS[notice.subject] ? t(KIND_WORDS[notice.subject]) : notice.subject;
    const full = notice.kind === 'reseller_pool_100';
    const key = forAdministrator
        ? (full ? 'rights.notice.pool100Admin' : 'rights.notice.pool80Admin')
        : (full ? 'rights.notice.pool100' : 'rights.notice.pool80');
    return t(key, { what, detail: notice.detail.replace(' of ', ' / '), name: notice.username });
}

function useAcknowledge() {
    const { t } = useI18n();
    const [busy, setBusy] = useState<number | null>(null);
    const acknowledge = async (ids: number[]) => {
        if (busy !== null || ids.length === 0) return;
        setBusy(ids[0]);
        try {
            for (const id of ids) {
                const result = await sendRights(`${RIGHTS_NOTICES_URL}/${id}/acknowledge`, 'POST');
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
            setBusy(null);
        }
    };
    return { busy, acknowledge };
}

/**
 * The administrator's notices on the Accounts page: accounts that became
 * Unlimited with the update, with a way to set each one's rights, and
 * resellers whose pool is nearly or fully used.
 */
export function AdminRightsNotices({ onSetRights, onOpenAccount }: { onSetRights: (userId: number) => void; onOpenAccount?: (username: string) => void }) {
    const { t } = useI18n();
    const read = useRemote(RIGHTS_NOTICES_URL, decodeNotices);
    const { busy, acknowledge } = useAcknowledge();
    const notices = read.remote.state === 'known' ? read.remote.value : [];
    const migrated = notices.filter((notice) => notice.kind === MIGRATION_NOTICE);
    const pools = notices.filter((notice) => POOL_NOTICES.includes(notice.kind));
    const traffic = notices.filter((notice) => notice.kind === TRAFFIC_MEASURED_NOTICE);
    return (
        <div className="mb-4 grid gap-3">
            {/* alpha.118 (D-044): whether measuring works, and the traffic and disk notices. */}
            <UsageStatusCard />
            <UsageNotices onOpenAccount={onOpenAccount} />
            {traffic.length > 0 && (
                <section className="rounded-lg border border-border bg-surface p-4 text-sm leading-6 text-fg" role="status" data-rights-notice="traffic-measured">
                    <h3 className="text-base font-semibold">{t('usage.measured.title')}</h3>
                    <p className="mt-1 max-w-[75ch]">{t('usage.measured.body')}</p>
                    <p className="mt-1 max-w-[75ch]"><span className="font-semibold">{t('problem.todo')}</span> {t('usage.measured.todo')}</p>
                    <Button type="button" icon={Check} className="mt-2 max-sm:min-h-11" onClick={() => void acknowledge(traffic.map((notice) => notice.id))} disabled={busy !== null}>{t('rights.notice.seen')}</Button>
                </section>
            )}
            {migrated.length > 0 && (
                <section className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg" role="status" data-rights-notice="migration">
                    <h3 className="flex items-start gap-2 text-base font-semibold">
                        <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                        <span>{t(migrated.length === 1 ? 'rights.notice.migratedOne' : 'rights.notice.migratedMany', { n: migrated.length })}</span>
                    </h3>
                    <p className="mt-1 max-w-[75ch]">{t('rights.notice.migratedWhy')}</p>
                    <p className="mt-1 max-w-[75ch]"><span className="font-semibold">{t('problem.todo')}</span> {t('rights.notice.migratedTodo')}</p>
                    <ul className="mt-3 divide-y divide-warning-mark/30">
                        {migrated.map((notice) => (
                            <li key={notice.id} className="flex flex-wrap items-center gap-2 py-2" data-migrated-account={notice.user_id}>
                                <span className="mr-auto font-medium">{notice.username || notice.detail}</span>
                                <Button type="button" variant="primary" onClick={() => onSetRights(notice.user_id)} className="max-sm:min-h-11">{t('rights.notice.setRights')}</Button>
                                <Button type="button" icon={Check} onClick={() => void acknowledge([notice.id])} disabled={busy !== null} className="max-sm:min-h-11">{t('rights.notice.keep')}</Button>
                            </li>
                        ))}
                    </ul>
                    {migrated.length > 1 && (
                        <Button type="button" className="mt-2" onClick={() => void acknowledge(migrated.map((notice) => notice.id))} disabled={busy !== null}>{t('rights.notice.keepAll')}</Button>
                    )}
                </section>
            )}
            {pools.length > 0 && <PoolNoticeList notices={pools} forAdministrator busy={busy} acknowledge={acknowledge} onSetRights={onSetRights} />}
        </div>
    );
}

function PoolNoticeList({ notices, forAdministrator, busy, acknowledge, onSetRights }: {
    notices: RightsNotice[];
    forAdministrator: boolean;
    /** alpha.127: the administrator opens the reseller's rights in place. */
    onSetRights?: (userId: number) => void;
    busy: number | null;
    acknowledge: (ids: number[]) => Promise<void>;
}) {
    const { t } = useI18n();
    return (
        <section className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg" role="status" data-rights-notice="pool">
            <h3 className="flex items-start gap-2 font-semibold">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span>{t(forAdministrator ? 'rights.notice.poolTitleAdmin' : 'rights.notice.poolTitle')}</span>
            </h3>
            <ul className="mt-2 divide-y divide-warning-mark/30">
                {notices.map((notice) => (
                    <li key={notice.id} className="flex flex-wrap items-center gap-2 py-2" data-pool-notice={notice.kind}>
                        <span className="mr-auto max-w-[75ch]">{poolNoticeText(notice, t, forAdministrator)}</span>
                        {forAdministrator && onSetRights && (
                            <Button type="button" variant="primary" onClick={() => onSetRights(notice.user_id)} className="max-sm:min-h-11" data-open-rights={notice.user_id}>
                                {t('adminNext.openRights', { name: notice.username })}
                            </Button>
                        )}
                        <Button type="button" icon={Check} onClick={() => void acknowledge([notice.id])} disabled={busy !== null} className="max-sm:min-h-11">{t('rights.notice.seen')}</Button>
                    </li>
                ))}
            </ul>
            <p className="mt-2 text-xs text-fg-muted">{t(forAdministrator ? 'rights.notice.poolTodoAdmin' : 'rights.notice.poolTodo')}</p>
        </section>
    );
}

/** A reseller's own pool notices, inside My plan. */
export function ResellerPoolNotices() {
    const read = useRemote(RIGHTS_NOTICES_URL, decodeNotices);
    const { busy, acknowledge } = useAcknowledge();
    if (read.remote.state !== 'known') return null;
    const pools = read.remote.value.filter((notice) => POOL_NOTICES.includes(notice.kind));
    if (pools.length === 0) return null;
    return <PoolNoticeList notices={pools} forAdministrator={false} busy={busy} acknowledge={acknowledge} />;
}
