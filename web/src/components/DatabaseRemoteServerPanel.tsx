import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Check, Copy, Eye, Globe, KeyRound, RefreshCw, Trash2 } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useNavigate, useLocation } from '../router';
import { useDbServerWords, type DbServerWords } from '../i18n/dbServer';
import type { DbServerKey } from '../i18n/dbServer/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { answerWasLost } from '../lib/requestIdentity';
import { useLostAnswer } from '../lib/lostAnswer';
import { confirmAction, followOperation, type OperationEnd, type OperationReport } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { decodeList, lastKnown, useRefreshEvery, useRemote, type Remote } from '../lib/remote';
import { showToast } from './Toast';
import { Button, Dialog, ResultUnknown, StatusDot } from './ui';
import { Guidance } from './kit/rows/Guidance';
import { RowActions } from './kit/rows/RowActions';
import { whoWord } from './kit/words';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// The server's own database matters (alpha.133): one lazily loaded part for
// the Security page and the MariaDB / PostgreSQL pages - the switch of
// connections from outside with each service's readiness, and the account
// CelikPanel connects to an engine with.
// Sunucunun kendi veritabanı işleri: Güvenlik sayfası ve MariaDB / PostgreSQL
// sayfaları için tek tembel parça.
//
// Security › Open to the outside (alpha.133, drawing R06): the server-wide
// switch that lets customers connect to their databases from outside, and
// under it, per installed database service, whether a listed address could
// work here. Before alpha.133 this was a block on the administrator's
// Databases page; the list of databases stays there, the server's own
// setting lives with the server's other doors.
//
// The switch is saved only with Save (alpha.132 save kit): the confirmation
// says what turning it on or off means - turning it off lists that every
// address customers listed stops working - and the change runs in the
// panel's progress window with the server's own steps. What a service still
// needs comes from the server as codes; each code is said in plain words
// with why, who acts, the next step and one button, and the server's own
// sentence is only under the closed "Details". The card reads itself again
// while it is open, because a restart or an installed Fail2ban changes it by
// itself (reads only: a poll never sends a change).
//
// Güvenlik › Dışarıya açık olanlar: müşterilerin veritabanlarına dışarıdan
// bağlanma anahtarı (yalnız Kaydet ile) ve hizmet başına hazır olma durumu.
// Sunucunun gönderdiği kod düz dille Neden · Kim yapar · Sonraki adım ve tek
// düğmeyle söylenir; sunucunun cümlesi yalnız kapalı "Ayrıntı"dadır.

/** The card's anchor: /security#outside-connections leads here. */
export const OUTSIDE_CONNECTIONS_ANCHOR = 'outside-connections';

export interface Problem { code: string; detail: string }
export interface EngineView {
    engine: string;
    port: number;
    rules: number;
    known: boolean;
    state: string;
    readError: string;
    readiness: Problem[];
    fail2ban: string;
    nameResolve: string;
}
export interface ServerView {
    offered: boolean;
    rules: number;
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
        rules: num(v.rules),
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
                nameResolve: str(state.name_resolve),
            };
        }),
    };
}

const URL = '/api/v1/database-remote/server';
const REFRESH_MS = 30_000;

/**
 * The codes the server sends (internal/transport/database_remote_contracts.go)
 * and the one button each needs. A code not listed here is said neutrally
 * with its details (tests/database-server-alpha133.test.mjs checks the list
 * and the words against the contract file).
 */
type Fix = 'components' | 'fail2ban' | 'fail2banStart' | 'apply' | 'nameLookup' | 'none';
export const PROBLEM_FIX: Record<string, Fix> = {
    NFT_MISSING: 'components',
    ENGINE_DOWN: 'components',
    OWNER_LISTEN: 'apply',
    OWNER_HBA: 'apply',
    OWNER_FILE: 'apply',
    RESTART_NEEDED: 'components',
    TLS_MISSING: 'apply',
    ALTER_SYSTEM_OFF: 'components',
    PASSWORD_MD5: 'apply',
    PRIVILEGED_USER: 'none',
    NO_ACCESS: 'apply',
    USER_MISSING: 'apply',
    ACCOUNT_CONFLICT: 'apply',
    NO_SLOWDOWN: 'components',
    FAILED: 'apply',
    RECORD_UNREADABLE: 'apply',
    FIREWALL_UNCHANGED: 'apply',
    NAME_RESOLVE: 'nameLookup',
    OWNER_NAME_RESOLVE: 'components',
    NAME_ACCOUNTS: 'apply',
    PG_FAIL2BAN: 'none',
};

