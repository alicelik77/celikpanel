import type { TranslationKey } from '../i18n/en';

// What deleting a database does to its users, in words, for both places a
// database is deleted from (the Databases page and the domain's Databases
// section; alpha.133 round 5). The server decides each user's fate the way
// the deletion will (cmd/panel/database_delete.go) and says it per user:
// absent = removed with the database, "shared" = another database uses the
// same account, "account" = an engine's or CelikPanel's own account,
// "server" = the database server is reached with its own account and
// CelikPanel does not remove users there, "server_shared" = another database
// uses it on such a server (it stays there untouched), "unknown" = it could
// not be read
// (then no deletion is offered).
//
// Bir veritabanı silinince kullanıcılarına ne olacağı; iki silme yerinde de
// aynı sözler.

export type DatabaseUserStays = 'shared' | 'account' | 'server' | 'server_shared' | 'unknown';

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;
type User = { username: string; stays?: DatabaseUserStays };

/** Whether the question can say what happens to every user (otherwise no deletion is offered). */
export function databaseDeleteDescribable(users: ReadonlyArray<User>): boolean {
    return !users.some((user) => user.stays === 'unknown');
}

/** The question's lines: which users go with the database, which stay and why. */
export function databaseDeleteUserLines(t: T, name: string, users: ReadonlyArray<User>): { lost: string[]; kept: string[] } {
    const names = (stays: DatabaseUserStays | undefined) => users.filter((user) => user.stays === stays).map((user) => user.username).join(', ');
    const removed = names(undefined);
    const kept: string[] = [];
    for (const [stays, key] of [['shared', 'databases.delete.keptShared'], ['account', 'databases.delete.keptAccount'], ['server', 'databases.delete.keptServer'], ['server_shared', 'databases.delete.keptServerShared']] as const) {
        const list = names(stays);
        if (list) kept.push(t(key, { users: list, name }));
    }
    return { lost: removed ? [t('databases.delete.lostUsers', { users: removed })] : [], kept };
}

/**
 * What a finished deletion says when the server kept accounts on the engine:
 * for rights their owner gave by hand, because what else they have could not
 * be checked, or because the server is reached with its own account; null
 * when it kept none. The deletion has succeeded (res.ok): a body that cannot
 * be read only loses this note, never the success.
 */
export async function databaseDeletedKeptWords(t: T, name: string, res: Response): Promise<string | null> {
    const answer: unknown = await res.json().then((value: unknown) => value, () => null);
    const list = (field: string) => {
        const value = answer && typeof answer === 'object' ? (answer as Record<string, unknown>)[field] : undefined;
        return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string' && item !== '').join(', ') : '';
    };
    const field = (key: string): unknown => (answer && typeof answer === 'object' ? (answer as Record<string, unknown>)[key] : undefined);
    const notes: string[] = [];
    for (const [key, words] of [['users_kept_on_engine', 'databases.keptNote.otherRights'], ['users_kept_unchecked', 'databases.keptNote.unchecked'], ['users_kept_on_server', 'databases.keptNote.server']] as const) {
        const users = list(key);
        if (users) notes.push(t(words, { name, users }));
    }
    // alpha.134: rights naming the database were taken from other accounts
    // (an administrator gets their names, everybody else the count), or may
    // remain on a server CelikPanel reaches with an account that could not.
    const accounts = list('rights_removed_from');
    const count = field('rights_removed_count');
    if (accounts) notes.push(t('databases.keptNote.rightsFrom', { name, accounts }));
    else if (typeof count === 'number' && count > 0) notes.push(t('databases.keptNote.rightsCount', { name, count }));
    if (field('rights_may_remain') === true) notes.push(t('databases.keptNote.rightsMayRemain', { name }));
    return notes.length > 0 ? [t('databases.deletedNamed', { name }), ...notes].join(' ') : null;
}
