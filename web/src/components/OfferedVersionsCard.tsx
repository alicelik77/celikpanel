// "Offered to customers" (v0.1.0-alpha.108, D-038 §14): on a runtime's own
// Manage page the administrator chooses, per version line, whether customers
// may pick it for their sites. Hiding a version changes no site: a site
// already on it keeps it and its owner's page says so. An administrator can
// always choose any installed version.
// alpha.132: ticking a line no longer saves it. The changed lines are marked
// and saved together with the save bar, one audited request per line; the
// card then reads what the server holds. A line the server did not take is
// named with its reason and stays a marked change.
// "Müşterilere sunulan": yönetici, müşterilerin seçebileceği sürümleri seçer;
// değişiklik yalnız Kaydet ile kaydedilir, kaydedilmeyen satır işaretli kalır.
import { useMemo, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { CouldNotCheck, Checking } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, lastKnown, useRemote } from '../lib/remote';
import { answerWasLost } from '../lib/requestIdentity';
import { useSupportTable } from '../lib/supportTable';
import { SupportBadge } from './SupportMark';
import { decodeOfferedVersions, offeredLine, OFFERED_VERSIONS_URL, type OfferedComponent } from '../lib/offeredVersions';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

const NAMES: Record<OfferedComponent, string> = { php: 'PHP', node: 'Node.js' };

/** The lines to show: every installed one, and any hidden one no longer installed. */
export function offeredLines(component: OfferedComponent, installed: string[], hidden: string[]): { line: string; versions: string[] }[] {
    const byLine = new Map<string, string[]>();
    for (const version of installed) {
        const line = offeredLine(component, version);
        if (line) byLine.set(line, [...(byLine.get(line) ?? []), version]);
    }
    for (const line of hidden) if (!byLine.has(line)) byLine.set(line, []);
    const numeric = (line: string) => line.split('.').map((part) => Number(part));
    return [...byLine.entries()]
        .map(([line, versions]) => ({ line, versions }))
        .sort((a, b) => {
            const [x, y] = [numeric(a.line), numeric(b.line)];
            return (y[0] - x[0]) || ((y[1] ?? 0) - (x[1] ?? 0));
        });
}

