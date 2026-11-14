// Server → Maintenance → What deletions left (alpha.135; owner, 14 Nov 2026:
// "30 gün sakla, sonra sil"; "it must not bloat the server; the
// administrator sees it in one place and deletes earlier if they want").
// The stored mail of deleted domains and mailboxes and the backups of deleted
// domains on this server are kept 30 days, then deleted by themselves
// (cmd/panel/deletion_holding.go). One list, always open: the total at its
// top, each item with its size and date, "Delete now…" and, for backups,
// "Download". Every deletion asks in the panel's own window and shows its
// steps; the list reads itself again after it. Folders and the last failure's
// details are under a closed "Details".
//
// Bakım → Silmelerden kalanlar: 30 gün saklanır, sonra kendiliğinden silinir;
// yönetici burada görür, indirir ya da daha önce siler.
import { useState } from 'react';
import { Archive, Download, Trash2 } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, RemoteGate } from './ui';
import { readRemote, useRemote } from '../lib/remote';
import { confirmAction, type OperationEnd, type OperationStep } from '../lib/confirmAction';
import { readApiError, refusalText } from '../lib/apiError';
import { formatDate } from '../lib/dateFormat';
import { sizeWord } from './kit/words';
import { RowActions, type RowAction } from './kit/rows/RowActions';
import {
    DELETION_HOLDING_URL, decodeDeletionHolding, decodeHeldBackupFiles, holdingItemWhat, type HeldItem, type DeletionHolding,
} from '../lib/deletionHolding';

