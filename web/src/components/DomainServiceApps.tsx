import { useState, type FormEvent, type ReactNode } from 'react';
import { ArrowUpCircle, Bot, ExternalLink, KeyRound, Loader2, Play, RotateCw, ScrollText, Square, Trash2 } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, ErrorBanner, Field, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { readRemote, useRefreshEvery, useRemote } from '../lib/remote';
import {
    decodeDomainServiceApps,
    newOperationID,
    type DomainServiceAppEntry,
    type DomainServiceAppsView,
    type ServiceAppInstall,
    type ServiceAppOperation,
} from '../lib/serviceApps';
import { ServiceAppRisk } from './ServiceAppRisk';

// The domain's "AI & automation" tab (alpha.118, docs/AI-APPS.md): the apps
// the administrator offers, each with its risk summary; the installed app with
// its state, start/stop, log, keys, update and removal; and the running or
// last operation, with who acts and what comes next. The operation runs on
// the server: this page only reads it (every few seconds while it runs) and
// never starts anything by reading. A request whose answer was lost is sent
// again with the same operation identity, so the server answers the same
// operation instead of starting a second one.
//
// Alan adının "Yapay zekâ ve otomasyon" sekmesi. İşlem sunucuda sürer; bu sayfa
// yalnız okur. Yanıtı yiten istek aynı işlem kimliğiyle yeniden gönderilir.

async function sendJSON(url: string, method: string, body: unknown): Promise<{ res: Response | null }> {
    try {
        const res = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
        return { res };
    } catch {
        return { res: null };
    }
}

function Notice({ tone = 'warning', children }: { tone?: 'warning' | 'danger' | 'info'; children: ReactNode }) {
    const palette = tone === 'danger' ? 'border-danger/30 bg-danger/10 text-danger' : tone === 'info' ? 'border-border bg-surface-2 text-fg-muted' : 'border-warning/30 bg-warning/10 text-warning';
    return <div className={`rounded-lg border px-3 py-2.5 text-sm ${palette}`}>{children}</div>;
}

