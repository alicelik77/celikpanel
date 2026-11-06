import { useEffect, useRef, useState } from 'react';
import { LayoutGrid, Download, ExternalLink, Loader2, RotateCcw, Trash2, KeyRound, ChevronDown, ChevronUp } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, Checking, ErrorBanner, KnownEmpty, RemoteGate, ResultUnknown } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useNavigate } from '../router';
import { useRefreshEvery, useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';
import {
    RUNNING_STATES, decodeDomainApps, domainAppInstallURL, domainAppOperationURL, domainAppsURL, phpExtensionsInstallPath,
    type DomainApp, type InstalledApp,
} from '../lib/appCatalog';
import type { TranslationKey } from '../i18n/en';
import { confirmAction, followOperation, type OperationEnd, type OperationReport, type OperationStep } from '../lib/confirmAction';
import { forget, remember, remembered } from '../lib/requestProgress';

// The application catalog of one domain (alpha.118, docs/APP-CATALOG.md):
// every application the administrator offers and this subscription may use,
// each with what it needs and whether this site has it, and the application
// the site already holds with its sign-in address, Retry and Remove. The
// install is one request; while it runs the list is read again every few
// seconds (reading only) so the step it is on is shown. An install whose
// answer was lost is not offered again blind: the server refuses a second
// application on the domain, and the list read again shows what happened.
//
// Bir alan adının uygulama kataloğu: yöneticinin sunduğu ve aboneliğin
// kullanabildiği her uygulama, gereksinimleri ve bu sitenin durumuyla; sitede
// kurulu uygulama, giriş adresi, Yeniden dene ve Kaldır ile.

interface Shown {
    name: string;
    admin_url?: string;
    site_url?: string;
    admin_user?: string;
    admin_password?: string;
    operation_id?: string;
}

const STEP_KEYS: Record<string, TranslationKey> = {
    reserved: 'apps.step.database',
    database_creating: 'apps.step.database',
    database_ready: 'apps.step.files',
    files_installing: 'apps.step.files',
    files_ready: 'apps.step.setup',
    setting_up: 'apps.step.setup',
    removing: 'apps.step.removing',
};

// The install window's steps are the install's own durable status, read from
// the list while the request runs (alpha.127): what the application needs is
// checked first (before anything is made), then the database, the files, the
// application's own setup (catalog applications) and the end.
// Kurulum penceresinin adımları kurulumun kalıcı durumudur.
type AppStepID = 'check' | 'database' | 'files' | 'setup' | 'finish';
const APP_STATUS_STEP: Record<string, AppStepID> = {
    reserved: 'database', database_creating: 'database', database_ready: 'files', files_installing: 'files',
    files_ready: 'setup', setting_up: 'setup', applied: 'finish',
};

export function appInstallSteps(
    t: ReturnType<typeof useI18n>['t'], name: string, withSetup: boolean, at: AppStepID, since: number,
    ended: 'done' | 'failed' | 'prerequisite' | 'unknown' | null,
): OperationStep[] {
    const order: AppStepID[] = withSetup ? ['check', 'database', 'files', 'setup', 'finish'] : ['check', 'database', 'files', 'finish'];
    const index = Math.max(0, order.indexOf(at));
    return order.map((id, position) => {
        const label = t(`opstep.app.${id}` as TranslationKey, { name });
        if (ended === 'done' || position < index) return { id, label, state: 'done' as const };
        if (position > index) return { id, label, state: 'pending' as const };
        if (ended === 'failed' || ended === 'prerequisite') {
            const prerequisite = ended === 'prerequisite' && id === 'check';
            return { id, label, state: 'failed' as const, prerequisite, note: t(prerequisite ? 'opstep.stoppedBeforeChange' : 'opstep.stoppedHere') };
        }
        if (ended === 'unknown') return { id, label, state: 'waiting' as const, note: t('opstep.stepUnknown') };
        return { id, label, state: 'running' as const, since };
    });
}

export function DomainAppsPanel({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useI18n();
    const url = domainAppsURL(domainId);
    const apps = useRemote(url, decodeDomainApps);
    const answer = useLostAnswer(() => apps.retry());
    const [busy, setBusy] = useState<string | null>(null);
    const [shown, setShown] = useState<Shown | null>(null);
    const [failure, setFailure] = useState<ApiError | null>(null);
    const [removing, setRemoving] = useState<InstalledApp | null>(null);
    const running = apps.remote.state === 'known' && apps.remote.value.installed.some((app) => RUNNING_STATES.includes(app.status));
    // The screen moves while something runs; reading only.
    useRefreshEvery(apps, 4000, busy !== null || running);

    // Said once, in the banner (useRefusal): no notice repeats it.
    const failed = (problem: ApiError) => setFailure(problem);

    // The status of the last answer (the install window tells a refused check
    // from a failure).
    const lastStatus = useRef(0);
    const post = async (target: string, body: unknown, method = 'POST') => {
        lastStatus.current = 0;
        const res = await answer.send(target, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
        if (!res) return null;
        lastStatus.current = res.status;
        if (!res.ok) {
            failed(await readApiError(res));
            void apps.retry();
            return null;
        }
        try {
            return (await res.json()) as Record<string, unknown>;
        } catch {
            answer.lose();
            return null;
        }
    };

    // Reads the list while the install request runs and shows the step its
    // durable status names. Reads only; stops when the request answers.
    // Kurulum isteği sürerken liste okunur ve kalıcı durumun adımı gösterilir.
    const sessionKey = `app.${domainId}`;
    const watchInstall = (appId: string, name: string, withSetup: boolean, report: OperationReport, isDone: () => boolean) => {
        const seen = { at: 'check' as AppStepID, since: Date.now() };
        void (async () => {
            while (!isDone()) {
                await new Promise((resolve) => setTimeout(resolve, 2000));
                if (isDone()) break;
                const read = await apps.retry();
                if (isDone() || read.state !== 'known') continue;
                const row = read.value.installed.find((item) => item.app_id === appId);
                const at = row ? APP_STATUS_STEP[row.status] : undefined;
                if (!at || at === 'finish') continue;
                if (at !== seen.at) {
                    seen.at = at;
                    seen.since = Date.now();
                }
                report.steps(appInstallSteps(t, name, withSetup, seen.at, seen.since, null));
            }
        })();
        return seen;
    };

    // After a reload: an install this tab started that the list still shows
    // running is followed in the locked window until it ends (reads only).
    // Sayfa yenilenirken süren kurulum, bitene dek kilitli pencerede izlenir.
    const followInstall = async (appId: string, name: string, withSetup: boolean, report: OperationReport): Promise<OperationEnd | void> => {
        let at: AppStepID = 'database';
        let since = Date.now();
        let unknownSince: number | null = null;
        try {
            for (;;) {
                const read = await apps.retry();
                if (read.state !== 'known') {
                    unknownSince ??= Date.now();
                    if (Date.now() - unknownSince > 60_000) return { state: 'unknown', reason: t('opstep.unread') };
                } else {
                    unknownSince = null;
                    const row = read.value.installed.find((item) => item.app_id === appId);
                    if (!row) return { state: 'unknown', reason: t('opstep.gone') };
                    if (row.status === 'applied') {
                        report.steps(appInstallSteps(t, name, withSetup, 'finish', since, 'done'));
                        return { state: 'done', message: t('apps.installed', { name }) };
                    }
                    const next = APP_STATUS_STEP[row.status];
                    if (!next || !RUNNING_STATES.includes(row.status)) {
                        report.steps(appInstallSteps(t, name, withSetup, at, since, row.status === 'needs_review' ? 'unknown' : 'failed'));
                        const why = row.last_error ? ` ${row.last_error}` : '';
                        return row.status === 'needs_review'
                            ? { state: 'unknown', reason: `${t('apps.installFailed')}${why}` }
                            : { state: 'failed', reason: `${t('apps.installFailed')}${why}` };
                    }
                    if (next !== at) {
                        at = next;
                        since = Date.now();
                    }
                    report.steps(appInstallSteps(t, name, withSetup, at, since, null));
                }
                await new Promise((resolve) => setTimeout(resolve, 3000));
            }
        } finally {
            forget(sessionKey);
        }
    };
    const resumed = useRef(false);
    useEffect(() => {
        if (resumed.current || apps.remote.state !== 'known') return;
        resumed.current = true;
        const kept = remembered(sessionKey);
        if (!kept) return;
        const row = apps.remote.value.installed.find((item) => item.app_id === kept.id);
        if (!row || !RUNNING_STATES.includes(row.status) || row.status === 'removing') {
            forget(sessionKey);
            return;
        }
        const name = kept.context.name ?? row.name;
        void followOperation({
            progressTitle: t('opstep.app.title', { name }),
            action: (report) => followInstall(kept.id, name, kept.context.setup === '1', report),
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [apps.remote]);

    const install = async (app: DomainApp) => {
        if (answer.holding || busy || apps.remote.state !== 'known') return;
        await confirmAction({ message: t('apps.confirmInstall', { name: app.name }),
            progressTitle: t('opstep.app.title', { name: app.name }),
            action: async (report): Promise<OperationEnd | void> => {
                setBusy(app.id);
                setShown(null);
                setFailure(null);
                // WordPress has no setup step of its own (its setup page opens after).
                const withSetup = app.id !== 'wordpress';
                report.steps(appInstallSteps(t, app.name, withSetup, 'check', Date.now(), null));
                remember(sessionKey, app.id, { name: app.name, setup: withSetup ? '1' : '' });
                let answered = false;
                const seen = watchInstall(app.id, app.name, withSetup, report, () => answered);
                // How the steps end once the request has answered.
                const close = (ended: 'done' | 'failed' | 'unknown') => {
                    answered = true;
                    forget(sessionKey);
                    const refusedCheck = ended === 'failed' && seen.at === 'check' && lastStatus.current >= 400 && lastStatus.current < 500;
                    report.steps(appInstallSteps(t, app.name, withSetup, ended === 'done' ? 'finish' : seen.at, seen.since,
                        refusedCheck ? 'prerequisite' : ended));
                };
                try {
                    const data = await post(domainAppInstallURL(domainId), { app: app.id });
                    if (!data) {
                        close(lastStatus.current >= 400 ? 'failed' : 'unknown');
                        return;
                    }
                    if (data.success !== true) {
                        close('failed');
                        failed({ message: typeof data.error === 'string' ? data.error : '' });
                        return;
                    }
                    close('done');
                    showToast('success', t('apps.installed', { name: app.name }));
                    answer.settle();
                    setShown({
                        name: app.name, site_url: str(data.site_url), admin_url: str(data.admin_url) || str(data.setup_url),
                        admin_user: str(data.admin_user), admin_password: str(data.admin_password), operation_id: str(data.operation_id),
                    });
                    void apps.retry();
                } finally {
                    answered = true;
                    setBusy(null);
                }
            },
        });
    };

    const retry = async (app: InstalledApp) => {
        if (answer.holding || busy) return;
        setBusy(app.operation_id);
        setFailure(null);
        try {
            const data = await post(domainAppOperationURL(domainId, app.operation_id, 'retry'), {});
            if (data?.success === true) {
                answer.settle();
                setShown({ name: app.name, site_url: str(data.site_url), admin_url: str(data.admin_url), admin_user: str(data.admin_user),
                    admin_password: str(data.admin_password), operation_id: app.operation_id });
                void apps.retry();
            }
        } finally {
            setBusy(null);
        }
    };

    // Shows the generated password once; the server forgets it.
    const reveal = async (app: InstalledApp) => {
        if (answer.holding || busy) return;
        setBusy(app.operation_id);
        try {
            const data = await post(domainAppOperationURL(domainId, app.operation_id, 'credentials'), {});
            if (data) {
                setShown({ name: app.name, admin_url: app.admin_url, admin_user: str(data.admin_user), admin_password: str(data.admin_password) });
                void apps.retry();
            }
        } finally {
            setBusy(null);
        }
    };

    // "I have saved it": the server forgets the password it kept for a lost answer.
    const saved = async () => {
        const op = shown?.operation_id;
        setShown(null);
        if (op && (await post(domainAppOperationURL(domainId, op, 'credentials'), {}))) void apps.retry();
    };

    return (
        <RemoteGate remote={apps.remote} checking={t('apps.checking')} failed={t('apps.unknown')} onRetry={() => void apps.retry()} busy={apps.reading}>
            {(known) => known.value.apps.length === 0 && known.value.installed.length === 0 ? (
                <KnownEmpty of={known} icon={LayoutGrid} title={t('apps.empty')} />
            ) : (
                <div data-app-catalog="">
                    <p className="mb-4 max-w-[75ch] text-sm text-fg-muted">{t('apps.catalogHint')}</p>
                    <ResultUnknown answer={answer} where={t('apps.resultUnknownWhere')} className="mb-4" />
                    <ErrorBanner className="mb-4" error={failure} sentence="apps.installFailed" />
                    {shown && <Credentials shown={shown} onDone={() => void saved()} />}
                    {known.value.installed.length > 0 && (
                        <section className="mb-6" data-installed-apps="">
                            <h3 className="mb-2 text-sm font-semibold text-fg">{t('apps.installedTitle')}</h3>
                            <ul className="grid gap-2">
                                {known.value.installed.map((app) => (
                                    <InstalledRow key={app.operation_id} app={app} busy={busy !== null || known.stale || answer.holding}
                                        onRetry={() => void retry(app)} onReveal={() => void reveal(app)} onRemove={() => setRemoving(app)} />
                                ))}
                            </ul>
                        </section>
                    )}
                    {removing && (
                        <RemoveForm domainId={domainId} domainName={domainName} app={removing} send={post}
                            onClose={() => { setRemoving(null); void apps.retry(); }} />
                    )}
                    {busy && !removing && (
                        <Checking className="mb-4" label={t('apps.working')} />
                    )}
                    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                        {known.value.apps.map((app) => (
                            <AppCard key={app.id} app={app} occupied={known.value.occupied} administrator={known.value.administrator}
                                installing={busy === app.id} disabled={busy !== null || known.stale || answer.holding}
                                onInstall={() => void install(app)} />
                        ))}
                    </div>
                </div>
            )}
        </RemoteGate>
    );
}

const str = (v: unknown) => (typeof v === 'string' ? v : '');

function Credentials({ shown, onDone }: { shown: Shown; onDone: () => void }) {
    const { t } = useI18n();
    return (
        <div className="mb-4 rounded-lg border border-success/30 bg-success/10 p-3 text-sm" data-app-credentials="">
            <p className="font-semibold text-success">{t('apps.readyTitle', { name: shown.name })}</p>
            {shown.admin_url && (
                <p className="mt-1"><a href={shown.admin_url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 font-medium text-primary underline underline-offset-2">
                    <ExternalLink className="h-4 w-4" />{shown.admin_url}</a></p>
            )}
            {!shown.admin_password && <p className="mt-1 text-fg">{t('apps.finishSetup')}</p>}
            {shown.admin_password && (
                <>
                    <p className="mt-2 text-fg">{t('apps.adminUser')}: <code className="rounded bg-surface px-1">{shown.admin_user || '—'}</code></p>
                    <p className="text-fg">{t('apps.adminPassword')}: <code className="select-all rounded bg-surface px-1">{shown.admin_password}</code></p>
                    <p className="mt-1 text-xs text-fg-muted">{t('apps.passwordOnce')}</p>
                </>
            )}
            <div className="mt-2"><Button onClick={onDone}>{t('apps.savedIt')}</Button></div>
        </div>
    );
}

function statusWords(app: InstalledApp, t: (key: TranslationKey, vars?: Record<string, string | number>) => string): { text: string; tone: string } {
    const step = STEP_KEYS[app.status];
    if (step) return { text: t(step, { name: app.name }) + ' ' + t('apps.waitNothingToDo'), tone: 'text-fg-muted' };
    switch (app.status) {
        case 'applied': return { text: t('apps.status.applied'), tone: 'text-success' };
        case 'setup_failed': return { text: t('apps.status.setupFailed', { name: app.name }), tone: 'text-warning' };
        // A removal that stopped part way is a known state with its own
        // next step (Remove again), not a lost answer (alpha.118).
        case 'needs_review': return app.error_kind === 'remove'
            ? { text: t('apps.status.removeIncomplete', { name: app.name }), tone: 'text-warning' }
            : { text: t('apps.status.needsReview'), tone: 'text-warning' };
        case 'failed': return { text: t('apps.status.failed'), tone: 'text-danger' };
    }
    return { text: app.status, tone: 'text-fg-muted' };
}

function InstalledRow({ app, busy, onRetry, onReveal, onRemove }: { app: InstalledApp; busy: boolean; onRetry: () => void; onReveal: () => void; onRemove: () => void }) {
    const { t } = useI18n();
    const status = statusWords(app, t);
    return (
        <li className="rounded-lg border border-border bg-surface p-3" data-installed-app={app.app_id} data-status={app.status}>
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                    <span className="font-semibold text-fg">{app.name}</span>
                    {app.version && <span className="ml-2 text-xs text-fg-muted">{app.version}</span>}
                    <p className={`text-sm ${status.tone}`}>{status.text}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                    {app.admin_url && (
                        <a href={app.admin_url} target="_blank" rel="noopener noreferrer"
                            className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2">
                            <ExternalLink className="h-4 w-4" />{t('apps.openAdmin')}
                        </a>
                    )}
                    {app.credentials_pending && app.status === 'applied' && <Button icon={KeyRound} disabled={busy} onClick={onReveal}>{t('apps.showPassword')}</Button>}
                    {app.can_retry && <Button variant="primary" icon={RotateCcw} disabled={busy} onClick={onRetry}>{t('apps.retrySetup')}</Button>}
                    {app.can_remove && !app.legacy && <Button icon={Trash2} disabled={busy} onClick={onRemove}>{t('apps.remove')}</Button>}
                </div>
            </div>
            {app.last_error && app.status !== 'applied' && (
                <p className="mt-2 whitespace-pre-wrap break-words text-xs text-fg-muted">{app.last_error}</p>
            )}
        </li>
    );
}

function Requirements({ app, administrator }: { app: DomainApp; administrator: boolean }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const req = app.requirements;
    if (!app.available && administrator && app.refused_by === 'administrator') {
        return <p className="text-sm text-warning">{t('apps.offerFirst')}</p>;
    }
    if (!app.available) {
        return <p className="text-sm text-warning">{t('apps.notOffered', { grantor: app.grantor ?? '' })}</p>;
    }
    if (!req) return null;
    if (req.state === 'ok') return <p className="text-sm text-success">{t('apps.ready')}</p>;
    // alpha.118: a check without an answer says why (busy server, refused check).
    if (req.state === 'unknown') {
        const key = req.reason === 'busy' ? 'apps.requirementsBusy' : req.reason === 'refused' ? 'apps.requirementsRefused' : 'apps.requirementsUnknown';
        return <p className="text-sm text-fg-muted">{t(key)}</p>;
    }
    const packages = req.missing.filter((m) => m.package).map((m) => m.package);
    const enable = req.missing.filter((m) => !m.package).map((m) => m.extension);
    return (
        <ul className="list-disc space-y-0.5 pl-5 text-sm text-warning" data-unmet="">
            {!req.php_ok && <li>{req.php_reason || t('apps.phpUnsupported', { range: app.php })} — {t('apps.phpAction')}</li>}
            {packages.length > 0 && <li>{t('apps.missingPackages', { packages: packages.join(' ') })}</li>}
            {/* The administrator's path in the panel (alpha.118): Components
                installs the site's PHP version with its standard extensions
                from the official packages. */}
            {packages.length > 0 && administrator && req.install_package && (
                <li className="my-1 list-none text-fg-muted" data-install-extensions={req.install_package}>
                    <Button icon={Download} onClick={() => navigate(phpExtensionsInstallPath(req.install_package!))}>
                        {t('apps.installExtensions', { php: req.php_line ?? '' })}
                    </Button>
                    <span className="block text-xs">{t('apps.installExtensionsHint', { php: req.php_line ?? '' })}</span>
                </li>
            )}
            {enable.length > 0 && <li>{t('apps.enableExtensions', { extensions: enable.join(', ') })}</li>}
            {(req.doc_root === 'content' || req.doc_root === 'unavailable') && <li>{t('apps.docRootNotEmpty')}</li>}
            {!req.database_ok && <li>{req.database_reason}</li>}
            {!req.disk_ok && <li>{req.disk_reason}</li>}
        </ul>
    );
}

function AppCard({ app, occupied, administrator, installing, disabled, onInstall }: { app: DomainApp; occupied: boolean; administrator: boolean; installing: boolean; disabled: boolean; onInstall: () => void }) {
    const { t } = useI18n();
    const [open, setOpen] = useState(false);
    const ready = app.available && !occupied && app.requirements?.state !== 'unmet';
    return (
        <div className="flex flex-col gap-2 rounded-xl border border-border bg-surface p-4" data-catalog-app={app.id}>
            <div className="flex items-start gap-3">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><LayoutGrid className="h-5 w-5" /></span>
                <div className="min-w-0 flex-1">
                    <div className="font-semibold text-fg">{app.name} <span className="text-xs font-normal text-fg-muted">{app.version}</span></div>
                    <p className="text-xs text-fg-muted">{app.description}</p>
                </div>
            </div>
            {occupied ? <p className="text-sm text-fg-muted">{t('apps.siteOccupied')}</p> : <Requirements app={app} administrator={administrator} />}
            <button type="button" onClick={() => setOpen(!open)} className="inline-flex items-center gap-1 self-start text-xs font-medium text-primary">
                {open ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}{t('apps.details')}
            </button>
            {open && (
                <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs text-fg-muted">
                    <dt>{t('apps.licence')}</dt><dd className="break-words">{app.license}</dd>
                    <dt>PHP</dt><dd>{app.php || '—'}</dd>
                    <dt>{t('apps.extensions')}</dt><dd className="break-words">{app.extensions.join(', ') || '—'}</dd>
                    <dt>{t('apps.database')}</dt><dd>{app.databases.join(', ') || '—'}</dd>
                    <dt>{t('apps.disk')}</dt><dd>{app.disk_mb} MB</dd>
                    <dt>{t('apps.source')}</dt><dd className="break-all">{app.source}</dd>
                    <dt>{t('apps.verified')}</dt><dd className="break-words">{app.pinned ? t('apps.pinnedSha') : ''} {app.checked}</dd>
                    <dt>{t('apps.risk')}</dt><dd className="break-words">{app.risk}</dd>
                </dl>
            )}
            <div>
                <Button variant="primary" icon={installing ? undefined : Download} disabled={disabled || !ready} onClick={onInstall}>
                    {installing ? (<><Loader2 className="h-4 w-4 animate-spin" />{t('apps.installing')}</>) : t('apps.install')}
                </Button>
            </div>
        </div>
    );
}

function RemoveForm({ domainId, domainName, app, send, onClose }: {
    domainId: number; domainName: string; app: InstalledApp;
    send: (url: string, body: unknown, method?: string) => Promise<Record<string, unknown> | null>; onClose: () => void;
}) {
    const { t } = useI18n();
    const [confirmName, setConfirmName] = useState('');
    const [database, setDatabase] = useState(false);
    const [data, setData] = useState(false);
    const [sending, setSending] = useState(false);
    const remove = async () => {
        if (sending) return;
        setSending(true);
        try {
            const result = await send(domainAppOperationURL(domainId, app.operation_id), { confirm: confirmName, remove_database: database, remove_data: data }, 'DELETE');
            if (result?.success === true) {
                const note = typeof result.database_note === 'string' && result.database_note ? result.database_note : '';
                // Items the owner added after the removal started were kept
                // (alpha.118); the removal is finished and says so.
                const kept = Array.isArray(result.kept_entries) ? result.kept_entries.filter((x): x is string => typeof x === 'string') : [];
                if (kept.length > 0) {
                    showToast('warning', t('apps.removedKept', { name: app.name, count: kept.length, items: kept.slice(0, 10).join(', ') + (kept.length > 10 ? ', …' : '') }));
                }
                showToast(note ? 'error' : 'success', note || t('apps.removed', { name: app.name }));
                onClose();
            }
        } finally {
            setSending(false);
        }
    };
    return (
        <div className="mb-4 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm" data-remove-app={app.app_id}>
            <p className="font-semibold text-fg">{t('apps.removeTitle', { name: app.name })}</p>
            <p className="mt-1 text-fg-muted">{t('apps.removeExplain')}</p>
            <label className="mt-2 flex items-start gap-2"><input type="checkbox" checked={database} onChange={(e) => setDatabase(e.target.checked)} className="mt-0.5 h-4 w-4 accent-primary" />
                <span>{t('apps.removeDatabase', { database: app.database_name ?? '' })}</span></label>
            {app.data_dir && (
                <label className="mt-1 flex items-start gap-2"><input type="checkbox" checked={data} disabled={!database} onChange={(e) => setData(e.target.checked)} className="mt-0.5 h-4 w-4 accent-primary" />
                    <span>{t('apps.removeData', { folder: app.data_dir })}</span></label>
            )}
            <label className="mt-2 block text-fg-muted">{t('apps.typeDomain', { domain: domainName })}
                <input type="text" value={confirmName} onChange={(e) => setConfirmName(e.target.value)} className="mt-1 block w-full max-w-sm rounded-lg border border-border bg-surface px-2 py-1.5 text-fg" />
            </label>
            <div className="mt-3 flex flex-wrap gap-2">
                <Button variant="danger" icon={Trash2} disabled={sending || confirmName.trim().toLowerCase() !== domainName.toLowerCase()} onClick={() => void remove()}>{t('apps.removeConfirm')}</Button>
                <Button disabled={sending} onClick={onClose}>{t('common.cancel')}</Button>
            </div>
        </div>
    );
}
