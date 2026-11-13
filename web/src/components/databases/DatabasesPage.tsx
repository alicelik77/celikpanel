import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ArrowRight, ArrowUp, Database, Globe, KeyRound, Plus, SearchX, Server, Trash2, X } from 'lucide-react';
import { Link, useNavigate } from '../../router';
import { useI18n } from '../../i18n';
import { useAuth } from '../../auth/AuthContext';
import { Button, Dialog, ErrorBanner, KnownEmpty, RemoteGate, SearchInput, type Shown } from '../ui';
import { PageHeader } from '../PageHeader';
import { EngineMark } from '../EngineMark';
import { RowActions, type RowAction } from '../kit/rows/RowActions';
import { Guidance } from '../kit/rows/Guidance';
import { sizeWord, whoWord } from '../kit/words';
import { decodeList, invalidate, lastKnown, useRefreshEvery, useRemote } from '../../lib/remote';
import { readApiError } from '../../lib/apiError';
import { useRefusal } from '../../lib/useRefusal';
import { confirmAction } from '../../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../../lib/requestProgress';
import { formatDate } from '../../lib/dateFormat';
import { showToast } from '../Toast';
import {
    VISIBLE_DATABASES_URL, databaseDeleteURL, databaseUserPasswordURL, decodeVisibleDatabases,
    type VisibleDatabase, type VisibleDatabases,
} from '../../lib/visibleDatabases';
// The new-password form is the one the domain's Databases section uses; it is
// fetched when it is opened.
const DatabaseUserPassword = lazy(() => import('../DatabaseUserPassword').then((module) => ({ default: module.DatabaseUserPassword })));

// The left menu's Databases page (alpha.133, drawings V15–V17). One rule
// decides what is listed (lib/visibleDatabases.ts): an administrator sees
// every database on the server, a reseller their customers', a customer and a
// team member their own. An administrator and a reseller read one list with
// an Owner column, a search and sortable columns; a customer reads theirs
// grouped by domain. A row opens its owner's domain page at the Databases
// section, gives a user a new password or deletes the database. Passwords
// and the contents of a database are never shown. What belongs to the
// server itself (its engines, outside connections, the panel's own engine
// account) is not on this page.
//
// The list is one of three things (D-024): being read, could not be read
// (nothing was changed; read again), or the server's answer. "No databases"
// and "no database service" are said only for an answer. It reads itself
// again after every change and every half minute while the tab is visible.
//
// Sol menüdeki Veritabanları sayfası. Yönetici sunucudaki bütün
// veritabanlarını, bayi müşterilerininkini, müşteri ve ekip üyesi kendisininkini
// görür. Parola ve veritabanının içi gösterilmez; sunucunun kendi işleri bu
// sayfada değildir. Liste her değişiklikten sonra ve sekme görünürken yarım
// dakikada bir kendiliğinden yeniden okunur.

const REFRESH_MS = 30_000;

type SortKey = 'name' | 'owner' | 'domain' | 'kind' | 'size';
type Sort = { key: SortKey; dir: 1 | -1 };

// The domain page's Databases section, where the database's card is.
const databaseSectionPath = (domainName: string) => `/domains/${encodeURIComponent(domainName)}?section=databases`;
// The same section with its "add a database" form open (DomainDatabaseManager reads ?create=1 once).
const databaseCreatePath = (domainName: string) => `${databaseSectionPath(domainName)}&create=1`;

const ENGINE_NAMES: Record<string, string> = { mariadb: 'MariaDB', postgresql: 'PostgreSQL' };

// Whether the tab is on screen: the half-minute read waits while it is not.
function useTabVisible(): boolean {
    const read = () => typeof document === 'undefined' || document.visibilityState !== 'hidden';
    const [visible, setVisible] = useState(read);
    useEffect(() => {
        if (typeof document === 'undefined' || !document.addEventListener) return;
        const update = () => setVisible(read());
        document.addEventListener('visibilitychange', update);
        return () => document.removeEventListener('visibilitychange', update);
    }, []);
    return visible;
}

