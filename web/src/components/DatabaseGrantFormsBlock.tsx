import { ShieldCheck } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { useDbServerWords, type DbServerWords } from '../i18n/dbServer';
import type { DbServerKey } from '../i18n/dbServer/en';
import { formatDateTime } from '../lib/dateFormat';
import { lastKnown, useRefreshEvery, useRemote } from '../lib/remote';
import { Guidance, type GuidanceTone } from './kit/rows/Guidance';
import { whoWord } from './kit/words';

// Database rights by exact name (alpha.134, docs/DATABASE-USERS.md), on the
// MariaDB component page, for the administrator only. The server rewrites
// the rights CelikPanel gave as name patterns to the database's exact name
// by itself, after an update is verified; this block only reads where that
// stands (GET /api/v1/admin/database-grant-forms never starts work): per
// MariaDB one guidance block (reason, who acts, next step, how it resumes),
// the rewritten rights that also reached other databases, and the rights it
// leaves as they are, each with why it matters and the exact statement under
// a closed Details.
//
// Veritabanı hakları tam adla: yalnız okur; her MariaDB için bir yol
// gösterme kutusu, daraltılan ve olduğu gibi bırakılan haklar.

export const DATABASE_GRANT_FORMS_URL = '/api/v1/admin/database-grant-forms';

export interface GrantFormsEngine {
    key: string;
    name: string;
    local: boolean;
    state: 'waiting' | 'running' | 'done' | 'list_only' | 'blocked';
    reason?: string;
    version?: string;
    checked_at?: string;
    narrowed: number;
    left: number;
    waiting: number;
}

export interface GrantFormsRow {
    engine: string;
    account: string;
    pattern: string;
    database?: string;
    kind: 'ours' | 'owner_row' | 'not_ours';
    reason?: string;
    reached: string[];
    statement?: string;
    at?: string;
}

export interface GrantFormsView {
    engines: GrantFormsEngine[];
    narrowed: GrantFormsRow[];
    left: GrantFormsRow[];
}

export function decodeGrantFormsView(raw: unknown): GrantFormsView {
    if (!raw || typeof raw !== 'object') throw new Error('shape');
    const value = raw as Partial<GrantFormsView>;
    if (!Array.isArray(value.engines) || !Array.isArray(value.narrowed) || !Array.isArray(value.left)) throw new Error('shape');
    return { engines: value.engines, narrowed: value.narrowed, left: value.left };
}

/** The block; nothing for anybody but an administrator. */
export function DatabaseGrantFormsBlock() {
    const { role } = useAuth();
    if (role !== 'admin') return null;
    return <GrantForms />;
}

/** Where one engine stands, in the guidance block's words. */
export function engineGuidance(t: DbServerWords, engine: GrantFormsEngine): { tone: GuidanceTone; title: string; why?: string; who?: string; next?: string; then?: string } {
    const reason = engine.reason ?? '';
    const key = (suffix: string) => `dbgf.state.${reason}${suffix}` as DbServerKey;
    const nobody = whoWord(t, 'nobody');
    const you = whoWord(t, 'you');
    switch (engine.state) {
        case 'done':
            return { tone: 'success', title: t('dbgf.state.done') };
        case 'running':
            return { tone: 'progress', title: t('dbgf.state.running'), who: nobody, then: t('dbgf.state.running.then') };
        case 'list_only':
            return { tone: 'neutral', title: t('dbgf.state.not_mariadb', { version: engine.version || '?' }), who: you, next: t('dbgf.state.not_mariadb.next') };
        case 'waiting':
            if (reason === 'update_unknown' || reason === 'agent_unreachable') {
                return { tone: 'waiting', title: t(key('')), why: t(key('.why')), who: you, next: t(key('.next')), then: t('dbgf.then.hourly') };
            }
            if (reason === 'convert_retry') {
                return { tone: 'waiting', title: t(key('')), why: t(key('.why')), who: nobody, then: t('dbgf.then.retry') };
            }
            if (['starting', 'update_verifying', 'server_busy', 'agent_version'].includes(reason)) {
                return { tone: 'waiting', title: t(key('')), why: t(key('.why')), who: nobody, then: t('dbgf.then.auto') };
            }
            return { tone: 'waiting', title: t('dbgf.state.server_busy'), who: nobody, then: t('dbgf.then.auto') };
        case 'blocked':
        default:
            if (reason === 'convert_failed') {
                return { tone: 'blocked', title: t('dbgf.state.convert_failed'), why: t('dbgf.state.convert_retry.why'), who: you, next: t('dbgf.state.convert_failed.next'), then: t('dbgf.then.day') };
            }
            if (reason === 'credential_refused' || reason === 'credential_unreadable') {
                const next = engine.local ? t('dbgf.state.credential_refused.next') : t('dbgf.state.credential_refused.nextRegistered');
                return { tone: 'blocked', title: t(key('')), who: you, next, then: t('dbgf.then.hourly') };
            }
            if (['engine_unreachable', 'no_grant_rights'].includes(reason)) {
                return { tone: 'blocked', title: t(key('')), who: you, next: t(key('.next')), then: t('dbgf.then.hourly') };
            }
            return { tone: 'blocked', title: t('dbgf.state.no_grant_rights'), who: you, next: t('dbgf.state.no_grant_rights.next'), then: t('dbgf.then.hourly') };
    }
}

