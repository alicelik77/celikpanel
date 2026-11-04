// Uploaded programs (alpha.119, docs/RUNTIME-APPS.md): the shapes the domain's
// "Programs" tab and the administrator's offering read, the decoders that
// refuse an answer of another shape, and the guidance of every state the
// server names (D-024): which kind of problem it is and who acts. The words
// are runtime.state.<code> (the reason), .next and .resume in the
// "runtimeApps" screen part.
//
// Yüklenen programlar: alan adının "Programlar" sekmesinin ve yöneticinin
// sunma ekranının okuduğu biçimler, çözücüler ve sunucunun adlandırdığı her
// durumun türü ile kimin işlem yapacağı.

export interface RuntimeRisk {
    can_do: string[];
    blocked: string[];
    network: string[];
    remains: string[];
}

export interface RuntimeLimits {
    memory_mb: number;
    cpu_percent: number;
    tasks_max: number;
}

export interface RuntimeSandboxLevel {
    known: boolean;
    level?: 'full' | 'reduced';
    gaps: string[];
}

export interface RuntimeKindView {
    kind: string;
    implemented: boolean;
    offering_id: string;
    offered: boolean;
    refused_by?: string;
    grantor?: string;
    permission_granted: boolean;
    permission_refused_by?: string;
    held: boolean;
    risk: RuntimeRisk;
    limits: RuntimeLimits;
    accepted_level: 'full' | 'reduced';
    accepted_gaps: string[];
}

export interface RuntimeEnvEntry {
    key: string;
    /** Absent for a secret: a secret value is never sent back. */
    value?: string;
    secret: boolean;
}

export interface RuntimeAppStatus {
    unit_file: string;
    drop_ins?: string[];
    unit: { present: boolean; active: boolean; enabled: boolean };
    active_state?: string;
    sub_state?: string;
    result?: string;
    restarts?: number;
    start_limit_hit?: boolean;
    memory_mb?: number;
    oom_kills?: number;
    pids_max?: number;
    serving: boolean;
    exposed?: string[];
    squatted?: boolean;
    health: { checked: boolean; status?: number; ok: boolean; error?: string };
    entry_check?: { code: string; detail?: string; format?: string };
    level?: string;
    gaps?: string[];
    sandbox_proof?: { ok: boolean; code: string; detail?: string };
    env_keys?: string[];
    sandbox_generation?: number;
}

export type RuntimeAppState = 'created' | 'deploying' | 'running' | 'stopped' | 'failed' | 'removing';

export interface RuntimeApp {
    id: number;
    kind: string;
    entry: string;
    args: string[];
    health_path: string;
    port: number;
    state: RuntimeAppState;
    deployed: boolean;
    url: string;
    folder: string;
    upload_dir: string;
    unit: string;
    env: RuntimeEnvEntry[];
    status?: RuntimeAppStatus;
    status_unknown?: boolean;
    /** D-024 state codes, most blocking first. */
    states: string[];
    created_at: string;
}

export type RuntimeClass = 'verified_failure' | 'prerequisite' | 'unknown';

export interface RuntimeOperation {
    id: string;
    app_id: number;
    kind: 'deploy' | 'remove';
    state: 'running' | 'succeeded' | 'failed';
    step: string;
    class?: RuntimeClass;
    reason?: string;
    message?: string;
    remove_files?: boolean;
    created_at: string;
    updated_at: string;
}

export interface RuntimeAppsView {
    domain: string;
    suspended: boolean;
    has_site: boolean;
    project_type: string;
    certificate: boolean;
    disk_full: boolean;
    sandbox: RuntimeSandboxLevel;
    kinds: RuntimeKindView[];
    app: RuntimeApp | null;
    operation: RuntimeOperation | null;
    port_range: [number, number];
}

export interface RuntimeAdminKind {
    kind: string;
    offering_id: string;
    implemented: boolean;
    risk: RuntimeRisk;
    risk_digest: string;
    offered: boolean;
    offer_stale: boolean;
    held: boolean;
    held_reason?: string;
    limits: RuntimeLimits;
    accepted_level: 'full' | 'reduced';
    accepted_gaps: string[];
    level_gap: boolean;
    apps: number;
    running: number;
}

export interface RuntimeAdminView {
    kinds: RuntimeAdminKind[];
    sandbox: RuntimeSandboxLevel;
    port_range: [number, number];
}

const isObject = (raw: unknown): raw is Record<string, unknown> => !!raw && typeof raw === 'object' && !Array.isArray(raw);
const isSandbox = (raw: unknown): raw is RuntimeSandboxLevel => isObject(raw) && typeof raw.known === 'boolean' && Array.isArray(raw.gaps);