export function DatabasesPage() {
    const { t, locale } = useI18n();
    const { role } = useAuth();
    const navigate = useNavigate();
    const isAdmin = role === 'admin';
    // An administrator and a reseller see other people's databases: one list with an Owner column.
    const wide = role === 'admin' || role === 'reseller';
    const list = useRemote(VISIBLE_DATABASES_URL, decodeVisibleDatabases);
    useRefreshEvery(list, REFRESH_MS, useTabVisible());
    const known = lastKnown(list.remote)?.value;
    const current = list.remote.state === 'known';
    const { refusal, setRefusal } = useRefusal();
    const [passwordFor, setPasswordFor] = useState<VisibleDatabase | null>(null);
    const [picking, setPicking] = useState(false);

    const engineWord = (engine: string) => ENGINE_NAMES[engine] ?? (isAdmin ? `${t('databases.engine.other')} (${engine})` : t('databases.engine.other'));
    const sizeText = (db: VisibleDatabase) => (db.sizeBytes === null ? null : sizeWord(db.sizeBytes, locale));
    const reread = () => {
        void list.retry();
        invalidate('databases');
    };

    const remove = async (db: VisibleDatabase) => {
        const size = sizeText(db);
        // Deleting removes the users only this database uses; one another
        // database of the same owner also uses stays (alpha.133 round 5, the
        // server decides the same way). Those databases are in this list: one
        // owner's databases are all visible or none.
        const elsewhere = new Set((known?.databases ?? []).filter((other) => other.id !== db.id).flatMap((other) => other.users.map((user) => user.id)));
        const names = (shared: boolean) => db.users.filter((user) => elsewhere.has(user.id) === shared).map((user) => user.username).join(', ');
        const removedUsers = names(false);
        const keptUsers = names(true);
        await confirmAction({
            title: t('databases.delete.title', { name: db.name }),
            message: t('databases.delete.message'),
            lost: [
                size ? t('databases.delete.lostDataSized', { name: db.name, size }) : t('databases.delete.lostData', { name: db.name }),
                ...(removedUsers ? [t('databases.delete.lostUsers', { users: removedUsers })] : []),
            ],
            kept: [
                ...(db.domain ? [t('databases.delete.keptDomain', { domain: db.domain.name })] : []),
                ...(keptUsers ? [t('databases.delete.keptShared', { users: keptUsers })] : []),
                t('databases.delete.keptOthers'),
            ],
            typedName: { value: db.name, label: t('databases.delete.typed', { name: db.name }) },
            confirmLabel: t('common.delete'),
            tone: 'danger',
            progressTitle: t('databases.delete.progress', { name: db.name }),
            action: async (report) => {
                try {
                    // The server's own steps (alpha.127).
                    const res = await sendWithSteps(report, flowSteps(t, 'dbDelete', ['check!', 'delete', 'records'], { name: db.name }), t, `dbDelete.${db.id}`,
                        (headers) => fetch(databaseDeleteURL(db.id), { method: 'DELETE', headers }));
                    if (!res.ok) {
                        // Said once, in the banner; it stays until the next change succeeds.
                        setRefusal({ problem: await readApiError(res), sentence: 'databases.deleteDbFailed' });
                        return;
                    }
                    setRefusal(null);
                    if (passwordFor?.id === db.id) setPasswordFor(null);
                    showToast('success', t('databases.dbDeleted'));
                } catch {
                    // No answer: whether it was deleted is not known; the list is read again.
                    showToast('error', t('common.resultUnknown'));
                } finally {
                    reread();
                }
            },
        });
    };

    // An action waits for a current answer: after a failed refresh the row may describe a database that is gone.
    const actionsFor = (db: VisibleDatabase, stale: boolean): { actions: [] | [RowAction]; more: RowAction[] } => {
        const why = stale ? t('databases.staleWhy') : undefined;
        return {
            actions: db.domain ? [{ id: 'open', label: t('databases.open'), icon: ArrowRight, onSelect: () => navigate(databaseSectionPath(db.domain!.name)) }] : [],
            more: [
                {
                    id: 'password', label: t('databases.newPassword'), icon: KeyRound,
                    disabled: stale || db.users.length === 0,
                    disabledReason: why ?? (db.users.length === 0 ? t('databases.noUsers') : undefined),
                    onSelect: () => setPasswordFor(db),
                },
                { id: 'delete', label: t('databases.deleteDb'), icon: Trash2, danger: true, disabled: stale, disabledReason: why, onSelect: () => remove(db) },
            ],
        };
    };

    // engines: null is "could not be asked right now", never "none installed";
    // a team member's answer carries none (the server's make-up is not theirs).
    const noEngine = known !== undefined && role !== 'additional_user' && known.engines !== null && !known.engines.some((engine) => engine.installed);
    // A team member's answer is always empty and says nothing about creating
    // (their grants are per domain): "Add database" asks which of their
    // domains, and lists only those where they may manage databases.
    const teamMember = role === 'additional_user';
    const mayAdd = current && known !== undefined && (known.mayCreate || teamMember) && !noEngine;

    // "All databases on this server · 9 databases, 5 accounts".
    const subtitle = (() => {
        const scope = t(wide ? (isAdmin ? 'databases.scope.admin' : 'databases.scope.reseller') : 'databases.scope.customer');
        // No count while it is not known, nor for an empty list (the list itself says so).
        if (!known || known.databases.length === 0) return scope;
        const count = known.databases.length;
        const parts = [count === 1 ? t('databases.count.databasesOne') : t('databases.count.databases', { n: count })];
        if (wide) {
            const owners = new Set(known.databases.filter((db) => isAdmin || !db.owner.self).map((db) => db.owner.id)).size;
            const key = isAdmin ? (owners === 1 ? 'databases.count.accountsOne' : 'databases.count.accounts') : (owners === 1 ? 'databases.count.customersOne' : 'databases.count.customers');
            parts.push(t(key, { n: owners }));
        }
        return `${scope} · ${parts.join(', ')}`;
    })();

    return (
        <div className="p-6 md:p-8" data-databases-page={wide ? 'list' : 'grouped'}>
            <PageHeader
                title={t('nav.databases')}
                subtitle={subtitle}
                breadcrumb={[t('common.home'), t('nav.databases')]}
                actions={mayAdd ? <Button variant="primary" icon={Plus} onClick={() => setPicking(true)} data-database-add="">{t('databases.addDatabase')}</Button> : undefined}
            />

            {role === 'reseller' && known && !known.mayCreate && (
                <Guidance tone="neutral" live="off" title={t('databases.resellerNoAccount')} className="mb-4" data-database-reseller-note="" />
            )}
            <ErrorBanner className="mb-4" error={refusal?.problem ?? null} sentence={refusal?.sentence} />
            {passwordFor && <NewPassword db={passwordFor} onClose={() => setPasswordFor(null)} onSettled={reread} />}

            <RemoteGate
                remote={list.remote}
                checking={t('databases.checking')}
                failed={t('databases.unknown')}
                onRetry={() => void list.retry()}
                busy={list.reading}
            >
                {(shown) => (
                    <>
                        {isAdmin && <EngineGuidance answer={shown.value} onOpen={() => navigate('/components')} engineWord={engineWord} />}
                        {shown.value.databases.length === 0
                            ? <Empty of={shown} role={role} noEngine={noEngine} mayAdd={mayAdd} />
                            : wide
                                ? <DatabaseList shown={shown} isAdmin={isAdmin} engineWord={engineWord} sizeText={sizeText} actionsFor={actionsFor} />
                                : <DatabaseGroups shown={shown} engineWord={engineWord} sizeText={sizeText} actionsFor={actionsFor} />}
                        {isAdmin && (
                            <p className="mt-4 text-sm text-fg-muted" data-database-outside-link="">
                                <Link to="/security#outside-connections" className="text-primary underline decoration-primary/40 underline-offset-2 hover:decoration-primary">
                                    {t('databases.outsideLink')}
                                </Link>
                            </p>
                        )}
                    </>
                )}
            </RemoteGate>

            {picking && <DomainPick teamMember={role === 'additional_user'} onClose={() => setPicking(false)} onPick={(name) => navigate(databaseCreatePath(name))} />}
        </div>
    );
}

