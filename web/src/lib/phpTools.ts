// Which PHP version webmail (Roundcube), phpMyAdmin and phpPgAdmin run on,
// and who uses each installed PHP-FPM version (alpha.104, owner decision of
// 2026-10-23). The Panel answers GET /api/v1/service/php-tools from the
// Agent's read-only binding read and its own site records; "unknown" means it
// could not be read now and nothing is claimed.
//
// Webmail, phpMyAdmin ve phpPgAdmin'in koştuğu PHP sürümü ve her kurulu PHP-FPM
// sürümünü kimin kullandığı. "unknown": şimdi okunamadı, hiçbir şey iddia edilmez.

import type { TranslationKey } from '../i18n/en';
import { answerWasLost } from './requestIdentity';

export type PHPToolName = 'webmail' | 'phpmyadmin' | 'phppgadmin';

export interface PHPToolState {
    tool: PHPToolName;
    service_id: string;
    installed: boolean;
    /** The bound version, or '' when nothing is bound yet. */
    version: string;
    /** The bound version's PHP-FPM socket exists now. */
    ready: boolean;
    version_installed: boolean;
    /** The tool's PHP extension packages are installed for its version
     *  (package presence, not proof PHP loads them). Absent from an older
     *  Panel: taken as present, nothing is claimed. */
    extensions_present: boolean;
}

export interface PHPVersionUsage {
    version: string;
    sites: number;
    site_names: string[];
    tools: PHPToolName[];
}

export type PHPToolsInfo =
    | { state: 'read'; tools: PHPToolState[]; versions: PHPVersionUsage[] }
    | { state: 'unknown' };

const TOOLS: ReadonlySet<string> = new Set(['webmail', 'phpmyadmin', 'phppgadmin']);
const VERSION = /^[0-9]{1,2}\.[0-9]{1,2}$/;
const strings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === 'string');

export const PHP_TOOL_SERVICE_ID: Record<PHPToolName, string> = { webmail: 'roundcube', phpmyadmin: 'phpmyadmin', phppgadmin: 'phppgadmin' };

export function phpToolForService(serviceId: string): PHPToolName | null {
    for (const [tool, id] of Object.entries(PHP_TOOL_SERVICE_ID)) if (id === serviceId) return tool as PHPToolName;
    return null;
}

export function decodePHPTools(value: unknown): PHPToolsInfo {
    if (!value || typeof value !== 'object') return { state: 'unknown' };
    const v = value as Record<string, unknown>;
    if (v.state !== 'read' || !Array.isArray(v.tools) || !Array.isArray(v.versions)) return { state: 'unknown' };
    const tools: PHPToolState[] = [];
    for (const raw of v.tools) {
        const t = raw as Record<string, unknown> | null;
        if (!t || typeof t !== 'object' || typeof t.tool !== 'string' || !TOOLS.has(t.tool) || typeof t.service_id !== 'string'
            || typeof t.installed !== 'boolean' || typeof t.version !== 'string' || (t.version !== '' && !VERSION.test(t.version))
            || typeof t.ready !== 'boolean' || typeof t.version_installed !== 'boolean') return { state: 'unknown' };
        tools.push({ tool: t.tool as PHPToolName, service_id: t.service_id, installed: t.installed, version: t.version, ready: t.ready, version_installed: t.version_installed, extensions_present: t.extensions_present !== false });
    }
    const versions: PHPVersionUsage[] = [];
    for (const raw of v.versions) {
        const u = raw as Record<string, unknown> | null;
        if (!u || typeof u !== 'object' || typeof u.version !== 'string' || !VERSION.test(u.version) || typeof u.sites !== 'number'
            || !Number.isInteger(u.sites) || u.sites < 0 || !strings(u.site_names) || !strings(u.tools) || !u.tools.every((tool) => TOOLS.has(tool))) return { state: 'unknown' };
        versions.push({ version: u.version, sites: u.sites, site_names: u.site_names, tools: u.tools as PHPToolName[] });
    }
    return { state: 'read', tools, versions };
}

export async function readPHPTools(signal?: AbortSignal): Promise<PHPToolsInfo> {
    try {
        const response = await fetch('/api/v1/service/php-tools', { cache: 'no-store', ...(signal ? { signal } : {}) });
        if (!response.ok) return { state: 'unknown' };
        return decodePHPTools(await response.json());
    } catch {
        return { state: 'unknown' };
    }
}

// The failure of a move, as the Panel answers it (409 with these fields).
// Bir taşımanın başarısızlığı, Panel'in yanıtıyla.
export interface PHPToolMoveFailure {
    error: string;
    code: string;
    /** The cause alone, when the server gave it; else ''. */
    detail: string;
    tool: PHPToolName | '';
    version: string;
    previous: string;
    restored: boolean;
    mutation_applied: boolean;
    moved: PHPToolName[];
    not_attempted: PHPToolName[];
    /** The structured cause, translated on the screen; null when absent. */
    cause: PHPToolCause | null;
}

