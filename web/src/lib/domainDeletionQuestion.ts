import type { ConfirmRequest } from './confirmAction';
import type { TranslationKey } from '../i18n/en';
import { decodeDomainDatabases, type DatabaseInfo } from './domainDatabases';
import { databaseDeleteDescribable, databaseDeleteUserLines, databaseKeptNotes } from './databaseDeleteWords';
import { readApiError, refusalText, type ApiError } from './apiError';

// The delete question of a domain and the words of its result (alpha.134).
//
// The question lists truthfully what goes and what stays and offers one
// choice for the domain's databases: deleting them or keeping them. Kept is
// the default: kept databases stay in the panel, on the Databases page,
// bound to no domain. Deleting them is the Databases page's own deletion
// (cmd/panel/database_delete.go, through the domain deletion's database
// step), so the question says per database which users go and which stay
// with the same lines as that page (lib/databaseDeleteWords.ts), and offers
// no deletion it cannot describe. A retry of a deletion that stopped repeats
// the choice accepted when it started and says it; it offers no choice.
// The server answers what stayed: the kept databases, where the stored mail
// was moved aside and the kept backups, which the result names with what can
// be done with them later; and a database deletion that did not finish, said
// with the Databases page's own sentence.
//
// Alan adı silme sorusu ve sonucu: veritabanları bir seçimdir (varsayılan
// tutmak); silinirse Veritabanları sayfasının silmesiyle ve sözleriyle.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** The domain's databases read before the question (with their users' fates), or null when not known. */
export type DomainDatabasesRead = DatabaseInfo[] | null;

/** Reads the domain's databases for the question; null when they could not be read. */
export async function readDomainDatabasesForDeletion(base: string, domainId: number): Promise<DomainDatabasesRead> {
    try {
        const response = await fetch(`${base}/domains/${domainId}/databases`);
        if (response.status !== 200) return null;
        const { databases } = decodeDomainDatabases(await response.json());
        if (databases.some((item) => !item || typeof item.name !== 'string' || item.name === '')) return null;
        return [...databases].sort((a, b) => a.name.localeCompare(b.name));
    } catch {
        return null;
    }
}

const baseLists = (t: Translate) => ({
    lost: [t('domains.delete.lost.site'), t('domains.delete.lost.mail'), t('domains.delete.lost.dns'), t('domains.delete.lost.schedule')],
    kept: [t('domains.delete.kept.backups'), t('domains.delete.kept.mail'), t('domains.delete.kept.dns')],
});

/**
 * The question's lists and its one choice. `databases`: the domain's
 * databases, [] when it has none, null when they could not be read (they are
 * then kept: nothing is deleted that the question did not name). `accepted`:
 * the choice of a deletion that was started earlier and stopped; its retry
 * repeats it and offers none.
 */
export function domainDeleteQuestion(t: Translate, databases: DomainDatabasesRead, accepted?: 'keep' | 'delete'): Pick<ConfirmRequest, 'message' | 'lost' | 'kept' | 'option'> {
    const { lost, kept } = baseLists(t);
    if (accepted) {
        return { message: `${t('domains.deleteWhat')} ${t(accepted === 'delete' ? 'domains.delete.retryDelete' : 'domains.delete.retryKeep')}`, lost, kept };
    }
    if (databases === null) {
        return { message: t('domains.deleteWhat'), lost, kept: [t('domains.delete.kept.databasesUnread'), ...kept] };
    }
    if (databases.length === 0) {
        return { message: t('domains.deleteWhat'), lost, kept };
    }
    // The Databases page offers no deletion whose users it cannot describe;
    // neither does this question.
    if (!databases.every((item) => databaseDeleteDescribable(item.users ?? []))) {
        return { message: t('domains.deleteWhat'), lost, kept: [t('domains.delete.kept.databasesUsersUnknown', { n: databases.length }), ...kept] };
    }
    const optionLost: string[] = [];
    const optionKept: string[] = [];
    for (const item of databases) {
        const users = databaseDeleteUserLines(t, item.name, item.users ?? []);
        optionLost.push(t('databases.delete.lostData', { name: item.name }), ...users.lost);
        optionKept.push(...users.kept);
    }
    return {
        message: `${t('domains.deleteWhat')} ${t('domains.delete.databasesStay', { n: databases.length })}`,
        lost,
        kept,
        option: {
            label: t('domains.delete.databasesOption', { n: databases.length }),
            lost: optionLost,
            ...(optionKept.length > 0 ? { kept: optionKept } : {}),
        },
    };
}

/** The databases choice a bulk deletion takes: kept, said once. */
export function domainBulkDeleteLists(t: Translate): Pick<ConfirmRequest, 'lost' | 'kept'> {
    const { lost, kept } = baseLists(t);
    return { lost, kept: [t('domains.delete.kept.databasesUnbound'), ...kept] };
}

