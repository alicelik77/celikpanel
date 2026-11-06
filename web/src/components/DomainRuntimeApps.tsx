import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Cpu, ExternalLink, KeyRound, Loader2, Play, Plus, RefreshCw, Rocket, RotateCw, ScrollText, Settings2, Square, Trash2, X } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, ErrorBanner, Field, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { LOADING, invalidate, lastKnown, readRemote, useRefreshEvery, useRereadWhileRechecking, type Remote, type RemoteHandle } from '../lib/remote';
import { newOperationID } from '../lib/serviceApps';
import {
    RUNTIME_DETAIL_REASONS,
    RUNTIME_STATE_GUIDANCE,
    argsFromText,
    argsValid,
    callableValid,
    decodeLogLines,
    decodeRuntimeApps,
    entryValid,
    envKeyProblem,
    healthPathValid,
    maxWorkers,
    parseRequirementsRefusal,
    type RuntimeApp,
    type RuntimeAppsView,
    type RuntimeKindView,
    type RuntimeOperation,
} from '../lib/runtimeApps';
import { RuntimeAppRisk } from './RuntimeAppRisk';

// The domain's "Programs" tab (alpha.119, docs/RUNTIME-APPS.md): a program the
// customer uploads (Go, Rust, any program for this server) or a Python web app
// (Django, Flask, FastAPI, built into its own environment from PyPI) runs as
// the site's own account in the site sandbox and opens over HTTPS at this
// domain. What stands in the way comes first, each with its reason, who acts, the next
// action and how work resumes (D-024); then the running or last operation;
// then the program with its actions, or the form that creates it.
//
// The server answers every read; this page never starts anything by reading.
// The first read and the person's own Refresh ask the program's health path
// once (`?health=1`); the reads every few seconds while an operation runs do
// not. A deploy or removal whose answer was lost is sent again with the same
// operation identity, so the server answers the same operation.
//
// Alan adının "Programlar" sekmesi. Önce engeller (neden, kim, sonraki adım,
// nasıl sürer), sonra işlem, sonra program. Sağlık yolu yalnız ilk okumada ve
// kişinin kendi Yenile'sinde bir kez sorulur; yoklamada asla.

async function sendJSON(url: string, method: string, body: unknown): Promise<Response | null> {
    try {
        return await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    } catch {
        return null;
    }
}

type Tone = 'warning' | 'danger' | 'info';
function Notice({ tone = 'warning', children }: { tone?: Tone; children: ReactNode }) {
    const palette = tone === 'danger' ? 'border-danger/30 bg-danger/10 text-danger' : tone === 'info' ? 'border-border bg-surface-2 text-fg-muted' : 'border-warning/30 bg-warning/10 text-warning';
    return <div className={`rounded-lg border px-3 py-2.5 text-sm ${palette}`}>{children}</div>;
}

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;
type After = (res: Response | null) => Promise<boolean>;

const toneOf = (cls: string | undefined): Tone => (cls === 'verified_failure' ? 'danger' : cls === 'prerequisite' ? 'warning' : 'info');

// The words of a state or of an operation's failure reason that is a state:
// the reason, who acts, the next action and how work resumes.
function Guidance({ code, vars }: { code: string; vars: Record<string, string | number> }) {
    const { t } = useI18n();
    const guide = RUNTIME_STATE_GUIDANCE[code];
    if (!guide) return <p>{t('runtime.state.other', { code })}</p>;
    return (
        <div className="space-y-1">
            <p>{t(('runtime.state.' + code) as TranslationKey, vars)}</p>
            <p><span className="font-medium">{t('runtime.label.who')}</span> {t(('runtime.who.' + guide.who) as TranslationKey)}</p>
            <p><span className="font-medium">{t('runtime.label.next')}</span> {t(('runtime.state.' + code + '.next') as TranslationKey, vars)}</p>
            <p><span className="font-medium">{t('runtime.label.resume')}</span> {t(('runtime.state.' + code + '.resume') as TranslationKey, vars)}</p>
        </div>
    );
}

function StateNotice({ code, vars }: { code: string; vars: Record<string, string | number> }) {
    const { t } = useI18n();
    const cls = RUNTIME_STATE_GUIDANCE[code]?.class ?? 'unknown';
    return (
        <Notice tone={toneOf(cls)}>
            <p className="mb-1 text-xs font-semibold uppercase tracking-wide">{t(('runtime.class.' + cls) as TranslationKey)}</p>
            <Guidance code={code} vars={vars} />
        </Notice>
    );
}