// Why a move failed, as the Agent says it (alpha.104 review): a kind the
// screen translates, with its facts. An unexpected kind is quoted.
// Taşımanın neden başarısız olduğu: ekranın çevirdiği tür ve olguları.
export interface PHPToolCause {
    kind: string;
    version: string;
    extensions: string[];
    status: number;
    running: string;
    detail: string;
}

const CAUSE_KINDS: ReadonlySet<string> = new Set([
    'missing_extensions', 'extensions_install_failed', 'wrong_version', 'no_answer', 'http_status', 'served_as_file', 'tool_error',
    'not_tool_page', 'not_running', 'version_not_installed', 'tool_not_installed', 'unsupported', 'fpm_reload_failed',
    'config_owner_edited', 'config_prepare_failed', 'config_read_failed', 'config_write_failed', 'nginx_rejected', 'nginx_reload_failed',
]);

export function decodePHPToolCause(value: unknown): PHPToolCause | null {
    if (!value || typeof value !== 'object') return null;
    const v = value as Record<string, unknown>;
    if (typeof v.kind !== 'string' || v.kind === '') return null;
    return {
        kind: v.kind,
        version: typeof v.version === 'string' ? v.version : '',
        extensions: strings(v.extensions) ? v.extensions : [],
        status: typeof v.status === 'number' && Number.isInteger(v.status) ? v.status : 0,
        running: typeof v.running === 'string' ? v.running : '',
        detail: typeof v.detail === 'string' ? v.detail : '',
    };
}

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

// The cause in the reader's language; a kind this screen does not know is
// quoted as the server said it, never guessed.
// Nedenin okurun dilinde söylenişi; bilinmeyen tür sunucunun sözüyle alıntılanır.
export function phpToolCauseText(t: Translate, cause: PHPToolCause | null, said: string): string {
    if (!cause || !CAUSE_KINDS.has(cause.kind)) {
        const words = cause?.detail || said;
        return words ? t('component.phpTool.cause.other', { detail: words }) : t('component.phpTool.failedUnknown');
    }
    return t(('component.phpTool.cause.' + cause.kind) as TranslationKey, {
        version: cause.version, extensions: cause.extensions.join(', '), status: cause.status, running: cause.running, detail: cause.detail,
    });
}

export function decodePHPToolMoveFailure(value: unknown): PHPToolMoveFailure | null {
    if (!value || typeof value !== 'object') return null;
    const v = value as Record<string, unknown>;
    if (typeof v.error !== 'string' || typeof v.code !== 'string') return null;
    const tools = (raw: unknown): PHPToolName[] => (strings(raw) ? raw.filter((tool) => TOOLS.has(tool)) as PHPToolName[] : []);
    return {
        error: v.error,
        code: v.code,
        detail: typeof v.detail === 'string' ? v.detail : '',
        tool: typeof v.tool === 'string' && TOOLS.has(v.tool) ? v.tool as PHPToolName : '',
        version: typeof v.version === 'string' ? v.version : '',
        previous: typeof v.previous === 'string' ? v.previous : '',
        restored: v.restored === true,
        mutation_applied: v.mutation_applied === true,
        moved: tools(v.moved),
        not_attempted: tools(v.not_attempted),
        cause: decodePHPToolCause(v.cause),
    };
}

// The JSON body of an answer, or null when it has none: the caller then
// says the server's coded refusal through readApiError, never a guess.
// Bir yanıtın JSON gövdesi; yoksa null.
export async function answerBody(response: Response): Promise<unknown> {
    try {
        return await response.clone().json();
    } catch {
        return null;
    }
}

export type PHPToolMoveAnswer =
    | { kind: 'moved' }
    | { kind: 'refused'; failure: PHPToolMoveFailure | null }
    | { kind: 'lost' };

// Moves one tool to one installed PHP version (POST, request identity added
// by the one fetch interceptor). "lost": no answer arrived, so whether the
// move happened is not known.
// Bir aracı kurulu bir PHP sürümüne taşır; "lost": yanıt gelmedi.
export async function movePHPTool(tool: PHPToolName, version: string): Promise<PHPToolMoveAnswer> {
    let response: Response;
    try {
        response = await fetch('/api/v1/service/php-tools/bind', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ tool, version }),
        });
    } catch {
        return { kind: 'lost' };
    }
    // A gateway's own page in place of the Panel's answer (502/504 HTML,
    // after the one re-ask): whether the move happened is not known.
    // Panel'in yanıtı yerine geçidin kendi sayfası: taşımanın olup olmadığı bilinmez.
    if (answerWasLost(response)) return { kind: 'lost' };
    const body = await answerBody(response);
    if (response.ok && body && typeof body === 'object' && (body as { bound?: unknown }).bound === true) return { kind: 'moved' };
    return { kind: 'refused', failure: decodePHPToolMoveFailure(body) };
}