function decodeApp(raw: unknown): RuntimeApp | null {
    if (raw === null) return null;
    if (!isObject(raw) || typeof raw.id !== 'number' || typeof raw.entry !== 'string' || !Array.isArray(raw.args)
        || !Array.isArray(raw.env) || !Array.isArray(raw.states) || typeof raw.state !== 'string') {
        throw new Error('shape');
    }
    return raw as unknown as RuntimeApp;
}

function decodeOperation(raw: unknown): RuntimeOperation | null {
    if (raw === null || raw === undefined) return null;
    if (!isObject(raw) || typeof raw.id !== 'string' || typeof raw.state !== 'string' || typeof raw.step !== 'string') throw new Error('shape');
    return raw as unknown as RuntimeOperation;
}

/** The domain's view; an answer of another shape is "unknown", never a default. */
export function decodeRuntimeApps(raw: unknown): RuntimeAppsView {
    if (!isObject(raw) || typeof raw.domain !== 'string' || !Array.isArray(raw.kinds) || !isSandbox(raw.sandbox)
        || typeof raw.has_site !== 'boolean' || !('app' in raw)) {
        throw new Error('shape');
    }
    return { ...(raw as unknown as RuntimeAppsView), app: decodeApp(raw.app), operation: decodeOperation(raw.operation) };
}

export function decodeRuntimeAdmin(raw: unknown): RuntimeAdminView {
    if (!isObject(raw) || !Array.isArray(raw.kinds) || !isSandbox(raw.sandbox)) throw new Error('shape');
    return raw as unknown as RuntimeAdminView;
}

/** The `{state}` answer of start, stop and restart, and `{lines}` of the log. */
export function decodeLogLines(raw: unknown): string[] {
    const lines = (raw as { lines?: unknown } | null)?.lines;
    if (lines === null) return [];
    if (!Array.isArray(lines)) throw new Error('shape');
    return lines.filter((line): line is string => typeof line === 'string');
}

// ---- guidance (D-024) ---------------------------------------------------------

/** Who acts on a state: the words are runtime.who.<who>. */
export type RuntimeWho = 'you' | 'administrator' | 'grantor' | 'youThenAdministrator' | 'nobody';

/**
 * Every state code the server can put in `app.states`, in the server's order
 * (most blocking first), with the kind of problem and who acts. A verified
 * failure was seen on the server; a prerequisite is something that has to be
 * done first; unknown is a result nobody could check.
 */
export const RUNTIME_STATE_GUIDANCE: Record<string, { class: RuntimeClass; who: RuntimeWho }> = {
    agent_unreachable: { class: 'unknown', who: 'youThenAdministrator' },
    suspended: { class: 'prerequisite', who: 'grantor' },
    held: { class: 'prerequisite', who: 'administrator' },
    not_offered: { class: 'prerequisite', who: 'grantor' },
    sandbox_reduced: { class: 'prerequisite', who: 'administrator' },
    no_certificate: { class: 'prerequisite', who: 'you' },
    disk_full: { class: 'prerequisite', who: 'you' },
    entry_missing: { class: 'prerequisite', who: 'you' },
    entry_not_executable: { class: 'prerequisite', who: 'you' },
    entry_link: { class: 'prerequisite', who: 'you' },
    entry_not_regular: { class: 'prerequisite', who: 'you' },
    entry_not_owned: { class: 'prerequisite', who: 'youThenAdministrator' },
    entry_setid: { class: 'prerequisite', who: 'you' },
    entry_writable: { class: 'prerequisite', who: 'you' },
    entry_unknown_format: { class: 'prerequisite', who: 'you' },
    entry_wrong_arch: { class: 'prerequisite', who: 'you' },
    entry_interpreter: { class: 'prerequisite', who: 'you' },
    app_folder_not_owned: { class: 'prerequisite', who: 'administrator' },
    app_folder_not_real: { class: 'prerequisite', who: 'you' },
    entry_unreadable: { class: 'unknown', who: 'youThenAdministrator' },
    restart_loop: { class: 'verified_failure', who: 'you' },
    oom: { class: 'verified_failure', who: 'you' },
    listens_elsewhere: { class: 'verified_failure', who: 'you' },
    port_taken: { class: 'verified_failure', who: 'administrator' },
    not_listening: { class: 'verified_failure', who: 'you' },
    health_failing: { class: 'verified_failure', who: 'you' },
};

export const RUNTIME_STATE_CODES = Object.keys(RUNTIME_STATE_GUIDANCE);

