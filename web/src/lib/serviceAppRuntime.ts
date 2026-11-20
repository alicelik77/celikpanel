// Why an AI or automation app cannot be installed on this server yet, said in
// the reader's language (alpha.140). The server sends facts, never a sentence:
// the app's runtime ("node" or "python") and the catalogue's versions
// (`needs`), and what this server has (`found`, with `found_known` false
// when the Agent did not report it). This module builds the whole sentence
// from them: the reason, who acts, the next step and how it resumes
// (D-024). An answer without the facts (an earlier release) still gets a
// sentence, only without the versions; the server's English `detail` is never
// put into it.
//
// Uygulamanın neden henüz kurulamadığı okurun dilinde: sunucu olguları
// gönderir, cümleyi bu modül kurar; sunucunun İngilizce `detail` alanı
// cümleye girmez.

export interface ServiceAppRuntimeFound {
    version: string;
    source: string;
    venv?: boolean;
}

export interface ServiceAppRuntimeFacts {
    name: string;
    runtime: string;
    available: boolean;
    reason?: string;
    needs?: string[];
    found?: ServiceAppRuntimeFound[] | null;
    found_known?: boolean;
}

type T = (key: string, vars?: Record<string, string | number>) => string;

/** Who reads the line: the administrator gets the action, anyone else who acts. */
export type RuntimeLineReader = 'admin' | 'account';

export interface RuntimeLine {
    /** The sentences, in order: what is needed, what the server has, the next step. */
    text: string;
    /** The administrator's action on Components, or none. */
    action: { to: string; label: string } | null;
}

const RUNTIME_REASONS = new Set(['runtime_missing', 'venv_missing', 'os_unsupported']);

/** Whether this module words the app's reason (otherwise the caller keeps its own line). */
export function hasRuntimeLine(app: Pick<ServiceAppRuntimeFacts, 'available' | 'reason'>): boolean {
    return !app.available && !!app.reason && RUNTIME_REASONS.has(app.reason);
}

function list(values: string[], t: T): string {
    if (values.length <= 1) return values.join('');
    return values.slice(0, -1).join(', ') + t('svcapps.line.or') + values[values.length - 1];
}

export function runtimeLine(app: ServiceAppRuntimeFacts, reader: RuntimeLineReader, t: T): RuntimeLine | null {
    if (!hasRuntimeLine(app)) return null;
    const node = app.runtime === 'node';
    const needs = (app.needs ?? []).filter((v) => typeof v === 'string' && v !== '');
    const versions = list(needs, t);
    const first = needs[0] ?? '';
    const found = app.found_known ? (app.found ?? []).filter((f) => f && typeof f.version === 'string') : null;
    const parts: string[] = [];

    // What is needed (the catalogue's versions).
    parts.push(versions
        ? t(node ? 'svcapps.line.need.node' : 'svcapps.line.need.python', { versions })
        : t(node ? 'svcapps.line.need.node.plain' : 'svcapps.line.need.python.plain'));

    // What this server has, when the Agent said (all of it: a comma list).
    if (found !== null) {
        const seen = found.map((f) => f.version).join(', ');
        if (app.reason === 'venv_missing') {
            const withoutVenv = found.filter((f) => !f.venv && needs.includes(f.version)).map((f) => f.version);
            parts.push(t('svcapps.line.found.venv', { versions: (withoutVenv.length ? withoutVenv : found.map((f) => f.version)).join(', ') }));
        } else if (found.length === 0) {
            parts.push(t(node ? 'svcapps.line.found.node.none' : 'svcapps.line.found.python.none'));
        } else {
            parts.push(t(node ? 'svcapps.line.found.node' : 'svcapps.line.found.python', { versions: seen }));
        }
    }

    // Who acts, the next step and how it resumes.
    let action: RuntimeLine['action'] = null;
    const who = reader === 'admin' ? 'admin' : 'account';
    if (app.reason === 'runtime_missing') {
        parts.push(first
            ? t(`svcapps.line.next.${who}.runtime_missing`, { version: first })
            : t(`svcapps.line.next.${who}.runtime_missing.plain`));
        if (reader === 'admin') {
            action = { to: '/components?component=node&install=', label: first ? t('svcapps.line.action.node', { version: first }) : t('svcapps.line.action.node.plain') };
        }
    } else if (app.reason === 'venv_missing') {
        parts.push(t(`svcapps.line.next.${who}.venv_missing`));
        if (reader === 'admin') action = { to: '/components?component=python&install=', label: t('svcapps.line.action.python') };
    } else {
        const has = found && found.length ? found.map((f) => f.version).join(', ') : '';
        parts.push(reader === 'admin'
            ? (has && versions
                ? t('svcapps.line.next.admin.os_unsupported', { app: app.name, found: has, versions })
                : t('svcapps.line.next.admin.os_unsupported.plain'))
            : t('svcapps.line.next.account.os_unsupported'));
    }
    return { text: parts.join(' '), action };
}