// A refused change: the screen's words, and the server's own sentence beside
// them when it carries the one detail that matters (which argument, which key).
function Refusal({ error }: { error: ApiError | null }) {
    const { t } = useI18n();
    if (!error) return null;
    const refined = 'err.RUNTIME_APP.' + (error.reason ?? '');
    const worded = error.code === 'RUNTIME_APP' && t(refined as TranslationKey) !== refined;
    const detail = error.code === 'RUNTIME_APP' && error.message && worded && RUNTIME_DETAIL_REASONS.has(error.reason ?? '');
    return (
        <div className="space-y-1">
            <ErrorBanner error={error} />
            {detail && <p className="text-xs text-fg-muted">{t('runtime.serverSaid', { message: error.message })}</p>}
        </div>
    );
}

export function DomainRuntimeApps({ domainId }: { domainId: number; domainName: string }) {
    const { t } = useI18n();
    const base = `/api/v1/domains/${domainId}/runtime-apps`;
    const [remote, setRemote] = useState<Remote<RuntimeAppsView>>(LOADING);
    const [reading, setReading] = useState(false);
    const [failure, setFailure] = useState<ApiError | null>(null);
    const [lost, setLost] = useState(false);
    const latest = useRef(remote);
    latest.current = remote;
    const run = useRef(0);

    // health: only the first read and the person's own Refresh ask the
    // program's health path; every other read (polling, after a change) does
    // not. A newer read replaces an older one still on its way.
    const load = useCallback(async (health: boolean): Promise<Remote<RuntimeAppsView>> => {
        const mine = ++run.current;
        setReading(true);
        const next = await readRemote(base + (health ? '?health=1' : ''), decodeRuntimeApps, latest.current);
        if (run.current === mine) {
            setRemote(next);
            setReading(false);
        }
        return next;
    }, [base]);
    const shownView = lastKnown(remote);
    useRereadWhileRechecking(shownView?.value.rechecking === true, shownView?.observedAt, () => void load(false));

    useEffect(() => {
        void load(true);
        return () => { run.current++; };
    }, [load]);

    const shown = lastKnown(remote)?.value;
    const running = shown?.operation?.state === 'running';
    // Polling only reads, never with the health check, and only while an
    // operation runs on the server.
    const poll: RemoteHandle<unknown> = { remote, reading, retry: () => load(false) };
    useRefreshEvery(poll, 4000, running);

    // An operation that finished changed the site's type: the domain list and
    // its tabs read again.
    const lastOp = useRef<string | undefined>(undefined);
    useEffect(() => {
        const now = shown?.operation ? shown.operation.id + ':' + shown.operation.state : undefined;
        if (lastOp.current && lastOp.current.endsWith(':running') && now !== lastOp.current) invalidate('domains');
        lastOp.current = now;
    }, [shown?.operation?.id, shown?.operation?.state]);

    const after: After = async (res) => {
        if (!res) {
            setLost(true);
            await load(false);
            return false;
        }
        setLost(false);
        if (!res.ok) {
            setFailure(await readApiError(res));
            await load(false);
            return false;
        }
        setFailure(null);
        await load(false);
        return true;
    };

    return (
        <div className="space-y-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                    <h3 className="flex items-center gap-2 text-base font-semibold text-fg"><Cpu className="h-5 w-5 text-primary" />{t('runtime.title')}</h3>
                    <p className="mt-1 text-sm text-fg-muted">{t('runtime.desc')}</p>
                </div>
                <Button icon={RefreshCw} loading={reading && !running} disabled={reading} title={t('runtime.refreshNote')} onClick={() => void load(true)}>
                    {t('runtime.refresh')}
                </Button>
            </div>
            {lost && <Notice tone="info">{t('runtime.lost')}</Notice>}
            <Refusal error={failure} />
            <RemoteGate remote={remote} checking={t('runtime.checking')} failed={t('runtime.unknown')} onRetry={() => void load(true)} busy={reading}>
                {(view) => <Body view={view.value} base={base} after={after} />}
            </RemoteGate>
        </div>
    );
}

function grantorWords(grantor: string | undefined, t: Translate): string {
    const provider = /^your provider \((.+)\)$/.exec(grantor ?? '');
    return provider ? t('rights.grantor.provider', { name: provider[1] }) : t('rights.grantor.administrator');
}

