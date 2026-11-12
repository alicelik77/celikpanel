import { useEffect, useMemo, useRef, useState } from 'react';
import { Copy, RefreshCw, Download } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, ErrorBanner, RemoteGate } from './ui';
import { readApiError, refusalText, type ApiError } from '../lib/apiError';
import { useRemote, useRefreshEvery } from '../lib/remote';
import { SiteToolGuidance, sendSiteTool } from './SiteToolGuidance';
import { AdminInstallLink } from './AdminInstallLink';
import { confirmAction, followOperation, type OperationEnd, type OperationReport, type OperationStep } from '../lib/confirmAction';
import { forget, remember, remembered } from '../lib/requestProgress';
import { answerWasLost } from '../lib/requestIdentity';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// WordPress tools of one site (alpha.118, docs/WORDPRESS-TOOLS.md): status,
// verified updates with a backup first, the switch for automatic minor
// releases, and a copy to another site of the subscription. A section of its
// own beside the application catalogue. The server enforces the
// "wordpress_tools" right; this screen only shows. Reading never starts an
// update: the open operation is read until it ends.
// Bir sitenin WordPress araçları; uygulama kataloğunun yanında ayrı bölüm.

interface WPItem {
    kind: string;
    slug: string;
    file: string;
    name: string;
    version: string;
    update_version?: string;
    source?: string;
    verifiable: boolean;
}

interface WPOperation {
    operation_id: string;
    kind: 'update' | 'clone';
    scope?: string;
    target_domain?: string;
    origin: string;
    status: string;
    backup_name?: string;
    code?: string;
    detail?: string;
    updated_at: string;
    open: boolean;
}

interface WPState {
    project_type: string;
    auto_minor_core: boolean;
    database_linked?: boolean;
    operations: WPOperation[];
    status: {
        detected: boolean;
        core_version?: string;
        core_update?: string;
        core_minor?: string;
        plugins: WPItem[];
        themes: WPItem[];
        updates_checked: boolean;
        code?: string;
    };
}

const decodeState = (raw: unknown): WPState => {
    const value = raw as WPState;
    if (!value || typeof value !== 'object' || !value.status || !Array.isArray(value.operations)) throw new Error('unreadable');
    return value;
};

const decodeTargets = (raw: unknown): { domain_id: number; domain: string }[] => {
    const list = (raw as { targets?: unknown }).targets;
    if (!Array.isArray(list)) throw new Error('unreadable');
    return list as { domain_id: number; domain: string }[];
};

function OperationGuidance({ op }: { op: WPOperation }) {
    const { t } = useI18n();
    const detail = op.detail ?? '';
    if (op.open) {
        const reason = op.kind === 'clone' && op.status !== 'queued'
            ? t('wpt.op.cloning', { target: op.target_domain ?? '' })
            : t(`wpt.op.${op.status}` as TranslationKey);
        return <SiteToolGuidance tone="progress" reason={reason} who="nobody" resumes={t('wpt.op.runningResumes')} />;
    }
    const backup = op.backup_name ? <p className="text-xs text-fg-muted">{t('wpt.op.backup', { backup: op.backup_name })}</p> : null;
    const coded = op.code ? t(`err.${op.code}` as TranslationKey) : '';
    const why = coded && coded !== `err.${op.code}` ? coded : detail;
    switch (op.status) {
        case 'applied':
            return <SiteToolGuidance tone="success" reason={t('wpt.op.applied', { detail })}>{backup}</SiteToolGuidance>;
        case 'failed':
            return <SiteToolGuidance tone="failure" reason={t('wpt.op.failed', { detail: why })} who="you" next={t('wpt.op.failedNext')}>{backup}</SiteToolGuidance>;
        case 'rolled_back':
            return <SiteToolGuidance tone="failure" reason={t('wpt.op.rolled_back', { detail })} who="you" next={t('wpt.op.rolledBackNext')}>{backup}</SiteToolGuidance>;
        default:
            // alpha.127: a restore refused for a PostgreSQL prerequisite says
            // which one (its own coded sentence, the server administrator acts),
            // with the facts underneath; nothing of the backup was written.
            if (op.code?.startsWith('WORDPRESS_RESTORE_PG_') && coded && coded !== `err.${op.code}`) {
                return (
                    <SiteToolGuidance tone="failure" reason={coded} who="administrator" next={t('wpt.op.needsReviewNext', { backup: op.backup_name ?? '—' })}>
                        {detail && <p className="text-xs text-fg-muted">{detail}</p>}
                    </SiteToolGuidance>
                );
            }
            return (
                <SiteToolGuidance tone="unknown" reason={t('wpt.op.needs_review', { detail })} who="you"
                    next={t('wpt.op.needsReviewNext', { backup: op.backup_name ?? '—' })} />
            );
    }
}

