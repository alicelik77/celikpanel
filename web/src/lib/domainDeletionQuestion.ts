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
// The choice comes first and the sentence under it changes with it (alpha.135).
// The server answers what stayed: the kept databases, and the stored mail and
// backups, which go into the holding list for 30 days (alpha.135): the result
// says each in a short row with its size and date, folders only under the
// administrator's closed Details; and a database deletion that did not finish,
// said with the Databases page's own sentence.
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
        message: t('domains.deleteWhat'),
        lost,
        kept,
        option: {
            label: t('domains.delete.databasesOption', { n: databases.length }),
            // The choice before its consequences; the sentence under it says
            // what happens, ticked or not (alpha.135).
            first: true,
            note: { off: t('domains.delete.databasesStay', { n: databases.length }), on: t('domains.delete.databasesGo', { n: databases.length }) },
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

/** One item a deletion put into the holding list (alpha.135): kept until `removeAfter`, then removed. */
export interface DomainDeletionHeld {
    /** -1: the size could not be read. */
    bytes: number;
    files: number;
    removeAfter: string;
    /** Where it is: the administrator's answer only. */
    path: string;
}

/** What a deletion's answer says stayed, which databases it deleted, and a database deletion that did not finish. */
export interface DomainDeletionKept {
    choice: 'keep' | 'delete' | null;
    databases: string[];
    deleted: DomainDeletedDatabase[];
    databaseFailure: ApiError | null;
    mail: DomainDeletionHeld | null;
    backups: DomainDeletionHeld | null;
}

function records(value: unknown): Record<string, unknown>[] {
    return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object' && !Array.isArray(item) &&
        typeof (item as Record<string, unknown>).name === 'string' && (item as Record<string, unknown>).name !== '') : [];
}

function held(value: unknown): DomainDeletionHeld | null {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    const item = value as Record<string, unknown>;
    if (typeof item.remove_after !== 'string' || Number.isNaN(Date.parse(item.remove_after))) return null;
    return {
        bytes: typeof item.bytes === 'number' ? item.bytes : -1,
        files: typeof item.files === 'number' ? item.files : 0,
        removeAfter: item.remove_after,
        path: typeof item.path === 'string' && item.path.startsWith('/') ? item.path : '',
    };
}

/** Reads `kept` of a deletion answer (200 or 202) without using up its body; null when there is none. */
export async function readDomainDeletionKept(response: Response): Promise<DomainDeletionKept | null> {
    try {
        const body: unknown = await response.clone().json();
        const kept = body && typeof body === 'object' && !Array.isArray(body) ? (body as Record<string, unknown>).kept : undefined;
        if (!kept || typeof kept !== 'object' || Array.isArray(kept)) return null;
        const value = kept as Record<string, unknown>;
        const failureValue = value.database_failure as Record<string, unknown> | undefined;
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
            databaseFailure, mail: held(value.mail), backups: held(value.backups),
        };
        return result.choice || result.databases.length || result.deleted.length || databaseFailure || result.mail || result.backups ? result : null;
    } catch {
        return null;
    }
}

/** The sentence of a database deletion that did not finish inside a domain deletion: the Databases page's own. */
export function domainDeletionDatabaseFailureText(t: Translate, kept: DomainDeletionKept | null): string {
    return kept?.databaseFailure ? refusalText(kept.databaseFailure, t, 'ddb.deleteFailed') : '';
}

/** The finished databases step, said as what happened ("2 databases stayed" / "2 databases deleted"). */
export function domainDeletionDatabasesDone(t: Translate, kept: DomainDeletionKept | null): string {
    if (kept?.deleted.length) return t('domains.step.done.databasesDeleted', { n: kept.deleted.length });
    if (kept?.databases.length) return t('domains.step.done.databasesKept', { n: kept.databases.length });
    return t('domains.step.done.noDatabases');
}

/**
 * The end of a finished deletion (alpha.135): one short sentence of what was
 * deleted and the notes about kept database users; the rows of what stayed
 * under "Kept" (databases and where to find them; mail and backups with
 * their size and the date they are removed); and, for the administrator
 * only, where the held items are, under a closed "Details". No folder and no
 * server command in the visible text.
 */
export function domainDeletionEnd(t: Translate, kept: DomainDeletionKept | null, admin: boolean,
    size: (bytes: number) => string, date: (iso: string) => string, first?: string,
): { message: string; kept?: { title: string; rows: string[] }; details?: { summary: string; lines: string[] } } {
    // The heading says it is deleted (domains.deleted); a DKIM note, when
    // there is one, comes first.
    const sentences = first ? [first] : [];
    const rows: string[] = [];
    const lines: string[] = [];
    if (kept?.deleted.length) {
        sentences.push(t('domains.deleted.databasesDeleted', { names: kept.deleted.map((item) => item.name).join(', ') }));
        for (const item of kept.deleted) sentences.push(...databaseKeptNotes(t, item.name, item.answer));
    }
    if (kept?.databases.length) rows.push(t('domains.deleted.row.databases', { names: kept.databases.join(', ') }));
    const amount = (item: DomainDeletionHeld) => (item.bytes >= 0 ? size(item.bytes) : t('domains.deleted.sizeUnknown'));
    let total = 0;
    if (kept?.mail) {
        rows.push(t('domains.deleted.row.mail', { size: amount(kept.mail), date: date(kept.mail.removeAfter) }));
        total += Math.max(kept.mail.bytes, 0);
        if (kept.mail.path) lines.push(t('domains.deleted.detail.mail', { path: kept.mail.path }));
    }
    if (kept?.backups) {
        rows.push(t('domains.deleted.row.backups', { n: kept.backups.files, size: amount(kept.backups), date: date(kept.backups.removeAfter) }));
        total += Math.max(kept.backups.bytes, 0);
        if (kept.backups.path) lines.push(t('domains.deleted.detail.backups', { path: kept.backups.path }));
    }
    if (kept?.mail || kept?.backups) {
        const until = date((kept.mail ?? kept.backups)!.removeAfter);
        rows.push(admin ? t('domains.deleted.row.holdingAdmin', { size: size(total) }) : t('domains.deleted.row.askProvider', { date: until }));
    }
    const message = sentences.map((sentence) => (/[.!?]$/.test(sentence) ? sentence : `${sentence}.`)).join(' ');
    return {
        message,
        ...(rows.length ? { kept: { title: t('domains.deleted.keptTitle'), rows } } : {}),
        ...(admin && lines.length ? { details: { summary: t('domains.deleted.detailsTitle'), lines } } : {}),
    };
}
