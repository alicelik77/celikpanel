import { useState } from 'react';
import { KeyRound, UserRound } from 'lucide-react';
import { useDataToolsWords } from './dataToolsWords';
import { invalidate } from '../lib/remote';
import type { DatabaseUserInfo } from '../lib/domainDatabases';
import { DatabaseUserPassword } from './DatabaseUserPassword';

// The users of one domain database (alpha.129): each with its access (made
// with the database, read only, or read and write) and a new password.
//
// Bir alan adı veritabanının kullanıcıları: erişimi ve yeni parola.

export function DatabaseUsers({ domainId, databaseId, users, readOnly }: {
    domainId: number;
    databaseId: number;
    users: DatabaseUserInfo[];
    readOnly: boolean;
}) {
    const { t } = useDataToolsWords();
    const [passwordFor, setPasswordFor] = useState<DatabaseUserInfo | null>(null);
    const base = `/api/v1/domains/${domainId}/databases/${databaseId}/users`;
    const accessLabel = (access: DatabaseUserInfo['access']) => t(access === 'read' ? 'dbu.accessRead' : access === 'write' ? 'dbu.accessWrite' : 'dbu.accessOwner');

    return (
        <div className="space-y-2">
            <p className="text-xs font-semibold text-fg-muted">{t('dbu.users')}</p>
            {users.length === 0 ? (
                <p className="text-xs text-fg-subtle">{t('dbu.noUsers')}</p>
            ) : (
                <ul className="divide-y divide-border rounded-lg border border-border">
                    {users.map((user) => (
                        <li key={user.id} className="flex flex-wrap items-center gap-2 px-3 py-2" data-database-user={user.username}>
                            <UserRound className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />
                            <span className="min-w-0 break-all font-mono text-sm text-fg">{user.username}</span>
                            <span className="rounded bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">{accessLabel(user.access)}</span>
                            {!readOnly && (
                                <button
                                    type="button"
                                    onClick={() => setPasswordFor(passwordFor?.id === user.id ? null : user)}
                                    title={t('dbu.newPasswordFor', { user: user.username })}
                                    aria-label={t('dbu.newPasswordFor', { user: user.username })}
                                    className="ml-auto inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
                                >
                                    <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                                    {t('dbu.newPassword')}
                                </button>
                            )}
                        </li>
                    ))}
                </ul>
            )}
            {passwordFor && !readOnly && (
                <DatabaseUserPassword key={passwordFor.id} url={`${base}/${passwordFor.id}/password`} user={passwordFor.username}
                    onClose={() => setPasswordFor(null)} onSettled={() => invalidate('databases')} />
            )}
        </div>
    );
}
