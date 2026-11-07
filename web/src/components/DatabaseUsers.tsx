import { useState } from 'react';
import { KeyRound, Plus, Trash2 } from 'lucide-react';
import { useDataToolsWords } from './dataToolsWords';
import { invalidate } from '../lib/remote';
import type { DatabaseUserInfo } from '../lib/domainDatabases';
import { readApiError, refusalText } from '../lib/apiError';
import { confirmAction, followOperation, type OperationEnd } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { Button, inputClass } from './ui';
import { PasswordInput } from './PasswordInput';
import { DatabaseUserPassword } from './DatabaseUserPassword';

// The users of one domain database (alpha.129): each with its access (made
// with the database, read only, or read and write) and a new password; more
// users can be added with read-only or read-and-write access and removed
// again, which never touches the database. The user made with the database
// goes only with it.
//
// Bir alan adı veritabanının kullanıcıları: erişimi, yeni parola, ek
// kullanıcı ekleme ve kaldırma (veritabanına dokunmaz).

const NAME = /^[a-z0-9_]{1,16}$/;

export function DatabaseUsers({ domainId, databaseId, databaseName, users, readOnly }: {
    domainId: number;
    databaseId: number;
    databaseName: string;
    users: DatabaseUserInfo[];
    readOnly: boolean;
}) {
    const { t } = useDataToolsWords();
    const [passwordFor, setPasswordFor] = useState<DatabaseUserInfo | null>(null);
    const [adding, setAdding] = useState(false);
    const [name, setName] = useState('');
    const [access, setAccess] = useState<'read' | 'write'>('read');
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState(false);
    const base = `/api/v1/domains/${domainId}/databases/${databaseId}/users`;
    const accessLabel = (value: DatabaseUserInfo['access']) => t(value === 'read' ? 'dbu.accessRead' : value === 'write' ? 'dbu.accessWrite' : 'dbu.accessOwner');
    const nameOK = NAME.test(name) && name !== 'user' && `${databaseName}_${name}`.length <= 63;
    const canAdd = nameOK && passwordAcceptable(password, 'database_user') && !busy;

    const add = async () => {
        if (!canAdd) return;
        setBusy(true);
        const user = `${databaseName}_${name}`;
        const body = JSON.stringify({ name, access, password });
        await followOperation({
            progressTitle: t('dbu.addTitle', { user }),
            action: async (report): Promise<OperationEnd> => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'dbUserAdd', ['check!', 'create', 'records'], { user }), t, `dbUserAdd.${databaseId}`,
                        (headers) => fetch(base, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body }));
                    invalidate('databases');
                    if (!res.ok) return { state: res.status < 500 ? 'blocked' : 'failed', reason: refusalText(await readApiError(res), t, 'dbu.addFailed') };
                    setAdding(false);
                    setName('');
                    setPassword('');
                    return { state: 'done', message: t(access === 'read' ? 'dbu.addedRead' : 'dbu.addedWrite', { user, database: databaseName }) };
                } catch {
                    invalidate('databases');
                    return { state: 'unknown', reason: t('dbu.addUnknown', { user }) };
                }
            },
        });
        setBusy(false);
    };

    const remove = async (user: DatabaseUserInfo) => {
        await confirmAction({
            message: t('dbu.removeConfirm', { user: user.username, database: databaseName }),
            confirmLabel: t('dbu.remove'),
            tone: 'danger',
            progressTitle: t('dbu.removeTitle', { user: user.username }),
            action: async (report): Promise<OperationEnd> => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'dbUserRemove', ['check!', 'delete', 'records'], { user: user.username }), t, `dbUserRemove.${user.id}`,
                        (headers) => fetch(`${base}/${user.id}`, { method: 'DELETE', headers }));
                    invalidate('databases');
                    if (!res.ok) return { state: res.status < 500 ? 'blocked' : 'failed', reason: refusalText(await readApiError(res), t, 'dbu.removeFailed') };
                    return { state: 'done', message: t('dbu.removed', { user: user.username, database: databaseName }) };
                } catch {
                    invalidate('databases');
                    return { state: 'unknown', reason: t('dbu.removeUnknown', { user: user.username }) };
                }
            },
        });
    };

    return (
        <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-xs font-semibold text-fg-muted">{t('dbu.users')}</p>
                {!readOnly && !adding && (
                    <button
                        type="button"
                        onClick={() => setAdding(true)}
                        className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-surface-2"
                    >
                        <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                        {t('dbu.add')}
                    </button>
                )}
            </div>
            {users.length === 0 ? (
                <p className="text-xs text-fg-subtle">{t('dbu.noUsers')}</p>
            ) : (
                <ul className="divide-y divide-border rounded-lg border border-border">
                    {users.map((user) => (
                        <li key={user.id} className="flex flex-wrap items-center gap-2 px-3 py-2" data-database-user={user.username}>
                            <span className="min-w-0 break-all font-mono text-sm text-fg">{user.username}</span>
                            <span className="rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">{accessLabel(user.access)}</span>
                            {!readOnly && (
                                <span className="ml-auto flex items-center gap-1">
                                    <button
                                        type="button"
                                        onClick={() => setPasswordFor(passwordFor?.id === user.id ? null : user)}
                                        title={t('dbu.newPasswordFor', { user: user.username })}
                                        aria-label={t('dbu.newPasswordFor', { user: user.username })}
                                        className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
                                    >
                                        <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                                        {t('dbu.newPassword')}
                                    </button>
                                    {user.access !== 'owner' && (
                                        <button
                                            type="button"
                                            onClick={() => void remove(user)}
                                            title={t('dbu.removeUser', { user: user.username })}
                                            aria-label={t('dbu.removeUser', { user: user.username })}
                                            className="rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-2 hover:text-danger"
                                        >
                                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                                        </button>
                                    )}
                                </span>
                            )}
                        </li>
                    ))}
                </ul>
            )}
            {passwordFor && !readOnly && (
                <DatabaseUserPassword key={passwordFor.id} url={`${base}/${passwordFor.id}/password`} user={passwordFor.username}
                    onClose={() => setPasswordFor(null)} onSettled={() => invalidate('databases')} />
            )}
            {adding && !readOnly && (
                <div className="space-y-2 rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-database-user-add={databaseName}>
                    <p className="font-semibold text-fg">{t('dbu.addHeading', { database: databaseName })}</p>
                    <label className="block text-xs text-fg-muted">
                        {t('dbu.name')}
                        <input type="text" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} placeholder="reader" maxLength={16}
                            className={`${inputClass} mt-1`} autoComplete="off" />
                        <span className="mt-1 block text-fg-subtle">
                            {t('dbu.fullName')} <span className="break-all font-mono text-fg">{databaseName}_{name || '…'}</span>. {t('dbu.nameHint')}
                        </span>
                    </label>
                    <fieldset className="space-y-1">
                        <legend className="text-xs text-fg-muted">{t('dbu.access')}</legend>
                        {(['read', 'write'] as const).map((value) => (
                            <label key={value} className="flex items-start gap-2 text-sm text-fg">
                                <input type="radio" name={`access-${databaseId}`} className="mt-1" checked={access === value} onChange={() => setAccess(value)} />
                                <span>
                                    {t(value === 'read' ? 'dbu.accessRead' : 'dbu.accessWrite')}
                                    <span className="block text-xs text-fg-muted">{t(value === 'read' ? 'dbu.accessReadHint' : 'dbu.accessWriteHint')}</span>
                                </span>
                            </label>
                        ))}
                    </fieldset>
                    <PasswordInput field="database_user" value={password} onChange={setPassword} maxLength={128} autoComplete="new-password" />
                    <div className="flex flex-wrap gap-2">
                        <Button variant="primary" icon={Plus} disabled={!canAdd} onClick={() => void add()}>{t('dbu.addStart')}</Button>
                        <Button disabled={busy} onClick={() => { setAdding(false); setName(''); setPassword(''); }}>{t('common.cancel')}</Button>
                    </div>
                </div>
            )}
        </div>
    );
}