// What stands in the way of creating a program (no program yet), most
// blocking first; each says who acts. null when nothing does.
function blockerOf(view: RuntimeAppsView, kind: RuntimeKindView, t: Translate): { tone: Tone; text: string } | null {
    if (!view.has_site) return { tone: 'warning', text: t('runtime.block.noSite') };
    if (view.suspended) return { tone: 'warning', text: t('runtime.block.suspended') };
    // Installed before offered: the server's own Python with its venv module.
    if (kind.kind === 'python' && !kind.python?.ready) {
        return { tone: 'warning', text: t('runtime.python.missing', { detail: kind.python?.detail || '—' }) };
    }
    if (kind.held) return { tone: 'warning', text: t('runtime.block.held') };
    if (!kind.permission_granted) return { tone: 'warning', text: t('runtime.block.notGranted', { grantor: grantorWords(kind.grantor, t) }) };
    if (!kind.offered) {
        const layer = kind.refused_by ?? 'administrator';
        const key = ('runtime.closed.' + layer) as TranslationKey;
        const said = t(key, { grantor: grantorWords(kind.grantor, t) });
        return { tone: 'warning', text: said === key ? t('runtime.closed.administrator') : said };
    }
    if (!view.sandbox.known) return { tone: 'info', text: t('runtime.block.levelUnknown') };
    if (view.sandbox.level === 'reduced' && (kind.accepted_level !== 'reduced' || view.sandbox.gaps.some((gap) => !kind.accepted_gaps.includes(gap)))) {
        return { tone: 'warning', text: t('runtime.block.sandboxReduced', { gaps: view.sandbox.gaps.join(', ') }) };
    }
    if (view.disk_full) return { tone: 'warning', text: t('runtime.block.diskFull') };
    return null;
}

function Body({ view, base, after }: { view: RuntimeAppsView; base: string; after: After }) {
    const { t } = useI18n();
    const kinds = view.kinds.filter((k) => k.implemented);
    const app = view.app;
    // Before an app exists the person chooses what runs here; an offered
    // kind is chosen first.
    const [choice, setChoice] = useState(() => (kinds.find((k) => k.offered) ?? kinds[0])?.kind ?? 'program');
    const kind = kinds.find((k) => k.kind === (app ? app.kind : choice)) ?? kinds[0];
    if (!kind) return <Notice tone="info">{t('runtime.kindUnavailable')}</Notice>;
    const blocker = app ? null : blockerOf(view, kind, t);
    const python = kind.kind === 'python';
    const vars = {
        domain: view.domain, entry: app?.entry ?? '', port: app?.port ?? '', path: app?.health_path ?? '/', dir: python ? 'app/src' : 'app/bin',
        gaps: view.sandbox.gaps.join(', '), detail: app?.status?.entry_check?.detail ?? '', format: app?.status?.entry_check?.format ?? '',
        placeholder: '{port}', minor: kind.python?.minor ?? '',
    };
    const states = app ? app.states : [];
    return (
        <div className="space-y-4">
            {states.length > 0 && (
                <div className="space-y-2" data-runtime-states="">
                    {states.map((code) => <StateNotice key={code} code={code} vars={vars} />)}
                </div>
            )}
            {blocker && <Notice tone={blocker.tone}>{blocker.text}</Notice>}
            {!app && !blocker && !view.certificate && <Notice tone="info">{t('runtime.block.noCertificate', { domain: view.domain })}</Notice>}
            {view.operation && <OperationLine op={view.operation} vars={vars} />}
            {!app && kinds.length > 1 && (
                <fieldset className="space-y-1 text-sm" data-runtime-kind-choice="">
                    <legend className="mb-1 font-medium text-fg">{t('runtime.kind.choose')}</legend>
                    {kinds.map((k) => (
                        <label key={k.kind} className="flex items-start gap-2 text-fg">
                            <input type="radio" name="runtime-kind" className="mt-0.5 h-4 w-4 accent-primary" checked={kind.kind === k.kind} onChange={() => setChoice(k.kind)} />
                            {t(('runtime.kind.' + k.kind) as TranslationKey)}
                        </label>
                    ))}
                </fieldset>
            )}
            {python && kind.python?.ready && (
                <p className="text-sm text-fg-muted">
                    {t('runtime.python.version', { minor: kind.python.minor ?? '' })}{' '}
                    {t(kind.python.source_builds ? 'runtime.python.sourceBuilds' : 'runtime.python.wheelsOnly', { minor: kind.python.minor ?? '' })}
                </p>
            )}
            {app ? (
                <AppCard view={view} app={app} kind={kind} base={base} after={after} />
            ) : !blocker ? (
                <div className="space-y-3 rounded-xl border border-border bg-surface p-4">
                    <div className="font-semibold text-fg">{t(python ? 'runtime.python.create' : 'runtime.create.title')}</div>
                    <UploadHelp vars={vars} folder="" python={python} />
                    <SettingsForm key={kind.kind} base={base} mode="create" kind={kind} after={after} />
                </div>
            ) : null}
            <details className="rounded-xl border border-border bg-surface p-4" open={!app}>
                <summary className="cursor-pointer text-sm font-medium text-primary">{t('runtime.risk.title')}</summary>
                <div className="mt-3"><RuntimeAppRisk risk={kind.risk} limits={kind.limits} sandbox={view.sandbox} /></div>
            </details>
        </div>
    );
}

