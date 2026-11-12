import { useState } from 'react';
import { Check, Copy, KeyRound, Plug, Trash2 } from 'lucide-react';
import { useI18n } from '../i18n';
import { apiTokensTranslation } from '../i18n/apiTokens';
import type { ApiTokensKey } from '../i18n/apiTokens/en';
import { readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';
import { useRemote } from '../lib/remote';
import { useAuth } from '../auth/AuthContext';
import { Link } from '../router';
import { Button, RemoteGate, inputClass } from './ui';
import { PasswordInput } from './PasswordInput';

// Remote API (alpha.128, G15; docs/REMOTE-API.md). Two pieces, one chunk:
// the "API tokens" card on My account (make a token with a name, a scope and
// an expiry; it is shown once with Copy; the list; Remove through the
// product's own confirmation) and the administrator's switch in Settings,
// Panel. The server decides who may make a token and says why not
// (refused_by); this screen only shows it, with who can change it.
//
// Uzak API: Hesabım'daki "API jetonları" kartı ve yöneticinin Ayarlar'daki
// anahtarı. Kimin jeton oluşturabileceğine sunucu karar verir; ekran yalnız
// gösterir.

export const API_TOKENS_URL = '/api/v1/account/api-tokens';
export const REMOTE_API_SETTING_URL = '/api/v1/panel/remote-api';

type Words = (key: ApiTokensKey, values?: Record<string, string | number>) => string;

function useWords(): { w: Words; locale: string } {
    const { locale } = useI18n();
    return { w: (key, values) => apiTokensTranslation(locale, key, values), locale };
}

export interface ApiTokenItem {
    id: number;
    name: string;
    prefix: string;
    scope: 'read' | 'manage';
    state: 'active' | 'expired' | 'ended';
    createdAt: number;
    expiresAt: number;
    lastUsedAt: number | null;
    lastUsedIp: string;
}

export interface ApiTokensState {
    serverEnabled: boolean;
    canCreate: boolean;
    refusedBy: string;
    grantorKind: string;
    grantorName: string;
    maxTokens: number;
    lifetimes: number[];
    /** Making a token asks for the two-factor code as well. */
    twoFactor: boolean;
    tokens: ApiTokenItem[];
}

const isObject = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

export function decodeApiTokens(raw: unknown): ApiTokensState {
    if (!isObject(raw) || typeof raw.server_enabled !== 'boolean' || typeof raw.can_create !== 'boolean' || !Array.isArray(raw.tokens)) {
        throw new Error('shape');
    }
    const tokens = raw.tokens.map((item): ApiTokenItem => {
        if (!isObject(item) || typeof item.id !== 'number' || typeof item.name !== 'string' || typeof item.prefix !== 'string'
            || typeof item.created_at !== 'number' || typeof item.expires_at !== 'number') throw new Error('shape');
        return {
            id: item.id,
            name: item.name,
            prefix: item.prefix,
            scope: item.scope === 'manage' ? 'manage' : 'read',
            state: item.state === 'active' ? 'active' : item.state === 'ended' ? 'ended' : 'expired',
            createdAt: item.created_at,
            expiresAt: item.expires_at,
            lastUsedAt: typeof item.last_used_at === 'number' ? item.last_used_at : null,
            lastUsedIp: typeof item.last_used_ip === 'string' ? item.last_used_ip : '',
        };
    });
    const lifetimes = Array.isArray(raw.lifetimes) && raw.lifetimes.every((n) => typeof n === 'number') ? raw.lifetimes as number[] : [7, 30, 90, 365];
    return {
        serverEnabled: raw.server_enabled,
        canCreate: raw.can_create,
        refusedBy: typeof raw.refused_by === 'string' ? raw.refused_by : '',
        grantorKind: typeof raw.grantor_kind === 'string' ? raw.grantor_kind : '',
        grantorName: typeof raw.grantor_name === 'string' ? raw.grantor_name : '',
        maxTokens: typeof raw.max_tokens === 'number' ? raw.max_tokens : 20,
        lifetimes,
        twoFactor: raw.two_factor === true,
        tokens,
    };
}

function grantorText(state: ApiTokensState, w: Words): string {
    return state.grantorKind === 'provider' && state.grantorName
        ? w('apitok.grantor.provider', { name: state.grantorName })
        : w('apitok.grantor.admin');
}

// The one sentence above the form: why no token can be made now, and who
// changes that. null when a token can be made.
export function refusalKey(state: ApiTokensState, isAdmin: boolean): ApiTokensKey | null {
    if (!state.serverEnabled) return isAdmin ? 'apitok.off.admin' : 'apitok.off.other';
    switch (state.refusedBy) {
        case '': return null;
        case 'impersonation': return 'apitok.refused.impersonation';
        case 'no_subscription': return 'apitok.refused.noSubscription';
        default: return 'apitok.refused.plan';
    }
}

function formatDate(seconds: number, locale: string): string {
    return new Date(seconds * 1000).toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric' });
}

async function readJSON(res: Response): Promise<unknown> {
    try {
        return await res.json();
    } catch {
        return null;
    }
}

// What a refused create says, in the screen's words where it has them.
async function createRefusal(res: Response, w: Words, max: number): Promise<string> {
    const error = await readApiError(res);
    switch (error.code) {
        case 'API_DISABLED': return w('apitok.error.disabled');
        case 'API_TOKEN_NOT_PERMITTED': return w('apitok.error.permission');
        case 'API_TOKEN_LIMIT': return w('apitok.limit', { n: max });
        case 'API_TOKEN_REQUEST_INVALID': return w('apitok.nameRequired');
        case 'API_TOKEN_REAUTH': return w('apitok.reauthFailed');
        case 'API_TOKEN_REAUTH_LIMITED': return w('apitok.reauthLimited');
        case 'REQUEST_COMPLETED_RESULT_NOT_RETAINED': return w('apitok.notRetained');
    }
    return error.message || w('apitok.error.generic');
}

export function ApiTokensCard() {
    const { w, locale } = useWords();
    const { role } = useAuth();
    const list = useRemote(API_TOKENS_URL, decodeApiTokens);
    const [name, setName] = useState('');
    const [scope, setScope] = useState<'read' | 'manage'>('read');
    const [days, setDays] = useState(90);
    const [password, setPassword] = useState('');
    const [code, setCode] = useState('');
    const [busy, setBusy] = useState(false);
    const [problem, setProblem] = useState('');
    const [made, setMade] = useState<{ token: string; name: string } | null>(null);
    const [copied, setCopied] = useState(false);

    const create = async (state: ApiTokensState) => {
        const trimmed = name.trim();
        if (!trimmed || trimmed.length > 64) {
            setProblem(w('apitok.nameRequired'));
            return;
        }
        if (state.tokens.filter((item) => item.state === 'active').length >= state.maxTokens) {
            setProblem(w('apitok.limit', { n: state.maxTokens }));
            return;
        }
        if (!password || (state.twoFactor && code.length !== 6)) {
            setProblem(w(state.twoFactor ? 'apitok.reauthNeededCode' : 'apitok.reauthNeeded'));
            return;
        }
        setBusy(true);
        setProblem('');
        try {
            const res = await fetch(API_TOKENS_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: trimmed, scope, expires_days: days, password, ...(state.twoFactor ? { code } : {}) }),
            });
            if (!res.ok) {
                setProblem(await createRefusal(res, w, state.maxTokens));
                return;
            }
            const body = await readJSON(res);
            const token = isObject(body) && typeof body.token === 'string' ? body.token : '';
            if (!token) {
                setProblem(w('apitok.unknown'));
                return;
            }
            setMade({ token, name: trimmed });
            setCopied(false);
            setName('');
            setPassword('');
            setCode('');
        } catch {
            // The answer did not arrive: whether a token was made is not known.
            setProblem(w('apitok.unknown'));
        } finally {
            setBusy(false);
            void list.retry();
        }
    };

    const copy = () => {
        if (!made) return;
        void navigator.clipboard?.writeText(made.token).then(() => setCopied(true), () => setCopied(false));
    };

    const remove = async (item: ApiTokenItem) => {
        await confirmAction({
            title: w('apitok.remove.title', { name: item.name }),
            message: w('apitok.remove.message'),
            confirmLabel: w('apitok.remove'),
            tone: 'danger',
            progressTitle: w('apitok.remove.progress'),
            action: async (): Promise<OperationEnd> => {
                try {
                    const res = await fetch(`${API_TOKENS_URL}/${item.id}`, { method: 'DELETE' });
                    if (res.ok) return { state: 'done', message: w('apitok.remove.done') };
                    const error = await readApiError(res);
                    return { state: 'failed', reason: w('apitok.remove.failed', { reason: error.message || w('apitok.error.generic') }) };
                } catch {
                    return { state: 'unknown', reason: w('apitok.remove.unknown') };
                } finally {
                    void list.retry();
                }
            },
        });
    };

    return (
        <section id="api-tokens" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="api-tokens">
            <div className="mb-1 flex items-center gap-2">
                <Plug className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{w('apitok.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{w('apitok.desc')}</p>
            <RemoteGate remote={list.remote} checking={w('apitok.checking')} failed={w('apitok.readFailed')} onRetry={() => void list.retry()} busy={list.reading}>
                {({ value: state }) => {
                    const refusal = refusalKey(state, role === 'admin');
                    return (
                        <div className="grid gap-4">
                            {refusal && (
                                <div className="rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm text-fg" role="status" data-api-tokens-refusal={state.serverEnabled ? state.refusedBy : 'server'}>
                                    <p>{w(refusal, { grantor: grantorText(state, w) })}</p>
                                    {!state.serverEnabled && role === 'admin' && (
                                        <Link to="/settings?section=panel#remote-api" className="mt-2 inline-block text-sm font-medium text-primary underline underline-offset-4">
                                            {w('apitok.off.open')}
                                        </Link>
                                    )}
                                </div>
                            )}
                            {made && (
                                <div className="rounded-lg border border-primary/40 bg-primary/5 p-3" data-api-token-made="">
                                    <p className="text-sm font-semibold text-fg">{w('apitok.made.title')}</p>
                                    <p className="mb-2 text-sm text-fg-muted">{w('apitok.made.desc')}</p>
                                    <code className="block break-all rounded-md bg-surface-subtle px-2 py-1.5 font-mono text-xs text-fg">{made.token}</code>
                                    <div className="mt-2 flex flex-wrap gap-2">
                                        <Button variant="secondary" icon={copied ? Check : Copy} onClick={copy}>
                                            {copied ? w('apitok.made.copied') : w('apitok.made.copy')}
                                        </Button>
                                        <Button variant="primary" onClick={() => setMade(null)}>{w('apitok.made.done')}</Button>
                                    </div>
                                </div>
                            )}
                            {state.canCreate && !made && (
                                <form
                                    className="grid gap-3 sm:grid-cols-[auto_auto_auto] sm:items-end sm:justify-start"
                                    onSubmit={(event) => { event.preventDefault(); void create(state); }}
                                    noValidate
                                >
                                    <label className="grid gap-1 text-sm text-fg">
                                        {w('apitok.name')}
                                        <input className={`${inputClass} field-md`} value={name} maxLength={64} placeholder={w('apitok.namePlaceholder')}
                                            onChange={(event) => setName(event.target.value)} disabled={busy} />
                                    </label>
                                    <label className="grid gap-1 text-sm text-fg">
                                        {w('apitok.scope')}
                                        <select className={`${inputClass} field-md`} value={scope} onChange={(event) => setScope(event.target.value === 'manage' ? 'manage' : 'read')} disabled={busy}>
                                            <option value="read">{w('apitok.scope.read')}</option>
                                            <option value="manage">{w('apitok.scope.manage')}</option>
                                        </select>
                                    </label>
                                    <label className="grid gap-1 text-sm text-fg">
                                        {w('apitok.expiry')}
                                        <select className={`${inputClass} field-md`} value={days} onChange={(event) => setDays(Number(event.target.value))} disabled={busy}>
                                            {state.lifetimes.map((n) => <option key={n} value={n}>{w('apitok.days', { n })}</option>)}
                                        </select>
                                    </label>
                                    <label className="grid gap-1 text-sm text-fg sm:col-span-2">
                                        {w('apitok.password')}
                                        <PasswordInput value={password} onChange={setPassword} autoComplete="current-password" disabled={busy} />
                                    </label>
                                    {state.twoFactor && (
                                        <label className="grid gap-1 text-sm text-fg">
                                            {w('apitok.code')}
                                            <input
                                                className={`${inputClass} font-mono tracking-[0.3em] field-sm`}
                                                value={code}
                                                onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                                                inputMode="numeric"
                                                autoComplete="one-time-code"
                                                placeholder="000000"
                                                disabled={busy}
                                            />
                                        </label>
                                    )}
                                    <p className="text-xs text-fg-muted sm:col-span-3">{w('apitok.reauthHint')}</p>
                                    <div className="sm:col-span-3">
                                        <Button type="submit" variant="primary" icon={KeyRound} loading={busy} className="max-sm:min-h-11">
                                            {busy ? w('apitok.creating') : w('apitok.create')}
                                        </Button>
                                    </div>
                                </form>
                            )}
                            {problem && <p className="text-sm text-danger" role="alert">{problem}</p>}
                            {state.tokens.length === 0 ? (
                                <p className="text-sm text-fg-muted">{w('apitok.empty')}</p>
                            ) : (
                                <ul className="divide-y divide-border rounded-lg border border-border" data-api-token-list="">
                                    {state.tokens.map((item) => (
                                        <li key={item.id} className="flex flex-wrap items-start justify-between gap-3 p-3" data-api-token={item.prefix}>
                                            <div className="min-w-0">
                                                <p className="break-words text-sm font-medium text-fg">
                                                    {item.name}
                                                    <span className="ml-2 rounded-md bg-surface-subtle px-1.5 py-0.5 text-xs font-normal text-fg-muted">
                                                        {item.scope === 'manage' ? w('apitok.scope.manage') : w('apitok.scope.read')}
                                                    </span>
                                                </p>
                                                <p className="font-mono text-xs text-fg-muted">{item.prefix}…</p>
                                                <p className={`text-xs ${item.state === 'active' ? 'text-success' : 'text-warning'}`}>
                                                    {w(item.state === 'active' ? 'apitok.state.active' : item.state === 'ended' ? 'apitok.state.ended' : 'apitok.state.expired')}
                                                </p>
                                                <p className="text-xs text-fg-muted">
                                                    {w('apitok.created', { date: formatDate(item.createdAt, locale) })}
                                                    {' · '}
                                                    {w('apitok.expires', { date: formatDate(item.expiresAt, locale) })}
                                                    {' · '}
                                                    {item.lastUsedAt !== null
                                                        ? w('apitok.lastUsed', { date: formatDate(item.lastUsedAt, locale), ip: item.lastUsedIp || '-' })
                                                        : w('apitok.neverUsed')}
                                                </p>
                                            </div>
                                            <Button variant="danger" icon={Trash2} onClick={() => void remove(item)} className="max-sm:min-h-11">
                                                {w('apitok.remove')}
                                            </Button>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

interface RemoteApiSetting {
    enabled: boolean;
    workingTokens: number;
}

export function decodeRemoteApiSetting(raw: unknown): RemoteApiSetting {
    if (!isObject(raw) || typeof raw.enabled !== 'boolean') throw new Error('shape');
    return { enabled: raw.enabled, workingTokens: typeof raw.working_tokens === 'number' ? raw.working_tokens : 0 };
}

// The administrator's server switch (Settings, Panel). Off until turned on.
export function RemoteApiSwitch() {
    const { w } = useWords();
    const setting = useRemote(REMOTE_API_SETTING_URL, decodeRemoteApiSetting);

    const turn = async (enable: boolean) => {
        await confirmAction({
            title: w(enable ? 'apitok.switch.onTitle' : 'apitok.switch.offTitle'),
            message: w(enable ? 'apitok.switch.onMessage' : 'apitok.switch.offMessage'),
            confirmLabel: w(enable ? 'apitok.switch.turnOn' : 'apitok.switch.turnOff'),
            tone: enable ? 'default' : 'danger',
            progressTitle: w(enable ? 'apitok.switch.progressOn' : 'apitok.switch.progressOff'),
            action: async (): Promise<OperationEnd> => {
                try {
                    const res = await fetch(REMOTE_API_SETTING_URL, {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ enabled: enable }),
                    });
                    if (res.ok) return { state: 'done', message: w(enable ? 'apitok.switch.doneOn' : 'apitok.switch.doneOff') };
                    const error = await readApiError(res);
                    return { state: 'failed', reason: w('apitok.switch.failed', { reason: error.message || w('apitok.error.generic') }) };
                } catch {
                    return { state: 'unknown', reason: w('apitok.switch.unknown') };
                } finally {
                    void setting.retry();
                }
            },
        });
    };

    return (
        <section id="remote-api" className="mt-5 scroll-mt-24 rounded-xl border border-border bg-surface p-6" data-settings-card="remote-api">
            <div className="mb-1 flex items-center gap-2">
                <Plug className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{w('apitok.switch.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{w('apitok.switch.desc')}</p>
            <RemoteGate remote={setting.remote} checking={w('apitok.switch.checking')} failed={w('apitok.switch.readFailed')} onRetry={() => void setting.retry()} busy={setting.reading}>
                {({ value }) => (
                    <div className="flex flex-wrap items-center justify-between gap-3">
                        <p className="text-sm text-fg" data-remote-api-state={value.enabled ? 'on' : 'off'}>
                            {value.enabled ? w('apitok.switch.on', { n: value.workingTokens }) : w('apitok.switch.off')}
                        </p>
                        <Button
                            variant={value.enabled ? 'danger' : 'primary'}
                            disabled={setting.reading}
                            onClick={() => void turn(!value.enabled)}
                            className="max-sm:min-h-11"
                        >
                            {w(value.enabled ? 'apitok.switch.turnOff' : 'apitok.switch.turnOn')}
                        </Button>
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}