// The window's steps are the server's own durable status of the operation
// (alpha.127): waiting for its turn, the backup made first, the update or the
// copy, the restore when an update fails its check, and the end.
// Pencerenin adımları, işlemin sunucudaki kalıcı durumudur.
type WPStepID = 'start' | 'queued' | 'backing_up' | 'updating' | 'restoring' | 'cloning' | 'finish';
const WP_ORDER: Record<'update' | 'clone', WPStepID[]> = {
    update: ['start', 'queued', 'backing_up', 'updating', 'finish'],
    clone: ['start', 'queued', 'cloning', 'finish'],
};

export function wordpressSteps(
    t: ReturnType<typeof useI18n>['t'], kind: 'update' | 'clone', at: WPStepID, since: number,
    ended: 'done' | 'failed' | 'unknown' | null, restored: boolean, target: string,
): OperationStep[] {
    const order = [...WP_ORDER[kind]];
    if (restored) order.splice(order.indexOf('updating') + 1, 0, 'restoring');
    const index = Math.max(0, order.indexOf(at));
    return order.map((id, position) => {
        const label = t(`opstep.wp.${id}` as TranslationKey, { target });
        if (ended === 'done' || position < index) {
            // The update that failed its check is the step that failed; the restore after it ran.
            return { id, label, state: id === 'updating' && restored ? 'failed' as const : 'done' as const };
        }
        if (position > index) return { id, label, state: 'pending' as const };
        if (ended === 'failed') return { id, label, state: 'failed' as const, note: t('opstep.stoppedHere') };
        if (ended === 'unknown') return { id, label, state: 'waiting' as const, note: t('opstep.stepUnknown') };
        return { id, label, state: 'running' as const, since };
    });
}

