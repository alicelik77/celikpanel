// The databases the signed-in person may see (alpha.133, the one visibility
// rule): an administrator every database on the server, a reseller their own
// customers', a customer and a team member only their own. One address, one
// decoder, and the two addresses the page's row actions send to: if the
// server's answer or routes change, this is the one file that follows.
//
// The answer carries names, owners, domains, engines and sizes only. A
// database's contents and any password or connection secret are never part
// of it; an answer that carries a field named like a secret is not the
// contract and is refused here (the page then says the list could not be
// read) instead of being drawn.
//
// Kişinin görebildiği veritabanları: yönetici hepsini, bayi kendi
// müşterilerininkini, müşteri ve ekip üyesi yalnız kendisininkini. Tek adres,
// tek çözücü ve satır işlemlerinin iki adresi burada. Gizli bilgiye benzeyen
// bir alan taşıyan yanıt sözleşme değildir; çizilmez, reddedilir.

export const VISIBLE_DATABASES_URL = '/api/v1/databases/visible';

/** Deletes one database (DELETE). */
export const databaseDeleteURL = (databaseId: number) => `/api/v1/databases/${databaseId}`;

/** Gives one database user a new password (POST {password}). */
export const databaseUserPasswordURL = (userId: number) => `/api/v1/database-users/${userId}/password`;

export interface VisibleOwner {
    id: number;
    username: string;
    /** 'admin' | 'reseller' | 'customer'; another value is kept and shown neutrally. */
    role: string;
    /** The reseller's username when the owner is a reseller's customer. */
    parent: string | null;
    /** The owner is the person reading the page. */
    self: boolean;
}

export interface VisibleDatabase {
    id: number;
    name: string;
    /** 'mariadb' | 'postgresql'; another value is kept and shown neutrally. */
    engine: string;
    owner: VisibleOwner;
    domain: { id: number; name: string } | null;
    /** null: not known right now (never "0 B"); always null in alpha.133. */
    sizeBytes: number | null;
    /** null: the record carries no creation time. */
    createdAt: string | null;
    users: Array<{ id: number; username: string }>;
}

export interface VisibleEngine {
    engine: string;
    installed: boolean;
    running: boolean;
}

export interface VisibleDatabases {
    databases: VisibleDatabase[];
    /** null: the server could not be asked right now (not "none installed"); empty for a team member. */
    engines: VisibleEngine[] | null;
    mayCreate: boolean;
}

// A field name that would hold a secret. The contract has none; one in the
// answer means the server sends more than this page may show.
const SECRET_FIELD = /pass(word|wd)?|secret|token|credential|private_?key|dsn|connection_?string/i;

function refuseSecrets(value: unknown, depth = 0): void {
    if (depth > 6 || value === null || typeof value !== 'object') return;
    if (Array.isArray(value)) {
        for (const item of value) refuseSecrets(item, depth + 1);
        return;
    }
    for (const [key, inner] of Object.entries(value as Record<string, unknown>)) {
        if (SECRET_FIELD.test(key)) throw new Error(`the answer carries a secret-looking field (${key})`);
        refuseSecrets(inner, depth + 1);
    }
}

const isId = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value > 0;
const isText = (value: unknown): value is string => typeof value === 'string' && value.length > 0;

function shape(what: string): never {
    throw new Error(`visible databases: ${what} is not the contract`);
}

function decodeOwner(raw: unknown): VisibleOwner {
    const owner = raw as Record<string, unknown> | null;
    if (!owner || typeof owner !== 'object' || !isId(owner.id) || !isText(owner.username) || typeof owner.role !== 'string' || typeof owner.self !== 'boolean') shape('owner');
    if (owner.parent !== null && owner.parent !== undefined && typeof owner.parent !== 'string') shape('owner.parent');
    return { id: owner.id, username: owner.username, role: owner.role, parent: isText(owner.parent) ? owner.parent : null, self: owner.self };
}

function decodeDatabase(raw: unknown): VisibleDatabase {
    const db = raw as Record<string, unknown> | null;
    if (!db || typeof db !== 'object' || !isId(db.id) || !isText(db.name) || !isText(db.engine) || typeof db.created_at !== 'string') shape('database');
    let domain: VisibleDatabase['domain'] = null;
    if (db.domain !== null && db.domain !== undefined) {
        const d = db.domain as Record<string, unknown>;
        if (typeof d !== 'object' || !isId(d.id) || !isText(d.name)) shape('database.domain');
        domain = { id: d.id, name: d.name };
    }
    if (db.size_bytes !== null && db.size_bytes !== undefined && (typeof db.size_bytes !== 'number' || db.size_bytes < 0)) shape('database.size_bytes');
    if (!Array.isArray(db.users)) shape('database.users');
    const users = db.users.map((item) => {
        const user = item as Record<string, unknown> | null;
        if (!user || typeof user !== 'object' || !isId(user.id) || !isText(user.username)) shape('database.users[]');
        return { id: user.id, username: user.username };
    });
    return {
        id: db.id,
        name: db.name,
        engine: db.engine,
        owner: decodeOwner(db.owner),
        domain,
        sizeBytes: typeof db.size_bytes === 'number' ? db.size_bytes : null,
        createdAt: db.created_at === '' ? null : db.created_at,
        users,
    };
}

/** The decoder of VISIBLE_DATABASES_URL; throws (the read is then "unknown") for anything else. */
export function decodeVisibleDatabases(raw: unknown): VisibleDatabases {
    refuseSecrets(raw);
    const body = raw as Record<string, unknown> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.databases) || (body.engines !== null && !Array.isArray(body.engines)) || typeof body.may_create !== 'boolean') shape('answer');
    const engines = body.engines === null ? null : body.engines.map((item) => {
        const engine = item as Record<string, unknown> | null;
        if (!engine || typeof engine !== 'object' || !isText(engine.engine) || typeof engine.installed !== 'boolean' || typeof engine.running !== 'boolean') shape('engine');
        return { engine: engine.engine, installed: engine.installed, running: engine.running };
    });
    return { databases: body.databases.map(decodeDatabase), engines, mayCreate: body.may_create };
}
