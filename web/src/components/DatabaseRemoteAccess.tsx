import { useEffect, useState } from 'react';
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
interface RuleView { id: number; user: string; address: string; status: string; problem: Problem | null; expiresAt: string }
interface RemoteView {
    engine: string;
    access: { mayEdit: boolean; reason: string; grantor: string };
    rules: RuleView[];
    users: Array<{ id: number; username: string }>;
    client: { host: string; port: number; sslMode: string };
    maxAddresses: number;
    yourAddress: string;
    engineState: { known: boolean; state: string; problems: Problem[]; readError: string; closePending: boolean } | null;
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
            return { id: num(rule.id), user: str(rule.user), address: str(rule.address), status: str(rule.status), problem: problemOf(rule.problem), expiresAt: str(rule.expires_at) };
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
            closePending: state.close_pending === true,
        },
    };
}

/** The I18nProvider's words with this part's own copy in front (route-scoped). */
export function useDbRemoteWords() {
    const i18n = useI18n();
    return (key: TranslationKey, values?: Record<string, string | number>) => dbRemoteTranslation(i18n.locale, key, values) ?? i18n.t(key, values);
}

/** How long an address stays allowed (alpha.130). One day is the default: a
 *  forgotten address closes by itself, and extending is one click. */
export const DURATIONS = ['1d', '7d', 'permanent'] as const;
export type Duration = (typeof DURATIONS)[number];

/** A rule's end in the interface language and the viewer's time zone. */
export function shownUntil(iso: string, locale: string): string {
    const at = new Date(iso);
    if (!iso || Number.isNaN(at.getTime())) return '';
    return at.toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' });
}

/** How often an open part reads itself again while something can change by
 *  itself on the server (an end, a waiting rule, a pending close). */
const REFRESH_MS = 30_000;

export function DatabaseRemoteAccess({ db, readOnly = false }: { db: DatabaseRemoteTarget; readOnly?: boolean }) {
    const t = useDbRemoteWords();
    const { locale } = useI18n();
    const { role } = useAuth();
    const navigate = useNavigate();
    const admin = role === 'admin';
    const [open, setOpen] = useState(false);
    const url = `/api/v1/database-remote/databases/${db.id}`;
    const view = useRemote(open ? url : null, decodeRemoteView);
    const shown = view.remote.state === 'known' ? view.remote.value : null;
    const [userId, setUserId] = useState(0);
    const [address, setAddress] = useState('');
    const [duration, setDuration] = useState<Duration>('1d');

    // The list reads itself again while it is open and something on the
    // server changes by itself: an end passing, a rule being applied, a
    // closing that waits for the server (no F5).
    const live = !!shown && (shown.rules.some((r) => r.expiresAt !== '' || r.status === 'waiting') || shown.engineState?.closePending === true);
    const { retry } = view;
    useEffect(() => {
        if (!open || !live) return undefined;
        const timer = window.setInterval(() => void retry(), REFRESH_MS);
        return () => window.clearInterval(timer);
    }, [open, live, retry]);
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
            (headers) => fetch(`${url}/rules`, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ user_id: chosen, address: typed, duration }) })),
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

    // A new end for an address that is already listed (alpha.130): nothing
    // is added again; the server is asked only when the address had ended.
    const extend = async (rule: RuleView, chosen: Duration) => {
        if (busy) return;
        await run(t('dbr.extendTitle', { address: rule.address }), (report) => sendWithSteps(report, steps(['check!', 'save']), t, `dbRemoteExtend.${rule.id}`,
            (headers) => fetch(`/api/v1/database-remote/rules/${rule.id}`, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ duration: chosen }) })),
        'dbr.extendFailed', (data) => {
            const until = shownUntil(str(data.expires_at), locale);
            showToast('success', until ? t('dbr.extended', { address: rule.address, until }) : t('dbr.extendedPermanent', { address: rule.address }));
        });
    };

    const serverChange = async (body: Record<string, boolean>, title: TranslationKey, confirm?: TranslationKey, confirmLabel?: TranslationKey) => {
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
        if (confirm) await confirmAction({ message: t(confirm), confirmLabel: t(confirmLabel ?? 'dbr.turnOn'), progressTitle: t(title), action: change });
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
    const untilText = (rule: RuleView) => {
        const until = shownUntil(rule.expiresAt, locale);
        return until ? t('dbr.until', { until }) : t('dbr.noEnd');
    };
    const durationLabel = (d: Duration) => t(`dbr.duration.${d}` as TranslationKey);
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
                                        {engineProblems.some((p) => p.code === 'NAME_RESOLVE') && (
                                            <Button variant="primary" disabled={busy}
                                                onClick={() => void serverChange({ mariadb_skip_name_resolve: true }, 'dbr.nameLookupTitle', 'dbr.nameLookupConfirm', 'dbr.nameLookupOff')}>
                                                {t('dbr.nameLookupOff')}
                                            </Button>
                                        )}
                                        {engineProblems.some((p) => needsComponents(p.code)) && (
                                            <Button onClick={() => navigate('/components')}>{t('dbr.components')}</Button>
                                        )}
                                        <Button disabled={busy} onClick={() => void serverChange({ apply: true }, 'dbr.applyAgainTitle')}>{t('dbr.applyAgain')}</Button>
                                    </div>
                                )}
                            </div>
                        )}
                        {shown.engineState?.closePending && (
                            <p className="rounded-lg border border-warning/40 bg-warning/10 p-3 text-fg" role="status">
                                {t(admin ? 'dbr.closePending.admin' : 'dbr.closePending.other', { engine: engineName })}
                            </p>
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
                                            <span className="block text-xs text-fg-muted">{untilText(rule)}</span>
                                        </span>
                                        {canAdd && (
                                            <select aria-label={t('dbr.extendLabel', { address: rule.address })} value="" disabled={busy}
                                                onChange={(e) => { const chosen = e.target.value as Duration; if (DURATIONS.includes(chosen)) void extend(rule, chosen); }}
                                                className={`${inputClass} w-auto shrink-0 text-xs`}>
                                                <option value="">{t('dbr.extend')}</option>
                                                {DURATIONS.map((d) => <option key={d} value={d}>{durationLabel(d)}</option>)}
                                            </select>
                                        )}
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
                                    <select aria-label={t('dbr.durationLabel')} value={duration} onChange={(e) => setDuration(e.target.value as Duration)} className={`${inputClass} w-auto`}>
                                        {DURATIONS.map((d) => <option key={d} value={d}>{durationLabel(d)}</option>)}
                                    </select>
                                    <Button type="submit" variant="primary" icon={Plus} disabled={busy || !address.trim()}>{t('dbr.add')}</Button>
                                </div>
                                <p className="text-xs text-fg-muted">{t('dbr.addressHint', { n: shown.maxAddresses })} {t('dbr.durationHint')}</p>
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
