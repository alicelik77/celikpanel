import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Cpu, ExternalLink, KeyRound, Loader2, Play, Plus, RefreshCw, Rocket, RotateCw, ScrollText, Settings2, Square, Trash2, X } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, ErrorBanner, Field, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { LOADING, invalidate, lastKnown, readRemote, useRefreshEvery, useRereadWhileRechecking, type Remote, type RemoteHandle } from '../lib/remote';
import { newOperationID } from '../lib/serviceApps';
import { confirmAction } from '../lib/confirmAction';
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
import { PasswordInput } from './PasswordInput';
import { AdminInstallLink } from './AdminInstallLink';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, changedRowClass } from './kit/save/SaveBar';
import { confirmLeaveIfDirty, useLeaveGuard } from './kit/save/leaveGuard';
import { useSaveWords } from './kit/save/words';
import { answerWasLost } from '../lib/requestIdentity';

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
// alpha.127: where the administrator acts on a state that names them, and the
// page that holds the action. Anyone else keeps the state's own words.
// Yöneticinin bir durumda nerede işlem yaptığı.
const CATALOG = '/addons?view=catalog';
const ADMIN_NEXT: Record<string, { to: string; label: TranslationKey }> = {
    held: { to: CATALOG, label: 'adminNext.openCatalog' },
    sandbox_reduced: { to: CATALOG, label: 'adminNext.openCatalog' },
    not_offered: { to: CATALOG, label: 'adminNext.openCatalog' },
    suspended: { to: '/users', label: 'adminNext.openUsers' },
    oom: { to: CATALOG, label: 'adminNext.openCatalog' },
    wheel_unavailable: { to: CATALOG, label: 'adminNext.openCatalog' },
};
// States whose "who acts" names the administrator: said as "you" to them.
const ADMIN_WHO = new Set(['administrator', 'grantor', 'youThenAdministrator']);

