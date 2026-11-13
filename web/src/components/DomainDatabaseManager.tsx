import { useEffect, useState } from 'react';
import { replaceCurrentRouterURL, useLocation } from '../router';
import { Database, Plus, Trash2, ExternalLink } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { formatDate } from '../lib/dateFormat';
import { useAuth } from '../auth/AuthContext';
import { readApiError } from '../lib/apiError';
import { useRefusal } from '../lib/useRefusal';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { useLostAnswer } from '../lib/lostAnswer';
import { Button, Checking, CouldNotCheck, ErrorBanner, RemoteGate, ResultUnknown } from './ui';
import { invalidate, lastKnown, mapRemote, useRemote, type Remote } from '../lib/remote';
import { decodeDomainDatabases, databaseEngine, engineLabel, type DatabaseEngine, type DatabaseInfo, type DatabaseType } from '../lib/domainDatabases';
import { useHostingCapabilities, type CapabilitiesRemote } from '../lib/hostingCapabilities';
import { confirmAction } from '../lib/confirmAction';
import { databaseDeleteDescribable, databaseDeleteUserLines, databaseDeletedKeptWords } from '../lib/databaseDeleteWords';
import { PasswordInput } from './PasswordInput';
import { AdminInstallLink } from './AdminInstallLink';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { DatabaseTools } from './DatabaseTools';
import { DatabaseRemoteAccess } from './DatabaseRemoteAccess';
import { DatabaseUsers } from './DatabaseUsers';
import { EngineMark } from './EngineMark';

interface DomainDatabaseManagerProps {
    domainId: number;
    domainName: string;
    readOnly?: boolean;
    isAdditionalUser?: boolean;
}

