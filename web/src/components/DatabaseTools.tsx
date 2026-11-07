import { useState, type ReactNode } from 'react';
import { CopyPlus, Download, ShieldCheck, Upload, Wrench } from 'lucide-react';
import type { TranslationKey } from '../i18n/en';
import { formatSize, useDataToolsWords } from './dataToolsWords';
import { readApiError, type ApiError } from '../lib/apiError';
import { followOperation, confirmAction, type OperationReport } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { newRequestId } from '../lib/requestIdentity';
import { invalidate, useRemote } from '../lib/remote';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { showToast } from './Toast';
import { Button, inputClass } from './ui';
import { PasswordInput } from './PasswordInput';

// The tools of one database (alpha.128, G7): size and tables, connection
// details, export (a .sql.gz download), import (upload a dump; a safety copy
// is taken first and a load that stops puts the database back), copy into a
// new database, and check/repair. Every change runs in the locked progress
// window with the server's own steps; a refusal is said once, in the
// manager's banner.
//
// Bir veritabanının araçları: boyut, bağlantı, dışa/içe aktarma, kopya,
// denetim. Her değişiklik sunucunun adımlarıyla kilitli pencerede çalışır.

export interface DatabaseToolsTarget {
    id: number;
    name: string;
    type: string;
}

interface ToolsInfo {
    known: boolean;
    sizeBytes: number;
    tables: number;
    connections: number;
    host: string;
    port: number;
    user: string;
    canRepair: boolean;
    importBlocker: '' | 'client_unsafe' | 'no_user';
}

function decodeInfo(raw: unknown): ToolsInfo {
    const v = (raw ?? {}) as Record<string, unknown>;
    const num = (x: unknown) => (typeof x === 'number' && Number.isFinite(x) ? x : 0);
    const str = (x: unknown) => (typeof x === 'string' ? x : '');
    const blocker = str(v.import_blocker);
    return {
        known: v.known === true,
        sizeBytes: num(v.size_bytes),
        tables: num(v.tables),
        connections: num(v.connections),
        host: str(v.host) || 'localhost',
        port: num(v.port),
        user: str(v.user),
        canRepair: v.can_repair === true,
        importBlocker: blocker === 'client_unsafe' || blocker === 'no_user' ? blocker : '',
    };
}

const UPLOAD_CHUNK = 4 * 1024 * 1024;
const UPLOAD_MAX = 4 * 1024 * 1024 * 1024;

