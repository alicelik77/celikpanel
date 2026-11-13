import { useEffect, useState } from 'react';
import { LogOut, MonitorSmartphone, Save, ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { panelSessionsTranslation, type PanelSessionsKey } from '../i18n/panelSessions';
import { showToast } from './Toast';
import { Button, RemoteGate, inputClass } from './ui';
import { CopyCommand } from './CopyCommand';
import { refusalText } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { useRemote } from '../lib/remote';
import { sendRights } from '../lib/rights';
import { useAuth } from '../auth/AuthContext';

// Panel sessions and panel access (alpha.128, G4).
//
// My account → Signed-in sessions: every browser where this account is
// signed in (this one marked), each with "End", and "End all other sessions".
// Security → Panel access (administrator): how long an unused session stays
// signed in, and from which addresses administrators may sign in (the
// screen offers the person's own address and says the local recovery
// first), and every account's live sessions with "End". Questions use the
// panel's own dialogue; lists refresh themselves after a change.
//
// Panel oturumları ve panel erişimi: hesabın açık oturumları, boşta kalma
// süresi, yönetici adresleri ve bütün hesapların oturumları.

export const OWN_SESSIONS_URL = '/api/v1/auth/sessions';
export const ALL_SESSIONS_URL = '/api/v1/panel/sessions';
export const PANEL_ACCESS_URL = '/api/v1/panel/access';
export const RESELLER_SESSIONS_URL = '/api/v1/reseller/sessions';

type Key = TranslationKey | PanelSessionsKey;

function useSessionWords() {
    const i18n = useI18n();
    return { ...i18n, t: (key: Key, values?: Record<string, string | number>) => panelSessionsTranslation(i18n.locale, key, values) ?? i18n.t(key as TranslationKey, values) };
}

export interface PanelSession {
    handle: string;
    current: boolean;
    user_id?: number;
    username?: string;
    role?: string;
    created_at: string;
    last_seen_at?: string;
    client_ip?: string;
    user_agent?: string;
    opened_by?: string;
}

export interface SessionsReport {
    sessions: PanelSession[];
    idle_minutes: number;
}

export function decodeSessions(raw: unknown): SessionsReport {
    const body = raw as SessionsReport | null;
    if (!body || !Array.isArray(body.sessions) || typeof body.idle_minutes !== 'number') throw new Error('shape');
    for (const session of body.sessions) {
        if (typeof session.handle !== 'string' || !/^[0-9a-f]{16}$/.test(session.handle) || typeof session.current !== 'boolean') throw new Error('shape');
    }
    return body;
}

export interface PanelAccess {
    idle_minutes: number;
    idle_choices: number[];
    allowlist: string[];
    your_address: string;
    your_address_allowed: boolean;
    recovery_open: boolean;
    recovery_command: string;
    recovery_undo: string;
}

export function decodePanelAccess(raw: unknown): PanelAccess {
    const body = raw as PanelAccess | null;
    if (!body || typeof body.idle_minutes !== 'number' || !Array.isArray(body.idle_choices) || !Array.isArray(body.allowlist)) throw new Error('shape');
    return body;
}

/** "Firefox on Linux" from a user agent; empty when nothing is recognised. */
export function browserSummary(agent: string | undefined): string {
    if (!agent) return '';
    const browser = /Edg\//.test(agent) ? 'Edge'
        : /OPR\//.test(agent) ? 'Opera'
            : /Firefox\//.test(agent) ? 'Firefox'
                : /Chrome\//.test(agent) ? 'Chrome'
                    : /Safari\//.test(agent) ? 'Safari'
                        : /curl\//.test(agent) ? 'curl' : '';
    const system = /Android/.test(agent) ? 'Android'
        : /iPhone|iPad|iOS/.test(agent) ? 'iOS'
            : /Windows/.test(agent) ? 'Windows'
                : /Mac OS X|Macintosh/.test(agent) ? 'macOS'
                    : /Linux/.test(agent) ? 'Linux' : '';
    return [browser, system].filter(Boolean).join(' · ');
}

/** The server keeps two time shapes: RFC 3339 and SQLite's "YYYY-MM-DD HH:MM:SS" (UTC). */
export function sessionTime(value: string | undefined, locale: string | undefined): string {
    if (!value) return '';
    const parsed = new Date(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? `${value.replace(' ', 'T')}Z` : value);
    if (Number.isNaN(parsed.getTime())) return value;
    return parsed.toLocaleString(locale === 'tr' ? 'tr-TR' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' });
}

function SessionRow({ session, showOwner, onEnd, canEnd }: { session: PanelSession; showOwner: boolean; onEnd: (session: PanelSession) => void; canEnd: boolean }) {
    const { t, locale } = useSessionWords();
    const role = session.role === 'admin' || session.role === 'reseller' || session.role === 'customer' ? t(`psess.role.${session.role}` as PanelSessionsKey) : '';
    return (
        <li className="flex flex-wrap items-start gap-3 rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-session={session.handle} data-session-current={session.current ? 'yes' : 'no'}>
            <MonitorSmartphone className="mt-0.5 h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />
            <div className="min-w-0 flex-1 basis-60">
                <p className="font-semibold text-fg">
                    {showOwner && session.username && <span>{session.username}{role && <span className="font-normal text-fg-muted"> ({role})</span>} · </span>}
                    {browserSummary(session.user_agent) || t('psess.unknownBrowser')}
                    {session.current && <span className="ml-2 rounded bg-primary/10 px-1.5 py-0.5 text-xs font-semibold text-primary">{t('psess.current')}</span>}
                </p>
                <p className="mt-0.5 text-fg-muted">
                    {session.last_seen_at ? t('psess.lastUsed', { time: sessionTime(session.last_seen_at, locale) }) : t('psess.notUsedYet')}
                    {session.client_ip && <> · {t('psess.from', { address: session.client_ip })}</>}
                </p>
                <p className="text-xs text-fg-subtle">{t('psess.signedIn', { time: sessionTime(session.created_at, locale) })}</p>
                {session.opened_by && <p className="text-xs text-fg-muted">{t('psess.openedBy', { name: session.opened_by })}</p>}
            </div>
            {!session.current && canEnd && (
                <Button type="button" icon={LogOut} className="max-sm:min-h-11" onClick={() => onEnd(session)} data-session-end={session.handle}>{t('psess.end')}</Button>
            )}
        </li>
    );
}

async function endSessions(url: string, body: unknown, t: (key: Key, values?: Record<string, string | number>) => string): Promise<void> {
    const result = await sendRights(url, 'POST', body);
    if (result.kind === 'lost') {
        showToast('error', t('common.resultUnknown'));
        return;
    }
    if (result.kind === 'refused') {
        showToast('error', refusalText(result.error, t as (key: TranslationKey, values?: Record<string, string | number>) => string, 'psess.failed' as TranslationKey));
        return;
    }
    const ended = (result.body as { ended?: number } | null)?.ended ?? 0;
    showToast('success', ended > 1 ? t('psess.endedMany', { n: ended }) : t('psess.ended'));
}

/** My account: this account's signed-in sessions. */
export function OwnSessionsCard() {
    const { t } = useSessionWords();
    const { user } = useAuth();
    const read = useRemote(OWN_SESSIONS_URL, decodeSessions);
    const impersonating = user.impersonating === true;
    const end = async (session: PanelSession | null) => {
        const ok = await confirmAction({
            title: t(session ? 'psess.endConfirm.title' : 'psess.endOthersConfirm.title'),
            message: t(session ? 'psess.endConfirm.text' : 'psess.endOthersConfirm.text'),
            confirmLabel: t(session ? 'psess.end' : 'psess.endOthers'),
            tone: 'danger',
            progressTitle: t('psess.ending'),
            action: async () => {
                await endSessions(session ? `${OWN_SESSIONS_URL}/end` : `${OWN_SESSIONS_URL}/end-others`, session ? { handle: session.handle } : undefined, t);
            },
        });
        if (ok) await read.retry();
    };
    return (
        <section id="sessions" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="sessions">
            <div className="mb-1 flex items-center gap-2">
                <MonitorSmartphone className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('psess.own.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('psess.own.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('psess.checking')} failed={t('psess.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value }) => {
                    const others = value.sessions.filter((session) => !session.current);
                    return (
                        <div className="grid gap-3">
                            {impersonating && <p className="text-sm text-fg-muted" data-sessions-impersonating>{t('psess.impersonating')}</p>}
                            <ul className="grid gap-2">
                                {value.sessions.map((session) => (
                                    <SessionRow key={session.handle} session={session} showOwner={false} canEnd={!impersonating} onEnd={(chosen) => void end(chosen)} />
                                ))}
                            </ul>
                            {others.length === 0 && <p className="text-sm text-fg-muted" data-sessions-only-this>{t('psess.onlyThis')}</p>}
                            <div className="flex flex-wrap items-center gap-3">
                                {others.length > 1 && !impersonating && (
                                    <Button type="button" variant="danger" icon={LogOut} className="max-sm:min-h-11" onClick={() => void end(null)} data-sessions-end-others>{t('psess.endOthers')}</Button>
                                )}
                                <span className="text-xs text-fg-muted">{t('psess.idleNote', { n: value.idle_minutes })}</span>
                            </div>
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

/**
 * Every session a manager may end: the administrator's every account
 * (Security), or a reseller's own customers and their team members (My
 * account). The server decides the scope.
 */
function ManagedSessionsCard({ url, kind }: { url: string; kind: 'all' | 'customers' }) {
    const { t } = useSessionWords();
    const { user } = useAuth();
    const read = useRemote(url, decodeSessions);
    const impersonating = user.impersonating === true;
    const end = async (session: PanelSession) => {
        const ok = await confirmAction({
            title: t('psess.endConfirm.title'),
            message: t('psess.endConfirm.text'),
            confirmLabel: t('psess.end'),
            tone: 'danger',
            progressTitle: t('psess.ending'),
            action: async () => {
                await endSessions(`${url}/end`, { handle: session.handle }, t);
            },
        });
        if (ok) await read.retry();
    };
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-security-section={kind === 'all' ? 'sessions' : undefined} data-account-section={kind === 'customers' ? 'customer-sessions' : undefined}>
            <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                <MonitorSmartphone className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {t(kind === 'all' ? 'psess.all.title' : 'psess.customers.title')}
            </h2>
            <p className="mb-4 mt-0.5 text-sm text-fg-muted">{t(kind === 'all' ? 'psess.all.desc' : 'psess.customers.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('psess.checking')} failed={t('psess.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value }) => (
                    <div className="grid gap-2">
                        <ul className="grid gap-2">
                            {value.sessions.map((session) => (
                                <SessionRow key={session.handle} session={session} showOwner canEnd={!impersonating} onEnd={(chosen) => void end(chosen)} />
                            ))}
                        </ul>
                        {value.sessions.every((session) => session.current) && <p className="text-sm text-fg-muted">{t(kind === 'all' ? 'psess.all.none' : 'psess.customers.none')}</p>}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}

/** Security (administrator): every account's live sessions. */
export function AllSessionsCard() {
    return <ManagedSessionsCard url={ALL_SESSIONS_URL} kind="all" />;
}

/** My account (reseller): the sessions of the reseller's customers and their team members. */
export function CustomerSessionsCard() {
    return <ManagedSessionsCard url={RESELLER_SESSIONS_URL} kind="customers" />;
}

function idleLabel(minutes: number, t: (key: Key, values?: Record<string, string | number>) => string): string {
    return minutes >= 60 && minutes % 60 === 0 ? t('pacc.hours', { n: minutes / 60 }) : t('pacc.minutes', { n: minutes });
}

/** Parses the address box: one entry per line (commas and spaces also split). */
export function addressLines(text: string): string[] {
    return text.split(/[\s,]+/).map((entry) => entry.trim()).filter(Boolean);
}

/** Security (administrator): the idle timeout and the administrator addresses. */
export function PanelAccessCard() {
    const { t } = useSessionWords();
    const { user } = useAuth();
    const read = useRemote(PANEL_ACCESS_URL, decodePanelAccess);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    const [idle, setIdle] = useState<number | null>(null);
    const [list, setList] = useState<string | null>(null);
    const [saving, setSaving] = useState(false);
    useEffect(() => {
        if (known) {
            setIdle(known.idle_minutes);
            setList(known.allowlist.join('\n'));
        }
    }, [known]);
    const impersonating = user.impersonating === true;
    const save = async () => {
        if (!known || saving) return;
        setSaving(true);
        try {
            const result = await sendRights(PANEL_ACCESS_URL, 'PUT', { idle_minutes: idle ?? known.idle_minutes, allowlist: addressLines(list ?? '') });
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            else if (result.kind === 'refused') showToast('error', refusalText(result.error, t as (key: TranslationKey, values?: Record<string, string | number>) => string, 'pacc.failed' as TranslationKey));
            else showToast('success', t('pacc.saved'));
        } finally {
            await read.retry();
            setSaving(false);
        }
    };
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-security-section="panel-access">
            <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                <ShieldCheck className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {t('pacc.title')}
            </h2>
            <p className="mb-4 mt-0.5 text-sm text-fg-muted">{t('pacc.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('pacc.checking')} failed={t('pacc.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value }) => {
                    const lines = addressLines(list ?? value.allowlist.join('\n'));
                    // A network may hold the address; the server decides those. Only a
                    // list of single addresses without it is surely a lock-out.
                    const mineListed = !value.your_address || lines.length === 0 || lines.includes(value.your_address) || lines.some((line) => line.includes('/'));
                    return (
                        <div className="grid gap-4">
                            {value.recovery_open && (
                                <div className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-3 text-sm text-fg" data-panel-access-recovery-open>
                                    <p>{t('pacc.recovery.open')}</p>
                                    <CopyCommand command={value.recovery_undo} />
                                </div>
                            )}
                            {impersonating && <p className="text-sm text-fg-muted">{t('pacc.impersonating')}</p>}
                            <fieldset className="grid gap-4" disabled={impersonating || saving}>
                                <label className="grid gap-1 text-sm">
                                    <span className="font-semibold text-fg">{t('pacc.idle')}</span>
                                    <select className={`${inputClass} field-sm`} value={idle ?? value.idle_minutes} onChange={(event) => setIdle(Number(event.target.value))} data-panel-access-idle>
                                        {value.idle_choices.map((choice) => <option key={choice} value={choice}>{idleLabel(choice, t)}</option>)}
                                    </select>
                                    <span className="text-xs text-fg-muted">{t('pacc.idleHint')}</span>
                                </label>
                                <label className="grid gap-1 text-sm">
                                    <span className="font-semibold text-fg">{t('pacc.list')}</span>
                                    <textarea className={`${inputClass} min-h-24 font-mono`} value={list ?? value.allowlist.join('\n')} onChange={(event) => setList(event.target.value)} spellCheck={false} data-panel-access-list />
                                    <span className="text-xs text-fg-muted">{t('pacc.listHint')}</span>
                                </label>
                                {value.your_address && (
                                    <div className="flex flex-wrap items-center gap-3 text-sm">
                                        <span className="text-fg">{t('pacc.yourAddress', { address: value.your_address })}</span>
                                        {!lines.includes(value.your_address) && (
                                            <Button type="button" className="max-sm:min-h-11" onClick={() => setList([...lines, value.your_address].join('\n'))} data-panel-access-add-mine>{t('pacc.addMine')}</Button>
                                        )}
                                    </div>
                                )}
                                {!mineListed && <p className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-3 text-sm text-fg" data-panel-access-mine-missing>{t('pacc.mineMissing')}</p>}
                                <p className="text-xs text-fg-muted">{t('pacc.loopback')}</p>
                            </fieldset>
                            <div className="rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-panel-access-recovery>
                                <p className="font-semibold text-fg">{t('pacc.recovery.title')}</p>
                                <p className="mt-0.5 text-fg-muted">{t('pacc.recovery.text')}</p>
                                <div className="mt-2 grid gap-2">
                                    <CopyCommand command={value.recovery_command} />
                                    <CopyCommand command={value.recovery_undo} />
                                </div>
                            </div>
                            {!impersonating && (
                                <div>
                                    <Button type="button" variant="primary" icon={Save} loading={saving} disabled={!mineListed} className="max-sm:min-h-11" onClick={() => void save()} data-panel-access-save>{t('pacc.save')}</Button>
                                </div>
                            )}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