/**
 * A slowing-down problem whose cause is Fail2ban itself gets the words and
 * the button for that cause. The server names the cause only in its English
 * detail, so this reads that detail's start; anything else keeps the code's
 * general words.
 */
function refinedProblem(p: Problem): { words: string; fix: Fix } {
    if (p.code === 'NO_SLOWDOWN' && p.detail.startsWith('Fail2ban is not installed')) return { words: 'NO_SLOWDOWN.f2bMissing', fix: 'fail2ban' };
    if (p.code === 'NO_SLOWDOWN' && p.detail.startsWith('Fail2ban is installed but does not run')) return { words: 'NO_SLOWDOWN.f2bStopped', fix: 'fail2banStart' };
    if (p.code in PROBLEM_FIX) return { words: p.code, fix: PROBLEM_FIX[p.code] };
    return { words: 'other', fix: 'none' };
}

const engineName = (engine: string) => (engine === 'postgresql' ? 'PostgreSQL' : 'MariaDB');

type Readiness = 'ready' | 'notReady' | 'unknown';
function readinessOf(e: EngineView): { state: Readiness; items: Problem[]; notes: Problem[] } {
    // The extra Fail2ban layer of PostgreSQL never holds a rule back: a note, not a blocker.
    const unique = e.readiness.filter((p, i, all) => all.findIndex((q) => q.code === p.code && q.detail === p.detail) === i);
    const notes = unique.filter((p) => p.code === 'PG_FAIL2BAN');
    const items = unique.filter((p) => p.code !== 'PG_FAIL2BAN');
    if (e.state === 'down') return { state: 'notReady', items: items.some((p) => p.code === 'ENGINE_DOWN') ? items : [{ code: 'ENGINE_DOWN', detail: e.readError }, ...items], notes };
    if (!e.known) return { state: 'unknown', items: [], notes: [] };
    return { state: items.length > 0 ? 'notReady' : 'ready', items, notes };
}

export function DatabaseRemoteServerPanel() {
    const { role } = useAuth();
    // The server switch and its readiness are the administrator's alone.
    if (role !== 'admin') return null;
    return <OutsideConnections />;
}