export function DatabaseTools({ domainId, db, readOnly, onRefusal, onClear, isAdmin = false, webTool, afterStats }: {
    domainId: number;
    db: DatabaseToolsTarget;
    readOnly: boolean;
    /** The administrator is shown the SSH tunnel route (alpha.129). */
    isAdmin?: boolean;
    /** Whether this engine's web tool is installed, when known. */
    webTool?: 'installed' | 'missing';
    /** Drawn under the numbers (alpha.129: the database's users). */
    afterStats?: ReactNode;
    onRefusal: (problem: ApiError, sentence: TranslationKey) => void;
    onClear: () => void;
}) {
    const { t } = useDataToolsWords();
    const base = `/api/v1/domains/${domainId}/databases/${db.id}`;
    const tools = useRemote(`${base}/tools`, decodeInfo, { freshFor: 30_000 });
    const info = tools.remote.state === 'known' ? tools.remote.value : null;
    const [panel, setPanel] = useState<'' | 'import' | 'copy'>('');
    const [busy, setBusy] = useState(false);
    const [file, setFile] = useState<File | null>(null);
    const [emptyFirst, setEmptyFirst] = useState(true);
    const [copyName, setCopyName] = useState('');
    const [copyPassword, setCopyPassword] = useState('');
    const [check, setCheck] = useState<null | { checked: number; problems: Array<{ table: string; message: string }>; selfRepairing: number; repair: boolean }>(null);
    const isMaria = db.type === 'mysql';
    // The panel's own address is the server the administrator signs in to.
    const sshHost = typeof window === 'undefined' ? '' : window.location.hostname;

    const refusedBy = async (res: Response, sentence: TranslationKey) => onRefusal(await readApiError(res), sentence);
    const lost = () => showToast('error', t('common.resultUnknown'));

    const runExport = async () => {
        if (busy) return;
        setBusy(true);
        await followOperation({
            progressTitle: t('dbt.exportTitle', { name: db.name }),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'dbExport', ['check!', 'dump', 'ready'], { name: db.name }), t, `dbExport.${db.id}`,
                        (headers) => fetch(`${base}/export`, { method: 'POST', headers }));
                    if (!res.ok) return void (await refusedBy(res, 'dbt.exportFailed'));
                    const data = (await res.json()) as { export_id?: string; size?: number; filename?: string };
                    if (typeof data.export_id !== 'string') return void lost();
                    onClear();
                    const link = document.createElement('a');
                    link.href = `${base}/export?id=${encodeURIComponent(data.export_id)}`;
                    // The engine is in the name (alpha.129): two databases of
                    // different engines may share a name.
                    link.download = data.filename ?? `${db.name}-${isMaria ? 'mariadb' : 'postgresql'}.sql.gz`;
                    document.body.appendChild(link);
                    link.click();
                    link.remove();
                    showToast('success', t('dbt.exportReady', { name: db.name, size: formatSize(data.size ?? 0) }));
                } catch {
                    lost();
                }
            },
        });
        setBusy(false);
    };

    // Sends the file in 4 MiB chunks; a chunk whose answer was lost is sent
    // again and the server does not write it twice. Answers the upload's
    // identity, or null after a refusal (already said).
    const upload = async (chosen: File, report: OperationReport, labels: ReturnType<typeof flowSteps>): Promise<string | null> => {
        const id = newRequestId();
        let offset = 0;
        const started = Date.now();
        while (offset < chosen.size) {
            report.steps([
                { id: 'upload', label: t('opstep.dbImport.upload'), state: 'running', since: started, note: t('dbt.uploadNote', { done: formatSize(offset), total: formatSize(chosen.size) }) },
                ...labels.map((label) => ({ id: label.id, label: label.label, state: 'pending' as const })),
            ]);
            const res = await fetch(`${base}/import/upload?id=${id}&offset=${offset}`, {
                method: 'POST', headers: { 'Content-Type': 'application/octet-stream' }, body: chosen.slice(offset, offset + UPLOAD_CHUNK),
            });
            // The size the server holds; an answer without one continues nothing.
            let held = -1;
            try {
                const body = (await res.clone().json()) as { size?: unknown };
                if (typeof body.size === 'number') held = body.size;
            } catch {
                held = -1;
            }
            if (res.ok && held > offset) {
                offset = held;
                continue;
            }
            if (res.status === 409 && held > offset && held <= chosen.size) {
                offset = held;
                continue;
            }
            await refusedBy(res, 'dbt.importFailed');
            return null;
        }
        return id;
    };

    const runImport = async () => {
        if (!file || busy || readOnly) return;
        if (file.size > UPLOAD_MAX) {
            showToast('error', t('dbt.tooBig'));
            return;
        }
        const chosen = file;
        await confirmAction({
            message: t(emptyFirst ? 'dbt.importConfirmEmpty' : 'dbt.importConfirm', { name: db.name, file: chosen.name }),
            confirmLabel: t('dbt.importStart'),
            tone: 'danger',
            progressTitle: t('dbt.importTitle', { name: db.name }),
            action: async (report) => {
                setBusy(true);
                try {
                    const labels = flowSteps(t, 'dbImport', ['check!', 'safety', 'load', 'finish'], { name: db.name });
                    const id = await upload(chosen, report, labels);
                    if (!id) return;
                    const res = await sendWithSteps(report, labels, t, `dbImport.${db.id}`, (headers) => fetch(`${base}/import`, {
                        method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
                        body: JSON.stringify({ upload_id: id, size: chosen.size, empty_first: emptyFirst }),
                    }));
                    void tools.retry();
                    invalidate('databases');
                    if (!res.ok) return void (await refusedBy(res, 'dbt.importFailed'));
                    const data = (await res.json()) as { safety_backup?: string };
                    onClear();
                    setPanel('');
                    setFile(null);
                    showToast('success', t('dbt.imported', { name: db.name, file: chosen.name, safety: data.safety_backup ?? '' }));
                } catch {
                    lost();
                    void tools.retry();
                } finally {
                    setBusy(false);
                }
            },
        });
    };

    const runCopy = async () => {
        const name = copyName.trim();
        if (!name || busy || readOnly || !passwordAcceptable(copyPassword, 'database_user')) return;
        setBusy(true);
        await followOperation({
            progressTitle: t('dbt.copyTitle', { name: db.name }),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'dbCopy', ['check!', 'create', 'copy', 'records'], { name: db.name, copy: name }), t, `dbCopy.${db.id}`,
                        (headers) => fetch(`${base}/copy`, {
                            method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ name, password: copyPassword }),
                        }));
                    invalidate('databases');
                    if (!res.ok) return void (await refusedBy(res, 'dbt.copyFailed'));
                    const data = (await res.json()) as { name?: string; user?: string };
                    onClear();
                    setPanel('');
                    setCopyName('');
                    setCopyPassword('');
                    showToast('success', t('dbt.copied', { copy: data.name ?? name, user: data.user ?? '' }));
                } catch {
                    lost();
                    invalidate('databases');
                }
            },
        });
        setBusy(false);
    };

    const runCheck = async (repair: boolean) => {
        if (busy || readOnly) return;
        const step = repair ? 'repair_tables' : 'check_tables';
        const run = async (report: OperationReport) => {
            setBusy(true);
            try {
                const res = await sendWithSteps(report, flowSteps(t, 'dbCheck', ['check!', step], { name: db.name }), t, `dbCheck.${db.id}`,
                    (headers) => fetch(`${base}/check`, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ repair }) }));
                if (!res.ok) return void (await refusedBy(res, 'dbt.checkFailed'));
                const data = (await res.json()) as { checked?: number; problems?: Array<{ table: string; message: string }>; self_repairing?: number };
                onClear();
                setCheck({ checked: data.checked ?? 0, problems: Array.isArray(data.problems) ? data.problems : [], selfRepairing: data.self_repairing ?? 0, repair });
                void tools.retry();
            } catch {
                lost();
            } finally {
                setBusy(false);
            }
        };
        const title = t(repair ? 'dbt.repairTitle' : 'dbt.checkTitle', { name: db.name });
        if (repair) await confirmAction({ message: title + '?', confirmLabel: t('dbt.repair'), progressTitle: title, action: run });
        else await followOperation({ progressTitle: title, action: run });
    };

    const blockerText = info?.importBlocker === 'client_unsafe' ? t('dbt.importBlockedClient') : info?.importBlocker === 'no_user' ? t('dbt.importBlockedUser') : '';

    return (
        <div className="mt-3 space-y-3 border-t border-border pt-3 text-sm">
            {/* alpha.129: size, tables and open connections as three small
                boxes; while they are read or could not be read, one line
                says so. */}
            {info && info.known ? (
                <dl className="grid grid-cols-3 gap-2" data-database-stats={db.id}>
                    {[
                        { label: t('dbt.size'), value: formatSize(info.sizeBytes) },
                        { label: t('dbt.tables'), value: String(info.tables) },
                        { label: t('dbt.connections'), value: String(info.connections) },
                    ].map((stat) => (
                        <div key={stat.label} className="min-w-0 rounded-lg border border-border bg-surface-2/50 px-3 py-2">
                            <dt className="truncate text-xs text-fg-muted">{stat.label}</dt>
                            <dd className="mt-0.5 text-base font-semibold tabular-nums text-fg">{stat.value}</dd>
                        </div>
                    ))}
                </dl>
            ) : (
                <p className="text-fg-muted">{tools.remote.state === 'loading' ? t('dbt.reading') : t('dbt.unread')}</p>
            )}
            {afterStats}
            {info && (
                <details className="text-xs text-fg-muted" data-database-connection-guide={db.id}>
                    <summary className="cursor-pointer">{t('dbt.connection')}</summary>
                    {/* alpha.129: the site's connection, then the way from
                        the person's own computer. Databases listen on this
                        server only; the administrator's route is an SSH
                        tunnel, anyone else uses the web tool in the panel. */}
                    <p className="mt-2 font-semibold text-fg">{t('dbt.fromSite')}</p>
                    <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
                        <dt>{t('dbt.host')}</dt><dd className="font-mono text-fg">{info.host}</dd>
                        <dt>{t('dbt.port')}</dt><dd className="font-mono text-fg">{info.port}</dd>
                        <dt>{t('dbt.database')}</dt><dd className="break-all font-mono text-fg">{db.name}</dd>
                        <dt>{t('dbt.user')}</dt><dd className="break-all font-mono text-fg">{info.user}</dd>
                    </dl>
                    <p className="mt-1">{t('dbt.passwordNote')}</p>
                    <p className="mt-3 font-semibold text-fg">{t('dbt.fromComputer')}</p>
                    {isAdmin ? (
                        <>
                            <p className="mt-1">{t('dbt.tunnelIntro')}</p>
                            <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
                                <dt>{t('dbt.sshHost')}</dt><dd className="break-all font-mono text-fg">{sshHost}</dd>
                                <dt>{t('dbt.sshPort')}</dt><dd className="font-mono text-fg">22</dd>
                                <dt>{t('dbt.sshUser')}</dt><dd className="text-fg">{t('dbt.sshUserValue')}</dd>
                                <dt>{t('dbt.host')}</dt><dd className="font-mono text-fg">127.0.0.1</dd>
                                <dt>{t('dbt.port')}</dt><dd className="font-mono text-fg">{info.port}</dd>
                                <dt>{t('dbt.database')}</dt><dd className="break-all font-mono text-fg">{db.name}</dd>
                            </dl>
                            <p className="mt-1">{t(isMaria ? 'dbt.tunnelToolsMaria' : 'dbt.tunnelToolsPg')}</p>
                        </>
                    ) : (
                        <p className="mt-1">
                            {t(webTool === 'installed' ? 'dbt.noRemoteUseTool' : 'dbt.noRemoteAskTool', { tool: isMaria ? 'phpMyAdmin' : 'phpPgAdmin' })}
                        </p>
                    )}
                </details>
            )}
            {!readOnly && (
                <div className="flex flex-wrap gap-2">
                    <Button icon={Download} disabled={busy} onClick={() => void runExport()}>{t('dbt.export')}</Button>
                    <Button icon={Upload} disabled={busy || !!blockerText} onClick={() => setPanel(panel === 'import' ? '' : 'import')}>{t('dbt.import')}</Button>
                    <Button icon={CopyPlus} disabled={busy} onClick={() => setPanel(panel === 'copy' ? '' : 'copy')}>{t('dbt.copy')}</Button>
                    <Button icon={ShieldCheck} disabled={busy} onClick={() => void runCheck(false)}>{t('dbt.check')}</Button>
                </div>
            )}
            {!readOnly && blockerText && <p className="text-xs text-fg-muted">{blockerText}</p>}
            {panel === 'import' && !blockerText && (
                <div className="space-y-2 rounded-lg border border-border bg-surface-2/50 p-3">
                    <p className="font-semibold text-fg">{t('dbt.importHeading', { name: db.name })}</p>
                    <p className="text-xs text-fg-muted">{t('dbt.importHint')}</p>
                    <label className="block text-xs text-fg-muted">
                        {t('dbt.importFile')}
                        <input type="file" accept=".sql,.gz,application/sql,application/gzip" className="mt-1 block w-full text-sm text-fg"
                            onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
                    </label>
                    <label className="flex items-start gap-2 text-sm text-fg">
                        <input type="checkbox" className="mt-1" checked={emptyFirst} onChange={(e) => setEmptyFirst(e.target.checked)} />
                        <span>{t('dbt.importEmpty')}{!emptyFirst && <span className="block text-xs text-fg-muted">{t('dbt.importEmptyHint')}</span>}</span>
                    </label>
                    {/* alpha.129: without a file the button is an ordinary
                        unavailable button with the reason beside it, not the
                        dashed outline. */}
                    {file ? (
                        <Button variant="primary" icon={Upload} disabled={busy} onClick={() => void runImport()}>{t('dbt.importStart')}</Button>
                    ) : (
                        <div className="flex flex-wrap items-center gap-2">
                            <button type="button" disabled aria-describedby={`import-hint-${db.id}`}
                                className="inline-flex cursor-not-allowed items-center gap-1.5 rounded-md border border-border bg-surface-2 px-3 py-1.5 text-sm font-medium text-fg-muted">
                                <Upload className="h-4 w-4" aria-hidden="true" />
                                {t('dbt.importStart')}
                            </button>
                            <span id={`import-hint-${db.id}`} className="text-xs text-fg-muted">{t('dbt.importChooseFirst')}</span>
                        </div>
                    )}
                </div>
            )}
            {panel === 'copy' && (
                <div className="space-y-2 rounded-lg border border-border bg-surface-2/50 p-3">
                    <p className="font-semibold text-fg">{t('dbt.copyHeading', { name: db.name })}</p>
                    <p className="text-xs text-fg-muted">{t('dbt.copyHint')}</p>
                    <label className="block text-xs text-fg-muted">
                        {t('dbt.copyName')}
                        <input type="text" value={copyName} onChange={(e) => setCopyName(e.target.value)} placeholder="test" className={`${inputClass} mt-1`} />
                    </label>
                    <PasswordInput field="database_user" value={copyPassword} onChange={setCopyPassword} maxLength={128} />
                    <Button variant="primary" icon={CopyPlus} disabled={busy || !copyName.trim() || !passwordAcceptable(copyPassword, 'database_user')} onClick={() => void runCopy()}>
                        {t('dbt.copyStart')}
                    </Button>
                </div>
            )}
            {check && (
                <div className="space-y-1 rounded-lg border border-border bg-surface-2/50 p-3 text-sm text-fg">
                    {!isMaria ? (
                        <p>{t('dbt.pgCheckOK', { n: check.checked })}</p>
                    ) : check.problems.length === 0 ? (
                        <p>{t('dbt.checkedOK', { n: check.checked })}</p>
                    ) : (
                        <>
                            <p>{t('dbt.checkedProblems', { n: check.checked, p: check.problems.length })}</p>
                            <ul className="font-mono text-xs">
                                {check.problems.map((problem) => <li key={problem.table + problem.message}>{problem.table}: {problem.message}</li>)}
                            </ul>
                        </>
                    )}
                    {isMaria && check.selfRepairing > 0 && <p className="text-xs text-fg-muted">{t('dbt.selfRepairing', { n: check.selfRepairing })}</p>}
                    <div className="flex gap-2 pt-1">
                        {isMaria && info?.canRepair && check.problems.length > 0 && !check.repair && !readOnly && (
                            <Button variant="primary" icon={Wrench} disabled={busy} onClick={() => void runCheck(true)}>{t('dbt.repair')}</Button>
                        )}
                        <Button onClick={() => setCheck(null)}>{t('dbt.close')}</Button>
                    </div>
                </div>
            )}
        </div>
    );
}
