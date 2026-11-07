import { useState, type ReactNode } from 'react';
import { FolderLock, Plus, Trash2, UserRound } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { useNavigate } from '../router';
import type { TranslationKey } from '../i18n/en';
import { Button, RemoteGate, inputClass } from './ui';
import { readApiError, refusalText, type ApiError } from '../lib/apiError';
import { decodeList, useRemote } from '../lib/remote';
import { PasswordInput } from './PasswordInput';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { confirmAction, type OperationEnd, type OperationReport } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';

// A site's other FTP/SFTP logins (alpha.128, gap G16, docs/FTP-LOGINS.md).
// Each login sees one folder of the site and uploads as the site. The server
// keeps the login (its account, its closed folder, OpenSSH and vsftpd), so it
// works without CelikPanel. Problems come first, then the list, then the
// form, then the connection recipe. Every change is asked in the panel's own
// window and shown there step by step; the list reads itself again after.
//
// Sitenin diğer FTP/SFTP girişleri: her biri sitenin tek klasörünü görür.
// Önce sorunlar, sonra liste, form ve bağlantı tarifi.

interface LoginRow {
    login: string;
    folder: string;
    /** ready | no_password | suspended | jail | unfinished | not_on_server | unknown */
    state: string;
    reason: string;
    readOnly: boolean;
}

interface LoginsView {
    domain: string;
    available: boolean;
    subscriptionStatus: string;
    fileAccess: boolean;
    owner: string;
    used: number;
    ceiling: number | null;
    rows: LoginRow[];
    stateError: string;
    sshdPresent: boolean | null;
    sshDropIn: string;
    ftpInstalled: boolean;
    ftpConfig: string;
    connection: { host: string; ftpPort: number; sftpPort: number; passiveMin: number; passiveMax: number };
}

const str = (value: unknown): string => (typeof value === 'string' ? value : '');
const num = (value: unknown): number => (typeof value === 'number' ? value : 0);
const obj = (value: unknown): Record<string, unknown> => (value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {});

function decodeLogins(raw: unknown): LoginsView {
    const body = obj(raw);
    if (typeof body.available !== 'boolean') throw new Error('shape');
    const rights = obj(body.rights);
    const limit = obj(body.limit);
    const connection = obj(body.connection);
    const state = body.state === undefined || body.state === null ? null : obj(body.state);
    const onServer = new Map<string, Record<string, unknown>>();
    for (const item of decodeList<Record<string, unknown>>(state?.logins ?? [])) onServer.set(str(item.login), item);
    const rows: LoginRow[] = [];
    const seen = new Set<string>();
    const describe = (login: string, folder: string): LoginRow => {
        const server = onServer.get(login);
        if (!state) return { login, folder, state: 'unknown', reason: '', readOnly: false };
        if (!server) return { login, folder, state: 'not_on_server', reason: '', readOnly: false };
        const readOnly = server.read_only === true;
        if (server.unfinished === true) return { login, folder, state: 'unfinished', reason: '', readOnly };
        if (server.suspended === true) return { login, folder: str(server.folder) || folder, state: 'suspended', reason: '', readOnly };
        if (server.jail_active !== true || server.account_ok !== true) {
            return { login, folder: str(server.folder) || folder, state: 'jail', reason: str(server.jail_problem), readOnly };
        }
        if (str(server.password) !== 'set') return { login, folder: str(server.folder) || folder, state: 'no_password', reason: '', readOnly };
        return { login, folder: str(server.folder) || folder, state: 'ready', reason: '', readOnly };
    };
    for (const record of decodeList<Record<string, unknown>>(body.records ?? [])) {
        const login = str(record.login);
        if (!login) continue;
        seen.add(login);
        rows.push(describe(login, str(record.folder)));
    }
    // A login the server has and the panel did not record (a lost answer).
    for (const login of onServer.keys()) if (login && !seen.has(login)) rows.push(describe(login, ''));
    rows.sort((a, b) => a.login.localeCompare(b.login));
    return {
        domain: str(body.domain),
        available: body.available,
        subscriptionStatus: str(body.subscription_status) || 'active',
        fileAccess: rights.file_access === true,
        owner: str(body.owner),
        used: num(limit.used),
        ceiling: typeof limit.ceiling === 'number' ? limit.ceiling : null,
        rows,
        stateError: str(body.state_error),
        sshdPresent: state ? state.sshd_present === true : null,
        sshDropIn: str(state?.sshd_drop_in),
        ftpInstalled: state?.ftp_installed === true,
        ftpConfig: str(state?.ftp_config),
        connection: {
            host: str(connection.sftp_host),
            ftpPort: num(connection.ftp_port) || 21,
            sftpPort: num(connection.sftp_port) || 22,
            passiveMin: num(connection.passive_min),
            passiveMax: num(connection.passive_max),
        },
    };
}

