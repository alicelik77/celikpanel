import type { SetupEditorStep } from './serverSetup';
import { setupHostingRootBlockerValues } from './serverSetupGuidance';

// What stops a reviewed setup plan from starting, said for the owner (owner
// feedback 2026-10-31: "I looked at this page and did not understand the
// problem"). Each blocker code the Panel sends (cmd/panel/server_setup_operations.go,
// addBlocker) maps to one plain sentence of what is wrong
// (setup.problem.<name>.what), one concrete action (setup.problem.<name>.todo)
// and, where the wizard or another page fixes it, the fix: a wizard step to
// go back to, removing Email from the plan, reading the plan again, or My
// account. A code without an entry keeps its earlier sentence; a new blocker
// adds one line here and two keys in i18n/screens/problems.
//
// Kurulum planini durduran her engel: ne yanlis (tek sade cumle), ne
// yapilmali (tek somut eylem) ve mumkunse duzeltme (sihirbaz adimi, e-postayi
// plandan cikarma, yeniden kontrol, Hesabim).

export type SetupProblemFix =
    | { kind: 'step'; step: SetupEditorStep }
    | { kind: 'removeMail' }
    | { kind: 'recheck' }
    | { kind: 'account' };

export interface SetupProblemGuide {
    /** The catalogue prefix: `${key}.what` and `${key}.todo`. */
    key: string;
    values: Record<string, string>;
    fix?: SetupProblemFix;
    /** Component ids among the values, which the screen names. */
    components?: Array<'component' | 'other'>;
}

const step = (name: SetupEditorStep): SetupProblemFix => ({ kind: 'step', step: name });
const recheck: SetupProblemFix = { kind: 'recheck' };
const mailComponents = ['postfix', 'dovecot', 'rspamd', 'roundcube'];

// The code before its first ":" selects the entry.
const table: Record<string, { name: string; fix?: SetupProblemFix }> = {
    server_setup_extension_needs_second_server: { name: 'extensionNeedsSecond' },
    server_setup_panel_domain_invalid: { name: 'panelDomain', fix: step('names') },
    panel_certificate_contact_email_invalid: { name: 'contact', fix: { kind: 'account' } },
    server_setup_panel_certificate_unmanaged: { name: 'panelCertUnmanaged', fix: recheck },
    server_setup_dns_mode_unsupported: { name: 'dnsMode', fix: step('dnshome') },
    server_setup_remote_dns_unavailable: { name: 'remoteDNS', fix: step('dnshome') },
    server_setup_dns_purpose_requires_local: { name: 'dnsPurposeLocal', fix: step('dnshome') },
    server_setup_peer_join_answers_changed: { name: 'answersChanged', fix: step('dnshome') },
    server_setup_peer_mail_both: { name: 'peerMailBoth', fix: { kind: 'removeMail' } },
    server_setup_dns_identity_required: { name: 'dnsIdentity', fix: step('dnshome') },
    server_setup_existing_dns_requires_migration: { name: 'dnsMigration', fix: step('roles') },
    server_setup_dns_installed_differs: { name: 'dnsInstalledDiffers', fix: step('dnshome') },
    server_setup_pdns_main_untested_system: { name: 'pdnsMain', fix: step('dnshome') },
    server_setup_pdns_main_untested_version: { name: 'pdnsMain', fix: step('dnshome') },
    server_setup_pdns_main_not_debian_archive: { name: 'pdnsRepo', fix: step('dnshome') },
    server_setup_pdns_main_not_official_repository: { name: 'pdnsRepo', fix: step('dnshome') },
    server_setup_dns_publisher_endpoint_required: { name: 'publisherEndpoint', fix: step('dnshome') },
    host_restart_required: { name: 'hostRestart', fix: recheck },
    firewall_kernel_unavailable: { name: 'firewallKernel', fix: recheck },
    firewall_engine_unavailable: { name: 'firewallEngine', fix: recheck },
    firewall_busy: { name: 'firewallBusy', fix: recheck },
    firewall_status_unknown: { name: 'firewallUnknown', fix: recheck },
    server_setup_dns_engine_unsupported: { name: 'dnsEngineUnsupported', fix: step('dnshome') },
    server_setup_service_unknown: { name: 'serviceUnknown', fix: step('roles') },
    server_setup_service_unsupported: { name: 'serviceUnsupported', fix: step('components') },
    server_setup_dependency_missing: { name: 'dependencyMissing', fix: step('components') },
    server_setup_service_conflict: { name: 'serviceConflict', fix: step('components') },
    server_setup_version_repository_unknown: { name: 'versionRepoUnknown', fix: recheck },
    server_setup_version_repository_required: { name: 'versionRepoRequired', fix: step('components') },
    server_setup_main_dns_server_mismatch: { name: 'mainDNSMismatch', fix: step('dnshome') },
    server_setup_components_required: { name: 'componentsRequired', fix: step('roles') },
    server_setup_node_version_required: { name: 'nodeVersion', fix: step('components') },
    server_setup_database_invalid: { name: 'database', fix: step('components') },
    server_setup_purpose_invalid: { name: 'purpose', fix: step('roles') },
    server_setup_hosting_root_not_traversable: { name: 'hostingRoot', fix: recheck },
    server_setup_mail_hostname_invalid: { name: 'mailHostname', fix: step('names') },
    server_setup_panel_dns_address_unavailable: { name: 'panelAddress', fix: step('dnshome') },
    server_setup_infrastructure_dns_conflict: { name: 'infraConflict', fix: step('records') },
    server_setup_infrastructure_dns_imported_clash: { name: 'importClash', fix: step('records') },
    server_setup_infrastructure_dns_peer_mail_own_name: { name: 'peerMailName' },
    server_setup_infrastructure_dns_peer_mail_outside_zone: { name: 'peerMailName' },
    server_setup_panel_renewal_unavailable: { name: 'renewalUnknown', fix: recheck },
    server_setup_mail_certificate_unavailable: { name: 'mailCertUnavailable', fix: recheck },
    server_setup_mail_enrollment_busy: { name: 'mailEnrollBusy', fix: recheck },
    server_setup_mail_enrollment_unavailable: { name: 'mailEnrollUnknown', fix: recheck },
};