function UploadHelp({ vars, folder, python }: { vars: Record<string, string | number>; folder: string; python: boolean }) {
    const { t } = useI18n();
    const prefix = python ? 'runtime.upload.python.' : 'runtime.upload.';
    return (
        <div className="space-y-1 text-sm text-fg-muted">
            <div className="font-medium text-fg">{t(python ? 'runtime.upload.python.title' : 'runtime.upload.title')}</div>
            <ol className="list-decimal space-y-1 pl-5">
                {[1, 2, 3, 4, 5].map((n) => <li key={n}>{t((prefix + 'step' + n) as TranslationKey, vars)}</li>)}
            </ol>
            {folder && <p className="text-xs text-fg-subtle">{t('runtime.upload.folder', { folder })}</p>}
        </div>
    );
}

function OperationLine({ op, vars }: { op: RuntimeOperation; vars: Record<string, string | number> }) {
    const { t } = useI18n();
    const kind = t(('runtime.op.kind.' + op.kind) as TranslationKey);
    if (op.state === 'running') {
        return (
            <Notice tone="info">
                <span className="flex items-start gap-2">
                    <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin" />
                    {t('runtime.op.running', { kind, step: t(('runtime.op.step.' + op.step) as TranslationKey) })}
                </span>
            </Notice>
        );
    }
    if (op.state !== 'failed') return null;
    const reason = op.reason ?? '';
    const reasonKey = ('runtime.reason.' + reason) as TranslationKey;
    const reasonText = t(reasonKey, vars);
    // A refused requirements line: the file, the line and why, in the
    // person's language; the line itself as the server read it.
    const refused = reason === 'requirements_refused' ? parseRequirementsRefusal(op.message) : null;
    return (
        <Notice tone={toneOf(op.class)}>
            <p className="font-medium">{t(('runtime.op.failed.' + (op.class || 'unknown')) as TranslationKey, { kind, step: t(('runtime.op.step.' + op.step) as TranslationKey) })}</p>
            <div className="mt-1">
                {RUNTIME_STATE_GUIDANCE[reason] ? <Guidance code={reason} vars={vars} />
                    : reasonText !== reasonKey ? <p>{reasonText}</p>
                    : <p>{t('runtime.reason.other')}</p>}
            </div>
            {refused && (
                <p className="mt-1 font-medium" data-runtime-req-line="">
                    {t('runtime.req.line', { file: refused.file, line: refused.line, why: t(('runtime.req.' + refused.code) as TranslationKey) })}
                </p>
            )}
            {op.message && <p className="mt-1 text-xs opacity-80">{t(refused ? 'runtime.req.text' : 'runtime.serverSaid', { message: refused ? refused.text : op.message })}</p>}
        </Notice>
    );
}

// States that keep a deploy, a start or a restart from being accepted.
const BLOCKS_NEW_USE = ['suspended', 'held', 'not_offered', 'sandbox_reduced', 'disk_full'];