function OutsideConnections() {
    const t = useDbServerWords();
    const say = useSaveWords();
    const navigate = useNavigate();
    const location = useLocation();
    const view = useRemote(URL, decodeServerView);
    useRefreshEvery(view, REFRESH_MS);
    const known = lastKnown(view.remote)?.value ?? null;
    const stale = view.remote.state === 'unknown';
    const base = useMemo(() => (known ? { offered: known.offered } : null), [known]);
    const draft = useDraft(base);
    const on = draft.values?.offered ?? false;
    const changed = draft.isChanged('offered');
    const section = useRef<HTMLElement | null>(null);

    // /security#outside-connections (the Databases page and a domain's
    // remote access lead here) brings the card into view once it is drawn.
    const drawn = known !== null;
    useEffect(() => {
        if (location.hash !== `#${OUTSIDE_CONNECTIONS_ANCHOR}`) return;
        section.current?.scrollIntoView?.({ block: 'start' });
    }, [location.hash, drawn]);

    const save = () => void draft.save(async (_changes, values) => {
        const turningOn = values.offered;
        const listed = known?.rules ?? 0;
        const end: { outcome: 'saved' | 'refused' | 'unknown' } = { outcome: 'refused' };
        await confirmAction({
            title: t(turningOn ? 'dbsv.onTitle' : 'dbsv.offTitle'),
            message: t(turningOn ? 'dbsv.onMessage' : 'dbsv.offMessage'),
            lost: turningOn ? undefined : [
                listed === 0 ? t('dbsv.offLostNone') : listed === 1 ? t('dbsv.offLostOne') : t('dbsv.offLost', { n: listed }),
                ...(listed > 0 ? [t('dbsv.offLostPrograms')] : []),
            ],
            kept: turningOn ? [t('dbsv.onKept')] : [t('dbsv.offKept'), t('dbsv.offKeptSites')],
            confirmLabel: t('dbsv.save'),
            tone: turningOn ? 'default' : 'danger',
            progressTitle: t(turningOn ? 'dbsv.progressOn' : 'dbsv.progressOff'),
            action: async (report) => {
                const sent = await serverChange(report, t, { offered: turningOn });
                end.outcome = sent.outcome;
                return sent.end;
            },
        });
        if (end.outcome === 'saved') await view.retry();
        return end.outcome;
    });

    // The guidance buttons that change something on the server (they ask
    // first when they change more than "apply again").
    const act = async (body: Record<string, boolean>, title: DbServerKey, confirm?: { message: DbServerKey; label: DbServerKey }) => {
        const run = async (report: OperationReport) => (await serverChange(report, t, body)).end;
        if (confirm) await confirmAction({ message: t(confirm.message), confirmLabel: t(confirm.label), progressTitle: t(title), action: run });
        else await followOperation({ progressTitle: t(title), action: run });
        void view.retry();
    };

    const fixButton = (fix: Fix, engine: string): ReactNode => {
        const busy = draft.saving;
        switch (fix) {
            case 'components':
                return <Button variant="primary" onClick={() => navigate('/components')} data-dbsv-fix="components">{t('dbsv.act.components')}</Button>;
            case 'fail2ban':
                return <Button variant="primary" onClick={() => navigate('/components?component=fail2ban&install=')} data-dbsv-fix="fail2ban">{t('dbsv.act.components')}</Button>;
            case 'fail2banStart':
                return <Button variant="primary" onClick={() => navigate('/components/fail2ban')} data-dbsv-fix="fail2ban">{t('dbsv.act.components')}</Button>;
            case 'apply':
                return known?.offered ? (
                    <Button variant="primary" disabled={busy} onClick={() => void act({ apply: true }, 'dbsv.act.applyTitle')} data-dbsv-fix="apply">{t('dbsv.act.apply')}</Button>
                ) : null;
            case 'nameLookup':
                return known && !known.skipNameResolve && engine !== 'postgresql' ? (
                    <Button variant="primary" disabled={busy} data-dbsv-fix="nameLookup"
                        onClick={() => void act({ mariadb_skip_name_resolve: true }, 'dbsv.nameLookup.title', { message: 'dbsv.nameLookup.confirm', label: 'dbsv.nameLookup.off' })}>
                        {t('dbsv.nameLookup.off')}
                    </Button>
                ) : null;
            default:
                return null;
        }
    };

    return (
        <section ref={section} id={OUTSIDE_CONNECTIONS_ANCHOR} className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" aria-labelledby="dbsv-title" data-security-section="outside" data-db-remote-server="">
            <h2 id="dbsv-title" className="flex items-center gap-2 text-base font-semibold text-fg">
                <Globe className="h-4 w-4" aria-hidden="true" />
                {t('dbsv.title')}
            </h2>
            <p className="mt-0.5 max-w-[75ch] text-sm text-fg-muted">{t('dbsv.intro')}</p>

            {view.remote.state === 'loading' && <p className="mt-3 text-sm text-fg-muted">{t('dbsv.reading')}</p>}
            {view.remote.state === 'unknown' && (
                <p className="mt-3 text-sm text-fg-muted" role="status">
                    {t('dbsv.unread')}{' '}
                    <button type="button" className="font-semibold text-primary underline" onClick={() => void view.retry()}>{t('dbsv.retry')}</button>
                </p>
            )}

            {known && (
                <>
                    <div className={`mt-4 rounded-lg border border-border p-3 ${changed ? changedRowClass : ''}`} data-dbsv-switch="">
                        <label className="flex cursor-pointer items-start gap-3">
                            <input
                                type="checkbox"
                                role="switch"
                                className="mt-1 h-4 w-4 shrink-0 accent-primary"
                                checked={on}
                                aria-checked={on}
                                disabled={draft.saving}
                                onChange={(event) => draft.set('offered', event.currentTarget.checked)}
                            />
                            <span className="min-w-0">
                                <span className="block text-sm font-medium text-fg">
                                    {t('dbsv.switch')}
                                    {changed && <ChangedMark switched={on} />}
                                </span>
                                <span className="mt-0.5 block max-w-[75ch] text-sm text-fg-muted">{t('dbsv.switchHelp')}</span>
                                {known.rules > 0 && <span className="mt-0.5 block text-sm text-fg-muted">{known.rules === 1 ? t('dbsv.listedOne') : t('dbsv.listed', { n: known.rules })}</span>}
                            </span>
                        </label>
                        {draft.isMoved('offered') && draft.base && (
                            <ServerMovedNote value={say(draft.base.offered ? 'on' : 'off')} onTake={() => draft.take('offered')} />
                        )}
                    </div>

                    {known.firewallNote && (
                        <Guidance className="mt-4" tone="blocked" title={t('dbsv.firewall.title')} why={t('dbsv.firewall.why')} who={whoWord(t, 'you')}
                            next={t('dbsv.firewall.next')} data-dbsv-firewall=""
                            action={<Button onClick={() => document.getElementById('firewall')?.scrollIntoView?.({ block: 'start' })}>{t('dbsv.firewall.go')}</Button>}>
                            <Detail t={t} text={known.firewallNote} />
                        </Guidance>
                    )}

                    <h3 className="mb-2 mt-5 text-sm font-semibold text-fg">{t('dbsv.engines')}</h3>
                    {known.engines.length === 0 ? (
                        <p className="text-sm text-fg-muted">{t('dbsv.noEngines')}</p>
                    ) : (
                        <ul className="divide-y divide-border rounded-lg border border-border">
                            {known.engines.map((e) => (
                                <EngineRow key={`${e.engine}:${e.port}`} e={e} t={t} fixButton={fixButton} />
                            ))}
                        </ul>
                    )}

                    {known.skipNameResolve && (
                        <p className="mt-3 text-xs text-fg-muted">
                            {t('dbsv.nameLookupAgreed')}{' '}
                            <button type="button" disabled={draft.saving} className="font-semibold text-primary underline disabled:opacity-40"
                                onClick={() => void act({ mariadb_skip_name_resolve: false }, 'dbsv.nameLookupWithdrawTitle')}>
                                {t('dbsv.nameLookupWithdraw')}
                            </button>
                        </p>
                    )}

                    <SaveBar
                        className="-mx-5 px-5 pb-0"
                        count={draft.count}
                        saving={draft.saving}
                        onSave={save}
                        onDiscard={draft.reset}
                        blockedBy={stale ? say('stale') : undefined}
                        unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: view.reading, onCheck: () => void view.retry() } : undefined}
                    />
                </>
            )}
        </section>
    );
}