function Guidance({ code, vars }: { code: string; vars: Record<string, string | number> }) {
    const { t } = useI18n();
    const guide = RUNTIME_STATE_GUIDANCE[code];
    if (!guide) return <p>{t('runtime.state.other', { code })}</p>;
    const who = t(('runtime.who.' + guide.who) as TranslationKey);
    const next = t(('runtime.state.' + code + '.next') as TranslationKey, vars);
    const step = ADMIN_NEXT[code];
    return (
        <div className="space-y-1">
            <p>{t(('runtime.state.' + code) as TranslationKey, vars)}</p>
            <p><span className="font-medium">{t('runtime.label.who')}</span> {ADMIN_WHO.has(guide.who) ? <AdminInstallLink admin={t('adminNext.you')} fallback={who} /> : who}</p>
            <p><span className="font-medium">{t('runtime.label.next')}</span> {step
                ? <AdminInstallLink to={step.to} label={t(step.label)} admin={t(('adminNext.runtime.next.' + code) as TranslationKey, vars)} fallback={next} />
                : code === 'runtime_missing' ? <>{next} <AdminInstallLink component="python" /></> : next}</p>
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
            {/* alpha.127: a refusal for a hold sends the administrator to where it is lifted. */}
            {error.code === 'RUNTIME_APP' && error.reason === 'held' && <AdminInstallLink to={CATALOG} label={t('adminNext.openCatalog')} />}
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
interface Blocker {
    tone: Tone;
    text: string;
    install?: string;
    /** alpha.127: the administrator's own sentence and the page that holds the action. */
    admin?: { text: string; to: string; label: TranslationKey };
}

function blockerOf(view: RuntimeAppsView, kind: RuntimeKindView, t: Translate): Blocker | null {
    if (!view.has_site) return { tone: 'warning', text: t('runtime.block.noSite') };
    if (view.suspended) return { tone: 'warning', text: t('runtime.block.suspended') };
    // Installed before offered: the server's own Python with its venv module.
    if (kind.kind === 'python' && !kind.python?.ready) {
        return { tone: 'warning', text: t('runtime.python.missing', { detail: kind.python?.detail || '—' }), install: 'python' };
    }
    if (kind.held) return { tone: 'warning', text: t('runtime.block.held'), admin: { text: t('adminNext.runtime.held'), to: CATALOG, label: 'adminNext.openCatalog' } };
    if (!kind.permission_granted) {
        return { tone: 'warning', text: t('runtime.block.notGranted', { grantor: grantorWords(kind.grantor, t) }), admin: { text: t('adminNext.runtime.notGranted'), to: '/users', label: 'adminNext.openUsers' } };
    }
    if (!kind.offered) {
        const layer = kind.refused_by ?? 'administrator';
        const key = ('runtime.closed.' + layer) as TranslationKey;
        const said = t(key, { grantor: grantorWords(kind.grantor, t) });
        const administrator = layer === 'administrator' || said === key;
        return {
            tone: 'warning', text: said === key ? t('runtime.closed.administrator') : said,
            ...(administrator ? { admin: { text: t('adminNext.runtime.notOffered'), to: CATALOG, label: 'adminNext.openCatalog' as TranslationKey } } : {}),
        };
    }
    if (!view.sandbox.known) return { tone: 'info', text: t('runtime.block.levelUnknown') };
    if (view.sandbox.level === 'reduced' && (kind.accepted_level !== 'reduced' || view.sandbox.gaps.some((gap) => !kind.accepted_gaps.includes(gap)))) {
        return {
            tone: 'warning', text: t('runtime.block.sandboxReduced', { gaps: view.sandbox.gaps.join(', ') }),
            admin: { text: t('adminNext.runtime.sandboxReduced', { gaps: view.sandbox.gaps.join(', ') }), to: CATALOG, label: 'adminNext.openCatalog' },
        };
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
            {blocker && <Notice tone={blocker.tone}>{blocker.admin
                ? <AdminInstallLink to={blocker.admin.to} label={t(blocker.admin.label)} admin={blocker.admin.text} fallback={blocker.text} />
                : <>{blocker.text}{blocker.install && <>{' '}<AdminInstallLink component={blocker.install} /></>}</>}</Notice>}
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
    const [removeID, setRemoveID] = useState(newOperationID);

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
    // Removing the program asks in the product's own confirmation (alpha.132;
    // it was a dialogue of its own): what stays, the heavier choice of
    // deleting the app folder too, and for that choice the domain typed out.
    // A lost answer keeps the identity, so asking again is the same removal;
    // any answer that arrived ends it, as for a deploy.
    // Programı kaldırmak ürünün kendi onay penceresinde sorulur; klasörü de
    // silmek için alan adı yazılır.
    const remove = () => void confirmAction({
        title: t('runtime.remove.title', { domain: view.domain }),
        message: t('runtime.remove.keep'),
        tone: 'danger',
        confirmLabel: t('runtime.remove'),
        option: {
            label: t('runtime.remove.files'),
            typedName: { value: view.domain, label: t('runtime.remove.confirm', { domain: view.domain }), ignoreCase: true },
        },
        action: async (_report, answer) => {
            const res = await sendJSON(base + '/remove', 'POST', { operation_id: removeID, remove_files: answer.option, confirm_domain: answer.option ? answer.typed.trim() : '' });
            if (res) setRemoveID(newOperationID());
            await after(res);
        },
    });
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
                <Button icon={Settings2} disabled={busy || !!acting} onClick={() => (editing ? void confirmLeaveIfDirty().then((close) => close && setEditing(false)) : setEditing(true))}>{t('runtime.settings.edit')}</Button>
                <Button variant="danger" icon={Trash2} disabled={busy || !!acting} onClick={remove}>{t('runtime.remove')}</Button>
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
        </div>
    );
}

// The program's settings as the form holds them, every value as typed.
interface SettingsValues { entry: string; argsText: string; health: string; server: 'wsgi' | 'asgi'; workers: string }

const settingsOf = (app: RuntimeApp | undefined, most: number): SettingsValues => ({
    entry: app?.python?.callable ?? app?.entry ?? '',
    argsText: (app?.args ?? []).join('\n'),
    health: app?.health_path ?? '/',
    server: app?.python?.server ?? 'wsgi',
    workers: String(app?.python?.workers ?? Math.min(2, most)),
});

// Creating a program is an action (its button names it). Editing one is a
// settings form (alpha.132): a draft over the program last read, saved only
// with the save bar's Save; Discard puts the saved settings back.
// Program oluşturmak bir eylemdir; düzenlemek Kaydet ile kaydedilen bir ayar formudur.
function SettingsForm({ base, mode, kind, app, after, onDone }: { base: string; mode: 'create' | 'edit'; kind: RuntimeKindView; app?: RuntimeApp; after: After; onDone?: () => void }) {
    const { t } = useI18n();
    const say = useSaveWords();
    const python = kind.kind === 'python';
    const most = kind.python?.max_workers ?? maxWorkers(kind.limits.memory_mb);
    const [created, setCreated] = useState<SettingsValues>(() => settingsOf(undefined, most));
    const known = useMemo(() => (mode === 'edit' && app ? settingsOf(app, most) : null), [mode, app, most]);
    const draft = useDraft(known);
    const values = mode === 'edit' ? draft.values ?? known ?? created : created;
    const set = <K extends keyof SettingsValues>(field: K, value: SettingsValues[K]) => {
        if (mode === 'edit') draft.set(field, value);
        else setCreated((current) => ({ ...current, [field]: value }));
    };
    const { entry, argsText, health, server, workers } = values;
    const [busy, setBusy] = useState(false);
    const args = argsFromText(argsText);
    const entryOK = python ? callableValid(entry) : entryValid(entry);
    const argsOK = python || argsValid(args);
    const workersOK = !python || (/^\d+$/.test(workers) && Number(workers) >= 1 && Number(workers) <= most);
    const healthOK = healthPathValid(health.trim());
    const valid = entryOK && argsOK && healthOK && workersOK;
    const prefix = 'runtime-' + mode;
    const bodyOf = (form: SettingsValues) => (python
        ? { kind: 'python', entry: form.entry.trim(), server: form.server, workers: Number(form.workers), health_path: form.health.trim() || '/' }
        : { kind: 'program', entry: form.entry.trim(), args: argsFromText(form.argsText), health_path: form.health.trim() || '/' });

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (mode === 'edit') {
            save();
            return;
        }
        if (!valid) return;
        setBusy(true);
        const res = await sendJSON(base + '/create', 'POST', bodyOf(created));
        setBusy(false);
        if (await after(res)) {
            showToast('success', t('runtime.create.done'));
            onDone?.();
        }
    };

    const save = () => void draft.save(async (_changes, form) => {
        if (!valid) return 'refused';
        const res = await sendJSON(base + '/settings', 'PUT', bodyOf(form));
        // No answer, or a gateway's in the Panel's place: not known.
        if (!res || answerWasLost(res)) {
            await after(null);
            return 'unknown';
        }
        if (!(await after(res))) return 'refused';
        showToast('success', t('runtime.settings.saved'));
        onDone?.();
        return 'saved';
    });

    const field = (name: keyof SettingsValues, children: ReactNode) => (mode === 'edit'
        ? <div className={`-mx-2 rounded-r px-2 py-1 ${draft.isChanged(name) ? changedRowClass : ''}`}>{draft.isChanged(name) && <p className="mb-1"><ChangedMark /></p>}{children}</div>
        : children);
    const locked = busy || draft.saving;

    return (
        <form className="space-y-3" onSubmit={(event) => void submit(event)} noValidate>
            {mode === 'edit' && <div className="text-sm font-medium text-fg">{t('runtime.settings.title')}</div>}
            {python && field('server', (
                <Field label={t('runtime.python.server')} htmlFor={prefix + '-server'}>
                    <select id={prefix + '-server'} className={`${inputClass} field-lg`} value={server} disabled={locked} onChange={(e) => set('server', e.target.value === 'asgi' ? 'asgi' : 'wsgi')}>
                        <option value="wsgi">{t('runtime.python.server.wsgi')}</option>
                        <option value="asgi">{t('runtime.python.server.asgi')}</option>
                    </select>
                </Field>
            ))}
            {field('entry', (
                <Field label={t(python ? 'runtime.python.callable' : 'runtime.entry')} htmlFor={prefix + '-entry'} hint={t(python ? 'runtime.python.callable.hint' : 'runtime.entry.hint')}>
                    <input id={prefix + '-entry'} className={`${inputClass} field-lg`} autoComplete="off" spellCheck={false} value={entry} disabled={locked} onChange={(e) => set('entry', e.target.value)} />
                </Field>
            ))}
            {entry !== '' && !entryOK && <p className="text-xs text-warning">{t(python ? 'err.RUNTIME_APP.python_settings' : 'err.RUNTIME_APP.entry_invalid', { max: most })}</p>}
            {python ? field('workers', (
                <Field label={t('runtime.python.workers')} htmlFor={prefix + '-workers'} hint={t('runtime.python.workers.hint', { max: most })}>
                    <input id={prefix + '-workers'} inputMode="numeric" className={`${inputClass} field-xs`} value={workers} disabled={locked} onChange={(e) => set('workers', e.target.value)} />
                </Field>
            )) : field('argsText', (
                <Field label={t('runtime.args')} htmlFor={prefix + '-args'} hint={t('runtime.args.hint', { placeholder: '{port}' })}>
                    <textarea id={prefix + '-args'} rows={3} className={inputClass + ' font-mono'} spellCheck={false} value={argsText} disabled={locked} onChange={(e) => set('argsText', e.target.value)} />
                </Field>
            ))}
            {!argsOK && <p className="text-xs text-warning">{t('err.RUNTIME_APP.args_invalid')}</p>}
            {!workersOK && <p className="text-xs text-warning">{t('runtime.python.workers.hint', { max: most })}</p>}
            {field('health', (
                <Field label={t('runtime.health')} htmlFor={prefix + '-health'} hint={t('runtime.health.hint')}>
                    <input id={prefix + '-health'} className={`${inputClass} field-lg`} autoComplete="off" spellCheck={false} value={health} disabled={locked} onChange={(e) => set('health', e.target.value)} />
                </Field>
            ))}
            {!healthOK && <p className="text-xs text-warning">{t('err.RUNTIME_APP.health_path_invalid')}</p>}
            {mode === 'edit' ? (
                <>
                    <p className="text-xs text-fg-subtle">{t('runtime.settings.applies')}</p>
                    <SaveBar
                        className="-mx-3 rounded-b-lg px-3 pb-0"
                        count={draft.count}
                        saving={draft.saving}
                        blockedBy={draft.held ? say('stale') : !valid ? t('runtime.settings.invalid') : undefined}
                        onSave={save}
                        onDiscard={draft.reset}
                    />
                </>
            ) : (
                <div className="flex flex-wrap gap-2">
                    <Button type="submit" variant="primary" icon={Plus} loading={busy} disabled={!valid}>
                        {t('runtime.create.button')}
                    </Button>
                </div>
            )}
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
    // alpha.132: what differs from the list the dialog opened with. Save is
    // offered only for a change; closing or leaving with one asks first.
    // Pencerenin açıldığı listeden farklı olan; Kaydet yalnız bir değişiklik için sunulur.
    const opened = useMemo(() => app.env.map((entry, i) => ({ id: i, key: entry.key, value: entry.secret ? '' : (entry.value ?? ''), secret: entry.secret })), [app.env]);
    const rowChanged = (row: EnvRow) => {
        const was = opened.find((item) => item.id === row.id);
        return !was || was.key !== row.key || was.value !== row.value || was.secret !== row.secret;
    };
    const changed = rows.length !== opened.length || rows.some(rowChanged);
    useLeaveGuard(changed ? 'some' : 0);
    const close = () => void (changed ? confirmLeaveIfDirty() : Promise.resolve(true)).then((go) => go && onClose());
    const update = (id: number, change: Partial<EnvRow>) => setRows((list) => list.map((row) => (row.id === id ? { ...row, ...change } : row)));

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!ready || !changed) return;
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
        <Dialog id="runtime-env" title={t('runtime.env.title')} icon={KeyRound} width="xl" busy={busy} onDismiss={close} onSubmit={(event) => void submit(event)} noValidate
            actions={<><Button type="button" onClick={close} disabled={busy}>{t('common.cancel')}</Button><Button type="submit" variant="primary" loading={busy} disabled={!ready || !changed}>{t('runtime.env.save')}</Button></>}>
            <div className="space-y-3 text-sm">
                <p className="text-fg-muted">{t('runtime.env.hint')}</p>
                {rows.length === 0 && <p className="text-fg-subtle">{t('runtime.env.empty')}</p>}
                {rows.map((row, i) => (
                    <div key={row.id} className={`space-y-1 rounded-lg border border-border p-2 ${rowChanged(row) ? changedRowClass : ''}`} data-runtime-env-row={row.key}>
                        <div className="flex flex-wrap items-start gap-2">
                            <input aria-label={t('runtime.env.key')} placeholder={t('runtime.env.key')} className={`${inputClass} font-mono field-md`} autoComplete="off" spellCheck={false}
                                value={row.key} onChange={(e) => update(row.id, { key: e.target.value.toUpperCase() })} />
                            {row.secret ? (
                                <PasswordInput aria-label={t('runtime.env.value')} autoComplete="new-password"
                                    placeholder={keeps(row) || (row.storedSecret !== '' && row.storedSecret === row.key) ? t('runtime.env.secretReplace') : t('runtime.env.value')}
                                    value={row.value} onChange={(value) => update(row.id, { value })} />
                            ) : (
                                <input aria-label={t('runtime.env.value')} placeholder={t('runtime.env.value')} className={`${inputClass} field-lg`} autoComplete="off" spellCheck={false}
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
                            {rowChanged(row) && <ChangedMark />}
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
