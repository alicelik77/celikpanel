// What a finished component operation says beyond "installed" (alpha.124,
// P0 of the boot path): the notes a PHP tool's proof or another tool's
// problem leave in the result, a refused install's notable packages, ClamAV
// waiting for its first signatures and a mail profile served the default
// certificate. The global operation provider boots with the shell, but these
// notes are read only when an operation ends, so they are fetched then
// (ComponentOperation.tsx, loadOperationNotes) and their words live in the
// screen copy, which every route has loaded by then. A word that is not
// there is never shown as its own name: the plain "installed" is said.
// Biten bileşen işleminin "kuruldu" dışındaki notları; işlem bitince
// getirilir, sözcükleri ekran metnindedir.

type Translate = (key: string, params?: Record<string, string | number>) => string;

// A PHP tool needing attention after this install (alpha.104 review): the
// Panel carries the Agent's per-tool problems in the operation result.
// Bu kurulumdan sonra ilgi isteyen PHP aracı; işlem sonucunda taşınır.
const PHP_TOOL_NAMES = { webmail: 'Roundcube', phpmyadmin: 'phpMyAdmin', phppgadmin: 'phpPgAdmin' } as const;
type PHPToolProblem = { tool: keyof typeof PHP_TOOL_NAMES; version: string; kind: 'version_gone' | 'extensions_failed' };

export function decodePHPToolProblems(result: unknown): PHPToolProblem[] {
    if (!result || typeof result !== 'object') return [];
    const raw = (result as { php_tool_problems?: unknown }).php_tool_problems;
    if (!Array.isArray(raw)) return [];
    const out: PHPToolProblem[] = [];
    for (const item of raw) {
        const p = item as Record<string, unknown> | null;
        if (!p || typeof p !== 'object' || typeof p.tool !== 'string' || !(p.tool in PHP_TOOL_NAMES)
            || typeof p.version !== 'string' || !/^[0-9]{1,2}\.[0-9]{1,2}$/.test(p.version)
            || (p.kind !== 'version_gone' && p.kind !== 'extensions_failed')) continue;
        out.push({ tool: p.tool as PHPToolProblem['tool'], version: p.version, kind: p.kind });
    }
    return out;
}

// The proof of a PHP tool just installed (alpha.105 review): the Panel
// carries the Agent's answer in the operation result. Said when it moved
// the tool to another version, or when no version worked.
// Yeni kurulan PHP aracının kanıtı; işlem sonucunda taşınır.
export interface PHPToolProofNote {
    tool: keyof typeof PHP_TOOL_NAMES; version: string; moved: boolean; proved: boolean; first: string; extensions: string[];
    /** alpha.108: the first attempt's cause, so the toast says it (Boston said only "did not answer as itself"). */
    kind: string; status: number; detail: string;
}

export function decodePHPToolProof(result: unknown): PHPToolProofNote | null {
    if (!result || typeof result !== 'object') return null;
    const raw = (result as { php_tool_proof?: unknown }).php_tool_proof as Record<string, unknown> | undefined;
    if (!raw || typeof raw !== 'object' || typeof raw.tool !== 'string' || !(raw.tool in PHP_TOOL_NAMES)) return null;
    const version = typeof raw.version === 'string' && /^[0-9]{1,2}\.[0-9]{1,2}$/.test(raw.version) ? raw.version : '';
    const attempts = Array.isArray(raw.attempts) ? raw.attempts as Array<Record<string, unknown>> : [];
    const first = attempts[0] && typeof attempts[0].version === 'string' && /^[0-9]{1,2}\.[0-9]{1,2}$/.test(attempts[0].version) ? attempts[0].version : '';
    const cause = attempts[0]?.cause as Record<string, unknown> | undefined;
    const extensions = cause && cause.kind === 'missing_extensions' && Array.isArray(cause.extensions)
        ? cause.extensions.filter((item): item is string => typeof item === 'string' && /^[a-z0-9_]{1,32}$/.test(item))
        : [];
    const kind = cause && typeof cause.kind === 'string' ? cause.kind : '';
    const status = cause && typeof cause.status === 'number' && Number.isInteger(cause.status) ? cause.status : 0;
    const detail = cause && typeof cause.detail === 'string' ? cause.detail.slice(0, 240) : '';
    return { tool: raw.tool as PHPToolProofNote['tool'], version, moved: raw.moved === true, proved: raw.proved === true, first, extensions, kind, status, detail };
}

// The packages a refused install would also have brought (alpha.105 review),
// with what the server said of each (alpha.109: said as grouped lines).
export interface RefusedNotable {
    package: string;
    reason: string;
    component?: string;
    version?: string;
    from?: string;
    to?: string;
}

