import type { ConfirmRequest } from './confirmAction';
import type { TranslationKey } from '../i18n/en';

// The delete question of a domain and the words of its result (alpha.134).
//
// The question lists truthfully what goes and what stays and offers one
// choice for the domain's databases: deleting them (with the users that have
// rights on no other database) or keeping them. Kept is the default: kept
// databases stay in the panel, on the Databases page, bound to no domain.
// The server carries out exactly that choice (cmd/panel/domain_delete_cleanup.go)
// and answers what stayed: the unbound databases, where the stored mail was
// moved aside and the kept backups, which the result names with what can be
// done with them later.
//
// Alan adı silme sorusu ve sonucu: veritabanları bir seçimdir (varsayılan
// tutmak); kalanlar sonuçta yeri ve boyutuyla söylenir.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** One database of the domain, as its Databases section lists it. */
export interface DomainDatabaseRef {
    id: number;
    name: string;
}

/** What the domain's databases read before the question: a list, or not known. */
export type DomainDatabasesRead = DomainDatabaseRef[] | null;

/** Reads the domain's databases for the question; null when they could not be read. */
export async function readDomainDatabasesForDeletion(base: string, domainId: number): Promise<DomainDatabasesRead> {
    try {
        const response = await fetch(`${base}/domains/${domainId}/databases`);
        if (response.status !== 200) return null;
        const body: unknown = await response.json();
        const list = body && typeof body === 'object' && !Array.isArray(body) ? (body as Record<string, unknown>).databases : undefined;
        if (!Array.isArray(list)) return null;
        const out: DomainDatabaseRef[] = [];
        for (const item of list) {
            if (!item || typeof item !== 'object') return null;
            const { id, name } = item as Record<string, unknown>;
            if (typeof id !== 'number' || typeof name !== 'string' || name === '') return null;
            out.push({ id, name });
        }
        return out.sort((a, b) => a.name.localeCompare(b.name));
    } catch {
        return null;
    }
}

/**
 * The question's lists and its one choice. `databases`: the domain's
 * databases, [] when it has none, null when they could not be read (they are
 * then kept: nothing is deleted that the question did not name).
 */
export function domainDeleteQuestion(t: Translate, databases: DomainDatabasesRead): Pick<ConfirmRequest, 'message' | 'lost' | 'kept' | 'option'> {
    const lost = [t('domains.delete.lost.site'), t('domains.delete.lost.mail'), t('domains.delete.lost.dns'), t('domains.delete.lost.schedule')];
    const kept = [t('domains.delete.kept.backups'), t('domains.delete.kept.mail'), t('domains.delete.kept.dns')];
    if (databases === null) {
        return { message: t('domains.deleteWhat'), lost, kept: [t('domains.delete.kept.databasesUnread'), ...kept] };
    }
    if (databases.length === 0) {
        return { message: t('domains.deleteWhat'), lost, kept };
    }
    const names = databases.map((item) => item.name).join(', ');
    return {
        message: `${t('domains.deleteWhat')} ${t('domains.delete.databasesStay', { n: databases.length })}`,
        lost,
        kept,
        option: {
            label: t('domains.delete.databasesOption', { n: databases.length }),
            lost: [t('domains.delete.lost.databases', { names })],
        },
    };
}

/** The databases choice a bulk deletion takes: kept, said once. */
export function domainBulkDeleteLists(t: Translate): Pick<ConfirmRequest, 'lost' | 'kept'> {
    return {
        lost: [t('domains.delete.lost.site'), t('domains.delete.lost.mail'), t('domains.delete.lost.dns'), t('domains.delete.lost.schedule')],
        kept: [t('domains.delete.kept.databasesUnbound'), t('domains.delete.kept.backups'), t('domains.delete.kept.mail'), t('domains.delete.kept.dns')],
    };
}

/** What a deletion's answer says stayed, and which databases it deleted. */
export interface DomainDeletionKept {
    databases: string[];
    deleted: string[];
    mail: { path: string; bytes: number } | null;
    backups: { path: string; files: number; bytes: number; unknown: boolean } | null;
}

function names(value: unknown): string[] {
    if (!Array.isArray(value)) return [];
    return value
        .map((item) => (item && typeof item === 'object' ? (item as Record<string, unknown>).name : undefined))
        .filter((name): name is string => typeof name === 'string' && name !== '');
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
        const result = { databases: names(value.databases), deleted: names(value.deleted_databases), mail, backups };
        return result.databases.length || result.deleted.length || mail || backups ? result : null;
    } catch {
        return null;
    }
}

/**
 * The result's sentences about what stayed, each with what can be done with
 * it later. Folders are named to the administrator only (a customer is told
 * whom to ask).
 */
export function domainDeletionKeptSentences(t: Translate, kept: DomainDeletionKept | null, admin: boolean, size: (bytes: number) => string): string[] {
    if (!kept) return [];
    const out: string[] = [];
    if (kept.deleted.length) out.push(t('domains.deleted.databasesDeleted', { names: kept.deleted.join(', ') }));
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