// The database web tools (phpMyAdmin / phpPgAdmin). Installed → a launch
// button opening the panel-proxied tool. Parent engine present but the tool
// not → a hint pointing to Services. Neither → nothing (the parent-engine
// requirement means this whole page would be hidden anyway). The card is
// drawn only for a known answer; while the capabilities are being read, or
// could not be read, the manager above says so once, for both of them.
// Veritabanı web araçları (phpMyAdmin / phpPgAdmin). Kurulu → panel-vekilli
// aracı açan bir düğme. Üst motor var ama araç yok → Servisler'e yönlendiren
// bir ipucu. Hiçbiri → hiçbir şey. Kart yalnız bilinen yanıt için çizilir.
function DBToolsCard({ capabilities }: { capabilities: CapabilitiesRemote }) {
    const { t } = useI18n();
    if (capabilities.state !== 'known') return null;
    const caps = capabilities.value;

    const tools = [
        { id: 'phpmyadmin', label: 'phpMyAdmin', engine: 'mariadb' },
        { id: 'phppgadmin', label: 'phpPgAdmin', engine: 'postgresql' },
    ];
    const installed = new Set(caps.db_tools);
    const engines = new Set(caps.database_servers);
    // Only tools whose parent engine is installed are relevant here.
    // Yalnız üst motoru kurulu olan araçlar burada anlamlıdır.
    const relevant = tools.filter((tl) => engines.has(tl.engine));
    if (relevant.length === 0) return null;

    return (
        <div className="rounded-lg border border-border bg-surface-2/50 p-4">
            <h4 className="mb-1 text-sm font-semibold text-fg">{t('dbtools.title')}</h4>
            <p className="mb-3 text-xs text-fg-subtle">{t('dbtools.hint')}</p>
            <div className="flex flex-wrap gap-2">
                {relevant.map((tl) =>
                    installed.has(tl.id) ? (
                        <a
                            key={tl.id}
                            href={`/dbtool/${tl.id}/`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-fg hover:bg-primary/90"
                        >
                            <ExternalLink className="h-3.5 w-3.5" />
                            {t('dbtools.open', { name: tl.label })}
                        </a>
                    ) : (
                        // alpha.127: the administrator installs the tool from here
                        // (its own install window); anyone else is told who does.
                        // Yönetici aracı buradan kurar; diğerleri kimin kurduğunu okur.
                        <AdminInstallLink
                            key={tl.id}
                            component={tl.id}
                            fallback={
                                <span className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-fg-subtle">
                                    {t('dbtools.install', { name: tl.label })}
                                </span>
                            }
                        />
                    ),
                )}
            </div>
        </div>
    );
}

// The decoder of this domain's databases is lib/domainDatabases.ts: one
// address, one decoder, shared with the Backups tab.
// Bu alan adının veritabanlarının çözücüsü lib/domainDatabases.ts'tedir;
// Yedekler sekmesiyle paylaşılır.

const engineLabels: Record<string, DatabaseEngine> = {
    mariadb: { value: 'mysql', label: 'MariaDB' },
    postgresql: { value: 'postgresql', label: 'PostgreSQL' },
};

export function DomainDatabaseManager({
    domainId,
    domainName,
    readOnly = false,
    isAdditionalUser = false,
}: DomainDatabaseManagerProps) {
    const { t, locale } = useI18n();
    const [creating, setCreating] = useState(false);
    // "Add database" on the Databases page brings the person here with
    // ?create=1 (alpha.133): the form starts open, and the parameter leaves
    // the address at once so Back or a reload does not open it again.
    // Veritabanları sayfasındaki "Veritabanı ekle" buraya ?create=1 ile gelir;
    // form açık başlar ve parametre adresten hemen kalkar.
    const { pathname, search, hash } = useLocation();
    const [showCreateForm, setShowCreateForm] = useState(() => !readOnly && new URLSearchParams(search).get('create') === '1');
    useEffect(() => {
        const params = new URLSearchParams(search);
        if (!params.has('create')) return;
        params.delete('create');
        const query = params.toString();
        replaceCurrentRouterURL(`${pathname}${query ? `?${query}` : ''}${hash}`);
    }, [pathname, search, hash]);

    // The list is read through lib/remote.ts: being read, read, or not
    // readable. "No databases yet" is drawn only for an answer the server gave.
    // A new domain is a new address, so nothing of the previous domain's
    // answer is ever shown for this one.
    // Liste lib/remote.ts üzerinden okunur: okunuyor, okundu ya da okunamadı.
    // "Henüz veritabanı yok" yalnız sunucunun verdiği yanıt için çizilir. Yeni
    // alan adı yeni adrestir; önceki alan adının yanıtı bunun için gösterilmez.
    const list = useRemote(`/api/v1/domains/${domainId}/databases`, decodeDomainDatabases);
    const listed = lastKnown(list.remote);
    const answer = useLostAnswer(() => list.retry());

    // Only engines that are actually installed may be offered — a dropdown
    // with MySQL and PostgreSQL on a server that runs neither is a settings
    // page for ghosts. The engine ids map to the panel's db types.
    //
    // Server-wide capability inventory is admin-only. A team member is given
    // only the tenant-safe available_types returned with this domain's
    // databases, so their engines come from the list's own answer and no
    // server-wide read is made for them.
    //
    // Either way the engines are known, being checked, or could not be
    // checked. A database is created only on an engine the server named: there
    // is no default engine.
    // Yalnız gerçekten kurulu motorlar sunulabilir. Sunucu geneli envanter
    // yalnız yöneticiye aittir; ekip üyesine yalnız bu alan adının
    // veritabanlarıyla dönen available_types verilir. Her iki durumda motorlar
    // ya bilinir, ya kontrol ediliyordur, ya da kontrol edilememiştir;
    // varsayılan motor yoktur.
    const capabilities = useHostingCapabilities({ enabled: !isAdditionalUser });
    // alpha.129: the connection guide gives the administrator the SSH tunnel
    // and anyone else the web tool, when it is known to be installed.
    const isAdmin = useAuth().role === 'admin';
    const webToolOf = (type: string): 'installed' | 'missing' | undefined => {
        if (capabilities.remote.state !== 'known') return undefined;
        return capabilities.remote.value.db_tools.includes(type === 'mysql' ? 'phpmyadmin' : 'phppgadmin') ? 'installed' : 'missing';
    };
    const engineSource: Remote<DatabaseEngine[]> = isAdditionalUser
        ? mapRemote(list.remote, (value) => value.availableTypes)
        : mapRemote(capabilities.remote, (value) => value.database_servers.flatMap((id) => engineLabels[id] ?? []));
    const engines = engineSource.state === 'known' ? engineSource.value : [];
    // Nothing is created while an earlier create has no result and the list
    // has not been read again.
    // Önceki oluşturmanın sonucu yokken ve liste yeniden okunmamışken hiçbir
    // şey oluşturulmaz.
    const enginesReady = !readOnly && engineSource.state === 'known' && engines.length > 0;
    const canCreate = enginesReady && !answer.holding;

    // Form state
    const [dbName, setDbName] = useState('');
    const [chosenType, setDbType] = useState<DatabaseType | null>(null);
    const dbType = engines.some((engine) => engine.value === chosenType) ? chosenType : engines[0]?.value ?? null;
    const [dbPassword, setDbPassword] = useState('');

    // A refusal stays on the page, with its fix, until the next change
    // succeeds (owner feedback 2026-10-31).
    // Said once, in the banner (useRefusal): no notice repeats it.
    const { refusal, refuse, clear: clearRefusal } = useRefusal();
    const passwordOK = passwordAcceptable(dbPassword, 'database_user');

    const handleCreateDatabase = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!canCreate || dbType === null) return;

        if (!dbName || !passwordOK) {
            showToast('error', t('ddb.namePasswordRequired'));
            return;
        }
        clearRefusal();

        setCreating(true);
        try {
            // The request carries an identity the server keeps (D-029), and a
            // lost answer has been asked for once more before `send` gives up.
            // When there is still no result, the list is read again and asked
            // one thing: does it name this database? If it does, the form is
            // closed and the notice says it was made; if not, what was typed
            // stays. Nothing here says "failed" for a result nobody knows
            // (10 Oct 2026: a dropped connection was "Failed to create
            // database").
            // İstek, sunucunun sakladığı bir kimlik taşır (D-029). Sonuç yine
            // yoksa liste yeniden okunur ve tek bir şey sorulur: bu veritabanını
            // adlandırıyor mu? Kimsenin bilmediği sonuca "başarısız" denmez.
            const name = dbName;
            const res = await answer.send(`/api/v1/domains/${domainId}/databases`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: dbName,
                    type: dbType,
                    password: dbPassword
                })
            }, {
                shows: ([read]) => {
                    const value = read?.value as { databases?: Array<{ name?: string }> } | undefined;
                    return Array.isArray(value?.databases) ? value.databases.some((db) => db.name === name) : null;
                },
                made: () => {
                    setShowCreateForm(false);
                    setDbName('');
                    setDbPassword('');
                    invalidate('databases');
                },
            });
            if (!res) return;
            answer.settle();

            if (res.ok) {
                const data = await res.json();
                showToast('success', t('ddb.created', { name: String(data.name ?? name) }));
                setShowCreateForm(false);
                setDbName('');
                setDbPassword('');
                invalidate('databases');
            } else {
                refuse(await readApiError(res), 'ddb.createFailed');
            }
        } catch {
            // The answer arrived and could not be read as one.
            // Yanıt geldi ama yanıt olarak okunamadı.
            answer.lose(undefined, 'asked');
        } finally {
            setCreating(false);
        }
    };

    const handleDeleteDatabase = async (db: DatabaseInfo) => {
        // Not offered while it cannot be said which users would go (review N-F).
        if (readOnly || list.remote.state !== 'known' || answer.holding || !databaseDeleteDescribable(db.users ?? [])) return;
        // alpha.133 round 5: the same lines as the Databases page - which users
        // go with it and which stay (lib/databaseDeleteWords.ts).
        const users = databaseDeleteUserLines(t, db.users ?? []);
        await confirmAction({ message: t('ddb.confirmDelete', { name: db.name }), confirmLabel: t('common.delete'), tone: 'danger',
            ...(users.lost.length > 0 ? { lost: users.lost } : {}),
            ...(users.kept.length > 0 ? { kept: users.kept } : {}),
            action: async (report) => {
                try {
                    // The server's own steps (alpha.127).
                    const res = await sendWithSteps(report, flowSteps(t, 'dbDelete', ['check!', 'delete', 'records'], { name: db.name }), t, `dbDelete.${db.id}`,
                        (headers) => fetch(`/api/v1/domains/${domainId}/databases/${db.id}`, { method: 'DELETE', headers }));

                    if (res.ok) {
                        clearRefusal();
                        const kept = await databaseDeletedKeptWords(t, db.name, res);
                        showToast('success', kept ?? t('ddb.deleted', { name: db.name }));
                        invalidate('databases');
                    } else {
                        // The server's reason (a user that still has grants…) is kept, on the page too.
                        refuse(await readApiError(res), 'ddb.deleteFailed');
                        // alpha.133 round 5: an unknown or partial result is read again too.
                        invalidate('databases');
                    }
                } catch {
                    // No answer: whether it was deleted is not known; the list is read again.
                    showToast('error', t('common.resultUnknown'));
                    invalidate('databases');
                }
            },
        });
    };

    return (
        <div className="space-y-6">
            <div>
                <h3 className="text-lg font-bold text-fg mb-2">{t('ddb.title')}</h3>
                <p className="text-sm text-fg-muted">
                    {t('ddb.subtitle', { domain: domainName })}
                </p>
            </div>

            {/* Create Database Button. It is in its place from the start and
                becomes usable when the engines are known; the line beside it
                says why it is not usable yet.
                Veritabanı Oluştur düğmesi. Baştan yerindedir ve motorlar
                bilindiğinde kullanılabilir olur; yanındaki satır henüz neden
                kullanılamadığını söyler. */}
            <ResultUnknown answer={answer} />
            <ErrorBanner error={refusal?.problem ?? null} sentence={refusal?.sentence} />

            {!readOnly && !showCreateForm && !(engineSource.state === 'known' && engines.length === 0) && (
                <div className="flex flex-wrap items-center gap-3">
                    {/* The shared primary button: its face is readable in both
                        themes, enabled and disabled. The hand-written one drew
                        white on the dark theme's light primary.
                        Ortak birincil düğme: yüzü iki temada da, etkin ve
                        kapalıyken okunur. Elle yazılanı, koyu temanın açık
                        birincil rengi üstüne beyaz çiziyordu. */}
                    <Button variant="primary" icon={Plus} disabled={!canCreate} onClick={() => setShowCreateForm(true)}>
                        {t('ddb.create')}
                    </Button>
                    {engineSource.state === 'loading' && <Checking label={t('db.checkingEngines')} />}
                </div>
            )}

            {/* The engines could not be checked. For a team member that is the
                list's own read, and the list says so below.
                Motorlar kontrol edilemedi. Ekip üyesinde bu listenin kendi
                okumasıdır ve liste bunu aşağıda söyler. */}
            {!readOnly && !isAdditionalUser && engineSource.state === 'unknown' && (
                <CouldNotCheck
                    text={t('db.enginesUnknown')}
                    onRetry={() => void capabilities.retry()}
                    busy={capabilities.reading}
                />
            )}

            {!readOnly && engineSource.state === 'known' && engines.length === 0 && (
                <div className="rounded-lg border border-info/30 bg-info/10 px-4 py-3 text-sm text-fg">
                    {isAdditionalUser ? t('db.teamEngineUnavailable') : t('databases.noServersHint')}
                </div>
            )}

            {/* Create Database Form */}
            {showCreateForm && enginesReady && (
                <div className="bg-surface-2/50 rounded-lg p-6 border border-border">
                    <h4 className="text-md font-semibold text-fg mb-4">{t('ddb.formTitle')}</h4>
                    <form onSubmit={handleCreateDatabase} className="space-y-4">
                        <div>
                            <label className="block text-sm text-fg-muted mb-2">{t('ddb.name')}</label>
                            <input
                                type="text"
                                value={dbName}
                                onChange={(e) => setDbName(e.target.value)}
                                placeholder="myapp"
                                className="bg-surface border border-border rounded px-4 py-2 text-fg focus:border-primary field-md"
                                required
                            />
                            <p className="text-xs text-fg-subtle mt-1">
                                {t('ddb.prefix', { name: `${domainName.replace(/\./g, '_')}_${dbName}` })}
                            </p>
                        </div>

                        <div>
                            <label className="block text-sm text-fg-muted mb-2">{t('ddb.type')}</label>
                            <select
                                value={dbType ?? ''}
                                onChange={(e) => setDbType(e.target.value as DatabaseType)}
                                className="bg-surface border border-border rounded px-4 py-2 text-fg focus:border-primary field-sm"
                            >
                                {engines.map((eng) => (
                                    <option key={eng.value} value={eng.value}>{eng.label}</option>
                                ))}
                            </select>
                        </div>

                        <div>
                            <label className="block text-sm text-fg-muted mb-2">{t('ddb.password')}</label>
                            <PasswordInput
                                field="database_user"
                                value={dbPassword}
                                onChange={setDbPassword}
                                placeholder={t('ddb.passwordPlaceholder')}
                                maxLength={128}
                                required
                            />
                        </div>

                        <div className="flex gap-2">
                            <button
                                type="submit"
                                disabled={creating || dbType === null || answer.holding || !passwordOK}
                                className="px-6 py-2 bg-success text-white rounded hover:bg-success disabled:opacity-50 flex items-center gap-2"
                            >
                                <Database className="w-4 h-4" />
                                {creating ? t('ddb.creating') : t('ddb.create')}
                            </button>
                            <button
                                type="button"
                                onClick={() => {
                                    setShowCreateForm(false);
                                    setDbName('');
                                    setDbPassword('');
                                }}
                                className="px-6 py-2 bg-surface-3 text-fg rounded hover:bg-surface-3"
                            >
                                {t('common.cancel')}
                            </button>
                        </div>
                    </form>
                </div>
            )}

            {/* Database List */}
            <div className="bg-surface-2/50 rounded-lg border border-border">
                <div className="flex items-center justify-between p-4 border-b border-border">
                    <div className="flex items-center gap-2">
                        <Database className="w-5 h-5 text-primary" />
                        <h4 className="text-md font-semibold text-fg">{t('databases.tab.databases')}</h4>
                        {listed && (
                            <span className="text-sm text-fg-muted">
                                ({listed.value.databases.length})
                            </span>
                        )}
                    </div>
                    {/* alpha.131 (owner: no manual refresh): every change on this
                        page reads the list again by itself (invalidate). */}
                </div>

                <div className="p-4">
                    <RemoteGate
                        remote={list.remote}
                        checking={t('db.checking')}
                        failed={t('db.unknown')}
                        onRetry={() => void list.retry()}
                        busy={list.reading}
                    >
                        {(shown) => (shown.value.databases.length === 0 ? (
                        <div className="text-center text-fg-subtle py-12">
                            <Database className="w-12 h-12 mx-auto mb-2 opacity-50" />
                            <p>{t('databases.empty.databases')}</p>
                            {canCreate && (
                                <p className="text-sm mt-1">{t('databases.empty.databasesHint')}</p>
                            )}
                        </div>
                    ) : (
                        <div className="space-y-3">
                            {shown.value.databases.map((db) => {
                                const engine = databaseEngine(db.engine, db.type);
                                return (
                                <div
                                    key={db.id}
                                    className="rounded-xl border border-border bg-surface p-4 transition-colors hover:border-border-strong"
                                    data-database-card={db.id}
                                >
                                    {/* alpha.129: the engine's mark and colour, its
                                        name from the server's engine (MariaDB is
                                        never "MYSQL"), then the database's name. */}
                                    <div className="flex items-start gap-3">
                                        <EngineMark engine={engine} />
                                        <div className="min-w-0 flex-1">
                                            <h5 className="break-all font-mono text-base font-semibold text-fg">{db.name}</h5>
                                            <p className="mt-0.5 text-xs text-fg-muted">
                                                <span className="font-medium text-fg">{engineLabel(engine, db.engine_version)}</span>
                                                <span aria-hidden="true"> · </span>
                                                {t('ddb.createdAt')} {formatDate(new Date(db.created_at), locale)}
                                            </p>
                                        </div>
                                        {!readOnly && (
                                            <button
                                                onClick={() => handleDeleteDatabase(db)}
                                                disabled={shown.stale || !databaseDeleteDescribable(db.users ?? [])}
                                                className="rounded-md p-2 text-fg-subtle transition-colors hover:bg-danger/10 hover:text-danger disabled:pointer-events-none disabled:opacity-40"
                                                title={t('databases.deleteDatabase', { name: db.name })}
                                                aria-label={t('databases.deleteDatabase', { name: db.name })}
                                            >
                                                <Trash2 className="h-4 w-4" />
                                            </button>
                                        )}
                                    </div>
                                    {/* alpha.128 (G7): the tools of this database;
                                        alpha.129: its users, each with a new
                                        password, right under its numbers. */}
                                    <DatabaseTools domainId={domainId} db={db} readOnly={readOnly || shown.stale} isAdmin={isAdmin}
                                        webTool={webToolOf(db.type)}
                                        onRefusal={(problem, sentence) => refuse(problem, sentence)} onClear={clearRefusal}
                                        afterStats={<DatabaseUsers domainId={domainId} databaseId={db.id} databaseName={db.name} users={db.users ?? []} readOnly={readOnly || shown.stale} />} />
                                    {!isAdditionalUser && <DatabaseRemoteAccess db={db} readOnly={readOnly || shown.stale} />}
                                </div>
                                );
                            })}
                        </div>
                        ))}
                    </RemoteGate>
                </div>
            </div>

            {!isAdditionalUser && <DBToolsCard capabilities={capabilities.remote} />}
        </div>
    );
}