function EngineRow({ e, t, fixButton }: { e: EngineView; t: DbServerWords; fixButton: (fix: Fix, engine: string) => ReactNode }) {
    const name = engineName(e.engine);
    const { state, items, notes } = readinessOf(e);
    const facts = [t('dbsv.port', { port: e.port })];
    if (state === 'ready') facts.push(t('dbsv.encrypted'));
    if (e.rules > 0) facts.push(t('dbsv.inForce', { n: e.rules }));
    return (
        <li className="space-y-3 px-3 py-3 text-sm" data-dbsv-engine={e.engine} data-dbsv-state={state}>
            <p className="flex flex-wrap items-center gap-x-4 gap-y-1">
                <span className="min-w-[7rem] font-semibold text-fg">{name}</span>
                <span className="inline-flex items-center gap-1.5 font-medium">
                    {state === 'notReady'
                        ? <span className="inline-block h-2 w-2 shrink-0 rounded-full bg-warning" aria-hidden="true" />
                        : <StatusDot tone={state === 'ready' ? 'success' : 'unknown'} />}
                    <span className={state === 'ready' ? 'text-success' : 'text-fg'}>{t(state === 'ready' ? 'dbsv.ready' : state === 'notReady' ? 'dbsv.notReady' : 'dbsv.unknown')}</span>
                </span>
                <span className="text-fg-muted">{facts.join(' · ')}</span>
            </p>
            {state === 'unknown' && (
                <Guidance tone="unknown" live="off" title={t('dbsv.unknownTitle', { engine: name })} why={t('dbsv.unknownWhy')} who={whoWord(t, 'nobody')} next={t('dbsv.unknownNext')}>
                    {e.readError && <Detail t={t} text={e.readError} />}
                </Guidance>
            )}
            {items.map((p) => {
                const { words, fix } = refinedProblem(p);
                return (
                    <Guidance key={`${p.code}:${p.detail}`} tone="blocked" live="off" data-dbsv-problem={p.code}
                        title={t('dbsv.notReadyTitle', { engine: name })}
                        why={t(`dbsv.problem.${words}.why` as DbServerKey, { engine: name })}
                        who={whoWord(t, 'you')}
                        next={t(`dbsv.problem.${words}.next` as DbServerKey, { engine: name })}
                        action={fixButton(fix, e.engine)}>
                        <Detail t={t} text={p.detail} code={p.code} />
                    </Guidance>
                );
            })}
            {state === 'ready' && (
                <>
                    {e.fail2ban === 'on' && <p className="text-xs text-fg-muted">{t('dbsv.note.f2bOn')}</p>}
                    {e.engine === 'postgresql' && e.fail2ban === 'off' && <p className="text-xs text-fg-muted">{t('dbsv.note.pgF2bLater')}</p>}
                    {notes.map((p) => (
                        <div key={p.detail} className="text-xs text-fg-muted" data-dbsv-note={p.code}>
                            <p>{t(p.detail.startsWith('Fail2ban is not installed') ? 'dbsv.note.pgFail2banMissing' : 'dbsv.note.pgFail2ban')}</p>
                            <Detail t={t} text={p.detail} />
                        </div>
                    ))}
                    {e.engine !== 'postgresql' && e.nameResolve === 'off' && <p className="text-xs text-fg-muted">{t('dbsv.note.nameLookupOff')}</p>}
                    {e.engine !== 'postgresql' && e.nameResolve === 'pending_restart' && <p className="text-xs text-fg-muted">{t('dbsv.note.nameLookupPending')}</p>}
                </>
            )}
        </li>
    );
}

