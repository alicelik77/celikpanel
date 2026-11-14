// The length rule of the names one database create makes, the browser half
// (alpha.134 fix round). The panel and the Agent check the same rule in
// internal/services/database_name_limits.go before anything is made; both
// halves are tested against internal/services/testdata/database-name-limits.json,
// so the field and the server never disagree about a name. Every name holds
// at most 63 characters (both engines); on MariaDB the database name's exact
// form also fits a right (each `_` counts twice, 64 in all).
//
// Bir oluşturmanın yaptığı adların uzunluk kuralı, tarayıcı yarısı; sunucu
// aynı kuralı denetler, iki yarı aynı test verisini okur.

const maxIdentifier = 63;
const maxExactForm = 64;

export interface NameOver {
    /** Characters the chosen name must lose at least; 0 when every name fits. */
    by: number;
    /** MariaDB's exact-name limit is exceeded most: fewer underscores help more. */
    underscores: boolean;
}

/** Whether the database (empty: none) and the users a create makes fit the engine ('mysql' is MariaDB). */
export function databaseNamesOver(engine: string, database: string, users: string[] = []): NameOver {
    let identifier = 0;
    for (const name of [database, ...users]) identifier = Math.max(identifier, name.length - maxIdentifier);
    let exact = 0;
    if (database !== '' && engine !== 'postgresql') {
        const underscores = database.split('_').length - 1;
        exact = Math.max(0, database.length + underscores - maxExactForm);
    }
    return exact > identifier ? { by: exact, underscores: true } : { by: identifier, underscores: false };
}

/** The part a person chooses holds only what the engine's names hold: letters, digits, `_`. */
export function databaseNameCharactersOK(chosen: string): boolean {
    return /^[A-Za-z0-9_]*$/.test(chosen);
}