export function WordPressToolsPanel({ domainId }: { domainId: number }) {
    const { t } = useI18n();
    const url = `/api/v1/wordpress/${domainId}`;
    const state = useRemote(url, decodeState);
    const known = state.remote.state === 'known' ? state.remote.value : undefined;
    const running = known?.operations.some((op) => op.open) ?? false;
    useRefreshEvery(state, 4000, running);
    const targets = useRemote(known?.status.detected ? `${url}/clone-targets` : null, decodeTargets);
    const [selected, setSelected] = useState<string[]>([]);
    const [target, setTarget] = useState('');
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<ApiError | null>(null);

    // Starts an update or a copy; answers the operation the server accepted
    // (its id, or '' when the answer named none), or null when it was refused.
    const start = async (path: string, method: 'POST', body: unknown, done?: () => void): Promise<string | null> => {
        if (busy) return null;
        setBusy(true);
        setFailure(null);
        try {
            const res = await sendSiteTool(`${url}/${path}`, method, body);
            if (!res.ok) {
                const problem = await readApiError(res);
                setFailure(problem);
                showToast('error', refusalText(problem, t, 'wpt.startFailed'));
                return null;
            }
            let id = '';
            try {
                const answer = await res.json() as { operation_id?: unknown } | null;
                if (typeof answer?.operation_id === 'string') id = answer.operation_id;
            } catch {
                // The change was accepted; which operation is read from the list.
                id = '';
            }
            done?.();
            await state.retry();
            return id;
        } finally {
            setBusy(false);
        }
    };

    // A confirmed update or copy runs in the locked progress window until the
    // server's operation is no longer open (owner's firm order, 6 Nov 2026).
    // The steps are the operation's own status, read every 4 s (reads only);
    // the end says how it ended, who acts and the next step (D-024). The
    // operation is kept in this tab's session, so a reload reopens the window.
    // Onaylanan güncelleme ya da kopya, sunucudaki işlem açık kaldıkça kilitli
    // pencerede sürer; adımlar işlemin kendi durumudur. Yalnız okunur.
    const sessionKey = `wp.${domainId}`;
    const followRun = async (report: OperationReport, kind: 'update' | 'clone', id: string, target: string): Promise<OperationEnd | void> => {
        let at: WPStepID = 'queued';
        let since = Date.now();
        let restored = false;
        let unknownSince: number | null = null;
        const show = (ended: 'done' | 'failed' | 'unknown' | null) => report.steps(wordpressSteps(t, kind, at, since, ended, restored, target));
        show(null);
        try {
            for (;;) {
                await new Promise((resolve) => window.setTimeout(resolve, 4000));
                const read = await state.retry();
                if (read.state !== 'known') {
                    unknownSince ??= Date.now();
                    if (Date.now() - unknownSince > 60_000) {
                        show('unknown');
                        return { state: 'unknown', reason: t('wpt.step.unread') };
                    }
                    continue;
                }
                unknownSince = null;
                const op = read.value.operations.find((item) => (id ? item.operation_id === id : item.kind === kind));
                if (!op) {
                    show('unknown');
                    return { state: 'unknown', reason: t('wpt.step.unread') };
                }
                if (op.open) {
                    const next: WPStepID = op.status === 'cloning' || op.status === 'updating' || op.status === 'backing_up' || op.status === 'restoring'
                        ? op.status : 'queued';
                    if (next === 'restoring') restored = true;
                    if (next !== at) {
                        at = next;
                        since = Date.now();
                    }
                    show(null);
                    continue;
                }
                const detail = op.detail ?? '';
                const coded = op.code ? t(`err.${op.code}` as TranslationKey) : '';
                const why = coded && coded !== `err.${op.code}` ? coded : detail;
                if (op.status === 'applied') {
                    at = 'finish';
                    show('done');
                    return { state: 'done', message: t('wpt.op.applied', { detail }) };
                }
                if (op.status === 'rolled_back') restored = true;
                if (op.status === 'failed' || op.status === 'rolled_back') {
                    show('failed');
                    return {
                        state: 'failed',
                        reason: op.status === 'failed'
                            ? `${t('wpt.op.failed', { detail: why })} ${t('wpt.op.failedNext')}`
                            : `${t('wpt.op.rolled_back', { detail })} ${t('wpt.op.rolledBackNext')}`,
                    };
                }
                show('unknown');
                return { state: 'unknown', reason: `${t('wpt.op.needs_review', { detail })} ${t('wpt.op.needsReviewNext', { backup: op.backup_name ?? '—' })}` };
            }
        } finally {
            forget(sessionKey);
        }
    };

    const startAndFollow = async (report: OperationReport, path: 'update' | 'clone', method: 'POST', body: unknown, done?: () => void, target = '') => {
        const since = Date.now();
        report.steps(wordpressSteps(t, path, 'start', since, null, false, target));
        const id = await start(path, method, body, done);
        if (id === null) {
            report.steps(wordpressSteps(t, path, 'start', since, 'failed', false, target));
            return;
        }
        remember(sessionKey, id, { kind: path, target });
        return followRun(report, path, id, target);
    };

    // An update or copy this tab started and the page reloaded during: the
    // window reopens at the operation's status while it is still open.
    // Sayfa yenilenirken süren işlem, açık kaldıkça yeniden izlenir.
    const resumed = useRef(false);
    useEffect(() => {
        if (resumed.current || !known) return;
        resumed.current = true;
        const kept = remembered(sessionKey);
        if (!kept) return;
        const op = known.operations.find((item) => item.operation_id === kept.id);
        if (!op?.open) {
            forget(sessionKey);
            return;
        }
        const kind = kept.context.kind === 'clone' ? 'clone' : 'update';
        const target = kept.context.target ?? '';
        void followOperation({
            progressTitle: kind === 'clone' ? t('wpt.step.cloneTitle', { target }) : t('wpt.step.pluginsTitle'),
            action: (report) => followRun(report, kind, kept.id, target),
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [known]);

    return (
        <section className="mt-6 border-t border-border pt-6" aria-labelledby={`wpt-${domainId}`}>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 id={`wpt-${domainId}`} className="text-base font-semibold text-fg">{t('wpt.title')}</h3>
                <Button icon={RefreshCw} loading={state.reading} onClick={() => void state.retry()}>{t('wpt.check')}</Button>
            </div>
            <RemoteGate remote={state.remote} checking={t('wpt.checking')} failed={t('wpt.unknown')} onRetry={() => void state.retry()} busy={state.reading}>
                {({ value, stale }) => {
                    const s = value.status;
                    if (value.project_type !== 'php') return <p className="text-sm text-fg-muted">{t('wpt.notPHP')}</p>;
                    if (s.code === 'WORDPRESS_PHP_CLI_MISSING') {
                        // alpha.127: the administrator opens PHP-FPM's page, where the site's version is installed again.
                        return (
                            <SiteToolGuidance tone="prerequisite" reason={t('wpt.phpMissing')} who="administrator" next={t('wpt.phpMissingNext')} resumes={t('wpt.phpMissingResumes')}>
                                <AdminInstallLink to="/components/php-fpm" label={t('adminNext.openPHPFPM')} />
                            </SiteToolGuidance>
                        );
                    }
                    if (!s.detected) return <SiteToolGuidance tone="prerequisite" reason={t('wpt.notDetected')} who="you" next={t('wpt.notDetectedNext')} />;
                    const latest = value.operations[0];
                    const locked = busy || stale || running;
                    const coreTarget = s.core_minor || s.core_update;
                    const updatable = s.plugins.filter((p) => p.update_version && p.verifiable);
                    return (
                        <div className="space-y-4">
                            {latest && <OperationGuidance op={latest} />}
                            <ErrorBanner error={failure} sentence="wpt.startFailed" />
                            {!s.updates_checked && <SiteToolGuidance tone="unknown" reason={t('wpt.updatesNotChecked')} who="you" next={t('wpt.check')} />}

                            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-surface p-3">
                                <div>
                                    <div className="font-medium text-fg">{t('wpt.core', { version: s.core_version ?? '' })}</div>
                                    <div className="text-xs text-fg-muted">
                                        {s.core_minor ? t('wpt.coreMinor', { version: s.core_minor })
                                            : s.core_update ? t('wpt.coreUpdate', { version: s.core_update }) : t('wpt.coreLatest')}
                                    </div>
                                </div>
                                {coreTarget && (
                                    <Button variant="primary" icon={Download} disabled={locked || value.database_linked === false}
                                        onClick={() => void confirmAction({ message: t('wpt.confirmCore', { version: coreTarget }), progressTitle: t('wpt.step.coreTitle', { version: coreTarget }), action: (report) => startAndFollow(report, 'update', 'POST', { kind: 'core', core_version: coreTarget }) })}>
                                        {t('wpt.updateTo', { version: coreTarget })}
                                    </Button>
                                )}
                            </div>
                            {value.database_linked === false && <SiteToolGuidance tone="prerequisite" reason={t('wpt.dbNotLinked')} />}

                            <AutoMinorSetting url={url} value={value} stale={stale} reading={state.reading} onSaved={() => state.retry()} />

                            <div>
                                <h4 className="mb-2 text-sm font-semibold text-fg">{t('wpt.plugins')}</h4>
                                {s.plugins.length === 0 ? <p className="text-sm text-fg-muted">{t('wpt.none')}</p> : (
                                    <ul className="divide-y divide-border rounded-lg border border-border">
                                        {s.plugins.map((p) => (
                                            <li key={p.file} className="flex flex-wrap items-center gap-3 px-3 py-2 text-sm">
                                                <input type="checkbox" aria-label={p.name} className="h-4 w-4 accent-primary"
                                                    disabled={!p.update_version || !p.verifiable || locked}
                                                    checked={selected.includes(p.file)}
                                                    onChange={(e) => setSelected((cur) => e.target.checked ? [...cur, p.file] : cur.filter((f) => f !== p.file))} />
                                                <span className="min-w-0 flex-1 break-words text-fg">{p.name} <span className="text-fg-muted">{p.version}</span></span>
                                                <span className="text-xs text-fg-muted">
                                                    {!p.update_version ? (p.source === 'other' ? t('wpt.notFromOrg') : t('wpt.upToDate'))
                                                        : p.verifiable ? `→ ${p.update_version}` : t('wpt.notVerifiable', { version: p.update_version })}
                                                </span>
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {updatable.length > 0 && (
                                    <Button className="mt-2" variant="primary" disabled={locked || selected.length === 0}
                                        onClick={() => void confirmAction({ message: t('wpt.confirmPlugins', { count: selected.length }), progressTitle: t('wpt.step.pluginsTitle'), action: (report) => startAndFollow(report, 'update', 'POST', { kind: 'plugin', items: selected }, () => setSelected([])) })}>
                                        {t('wpt.updateSelected', { count: selected.length })}
                                    </Button>
                                )}
                            </div>

                            <div>
                                <h4 className="mb-2 text-sm font-semibold text-fg">{t('wpt.themes')}</h4>
                                {s.themes.length === 0 ? <p className="text-sm text-fg-muted">{t('wpt.none')}</p> : (
                                    <ul className="divide-y divide-border rounded-lg border border-border">
                                        {s.themes.map((theme) => (
                                            <li key={theme.slug} className="flex flex-wrap items-center gap-3 px-3 py-2 text-sm">
                                                <span className="min-w-0 flex-1 break-words text-fg">{theme.name} <span className="text-fg-muted">{theme.version}</span></span>
                                                <span className="text-xs text-fg-muted">{theme.update_version ? `→ ${theme.update_version}` : t('wpt.upToDate')}</span>
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {s.themes.some((theme) => theme.update_version) && <p className="mt-2 text-xs text-fg-muted">{t('wpt.themeNote')}</p>}
                            </div>

                            <div className="rounded-lg border border-border bg-surface p-3">
                                <h4 className="text-sm font-semibold text-fg">{t('wpt.clone.title')}</h4>
                                <p className="mb-2 text-xs text-fg-muted">{t('wpt.clone.hint')}</p>
                                {targets.remote.state === 'known' && targets.remote.value.length === 0 ? (
                                    <p className="text-sm text-fg-muted">{t('wpt.clone.noTargets')}</p>
                                ) : (
                                    <div className="flex flex-wrap items-center gap-2">
                                        <label className="sr-only" htmlFor={`wpt-target-${domainId}`}>{t('wpt.clone.target')}</label>
                                        <select id={`wpt-target-${domainId}`} value={target} onChange={(e) => setTarget(e.target.value)}
                                            className="min-h-9 rounded-lg border border-border bg-surface px-2 text-sm text-fg">
                                            <option value="">{t('wpt.clone.choose')}</option>
                                            {targets.remote.state === 'known' && targets.remote.value.map((c) => <option key={c.domain_id} value={c.domain_id}>{c.domain}</option>)}
                                        </select>
                                        <Button icon={Copy} disabled={locked || target === ''}
                                            onClick={() => {
                                                const name = targets.remote.state === 'known' ? targets.remote.value.find((c) => String(c.domain_id) === target)?.domain ?? '' : '';
                                                void confirmAction({ message: t('wpt.clone.confirm', { target: name }), tone: 'danger', progressTitle: t('wpt.step.cloneTitle', { target: name }), action: (report) => startAndFollow(report, 'clone', 'POST', { target_domain_id: Number(target) }, () => setTarget(''), name) });
                                            }}>
                                            {t('wpt.clone.start')}
                                        </Button>
                                    </div>
                                )}
                            </div>
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

// alpha.132: automatic minor releases are a setting saved by Save, not by
// ticking it (owner, 8 Nov 2026). The tick is a draft over the state last
// read; the update and copy above stay actions.
// Küçük sürümlerin kendiliğinden kurulması Kaydet ile kaydedilen bir ayardır.
function AutoMinorSetting({ url, value, stale, reading, onSaved }: {
    url: string; value: WPState; stale: boolean; reading: boolean; onSaved: () => Promise<unknown>;
}) {
    const { t } = useI18n();
    const say = useSaveWords();
    const known = useMemo(() => ({ auto_minor_core: value.auto_minor_core }), [value]);
    const draft = useDraft(known);
    const on = draft.values?.auto_minor_core ?? value.auto_minor_core;
    const changed = draft.isChanged('auto_minor_core');
    const save = () => void draft.save(async (changes) => {
        let res: Response | null = null;
        try {
            res = await sendSiteTool(`${url}/auto-update`, 'PUT', { minor_core: changes.auto_minor_core });
        } catch {
            res = null;
        }
        if (!res || answerWasLost(res)) return 'unknown';
        if (!res.ok) {
            showToast('error', refusalText(await readApiError(res), t, 'wpt.autoMinorFailed'));
            return 'refused';
        }
        showToast('success', t('wpt.autoMinorSaved'));
        await onSaved();
        return 'saved';
    });
    return (
        <div className="rounded-lg border border-border px-3">
            <label className={`-mx-3 flex cursor-pointer items-start gap-3 px-3 py-3 ${changed ? changedRowClass : ''}`}>
                <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={on} disabled={draft.saving}
                    onChange={(e) => draft.set('auto_minor_core', e.target.checked)} />
                <span>
                    <span className="block text-sm text-fg">
                        {t('wpt.autoMinor')}
                        {changed && <ChangedMark switched={on} />}
                    </span>
                    <span className="block text-xs text-fg-subtle">{t('wpt.autoMinorHint')}</span>
                </span>
            </label>
            {draft.isMoved('auto_minor_core') && draft.base && (
                <ServerMovedNote value={say(draft.base.auto_minor_core ? 'on' : 'off')} onTake={() => draft.take('auto_minor_core')} />
            )}
            <SaveBar
                className="-mx-3 mt-0 rounded-b-lg px-3"
                count={draft.count}
                saving={draft.saving}
                blockedBy={stale ? say('stale') : undefined}
                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: reading, onCheck: () => void onSaved() } : undefined}
                onSave={save}
                onDiscard={draft.reset}
            />
        </div>
    );
}
