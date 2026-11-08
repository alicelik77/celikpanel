import { useEffect, useState } from 'react';
import { Globe } from 'lucide-react';
import type { TranslationKey } from '../i18n/en';
import { useNavigate } from '../router';
import { readApiError, type ApiError } from '../lib/apiError';
import { confirmAction, followOperation, type OperationReport } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { useRemote } from '../lib/remote';
import { showToast } from './Toast';
import { Button } from './ui';
import { useDbRemoteWords } from './DatabaseRemoteAccess';

// The administrator's server switch of connections from outside to databases
// (alpha.130), on the Databases page: before this it appeared only inside a
// domain's database card. It shows, per engine of this server, what is still
// missing before a listed address could work, in plain words with the step
// next to it (install Fail2ban in Components, restart PostgreSQL, turn
// MariaDB's host name lookup off, ...). The switch is the same stored offer
// the Offered tab of plans and rights writes, so both show the same state.
//
// Yöneticinin "dışarıdan bağlantılar" anahtarı, Veritabanları sayfasında; her
// motor için eksik olan ve adımı düz dille yan yana.

interface Problem { code: string; detail: string }
interface EngineView {
    engine: string;
    port: number;
    rules: number;
    known: boolean;
    state: string;
    readError: string;
    readiness: Problem[];
    fail2ban: string;
    fail2banDetail: string;
    nameResolve: string;
}
interface ServerView {
    offered: boolean;
    skipNameResolve: boolean;
    firewallNote: string;
    engines: EngineView[];
}

const str = (x: unknown) => (typeof x === 'string' ? x : '');
const num = (x: unknown) => (typeof x === 'number' && Number.isFinite(x) ? x : 0);
const obj = (x: unknown) => ((x && typeof x === 'object' && !Array.isArray(x) ? x : {}) as Record<string, unknown>);
const problems = (x: unknown): Problem[] => (Array.isArray(x) ? x.map((p) => ({ code: str(obj(p).Code), detail: str(obj(p).Detail) })).filter((p) => p.code) : []);

export function decodeServerView(raw: unknown): ServerView {
    const v = obj(raw);
    if (typeof v.offered !== 'boolean' || !Array.isArray(v.engines)) throw new Error('shape');
    return {
        offered: v.offered,
        skipNameResolve: v.mariadb_skip_name_resolve === true,
        firewallNote: str(v.panel_firewall_note),
        engines: v.engines.map((e) => {
            const engine = obj(e);
            const state = obj(engine.state);
            return {
                engine: str(engine.engine),
                port: num(engine.port),
                rules: num(engine.rules),
                known: state.known === true,
                state: str(state.state),
                readError: str(state.read_error),
                readiness: problems(state.readiness),
                fail2ban: str(state.fail2ban),
                fail2banDetail: str(state.fail2ban_detail),
                nameResolve: str(state.name_resolve),
            };
        }),
    };
}

const URL = '/api/v1/database-remote/server';
const REFRESH_MS = 60_000;

