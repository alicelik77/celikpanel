import { useState } from 'react';
import { Copy, RefreshCw, Download } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, ErrorBanner, RemoteGate } from './ui';
import { readApiError, refusalText, type ApiError } from '../lib/apiError';
import { useRemote, useRefreshEvery } from '../lib/remote';
import { SiteToolGuidance, sendSiteTool } from './SiteToolGuidance';
import { confirmAction, type OperationReport } from '../lib/confirmAction';

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

    // Starts an update or a copy and returns whether the server accepted it.
    const start = async (path: string, method: 'POST' | 'PUT', body: unknown, done?: () => void): Promise<boolean> => {
        if (busy) return false;
        setBusy(true);
        setFailure(null);
        try {
            const res = await sendSiteTool(`${url}/${path}`, method, body);
            if (!res.ok) {
                const problem = await readApiError(res);
                setFailure(problem);
                showToast('error', refusalText(problem, t, 'wpt.startFailed'));
                return false;
            }
            done?.();
            await state.retry();
            return true;
        } finally {
            setBusy(false);
        }
    };

    // A confirmed update or copy runs in the locked progress window until the
    // server's operation is no longer open (owner's firm order, 6 Nov 2026):
    // first the start, then the server's own operation, read every 4 s (reads
    // only). How it ended stays on the page (OperationGuidance).
    // Onaylanan güncelleme ya da kopya, sunucudaki işlem açık kaldıkça kilitli
    // pencerede sürer; yalnız okunur.
    const startAndFollow = async (report: OperationReport, path: string, method: 'POST' | 'PUT', body: unknown, done?: () => void) => {
        const since = Date.now();
        report.steps([
            { id: 'start', label: t('wpt.step.start'), state: 'running', since },
            { id: 'run', label: t('wpt.step.run'), state: 'pending' },
        ]);
        if (!(await start(path, method, body, done))) return;
        const running = Date.now();
        report.steps([
            { id: 'start', label: t('wpt.step.start'), state: 'done' },
            { id: 'run', label: t('wpt.step.run'), state: 'running', since: running, note: t('wpt.step.runNote') },
        ]);
        let unknownSince: number | null = null;
        for (;;) {
            await new Promise((resolve) => window.setTimeout(resolve, 4000));
            const read = await state.retry();
            if (read.state === 'known') {
                unknownSince = null;
                if (!read.value.operations.some((op) => op.open)) return;
            } else {
                unknownSince ??= Date.now();
                if (Date.now() - unknownSince > 60_000) return { state: 'unknown' as const, reason: t('wpt.step.unread') };
            }
        }
    };

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
                        return <SiteToolGuidance tone="prerequisite" reason={t('wpt.phpMissing')} who="administrator" next={t('wpt.phpMissingNext')} resumes={t('wpt.phpMissingResumes')} />;
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

                            <label className="flex cursor-pointer items-start gap-3">
                                <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={value.auto_minor_core} disabled={busy || stale}
                                    onChange={(e) => void start('auto-update', 'PUT', { minor_core: e.target.checked })} />
                                <span>
                                    <span className="block text-sm text-fg">{t('wpt.autoMinor')}</span>
                                    <span className="block text-xs text-fg-subtle">{t('wpt.autoMinorHint')}</span>
                                </span>
                            </label>

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
                                                void confirmAction({ message: t('wpt.clone.confirm', { target: name }), tone: 'danger', progressTitle: t('wpt.step.cloneTitle', { target: name }), action: (report) => startAndFollow(report, 'clone', 'POST', { target_domain_id: Number(target) }, () => setTarget('')) });
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