export function DeletionHoldingCard() {
    const { t } = useI18n();
    const handle = useRemote(DELETION_HOLDING_URL, decodeDeletionHolding);
    return (
        <section id="deletion-holding" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-maintenance-section="deletion-holding">
            <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                <Archive className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {t('holding.title')}
            </h2>
            <p className="mt-0.5 text-sm text-fg-muted">{t('holding.intro')}</p>
            <div className="mt-4">
                <RemoteGate remote={handle.remote} checking={t('holding.checking')} failed={t('holding.unread')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <HoldingList value={value} reread={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function HoldingList({ value, reread }: { value: DeletionHolding; reread: () => void }) {
    const { t, locale } = useI18n();
    const size = (bytes: number) => (bytes >= 0 ? sizeWord(bytes, locale) : t('holding.sizeUnknown'));
    const date = (iso: string) => formatDate(iso, locale);
    const what = (item: HeldItem) => holdingItemWhat(item, t);

    // Deletes the items one after the other, each its own step; the list
    // reads itself again at the end.
    const removeNow = async (items: HeldItem[]) => {
        const all = items.length > 1;
        await confirmAction({
            title: all ? t('holding.confirm.allTitle', { n: items.length }) : t('holding.confirm.title', { what: what(items[0]) }),
            message: t('holding.confirm.message'),
            lost: items.map((item) => t('holding.confirm.line', { what: what(item), size: size(item.bytes) })),
            confirmLabel: t('common.delete'),
            tone: 'danger',
            progressTitle: all ? t('holding.progressAll') : t('holding.progress', { what: what(items[0]) }),
            action: async (report): Promise<OperationEnd> => {
                const steps: OperationStep[] = items.map((item) => ({ id: item.id, label: what(item), state: 'pending' }));
                const show = (index: number, step: Partial<OperationStep>) => {
                    steps[index] = { ...steps[index], ...step };
                    report.steps([...steps]);
                };
                let unfinished = 0;
                let lastReason = '';
                for (const [index, item] of items.entries()) {
                    show(index, { state: 'running', since: Date.now() });
                    try {
                        const res = await fetch(`${DELETION_HOLDING_URL}/${encodeURIComponent(item.id)}`, { method: 'DELETE' });
                        if (res.ok) {
                            show(index, { state: 'done' });
                            continue;
                        }
                        unfinished++;
                        lastReason = refusalText(await readApiError(res), t, 'holding.removeFailed');
                        show(index, { state: 'failed', note: lastReason });
                    } catch {
                        // No answer: whether it was deleted is not known; the list is read again.
                        unfinished++;
                        lastReason = t('common.resultUnknown');
                        show(index, { state: 'failed', note: lastReason });
                    }
                }
                reread();
                if (unfinished === 0) return { state: 'done', title: all ? t('holding.doneAll') : t('holding.done', { what: what(items[0]) }) };
                return { state: 'failed', reason: all ? t('holding.unfinished', { n: unfinished }) : lastReason };
            },
        });
    };

    if (value.items.length === 0) {
        return <p className="text-sm text-fg-muted" data-holding-empty="">{t('holding.empty')}</p>;
    }
    return (
        <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm font-semibold text-fg" data-holding-summary="">
                    {t('holding.summary', { n: value.summary.count, size: sizeWord(value.summary.bytes, locale), date: date(value.summary.earliest) })}
                </p>
                <Button variant="danger" onClick={() => void removeNow(value.items)} className="max-sm:min-h-11" data-holding-remove-all="">
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    {t('holding.removeAll')}
                </Button>
            </div>
            <ul className="divide-y divide-border rounded-lg border border-border" data-holding-list="">
                {value.items.map((item) => <HoldingRow key={item.id} item={item} what={what(item)} size={size} date={date} removeNow={() => void removeNow([item])} />)}
            </ul>
        </div>
    );
}

function HoldingRow({ item, what, size, date, removeNow }: {
    item: HeldItem; what: string; size: (bytes: number) => string; date: (iso: string) => string; removeNow: () => void;
}) {
    const { t } = useI18n();
    const [files, setFiles] = useState<{ state: 'closed' | 'reading' | 'unread' } | { state: 'read'; names: { name: string; size: number }[] }>({ state: 'closed' });
    const toggleFiles = async () => {
        if (files.state !== 'closed') {
            setFiles({ state: 'closed' });
            return;
        }
        setFiles({ state: 'reading' });
        const read = await readRemote(`${DELETION_HOLDING_URL}/${encodeURIComponent(item.id)}/backups`, decodeHeldBackupFiles);
        setFiles(read.state === 'known' ? { state: 'read', names: read.value } : { state: 'unread' });
    };
    const actions: [RowAction] | [RowAction, RowAction] = item.kind === 'backups'
        ? [{ id: 'download', label: t('holding.download'), icon: Download, onSelect: () => void toggleFiles() },
            { id: 'remove', label: t('holding.removeNow'), icon: Trash2, onSelect: removeNow }]
        : [{ id: 'remove', label: t('holding.removeNow'), icon: Trash2, onSelect: removeNow }];
    const state = item.state === 'waiting' ? t('holding.state.waiting')
        : item.state === 'failed' ? t('holding.state.failed', { date: date(item.nextTryAt || item.removeAfter) })
            : item.state === 'unknown' ? t('holding.state.unknown', { date: date(item.nextTryAt || item.removeAfter) })
                : '';
    return (
        <li className="flex flex-col gap-2 p-3 text-sm sm:flex-row sm:items-start" data-holding-item={item.id} data-holding-state={item.state}>
            <div className="min-w-0 flex-1">
                <p className="font-medium text-fg [overflow-wrap:anywhere]">{what}</p>
                <p className="mt-0.5 text-fg-muted">{t('holding.row', { size: size(item.bytes), date: date(item.removeAfter) })}</p>
                {state && <p className={`mt-1 ${item.state === 'failed' ? 'text-danger' : 'text-fg-muted'}`} role={item.state === 'failed' ? 'alert' : undefined}>{state}</p>}
                {files.state === 'reading' && <p className="mt-1 text-fg-muted">{t('holding.files.reading')}</p>}
                {files.state === 'unread' && <p className="mt-1 text-fg-muted">{t('holding.files.unread')}</p>}
                {files.state === 'read' && (files.names.length === 0
                    ? <p className="mt-1 text-fg-muted">{t('holding.files.none')}</p>
                    : <ul className="mt-1 space-y-1" data-holding-files="">
                        {files.names.map((file) => (
                            <li key={file.name}>
                                <a className="font-mono text-xs text-primary underline [overflow-wrap:anywhere]" href={`${DELETION_HOLDING_URL}/${encodeURIComponent(item.id)}/download?name=${encodeURIComponent(file.name)}`} download>
                                    {file.name}
                                </a>
                                <span className="ml-2 text-xs text-fg-subtle">{size(file.size)}</span>
                            </li>
                        ))}
                    </ul>)}
                <details className="mt-1 text-fg-muted">
                    <summary className="cursor-pointer text-xs font-semibold">{t('holding.details')}</summary>
                    <ul className="mt-1 space-y-0.5 text-xs">
                        {item.path && <li className="font-mono [overflow-wrap:anywhere]">{item.path}</li>}
                        <li>{t('holding.deletedAt', { date: date(item.deletedAt) })}</li>
                        {item.adopted && <li>{t('holding.adopted', { date: date(item.deletedAt) })}</li>}
                        {item.failureLine && <li className="[overflow-wrap:anywhere]">{t('holding.failure', { line: item.failureLine })}</li>}
                    </ul>
                </details>
            </div>
            <RowActions name={what} actions={actions} />
        </li>
    );
}
