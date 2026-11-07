import { useState } from 'react';
import { Globe, Plus, Trash2 } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { dbRemoteTranslation } from '../i18n/dbRemote';
import { useAuth } from '../auth/AuthContext';
import { useNavigate } from '../router';
import { readApiError, type ApiError } from '../lib/apiError';
import { confirmAction, followOperation, type OperationReport } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { useRemote } from '../lib/remote';
import { showToast } from './Toast';
import { Button, inputClass } from './ui';

// Connections from outside to one database (alpha.129,
// docs/DATABASE-REMOTE-ACCESS.md): off by default; the customer lists the
// addresses that may connect (with TLS), each for one user of the database.
// The problem and its fix come first, worded for who acts (the administrator
// gets the direct step, everyone else "ask your provider"); every change runs
// in the locked progress window with the server's own steps, and the list
// reads itself again afterwards. The part is read only when it is opened, so
// a long list of databases asks the server nothing.
//
// Bir veritabanına dışarıdan bağlantılar: varsayılan kapalı; müşteri bağlanacak
// adresleri listeler. Sorun ve çözümü önce gelir; her değişiklik sunucunun
// adımlarıyla kilitli pencerede çalışır.

export interface DatabaseRemoteTarget {
    id: number;
    name: string;
    type: string;
}

interface Problem { code: string; detail: string }
interface RuleView { id: number; user: string; address: string; status: string; problem: Problem | null }
interface RemoteView {
    engine: string;
    access: { mayEdit: boolean; reason: string; grantor: string };
    rules: RuleView[];
    users: Array<{ id: number; username: string }>;
    client: { host: string; port: number; sslMode: string };
    maxAddresses: number;
    yourAddress: string;
    engineState: { known: boolean; state: string; problems: Problem[]; readError: string } | null;
}

const str = (x: unknown) => (typeof x === 'string' ? x : '');
const num = (x: unknown) => (typeof x === 'number' && Number.isFinite(x) ? x : 0);
const obj = (x: unknown) => ((x && typeof x === 'object' && !Array.isArray(x) ? x : {}) as Record<string, unknown>);
const problemOf = (x: unknown): Problem | null => {
    const p = obj(x);
    return str(p.Code) ? { code: str(p.Code), detail: str(p.Detail) } : null;
};

export function decodeRemoteView(raw: unknown): RemoteView {
    const v = obj(raw);
    if (!Array.isArray(v.rules) || !Array.isArray(v.users)) throw new Error('shape');
    const access = obj(v.access);
    const client = obj(v.client);
    const state = v.engine_state === undefined ? null : obj(v.engine_state);
    return {
        engine: str(v.engine),
        access: { mayEdit: access.may_edit === true, reason: str(access.reason), grantor: str(access.grantor) },
        rules: v.rules.map((r) => {
            const rule = obj(r);
            return { id: num(rule.id), user: str(rule.user), address: str(rule.address), status: str(rule.status), problem: problemOf(rule.problem) };
        }),
        users: v.users.map((u) => ({ id: num(obj(u).id), username: str(obj(u).username) })),
        client: { host: str(client.host), port: num(client.port), sslMode: str(client.ssl_mode) || 'require' },
        maxAddresses: num(v.max_addresses_user) || 10,
        yourAddress: str(v.your_address),
        engineState: state && {
            known: state.known === true,
            state: str(state.state),
            problems: Array.isArray(state.problems) ? state.problems.flatMap((p) => problemOf(p) ?? []) : [],
            readError: str(state.read_error),
        },
    };
}

/** The I18nProvider's words with this part's own copy in front (route-scoped). */
function useDbRemoteWords() {
    const i18n = useI18n();
    return (key: TranslationKey, values?: Record<string, string | number>) => dbRemoteTranslation(i18n.locale, key, values) ?? i18n.t(key, values);
}