/** A database a deletion deleted, with its deletion's answer fields. */
export interface DomainDeletedDatabase {
    name: string;
    answer: Record<string, unknown>;
}

/** What a deletion's answer says stayed, which databases it deleted, and a database deletion that did not finish. */
export interface DomainDeletionKept {
    choice: 'keep' | 'delete' | null;
    databases: string[];
    deleted: DomainDeletedDatabase[];
    databaseFailure: ApiError | null;
    mail: { path: string; bytes: number } | null;
    backups: { path: string; files: number; bytes: number; unknown: boolean } | null;
}

function records(value: unknown): Record<string, unknown>[] {
    return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object' && !Array.isArray(item) &&
        typeof (item as Record<string, unknown>).name === 'string' && (item as Record<string, unknown>).name !== '') : [];
}

/** Reads `kept` of a deletion answer (200 or 202) without using up its body; null when there is none. */
export async function readDomainDeletionKept(response: Response): Promise<DomainDeletionKept | null> {
    try {
        const body: unknown = await response.clone().json();
        const kept = body && typeof body === 'object' && !Array.isArray(body) ? (body as Record<string, unknown>).kept : undefined;
        if (!kept || typeof kept !== 'object' || Array.isArray(kept)) return null;
        const value = kept as Record<string, unknown>;
        const mailValue = value.mail as Record<string, unknown> | undefined;
        const backupValue = value.backups as Record<string, unknown> | undefined;
        const failureValue = value.database_failure as Record<string, unknown> | undefined;
        const mail = mailValue && typeof mailValue.path === 'string' && mailValue.path.startsWith('/')
            ? { path: mailValue.path, bytes: typeof mailValue.bytes === 'number' ? mailValue.bytes : -1 }
            : null;
        const backups = backupValue && typeof backupValue.path === 'string' && backupValue.path.startsWith('/')
            ? {
                path: backupValue.path,
                files: typeof backupValue.files === 'number' ? backupValue.files : 0,
                bytes: typeof backupValue.bytes === 'number' ? backupValue.bytes : 0,
                unknown: backupValue.unknown === true,
            }
            : null;
        // The Databases page's own answer for the database whose deletion did
        // not finish, read the one way an error answer is read.
        const databaseFailure = failureValue && failureValue.answer && typeof failureValue.answer === 'object'
            ? await readApiError(new Response(JSON.stringify(failureValue.answer), {
                status: typeof failureValue.status === 'number' && failureValue.status >= 200 && failureValue.status < 600 ? failureValue.status : 500,
            }))
            : null;
        const result: DomainDeletionKept = {
            choice: value.choice === 'keep' || value.choice === 'delete' ? value.choice : null,
            databases: records(value.databases).map((item) => item.name as string),
            deleted: records(value.deleted_databases).map((item) => ({ name: item.name as string, answer: item })),
            databaseFailure, mail, backups,
        };
        return result.choice || result.databases.length || result.deleted.length || databaseFailure || mail || backups ? result : null;
    } catch {
        return null;
    }
}

/** The sentence of a database deletion that did not finish inside a domain deletion: the Databases page's own. */
export function domainDeletionDatabaseFailureText(t: Translate, kept: DomainDeletionKept | null): string {
    return kept?.databaseFailure ? refusalText(kept.databaseFailure, t, 'ddb.deleteFailed') : '';
}

/**
 * The result's sentences about what stayed, each with what can be done with
 * it later. Folders are named to the administrator only (a customer is told
 * whom to ask).
 */
export function domainDeletionKeptSentences(t: Translate, kept: DomainDeletionKept | null, admin: boolean, size: (bytes: number) => string): string[] {
    if (!kept) return [];
    const out: string[] = [];
    if (kept.deleted.length) {
        out.push(t('domains.deleted.databasesDeleted', { names: kept.deleted.map((item) => item.name).join(', ') }));
        for (const item of kept.deleted) out.push(...databaseKeptNotes(t, item.name, item.answer));
    }
    if (kept.databases.length) out.push(t('domains.deleted.databasesKept', { names: kept.databases.join(', ') }));
    if (kept.mail) {
        const amount = kept.mail.bytes >= 0 ? size(kept.mail.bytes) : t('domains.deleted.sizeUnknown');
        out.push(admin
            ? t('domains.deleted.mailKeptAdmin', { size: amount, path: kept.mail.path })
            : t('domains.deleted.mailKept', { size: amount }));
    }
    if (kept.backups) {
        const amount = kept.backups.unknown ? t('domains.deleted.sizeUnknown') : size(kept.backups.bytes);
        out.push(admin
            ? t('domains.deleted.backupsKeptAdmin', { n: kept.backups.files, size: amount, path: kept.backups.path })
            : t('domains.deleted.backupsKept', { n: kept.backups.files, size: amount }));
    }
    return out;
}