// The administrator's guidance about the engines: none installed, or one
// installed that does not run. Nobody else is told how the server is built.
function EngineGuidance({ answer, onOpen, engineWord }: { answer: VisibleDatabases; onOpen: () => void; engineWord: (engine: string) => string }) {
    const { t } = useI18n();
    const open = <Button icon={Server} onClick={onOpen}>{t('databases.openComponents')}</Button>;
    if (answer.engines === null) {
        // Not known right now: said as such, never as "no database service".
        return (
            <Guidance tone="unknown" className="mb-4" data-database-guidance="engines-unknown"
                title={t('databases.enginesUnknown.title')} why={t('databases.enginesUnknown.why')} who={whoWord(t, 'nobody')}
                next={t('databases.enginesUnknown.next')} then={t('databases.enginesUnknown.then')} />
        );
    }
    if (!answer.engines.some((engine) => engine.installed)) {
        return (
            <Guidance tone="blocked" titleAs="h2" className="mb-4" data-database-guidance="no-engine"
                title={t('databases.noEngine.title')} why={t('databases.noEngine.why')} who={whoWord(t, 'you')}
                next={t('databases.noEngine.next')} then={t('databases.noEngine.then')} action={open} />
        );
    }
    const stopped = answer.engines.filter((engine) => engine.installed && !engine.running);
    return (
        <>
            {stopped.map((engine) => (
                <Guidance key={engine.engine} tone="blocked" className="mb-4" data-database-guidance={`stopped-${engine.engine}`}
                    title={t('databases.stopped.title', { engine: engineWord(engine.engine) })} why={t('databases.stopped.why')} who={whoWord(t, 'you')}
                    next={t('databases.stopped.next', { engine: engineWord(engine.engine) })} then={t('databases.stopped.then')} action={open} />
            ))}
        </>
    );
}

