import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, Card, RemoteGate } from './ui';
import { Guidance } from './kit/rows/Guidance';
import { useInvalidation, useRefreshEvery, useRemote } from '../lib/remote';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';

// PostgreSQL databases made before alpha.141 that every account of the
// server can enter (security lane S1b, 20 November 2026;
// cmd/panel/postgresql_public_access.go). Administrators only: the
// PostgreSQL page is theirs. What CelikPanel left open is listed with what it
// found and what closing changes, beside the one button that closes it; what
// it closed (by itself or by an administrator) is listed with when, by whom,
// the panel's undo and the plain statements of the undo. Reading changes
// nothing; the list reads again by itself after every action and every
// five minutes.
//
// alpha.141'den önce açılan, sunucudaki her hesabın girebildiği PostgreSQL
// veritabanları: bulunan, kapatma ve geri alma bir arada.

export const PG_PUBLIC_URL = '/api/v1/admin/postgresql/public-access';
const REFRESH_MS = 5 * 60_000;

interface Grant { role: string; temporary?: boolean }

export interface PGPublicEntry {
    database_id: number;
    name: string;
    port: number;
    state: string;
    by?: string;
    at?: string;
    owner?: string;
    acl?: string;
    found?: string[];
    other_roles?: string[];
    sessions?: string[];
    monitoring?: string[];
    roles?: string[];
    grants?: Grant[];
    undo_statements?: string[];
    reason?: string;
}

export interface PGPublicView {
    checked_at?: string;
    open: PGPublicEntry[];
    closed: PGPublicEntry[];
    unknown: PGPublicEntry[];
}

const FOUND = ['waiting', 'undone', 'changed_rights', 'other_roles', 'other_sessions', 'monitoring_roles'] as const;

const strings = (value: unknown): string[] => (Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []);

// An answer of another shape is not a list.
export function decodePGPublic(raw: unknown): PGPublicView {
    const body = raw as Partial<Record<keyof PGPublicView, unknown>> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.open) || !Array.isArray(body.closed) || !Array.isArray(body.unknown)) throw new Error('shape');
    const entries = (list: unknown[]): PGPublicEntry[] => list.filter((item): item is PGPublicEntry => (
        !!item && typeof item === 'object' && typeof (item as PGPublicEntry).database_id === 'number' && typeof (item as PGPublicEntry).name === 'string'
    )).map((item) => ({
        ...item,
        found: strings(item.found).filter((reason) => (FOUND as readonly string[]).includes(reason)),
        other_roles: strings(item.other_roles),
        sessions: strings(item.sessions),
        monitoring: strings(item.monitoring),
        roles: strings(item.roles),
        undo_statements: strings(item.undo_statements),
        grants: Array.isArray(item.grants) ? item.grants.filter((grant): grant is Grant => !!grant && typeof grant.role === 'string') : [],
    }));
    return {
        checked_at: typeof body.checked_at === 'string' ? body.checked_at : undefined,
        open: entries(body.open),
        closed: entries(body.closed),
        unknown: entries(body.unknown),
    };
}