/** The server's own sentence, for the administrator, under a closed "Details". */
function Detail({ t, text, code }: { t: DbServerWords; text: string; code?: string }) {
    if (!text && !code) return null;
    return (
        <details className="text-xs text-fg-muted" data-dbsv-detail="">
            <summary className="cursor-pointer">{t('problem.details')}</summary>
            {text && <p className="mt-1 whitespace-pre-wrap break-words">{text}</p>}
            {code && <code className="break-words">{code}</code>}
        </details>
    );
}

/**
 * One change of the server setting (PUT /api/v1/database-remote/server) in
 * the open progress window, with the server's steps. The answer is the
 * window's end and whether the setting is now stored.
 */
async function serverChange(report: OperationReport, t: DbServerWords, body: Record<string, boolean>): Promise<{ outcome: 'saved' | 'refused' | 'unknown'; end: OperationEnd }> {
    let res: Response | null;
    try {
        res = await sendWithSteps(report, flowSteps(t, 'dbRemoteServer', ['save', 'apply', 'firewall']), t, 'dbRemoteServer',
            (headers) => fetch(URL, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }));
    } catch {
        res = null;
    }
    if (!res || answerWasLost(res)) return { outcome: 'unknown', end: { state: 'unknown', reason: t('common.resultUnknown') } };
    if (!res.ok) {
        const problem = await readApiError(res);
        // Saved, but the server has not confirmed it yet: the panel retries by itself.
        if (problem.code === 'DATABASE_REMOTE_AGENT') return { outcome: 'saved', end: { state: 'unknown', reason: t('dbsv.unconfirmed') } };
        if (problem.code === 'PANEL_STARTING') return { outcome: 'refused', end: { state: 'blocked', reason: t('dbsv.starting') } };
        return { outcome: 'refused', end: { state: 'failed', reason: apiErrorText(problem, t) } };
    }
    let answer: Record<string, unknown> = {};
    try {
        answer = obj(await res.json());
    } catch {
        answer = {};
    }
    const message = t(answer.changed === false ? 'dbsv.unchanged' : 'dbsv.saved');
    showToast('success', message);
    return { outcome: 'saved', end: { state: 'done', message } };
}

