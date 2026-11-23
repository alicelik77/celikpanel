import { decodeList } from './remote';
import type { DatabaseUserStays } from './databaseDeleteWords';

// The databases of one domain: GET /api/v1/domains/{id}/databases. One
// address, one decoder, for every screen that reads it (the Databases tab and
// the Backups tab of a domain).
// Bir alan adının veritabanları. Okuyan her ekran için (alan adının
// Veritabanları ve Yedekler sekmeleri) tek adres, tek çözücü.
export type DatabaseType = 'mysql' | 'postgresql';
export type DatabaseEngine = { value: DatabaseType; label: string };

/** One user of a database (alpha.129): the first is made with it ("owner"). */
export interface DatabaseUserInfo {
    id: number;
    username: string;
    access: 'owner' | 'read' | 'write';
    /** alpha.133 round 5: what deleting the database leaves of the user; absent = removed with it. */
    stays?: DatabaseUserStays;
}

export interface DatabaseInfo {
    id: number;
    name: string;
    type: string;
    user: string;
    created_at: string;
    /** alpha.129: the engine the server runs and its major version, when known. */
    engine?: 'mariadb' | 'mysql' | 'postgresql';
    engine_version?: string;
    users?: DatabaseUserInfo[];
    /** 20 Nov 2026: a PostgreSQL database's encoding as the server names it ("UTF8", "SQL_ASCII"…); absent when not known. */
    encoding?: string;
}

// 20 Nov 2026: what a PostgreSQL database's encoding means to a person.
// Only UTF8 holds text the server checks as Unicode; SQL_ASCII keeps bytes
// unchecked; any other encoding (LATIN1…) has no room for many letters.
// Bir PostgreSQL veritabanının kodlamasının kişi için anlamı.
export type DatabaseEncodingWarning = 'unchecked' | 'limited';

/** "UTF-8" for UTF8, else the server's own name. */
export function encodingLabel(encoding: string): string {
    return encoding.trim().toUpperCase() === 'UTF8' ? 'UTF-8' : encoding.trim();
}

/** The warning a database's card shows, or null: none for UTF8, for an unknown encoding or for another engine. */
export function databaseEncodingWarning(engine: DatabaseEngineName, encoding: string | undefined): DatabaseEncodingWarning | null {
    const name = (encoding ?? '').trim().toUpperCase();
    if (engine !== 'postgresql' || name === '' || name === 'UTF8') return null;
    return name === 'SQL_ASCII' ? 'unchecked' : 'limited';
}

export function parseAvailableDatabaseTypes(value: unknown): DatabaseEngine[] {
    if (!Array.isArray(value)) return [];

    const parsed: DatabaseEngine[] = [];
    const seen = new Set<DatabaseType>();
    for (const item of value) {
        if (item !== 'mysql' && item !== 'postgresql') return [];
        if (seen.has(item)) continue;
        seen.add(item);
        parsed.push({
            value: item,
            label: item === 'mysql' ? 'MariaDB' : 'PostgreSQL',
        });
    }
    return parsed;
}

// One answer carries this domain's databases and, for a team member, the
// engines that member may create on. A body that is not this shape is unknown.
// Tek yanıt bu alan adının veritabanlarını ve ekip üyesi için oluşturabileceği
// motorları taşır. Bu biçimde olmayan gövde bilinmeyendir.
export interface DomainDatabases {
    databases: DatabaseInfo[];
    availableTypes: DatabaseEngine[];
    /** alpha.134: what a new database's name begins with, as the server makes it ("k_1_dr_test_"). */
    namePrefix?: string;
}

export function decodeDomainDatabases(raw: unknown): DomainDatabases {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const payload = raw as Record<string, unknown>;
    return {
        databases: decodeList<DatabaseInfo>(payload.databases),
        availableTypes: parseAvailableDatabaseTypes(payload.available_types),
        ...(typeof payload.name_prefix === 'string' && payload.name_prefix ? { namePrefix: payload.name_prefix } : {}),
    };
}

// alpha.129: the engine a card names and marks (EngineMark).
// Kartın adlandırdığı ve işaretlediği motor.
export type DatabaseEngineName = 'mariadb' | 'mysql' | 'postgresql';

/** The engine of a database: the server's answer, else its API type. */
export function databaseEngine(engine: string | undefined, type: string): DatabaseEngineName {
    if (engine === 'mariadb' || engine === 'mysql' || engine === 'postgresql') return engine;
    return type === 'postgresql' ? 'postgresql' : 'mariadb';
}

const names: Record<DatabaseEngineName, string> = { mariadb: 'MariaDB', mysql: 'MySQL', postgresql: 'PostgreSQL' };

/** "MariaDB 11.8", "PostgreSQL 17", or the bare name. */
export function engineLabel(engine: DatabaseEngineName, version?: string): string {
    return version ? `${names[engine]} ${version}` : names[engine];
}