export function decodeNotableRefusal(result: unknown): RefusedNotable[] {
    if (!result || typeof result !== 'object') return [];
    const raw = (result as { notable_installs?: unknown }).notable_installs;
    if (!Array.isArray(raw)) return [];
    const text = (value: unknown) => (typeof value === 'string' && value.length <= 200 ? value : undefined);
    return raw.flatMap((item): RefusedNotable[] => {
        if (!item || typeof item !== 'object') return [];
        const value = item as Record<string, unknown>;
        if (typeof value.package !== 'string' || !/^[a-z0-9][a-z0-9+.:-]{0,160}$/.test(value.package)) return [];
        return [{
            package: value.package,
            reason: text(value.reason) ?? 'component',
            ...(text(value.component) ? { component: text(value.component) } : {}),
            ...(text(value.version) ? { version: text(value.version) } : {}),
            ...(text(value.from) ? { from: text(value.from) } : {}),
            ...(text(value.to) ? { to: text(value.to) } : {}),
        }];
    });
}

// notableRefusalLines: one package a line, packages sharing a reason under
// that reason, versions short ("php-common (2:93 → 2:101)"), in the words the
// install dialog uses (lib/installPreview.ts; repeated here because the
// operation provider loads this module without the screen's modules).
// Reddedilen kurulumun paketleri, satır satır ve nedene göre gruplu.
type Say = (key: string, params?: Record<string, string>) => string;
const shortVersion = (version: string) => {
    const match = /^(?:([0-9]+):)?([0-9]+(?:\.[0-9]+)*)/.exec(version);
    return match ? (match[1] ? `${match[1]}:` : '') + match[2] : version;
};
export function notableRefusalLines(items: readonly RefusedNotable[], say: Say, locale: string): string[] {
    const NOTABLE_WHY = ['new_php_version', 'apache', 'component', 'database_server', 'upgrade'];
    const repoName = (id: string) => {
        const key = `services.notable.repoName.${id}`;
        const said = id ? say(key) : key;
        return said === key ? id || say('services.notable.repoUnknown') : said;
    };
    const groups: { key: string; phrase: string; items: RefusedNotable[] }[] = [];
    const sharedRepo = items.find((item) => item.reason === 'vendor_shared' && item.component)?.component ?? '';
    for (const item of items) {
        const vendor = item.reason === 'vendor_shared' || item.reason === 'vendor_replaces_distribution';
        const reason = vendor ? 'vendor_shared' : NOTABLE_WHY.includes(item.reason) ? item.reason : 'component';
        const component = vendor ? item.component ?? sharedRepo : item.component ?? '';
        const key = `${reason}:${reason === 'new_php_version' ? item.version ?? '' : reason === 'apache' || reason === 'upgrade' ? '' : component}`;
        let group = groups.find((candidate) => candidate.key === key);
        if (!group) {
            const phrase = vendor
                ? say('services.notable.why.vendor_shared', { repo: repoName(component) })
                : say(`services.notable.why.${reason}`, { version: item.version ?? '', component });
            group = { key, phrase, items: [] };
            groups.push(group);
        }
        group.items.push(item);
    }
    const versions = (item: RefusedNotable) => {
        if (!item.from || !item.to) return '';
        const from = shortVersion(item.from);
        const to = shortVersion(item.to);
        return from === to ? ` (${item.from} → ${item.to})` : ` (${from} → ${to})`;
    };
    const lines: string[] = [];
    for (const group of groups) {
        if (group.items.length === 1) {
            lines.push(`${group.items[0].package}${versions(group.items[0])}: ${group.phrase}`);
            continue;
        }
        lines.push(`${group.phrase.charAt(0).toLocaleUpperCase(locale) + group.phrase.slice(1)}:`);
        for (const item of group.items) lines.push(`– ${item.package}${versions(item)}`);
    }
    return lines;
}

// The words of one key, or '' when the screen copy does not hold it.
const said = (t: Translate, key: string, params?: Record<string, string | number>) => {
    const text = t(key, params);
    return text === key ? '' : text;
};

// refusalDetails: the lines a refused notable install adds to its failure.
export function refusalDetails(result: unknown, t: Translate, locale: string): string[] {
    const notable = decodeNotableRefusal(result);
    const heading = said(t, 'services.installRefused.notable');
    if (notable.length === 0 || !heading) return [];
    return [heading, ...notableRefusalLines(notable, (key, params) => t(key, params), locale)];
}