function AppCard({ view, app, kind, base, after }: { view: RuntimeAppsView; app: RuntimeApp; kind: RuntimeKindView; base: string; after: After }) {
    const { t } = useI18n();
    const busy = view.operation?.state === 'running';
    const [acting, setActing] = useState('');
    const [deployID, setDeployID] = useState(newOperationID);
    const [logs, setLogs] = useState<string[] | null>(null);
    const [logsFailure, setLogsFailure] = useState<ApiError | null>(null);
    const [editing, setEditing] = useState(false);
    const [envOpen, setEnvOpen] = useState(false);
    const [removeOpen, setRemoveOpen] = useState(false);

    const blocked = app.states.some((code) => BLOCKS_NEW_USE.includes(code));
    const deployBlocked = blocked || !view.certificate;
    const startBlocked = blocked || !app.deployed;
    const status = app.status;
    const active = status?.unit.active;

    const control = async (action: 'start' | 'stop' | 'restart') => {
        setActing(action);
        const res = await sendJSON(base + '/control', 'POST', { action });
        setActing('');
        if (!(await after(res))) return;
        // Stopping an app that was never deployed changes nothing: say why
        // and what starts it, instead of a bare "Done".
        if (action === 'stop' && !app.deployed) showToast('info', t('runtime.stopNotDeployed'));
        else showToast('success', t('runtime.actionDone'));
    };
    const deploy = async () => {
        setActing('deploy');
        const res = await sendJSON(base + '/deploy', 'POST', { operation_id: deployID });
        setActing('');
        // A lost answer keeps the identity: pressing again is answered with
        // the same operation. Any answer that arrived ends this identity.
        if (res) setDeployID(newOperationID());
        await after(res);
    };
    const readLogs = async () => {
        setActing('logs');
        setLogsFailure(null);
        const read = await readRemote(`${base}/logs?lines=200`, decodeLogLines);
        setActing('');
        if (read.state === 'known') setLogs(read.value);
        else setLogsFailure(read.reason);
    };

    const stateKey = app.status_unknown ? 'runtime.app.state.unknown' : ('runtime.app.state.' + app.state) as TranslationKey;
    return (
        <div className="space-y-3 rounded-xl border border-border bg-surface p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="break-all font-semibold text-fg">{app.python ? t('runtime.python.app', { callable: app.python.callable }) : t('runtime.app.entry', { entry: app.entry })}</div>
                    <div className="text-sm text-fg-muted">
                        {t(stateKey)}
                        {status?.memory_mb ? ' · ' + t('runtime.app.memory', { mb: status.memory_mb }) : ''}
                        {status?.restarts ? ' · ' + t('runtime.app.restarts', { count: status.restarts }) : ''}
                    </div>
                </div>
                {app.deployed && active && status?.serving && (
                    <a href={app.url} target="_blank" rel="noopener noreferrer"
                        className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2">
                        <ExternalLink className="h-4 w-4" />{t('runtime.app.open')}
                    </a>
                )}
            </div>
            <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
                <dt className="text-fg-subtle">{t('runtime.app.portLabel')}</dt>
                <dd className="text-fg">{t('runtime.app.port', { port: app.port })}</dd>
                {app.python ? (
                    <>
                        <dt className="text-fg-subtle">{t('runtime.python.server')}</dt>
                        <dd className="text-fg">{t(('runtime.python.server.' + app.python.server) as TranslationKey)} · {t('runtime.python.workersCount', { count: app.python.workers })}</dd>
                    </>
                ) : (
                    <>
                        <dt className="text-fg-subtle">{t('runtime.args')}</dt>
                        <dd className="break-all font-mono text-xs text-fg">{app.args.length ? app.args.join(' ') : t('runtime.app.noArgs')}</dd>
                    </>
                )}
                <dt className="text-fg-subtle">{t('runtime.health')}</dt>
                <dd className="text-fg">
                    {app.health_path}
                    {status?.health.checked && (status.health.ok
                        ? ' · ' + t('runtime.app.healthOk', { status: status.health.status ?? '' })
                        : ' · ' + t('runtime.app.healthBad', { status: status.health.status ?? '—' }))}
                </dd>
            </dl>
            {!app.deployed && <Notice tone="info">{t(app.python ? 'runtime.python.notDeployed' : 'runtime.app.notDeployed')}</Notice>}
            <div className="flex flex-wrap gap-2">
                <Button variant="primary" icon={Rocket} loading={acting === 'deploy'} disabled={busy || !!acting || deployBlocked} onClick={() => void deploy()}>
                    {t(app.deployed ? 'runtime.redeploy' : 'runtime.deploy')}
                </Button>
                {app.deployed && active !== true && (
                    <Button icon={Play} loading={acting === 'start'} disabled={busy || !!acting || startBlocked} onClick={() => void control('start')}>{t('runtime.start')}</Button>
                )}
                {app.deployed && active !== false && (
                    <>
                        <Button icon={Square} loading={acting === 'stop'} disabled={busy || !!acting} onClick={() => void control('stop')}>{t('runtime.stop')}</Button>
                        <Button icon={RotateCw} loading={acting === 'restart'} disabled={busy || !!acting || startBlocked} onClick={() => void control('restart')}>{t('runtime.restart')}</Button>
                    </>
                )}
                <Button icon={ScrollText} loading={acting === 'logs'} disabled={!!acting} onClick={() => void readLogs()}>{t('runtime.logs')}</Button>
                <Button icon={KeyRound} disabled={busy || !!acting} onClick={() => setEnvOpen(true)}>{t('runtime.env')}</Button>
                <Button icon={Settings2} disabled={busy || !!acting} onClick={() => setEditing((on) => !on)}>{t('runtime.settings.edit')}</Button>
                <Button variant="danger" icon={Trash2} disabled={busy || !!acting} onClick={() => setRemoveOpen(true)}>{t('runtime.remove')}</Button>
            </div>
            {deployBlocked && !busy && <p className="text-xs text-fg-subtle">{t('runtime.deployBlocked')}</p>}
            {editing && (
                <div className="rounded-lg border border-border bg-surface-2 p-3">
                    <SettingsForm base={base} mode="edit" kind={kind} app={app} after={after} onDone={() => setEditing(false)} />
                </div>
            )}
            <details className="text-sm" open={!app.deployed}>
                <summary className="cursor-pointer text-primary">{t(app.python ? 'runtime.upload.python.title' : 'runtime.upload.title')}</summary>
                <div className="mt-2">
                    <UploadHelp vars={{ dir: app.upload_dir, port: app.port, placeholder: '{port}' }} folder={app.folder} python={!!app.python} />
                </div>
            </details>
            <p className="text-xs text-fg-subtle">{t('runtime.independent', { unit: app.unit, folder: app.folder })}</p>
            <ErrorBanner error={logsFailure} />
            {logs && (
                <div>
                    <div className="mb-1 text-xs font-medium text-fg-muted">{t('runtime.logs.title')}</div>
                    <pre className="max-h-80 overflow-auto rounded-lg border border-border bg-surface-2 p-3 text-xs text-fg">{logs.length ? logs.join('\n') : t('runtime.logs.empty')}</pre>
                </div>
            )}
            {envOpen && <EnvDialog app={app} base={base} after={after} onClose={() => setEnvOpen(false)} />}
            {removeOpen && <RemoveDialog domain={view.domain} base={base} after={after} onClose={() => setRemoveOpen(false)} />}
        </div>
    );
}