function Notice({ tone, children }: { tone: 'warning' | 'danger' | 'info'; children: ReactNode }) {
    const colors = tone === 'danger'
        ? 'border-danger/40 bg-danger/10 text-danger'
        : tone === 'warning' ? 'border-warning/40 bg-warning/10 text-fg' : 'border-border bg-surface-muted text-fg-muted';
    return <div role={tone === 'info' ? undefined : 'alert'} className={`rounded-lg border px-3 py-2 text-sm ${colors}`}>{children}</div>;
}

const LOGIN_NAME = /^[a-z][a-z0-9_-]{1,30}[a-z0-9]$/;

export function FTPLogins({ domainId, isAdmin }: { domainId: number; isAdmin: boolean }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const url = `/api/v1/domains/${domainId}/access/logins`;
    const logins = useRemote(url, decodeLogins);
    const [adding, setAdding] = useState(false);
    const [name, setName] = useState('');
    const [folder, setFolder] = useState('public_html');
    const [password, setPassword] = useState('');
    const [editing, setEditing] = useState<string | null>(null);
    const [editFolder, setEditFolder] = useState('');
    const [editPassword, setEditPassword] = useState('');
    const [busy, setBusy] = useState(false);
    const [problem, setProblem] = useState('');
    const [warnings, setWarnings] = useState<string[]>([]);

    const explain = (error: ApiError): string => {
        const words = refusalText(error, t, 'err.FTP_LOGIN_REFUSED');
        // The server's own sentence says exactly what it found; the screen's
        // words come first, in the person's language.
        return error.message && !words.includes(error.message) && (error.code === 'FTP_LOGIN_REFUSED' || error.code === 'FTP_LOGIN_INVALID')
            ? `${words} ${error.message}` : words;
    };

    // One change, in the window that is already open, following the server's steps.
    const run = async (report: OperationReport, key: string, method: 'POST' | 'PUT' | 'DELETE', target: string, body: unknown, done: string): Promise<OperationEnd> => {
        setBusy(true);
        setProblem('');
        try {
            const res = await sendWithSteps(report, flowSteps(t, 'ftplogin', ['check!', 'server_change', 'finish']), t, key,
                async (headers) => {
                    try {
                        return await fetch(target, {
                            method, headers: { 'Content-Type': 'application/json', ...headers },
                            body: body === undefined ? undefined : JSON.stringify(body),
                        });
                    } catch {
                        return null;
                    }
                });
            if (!res) {
                await logins.retry();
                return { state: 'unknown', reason: t('common.resultUnknown') };
            }
            if (!res.ok) {
                const error = await readApiError(res);
                const reason = explain(error);
                setProblem(reason);
                await logins.retry();
                const blocked = res.status < 500 && (error.code !== 'FTP_LOGIN_REFUSED' || ['folder', 'sshd_owner_config', 'unit_owner_config', 'no_sshd', 'name_taken', 'site_account', 'sshd_include_missing'].includes(error.reason ?? ''));
                return { state: blocked ? 'blocked' : 'failed', reason };
            }
            // The change is made; when its details cannot be read there are
            // no warnings to show, and the list read below says the rest.
            const answer = obj(await res.json().then((body: unknown) => body, () => ({ warnings: [] })));
            setWarnings(decodeList<unknown>(answer.warnings ?? []).map(str).filter((w) => t(`ftpLogins.warn.${w}` as TranslationKey) !== `ftpLogins.warn.${w}`));
            await logins.retry();
            showToast('success', done);
            return { state: 'done', message: done };
        } finally {
            setBusy(false);
        }
    };

    return (
        <RemoteGate
            remote={logins.remote}
            checking={t('ftpLogins.checking')}
            failed={t('ftpLogins.unknown')}
            onRetry={() => void logins.retry()}
            busy={logins.reading}
        >
            {(shown) => {
                const view = shown.value;
                const settled = logins.remote.state === 'known' && !logins.reading && !shown.stale;
                const managed = isAdmin || view.owner !== '';
                const blockers: ReactNode[] = [];
                if (!view.available) blockers.push(<Notice key="site" tone="info">{t('ftpLogins.block.noSite')}</Notice>);
                else if (!view.fileAccess) {
                    blockers.push(<Notice key="plan" tone="warning">
                        {view.owner ? t('ftpLogins.block.notInPlanManager', { name: view.owner }) : t('ftpLogins.block.notInPlan')}
                        {view.owner && <>{' '}<button type="button" onClick={() => navigate(`/users?rights=${encodeURIComponent(view.owner)}`)} className="font-semibold text-primary underline underline-offset-2 max-sm:min-h-11">{t('ftpLogins.openRights', { name: view.owner })}</button></>}
                    </Notice>);
                }
                if (view.available && view.subscriptionStatus !== 'active') blockers.push(<Notice key="sub" tone="warning">{t('ftpLogins.block.suspended')}</Notice>);
                if (view.stateError === 'agent_upgrade_required') blockers.push(<Notice key="agent" tone="warning">{t('ftpLogins.block.agentUpgrade')}</Notice>);
                if (view.stateError === 'agent_unavailable') blockers.push(<Notice key="agent" tone="warning">{t('ftpLogins.block.agentUnavailable')}</Notice>);
                if (view.sshdPresent === false) blockers.push(<Notice key="sshd" tone="warning">{t(isAdmin ? 'ftpLogins.block.noSSHDAdmin' : 'ftpLogins.block.noSSHD')}</Notice>);
                if (view.sshDropIn === 'owner') blockers.push(<Notice key="owner" tone="warning">{t(isAdmin ? 'ftpLogins.block.sshOwnerAdmin' : 'ftpLogins.block.sshOwner')}</Notice>);
                const canOpen = settled && !busy && view.available && view.fileAccess && view.subscriptionStatus === 'active'
                    && view.stateError === '' && view.sshdPresent === true && view.sshDropIn !== 'owner';
                const full = view.ceiling !== null && view.used >= view.ceiling;
                const nameOK = LOGIN_NAME.test(name) && name.includes('-') && !name.includes('--');
                const passwordOK = password !== '' && passwordAcceptable(password, 'file_access');
                const folderShown = (f: string) => (f === '/' ? '/' : f.replace(/^\//, ''));

                const add = () => {
                    const shownFolder = folderShown('/' + folder.replace(/^\/+/, ''));
                    void confirmAction({
                        title: t('ftpLogins.confirmAddTitle', { login: name }),
                        message: t('ftpLogins.confirmAdd', { login: name, folder: shownFolder, domain: view.domain }),
                        confirmLabel: t('ftpLogins.save'),
                        progressTitle: t('ftpLogins.progressAdd', { login: name }),
                        action: async (report) => {
                            const end = await run(report, `ftplogin.add.${domainId}`, 'POST', url, { login: name, folder, password }, t('ftpLogins.added', { login: name }));
                            if (end.state === 'done') {
                                setAdding(false);
                                setName('');
                                setPassword('');
                                setFolder('public_html');
                            }
                            return end;
                        },
                    });
                };
                const change = (login: string, current: string) => {
                    const body: Record<string, string> = {};
                    const nextFolder = editFolder.trim();
                    if (nextFolder && '/' + nextFolder.replace(/^\/+/, '') !== current) body.folder = nextFolder;
                    if (editPassword) body.password = editPassword;
                    if (Object.keys(body).length === 0) return;
                    void confirmAction({
                        title: t('ftpLogins.confirmChangeTitle', { login }),
                        message: t('ftpLogins.confirmChange'),
                        confirmLabel: t('ftpLogins.saveChange'),
                        progressTitle: t('ftpLogins.progressChange', { login }),
                        action: async (report) => {
                            const end = await run(report, `ftplogin.change.${domainId}.${login}`, 'PUT', `${url}/${login}`, body, t('ftpLogins.changed', { login }));
                            if (end.state === 'done') {
                                setEditing(null);
                                setEditPassword('');
                            }
                            return end;
                        },
                    });
                };
                const remove = (login: string) => {
                    void confirmAction({
                        title: t('ftpLogins.confirmRemoveTitle', { login }),
                        message: t('ftpLogins.confirmRemove', { login }),
                        confirmLabel: t('ftpLogins.remove'),
                        tone: 'danger',
                        progressTitle: t('ftpLogins.progressRemove', { login }),
                        action: (report) => run(report, `ftplogin.remove.${domainId}.${login}`, 'DELETE', `${url}/${login}`, undefined, t('ftpLogins.removed', { login })),
                    });
                };

                return (
                    <section className="space-y-4 border-t border-border pt-5" aria-labelledby="ftp-logins-title">
                        <div>
                            <div className="flex items-center gap-2">
                                <FolderLock className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                <h3 id="ftp-logins-title" className="text-base font-semibold text-fg">{t('ftpLogins.title')}</h3>
                            </div>
                            <p className="mt-1 text-sm text-fg-muted">{t('ftpLogins.desc')}</p>
                        </div>
                        {blockers.length > 0 && <div className="space-y-2">{blockers}</div>}
                        {problem && <Notice tone="danger">{problem}</Notice>}
                        {warnings.map((w) => <Notice key={w} tone="warning">{t(`ftpLogins.warn.${w}` as TranslationKey)}</Notice>)}
                        {view.available && view.fileAccess && (
                            <p className="text-sm text-fg-muted">
                                {view.ceiling === null ? t('ftpLogins.limitNone', { used: view.used }) : t('ftpLogins.limit', { used: view.used, ceiling: view.ceiling })}
                            </p>
                        )}
                        {full && canOpen && (
                            <Notice tone="info">
                                {managed && view.owner ? t('ftpLogins.limitReachedManager', { name: view.owner }) : t('ftpLogins.limitReached')}
                                {view.owner && <>{' '}<button type="button" onClick={() => navigate(`/users?rights=${encodeURIComponent(view.owner)}`)} className="font-semibold text-primary underline underline-offset-2 max-sm:min-h-11">{t('ftpLogins.openRights', { name: view.owner })}</button></>}
                            </Notice>
                        )}

                        {view.available && (view.rows.length === 0 ? (
                            <p className="text-sm text-fg-subtle">{t('ftpLogins.none')}</p>
                        ) : (
                            <ul className="space-y-2">
                                {view.rows.map((row) => {
                                    const stateWords = row.state === 'ready' ? t('ftpLogins.state.ready')
                                        : row.state === 'no_password' ? t('ftpLogins.state.noPassword')
                                            : row.state === 'suspended' ? t('ftpLogins.state.suspended')
                                                : row.state === 'unfinished' ? t('ftpLogins.state.unfinished')
                                                    : row.state === 'not_on_server' ? t('ftpLogins.state.notOnServer')
                                                        : row.state === 'jail' ? t('ftpLogins.state.jail', { reason: row.reason })
                                                            : t('common.resultUnknown');
                                    const healthy = row.state === 'ready';
                                    const open = editing === row.login;
                                    return (
                                        <li key={row.login} className="rounded-xl border border-border bg-surface p-3 space-y-2">
                                            <div className="flex flex-wrap items-center justify-between gap-2">
                                                <div className="min-w-0">
                                                    <div className="flex items-center gap-2 text-sm font-medium text-fg">
                                                        <UserRound className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                                                        <span className="font-mono break-all">{row.login}</span>
                                                    </div>
                                                    {row.folder && <p className="text-xs text-fg-muted font-mono break-all">{t('ftpLogins.folderLabel', { folder: folderShown(row.folder) })}</p>}
                                                    <p className={`text-xs ${healthy ? 'text-success' : 'text-warning'}`}>{stateWords}</p>
                                                    {row.state === 'jail' && isAdmin && <p className="text-xs text-fg-muted font-mono break-all">{t('ftpLogins.state.jailAdmin', { unit: `celikpanel-ftp-${row.login}` })}</p>}
                                                    {row.readOnly && <p className="text-xs text-fg-muted">{t('ftpLogins.state.readOnly')}</p>}
                                                </div>
                                                <div className="flex flex-wrap gap-2">
                                                    {row.state !== 'unfinished' && row.state !== 'not_on_server' && (
                                                        <Button disabled={!canOpen} onClick={() => { setEditing(open ? null : row.login); setEditFolder(folderShown(row.folder)); setEditPassword(''); }}>
                                                            {t('ftpLogins.change')}
                                                        </Button>
                                                    )}
                                                    <Button variant="danger" icon={Trash2} disabled={!settled || busy || view.stateError !== ''} onClick={() => remove(row.login)}>
                                                        {t('ftpLogins.remove')}
                                                    </Button>
                                                </div>
                                            </div>
                                            {open && (
                                                <div className="space-y-3 border-t border-border pt-3">
                                                    <label className="block space-y-1 text-sm font-medium text-fg">
                                                        <span>{t('ftpLogins.newFolder')}</span>
                                                        <input className={inputClass} value={editFolder} maxLength={512} spellCheck={false} disabled={!canOpen}
                                                            onChange={(e) => setEditFolder(e.target.value)} />
                                                    </label>
                                                    <label className="block space-y-1 text-sm font-medium text-fg">
                                                        <span>{t('ftpLogins.newPassword')}</span>
                                                        <PasswordInput field="file_access" value={editPassword} maxLength={128} disabled={!canOpen} onChange={setEditPassword} />
                                                    </label>
                                                    <div className="flex flex-wrap gap-2">
                                                        <Button variant="primary" disabled={!canOpen || (editPassword !== '' && !passwordAcceptable(editPassword, 'file_access'))}
                                                            onClick={() => change(row.login, row.folder)}>
                                                            {t('ftpLogins.saveChange')}
                                                        </Button>
                                                        <Button onClick={() => setEditing(null)}>{t('ftpLogins.cancel')}</Button>
                                                    </div>
                                                </div>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                        ))}

                        {canOpen && !full && !adding && (
                            <Button icon={Plus} onClick={() => setAdding(true)}>{t('ftpLogins.add')}</Button>
                        )}
                        {canOpen && adding && (
                            <div className="rounded-xl border border-border bg-surface p-4 space-y-3">
                                <label className="block space-y-1 text-sm font-medium text-fg">
                                    <span>{t('ftpLogins.name')}</span>
                                    <input className={inputClass} value={name} maxLength={32} spellCheck={false} autoComplete="off"
                                        onChange={(e) => setName(e.target.value.toLowerCase().trim())} />
                                    <span className="block text-xs font-normal text-fg-muted">{t('ftpLogins.nameHint')}</span>
                                </label>
                                <label className="block space-y-1 text-sm font-medium text-fg">
                                    <span>{t('ftpLogins.folder')}</span>
                                    <input className={inputClass} value={folder} maxLength={512} spellCheck={false}
                                        onChange={(e) => setFolder(e.target.value)} />
                                    <span className="block text-xs font-normal text-fg-muted">{t('ftpLogins.folderHint')}</span>
                                </label>
                                <label className="block space-y-1 text-sm font-medium text-fg">
                                    <span>{t('ftpLogins.password')}</span>
                                    <PasswordInput field="file_access" value={password} maxLength={128} onChange={setPassword} />
                                </label>
                                <div className="flex flex-wrap gap-2">
                                    <Button variant="primary" disabled={!nameOK || !passwordOK || busy} onClick={add}>{t('ftpLogins.save')}</Button>
                                    <Button onClick={() => { setAdding(false); setProblem(''); }}>{t('ftpLogins.cancel')}</Button>
                                </div>
                            </div>
                        )}

                        {view.available && view.fileAccess && view.rows.length > 0 && (
                            <div className="rounded-xl border border-border bg-surface-muted p-4 text-sm text-fg-muted space-y-1">
                                <p className="font-medium text-fg">{t('ftpLogins.connect')}</p>
                                <p>{t('ftpLogins.connectHost', { host: view.connection.host })}</p>
                                <p>{t('ftpLogins.connectUser')}</p>
                                <p>{t('ftpLogins.connectSftp', { port: view.connection.sftpPort })}</p>
                                {view.ftpInstalled && view.ftpConfig === 'managed' && (
                                    <p>{t('ftpLogins.connectFtp', { port: view.connection.ftpPort, min: view.connection.passiveMin, max: view.connection.passiveMax })}</p>
                                )}
                                {!view.ftpInstalled && <p>{t('ftpLogins.block.ftpMissing')}</p>}
                                {view.ftpInstalled && view.ftpConfig === 'owner' && <p>{t('ftpLogins.block.ftpOwner')}</p>}
                                <p>{t('ftpLogins.connectFolder')}</p>
                            </div>
                        )}
                    </section>
                );
            }}
        </RemoteGate>
    );
}
