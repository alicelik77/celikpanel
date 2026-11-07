import { useState } from 'react';
import { KeyRound, Play, Plus, Trash2 } from 'lucide-react';
import { Button, inputClass } from './ui';
import { readRemote, useRemote } from '../lib/remote';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { confirmAction, followOperation, type OperationEnd, type OperationReport } from '../lib/confirmAction';
import { followRequestProgress } from '../lib/requestProgress';
import { useLostAnswer } from '../lib/lostAnswer';
import { useWebWords, type WebWords } from '../i18n/webSettings';
import type { TranslationKey } from '../i18n/en';

// A Node.js site's environment variables and package scripts (alpha.128,
// PLESK-GAP G19), on its Hosting tab once the application runs in the site
// sandbox. Fetched when shown, never with the domain page itself.
//
// Environment: names and values the application reads; the server keeps them
// in one root-owned file and never sends a secret back, so a saved secret
// shows as "Saved (hidden)" and an empty field keeps it. Saving follows the
// server's steps and restarts a running application.
//
// Package scripts: one of package.json's scripts (no free command box), run
// as the site's own account in the sandbox; the progress window shows its
// last output lines while it runs.
//
// Node.js sitesinin ortam değişkenleri ve paket betikleri.

type Words = (key: TranslationKey, values?: Record<string, string | number>) => string;
const asWords = (t: WebWords) => t as unknown as Words;

interface EnvEntry { key: string; value?: string; secret: boolean }
interface EnvView { entries: EnvEntry[]; min_secret: number }
interface Row { key: string; value: string; secret: boolean; saved: boolean }

function decodeEnv(raw: unknown): EnvView {
    const body = raw as EnvView | null;
    if (!body || !Array.isArray(body.entries) || typeof body.min_secret !== 'number') throw new Error('shape');
    return body;
}

interface InstallState { phase: string; lines?: string[]; reason?: string; error?: string; scripts?: string[]; script?: string }
function decodeInstall(raw: unknown): InstallState {
    if (!raw || typeof raw !== 'object' || typeof (raw as { phase?: unknown }).phase !== 'string') throw new Error('shape');
    return raw as InstallState;
}

const KEY = /^[A-Z][A-Z0-9_]{0,63}$/;
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
function newRequestID(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return 'node-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function NodeSiteTools({ domainId, domainName }: { domainId: number; domainName: string }) {
    return (
        <div className="space-y-4">
            <NodeEnv domainId={domainId} domainName={domainName} />
            <NodeScripts domainId={domainId} domainName={domainName} />
        </div>
    );
}