// The list is really empty: said per role, for the server's answer only.
function Empty({ of, role, noEngine, mayAdd }: { of: Shown<VisibleDatabases>; role: string; noEngine: boolean; mayAdd: boolean }) {
    const { t } = useI18n();
    if (role === 'admin') {
        // With no engine the guidance above says what to do; the list only says it is empty.
        return <KnownEmpty of={of} icon={Database} title={t('databases.empty.admin')} hint={noEngine ? undefined : t('databases.empty.adminHint')} />;
    }
    if (noEngine) return <KnownEmpty of={of} icon={Database} title={t('databases.noEngine.tenant')} />;
    if (role === 'reseller') return <KnownEmpty of={of} icon={Database} title={t('databases.empty.reseller')} />;
    // A team member's databases are on the pages of the domains they were given.
    if (role === 'additional_user') return <KnownEmpty of={of} icon={Database} title={t('databases.empty.team')} hint={t('databases.empty.teamHint')} />;
    // "Add database" stands once, in the header; the hint points to it.
    return <KnownEmpty of={of} icon={Database} title={t('databases.empty.customer')} hint={t(mayAdd ? 'databases.empty.customerHint' : 'databases.empty.customerClosed')} />;
}

type ActionsFor = (db: VisibleDatabase, stale: boolean) => { actions: [] | [RowAction]; more: RowAction[] };