/**
 * Failure reasons of an operation that are not state codes; each has
 * runtime.reason.<code>. A reason that is also a state code is shown with
 * that state's guidance (reason, who acts, next action, how it resumes).
 */
export const RUNTIME_OPERATION_REASONS = [
    'env_edited',
    'entry_invalid', 'entry_unreadable', 'unit_edited', 'unit_render', 'unit_write', 'unit_dir', 'unit_copy',
    'unit_remove', 'unit_missing', 'env_remove', 'env_file', 'sandbox_egress', 'sandbox_helper', 'sandbox_proof', 'site_folder',
    'site_account', 'build_mismatch', 'systemctl', 'start_failed', 'publish', 'unpublish', 'files', 'verify',
    'app_missing', 'invalid_request',
] as const;

export const RUNTIME_STEPS = ['validate', 'render', 'restart', 'proof', 'health', 'publish', 'done', 'unpublish', 'remove'] as const;

/** The risk codes the server sends (runtime.risk.<code>). */
export const RUNTIME_RISK_CODES: Record<keyof RuntimeRisk, readonly string[]> = {
    can_do: ['any_code', 'own_folder', 'loopback_port', 'own_outbound'],
    blocked: ['other_sites', 'panel_files', 'privileges', 'mail_ports', 'metadata', 'other_ports', 'system_interpreters'],
    network: ['https_only', 'outbound_open'],
    remains: ['outbound_abuse', 'mail_apis', 'cpu_memory_share', 'reduced_level', 'customer_code'],
};

/** The refusal reasons of code RUNTIME_APP (err.RUNTIME_APP.<reason>). */
export const RUNTIME_REFUSAL_REASONS = [
    'env_edited',
    'no_site', 'suspended', 'held', 'kind_unavailable', 'agent_unreachable', 'sandbox_reduced', 'entry_invalid',
    'args_invalid', 'health_path_invalid', 'site_type', 'site_features', 'already_created', 'port_taken',
    'not_created', 'busy', 'no_certificate', 'not_deployed', 'env_values', 'logs_unavailable', 'confirm_files',
    'app_created', 'unit_missing', 'systemctl', 'env_file', 'invalid_request', 'risk_changed', 'limits', 'hold_reason',
] as const;

/**
 * Refusals whose server sentence carries the one detail that matters (which
 * argument, which key, what systemd said): the screen shows it beside its
 * own words.
 */
export const RUNTIME_DETAIL_REASONS: ReadonlySet<string> = new Set([
    'entry_invalid', 'args_invalid', 'env_values', 'site_features', 'unit_missing', 'systemctl', 'env_file', 'invalid_request',
]);

/** Environment keys CelikPanel or the sandbox sets; the server refuses them. */
export const RUNTIME_REFUSED_ENV_KEYS = ['PATH', 'HOME', 'PORT', 'HOST', 'USER', 'LOGNAME', 'SHELL', 'PYTHONPATH', 'PYTHONHOME', 'PYTHONSTARTUP'];
export const RUNTIME_REFUSED_ENV_PREFIXES = ['LD_', 'CELIKPANEL_', 'SYSTEMD_'];
const ENV_KEY = /^[A-Z][A-Z0-9_]{0,63}$/;
const ARG = /^(?:[A-Za-z0-9._@:/+=,-]|\{port\})+$/;
const ENTRY = /^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}(\/[A-Za-z0-9_][A-Za-z0-9._-]{0,63}){0,3}$/;
const HEALTH = /^\/[A-Za-z0-9._~/-]*$/;

/** Why the screen would not send a key ('' = fine); the server decides. */
export function envKeyProblem(key: string): '' | 'format' | 'reserved' {
    if (!ENV_KEY.test(key)) return 'format';
    if (RUNTIME_REFUSED_ENV_KEYS.includes(key) || RUNTIME_REFUSED_ENV_PREFIXES.some((prefix) => key.startsWith(prefix))) return 'reserved';
    return '';
}

/** The argument tokens of a text field, one per line; empty lines dropped. */
export function argsFromText(text: string): string[] {
    return text.split('\n').map((line) => line.trim()).filter((line) => line !== '');
}

export const argsValid = (args: string[]): boolean => args.length <= 32 && args.every((arg) => arg.length <= 256 && ARG.test(arg));
export const entryValid = (entry: string): boolean => ENTRY.test(entry.trim().replace(/^bin\//, '')) && !entry.split('/').some((part) => part.startsWith('..'));
export const healthPathValid = (path: string): boolean => path === '' || HEALTH.test(path);