function SettingsForm({ base, mode, kind, app, after, onDone }: { base: string; mode: 'create' | 'edit'; kind: RuntimeKindView; app?: RuntimeApp; after: After; onDone?: () => void }) {
    const { t } = useI18n();
    const python = kind.kind === 'python';
    const [entry, setEntry] = useState(app?.python?.callable ?? app?.entry ?? '');
    const [argsText, setArgsText] = useState((app?.args ?? []).join('\n'));
    const [health, setHealth] = useState(app?.health_path ?? '/');
    const [server, setServer] = useState<'wsgi' | 'asgi'>(app?.python?.server ?? 'wsgi');
    const most = kind.python?.max_workers ?? maxWorkers(kind.limits.memory_mb);
    const [workers, setWorkers] = useState(String(app?.python?.workers ?? Math.min(2, most)));
    const [busy, setBusy] = useState(false);
    const args = argsFromText(argsText);
    const entryOK = python ? callableValid(entry) : entryValid(entry);
    const argsOK = python || argsValid(args);
    const workersOK = !python || (/^\d+$/.test(workers) && Number(workers) >= 1 && Number(workers) <= most);
    const healthOK = healthPathValid(health.trim());
    const prefix = 'runtime-' + mode;

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!entryOK || !argsOK || !healthOK || !workersOK) return;
        setBusy(true);
        const body = python
            ? { kind: 'python', entry: entry.trim(), server, workers: Number(workers), health_path: health.trim() || '/' }
            : { kind: 'program', entry: entry.trim(), args, health_path: health.trim() || '/' };
        const res = await sendJSON(base + (mode === 'create' ? '/create' : '/settings'), mode === 'create' ? 'POST' : 'PUT', body);
        setBusy(false);
        if (await after(res)) {
            showToast('success', t(mode === 'create' ? 'runtime.create.done' : 'runtime.settings.saved'));
            onDone?.();
        }
    };

    return (
        <form className="space-y-3" onSubmit={(event) => void submit(event)} noValidate>
            {mode === 'edit' && <div className="text-sm font-medium text-fg">{t('runtime.settings.title')}</div>}
            {python && (
                <Field label={t('runtime.python.server')} htmlFor={prefix + '-server'}>
                    <select id={prefix + '-server'} className={inputClass} value={server} onChange={(e) => setServer(e.target.value === 'asgi' ? 'asgi' : 'wsgi')}>
                        <option value="wsgi">{t('runtime.python.server.wsgi')}</option>
                        <option value="asgi">{t('runtime.python.server.asgi')}</option>
                    </select>
                </Field>
            )}
            <Field label={t(python ? 'runtime.python.callable' : 'runtime.entry')} htmlFor={prefix + '-entry'} hint={t(python ? 'runtime.python.callable.hint' : 'runtime.entry.hint')}>
                <input id={prefix + '-entry'} className={inputClass} autoComplete="off" spellCheck={false} value={entry} onChange={(e) => setEntry(e.target.value)} />
            </Field>
            {entry !== '' && !entryOK && <p className="text-xs text-warning">{t(python ? 'err.RUNTIME_APP.python_settings' : 'err.RUNTIME_APP.entry_invalid', { max: most })}</p>}
            {python ? (
                <Field label={t('runtime.python.workers')} htmlFor={prefix + '-workers'} hint={t('runtime.python.workers.hint', { max: most })}>
                    <input id={prefix + '-workers'} inputMode="numeric" className={inputClass} value={workers} onChange={(e) => setWorkers(e.target.value)} />
                </Field>
            ) : (
                <Field label={t('runtime.args')} htmlFor={prefix + '-args'} hint={t('runtime.args.hint', { placeholder: '{port}' })}>
                    <textarea id={prefix + '-args'} rows={3} className={inputClass + ' font-mono'} spellCheck={false} value={argsText} onChange={(e) => setArgsText(e.target.value)} />
                </Field>
            )}
            {!argsOK && <p className="text-xs text-warning">{t('err.RUNTIME_APP.args_invalid')}</p>}
            {!workersOK && <p className="text-xs text-warning">{t('runtime.python.workers.hint', { max: most })}</p>}
            <Field label={t('runtime.health')} htmlFor={prefix + '-health'} hint={t('runtime.health.hint')}>
                <input id={prefix + '-health'} className={inputClass} autoComplete="off" spellCheck={false} value={health} onChange={(e) => setHealth(e.target.value)} />
            </Field>
            {!healthOK && <p className="text-xs text-warning">{t('err.RUNTIME_APP.health_path_invalid')}</p>}
            {mode === 'edit' && <p className="text-xs text-fg-subtle">{t('runtime.settings.applies')}</p>}
            <div className="flex flex-wrap gap-2">
                <Button type="submit" variant="primary" icon={mode === 'create' ? Plus : undefined} loading={busy} disabled={!entryOK || !argsOK || !healthOK || !workersOK}>
                    {t(mode === 'create' ? 'runtime.create.button' : 'runtime.settings.save')}
                </Button>
                {onDone && <Button type="button" onClick={onDone} disabled={busy}>{t('common.cancel')}</Button>}
            </div>
        </form>
    );
}