// --- MariaDB / PostgreSQL: the account CelikPanel connects with -------------

// R-065. CelikPanel opens an account of its own on a database engine and
// connects as that, so the operator's own root is never touched. That is only
// true on screen if the screen says so: an administrator has to be able to see
// whether the account is there, read its password, give it a new one, and take
// it away.
//
// alpha.133 (drawings B18, B19): the account belongs to the service, so it is
// shown on the MariaDB and PostgreSQL component pages ("The account CelikPanel
// connects to this service with"), for the administrator and only while the
// service is installed. One quiet line says what the account is for and what
// stops without it; the rare actions are words in the "More" menu, removal
// last and red, and every one that changes something asks first.
//
// R-065. CelikPanel motorda kendi hesabını açar ve onunla bağlanır; böylece
// operatörün kök hesabına hiç dokunulmaz. alpha.133: hesap hizmete aittir; bu
// yüzden MariaDB ve PostgreSQL bileşen sayfalarında, yalnız yöneticiye ve
// hizmet kuruluyken gösterilir. Seyrek işler "Diğer" menüsündedir.

export interface DatabaseAccountServer {
    id: number;
    admin_username?: string;
    is_local?: boolean;
    type_name?: string;
    port?: number;
}

type Engine = 'mariadb' | 'postgresql';

/** Which component page a database server record belongs to. */
export function engineOfServer(typeName: string | undefined): Engine | null {
    const name = (typeName ?? '').toLowerCase();
    if (name.includes('postgres')) return 'postgresql';
    if (name.includes('maria') || name.includes('mysql')) return 'mariadb';
    return null;
}

/**
 * The block on a database component page: the account CelikPanel connects to
 * this service with, for each record of this machine's service (PostgreSQL
 * may run several clusters). Nothing is drawn for anybody but an
 * administrator, nor while the service has no record (it is not installed).
 */
export function EngineAccountBlock({ engine }: { engine: Engine }) {
    const { role } = useAuth();
    if (role !== 'admin') return null;
    return <EngineAccounts engine={engine} />;
}

function EngineAccounts({ engine }: { engine: Engine }) {
    const t = useDbServerWords();
    // The same address and decoder as every other reader of the server list.
    const servers = useRemote('/api/v1/database-servers', decodeList<DatabaseAccountServer>);
    if (servers.remote.state === 'loading') return null;
    const listed = lastKnown(servers.remote)?.value ?? null;
    const mine = (listed ?? []).filter((s) => s.is_local && engineOfServer(s.type_name) === engine);
    if (listed && mine.length === 0) return null;
    return (
        <section id="panel-account" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" aria-labelledby={`dbacc-title-${engine}`} data-engine-account={engine}>
            <h2 id={`dbacc-title-${engine}`} className="flex items-center gap-2 text-base font-semibold text-fg">
                <KeyRound className="h-4 w-4" aria-hidden="true" />
                {t('dbacc.title')}
            </h2>
            <p className="mt-0.5 max-w-[75ch] text-sm text-fg-muted">{t('dbacc.intro')}</p>
            {!listed && (
                <p className="mt-3 text-sm text-fg-muted" role="status">
                    {t('dbacc.unread')}{' '}
                    <button type="button" className="font-semibold text-primary underline" onClick={() => void servers.retry()}>{t('dbacc.retry')}</button>
                </p>
            )}
            <div className="mt-3 space-y-3">
                {mine.map((server) => (
                    <DatabaseAccountStrip key={server.id} server={server} several={mine.length > 1}
                        onChanged={() => servers.retry()} current={servers.remote.state === 'known' && !servers.reading} />
                ))}
            </div>
        </section>
    );
}