/** The plain-language guide for one plan blocker, or null when it has none yet. */
export function setupProblemGuide(code: string): SetupProblemGuide | null {
    const [base, ...rest] = code.split(':');
    const entry = table[base];
    if (!entry) return null;
    if (base === 'server_setup_hosting_root_not_traversable') {
        const values = setupHostingRootBlockerValues(code);
        return values ? { key: `setup.problem.${entry.name}`, values, fix: entry.fix } : null;
    }
    // server_setup_service_unsupported:<id>, ..._dependency_missing:<id>,
    // ..._service_conflict:<id>:<taken>, ..._version_repository_*:<id>.
    const component = rest[0] || '';
    if (base === 'server_setup_service_unsupported' && mailComponents.includes(component)) {
        return { key: 'setup.problem.mailUnsupported', values: {}, fix: { kind: 'removeMail' } };
    }
    if (['server_setup_service_unsupported', 'server_setup_dependency_missing', 'server_setup_version_repository_unknown', 'server_setup_version_repository_required'].includes(base)) {
        if (!component) return null;
        return { key: `setup.problem.${entry.name}`, values: { component }, fix: entry.fix, components: ['component'] };
    }
    if (base === 'server_setup_service_conflict') {
        if (!component || !rest[1]) return null;
        return { key: `setup.problem.${entry.name}`, values: { component, other: rest[1] }, fix: entry.fix, components: ['component', 'other'] };
    }
    return { key: `setup.problem.${entry.name}`, values: {}, fix: entry.fix };
}

/** Every blocker base code with a guide (tests pin each to both catalogues). */
export const setupProblemCodes = Object.keys(table);
export const setupProblemNames = [...new Set([...Object.values(table).map(entry => entry.name), 'mailUnsupported'])];

// Why "Start" cannot be pressed, said beside it: the problems above, the
// unread package list, the confirmation box, or a plan the Panel did not
// allow without a named blocker. target is the element the reason points to.
// Baslat neden basilamiyor: yanindaki kisa neden ve isaret ettigi yer.
export const setupProblemsID = 'setup-plan-problems';
export type SetupStartBlock =
    | { reason: 'problems'; count: number; target: string }
    | { reason: 'packages'; target: string }
    | { reason: 'plan'; target: string }
    | { reason: 'confirm'; target: string };
export function setupStartBlock(plan: { can_start: boolean; blockers: string[]; package_updates_state?: string } | null, acknowledged: boolean): SetupStartBlock | null {
    if (!plan) return null;
    if (plan.blockers.length > 0) return { reason: 'problems', count: plan.blockers.length, target: setupProblemsID };
    if (!plan.can_start && plan.package_updates_state === 'unknown') return { reason: 'packages', target: 'setup-package-updates' };
    if (!plan.can_start) return { reason: 'plan', target: 'setup-plan-title' };
    if (!acknowledged) return { reason: 'confirm', target: 'setup-confirm' };
    return null;
}