export function OfferedVersionsCard({ component, installed }: { component: OfferedComponent; installed: string[] }) {
    const { t } = useI18n();
    const say = useSaveWords();
    const handle = useRemote(OFFERED_VERSIONS_URL, decodeOfferedVersions);
    const support = useSupportTable();
    // What the last save could not do, line by line, and what it did.
    const [report, setReport] = useState<{ saved: string[]; failed: string[] } | null>(null);
    const name = NAMES[component];
    const remote = handle.remote;
    const answer = lastKnown(remote)?.value;
    const hidden = answer ? answer.hidden[component] : [];
    const lines = offeredLines(component, installed, hidden);
    const linesKey = lines.map(({ line }) => line).join(' ');
    // The draft: line → offered, over the server's last answer.
    const known = useMemo(
        () => (answer ? Object.fromEntries(linesKey.split(' ').filter(Boolean).map((line) => [line, !answer.hidden[component].includes(line)])) : null),
        [answer, linesKey, component],
    );
    const draft = useDraft<Record<string, boolean>>(known);
    if (lines.length === 0) return null;
    const lineName = (line: string) => `${name} ${line}`;

    const save = () => void draft.save(async (changes) => {
        setReport(null);
        const saved: string[] = [];
        const failed: string[] = [];
        let lost = false;
        // One request per line, one after another: each is audited by itself.
        for (const [line, offered] of Object.entries(changes) as [string, boolean][]) {
            let res: Response | null;
            try {
                res = await fetch(OFFERED_VERSIONS_URL, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ component, version: line, offered }),
                });
            } catch {
                res = null;
            }
            if (!res || answerWasLost(res)) {
                // No answer: the list read below says what the server holds;
                // the change is not sent a second time.
                lost = true;
                failed.push(t('life.offer.notSaved', { what: lineName(line), reason: t('common.resultUnknown') }));
            } else if (!res.ok) {
                failed.push(t('life.offer.notSaved', { what: lineName(line), reason: apiErrorText(await readApiError(res), t, 'life.offer.failed') }));
            } else saved.push(lineName(line));
        }
        // The version pickers and the plans' and accounts' rights screens follow.
        invalidate('components', 'rights');
        await handle.retry();
        if (failed.length === 0) return 'saved';
        setReport({ saved, failed });
        return lost ? 'unknown' : 'refused';
    });

    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-offered-versions={component}>
            <h3 className="text-sm font-semibold text-fg">{t('life.offer.title')}</h3>
            <p className="mt-1 max-w-[75ch] text-sm leading-6 text-fg-muted">{t('life.offer.intro', { name })}</p>
            {remote.state === 'loading' && <Checking className="mt-3" label={t('life.offer.checking')} />}
            {remote.state === 'unknown' && (
                <CouldNotCheck className="mt-3" text={t('life.offer.unknown')} onRetry={() => void handle.retry()} busy={handle.reading} />
            )}
            {answer && (
                <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
                    {lines.map(({ line, versions }) => {
                        const offered = draft.values?.[line] ?? !hidden.includes(line);
                        const changed = draft.isChanged(line);
                        // The word on the right is the server's state; the mark says what Save changes.
                        const live = draft.base?.[line] ?? !hidden.includes(line);
                        const label = versions.length > 1 ? versions.join(', ') : versions[0] ?? line;
                        return (
                            <li key={line} className={`px-3 py-2 ${changed ? changedRowClass : ''}`} data-offered-line={line} data-offered={offered ? 'yes' : 'no'}>
                                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                                    <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 max-sm:min-h-11">
                                        <input
                                            type="checkbox"
                                            checked={offered}
                                            disabled={draft.saving || remote.state !== 'known'}
                                            onChange={(event) => draft.set(line, event.target.checked)}
                                            className="h-4 w-4 shrink-0 accent-primary"
                                            aria-label={t('life.offer.toggle', { name, version: line })}
                                        />
                                        <span className="min-w-0 text-sm text-fg">
                                            <span className="font-medium">{name} {label}</span>
                                            {versions.length === 0 && <span className="text-fg-muted"> {t('life.offer.notInstalled')}</span>}
                                            {changed && <ChangedMark switched={offered} />}
                                        </span>
                                    </label>
                                    <SupportBadge table={support} component={component} version={line} />
                                    <span className={`text-xs ${live ? 'text-fg-muted' : 'font-semibold text-warning'}`} data-offered-now={live ? 'yes' : 'no'}>
                                        {t((live ? 'life.offer.offered' : 'life.offer.hidden') as TranslationKey)}
                                    </span>
                                </div>
                                {draft.isMoved(line) && draft.base && (
                                    <ServerMovedNote value={t(draft.base[line] ? 'life.offer.offered' : 'life.offer.hidden')} onTake={() => draft.take(line)} />
                                )}
                            </li>
                        );
                    })}
                </ul>
            )}
            {report && (
                <div className="mt-3 rounded-lg border border-danger/40 bg-danger/5 px-3 py-2 text-sm leading-6 text-fg" role="alert" data-offered-failure="">
                    {report.saved.length > 0 && <p>{t('life.offer.savedSome', { what: report.saved.join(', ') })}</p>}
                    {report.failed.map((line) => <p key={line}>{line}</p>)}
                </div>
            )}
            <SaveBar
                className="-mx-5 px-5 pb-0"
                count={draft.count}
                saving={draft.saving}
                blockedBy={remote.state !== 'known' ? say('stale') : undefined}
                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: handle.reading, onCheck: () => void handle.retry() } : undefined}
                onSave={save}
                onDiscard={() => { setReport(null); draft.reset(); }}
            />
        </section>
    );
}

/** The note under a site's version picker when its version is hidden from customers. */
export function OfferedVersionNote({ component, version, offered, restricted }: {
    component: OfferedComponent;
    version: string;
    offered: boolean;
    restricted: boolean;
}) {
    const { t } = useI18n();
    if (!version || offered) return null;
    return (
        <p className="mt-1.5 rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-sm leading-6 text-fg" data-offered-note={restricted ? 'customer' : 'admin'}>
            {t(restricted ? 'life.offer.customerKept' : 'life.offer.adminKept', { name: NAMES[component], version })}
        </p>
    );
}