export function DatabaseAccountStrip({
    server,
    onChanged,
    current,
    several = false,
}: {
    server: DatabaseAccountServer;
    /** Reads the server list again and says what it now knows. It only reads. */
    onChanged: () => Promise<Remote<unknown>>;
    /**
     * The row shown is the server's current answer. While the list is being
     * read again after a change here, or after that read failed, the row may
     * describe an account that no longer exists, and every control below acts
     * on what the row says - so none of them is offered until it is current.
     * Gösterilen satır sunucunun güncel yanıtıdır; satır güncel olana dek
     * hiçbir denetim sunulmaz.
     */
    current: boolean;
    /** More than one record of this service: each row names its port. */
    several?: boolean;
}) {
    const t = useDbServerWords();
    const [working, setBusy] = useState(false);
    // Opening and re-keying the account carry an identity the server keeps
    // (D-029): a lost answer has been asked for once more before this row
    // hears of it, and the engine was given at most one new password. When
    // there is still no result the server list is read again; nothing here
    // acts on the row until that read answers.
    // Hesabı açma ve yeniden anahtarlama, sunucunun sakladığı bir kimlik taşır
    // (D-029). Sonuç yine yoksa sunucu listesi yeniden okunur; o okuma
    // yanıtlanana dek buradaki hiçbir denetim satır üzerinde işlem yapmaz.
    const answer = useLostAnswer(() => onChanged());
    const busy = working || !current || answer.holding;
    // Shown once: the password lives here only while its window is open.
    const [password, setPassword] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);

    const account = (server.admin_username ?? '').trim();
    const engine = engineName(engineOfServer(server.type_name) ?? '');
    const closePassword = () => {
        setPassword(null);
        setCopied(false);
    };

    const provision = async (rotating: boolean) => {
        setBusy(true);
        try {
            const res = await answer.send(`/api/v1/database-servers/${server.id}/admin-account`, {
                method: 'POST',
            });
            // No result of its own: the notice is up and the list is being
            // read again.
            // Kendi sonucu yok: bildirim açıldı, liste yeniden okunuyor.
            if (!res) return;
            answer.settle();
            // The answer was lost and asked for again (D-029): the server says
            // the account was already opened or re-keyed by this same request.
            // Nothing of that answer is missing here - the password is read
            // with "Show password" - so it is the success it was.
            // Yanıt kayboldu ve yeniden soruldu (D-029): sunucu, hesabın bu aynı
            // istekle zaten açıldığını ya da yeniden anahtarlandığını söylüyor.
            const problem = res.ok ? null : await readApiError(res);
            if (problem && !(problem.code === 'REQUEST_COMPLETED_RESULT_NOT_RETAINED' && !problem.reason)) {
                // The engine's refusal is written by the panel and is the
                // whole point of showing it; a generic failure would send the
                // administrator back to guessing.
                // Motorun reddini panel yazar ve onu göstermek işin özüdür.
                throw new Error(problem.code?.startsWith('REQUEST_') ? apiErrorText(problem, t) : problem.message || t('common.error'));
            }
            showToast('success', rotating ? t('dbacc.rotated') : t('dbacc.opened', { engine }));
            void onChanged();
        } catch (error) {
            showToast('error', error instanceof Error ? error.message : t('common.error'));
        } finally {
            setBusy(false);
        }
    };

    const rotate = async () => {
        await confirmAction({
            title: t('dbacc.rotateTitle'),
            message: t('dbacc.rotateMessage'),
            confirmLabel: t('dbacc.rotateConfirm'),
            progressTitle: t('dbacc.rotateProgress'),
            // The page says the result (a notice, or the lost-answer note).
            action: async () => {
                await provision(true);
            },
        });
    };

    const reveal = async () => {
        setBusy(true);
        try {
            const res = await fetch(`/api/v1/database-servers/${server.id}/admin-account`);
            if (!res.ok) throw new Error();
            const body = await res.json();
            setPassword(body.password ?? '');
            setCopied(false);
        } catch {
            showToast('error', t('common.error'));
        } finally {
            setBusy(false);
        }
    };

    const remove = async () => {
        await confirmAction({
            title: t('dbacc.removeTitle', { engine }),
            lost: [t('dbacc.removeLostAdd'), t('dbacc.removeLostUsers')],
            kept: [t('dbacc.removeKeptData'), t('dbacc.removeKeptSites'), t('dbacc.removeKeptOwn')],
            confirmLabel: t('dbacc.removeConfirm'),
            tone: 'danger',
            progressTitle: t('dbacc.removeProgress'),
            action: async (report) => {
                setBusy(true);
                try {
                    // The server's own steps (alpha.127).
                    const res = await sendWithSteps(report, flowSteps(t, 'adminRemove', ['check!', 'remove', 'records']), t, `adminRemove.${server.id}`,
                        (headers) => fetch(`/api/v1/database-servers/${server.id}/admin-account`, { method: 'DELETE', headers }));
                    if (!res.ok) return { state: 'failed', reason: apiErrorText(await readApiError(res), t) };
                    showToast('success', t('dbacc.removed', { engine }));
                    void onChanged();
                    return { state: 'done', message: t('dbacc.removed', { engine }) };
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    setBusy(false);
                }
            },
        });
    };

    const copy = async () => {
        if (password === null) return;
        try {
            await navigator.clipboard.writeText(password);
            setCopied(true);
        } catch {
            showToast('error', t('common.error'));
        }
    };

    if (!account) {
        // The blocking state: adding databases fails while this is true, so
        // it is said plainly, with the one action that resolves it.
        // Engelleyen durum: bu doğruyken veritabanı eklemek başarısız olur.
        return (
            <>
                <ResultUnknown answer={answer} />
                <Guidance tone="blocked" live="off" data-engine-account-missing=""
                    title={t('dbacc.missingTitle', { engine })}
                    why={t('dbacc.missingWhy')}
                    who={whoWord(t, 'you')}
                    next={t('dbacc.missingNext')}
                    action={(
                        <Button variant="primary" icon={KeyRound} disabled={busy} onClick={() => void provision(false)}>
                            {working ? t('dbacc.opening') : t('dbacc.open')}
                        </Button>
                    )} />
            </>
        );
    }

    return (
        <>
            <ResultUnknown answer={answer} />
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 max-sm:flex-col max-sm:items-stretch" data-engine-account-row={server.id}>
                <div className="flex min-w-0 flex-1 items-start gap-3">
                    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-fg-muted" aria-hidden="true">
                        <KeyRound className="h-4 w-4" />
                    </span>
                    <span className="min-w-0">
                        <span className="block truncate font-mono text-sm text-fg">{account}</span>
                        <span className="block text-xs text-fg-muted">
                            {several && server.port ? `${t('dbacc.port', { port: server.port })} · ` : ''}{t('dbacc.recorded')}
                        </span>
                    </span>
                </div>
                <RowActions
                    name={account}
                    actions={[]}
                    more={[
                        { id: 'show', label: t('dbacc.show'), icon: Eye, onSelect: () => void reveal(), disabled: busy, disabledReason: t('dbacc.busy') },
                        { id: 'rotate', label: t('dbacc.rotate'), icon: RefreshCw, onSelect: () => void rotate(), disabled: busy, disabledReason: t('dbacc.busy') },
                        { id: 'remove', label: t('dbacc.remove'), icon: Trash2, danger: true, onSelect: () => void remove(), disabled: busy, disabledReason: t('dbacc.busy') },
                    ]}
                />
            </div>

            {password !== null && (
                <Dialog
                    id="database-admin-password"
                    title={t('dbacc.passwordTitle', { engine })}
                    description={t('dbacc.passwordRecorded')}
                    icon={KeyRound}
                    width="md"
                    onDismiss={closePassword}
                    actions={
                        <>
                            <Button icon={copied ? Check : Copy} onClick={copy}>
                                {t(copied ? 'dbacc.copied' : 'dbacc.copy')}
                            </Button>
                            <Button variant="primary" onClick={closePassword}>
                                {t('common.close')}
                            </Button>
                        </>
                    }
                >
                    {/* Read character by character, so it is set in the face
                        that distinguishes them, and selectable as one unit.
                        Karakter karakter okunur; bu yüzden onları ayırt eden
                        yazı karakteriyle dizilir. */}
                    <p className="text-xs font-medium uppercase tracking-wide text-fg-subtle">
                        {account}
                    </p>
                    <p className="mt-2 select-all break-all rounded-lg border border-border bg-surface-2 px-3 py-2.5 font-mono text-sm text-fg">
                        {password}
                    </p>
                </Dialog>
            )}
        </>
    );
}