export function PostgreSQLPublicAccessBlock() {
    const { t, locale } = useI18n();
    const handle = useRemote(PG_PUBLIC_URL, decodePGPublic);
    useRefreshEvery(handle, REFRESH_MS);
    useInvalidation(['databases'], () => void handle.retry());
    const [busy, setBusy] = useState<number | null>(null);
    const [said, setSaid] = useState<{ id: number; tone: 'ok' | 'bad'; text: string } | null>(null);

    const remote = handle.remote;
    // Nothing to show while the first read is on its way (most servers have
    // nothing here), or when no database of this server is open to other
    // accounts, none was closed by CelikPanel and every one was read.
    if (remote.state === 'loading') return null;
    if (remote.state === 'known' && remote.value.open.length === 0 && remote.value.closed.length === 0 && remote.value.unknown.length === 0) return null;

    const when = (iso?: string) => {
        if (!iso) return '';
        const at = new Date(iso);
        return Number.isNaN(at.getTime()) ? '' : at.toLocaleString(locale, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
    };

    const send = async (entry: PGPublicEntry, action: 'close' | 'undo'): Promise<OperationEnd> => {
        const vars = { name: entry.name };
        let end: OperationEnd;
        try {
            const res = await fetch(PG_PUBLIC_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(action === 'close'
                    ? { action, database_id: entry.database_id, acl: entry.acl ?? '', roles: entry.roles ?? [] }
                    : { action, database_id: entry.database_id }),
            });
            if (res.ok) {
                end = { state: 'done', message: t(action === 'close' ? 'pgpub.done' : 'pgpub.undo.done', vars) };
            } else {
                const problem = await readApiError(res);
                end = { state: res.status === 502 ? 'unknown' : res.status < 500 ? 'blocked' : 'failed', reason: apiErrorText(problem, t, 'pgpub.unknown') };
                if (problem.details?.length) end.reason += ' ' + problem.details.join(' ');
            }
        } catch {
            end = { state: 'unknown', reason: t('pgpub.unknown', vars) };
        }
        setSaid({ id: entry.database_id, tone: end.state === 'done' ? 'ok' : 'bad', text: end.state === 'done' ? end.message ?? '' : end.reason });
        // Whatever came of it, the list is read again: it says what holds now.
        void handle.retry();
        return end;
    };

    const close = async (entry: PGPublicEntry) => {
        if (busy !== null) return;
        setBusy(entry.database_id);
        setSaid(null);
        const vars = { name: entry.name, owner: entry.owner ?? '' };
        const lost = [t('pgpub.confirm.lost')];
        const outside = [...new Set([...(entry.other_roles ?? []), ...(entry.monitoring ?? [])])]
            .filter((role) => !(entry.grants ?? []).some((grant) => grant.role === role));
        if (outside.length > 0) lost.push(t('pgpub.confirm.lostRoles', { roles: outside.join(', ') }));
        try {
            await confirmAction({
                title: t('pgpub.confirm.title', vars),
                message: t('pgpub.confirm.body', vars),
                confirmLabel: t('pgpub.close'),
                lost,
                kept: [t('pgpub.confirm.kept', vars), t('pgpub.confirm.keptUndo')],
                progressTitle: t('pgpub.progress', vars),
                action: () => send(entry, 'close'),
            });
        } finally {
            setBusy(null);
        }
    };

    const undo = async (entry: PGPublicEntry) => {
        if (busy !== null) return;
        setBusy(entry.database_id);
        setSaid(null);
        const vars = { name: entry.name };
        try {
            await confirmAction({
                title: t('pgpub.undo.title', vars),
                message: t('pgpub.undo.body', vars),
                confirmLabel: t('pgpub.undo'),
                tone: 'danger',
                lost: [t('pgpub.undo.lost', vars)],
                kept: [t('pgpub.undo.kept')],
                progressTitle: t('pgpub.undo.progress', vars),
                action: () => send(entry, 'undo'),
            });
        } finally {
            setBusy(null);
        }
    };

    const note = (entry: PGPublicEntry) => said?.id === entry.database_id && (
        <p className={`mt-2 text-sm ${said.tone === 'ok' ? 'text-success' : 'text-danger'}`} role={said.tone === 'ok' ? 'status' : 'alert'} data-pgpub-said={said.tone}>{said.text}</p>
    );

    const foundLine = (entry: PGPublicEntry, reason: string) => {
        switch (reason) {
            case 'changed_rights': return t('pgpub.found.changed_rights', { acl: entry.acl ?? '' });
            case 'other_roles': return t('pgpub.found.other_roles', { roles: (entry.other_roles ?? []).join(', ') });
            case 'other_sessions': return t('pgpub.found.other_sessions', { roles: (entry.sessions ?? []).join(', ') });
            case 'monitoring_roles': return t('pgpub.found.monitoring_roles', { roles: (entry.monitoring ?? []).join(', ') });
            case 'undone': return t('pgpub.found.undone');
            default: return t('pgpub.found.waiting');
        }
    };

    return (
        <Card title={t('pgpub.title')} icon={ShieldCheck}>
            <div id="databases-open" className="scroll-mt-24 p-4" data-pgpub="">
                <RemoteGate remote={remote} checking={t('pgpub.checking')} failed={t('pgpub.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value: view }) => (
                        <div className="space-y-6">
                            {view.open.length > 0 && (
                                <section data-pgpub-open="">
                                    <Guidance
                                        tone="blocked"
                                        titleAs="h3"
                                        title={t('pgpub.open.title', { n: view.open.length })}
                                        why={t('pgpub.open.why')}
                                        who={t('pgpub.open.who')}
                                        next={t('pgpub.open.next')}
                                        then={t('pgpub.open.then')}
                                    />
                                    <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
                                        {view.open.map((entry) => (
                                            <li key={entry.database_id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-start" data-pgpub-database={entry.name}>
                                                <div className="min-w-0 flex-1 text-sm leading-6">
                                                    <p className="break-words font-mono font-semibold text-fg">{entry.name} <span className="font-sans text-xs font-normal text-fg-subtle">· {entry.port}</span></p>
                                                    {entry.owner && <p className="text-fg-muted">{t('pgpub.owner', { owner: entry.owner })}</p>}
                                                    <ul className="mt-1 list-disc space-y-1 pl-5 text-fg">
                                                        {(entry.found ?? []).map((reason) => <li key={reason} className="break-words" data-pgpub-found={reason}>{foundLine(entry, reason)}</li>)}
                                                    </ul>
                                                    <p className="mt-1 break-words text-fg-muted">
                                                        {(entry.grants ?? []).length > 0
                                                            ? t('pgpub.gives', { users: (entry.grants ?? []).map((grant) => (grant.temporary ? t('pgpub.user.write', { user: grant.role }) : grant.role)).join(', ') })
                                                            : t('pgpub.gives.none')}
                                                    </p>
                                                    {note(entry)}
                                                </div>
                                                <Button variant="primary" className="shrink-0 self-start max-sm:min-h-11" loading={busy === entry.database_id} disabled={busy !== null} onClick={() => void close(entry)}>
                                                    {t('pgpub.close')}
                                                </Button>
                                            </li>
                                        ))}
                                    </ul>
                                </section>
                            )}
                            {view.unknown.length > 0 && (
                                <section className="space-y-2" data-pgpub-unread="">
                                    {view.unknown.map((entry) => (
                                        <Guidance key={entry.database_id} tone="unknown" title={t('pgpub.unread.title', { name: entry.name })} why={entry.reason} next={t('pgpub.unread.next')}
                                            action={<Button onClick={() => void handle.retry()} loading={handle.reading}>{t('pgpub.reread')}</Button>} />
                                    ))}
                                </section>
                            )}
                            {view.closed.length > 0 && (
                                <section data-pgpub-closed="">
                                    <h3 className="mb-2 text-sm font-semibold text-fg">{t('pgpub.closed.title')}</h3>
                                    <ul className="divide-y divide-border rounded-lg border border-border">
                                        {view.closed.map((entry) => (
                                            <li key={entry.database_id} className="p-3 text-sm leading-6" data-pgpub-database={entry.name}>
                                                <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
                                                    <div className="min-w-0 flex-1">
                                                        <p className="break-words font-mono font-semibold text-fg">{entry.name} <span className="font-sans text-xs font-normal text-fg-subtle">· {entry.port}</span></p>
                                                        <p className="text-fg-muted">{t(entry.by === 'administrator' ? 'pgpub.closed.administrator' : 'pgpub.closed.panel', { time: when(entry.at) })}</p>
                                                    </div>
                                                    <Button className="shrink-0 self-start max-sm:min-h-11" loading={busy === entry.database_id} disabled={busy !== null} onClick={() => void undo(entry)}>
                                                        {t('pgpub.undo')}
                                                    </Button>
                                                </div>
                                                {(entry.undo_statements ?? []).length > 0 && (
                                                    <details className="mt-2">
                                                        <summary className="cursor-pointer text-fg-muted">{t('pgpub.statements')}</summary>
                                                        <p className="mt-1 text-fg-muted">{t('pgpub.statements.how', { port: entry.port })}</p>
                                                        <pre className="mt-1 overflow-x-auto whitespace-pre rounded bg-surface-2 p-2 font-mono text-xs text-fg">{(entry.undo_statements ?? []).join('\n')}</pre>
                                                    </details>
                                                )}
                                                {note(entry)}
                                            </li>
                                        ))}
                                    </ul>
                                </section>
                            )}
                        </div>
                    )}
                </RemoteGate>
            </div>
        </Card>
    );
}