export interface InstalledNote {
    kind: 'success' | 'info' | 'warning';
    text: string;
    /** alpha.127: the page that holds the next step (the tool's own page, where Manage is). */
    link?: { label: string; to: string };
}

// The component page of each PHP tool (Roundcube is the webmail tool).
const PHP_TOOL_PAGES: Record<keyof typeof PHP_TOOL_NAMES, string> = { webmail: '/components/roundcube', phpmyadmin: '/components/phpmyadmin', phppgadmin: '/components/phppgadmin' };
const toolLink = (t: Translate, tool: keyof typeof PHP_TOOL_NAMES) => ({ label: t('adminNext.openTool', { tool: PHP_TOOL_NAMES[tool] }), to: PHP_TOOL_PAGES[tool] });

// installedNotes: what a finished install says, in order. The first note
// replaces the plain "installed" (a mail profile on the default certificate,
// ClamAV waiting for its signatures); the PHP tool notes follow it.
export function installedNotes({ result, name, fallbackOnly, t }: { result: unknown; name: string; fallbackOnly: boolean; t: Translate }): InstalledNote[] {
    const notes: InstalledNote[] = [];
    const fallback = fallbackOnly ? said(t, 'services.mailProfiles.fallbackWarning', { name }) : '';
    // alpha.118 (ClamAV): installed, starting by itself after the first
    // signature download. Not a failure.
    const waiting = (result as { waiting?: unknown } | null | undefined)?.waiting === 'first_signatures'
        ? said(t, 'services.installedWaitingSignatures', { name })
        : '';
    if (fallback) notes.push({ kind: 'warning', text: fallback });
    else if (waiting) notes.push({ kind: 'info', text: waiting });
    else notes.push({ kind: 'success', text: t('services.installed', { name }) });
    // The tool just installed: moved to another version, or Not ready; the
    // first attempt's own cause (alpha.108).
    const proof = decodePHPToolProof(result);
    if (proof && (proof.moved || !proof.proved)) {
        const why = proof.extensions.length > 0
            ? t('services.phpToolProof.whyMissing', { version: proof.first, extensions: proof.extensions.join(', ') })
            : proof.kind === 'check_unreadable'
                ? t('services.phpToolProof.whyCheck', { version: proof.first, status: proof.status })
                : proof.detail
                    ? t('services.phpToolProof.whySaid', { detail: proof.detail })
                    : t('services.phpToolProof.whyOther');
        const text = proof.moved
            ? said(t, 'services.phpToolProof.moved', { tool: PHP_TOOL_NAMES[proof.tool], version: proof.version, first: proof.first, why })
            : said(t, 'services.phpToolProof.notReady', { tool: PHP_TOOL_NAMES[proof.tool], version: proof.version || proof.first, why });
        if (text) notes.push({ kind: 'warning', text, link: toolLink(t, proof.tool) });
    }
    // alpha.124: the PHP version chosen in the install window could not be
    // used; the tool runs on the version it had.
    const bind = (result as { php_tool_bind?: Record<string, unknown> } | null | undefined)?.php_tool_bind;
    const version = typeof bind?.version === 'string' && /^[0-9]{1,2}\.[0-9]{1,2}$/.test(bind.version) ? bind.version : '';
    if (bind && bind.bound === false && version && typeof bind.tool === 'string' && bind.tool in PHP_TOOL_NAMES) {
        const previous = typeof bind.previous === 'string' && /^[0-9]{1,2}\.[0-9]{1,2}$/.test(bind.previous) ? bind.previous : proof?.version || '—';
        const detail = typeof bind.detail === 'string' ? bind.detail.slice(0, 240) : '';
        const text = said(t, 'cinst.bindFailed', { tool: PHP_TOOL_NAMES[bind.tool as keyof typeof PHP_TOOL_NAMES], version, previous, why: detail ? ` (${detail})` : '' });
        if (text) notes.push({ kind: 'warning', text, link: toolLink(t, bind.tool as keyof typeof PHP_TOOL_NAMES) });
    }
    // Another PHP tool needing attention does not fail this install
    // (alpha.104 review): it is said as a warning that names the tool.
    for (const problem of decodePHPToolProblems(result)) {
        const text = said(t, problem.kind === 'version_gone' ? 'services.phpToolProblem.versionGone' : 'services.phpToolProblem.extensionsFailed', {
            tool: PHP_TOOL_NAMES[problem.tool],
            version: problem.version,
        });
        if (text) notes.push({ kind: 'warning', text, link: toolLink(t, problem.tool) });
    }
    return notes;
}