interface EnvRow {
    id: number;
    key: string;
    /** Typed in this dialog only. A secret already on the server starts empty: its value is never sent to the screen. */
    value: string;
    secret: boolean;
    /** The key whose secret value the server holds, when this row started as one. */
    storedSecret: string;
}

// The environment, as one list: saving sends the full list and the server
// removes what is not in it. A secret already set shows only that it is set;
// leaving its field empty keeps the value on the server.
function EnvDialog({ app, base, after, onClose }: { app: RuntimeApp; base: string; after: After; onClose: () => void }) {
    const { t } = useI18n();
    const nextID = useRef(app.env.length);
    const [rows, setRows] = useState<EnvRow[]>(() => app.env.map((entry, i) => ({
        id: i, key: entry.key, value: entry.secret ? '' : (entry.value ?? ''), secret: entry.secret, storedSecret: entry.secret ? entry.key : '',
    })));
    const [busy, setBusy] = useState(false);
    const keeps = (row: EnvRow) => row.secret && row.storedSecret !== '' && row.storedSecret === row.key && row.value === '';
    const problems = rows.map((row) => {
        const key = row.key.trim();
        const why = envKeyProblem(key);
        if (why) return t(('runtime.env.problem.' + why) as TranslationKey, { key: key || '—' });
        if (rows.filter((other) => other.key.trim() === key).length > 1) return t('runtime.env.problem.twice', { key });
        if (!keeps(row) && (row.value === '' || row.value.length > 4096 || /[\u0000-\u001f\u007f]/.test(row.value))) return t('runtime.env.problem.value', { key });
        return '';
    });
    const ready = rows.length <= 64 && problems.every((p) => p === '');
    const update = (id: number, change: Partial<EnvRow>) => setRows((list) => list.map((row) => (row.id === id ? { ...row, ...change } : row)));

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!ready) return;
        setBusy(true);
        const entries = rows.map((row) => {
            const key = row.key.trim();
            if (row.secret) return row.value === '' ? { key, secret: true } : { key, value: row.value, secret: true };
            return { key, value: row.value, secret: false };
        });
        let res: Response | null = null;
        try {
            res = await fetch(base + '/env', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ entries }) });
        } catch {
            res = null;
        }
        setBusy(false);
        if (await after(res)) {
            showToast('success', t('runtime.env.saved'));
            onClose();
        }
    };

    return (
        <Dialog id="runtime-env" title={t('runtime.env.title')} icon={KeyRound} width="xl" busy={busy} onDismiss={onClose} onSubmit={(event) => void submit(event)} noValidate
            actions={<><Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button><Button type="submit" variant="primary" loading={busy} disabled={!ready}>{t('runtime.env.save')}</Button></>}>
            <div className="space-y-3 text-sm">
                <p className="text-fg-muted">{t('runtime.env.hint')}</p>
                {rows.length === 0 && <p className="text-fg-subtle">{t('runtime.env.empty')}</p>}
                {rows.map((row, i) => (
                    <div key={row.id} className="space-y-1 rounded-lg border border-border p-2" data-runtime-env-row={row.key}>
                        <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_auto]">
                            <input aria-label={t('runtime.env.key')} placeholder={t('runtime.env.key')} className={inputClass + ' font-mono'} autoComplete="off" spellCheck={false}
                                value={row.key} onChange={(e) => update(row.id, { key: e.target.value.toUpperCase() })} />
                            {row.secret ? (
                                <input aria-label={t('runtime.env.value')} type="password" autoComplete="new-password" className={inputClass}
                                    placeholder={keeps(row) || (row.storedSecret !== '' && row.storedSecret === row.key) ? t('runtime.env.secretReplace') : t('runtime.env.value')}
                                    value={row.value} onChange={(e) => update(row.id, { value: e.target.value })} />
                            ) : (
                                <input aria-label={t('runtime.env.value')} placeholder={t('runtime.env.value')} className={inputClass} autoComplete="off" spellCheck={false}
                                    value={row.value} onChange={(e) => update(row.id, { value: e.target.value })} />
                            )}
                            <Button type="button" icon={X} aria-label={t('runtime.env.removeRow', { key: row.key || '—' })} onClick={() => setRows((list) => list.filter((other) => other.id !== row.id))} />
                        </div>
                        <div className="flex flex-wrap items-center gap-3 text-xs text-fg-muted">
                            <label className="flex items-center gap-1.5">
                                <input type="checkbox" className="h-3.5 w-3.5 accent-primary" checked={row.secret}
                                    onChange={(e) => update(row.id, { secret: e.target.checked, value: e.target.checked ? row.value : (keeps(row) ? '' : row.value) })} />
                                {t('runtime.env.secret')}
                            </label>
                            {keeps(row) && <span>{t('runtime.env.secretSet')}</span>}
                            {problems[i] && <span className="text-warning">{problems[i]}</span>}
                        </div>
                    </div>
                ))}
                <Button type="button" icon={Plus} disabled={rows.length >= 64}
                    onClick={() => setRows((list) => [...list, { id: nextID.current++, key: '', value: '', secret: false, storedSecret: '' }])}>
                    {t('runtime.env.add')}
                </Button>
                {app.deployed && <p className="text-xs text-fg-subtle">{t('runtime.env.restart')}</p>}
            </div>
        </Dialog>
    );
}