function NodeEnv({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useWebWords();
    const url = `/api/v1/domains/${domainId}/app/env`;
    const env = useRemote(url, decodeEnv);
    const answer = useLostAnswer(() => env.retry());
    const [rows, setRows] = useState<Row[] | null>(null);
    const [problem, setProblem] = useState('');
    if (env.remote.state !== 'known') return null;
    const view = env.remote.value;
    const shown: Row[] = rows ?? view.entries.map((e) => ({ key: e.key, value: e.value ?? '', secret: e.secret, saved: e.secret }));
    const set = (next: Row[]) => setRows(next);

    const save = async () => {
        for (const row of shown) {
            const keeps = row.secret && row.saved && row.value === '';
            if (!KEY.test(row.key.trim()) || (!keeps && (row.value === '' || /[\u0000-\u001f\u007f]/.test(row.value))) ||
                (row.secret && !keeps && row.value.length < view.min_secret)) {
                setProblem(t('node.env.problem', { min: view.min_secret }));
                return;
            }
        }
        setProblem('');
        const entries = shown.map((row) => ({ key: row.key.trim(), value: row.value, secret: row.secret }));
        await followOperation({
            progressTitle: t('node.env.progress', { name: domainName }),
            action: async (report): Promise<OperationEnd> => {
                const progress = followRequestProgress(report, [
                    { id: 'check', label: t('node.env.step.check'), check: true },
                    { id: 'save', label: t('node.env.step.save') },
                    { id: 'unit', label: t('node.env.step.unit') },
                    { id: 'restart', label: t('node.env.step.restart') },
                ], asWords(t), `nodeenv.${domainId}`);
                try {
                    const res = await answer.send(url, { method: 'POST', headers: { 'Content-Type': 'application/json', ...progress.headers }, body: JSON.stringify({ entries }) });
                    progress.settle(res);
                    if (!res) return { state: 'unknown', reason: t('node.env.failed') };
                    if (!res.ok) {
                        const error: ApiError = await readApiError(res);
                        await env.retry();
                        return { state: res.status === 409 ? 'blocked' : 'failed', reason: `${apiErrorText(error, asWords(t))}${error.message ? ` (${error.message})` : ''}` };
                    }
                    const body = await res.json().catch(() => ({})) as { restarted?: boolean };
                    answer.settle();
                    setRows(null);
                    await env.retry();
                    return { state: 'done', message: body.restarted ? t('node.env.saved') : t('node.env.savedStopped') };
                } finally {
                    progress.settle(null);
                }
            },
        });
    };

    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            <div className="flex items-center gap-2">
                <KeyRound className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                <h4 className="text-sm font-semibold text-fg">{t('node.env.title')}</h4>
            </div>
            <p className="mt-0.5 text-xs text-fg-muted">{t('node.env.desc')}</p>
            <div className="mt-3 space-y-2">
                {shown.map((row, at) => (
                    <div key={at} className="flex flex-col gap-2 sm:flex-row sm:items-center">
                        <input value={row.key} onChange={(e) => set(shown.map((r, i) => i === at ? { ...r, key: e.target.value.toUpperCase() } : r))} placeholder={t('node.env.key')} aria-label={t('node.env.key')} spellCheck={false} className={`${inputClass} font-mono sm:w-64`} />
                        <input value={row.value} type={row.secret ? 'password' : 'text'} autoComplete="new-password" onChange={(e) => set(shown.map((r, i) => i === at ? { ...r, value: e.target.value } : r))}
                            placeholder={row.secret && row.saved ? t('node.env.secretKept') : t('node.env.value')} aria-label={t('node.env.value')} spellCheck={false} className={`${inputClass} font-mono`} />
                        <label className="flex items-center gap-2 text-sm text-fg"><input type="checkbox" className="accent-primary" checked={row.secret} onChange={(e) => set(shown.map((r, i) => i === at ? { ...r, secret: e.target.checked, saved: false } : r))} />{t('node.env.secret')}</label>
                        <Button type="button" icon={Trash2} onClick={() => set(shown.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>
                    </div>
                ))}
            </div>
            {problem && <p role="alert" className="mt-2 text-sm text-danger">{problem}</p>}
            <div className="mt-3 flex flex-wrap justify-between gap-2">
                <Button type="button" icon={Plus} onClick={() => set([...shown, { key: '', value: '', secret: false, saved: false }])}>{t('node.env.add')}</Button>
                <Button type="button" variant="primary" disabled={rows === null || answer.holding} onClick={() => void save()}>{t('node.env.save')}</Button>
            </div>
        </div>
    );
}

function NodeScripts({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useWebWords();
    const url = `/api/v1/domains/${domainId}/app/packages`;
    const state = useRemote(url, decodeInstall);
    const [picked, setPicked] = useState('');
    const [requestID, setRequestID] = useState(newRequestID);
    if (state.remote.state !== 'known') return null;
    const scripts = state.remote.value.scripts ?? [];
    const script = picked || scripts[0] || '';

    // Follows the step until it ends; it only reads.
    const follow = async (report: OperationReport, since: number): Promise<OperationEnd> => {
        for (;;) {
            const read = await readRemote(url, decodeInstall);
            if (read.state !== 'known') return { state: 'unknown', reason: apiErrorText(read.reason, asWords(t)) };
            const now = read.value;
            const note = now.lines?.slice(-4).join(' · ');
            const label = t('node.scripts.step', { script });
            if (now.phase === 'running') {
                report.steps([{ id: 'npm', label, state: 'running', since, note }]);
                await pause(2000);
                continue;
            }
            report.steps([{ id: 'npm', label, state: now.phase === 'succeeded' ? 'done' : 'failed', note }]);
            if (now.phase === 'succeeded') return { state: 'done', message: t('node.scripts.done') };
            return { state: 'failed', reason: now.error || t('node.scripts.failed') };
        }
    };

    const run = async () => {
        await confirmAction({
            title: t('node.scripts.confirm', { script, site: domainName }),
            message: t('node.scripts.confirmBody'),
            confirmLabel: t('node.scripts.run'),
            progressTitle: t('node.scripts.progress', { script, site: domainName }),
            action: async (report): Promise<OperationEnd> => {
                const since = Date.now();
                report.steps([{ id: 'npm', label: t('node.scripts.step', { script }), state: 'running', since }]);
                try {
                    const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: requestID, script }) });
                    if (!res.ok) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: apiErrorText(await readApiError(res), asWords(t)) };
                    }
                    const started = (await res.json()) as InstallState;
                    if (started.reason === 'install_running') return follow(report, since);
                    if (!started.phase) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: started.error || t('node.scripts.failed') };
                    }
                    const end = await follow(report, since);
                    setRequestID(newRequestID());
                    return end;
                } catch {
                    // A lost answer keeps the request id: a second press answers the first.
                    return { state: 'unknown', reason: t('nodesandbox.lost') };
                }
            },
        });
    };

    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            <h4 className="text-sm font-semibold text-fg">{t('node.scripts.title')}</h4>
            <p className="mt-0.5 text-xs text-fg-muted">{t('node.scripts.desc')}</p>
            {scripts.length === 0 ? (
                <p className="mt-2 text-sm text-fg-subtle">{t('node.scripts.none')}</p>
            ) : (
                <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                    <select value={script} onChange={(e) => setPicked(e.target.value)} className={`${inputClass} font-mono sm:w-72`} aria-label={t('node.scripts.title')}>
                        {scripts.map((name) => <option key={name} value={name}>{name}</option>)}
                    </select>
                    <Button type="button" icon={Play} onClick={() => void run()}>{t('node.scripts.run')}</Button>
                </div>
            )}
        </div>
    );
}
