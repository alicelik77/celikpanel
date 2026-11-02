import { useState } from 'react';
import { Archive, Power } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useRefreshEvery, useRemote } from '../lib/remote';
import { DELETABLE_URL, causeKeys, decodeDeletable, type DeletableEntry } from '../lib/suspension';
import { Button, KnownEmpty, RemoteGate } from './ui';
import { SuspensionDialog, type SuspensionTarget } from './SuspensionDialog';

// The "Deletable" list (owner, 2 Nov 2026): a subscription whose services are
// suspended goes straight onto it, with what it still holds. Deleting stays the
// manual flow that already exists (its domains and databases from their own
// screens, then the account); nothing here deletes anything. The only action is
// to resume it, which opens the same dialogue that shows what will start again
// and then how far it is.
//
// "Silinebilir" listesi: hizmetleri askıya alınmış abonelik doğrudan buraya
// düşer ve hâlâ neyi tuttuğu görünür. Silme, mevcut elle akışta kalır; burası
// hiçbir şey silmez. Tek eylem yeniden açmaktır.
const REFRESH_MS = 5000;

export function DeletableAccounts() {
    const { t, locale } = useI18n();
    const handle = useRemote(DELETABLE_URL, decodeDeletable);
    const [target, setTarget] = useState<SuspensionTarget | null>(null);
    // A resumed subscription leaves the list on its own; reading never changes anything.
    useRefreshEvery(handle, REFRESH_MS);
    const known = handle.remote.state === 'known' && !handle.reading;

    return (
        <div data-deletable-list="">
            <p className="mb-3 max-w-prose text-xs text-fg-subtle">{t('susp.del.hint')}</p>
            <RemoteGate
                remote={handle.remote}
                checking={t('susp.del.checking')}
                failed={t('susp.del.unknown')}
                onRetry={() => void handle.retry()}
                busy={handle.reading}
                className="py-3"
            >
                {(shown) => (shown.value.length === 0 ? (
                    <KnownEmpty of={shown} icon={Archive} title={t('susp.del.empty')} hint={t('susp.del.emptyHint')} />
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                    <th className="px-4 py-2.5">{t('susp.del.col.owner')}</th>
                                    <th className="px-4 py-2.5">{t('susp.del.col.cause')}</th>
                                    <th className="px-4 py-2.5">{t('susp.del.col.contents')}</th>
                                    <th className="px-4 py-2.5" />
                                </tr>
                            </thead>
                            <tbody>
                                {shown.value.map((entry) => (
                                    <tr key={entry.subscription_id} className="border-b border-border align-top last:border-0 hover:bg-surface-2/60" data-deletable={entry.subscription_id}>
                                        <td className="px-4 py-3">
                                            <div className="text-base font-medium text-fg">{entry.owner}</div>
                                            <div className="text-xs text-fg-subtle">{entry.subscription_name}</div>
                                            {entry.reseller && <div className="text-xs text-fg-subtle">{t('susp.del.reseller', { name: entry.reseller })}</div>}
                                        </td>
                                        <td className="px-4 py-3 text-fg-muted">
                                            <Cause entry={entry} locale={locale} />
                                        </td>
                                        <td className="px-4 py-3 text-fg-muted">
                                            <Contents entry={entry} />
                                            {entry.delete_hint && <p className="mt-1 max-w-prose text-xs text-fg-subtle">{entry.delete_hint}</p>}
                                        </td>
                                        <td className="px-4 py-3 text-right">
                                            <Button
                                                icon={Power}
                                                disabled={!known}
                                                onClick={() => setTarget({ kind: 'subscription', id: entry.subscription_id, name: entry.subscription_name })}
                                            >
                                                {t('susp.choose.resume')}
                                            </Button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                ))}
            </RemoteGate>
            {target && <SuspensionDialog target={target} onClose={() => setTarget(null)} />}
        </div>
    );
}

function Cause({ entry, locale }: { entry: DeletableEntry; locale: string }) {
    const { t } = useI18n();
    const since = entry.suspended_at ? new Date(entry.suspended_at) : null;
    const sinceText = since && !Number.isNaN(since.getTime()) ? since.toLocaleDateString(locale) : '';
    return (
        <>
            {causeKeys(entry.causes).map((cause) => (
                <p key={cause}>{t(`susp.cause.${cause}` as TranslationKey, { name: entry.causes.reseller ?? '' })}</p>
            ))}
            {entry.reason && <p className="mt-1 break-words">{t('susp.reasonGiven', { reason: entry.reason })}</p>}
            {sinceText && <p className="mt-1 text-xs text-fg-subtle">{t('susp.since', { date: sinceText })}</p>}
        </>
    );
}

// What the subscription still holds: the names of its domains and the counts
// the owner must clear before the account can be deleted.
function Contents({ entry }: { entry: DeletableEntry }) {
    const { t } = useI18n();
    const { contents } = entry;
    return (
        <>
            <p className="break-words">{contents.domains.length > 0 ? t('susp.del.domains', { names: contents.domains.join(', ') }) : t('susp.del.noDomains')}</p>
            <p className="text-xs">
                {t('susp.del.counts', {
                    sites: contents.sites,
                    mailboxes: contents.mailboxes,
                    databases: contents.databases,
                    dbUsers: contents.database_users,
                    vpn: contents.vpn_peers,
                    backups: contents.backup_schedules,
                })}
            </p>
        </>
    );
}
