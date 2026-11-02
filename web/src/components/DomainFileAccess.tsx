import { useState, type ReactNode } from 'react';
import { KeyRound, Plug, ShieldCheck, Trash2 } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, RemoteGate, ResultUnknown, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { decodeList, useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';

// The site's file access (alpha.117): FTP over TLS, SFTP and, when the plan
// gives the command line, a jailed SSH. The account is the site's own system
// account, so uploaded files keep the owner the site runs as. The server's
// own OpenSSH and vsftpd do the work, so access keeps working without
// CelikPanel. Problems come first, then the switch, then the recipe.
//
// Sitenin dosya erişimi: TLS'li FTP, SFTP ve plan izin verirse hapsedilmiş
// SSH. Önce sorunlar, sonra ayar, en sonda bağlantı tarifi.

interface AccessKey {
    type: string;
    fingerprint: string;
    comment: string;
}

interface AccessView {
    domain: string;
    available: boolean;
    subscriptionStatus: string;
    rights: { fileAccess: boolean; ssh: boolean };
    connection: {
        ftpHost: string;
        sftpHost: string;
        serverIP: string;
        ftpPort: number;
        sftpPort: number;
        username: string;
        startDir: string;
        passiveMin: number;
        passiveMax: number;
    };
    stateError: string;
    account: null | {
        enabled: boolean;
        ssh: boolean;
        password: string;
        exists: boolean;
        homeMatches: boolean;
        keysFile: string;
        keys: AccessKey[];
    };
    server: null | {
        sshdPresent: boolean;
        sshDropIn: string;
        passwordAuthentication: string;
        ftpInstalled: boolean;
        ftpConfig: string;
        ftpCertificate: string;
        ftpCertificateSHA256: string;
        fail2banInstalled: boolean;
        siteUsersWithShell: number;
    };
}

const str = (value: unknown): string => (typeof value === 'string' ? value : '');
const num = (value: unknown): number => (typeof value === 'number' ? value : 0);

function decodeAccess(raw: unknown): AccessView {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    const rights = (body.rights ?? {}) as Record<string, unknown>;
    const connection = (body.connection ?? {}) as Record<string, unknown>;
    if (typeof body.available !== 'boolean' || typeof rights.file_access !== 'boolean') throw new Error('field');
    const state = body.state as Record<string, unknown> | undefined;
    const account = state?.account as Record<string, unknown> | undefined;
    const server = state?.server as Record<string, unknown> | undefined;
    return {
        domain: str(body.domain),
        available: body.available,
        subscriptionStatus: str(body.subscription_status) || 'active',
        rights: { fileAccess: rights.file_access, ssh: rights.ssh === true },
        connection: {
            ftpHost: str(connection.ftp_host),
            sftpHost: str(connection.sftp_host),
            serverIP: str(connection.server_ip),
            ftpPort: num(connection.ftp_port) || 21,
            sftpPort: num(connection.sftp_port) || 22,
            username: str(connection.username),
            startDir: str(connection.start_dir),
            passiveMin: num(connection.passive_min),
            passiveMax: num(connection.passive_max),
        },
        stateError: str(body.state_error),
        account: account ? {
            enabled: account.enabled === true,
            ssh: account.ssh === true,
            password: str(account.password),
            exists: account.exists === true,
            homeMatches: account.home_matches === true,
            keysFile: str(account.keys_file),
            keys: decodeList<Record<string, unknown>>(account.keys ?? []).map((key) => ({
                type: str(key.type), fingerprint: str(key.fingerprint), comment: str(key.comment),
            })),
        } : null,
        server: server ? {
            sshdPresent: server.sshd_present === true,
            sshDropIn: str(server.ssh_drop_in),
            passwordAuthentication: str(server.password_authentication),
            ftpInstalled: server.ftp_installed === true,
            ftpConfig: str(server.ftp_config),
            ftpCertificate: str(server.ftp_certificate),
            ftpCertificateSHA256: str(server.ftp_certificate_sha256),
            fail2banInstalled: server.fail2ban_installed === true,
            siteUsersWithShell: num(server.site_users_with_shell),
        } : null,
    };
}

interface Change {
    kind: string;
    target: string;
    action: string;
    detail: string;
}

interface Preview {
    refusal: string;
    detail: string;
    changes: Change[];
    warnings: string[];
}

function decodePreview(raw: unknown): Preview {
    const body = (raw ?? {}) as Record<string, unknown>;
    return {
        refusal: str(body.refusal),
        detail: str(body.detail),
        changes: decodeList<Record<string, unknown>>(body.changes ?? []).map((change) => ({
            kind: str(change.kind), target: str(change.target), action: str(change.action), detail: str(change.detail),
        })),
        warnings: decodeList<unknown>(body.warnings ?? []).map(str).filter(Boolean),
    };
}

function Notice({ tone, children }: { tone: 'warning' | 'danger' | 'info'; children: ReactNode }) {
    const colors = tone === 'danger'
        ? 'border-danger/40 bg-danger/10'
        : tone === 'warning' ? 'border-warning/40 bg-warning/10' : 'border-border bg-surface-2';
    return <p className={`rounded-lg border px-3 py-2 text-sm text-fg ${colors}`}>{children}</p>;
}

export function DomainFileAccess({ domainId, isAdmin = false }: { domainId: number; domainName: string; isAdmin?: boolean }) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/access`;
    const access = useRemote(url, decodeAccess);
    const answer = useLostAnswer(() => access.retry());
    const [enabled, setEnabled] = useState<boolean | null>(null);
    const [ssh, setSSH] = useState<boolean | null>(null);
    const [password, setPassword] = useState('');
    const [newKey, setNewKey] = useState('');
    const [removeKeys, setRemoveKeys] = useState<string[]>([]);
    const [preview, setPreview] = useState<Preview | null>(null);
    const [busy, setBusy] = useState(false);
    const [problem, setProblem] = useState('');

    const reset = () => {
        setEnabled(null);
        setSSH(null);
        setPassword('');
        setNewKey('');
        setRemoveKeys([]);
        setPreview(null);
        setProblem('');
    };

    return (
        <RemoteGate
            remote={access.remote}
            checking={t('fileAccess.checking')}
            failed={t('fileAccess.unknown')}
            onRetry={() => void access.retry()}
            busy={access.reading}
        >
            {(shown) => {
                const view = shown.value;
                const settled = access.remote.state === 'known' && !access.reading && !answer.holding && !shown.stale;
                const current = view.account;
                const wantEnabled = enabled ?? current?.enabled ?? false;
                const wantSSH = wantEnabled && (ssh ?? current?.ssh ?? false);
                const body = {
                    enabled: wantEnabled,
                    ssh: wantSSH,
                    password: wantEnabled ? password : '',
                    add_keys: wantEnabled && newKey.trim() ? [newKey.trim()] : [],
                    remove_keys: removeKeys,
                };
                const dirty = current !== null && (wantEnabled !== current.enabled || wantSSH !== current.ssh
                    || body.password !== '' || body.add_keys.length > 0 || removeKeys.length > 0);
                const canChange = settled && !busy && current !== null && view.subscriptionStatus === 'active' && view.rights.fileAccess;

                const send = async (method: 'POST' | 'PUT') => {
                    if (body.password && (body.password.length < 8 || body.password.length > 128)) {
                        setProblem(t('fileAccess.check.password'));
                        return;
                    }
                    setProblem('');
                    setBusy(true);
                    try {
                        const target = method === 'POST' ? `${url}/preview` : url;
                        const init = { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
                        // A preview only reads: when its answer is lost, the
                        // person is told and nothing is repeated. A change goes
                        // through the lost-answer guard (D-024).
                        let res: Response | null;
                        if (method === 'POST') {
                            try {
                                res = await fetch(target, init);
                            } catch {
                                setProblem(t('fileAccess.previewUnknown'));
                                return;
                            }
                        } else {
                            res = await answer.send(target, init);
                        }
                        if (!res) return;
                        if (!res.ok) {
                            const error = await readApiError(res);
                            setProblem(error.message && error.code === 'FILE_ACCESS_REFUSED'
                                ? `${apiErrorText(error, t)} ${error.message}` : apiErrorText(error, t));
                            setPreview(null);
                            return;
                        }
                        let raw: unknown;
                        try {
                            raw = await res.json();
                        } catch {
                            if (method === 'PUT') answer.lose();
                            else setProblem(t('fileAccess.previewUnknown'));
                            return;
                        }
                        const answered = decodePreview(raw);
                        if (method === 'POST') {
                            setPreview(answered);
                            return;
                        }
                        answer.settle();
                        for (const warning of answered.warnings) {
                            const key = `fileAccess.warn.${warning}` as TranslationKey;
                            const text = t(key);
                            if (text !== key) showToast('warning', text);
                        }
                        showToast('success', t(wantEnabled ? 'fileAccess.saved' : 'fileAccess.closed'));
                        reset();
                        await access.retry();
                    } finally {
                        setBusy(false);
                    }
                };

                return (
                    <div className="space-y-5">
                        <div>
                            <div className="flex items-center gap-2">
                                <Plug className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                <h3 className="text-base font-semibold text-fg">{t('fileAccess.title')}</h3>
                            </div>
                            <p className="mt-1 text-sm text-fg-muted">{t('fileAccess.desc')}</p>
                        </div>
                        <ResultUnknown answer={answer} />
                        <AccessBlockers view={view} isAdmin={isAdmin} />

                        {current && view.available && (
                            <div className="rounded-xl border border-border bg-surface p-4 space-y-4">
                                <label className="flex items-start gap-3">
                                    <input
                                        type="checkbox"
                                        className="mt-0.5 h-4 w-4 accent-primary"
                                        checked={wantEnabled}
                                        disabled={!canChange && !(current.enabled && settled)}
                                        onChange={(e) => { setEnabled(e.target.checked); setPreview(null); }}
                                    />
                                    <span>
                                        <span className="block text-sm font-medium text-fg">{t('fileAccess.enable')}</span>
                                        <span className="block text-xs text-fg-subtle">{t('fileAccess.enableHint')}</span>
                                    </span>
                                </label>
                                {wantEnabled && (
                                    <>
                                        <label className="flex items-start gap-3">
                                            <input
                                                type="checkbox"
                                                className="mt-0.5 h-4 w-4 accent-primary"
                                                checked={wantSSH}
                                                disabled={!canChange || !view.rights.ssh}
                                                onChange={(e) => { setSSH(e.target.checked); setPreview(null); }}
                                            />
                                            <span>
                                                <span className="block text-sm font-medium text-fg">{t('fileAccess.ssh')}</span>
                                                <span className="block text-xs text-fg-subtle">
                                                    {t(view.rights.ssh ? 'fileAccess.sshHint' : 'fileAccess.sshNotInPlan')}
                                                </span>
                                            </span>
                                        </label>
                                        <label className="block space-y-1 text-sm font-medium text-fg">
                                            <span>{t(current.password === 'set' ? 'fileAccess.passwordReset' : 'fileAccess.passwordSet')}</span>
                                            <input
                                                type="password"
                                                value={password}
                                                autoComplete="new-password"
                                                disabled={!canChange}
                                                onChange={(e) => { setPassword(e.target.value); setPreview(null); }}
                                                className={inputClass}
                                            />
                                            <span className="block text-xs font-normal text-fg-muted">
                                                {t(current.password === 'set' ? 'fileAccess.passwordIsSet' : 'fileAccess.passwordNone')}
                                            </span>
                                        </label>
                                        <div className="space-y-2">
                                            <div className="flex items-center gap-2 text-sm font-medium text-fg">
                                                <KeyRound className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                                                {t('fileAccess.keys')}
                                            </div>
                                            {current.keys.length === 0 ? (
                                                <p className="text-sm text-fg-subtle">{t('fileAccess.keysNone')}</p>
                                            ) : (
                                                <ul className="space-y-1">
                                                    {current.keys.map((key) => (
                                                        <li key={key.fingerprint} className="flex items-center justify-between gap-2 rounded-lg border border-border px-3 py-2 text-xs">
                                                            <span className={`font-mono break-all ${removeKeys.includes(key.fingerprint) ? 'line-through text-fg-subtle' : 'text-fg'}`}>
                                                                {key.type} {key.fingerprint} {key.comment}
                                                            </span>
                                                            <Button
                                                                icon={Trash2}
                                                                disabled={!canChange}
                                                                aria-label={t('fileAccess.keyRemove')}
                                                                onClick={() => {
                                                                    setRemoveKeys((list) => list.includes(key.fingerprint)
                                                                        ? list.filter((item) => item !== key.fingerprint) : [...list, key.fingerprint]);
                                                                    setPreview(null);
                                                                }}
                                                            >
                                                                {t('fileAccess.keyRemove')}
                                                            </Button>
                                                        </li>
                                                    ))}
                                                </ul>
                                            )}
                                            <textarea
                                                value={newKey}
                                                rows={2}
                                                spellCheck={false}
                                                disabled={!canChange}
                                                placeholder="ssh-ed25519 AAAA… you@laptop"
                                                onChange={(e) => { setNewKey(e.target.value); setPreview(null); }}
                                                className={`${inputClass} font-mono text-xs`}
                                            />
                                            <span className="block text-xs text-fg-muted">{t('fileAccess.keyHint')}</span>
                                        </div>
                                    </>
                                )}
                                {problem && <p role="alert" className="text-sm text-danger">{problem}</p>}
                                {preview && (
                                    <div className="rounded-lg border border-border bg-surface-2 p-3 text-sm">
                                        <p className="font-medium text-fg">{t('fileAccess.previewTitle')}</p>
                                        {preview.refusal ? (
                                            <p className="mt-1 text-danger">{preview.detail}</p>
                                        ) : preview.changes.length === 0 ? (
                                            <p className="mt-1 text-fg-muted">{t('fileAccess.previewNothing')}</p>
                                        ) : (
                                            <ul className="mt-1 list-disc space-y-0.5 pl-5 text-fg-muted">
                                                {preview.changes.map((change, index) => (
                                                    <li key={index}><span className="font-mono text-xs">{change.target}</span>: {change.action}{change.detail ? ` (${change.detail})` : ''}</li>
                                                ))}
                                            </ul>
                                        )}
                                    </div>
                                )}
                                <div className="flex flex-wrap gap-2">
                                    {!preview ? (
                                        <Button variant="primary" icon={ShieldCheck} loading={busy} disabled={!dirty || !settled} onClick={() => void send('POST')}>
                                            {t('fileAccess.review')}
                                        </Button>
                                    ) : (
                                        <Button variant="primary" icon={ShieldCheck} loading={busy} disabled={!dirty || !settled || preview.refusal !== ''} onClick={() => void send('PUT')}>
                                            {t('fileAccess.apply')}
                                        </Button>
                                    )}
                                    {dirty && <Button disabled={busy} onClick={reset}>{t('fileAccess.cancel')}</Button>}
                                </div>
                            </div>
                        )}

                        {current?.enabled && <AccessRecipe view={view} />}
                    </div>
                );
            }}
        </RemoteGate>
    );
}

// What stops access, or limits it, comes first: who acts and what they do.
function AccessBlockers({ view, isAdmin }: { view: AccessView; isAdmin: boolean }) {
    const { t } = useI18n();
    const notes: Array<{ tone: 'warning' | 'danger' | 'info'; key: TranslationKey; vars?: Record<string, string | number> }> = [];
    if (!view.available) notes.push({ tone: 'info', key: 'fileAccess.block.noSite' });
    else if (!view.rights.fileAccess) notes.push({ tone: 'warning', key: 'fileAccess.block.notInPlan' });
    if (view.available && view.subscriptionStatus !== 'active') notes.push({ tone: 'danger', key: 'fileAccess.block.suspended' });
    if (view.stateError === 'agent_upgrade_required') notes.push({ tone: 'warning', key: 'fileAccess.block.agentUpgrade' });
    if (view.stateError === 'agent_unavailable') notes.push({ tone: 'warning', key: 'fileAccess.block.agentUnavailable' });
    const account = view.account;
    if (account && !account.exists) notes.push({ tone: 'danger', key: 'fileAccess.block.noAccount' });
    else if (account && !account.homeMatches) notes.push({ tone: 'danger', key: 'fileAccess.block.accountMismatch' });
    const server = view.server;
    if (server) {
        if (!server.sshdPresent) notes.push({ tone: 'danger', key: 'fileAccess.block.noSSHD' });
        if (server.sshDropIn === 'owner') notes.push({ tone: 'danger', key: 'fileAccess.block.sshOwner' });
        if (account?.keysFile === 'owner') notes.push({ tone: 'warning', key: 'fileAccess.block.keysOwner' });
        if (server.passwordAuthentication === 'no') notes.push({ tone: 'warning', key: 'fileAccess.block.passwordAuthOff' });
        if (!server.ftpInstalled) notes.push({ tone: 'info', key: isAdmin ? 'fileAccess.block.ftpMissingAdmin' : 'fileAccess.block.ftpMissing' });
        else if (server.ftpConfig === 'owner') notes.push({ tone: 'info', key: 'fileAccess.block.ftpOwner' });
        if (server.ftpCertificate === 'self_signed') notes.push({ tone: 'info', key: 'fileAccess.block.selfSigned', vars: { sha: server.ftpCertificateSHA256 } });
        if (isAdmin && !server.fail2banInstalled) notes.push({ tone: 'info', key: 'fileAccess.block.fail2ban' });
        if (isAdmin && server.siteUsersWithShell > 0) notes.push({ tone: 'warning', key: 'fileAccess.block.shells', vars: { count: server.siteUsersWithShell } });
    }
    if (notes.length === 0) return null;
    return (
        <div className="space-y-2">
            {notes.map((note) => <Notice key={note.key} tone={note.tone}>{t(note.key, note.vars)}</Notice>)}
        </div>
    );
}

// The connection recipe: what goes into FileZilla's Site Manager.
function AccessRecipe({ view }: { view: AccessView }) {
    const { t } = useI18n();
    const c = view.connection;
    const rows: Array<[TranslationKey, string]> = [
        ['fileAccess.recipe.ftpHost', `${c.ftpHost}:${c.ftpPort}`],
        ['fileAccess.recipe.sftpHost', `${c.sftpHost}:${c.sftpPort}`],
        ['fileAccess.recipe.username', c.username],
        ['fileAccess.recipe.folder', c.startDir],
    ];
    return (
        <div className="rounded-xl border border-border bg-surface p-4 text-sm">
            <h4 className="font-semibold text-fg">{t('fileAccess.recipe.title')}</h4>
            <dl className="mt-2 grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-[max-content_1fr]">
                {rows.map(([key, value]) => (
                    <div key={key} className="contents">
                        <dt className="text-fg-muted">{t(key)}</dt>
                        <dd className="font-mono text-fg break-all">{value}</dd>
                    </div>
                ))}
            </dl>
            <ol className="mt-3 list-decimal space-y-1 pl-5 text-fg-muted">
                <li>{t('fileAccess.recipe.step1')}</li>
                <li>{t('fileAccess.recipe.step2', { host: c.ftpHost })}</li>
                <li>{t('fileAccess.recipe.step3', { username: c.username })}</li>
                <li>{t('fileAccess.recipe.step4', { folder: c.startDir })}</li>
                <li>{t('fileAccess.recipe.step5', { host: c.sftpHost, port: c.sftpPort })}</li>
            </ol>
            <p className="mt-2 text-xs text-fg-subtle">
                {t('fileAccess.recipe.note', { min: c.passiveMin, max: c.passiveMax, ip: c.serverIP || '—' })}
            </p>
        </div>
    );
}