function RemoveDialog({ domain, base, after, onClose }: { domain: string; base: string; after: After; onClose: () => void }) {
    const { t } = useI18n();
    const [removeFiles, setRemoveFiles] = useState(false);
    const [typed, setTyped] = useState('');
    const [busy, setBusy] = useState(false);
    // One identity for this removal: a repeat after a lost answer is the same.
    const [operationID] = useState(newOperationID);
    const ready = !removeFiles || typed.trim().toLowerCase() === domain.toLowerCase();
    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!ready) return;
        setBusy(true);
        const res = await sendJSON(base + '/remove', 'POST', { operation_id: operationID, remove_files: removeFiles, confirm_domain: removeFiles ? typed.trim() : '' });
        setBusy(false);
        if (await after(res)) onClose();
    };
    return (
        <Dialog id="runtime-remove" tone="danger" title={t('runtime.remove.title', { domain })} icon={Trash2} busy={busy} onDismiss={onClose} onSubmit={(event) => void submit(event)} noValidate
            actions={<><Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button><Button type="submit" variant="danger" loading={busy} disabled={!ready}>{t('runtime.remove')}</Button></>}>
            <div className="space-y-3 text-sm">
                <p className="text-fg-muted">{t('runtime.remove.keep')}</p>
                <label className="flex items-start gap-2 text-fg">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={removeFiles} onChange={(e) => setRemoveFiles(e.target.checked)} />
                    {t('runtime.remove.files')}
                </label>
                {removeFiles && (
                    <Field label={t('runtime.remove.confirm', { domain })} htmlFor="runtime-confirm-domain">
                        <input id="runtime-confirm-domain" className={inputClass} autoComplete="off" value={typed} onChange={(e) => setTyped(e.target.value)} />
                    </Field>
                )}
            </div>
        </Dialog>
    );
}