export function DatabaseRemoteAccess({ db, readOnly = false }: { db: DatabaseRemoteTarget; readOnly?: boolean }) {
    const t = useDbRemoteWords();
    const { role } = useAuth();
    const navigate = useNavigate();
    const admin = role === 'admin';
    const [open, setOpen] = useState(false);
    const url = `/api/v1/database-remote/databases/${db.id}`;
    const view = useRemote(open ? url : null, decodeRemoteView);
    const shown = view.remote.state === 'known' ? view.remote.value : null;
    const [userId, setUserId] = useState(0);
    const [address, setAddress] = useState('');
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const engineName = db.type === 'postgresql' ? 'PostgreSQL' : 'MariaDB';
    const steps = (ids: string[]) => flowSteps(t, 'dbRemote', ids);

    const run = async (title: string, send: (report: OperationReport) => Promise<Response | null>, failed: TranslationKey, done: (data: Record<string, unknown>) => void) => {
        setBusy(true);
        setRefusal(null);
        try {
            await followOperation({
                progressTitle: title,
                action: async (report) => {
                    try {
                        const res = await send(report);
                        if (!res) return void showToast('error', t('common.resultUnknown'));
                        if (!res.ok) {
                            const problem = await readApiError(res);
                            setRefusal(problem);
                            showToast('error', t(failed));
                            return;
                        }
                        // A success whose body cannot be read is still a success;
                        // the list reads itself again below.
                        let body: unknown = null;
                        try {
                            body = await res.json();
                        } catch {
                            body = null;
                        }
                        done(obj(body));
                    } catch {
                        showToast('error', t('common.resultUnknown'));
                    }
                },
            });
        } finally {
            setBusy(false);
            void view.retry();
        }
    };

    const add = async (value: string) => {
        const chosen = userId || shown?.users[0]?.id || 0;
        const typed = value.trim();
        if (!typed || !chosen || busy) return;
        await run(t('dbr.addTitle', { address: typed }), (report) => sendWithSteps(report, steps(['check!', 'save', 'apply', 'firewall']), t, `dbRemoteAdd.${db.id}`,
            (headers) => fetch(`${url}/rules`, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ user_id: chosen, address: typed }) })),
        'dbr.addFailed', (data) => {
            setAddress('');
            showToast('success', t(data.status === 'active' ? 'dbr.added' : 'dbr.addedWaiting', { address: typed, database: db.name, engine: engineName }));
        });
    };

    const remove = async (rule: RuleView) => {
        if (busy) return;
        await confirmAction({
            message: t('dbr.removeConfirm', { address: rule.address, database: db.name }),
            confirmLabel: t('dbr.remove', { address: rule.address }),
            tone: 'danger',
            progressTitle: t('dbr.removeTitle', { address: rule.address }),
            action: async (report) => {
                setBusy(true);
                setRefusal(null);
                try {
                    const res = await sendWithSteps(report, steps(['check!', 'save', 'apply', 'firewall']), t, `dbRemoteRemove.${rule.id}`,
                        (headers) => fetch(`/api/v1/database-remote/rules/${rule.id}`, { method: 'DELETE', headers }));
                    if (!res) return void showToast('error', t('common.resultUnknown'));
                    if (!res.ok) {
                        setRefusal(await readApiError(res));
                        return void showToast('error', t('dbr.removeFailed'));
                    }
                    showToast('success', t('dbr.removed', { address: rule.address, database: db.name }));
                } catch {
                    showToast('error', t('common.resultUnknown'));
                } finally {
                    setBusy(false);
                    void view.retry();
                }
            },
        });
    };

    const serverChange = async (body: Record<string, boolean>, title: TranslationKey, confirm?: TranslationKey) => {
        const change = async (report: OperationReport) => {
            setBusy(true);
            setRefusal(null);
            try {
                const res = await sendWithSteps(report, steps(['save', 'apply', 'firewall']), t, 'dbRemoteServer',
                    (headers) => fetch('/api/v1/database-remote/server', { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }));
                if (!res) return void showToast('error', t('common.resultUnknown'));
                if (!res.ok) {
                    setRefusal(await readApiError(res));
                    return void showToast('error', t('dbr.turnOnFailed'));
                }
            } catch {
                showToast('error', t('common.resultUnknown'));
            } finally {
                setBusy(false);
                void view.retry();
            }
        };
        if (confirm) await confirmAction({ message: t(confirm), confirmLabel: t('dbr.turnOn'), progressTitle: t(title), action: change });
        else await followOperation({ progressTitle: t(title), action: change });
    };

    // The first thing to do, worded for who does it.
    const blocker = (() => {
        if (!shown) return null;
        const { reason, grantor } = shown.access;
        if (reason === 'not_offered') return { text: t(admin ? 'dbr.off.admin' : 'dbr.off.other'), turnOn: admin };
        if (reason === 'not_in_plan') return { text: t('dbr.notInPlan', { grantor: grantor === 'the administrator' || !grantor ? t('dbr.grantor.admin') : grantor }) };
        if (reason === 'remote_server') return { text: t('dbr.remoteServer') };
        if (reason.startsWith('subscription_')) return { text: t('dbr.subscriptionInactive') };
        return null;
    })();
    const engineProblems = (shown?.engineState?.problems ?? []).filter((p, i, all) => all.findIndex((q) => q.code === p.code) === i);
    const problemText = (p: Problem) => {
        const who = admin ? 'admin' : 'other';
        const specific = `dbr.problem.${p.code}.${who}` as TranslationKey;
        const text = t(specific, { engine: engineName, detail: p.detail });
        if (text !== specific) return text;
        return admin ? t('dbr.problem.FAILED.admin', { detail: p.detail || p.code }) : t('dbr.problem.other');
    };
    const needsComponents = (code: string) => ['RESTART_NEEDED', 'NFT_MISSING', 'NO_SLOWDOWN', 'ENGINE_DOWN'].includes(code);
    const statusText = (rule: RuleView) => {
        if (rule.status === 'refused' && rule.problem) return `${t('dbr.status.refused')}: ${rule.problem.detail}`;
        const key = `dbr.status.${rule.status}` as TranslationKey;
        const text = t(key);
        return text === key ? t('dbr.status.other') : text;
    };
    const canAdd = !readOnly && !!shown?.access.mayEdit;

    return (
        <details className="text-xs text-fg-muted" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} data-db-remote={db.id}>
            <summary className="cursor-pointer">
                <Globe className="mr-1 inline h-3.5 w-3.5" aria-hidden="true" />
                {t('dbr.title')}
                {shown && `: ${shown.rules.length === 0 ? t('dbr.summary.off') : shown.rules.length === 1 ? t('dbr.summary.one') : t('dbr.summary.count', { n: shown.rules.length })}`}
            </summary>
            <div className="mt-2 space-y-3 text-sm">
                {view.remote.state === 'loading' && <p className="text-fg-muted">{t('dbr.reading')}</p>}
                {view.remote.state === 'unknown' && (
                    <p className="text-fg-muted">
                        {t('dbr.unread')}{' '}
                        <button type="button" className="font-semibold text-primary underline" onClick={() => void view.retry()}>{t('dbr.retry')}</button>
                    </p>
                )}
                {shown && (
                    <>
                        {blocker && (
                            <div className="rounded-lg border border-warning/40 bg-warning/10 p-3 text-fg" role="status">
                                <p>{blocker.text}</p>
                                {blocker.turnOn && (
                                    <Button variant="primary" className="mt-2" disabled={busy} onClick={() => void serverChange({ offered: true }, 'dbr.turnOnTitle', 'dbr.turnOnConfirm')}>
                                        {t('dbr.turnOn')}
                                    </Button>
                                )}
                            </div>
                        )}
                        {!blocker && engineProblems.length > 0 && (
                            <div className="space-y-2 rounded-lg border border-warning/40 bg-warning/10 p-3 text-fg" role="status">
                                {engineProblems.map((p) => <p key={p.code}>{problemText(p)}</p>)}
                                {admin && (
                                    <div className="flex flex-wrap gap-2">
                                        {engineProblems.some((p) => needsComponents(p.code)) && (
                                            <Button onClick={() => navigate('/components')}>{t('dbr.components')}</Button>
                                        )}
                                        <Button disabled={busy} onClick={() => void serverChange({ apply: true }, 'dbr.applyAgainTitle')}>{t('dbr.applyAgain')}</Button>
                                    </div>
                                )}
                            </div>
                        )}
                        {refusal && <p className="rounded-lg border border-danger/40 bg-danger/10 p-3 text-fg" role="alert">{refusal.message}</p>}
                        {!blocker && <p className="text-fg-muted">{t('dbr.intro')}</p>}

                        {shown.rules.length === 0 && !blocker && <p className="text-fg-muted">{t('dbr.empty')}</p>}
                        {shown.rules.length > 0 && (
                            <ul className="divide-y divide-border rounded-lg border border-border">
                                {shown.rules.map((rule) => (
                                    <li key={rule.id} className="flex items-center justify-between gap-3 px-3 py-2">
                                        <span className="min-w-0">
                                            <span className="font-mono text-fg">{rule.address}</span>
                                            <span className="ml-2 text-fg-muted">{t('dbr.user')}: <span className="font-mono">{rule.user}</span></span>
                                            <span className={`block text-xs ${rule.status === 'active' ? 'text-success' : 'text-fg-muted'}`}>{statusText(rule)}</span>
                                        </span>
                                        {!readOnly && (
                                            <button type="button" disabled={busy} onClick={() => void remove(rule)}
                                                className="rounded p-2 text-danger hover:bg-danger/20 disabled:opacity-40"
                                                title={t('dbr.remove', { address: rule.address })} aria-label={t('dbr.remove', { address: rule.address })}>
                                                <Trash2 className="h-4 w-4" />
                                            </button>
                                        )}
                                    </li>
                                ))}
                            </ul>
                        )}

                        {canAdd && (shown.users.length === 0 ? <p className="text-fg-muted">{t('dbr.noUsers')}</p> : (
                            <form className="space-y-2 rounded-lg border border-border bg-surface-2/50 p-3" onSubmit={(e) => { e.preventDefault(); void add(address); }}>
                                <p className="font-semibold text-fg">{t('dbr.addHeading')}</p>
                                <div className="flex flex-wrap gap-2">
                                    {shown.users.length > 1 && (
                                        <select aria-label={t('dbr.user')} value={userId || shown.users[0].id} onChange={(e) => setUserId(Number(e.target.value))} className={`${inputClass} w-auto`}>
                                            {shown.users.map((u) => <option key={u.id} value={u.id}>{u.username}</option>)}
                                        </select>
                                    )}
                                    <input type="text" inputMode="text" autoComplete="off" spellCheck={false} aria-label={t('dbr.address')}
                                        value={address} onChange={(e) => setAddress(e.target.value)} placeholder={t('dbr.addressPlaceholder')}
                                        className={`${inputClass} w-auto min-w-[12rem] flex-1 font-mono`} />
                                    <Button type="submit" variant="primary" icon={Plus} disabled={busy || !address.trim()}>{t('dbr.add')}</Button>
                                </div>
                                <p className="text-xs text-fg-muted">{t('dbr.addressHint', { n: shown.maxAddresses })}</p>
                                {shown.yourAddress && !shown.rules.some((r) => r.address === shown.yourAddress) && (
                                    <button type="button" disabled={busy} className="text-xs font-semibold text-primary underline" onClick={() => void add(shown.yourAddress)}>
                                        {t('dbr.useMine', { address: shown.yourAddress })}
                                    </button>
                                )}
                            </form>
                        ))}

                        {shown.rules.length > 0 && (
                            <div className="rounded-lg border border-border p-3">
                                <p className="mb-1 font-semibold text-fg">{t('dbr.settings')}</p>
                                <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
                                    <dt>{t('dbr.host')}</dt><dd className="font-mono text-fg">{shown.client.host || '—'}</dd>
                                    <dt>{t('dbr.port')}</dt><dd className="font-mono text-fg">{shown.client.port}</dd>
                                    <dt>{t('dbr.database')}</dt><dd className="font-mono text-fg">{db.name}</dd>
                                    <dt>{t('dbr.userName')}</dt><dd className="font-mono text-fg">{[...new Set(shown.rules.map((r) => r.user))].join(', ')}</dd>
                                    <dt>{t('dbr.password')}</dt><dd className="text-fg">{t('dbr.passwordValue')}</dd>
                                    <dt>{t('dbr.sslMode')}</dt><dd className="font-mono text-fg">{t('dbr.sslRequire')}</dd>
                                </dl>
                                <ul className="mt-2 list-disc space-y-0.5 pl-4 text-xs">
                                    {db.type === 'postgresql' && <li>{t('dbr.tool.pgadmin')}</li>}
                                    <li>{t('dbr.tool.dbeaver')}</li>
                                    {db.type !== 'postgresql' && <li>{t('dbr.tool.heidisql')}</li>}
                                </ul>
                            </div>
                        )}
                    </>
                )}
            </div>
        </details>
    );
}