export function DatabaseRemoteServerPanel() {
    const t = useDbRemoteWords();
    const navigate = useNavigate();
    const view = useRemote(URL, decodeServerView);
    const shown = view.remote.state === 'known' ? view.remote.value : null;
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const { retry } = view;
    // A restart done in Components or Fail2ban installed elsewhere shows here
    // by itself.
    useEffect(() => {
        const timer = window.setInterval(() => void retry(), REFRESH_MS);
        return () => window.clearInterval(timer);
    }, [retry]);

    const change = async (body: Record<string, boolean>, title: TranslationKey, confirm?: { message: TranslationKey; label: TranslationKey; danger?: boolean }) => {
        const action = async (report: OperationReport) => {
            setBusy(true);
            setRefusal(null);
            try {
                const res = await sendWithSteps(report, flowSteps(t, 'dbRemote', ['save', 'apply', 'firewall']), t, 'dbRemoteServer',
                    (headers) => fetch(URL, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }));
                if (!res) return void showToast('error', t('common.resultUnknown'));
                if (!res.ok) {
                    setRefusal(await readApiError(res));
                    return void showToast('error', t('dbrs.failed'));
                }
                let answer: Record<string, unknown> = {};
                try {
                    answer = obj(await res.json());
                } catch {
                    answer = {};
                }
                showToast('success', t(answer.changed === false ? 'dbrs.unchanged' : 'dbrs.saved'));
            } catch {
                showToast('error', t('common.resultUnknown'));
            } finally {
                setBusy(false);
                void retry();
            }
        };
        if (confirm) {
            await confirmAction({ message: t(confirm.message), confirmLabel: t(confirm.label), tone: confirm.danger ? 'danger' : 'default', progressTitle: t(title), action });
        } else {
            await followOperation({ progressTitle: t(title), action });
        }
    };

    const engineName = (engine: string) => (engine === 'postgresql' ? 'PostgreSQL' : 'MariaDB');
    const itemText = (p: Problem, engine: string) => {
        const key = `dbr.problem.${p.code}.admin` as TranslationKey;
        const text = t(key, { engine: engineName(engine), detail: p.detail });
        return text === key ? t('dbr.problem.FAILED.admin', { detail: p.detail || p.code }) : text;
    };
    const needsComponents = (code: string) => ['RESTART_NEEDED', 'NFT_MISSING', 'NO_SLOWDOWN', 'ENGINE_DOWN', 'PG_FAIL2BAN'].includes(code);

    return (
        <section className="mb-4 rounded-xl border border-border bg-surface p-4" aria-labelledby="dbrs-title" data-db-remote-server>
            <h2 id="dbrs-title" className="flex items-center gap-2 text-base font-semibold text-fg">
                <Globe className="h-4 w-4" aria-hidden="true" />
                {t('dbrs.title')}
            </h2>
            {view.remote.state === 'loading' && <p className="mt-2 text-sm text-fg-muted">{t('dbrs.reading')}</p>}
            {view.remote.state === 'unknown' && (
                <p className="mt-2 text-sm text-fg-muted">
                    {t('dbrs.unread')}{' '}
                    <button type="button" className="font-semibold text-primary underline" onClick={() => void retry()}>{t('dbr.retry')}</button>
                </p>
            )}
            {shown && (
                <div className="mt-2 space-y-3 text-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className={shown.offered ? 'text-fg' : 'text-fg-muted'}>{t(shown.offered ? 'dbrs.on' : 'dbrs.off')}</p>
                        {shown.offered ? (
                            <Button disabled={busy} onClick={() => void change({ offered: false }, 'dbrs.turnOffTitle', { message: 'dbrs.turnOffConfirm', label: 'dbrs.turnOff', danger: true })}>
                                {t('dbrs.turnOff')}
                            </Button>
                        ) : (
                            <Button variant="primary" disabled={busy} onClick={() => void change({ offered: true }, 'dbr.turnOnTitle', { message: 'dbr.turnOnConfirm', label: 'dbrs.turnOn' })}>
                                {t('dbrs.turnOn')}
                            </Button>
                        )}
                    </div>
                    <p className="text-xs text-fg-muted">{t('dbrs.intro')}</p>
                    {refusal && <p className="rounded-lg border border-danger/40 bg-danger/10 p-3 text-fg" role="alert">{refusal.message}</p>}
                    {shown.firewallNote && <p className="rounded-lg border border-warning/40 bg-warning/10 p-3 text-fg" role="status">{t('dbrs.firewallNote', { detail: shown.firewallNote })}</p>}
                    {shown.engines.length === 0 && <p className="text-fg-muted">{t('dbrs.noEngines')}</p>}
                    {shown.engines.length > 0 && (
                        <ul className="divide-y divide-border rounded-lg border border-border">
                            {shown.engines.map((e) => {
                                const name = engineName(e.engine);
                                const items = e.readiness.filter((p, i, all) => all.findIndex((q) => q.code === p.code && q.detail === p.detail) === i);
                                const down = e.state === 'down' || (!e.known && !!e.readError);
                                return (
                                    <li key={`${e.engine}:${e.port}`} className="space-y-2 px-3 py-2">
                                        <p className="font-semibold text-fg">{t('dbrs.engine', { engine: name, port: e.port })}</p>
                                        {e.rules > 0 && <p className="text-xs text-fg-muted">{t('dbrs.inForce', { n: e.rules })}</p>}
                                        {down && <p className="text-fg">{t('dbrs.down', { engine: name })}</p>}
                                        {!down && items.length === 0 && <p className="text-success">{t('dbrs.ready', { engine: name })}</p>}
                                        {!down && items.length > 0 && (
                                            <div className="space-y-1 rounded-lg border border-warning/40 bg-warning/10 p-3 text-fg" role="status">
                                                <p className="font-semibold">{t('dbrs.missing')}</p>
                                                <ul className="list-disc space-y-1 pl-4">
                                                    {items.map((p) => <li key={`${p.code}:${p.detail}`}>{itemText(p, e.engine)}</li>)}
                                                </ul>
                                                <div className="flex flex-wrap gap-2 pt-1">
                                                    {items.some((p) => p.code === 'NAME_RESOLVE') && !shown.skipNameResolve && (
                                                        <Button variant="primary" disabled={busy}
                                                            onClick={() => void change({ mariadb_skip_name_resolve: true }, 'dbr.nameLookupTitle', { message: 'dbr.nameLookupConfirm', label: 'dbr.nameLookupOff' })}>
                                                            {t('dbr.nameLookupOff')}
                                                        </Button>
                                                    )}
                                                    {items.some((p) => needsComponents(p.code)) && (
                                                        <Button onClick={() => navigate('/components')}>{t('dbr.components')}</Button>
                                                    )}
                                                    {shown.offered && (
                                                        <Button disabled={busy} onClick={() => void change({ apply: true }, 'dbr.applyAgainTitle')}>{t('dbr.applyAgain')}</Button>
                                                    )}
                                                </div>
                                            </div>
                                        )}
                                        {e.fail2ban === 'on' && <p className="text-xs text-fg-muted">{t('dbrs.fail2ban.on')}</p>}
                                        {e.engine === 'postgresql' && e.fail2ban === 'off' && <p className="text-xs text-fg-muted">{t('dbrs.fail2ban.pgOff')}</p>}
                                        {e.engine !== 'postgresql' && e.nameResolve === 'off' && <p className="text-xs text-fg-muted">{t('dbrs.nameLookup.off')}</p>}
                                        {e.engine !== 'postgresql' && e.nameResolve === 'pending_restart' && <p className="text-xs text-fg-muted">{t('dbrs.nameLookup.pending')}</p>}
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                    {shown.skipNameResolve && (
                        <p className="text-xs text-fg-muted">
                            {t('dbrs.nameLookupAgreed')}{' '}
                            <button type="button" disabled={busy} className="font-semibold text-primary underline disabled:opacity-40"
                                onClick={() => void change({ mariadb_skip_name_resolve: false }, 'dbrs.nameLookupWithdrawTitle')}>
                                {t('dbrs.nameLookupWithdraw')}
                            </button>
                        </p>
                    )}
                </div>
            )}
        </section>
    );
}