function GrantForms() {
    const t = useDbServerWords();
    const { locale } = useI18n();
    const handle = useRemote(DATABASE_GRANT_FORMS_URL, decodeGrantFormsView);
    const known = lastKnown(handle.remote)?.value ?? null;
    // While a MariaDB is being worked on or waits, the block follows it by
    // reading again (reads only; the server's own loop does the work).
    const moving = !!known?.engines.some((engine) => engine.state === 'running' || engine.state === 'waiting');
    useRefreshEvery(handle, 60_000, moving);
    if (handle.remote.state === 'loading') return null;
    if (known && known.engines.length === 0) return null;
    const list = (names: string[]) => names.join(', ');
    return (
        <section className="rounded-xl border border-border bg-surface p-5" aria-labelledby="dbgf-title" data-grant-forms="">
            <h2 id="dbgf-title" className="flex items-center gap-2 text-base font-semibold text-fg">
                <ShieldCheck className="h-4 w-4" aria-hidden="true" />
                {t('dbgf.title')}
            </h2>
            <p className="mt-0.5 max-w-[75ch] text-sm text-fg-muted">{t('dbgf.intro')}</p>
            {!known && (
                <p className="mt-3 text-sm text-fg-muted" role="status">
                    {t('dbgf.unread')}{' '}
                    <button type="button" className="font-semibold text-primary underline" onClick={() => void handle.retry()}>{t('dbgf.retry')}</button>
                </p>
            )}
            {known && (
                <div className="mt-3 space-y-3">
                    {known.engines.map((engine) => {
                        const guidance = engineGuidance(t, engine);
                        return (
                            <div key={engine.key} data-grant-forms-engine={engine.state}>
                                <p className="mb-1.5 text-sm font-medium text-fg">
                                    {engine.local ? t('dbgf.engine.local') : t('dbgf.engine.registered', { name: engine.name })}
                                </p>
                                <Guidance tone={guidance.tone} title={guidance.title} why={guidance.why} who={guidance.who} next={guidance.next} then={guidance.then}>
                                    <p className="mt-2 text-fg-muted">
                                        {t('dbgf.counts', { narrowed: engine.narrowed, left: engine.left, waiting: engine.waiting })}
                                        {engine.checked_at ? ` ${t('dbgf.checked', { time: formatDateTime(engine.checked_at, locale) })}` : ''}
                                    </p>
                                </Guidance>
                            </div>
                        );
                    })}
                    {known.narrowed.length > 0 && (
                        <div data-grant-forms-narrowed="">
                            <h3 className="text-sm font-semibold text-fg">{t('dbgf.narrowed.title')}</h3>
                            <p className="text-sm text-fg-muted">{t('dbgf.narrowed.hint')}</p>
                            <ul className="mt-1.5 space-y-1 text-sm text-fg">
                                {known.narrowed.map((row) => (
                                    <li key={`${row.engine} ${row.account} ${row.pattern}`} className="break-words">
                                        {t('dbgf.narrowed.row', { account: row.account, database: row.database ?? row.pattern, reached: list(row.reached) })}
                                    </li>
                                ))}
                            </ul>
                        </div>
                    )}
                    {known.left.length > 0 && (
                        <div data-grant-forms-left="">
                            <h3 className="text-sm font-semibold text-fg">{t('dbgf.left.title')}</h3>
                            <ul className="mt-1.5 space-y-2">
                                {known.left.map((row) => (
                                    <li key={`${row.engine} ${row.account} ${row.pattern}`} className="rounded-lg border border-border p-3 text-sm">
                                        <p className="break-words font-medium text-fg"><span className="font-mono">{row.account}</span> · <span className="font-mono">{row.pattern}</span></p>
                                        <p className="mt-1 text-fg-muted">{t('dbgf.left.why')} {row.reached.length > 0 ? t('dbgf.left.reachesNow', { list: list(row.reached) }) : t('dbgf.left.reachesNone')}</p>
                                        <p className="mt-1 text-fg-muted">{t(`dbgf.left.reason.${row.reason === 'owner_row' || row.reason === 'not_mariadb' ? row.reason : 'not_provable'}` as DbServerKey)} {t('dbgf.left.todo')}</p>
                                        {row.statement && (
                                            <details className="mt-1.5">
                                                <summary className="cursor-pointer text-fg-muted">{t('dbgf.details')}</summary>
                                                <pre className="mt-1 overflow-x-auto whitespace-pre-wrap break-all rounded bg-surface-2 p-2 font-mono text-xs text-fg">{row.statement}</pre>
                                            </details>
                                        )}
                                    </li>
                                ))}
                            </ul>
                        </div>
                    )}
                </div>
            )}
        </section>
    );
}
