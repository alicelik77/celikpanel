import type { TranslationKey } from '../i18n/en';

// What deleting a database does to its users, in words, for both places a
// database is deleted from (the Databases page and the domain's Databases
// section; alpha.133 round 5). The server decides each user's fate the way
// the deletion will (cmd/panel/database_delete.go) and says it per user:
// absent = removed with the database, "shared" = another database uses the
// same account, "account" = an engine's or CelikPanel's own account.
//
// Bir veritabanı silinince kullanıcılarına ne olacağı; iki silme yerinde de
// aynı sözler.

export type DatabaseUserStays = 'shared' | 'account';

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** The question's lines: which users go with the database, which stay and why. */
export function databaseDeleteUserLines(t: T, users: ReadonlyArray<{ username: string; stays?: DatabaseUserStays }>): { lost: string[]; kept: string[] } {
    const names = (stays: DatabaseUserStays | undefined) => users.filter((user) => user.stays === stays).map((user) => user.username).join(', ');
    const removed = names(undefined);
    const shared = names('shared');
    const account = names('account');
    return {
        lost: removed ? [t('databases.delete.lostUsers', { users: removed })] : [],
        kept: [
            ...(shared ? [t('databases.delete.keptShared', { users: shared })] : []),
            ...(account ? [t('databases.delete.keptAccount', { users: account })] : []),
        ],
    };
}

/**
 * What a finished deletion adds, when the server kept accounts on the engine
 * because their owner gave them rights of their own; null otherwise. The
 * deletion has succeeded (res.ok): a body that cannot be read only loses this
 * note, never the success.
 */
export async function databaseDeletedKeptWords(t: T, name: string, res: Response): Promise<string | null> {
    const answer: unknown = await res.json().then((value: unknown) => value, () => null);
    const kept = answer && typeof answer === 'object' ? (answer as { users_kept_on_engine?: unknown }).users_kept_on_engine : undefined;
    const users = Array.isArray(kept) ? kept.filter((item): item is string => typeof item === 'string' && item !== '') : [];
    return users.length > 0 ? t('databases.dbDeletedKept', { name, users: users.join(', ') }) : null;
}
