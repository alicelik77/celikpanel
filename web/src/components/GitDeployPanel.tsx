import { useState } from 'react';
import { KeyRound } from 'lucide-react';
import { Button, inputClass } from './ui';
import { readRemote, useRemote } from '../lib/remote';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd, type OperationReport } from '../lib/confirmAction';
import { useWebWords } from '../i18n/webSettings';
import { useAuth } from '../auth/AuthContext';
import { useNavigate } from '../router';
import type { TranslationKey } from '../i18n/en';

// Git deploy of the site (alpha.128, PLESK-GAP G32): a repository (https://
// or SSH with the site's deploy key), one branch, and for a Node.js site an
// optional install and build. A deploy runs as the site's own account in the
// site sandbox; the window shows its real output while it runs. The deploy
// key is made on the server; only its public half is shown, once.
//
// Git ile yayın: depo, dal ve isteğe bağlı derleme; site korumasında çalışır.

interface Settings { repo_url: string; branch: string; build: boolean; key_created_at?: string }
interface Status { request_id?: string; phase: string; lines?: string[]; reason?: string; error?: string; key_fingerprint?: string }
interface GitView { settings: Settings; can_build: boolean; status: Status; status_unknown?: boolean }

function decodeGit(raw: unknown): GitView {
    const body = raw as GitView | null;
    if (!body || !body.settings || typeof body.can_build !== 'boolean' || !body.status) throw new Error('shape');
    return body;
}