export function DomainServiceApps({ domainId }: { domainId: number; domainName: string }) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/service-apps`;
    const view = useRemote(url, decodeDomainServiceApps);
    const running = view.remote.state === 'known' && view.remote.value.operation?.state === 'running';
    useRefreshEvery(view, 4000, running);
    const [failure, setFailure] = useState<ApiError | null>(null);
    const [lost, setLost] = useState(false);
    const [installing, setInstalling] = useState<DomainServiceAppEntry | null>(null);

    const after = async (res: Response | null, done?: () => void) => {
        if (!res) {
            setLost(true);
            await view.retry();
            return false;
        }
        setLost(false);
        if (!res.ok) {
            setFailure(await readApiError(res));
            await view.retry();
            return false;
        }
        setFailure(null);
        done?.();
        await view.retry();
        return true;
    };

    return (
        <RemoteGate remote={view.remote} checking={t('svcapps.checking')} failed={t('svcapps.unknown')} onRetry={() => void view.retry()} busy={view.reading}>
            {(shown) => {
                const v = shown.value;
                return (
                    <div className="space-y-4">
                        <div>
                            <h3 className="flex items-center gap-2 text-base font-semibold text-fg"><Bot className="h-5 w-5 text-primary" />{t('svcapps.title')}</h3>
                            <p className="mt-1 text-sm text-fg-muted">{t('svcapps.desc')}</p>
                        </div>
                        {lost && <Notice tone="info">{t('svcapps.reason.agent_unreachable')}</Notice>}
                        <ErrorBanner error={failure} />
                        <Blockers view={v} />
                        {v.operation && <OperationLine op={v.operation} />}
                        {v.install ? (
                            <InstalledApp view={v} install={v.install} base={url} after={after} onReinstall={setInstalling} />
                        ) : (
                            <Catalog view={v} onInstall={setInstalling} />
                        )}
                        {installing && (
                            <InstallDialog
                                app={installing}
                                domain={v.domain}
                                url={url}
                                onClose={() => setInstalling(null)}
                                after={after}
                            />
                        )}
                    </div>
                );
            }}
        </RemoteGate>
    );
}

function Blockers({ view }: { view: DomainServiceAppsView }) {
    const { t } = useI18n();
    if (!view.permission_granted) return <Notice>{t('svcapps.notGranted', { grantor: view.permission_grantor ?? '' })}</Notice>;
    if (view.suspended) return <Notice>{t('svcapps.suspended')}</Notice>;
    if (!view.has_site) return <Notice>{t('svcapps.noSite')}</Notice>;
    if (!view.install && !view.certificate) return <Notice>{t('svcapps.noCertificate')}</Notice>;
    if (!view.availability_known) return <Notice tone="info">{t('svcapps.availabilityUnknown')}</Notice>;
    return null;
}

function OperationLine({ op }: { op: ServiceAppOperation }) {
    const { t } = useI18n();
    if (op.state === 'running') {
        return (
            <Notice tone="info">
                <span className="flex items-start gap-2">
                    <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin" />
                    {t('svcapps.op.running', { step: t(('svcapps.op.step.' + op.step) as TranslationKey) })}
                </span>
            </Notice>
        );
    }
    if (op.state === 'failed') {
        const kind = t(('svcapps.op.kind.' + op.kind) as TranslationKey);
        const guidanceKey = ('svcapps.reason.' + (op.reason ?? '')) as TranslationKey;
        const guidance = t(guidanceKey);
        return (
            <Notice tone={op.class === 'verified_failure' ? 'danger' : 'warning'}>
                <p className="font-medium">{t(('svcapps.op.failed.' + (op.class || 'unknown')) as TranslationKey, { kind })}</p>
                {guidance !== guidanceKey && <p className="mt-1">{guidance}</p>}
                {op.message && <p className="mt-1 text-xs opacity-80">{t('svcapps.op.serverSaid', { message: op.message })}</p>}
            </Notice>
        );
    }
    if (op.message) return <Notice tone="info">{t('svcapps.op.done.note', { message: op.message })}</Notice>;
    return null;
}

function closedReason(app: DomainServiceAppEntry, t: (key: TranslationKey, vars?: Record<string, string | number>) => string): string {
    if (app.deferred) return t(('svcapps.deferred.' + app.deferred) as TranslationKey);
    if (app.held) return t('svcapps.heldNote');
    if (!app.offered) return t(('svcapps.closed.' + (app.refused_by ?? 'administrator')) as TranslationKey, { grantor: app.grantor ?? '' });
    if (!app.available) return t(('svcapps.unavailable.' + (app.reason ?? 'probe_failed')) as TranslationKey, { detail: app.detail ?? '' });
    return '';
}

function Catalog({ view, onInstall }: { view: DomainServiceAppsView; onInstall: (app: DomainServiceAppEntry) => void }) {
    const { t } = useI18n();
    const blocked = !view.permission_granted || view.suspended || !view.has_site || !view.certificate || view.operation?.state === 'running';
    const offerable = view.apps.filter((app) => !app.deferred);
    const deferred = view.apps.filter((app) => app.deferred);
    return (
        <div className="space-y-3">
            <p className="text-xs text-fg-subtle">{t('svcapps.useSubdomain', { domain: view.domain })}</p>
            {offerable.map((app) => {
                const why = closedReason(app, t);
                return (
                    <div key={app.id} className="rounded-xl border border-border bg-surface p-4">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                            <div className="min-w-0">
                                <div className="font-semibold text-fg">{app.name}</div>
                                <div className="text-xs text-fg-subtle">{app.vendor}</div>
                            </div>
                            <Button variant="primary" disabled={blocked || why !== ''} onClick={() => onInstall(app)}>{t('svcapps.install')}</Button>
                        </div>
                        {why && <p className="mt-2 text-sm text-warning">{why}</p>}
                        <details className="mt-3">
                            <summary className="cursor-pointer text-sm text-primary">{t('svcapps.risk.canDo')} · {t('svcapps.risk.remains')}</summary>
                            <div className="mt-3"><ServiceAppRisk app={app} /></div>
                        </details>
                    </div>
                );
            })}
            {deferred.length > 0 && (
                <details className="rounded-xl border border-border bg-surface-2 p-4">
                    <summary className="cursor-pointer text-sm font-medium text-fg-muted">{t('svcapps.admin.deferredTitle')}</summary>
                    <ul className="mt-2 space-y-2 text-sm text-fg-muted">
                        {deferred.map((app) => (
                            <li key={app.id}><span className="font-medium text-fg">{app.name}</span> — {closedReason(app, t)}</li>
                        ))}
                    </ul>
                </details>
            )}
        </div>
    );
}

type After = (res: Response | null, done?: () => void) => Promise<boolean>;

const decodeLogLines = (raw: unknown): string[] => {
    const lines = (raw as { lines?: unknown } | null)?.lines;
    if (!Array.isArray(lines)) throw new Error('shape');
    return lines.filter((l): l is string => typeof l === 'string');
};

function InstallDialog({ app, domain, url, onClose, after }: { app: DomainServiceAppEntry; domain: string; url: string; onClose: () => void; after: After }) {
    const { t } = useI18n();
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [secrets, setSecrets] = useState<Record<string, string>>({});
    const [confirmReplace, setConfirmReplace] = useState(false);
    const [readRisks, setReadRisks] = useState(false);
    const [busy, setBusy] = useState(false);
    // One identity for this install: a repeat after a lost answer is the same.
    const [operationID] = useState(newOperationID);
    const ownerOK = !app.owner_login || (/^\S+@\S+\.\S+$/.test(email) && password.length >= 12 && password.length <= 128);

    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!ownerOK || !confirmReplace || !readRisks) return;
        setBusy(true);
        const { res } = await sendJSON(url + '/install', 'POST', {
            app: app.id, operation_id: operationID, owner_email: email, owner_password: password, secrets, confirm_replace: true,
        });
        setBusy(false);
        if (await after(res)) {
            setPassword('');
            setSecrets({});
            onClose();
        }
    };

    return (
        <Dialog
            id="svcapps-install"
            title={t('svcapps.installTitle', { name: app.name, domain })}
            icon={Bot}
            width="lg"
            busy={busy}
            onDismiss={onClose}
            onSubmit={submit}
            noValidate
            footerLead={
                <label className="flex items-start gap-2 text-sm text-fg">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={readRisks} onChange={(e) => setReadRisks(e.target.checked)} />
                    {t('svcapps.readRisks', { name: app.name })}
                </label>
            }
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    <Button type="submit" variant="primary" loading={busy} disabled={!ownerOK || !confirmReplace || !readRisks}>{t('svcapps.install')}</Button>
                </>
            }
        >
            <div className="space-y-4">
                <ServiceAppRisk app={app} />
                {app.owner_login && (
                    <div className="grid gap-3 sm:grid-cols-2">
                        <Field label={t('svcapps.ownerEmail')} htmlFor="svcapps-owner-email">
                            <input id="svcapps-owner-email" type="email" autoComplete="off" className={inputClass} value={email} onChange={(e) => setEmail(e.target.value)} />
                        </Field>
                        <Field label={t('svcapps.ownerPassword')} htmlFor="svcapps-owner-password">
                            <input id="svcapps-owner-password" type="password" autoComplete="new-password" className={inputClass} value={password} onChange={(e) => setPassword(e.target.value)} />
                        </Field>
                        <p className="text-xs text-fg-subtle sm:col-span-2">{t('svcapps.ownerHint')}</p>
                    </div>
                )}
                {(app.secrets ?? []).length > 0 && (
                    <div className="space-y-2">
                        <div className="text-sm font-medium text-fg">{t('svcapps.keysTitle')}</div>
                        {(app.secrets ?? []).map((secret) => (
                            <Field key={secret.key} label={secret.label} htmlFor={'svcapps-secret-' + secret.key}>
                                <input id={'svcapps-secret-' + secret.key} type={secret.key.endsWith('_KEY') ? 'password' : 'text'} autoComplete="off" className={inputClass}
                                    value={secrets[secret.key] ?? ''} onChange={(e) => setSecrets((s) => ({ ...s, [secret.key]: e.target.value }))} />
                            </Field>
                        ))}
                        <p className="text-xs text-fg-subtle">{t('svcapps.keyHint')}</p>
                    </div>
                )}
                {app.owner_login && !ownerOK && (email || password) && <p className="text-xs text-warning">{t('svcapps.check.owner')}</p>}
                <label className="flex items-start gap-2 text-sm text-fg">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={confirmReplace} onChange={(e) => setConfirmReplace(e.target.checked)} />
                    {t('svcapps.confirmReplace', { domain, name: app.name })}
                </label>
            </div>
        </Dialog>
    );
}

function InstalledApp({ view, install, base, after, onReinstall }: {
    view: DomainServiceAppsView;
    install: ServiceAppInstall;
    base: string;
    after: After;
    onReinstall: (app: DomainServiceAppEntry) => void;
}) {
    const { t } = useI18n();
    const busy = view.operation?.state === 'running';
    const [acting, setActing] = useState('');
    const [logs, setLogs] = useState<{ install: boolean; lines: string[] } | null>(null);
    const [logsFailure, setLogsFailure] = useState<ApiError | null>(null);
    const [keysOpen, setKeysOpen] = useState(false);
    const [removeOpen, setRemoveOpen] = useState(false);
    const [opID, setOpID] = useState(newOperationID);
    const app = view.apps.find((a) => a.id === install.app_id);

    const act = async (path: string, body: unknown, label: string, done?: () => void) => {
        setActing(label);
        const { res } = await sendJSON(base + path, 'POST', body);
        setActing('');
        if (await after(res, done)) showToast('success', t('svcapps.actionDone'));
    };
    const readLogs = async (installLog: boolean) => {
        setActing(installLog ? 'installLog' : 'logs');
        setLogsFailure(null);
        const read = await readRemote(`${base}/logs?lines=200${installLog ? '&install=1' : ''}`, decodeLogLines);
        setActing('');
        if (read.state === 'known') setLogs({ install: installLog, lines: read.value });
        else setLogsFailure(read.reason);
    };

    const stateText = install.status_unknown ? t('svcapps.state.unknown')
        : install.active ? (install.serving ? t('svcapps.state.running') : t('svcapps.state.notServing'))
        : t('svcapps.state.stopped');
    const lastFailed = view.operation?.state === 'failed';
    const retryKind = lastFailed ? view.operation?.kind : undefined;
    return (
        <div className="space-y-3 rounded-xl border border-border bg-surface p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                    <div className="font-semibold text-fg">{install.name} {install.version || install.target_version}</div>
                    <div className="text-sm text-fg-muted">
                        {stateText}{install.memory_mb ? ' · ' + t('svcapps.memory', { mb: install.memory_mb }) : ''}
                    </div>
                </div>
                {install.version && install.active && install.serving && (
                    <a href={install.url} target="_blank" rel="noopener noreferrer"
                        className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2">
                        <ExternalLink className="h-4 w-4" />{t('svcapps.open')}
                    </a>
                )}
            </div>
            {install.held && <Notice>{t('svcapps.heldNote')}</Notice>}
            {install.needs_sandbox && <Notice tone="info">{t('svcapps.needsSandbox')}</Notice>}
            {install.update_available && !busy && (
                <Notice tone="info">
                    <span className="flex flex-wrap items-center justify-between gap-2">
                        <span>{t('svcapps.updateNote')}</span>
                        <Button icon={ArrowUpCircle} loading={acting === 'update'} disabled={!!acting || install.held}
                            onClick={() => void act('/update', { operation_id: opID }, 'update', () => setOpID(newOperationID()))}>
                            {t('svcapps.update', { version: install.update_available })}
                        </Button>
                    </span>
                </Notice>
            )}
            <div className="flex flex-wrap gap-2">
                {install.version && !install.active && (
                    <Button icon={Play} loading={acting === 'start'} disabled={busy || !!acting || install.held || view.suspended} onClick={() => void act('/control', { action: 'start' }, 'start')}>{t('svcapps.start')}</Button>
                )}
                {install.version && install.active && (
                    <>
                        <Button icon={Square} loading={acting === 'stop'} disabled={busy || !!acting} onClick={() => void act('/control', { action: 'stop' }, 'stop')}>{t('svcapps.stop')}</Button>
                        <Button icon={RotateCw} loading={acting === 'restart'} disabled={busy || !!acting || install.held} onClick={() => void act('/control', { action: 'restart' }, 'restart')}>{t('svcapps.restart')}</Button>
                    </>
                )}
                <Button icon={ScrollText} loading={acting === 'logs'} disabled={!!acting} onClick={() => void readLogs(false)}>{t('svcapps.logs')}</Button>
                <Button icon={ScrollText} loading={acting === 'installLog'} disabled={!!acting} onClick={() => void readLogs(true)}>{t('svcapps.installLog')}</Button>
                {install.version && app && ((app.secrets ?? []).length > 0 || app.id === 'n8n') && (
                    <Button icon={KeyRound} disabled={busy || !!acting} onClick={() => setKeysOpen(true)}>{t('svcapps.keys')}</Button>
                )}
                {retryKind === 'install' && !install.version && app && (
                    <Button icon={RotateCw} disabled={!!acting || closedReason(app, t) !== '' || !view.certificate} onClick={() => onReinstall(app)}>{t('svcapps.op.retry')}</Button>
                )}
                {retryKind === 'update' && (
                    <Button icon={RotateCw} loading={acting === 'update'} disabled={!!acting} onClick={() => void act('/update', { operation_id: opID }, 'update', () => setOpID(newOperationID()))}>{t('svcapps.op.retry')}</Button>
                )}
                <Button variant="danger" icon={Trash2} disabled={busy || !!acting} onClick={() => setRemoveOpen(true)}>{t('svcapps.uninstall')}</Button>
            </div>
            <p className="text-xs text-fg-subtle">{t('svcapps.independent', { unit: install.unit, folder: install.folder })}</p>
            <ErrorBanner error={logsFailure} />
            {logs && (
                <div>
                    <div className="mb-1 text-xs font-medium text-fg-muted">{t('svcapps.logsTitle')}</div>
                    <pre className="max-h-80 overflow-auto rounded-lg border border-border bg-surface-2 p-3 text-xs text-fg">{logs.lines.length ? logs.lines.join('\n') : t('svcapps.logsEmpty')}</pre>
                </div>
            )}
            {keysOpen && app && <KeysDialog app={app} base={base} onClose={() => setKeysOpen(false)} after={after} />}
            {removeOpen && <RemoveDialog install={install} domain={view.domain} base={base} onClose={() => setRemoveOpen(false)} after={after} />}
        </div>
    );
}

function KeysDialog({ app, base, onClose, after }: { app: DomainServiceAppEntry; base: string; onClose: () => void; after: After }) {
    const { t } = useI18n();
    const [values, setValues] = useState<Record<string, string>>({});
    const [clear, setClear] = useState<string[]>([]);
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState(false);
    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setBusy(true);
        let res: Response | null = null;
        try {
            res = await fetch(base + '/secrets', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ secrets: values, clear, owner_password: password }) });
        } catch {
            res = null;
        }
        setBusy(false);
        if (await after(res)) {
            showToast('success', t('svcapps.keysSaved'));
            onClose();
        }
    };
    const changed = Object.values(values).some((v) => v.trim() !== '') || clear.length > 0 || password.length >= 12;
    return (
        <Dialog id="svcapps-keys" title={t('svcapps.keys')} icon={KeyRound} busy={busy} onDismiss={onClose} onSubmit={submit} noValidate
            actions={<><Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button><Button type="submit" variant="primary" loading={busy} disabled={!changed}>{t('svcapps.save')}</Button></>}>
            <div className="space-y-3">
                {(app.secrets ?? []).map((secret) => (
                    <Field key={secret.key} label={secret.label} htmlFor={'svcapps-key-' + secret.key}>
                        <input id={'svcapps-key-' + secret.key} type="password" autoComplete="off" className={inputClass} value={values[secret.key] ?? ''}
                            onChange={(e) => setValues((s) => ({ ...s, [secret.key]: e.target.value }))} />
                        <label className="mt-1 flex items-center gap-2 text-xs text-fg-muted">
                            <input type="checkbox" className="h-3.5 w-3.5 accent-primary" checked={clear.includes(secret.key)}
                                onChange={(e) => setClear((c) => (e.target.checked ? [...c, secret.key] : c.filter((k) => k !== secret.key)))} />
                            {t('svcapps.keyClear')}
                        </label>
                    </Field>
                ))}
                {app.id === 'n8n' && (
                    <Field label={t('svcapps.newOwnerPassword')} htmlFor="svcapps-new-owner" hint={t('svcapps.ownerHint')}>
                        <input id="svcapps-new-owner" type="password" autoComplete="new-password" className={inputClass} value={password} onChange={(e) => setPassword(e.target.value)} />
                    </Field>
                )}
                <p className="text-xs text-fg-subtle">{t('svcapps.keyHint')}</p>
            </div>
        </Dialog>
    );
}

function RemoveDialog({ install, domain, base, onClose, after }: { install: ServiceAppInstall; domain: string; base: string; onClose: () => void; after: After }) {
    const { t } = useI18n();
    const [removeData, setRemoveData] = useState(false);
    const [typed, setTyped] = useState('');
    const [busy, setBusy] = useState(false);
    const [operationID] = useState(newOperationID);
    const ready = !removeData || typed.trim().toLowerCase() === domain.toLowerCase();
    const submit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!ready) return;
        setBusy(true);
        const { res } = await sendJSON(base + '/uninstall', 'POST', { operation_id: operationID, remove_data: removeData, confirm_domain: typed });
        setBusy(false);
        if (await after(res)) onClose();
    };
    return (
        <Dialog id="svcapps-remove" tone="danger" title={t('svcapps.uninstallTitle', { name: install.name, domain })} icon={Trash2} busy={busy} onDismiss={onClose} onSubmit={submit} noValidate
            actions={<><Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button><Button type="submit" variant="danger" loading={busy} disabled={!ready}>{t('svcapps.uninstall')}</Button></>}>
            <div className="space-y-3 text-sm">
                <p className="text-fg-muted">{t('svcapps.uninstallKeep')}</p>
                <label className="flex items-start gap-2 text-fg">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={removeData} onChange={(e) => setRemoveData(e.target.checked)} />
                    {t('svcapps.removeData')}
                </label>
                {removeData && (
                    <Field label={t('svcapps.confirmDomain', { domain })} htmlFor="svcapps-confirm-domain">
                        <input id="svcapps-confirm-domain" className={inputClass} autoComplete="off" value={typed} onChange={(e) => setTyped(e.target.value)} />
                    </Field>
                )}
            </div>
        </Dialog>
    );
}