// One list for an administrator and a reseller: sorted by owner then name,
// a click on a column's heading sorts by it (again: the other way), one
// search over name, owner and domain. On a phone each row is a card.
function DatabaseList({ shown, isAdmin, engineWord, sizeText, actionsFor }: {
    shown: Shown<VisibleDatabases>;
    isAdmin: boolean;
    engineWord: (engine: string) => string;
    sizeText: (db: VisibleDatabase) => string | null;
    actionsFor: ActionsFor;
}) {
    const { t, locale } = useI18n();
    const navigate = useNavigate();
    const [query, setQuery] = useState('');
    const [sort, setSort] = useState<Sort>({ key: 'owner', dir: 1 });
    const collator = useMemo(() => new Intl.Collator(locale, { sensitivity: 'base', numeric: true }), [locale]);
    // The size column, its sort and its line on a phone card exist only when
    // the answer carries a size (alpha.133 reads none: the column is absent).
    const sized = shown.value.databases.some((db) => db.sizeBytes !== null);

    const rows = useMemo(() => {
        // Letter case does not count, nor the Turkish dotted and dotless i: "BAYI1" finds bayi1.
        const fold = (text: string) => text.toLocaleLowerCase(locale).replace(/ı/g, 'i');
        const needle = fold(query.trim());
        const matches = (db: VisibleDatabase) => !needle || [db.name, db.owner.username, db.owner.parent ?? '', db.domain?.name ?? '']
            .some((text) => fold(text).includes(needle));
        const byOwnerThenName = (a: VisibleDatabase, b: VisibleDatabase) => collator.compare(a.owner.username, b.owner.username) || collator.compare(a.name, b.name);
        const value = (db: VisibleDatabase): string | number | null => {
            switch (sort.key) {
                case 'name': return db.name;
                case 'owner': return db.owner.username;
                case 'domain': return db.domain?.name ?? null;
                case 'kind': return engineWord(db.engine);
                case 'size': return db.sizeBytes;
            }
        };
        return shown.value.databases.filter(matches).sort((a, b) => {
            const left = value(a);
            const right = value(b);
            // What is not known (no domain, size not read) goes last either way.
            if (left === null || right === null) return left === right ? byOwnerThenName(a, b) : left === null ? 1 : -1;
            const order = typeof left === 'number' && typeof right === 'number' ? left - right : collator.compare(String(left), String(right));
            return order * sort.dir || byOwnerThenName(a, b);
        });
    }, [shown.value.databases, query, sort, collator, locale, engineWord]);

    const heading = (key: SortKey, label: string, className = '') => {
        const active = sort.key === key;
        const Arrow = sort.dir === 1 ? ArrowUp : ArrowDown;
        return (
            <th className={`px-4 py-2.5 ${className}`} aria-sort={active ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none'}>
                <button type="button" className="inline-flex items-center gap-1 font-semibold hover:text-fg" data-sort={key}
                    onClick={() => setSort(active ? { key, dir: sort.dir === 1 ? -1 : 1 } : { key, dir: 1 })}>
                    {label}
                    {active && <Arrow className="h-3.5 w-3.5" aria-hidden="true" />}
                </button>
            </th>
        );
    };
    const cell = 'px-4 py-3 max-sm:p-0';
    const labelled = 'max-sm:basis-full max-sm:before:mr-2 max-sm:before:text-fg-subtle max-sm:before:content-[attr(data-label)]';

    return (
        <div className="rounded-xl border border-border-strong bg-surface p-4">
            <div className="mb-4">
                <SearchInput value={query} onChange={setQuery} placeholder={t('databases.search')} />
            </div>
            {rows.length === 0 ? (
                <KnownEmpty of={shown} icon={SearchX} title={t('databases.noMatch')} hint={t('databases.noMatchHint', { q: query.trim() })}
                    action={<Button icon={X} onClick={() => setQuery('')} data-database-clear-search="">{t('databases.clearSearch')}</Button>} />
            ) : (
                <div className="overflow-x-auto rounded-lg border border-border max-sm:overflow-visible">
                    <table className="w-full text-sm max-sm:block">
                        <thead className="max-sm:hidden">
                            <tr className="border-b border-border bg-surface-2/50 text-left text-xs text-fg-muted">
                                {heading('name', t('databases.col.name'))}
                                {heading('owner', t('databases.col.owner'))}
                                {heading('domain', t('databases.col.domain'))}
                                {heading('kind', t('databases.col.kind'))}
                                {sized && heading('size', t('databases.col.size'), 'text-right')}
                                <th className="px-4 py-2.5"><span className="sr-only">{t('databases.col.actions')}</span></th>
                            </tr>
                        </thead>
                        <tbody className="max-sm:block">
                            {rows.map((db) => {
                                const { actions, more } = actionsFor(db, shown.stale);
                                const size = sizeText(db);
                                return (
                                    <tr key={db.id} data-database-row={db.name}
                                        className="border-b border-border last:border-0 hover:bg-surface-2/60 max-sm:flex max-sm:flex-wrap max-sm:items-start max-sm:gap-x-3 max-sm:gap-y-1.5 max-sm:p-4">
                                        <td className={`${cell} max-sm:basis-full`}>
                                            {db.domain ? (
                                                <button type="button" onClick={() => navigate(databaseSectionPath(db.domain!.name))}
                                                    className="break-all text-left font-mono text-sm text-primary underline decoration-primary/40 underline-offset-2 hover:decoration-primary">
                                                    {db.name}
                                                </button>
                                            ) : <span className="break-all font-mono text-sm text-fg">{db.name}</span>}
                                        </td>
                                        <td className={`${cell} ${labelled}`} data-label={t('databases.col.owner')}>
                                            <span className="font-semibold text-fg" data-database-owner={db.owner.username}>{db.owner.username}</span>
                                            <OwnerNote owner={db.owner} isAdmin={isAdmin} />
                                        </td>
                                        <td className={`${cell} ${labelled}`} data-label={t('databases.col.domain')}>
                                            {db.domain ? <span className="break-all text-fg">{db.domain.name}</span> : <span className="text-fg-muted">{t('databases.unbound')}</span>}
                                        </td>
                                        <td className={`${cell} ${labelled} text-fg`} data-label={t('databases.col.kind')}>{engineWord(db.engine)}</td>
                                        {sized && (
                                            <td className={`${cell} ${labelled} whitespace-nowrap text-right text-fg max-sm:text-left`} data-label={t('databases.col.size')}>
                                                {size ?? <span className="text-fg-subtle" title={t('databases.sizeNotKnown')}>—<span className="sr-only">{t('databases.sizeNotKnown')}</span></span>}
                                            </td>
                                        )}
                                        <td className={`${cell} max-sm:basis-full max-sm:pt-2`}>
                                            <RowActions name={db.name} actions={actions} more={more} />
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

// Under the owner's name: "you" for the reader's own databases; for an
// administrator, the reseller a customer belongs to, or that the owner is a reseller.
function OwnerNote({ owner, isAdmin }: { owner: VisibleDatabase['owner']; isAdmin: boolean }) {
    const { t } = useI18n();
    const note = owner.self ? t('databases.owner.you')
        : isAdmin && owner.parent ? t('databases.owner.reseller', { name: owner.parent })
            : isAdmin && owner.role === 'reseller' ? t('databases.owner.isReseller')
                : null;
    return note ? <span className="block text-xs text-fg-muted max-sm:ml-1 max-sm:inline">{note}</span> : null;
}

// A customer's (and a team member's) databases, grouped by domain; the ones
// bound to no domain last.
function DatabaseGroups({ shown, engineWord, sizeText, actionsFor }: {
    shown: Shown<VisibleDatabases>;
    engineWord: (engine: string) => string;
    sizeText: (db: VisibleDatabase) => string | null;
    actionsFor: ActionsFor;
}) {
    const { t, locale } = useI18n();
    const groups = useMemo(() => {
        const collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
        const byDomain = new Map<string, VisibleDatabase[]>();
        for (const db of shown.value.databases) {
            const key = db.domain?.name ?? '';
            byDomain.set(key, [...(byDomain.get(key) ?? []), db]);
        }
        return [...byDomain.entries()]
            .sort(([a], [b]) => (a === '' ? 1 : b === '' ? -1 : collator.compare(a, b)))
            .map(([domain, items]) => ({ domain, items: items.sort((a, b) => collator.compare(a.name, b.name)) }));
    }, [shown.value.databases, locale]);

    return (
        <div className="space-y-4">
            {groups.map(({ domain, items }) => (
                <section key={domain || '-'} className="rounded-xl border border-border-strong bg-surface p-4" data-database-group={domain || 'unbound'}>
                    <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-fg">
                        {domain ? <Globe className="h-4 w-4 text-fg-muted" aria-hidden="true" /> : <Database className="h-4 w-4 text-fg-muted" aria-hidden="true" />}
                        <span className="min-w-0 break-all">{domain || t('databases.group.unbound')}</span>
                    </h2>
                    <ul className="divide-y divide-border rounded-lg border border-border">
                        {items.map((db) => {
                            const { actions, more } = actionsFor(db, shown.stale);
                            const size = sizeText(db);
                            // A size and a creation time are said only when known (alpha.133 reads no size).
                            const facts = [size, db.createdAt ? t('databases.added', { date: formatDate(db.createdAt, locale) }) : null].filter((fact): fact is string => fact !== null);
                            return (
                                <li key={db.id} className="flex flex-wrap items-center gap-3 p-3 max-sm:items-start" data-database-row={db.name}>
                                    {(db.engine === 'mariadb' || db.engine === 'postgresql') && <EngineMark engine={db.engine} size="sm" />}
                                    <div className="min-w-0 flex-1">
                                        <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
                                            <span className="font-semibold text-fg">{engineWord(db.engine)}</span>
                                            <span className="break-all font-mono text-fg">{db.name}</span>
                                        </p>
                                        {facts.length > 0 && <p className="text-xs text-fg-muted">{facts.join(' · ')}</p>}
                                    </div>
                                    <RowActions name={db.name} actions={actions} more={more} className="max-sm:basis-full" />
                                </li>
                            );
                        })}
                    </ul>
                </section>
            ))}
        </div>
    );
}

// A database user's new password: the user is asked for when the database
// has several; the field and its rule are the shared ones, and the server's
// steps are shown in the locked window. No password is ever read back.
function NewPassword({ db, onClose, onSettled }: { db: VisibleDatabase; onClose: () => void; onSettled: () => void }) {
    const { t } = useI18n();
    const [userId, setUserId] = useState<number | null>(db.users.length === 1 ? db.users[0].id : null);
    const box = useRef<HTMLDivElement | null>(null);
    useEffect(() => { box.current?.scrollIntoView?.({ block: 'nearest' }); }, [db.id]);
    const user = db.users.find((item) => item.id === userId) ?? null;
    return (
        <div ref={box} className="mb-4 space-y-3 rounded-xl border border-border-strong bg-surface p-4" data-database-password={db.name}>
            <h2 className="text-base font-semibold text-fg">{t('databases.password.title', { name: db.name })}</h2>
            {db.users.length > 1 && (
                <fieldset>
                    <legend className="mb-2 text-sm font-medium text-fg">{t('databases.password.which')}</legend>
                    <div className="flex flex-wrap gap-2">
                        {db.users.map((item) => (
                            <label key={item.id} className={`inline-flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-1.5 text-sm max-sm:min-h-11 ${item.id === userId ? 'border-primary bg-primary/5 text-fg' : 'border-border-strong text-fg'}`}>
                                <input type="radio" name={`database-user-${db.id}`} value={item.id} checked={item.id === userId} onChange={() => setUserId(item.id)} />
                                <span className="font-mono">{item.username}</span>
                            </label>
                        ))}
                    </div>
                </fieldset>
            )}
            {user ? (
                <Suspense fallback={<p className="text-sm text-fg-muted">{t('common.loading')}</p>}>
                    <DatabaseUserPassword key={user.id} url={databaseUserPasswordURL(user.id)} user={user.username} onClose={onClose} onSettled={onSettled} />
                </Suspense>
            ) : (
                <Button onClick={onClose}>{t('common.cancel')}</Button>
            )}
        </div>
    );
}

interface PickableDomain { id: number; name: string }

// A domain where a database may be added: not being deleted, databases not
// closed by its plan, and for a team member, databases granted to manage.
function pickable(raw: unknown, teamMember: boolean): PickableDomain | null {
    const domain = raw as { id?: unknown; domain_name?: unknown; status?: unknown; rights_closed?: unknown; access?: unknown } | null;
    if (!domain || typeof domain.id !== 'number' || typeof domain.domain_name !== 'string') return null;
    if (domain.status === 'pending') return null;
    if (Array.isArray(domain.rights_closed) && domain.rights_closed.includes('databases')) return null;
    // The grant as the domain list carries it (auth/domainAccess.ts): only "manage" may add.
    if (teamMember && (domain.access as { databases?: unknown } | undefined)?.databases !== 'manage') return null;
    return { id: domain.id, name: domain.domain_name };
}

// "For which domain?": a database is added on its domain's page, with that
// page's own form; this only takes the person there.
function DomainPick({ teamMember, onClose, onPick }: { teamMember: boolean; onClose: () => void; onPick: (name: string) => void }) {
    const { t, locale } = useI18n();
    // The address and decoder the domain pages read, so the answer is shared.
    const domains = useRemote('/api/v1/domains', decodeList<unknown>, { freshFor: 30_000 });
    return (
        <Dialog id="database-domain-pick" title={t('databases.pick.title')} description={t('databases.pick.hint')} icon={Database} onDismiss={onClose}
            actions={<Button onClick={onClose}>{t('common.cancel')}</Button>}>
            <RemoteGate remote={domains.remote} checking={t('databases.pick.checking')} failed={t('databases.pick.unknown')} onRetry={() => void domains.retry()} busy={domains.reading}>
                {(shown) => {
                    const collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
                    const choices = shown.value.map((raw) => pickable(raw, teamMember)).filter((item): item is PickableDomain => item !== null)
                        .sort((a, b) => collator.compare(a.name, b.name));
                    if (choices.length === 0) return <p className="text-sm text-fg" data-database-pick-none="">{t('databases.pick.none')}</p>;
                    return (
                        <ul className="space-y-2">
                            {choices.map((domain) => (
                                <li key={domain.id}>
                                    <button type="button" data-database-pick={domain.name} disabled={shown.stale} onClick={() => { onClose(); onPick(domain.name); }}
                                        className="flex w-full items-center gap-2 rounded-lg border border-border-strong px-3 py-2.5 text-left text-sm text-fg transition-colors hover:bg-surface-2 disabled:opacity-50 max-sm:min-h-11">
                                        <Globe className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                                        <span className="min-w-0 flex-1 break-all">{domain.name}</span>
                                        <ArrowRight className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                                    </button>
                                </li>
                            ))}
                        </ul>
                    );
                }}
            </RemoteGate>
        </Dialog>
    );
}
