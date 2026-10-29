import { setupPurposes, type ServerSetupCheck, type SetupPurpose } from './serverSetup';

export interface SetupPlanStep { id: string; kind: 'infrastructure_dns' | 'access_dns' | 'dns' | 'dns_publisher' | 'dns_readiness' | 'service' | 'runtime' | 'mail_profile' | 'mail_enrollment' | 'mail_dns' | 'peer_join' | 'peer_names' | 'firewall' | 'panel_certificate' | 'mail_certificate' | 'verify'; target: string; qualifier?: string }
export interface SetupInfrastructureDNSPlan {
    zone: string; existing_zone_id?: number; expected_digest: string;
    records: { name: string; type: string; content: string; ttl: number; action: 'add' | 'keep' }[];
}
export interface ServerSetupPlan {
    id: string; version: number; revision: number; purpose: SetupPurpose;
    steps: SetupPlanStep[]; blockers: string[]; can_start: boolean;
    tcp_ports: number[]; udp_ports: number[]; preserve_ssh: boolean;
    persist_firewall: boolean; hostname_change?: string; contact_email: string;
    infrastructure_dns?: SetupInfrastructureDNSPlan;
    server_ip?: string;
    components?: { id: string; selected: boolean; required: boolean; installed: boolean }[];
    remote_dns_connection?: { id: string; endpoint: string; nameservers: string[] };
    // Distribution point updates of installed packages the component installs
    // would also bring; the setup confirmation covers them (Beta-1 fix).
    package_updates?: { package: string; from?: string; to?: string }[];
    package_updates_state?: 'known' | 'unknown';
    // The stored list's digest; the start sends it back (the list the page showed).
    package_updates_digest?: string;
}
export interface ServerSetupExecution {
    id: string; request_id: string; plan_id: string;
    status: 'running' | 'waiting' | 'failed' | 'succeeded'; phase: string;
    steps: (SetupPlanStep & { status: 'pending' | 'running' | 'failed' | 'succeeded' })[];
    error?: SetupExecutionError; panel_url?: string; checks?: ServerSetupCheck[];
    context?: SetupExecutionContext;
    // The run succeeded and belongs to a completed setup that is now being
    // extended (server answer, set18 D1): it is not the current operation.
    // Genisletilen tamamlanmis kurulumun calismasi; guncel islem degildir.
    superseded?: boolean;
}
// component/step/detail are optional install-failure guidance; detail is one
// bounded, redacted host line. reason is the typed cause of HOST_MUTATION_BUSY.
export interface SetupExecutionError { code: string; message: string; component?: string; step?: string; detail?: string; reason?: string }
export interface SetupExecutionContext {
    dns_mode: 'local' | 'external' | 'existing'; dns_role: 'primary' | 'secondary' | '';
    dns_engine: string; local_nameserver: string; local_ip: string; peer_nameserver: string; peer_ip: string;
    panel_domain: string; mail_hostname: string; dns_hosting_management: string;
    access_dns_ip?: string; infrastructure_dns?: SetupInfrastructureDNSPlan;
    /** On a DNS secondary whose public DNS check waits (2026-10-16): whether this
     *  server can ask the primary for the record, and what that request got. */
    peer_host_record?: SetupPeerHostRecord;
}
export interface SetupPeerHostRecord {
    channel: boolean;
    outcome?: { status: 'applied' | 'refused' | 'unknown' | 'unsupported' | 'none'; code?: string; result?: string; zone?: string; name: string; content: string };
}
// An additive field that is not the contract is dropped on its own; the rest of
// the context stays. Ek alan sozlesme degilse yalniz o alan dusurulur.
function decodePeerHostRecord(value: unknown): SetupPeerHostRecord | undefined {
    if (!isRecord(value) || typeof value.channel !== 'boolean') return undefined;
    const outcome = value.outcome;
    if (outcome === undefined) return { channel: value.channel };
    if (!isRecord(outcome) || !['applied', 'refused', 'unknown', 'unsupported', 'none'].includes(String(outcome.status))
        || typeof outcome.name !== 'string' || typeof outcome.content !== 'string'
        || ['code', 'result', 'zone'].some(key => outcome[key] !== undefined && (typeof outcome[key] !== 'string' || (outcome[key] as string).length > 253))) return { channel: value.channel };
    return { channel: value.channel, outcome: {
        status: outcome.status as NonNullable<SetupPeerHostRecord['outcome']>['status'], name: outcome.name, content: outcome.content,
        ...(outcome.code ? { code: outcome.code as string } : {}), ...(outcome.result ? { result: outcome.result as string } : {}), ...(outcome.zone ? { zone: outcome.zone as string } : {}),
    } };
}
// mail_dns since alpha.112; peer_join and peer_names since alpha.114.
const kinds = ['infrastructure_dns', 'access_dns', 'dns', 'dns_publisher', 'dns_readiness', 'service', 'runtime', 'mail_profile', 'mail_enrollment', 'mail_dns', 'peer_join', 'peer_names', 'firewall', 'panel_certificate', 'mail_certificate', 'verify'];
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const identity = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
function isStep(value: unknown): value is SetupPlanStep {
    return isRecord(value) && typeof value.id === 'string' && value.id !== ''
        && kinds.includes(String(value.kind)) && typeof value.target === 'string'
        && (value.qualifier === undefined || typeof value.qualifier === 'string');
}
function isInfrastructureDNSPlan(value: unknown): value is SetupInfrastructureDNSPlan {
    return isRecord(value) && typeof value.zone === 'string' && value.zone.length > 0 && value.zone.length <= 253
        && (value.existing_zone_id === undefined || (Number.isSafeInteger(value.existing_zone_id) && (value.existing_zone_id as number) > 0))
        && typeof value.expected_digest === 'string' && value.expected_digest.length <= 128
        // The setup's own records plus, since alpha.112, the imported and mail
        // records of the main domain (at most 64 imports).
        && Array.isArray(value.records) && value.records.length > 0 && value.records.length <= 128
        && value.records.every(item => isRecord(item) && typeof item.name === 'string' && item.name.length <= 253
            && typeof item.type === 'string' && ['SOA', 'NS', 'A', 'AAAA', 'CNAME', 'MX', 'TXT', 'CAA'].includes(item.type)
            && typeof item.content === 'string' && item.content.length <= 8192
            && Number.isSafeInteger(item.ttl) && (item.ttl as number) >= 0 && (item.ttl as number) <= 2147483647
            && ['add', 'keep'].includes(String(item.action)));
}
export function decodeSetupPlan(value: unknown, revision: number): ServerSetupPlan | null {
    if (!isRecord(value) || !identity(value.id) || value.version !== 1 || value.revision !== revision
        || !setupPurposes.includes(value.purpose as SetupPurpose) || !Array.isArray(value.steps) || !value.steps.every(isStep)
        || !Array.isArray(value.blockers) || !value.blockers.every(code => typeof code === 'string')
        || typeof value.can_start !== 'boolean' || (value.can_start && value.blockers.length > 0)
        || !Array.isArray(value.tcp_ports) || !Array.isArray(value.udp_ports)
        || [...value.tcp_ports, ...value.udp_ports].some(port => !Number.isInteger(port) || port < 1 || port > 65535)
        || value.preserve_ssh !== true || value.persist_firewall !== true
        || typeof value.contact_email !== 'string'
        || (value.remote_dns_connection !== undefined && (!isRecord(value.remote_dns_connection) || !identity(value.remote_dns_connection.id) || typeof value.remote_dns_connection.endpoint !== 'string' || !Array.isArray(value.remote_dns_connection.nameservers) || value.remote_dns_connection.nameservers.some(name => typeof name !== 'string')))
        || (value.hostname_change !== undefined && typeof value.hostname_change !== 'string')) return null;
    if (value.server_ip !== undefined && (typeof value.server_ip !== 'string' || value.server_ip.length > 64)) return null;
    if (value.infrastructure_dns !== undefined && !isInfrastructureDNSPlan(value.infrastructure_dns)) return null;
    if (value.components !== undefined && (!Array.isArray(value.components) || value.components.length > 80
        || value.components.some(item => !isRecord(item) || typeof item.id !== 'string' || typeof item.selected !== 'boolean' || typeof item.required !== 'boolean' || typeof item.installed !== 'boolean'))) return null;
    if (value.package_updates !== undefined && (!Array.isArray(value.package_updates) || value.package_updates.length > 400
        || value.package_updates.some(item => !isRecord(item) || typeof item.package !== 'string' || item.package.length > 128
            || (item.from !== undefined && typeof item.from !== 'string') || (item.to !== undefined && typeof item.to !== 'string')))) return null;
    if (value.package_updates_state !== undefined && !['known', 'unknown'].includes(String(value.package_updates_state))) return null;
    return value as unknown as ServerSetupPlan;
}
export function decodeSetupExecution(value: unknown, marker?: SetupStartMarker | null): ServerSetupExecution | null {
    if (!isRecord(value) || !identity(value.id) || !identity(value.request_id) || !identity(value.plan_id)
        || !['running', 'waiting', 'failed', 'succeeded'].includes(String(value.status))
        || typeof value.phase !== 'string' || !Array.isArray(value.steps)
        || !value.steps.every(step => isStep(step) && isRecord(step) && ['pending', 'running', 'failed', 'succeeded'].includes(String(step.status)))
        || (marker && (value.request_id !== marker.request_id || value.plan_id !== marker.plan_id))
        || (value.error !== undefined && (!isRecord(value.error) || typeof value.error.code !== 'string' || typeof value.error.message !== 'string'))
        || (value.checks !== undefined && (!Array.isArray(value.checks) || value.checks.some(check => !isRecord(check) || typeof check.id !== 'string' || !['ready', 'action_required', 'unknown'].includes(String(check.state)) || typeof check.code !== 'string')))
        || (value.panel_url !== undefined && typeof value.panel_url !== 'string')) return null;
    // Invalid optional metadata must not hide the durable execution.
    // Gecersiz istege bagli bilgi kalici islemi gizlememelidir.
    if (isRecord(value.error)) {
        const error = value.error;
        const optional = (key: string, limit: number) => error[key] === undefined || (typeof error[key] === 'string' && (error[key] as string).length <= limit);
        if (!optional('component', 64) || !optional('step', 32) || !optional('detail', 400) || !optional('reason', 64)) {
            const { component: _component, step: _step, detail: _detail, reason: _reason, ...kept } = error;
            value = { ...value, error: kept };
        }
    }
    if (!isRecord(value)) return null;
    // Only a succeeded run can be superseded; any other value is dropped, so a
    // malformed flag can never hide a run that is still in progress or failed.
    if (value.superseded !== undefined && (value.superseded !== true || value.status !== 'succeeded')) {
        const { superseded: _superseded, ...kept } = value;
        value = kept;
    }
    if (!isRecord(value)) return null;
    const context = value.context;
    if (context !== undefined && (!isRecord(context)
        || !['local', 'external', 'existing'].includes(String(context.dns_mode))
        || !(context.dns_mode === 'local' ? ['primary', 'secondary'] : ['', 'primary', 'secondary']).includes(String(context.dns_role))
        || (context.infrastructure_dns !== undefined && !isInfrastructureDNSPlan(context.infrastructure_dns))
        || (context.access_dns_ip !== undefined && (typeof context.access_dns_ip !== 'string' || context.access_dns_ip.length > 64))
        || ['dns_engine', 'local_nameserver', 'local_ip', 'peer_nameserver', 'peer_ip', 'panel_domain', 'mail_hostname', 'dns_hosting_management']
            .some(key => typeof context[key] !== 'string' || (context[key] as string).length > 1024))) {
        const { context: _invalid, ...execution } = value;
        return execution as unknown as ServerSetupExecution;
    }
    if (context !== undefined && context.peer_host_record !== undefined) {
        const peer = decodePeerHostRecord(context.peer_host_record);
        const { peer_host_record: _peer, ...rest } = context;
        value = { ...value, context: peer ? { ...rest, peer_host_record: peer } : rest };
    }
    return value as unknown as ServerSetupExecution;
}
// handover: the reviewed plan contained the panel certificate step, so the
// Panel restarts once during this setup (lib/panelHandover.ts).
export interface SetupStartMarker { request_id: string; plan_id: string; panel_domain: string; handover?: boolean; package_updates_digest?: string }
export function decodeSetupMarker(raw: string | null): SetupStartMarker | null {
    try {
        const value: unknown = JSON.parse(raw || 'null');
        return isRecord(value) && identity(value.request_id) && identity(value.plan_id) && typeof value.panel_domain === 'string'
            ? value as unknown as SetupStartMarker : null;
    } catch { return null; }
}
export function newSetupRequestID(): string {
    return Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('');
}
export function safeSetupPanelURL(raw: string | undefined, hostname: string): string | null {
    if (!raw || !hostname) return null;
    try {
        const url = new URL(raw);
        return url.protocol === 'https:' && url.hostname === hostname.toLowerCase().replace(/\.$/, '') && !url.username && !url.password
            ? url.href : null;
    } catch { return null; }
}

// A visible recorded wait permits an explicit owner action, never a polling POST.
export function continuableMailEnrollment(execution: ServerSetupExecution | null): SetupPlanStep | null {
    if (!execution || execution.status !== 'running' || !['server_setup_mail_enrollment_running', 'server_setup_mail_enrollment_rollback'].includes(execution.error?.code || '')) return null;
    return execution.steps.find(step => step.kind === 'mail_enrollment' && step.status === 'running' && step.id === execution.phase) || null;
}

export function retryableMailEnrollmentHandoff(execution: ServerSetupExecution | null): SetupPlanStep | null {
    if (!execution || execution.status !== 'running' || execution.error?.code !== 'server_setup_mail_enrollment_not_recorded') return null;
    return execution.steps.find(step => step.kind === 'mail_enrollment' && step.status === 'running' && step.id === execution.phase) || null;
}