const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
function newRequestID(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return 'git-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function GitDeployPanel({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useWebWords();
    const words = t as unknown as (key: TranslationKey, values?: Record<string, string | number>) => string;
    const url = `/api/v1/domains/${domainId}/git`;
    const view = useRemote(url, decodeGit);
    const { role } = useAuth();
    const navigate = useNavigate();
    // alpha.128 lab: git missing is the administrator's one click in Components.
    const gitFix = role === 'admin' ? { label: t('git.installGit'), run: () => navigate('/components?component=git&install=') } : undefined;
    const [edit, setEdit] = useState<Settings | null>(null);
    const [publicKey, setPublicKey] = useState('');
    const [problem, setProblem] = useState('');
    const [requestID, setRequestID] = useState(newRequestID);
    if (view.remote.state !== 'known') return null;
    const state = view.remote.value;
    const form = edit ?? state.settings;
    const fingerprint = state.status.key_fingerprint;
    const isSSH = /^(ssh:\/\/|[a-z_][a-z0-9_-]*@)/.test(form.repo_url.trim());

    const save = async () => {
        setProblem('');
        const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(form) });
        if (!res.ok) {
            const error = await readApiError(res);
            setProblem(`${apiErrorText(error, words)}${error.message ? ` (${error.message})` : ''}`);
            return;
        }
        setEdit(null);
        await view.retry();
    };

    const makeKey = async (replace: boolean) => {
        await confirmAction({
            title: t(replace ? 'git.key.replaceTitle' : 'git.key.makeTitle'),
            message: t(replace ? 'git.key.replaceBody' : 'git.key.makeBody'),
            confirmLabel: t(replace ? 'git.key.replace' : 'git.key.make'),
            tone: replace ? 'danger' : 'default',
            progressTitle: t('git.key.progress', { site: domainName }),
            action: async (): Promise<OperationEnd> => {
                const res = await fetch(`${url}/key`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ replace }) });
                if (!res.ok) return { state: 'failed', reason: apiErrorText(await readApiError(res), words) };
                const answer = (await res.json()) as { public_key: string };
                setPublicKey(answer.public_key);
                await view.retry();
                return { state: 'done', message: t('git.key.made') };
            },
        });
    };

    const follow = async (report: OperationReport, since: number): Promise<OperationEnd> => {
        for (;;) {
            const read = await readRemote(url, decodeGit);
            if (read.state !== 'known') return { state: 'unknown', reason: apiErrorText(read.reason, words) };
            const now = read.value.status;
            const note = now.lines?.slice(-4).join(' · ');
            if (now.phase === 'running') {
                report.steps([{ id: 'git', label: t('git.step', { branch: form.branch }), state: 'running', since, note }]);
                await pause(2000);
                continue;
            }
            report.steps([{ id: 'git', label: t('git.step', { branch: form.branch }), state: now.phase === 'succeeded' ? 'done' : 'failed', note }]);
            if (now.phase === 'succeeded') return { state: 'done', message: t('git.done') };
            return { state: now.reason === 'deploy_lost' ? 'unknown' : 'failed', reason: now.error || t('git.failed') };
        }
    };

    const deploy = async () => {
        await confirmAction({
            title: t('git.deployTitle', { site: domainName, branch: state.settings.branch }),
            message: t(state.settings.build ? 'git.deployBodyBuild' : 'git.deployBody'),
            confirmLabel: t('git.deploy'),
            progressTitle: t('git.progress', { site: domainName }),
            action: async (report): Promise<OperationEnd> => {
                const since = Date.now();
                report.steps([{ id: 'git', label: t('git.step', { branch: state.settings.branch }), state: 'running', since }]);
                try {
                    const res = await fetch(`${url}/deploy`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: requestID }) });
                    if (!res.ok) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: apiErrorText(await readApiError(res), words) };
                    }
                    const started = (await res.json()) as Status;
                    if (started.reason === 'deploy_running') return follow(report, since);
                    if (!started.phase) {
                        setRequestID(newRequestID());
                        if (started.reason === 'git_missing') {
                            return { state: 'blocked', reason: t(role === 'admin' ? 'git.missingAdmin' : 'git.missingOther'), fix: gitFix };
                        }
                        return { state: 'blocked', reason: started.error || t('git.failed') };
                    }
                    const end = await follow(report, since);
                    setRequestID(newRequestID());
                    return end;
                } catch {
                    // The same request id is kept: a second press answers the first.
                    return { state: 'unknown', reason: t('git.lost') };
                }
            },
        });
    };

    return (
        <div className="space-y-4" data-git-deploy="">
            <div>
                <div className="flex items-center gap-2">
                    <h3 className="text-base font-semibold text-fg">{t('git.title')}</h3>
                </div>
                <p className="mt-1 text-sm text-fg-muted">{t('git.desc')}</p>
            </div>
            <div className="rounded-xl border border-border bg-surface p-4 space-y-3">
                <label className="block text-sm font-medium text-fg">
                    <span>{t('git.repo')}</span>
                    <input value={form.repo_url} onChange={(e) => setEdit({ ...form, repo_url: e.target.value })} placeholder="https://github.com/acme/site.git" spellCheck={false} autoComplete="off" className={`${inputClass} mt-1 font-mono`} />
                    <span className="mt-1 block text-xs font-normal text-fg-muted">{t('git.repoHint')}</span>
                </label>
                <label className="block text-sm font-medium text-fg">
                    <span>{t('git.branch')}</span>
                    <input value={form.branch} onChange={(e) => setEdit({ ...form, branch: e.target.value })} spellCheck={false} autoComplete="off" className={`${inputClass} mt-1 font-mono sm:w-64`} />
                </label>
                {state.can_build && (
                    <label className="flex items-center gap-2 text-sm text-fg">
                        <input type="checkbox" className="accent-primary" checked={form.build} onChange={(e) => setEdit({ ...form, build: e.target.checked })} />
                        {t('git.build')}
                    </label>
                )}
                {problem && <p role="alert" className="text-sm text-danger">{problem}</p>}
                <div className="flex flex-wrap justify-end gap-2">
                    <Button type="button" disabled={edit === null} onClick={() => void save()}>{t('git.save')}</Button>
                    <Button type="button" variant="primary" disabled={edit !== null || !state.settings.repo_url} onClick={() => void deploy()}>{t('git.deploy')}</Button>
                </div>
            </div>
            {(isSSH || fingerprint) && (
                <div className="rounded-xl border border-border bg-surface p-4">
                    <div className="flex items-center gap-2">
                        <KeyRound className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                        <h4 className="text-sm font-semibold text-fg">{t('git.key.title')}</h4>
                    </div>
                    <p className="mt-1 text-xs text-fg-muted">{t('git.key.desc')}</p>
                    {fingerprint ? <p className="mt-2 font-mono text-xs text-fg">{fingerprint}</p> : <p className="mt-2 text-sm text-fg-subtle">{t('git.key.none')}</p>}
                    {publicKey && (
                        <div className="mt-2 rounded-lg border border-warning/40 bg-warning/10 p-3" role="status">
                            <p className="text-xs text-fg">{t('git.key.once')}</p>
                            <p className="mt-1 break-all font-mono text-xs text-fg">{publicKey}</p>
                        </div>
                    )}
                    <div className="mt-3"><Button type="button" onClick={() => void makeKey(Boolean(fingerprint))}>{fingerprint ? t('git.key.replace') : t('git.key.make')}</Button></div>
                </div>
            )}
            {state.status.phase && state.status.phase !== 'running' && (
                <p className={`text-sm ${state.status.phase === 'succeeded' ? 'text-fg-muted' : 'text-danger'}`}>
                    {state.status.phase === 'succeeded' ? t('git.lastOk') : (state.status.error || t('git.failed'))}
                </p>
            )}
        </div>
    );
}
