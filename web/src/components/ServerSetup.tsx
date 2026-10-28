import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ArrowRight, Check, Circle, Loader2 } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import { setupDNSTranslation } from '../i18n/setupDNS';
import type { TranslationKey } from '../i18n/en';
import { Link, Navigate } from '../router';
import { decodeSetupEditorCheckpoint, changeSetupWizardDNSRole, setupJoinPendingDraft, setupDNSNames, setupDetectedIPv4, decodeServerSetup, setupEditorKey, setupExtensionChanges, setupNextPath, normalizeSetupDomain, validSetupDomain, validSetupShortName, setupNameWithin, setupInitialMainDomain, setupProposedNames, setupApplyProposedNames, type ServerSetupCheck, type ServerSetupDraft, type ServerSetupSnapshot, type SetupEditorStep, type SetupExtensionChangeKey, type SetupProposedNames } from '../lib/serverSetup';
import { retryableMailEnrollmentHandoff, continuableMailEnrollment, decodeSetupExecution, decodeSetupMarker, decodeSetupPlan, newSetupRequestID, safeSetupPanelURL, type ServerSetupExecution, type SetupInfrastructureDNSPlan, type ServerSetupPlan, type SetupStartMarker } from '../lib/serverSetupOperation';
import { ServerSetupShell, useServerSetup } from './ServerSetupGate';
import { ServerSetupSteps } from './ServerSetupSteps';
import { ServerSetupDNSConnection } from './ServerSetupDNSConnections';
import { remoteDNSEndpoint } from '../lib/remoteDNS';
import { ServerSetupChoice, ServerSetupManualAction } from './ServerSetupChoice';
import { Button, inputClass, KeepWords, Spinner } from './ui';
import { SetupCountStep, SetupDNSHomeStep, SetupDomainStep, SetupJoinCard, SetupJoinPanel, SetupJoinedSummary, SetupMailStep, SetupOpenTasks, SetupRecordsStep, SetupNameStep, SetupNamesStep, SetupPanelAccessCard, SetupProviderRecordsCard, SetupRegistrarCard, SetupRolesStep, SetupWizardInput, setupEngineChoice, setupServerLabel, useSetupPeerLink } from './ServerSetupWizardSteps';
import { AddressLink } from './AddressLink';
import { ServerSetupComponents, useSetupComponentCatalog } from './ServerSetupComponents';
import { setupCheckLine, setupComponentName, setupExecutionGuidance, setupHostBusyGuidance, setupHostingRootBlockerValues, setupPlanReopened, setupStepState, setupStepStateLabel, type SetupGuidanceText } from '../lib/serverSetupGuidance';
import { applySetupRoles, resetSetupCustomization, setupCustomized, setupDraftRoles, setupEffectiveComponents, setupRolesPreset } from '../lib/serverSetupComponents';
import { componentName } from '../lib/componentName';
import { handoverAddress, handoverSettled, plannedHandoverDrop, setupHandover, setupStartMarkerKey } from '../lib/panelHandover';
import { startVisibleReads } from '../lib/visibleReads';

function useSetupI18n() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => setupDNSTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}

type Step = SetupEditorStep | 'progress';
const requestOptions = (body: unknown) => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
async function setupFetch(url: string, options?: RequestInit) {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 20000);
    try { return await fetch(url, { ...options, signal: controller.signal }); }
    finally { window.clearTimeout(timeout); }
}
const editorKey = setupEditorKey;
const markerKey = setupStartMarkerKey;
// A mail component this distribution cannot install gets the mail-specific
// refusal: what is missing, who acts and the two ways forward (upd1 P2).
const mailServiceUnsupported = (code: string) => /^server_setup_service_unsupported:(postfix|dovecot|rspamd|roundcube)$/.test(code);
const codeKey: Record<string, TranslationKey> = {
    license_required: 'license.restricted',
    service_install_failed: 'setup.failure.install',
    mail_profile_install_failed: 'setup.failure.install',
    node_runtime_install_failed: 'setup.failure.install',
    server_setup_build_changed: 'setup.guide.buildChanged',
    server_setup_reconciling: 'setup.confirmingPrevious',
    REMOTE_DNS_UNAVAILABLE: 'setup.blocker.remote',
    REMOTE_DNS_AUTHORITY_NOT_READY: 'setup.remote.authorityNotReady',
    dns_peer_ipv6_unverified: 'setup.blocker.peerIPv6',
    mail_identity_required: 'setup.blocker.mailIdentity',
    mail_identity_unavailable: 'setup.blocker.mailIdentity',
    mail_delivery_required: 'setup.blocker.mailDelivery',
    mail_delivery_unavailable: 'setup.blocker.mailDelivery',
    server_setup_mail_certificate_failed: 'setup.blocker.mailDNS',
    server_setup_mail_enrollment_unavailable: 'setup.blocker.mailEnrollment',
    server_setup_mail_enrollment_busy: 'setup.blocker.mailEnrollmentBusy',
    server_setup_dns_identity_required: 'setup.blocker.dnsIdentity',
    server_setup_hosting_requires_dns_publisher: 'setup.publisher.endpointRequired',
    server_setup_main_dns_server_mismatch: 'setup.v2.blocker.mainDNS',
    server_setup_infrastructure_dns_imported_clash: 'setup.v3.blocker.importClash',
    server_setup_peer_join_answers_changed: 'setup.v5.blocker.answersChanged',
    server_setup_peer_names_clash: 'setup.v5.wait.server_setup_peer_names_clash',
    server_setup_peer_names_outside_domain: 'setup.v5.wait.server_setup_peer_names_outside_domain',
    server_setup_peer_names_invalid: 'setup.v5.wait.server_setup_peer_names_invalid',
    server_setup_peer_names_bound: 'setup.v5.wait.server_setup_peer_names_bound',
    server_setup_peer_mail_both: 'setup.v5.wait.server_setup_peer_mail_both',
    server_setup_peer_link_missing: 'setup.v5.wait.server_setup_peer_link_missing',
    server_setup_peer_version_incompatible: 'setup.v5.wait.server_setup_peer_version_incompatible',
    server_setup_dns_publisher_refused: 'setup.v5.wait.server_setup_dns_publisher_refused',
    server_setup_peer_join_waiting: 'setup.v5.wait.server_setup_peer_join_waiting',
    server_setup_peer_names_waiting: 'setup.v5.wait.server_setup_peer_names_waiting',
    server_setup_peer_join_changed: 'setup.v5.wait.server_setup_peer_join_changed',
    server_setup_peer_names_unreachable: 'setup.v5.wait.server_setup_peer_names_unreachable',
    server_setup_peer_names_unknown: 'setup.v5.wait.server_setup_peer_names_unknown',
    server_setup_dns_publisher_waiting: 'setup.v5.wait.server_setup_dns_publisher_waiting',
    server_setup_dns_publisher_local_waiting: 'setup.v5.wait.server_setup_dns_publisher_local_waiting',
    server_setup_peer_dkim_zone_waiting: 'setup.v5.wait.server_setup_peer_dkim_zone_waiting',
    server_setup_peer_dkim_publishing: 'setup.v5.wait.server_setup_peer_dkim_publishing',
    server_setup_peer_dkim_unreachable: 'setup.v5.wait.server_setup_peer_dkim_unreachable',
    server_setup_peer_dkim_unknown: 'setup.v5.wait.server_setup_peer_dkim_unknown',
    server_setup_join_certificate_unavailable: 'setup.v5.wait.server_setup_join_certificate_unavailable',
    server_setup_pdns_main_untested_system: 'setup.v2.engine.untested',
    server_setup_version_repository_required: 'setup.v2.blocker.versionRepository',
    server_setup_version_repository_unknown: 'setup.v2.blocker.versionRepositoryUnknown',
    server_setup_dns_publisher_endpoint_required: 'setup.publisher.endpointRequired',
    server_setup_dns_publisher_required: 'setup.publisher.waitingHelp',
    server_setup_dns_readiness_required: 'setup.publisher.pairWaiting',
    server_setup_dns_engine_unsupported: 'setup.blocker.profile',
    server_setup_existing_dns_requires_migration: 'setup.blocker.migration',
    pdns_primary_switch_paused: 'setup.pdnsPrimaryPaused',
    server_setup_dns_failed: 'setup.blocker.dnsIdentity',
    server_setup_dns_rolled_back: 'setup.guide.dnsRolledBack',
    server_setup_dns_agent_failed: 'setup.guide.dnsAgentFailed',
    server_setup_dns_recovery_held: 'setup.guide.dnsRecoveryHeld',
    server_setup_dns_result_unknown: 'setup.guide.dnsResultUnknown',
    LICENSE_VERIFICATION_UNAVAILABLE: 'setup.guide.licenseUnverified',
    LICENSE_STATUS_UNAVAILABLE: 'setup.guide.licenseUnverified',
    server_setup_access_dns_required: 'setup.infrastructure.accessRequired',
    server_setup_access_dns_mismatch: 'setup.infrastructure.accessMismatch',
    server_setup_primary_dns_required: 'setup.infrastructure.primaryRequired',
    server_setup_infrastructure_dns_unknown: 'setup.infrastructure.unknown',
    server_setup_infrastructure_dns_invalid: 'setup.infrastructure.invalid',
    server_setup_infrastructure_dns_conflict: 'setup.infrastructure.conflict',
    server_setup_infrastructure_dns_changed: 'setup.infrastructure.conflict',
    server_setup_infrastructure_dns_peer_mail_own_name: 'setup.infrastructure.peerMailOwnName',
    server_setup_infrastructure_dns_peer_mail_outside_zone: 'setup.infrastructure.peerMailOutsideZone',
    server_setup_infrastructure_dns_failed: 'setup.blocker.dnsIdentity',
    server_setup_access_dns_failed: 'setup.infrastructure.accessRequired',
    server_setup_panel_certificate_failed: 'setup.blocker.panelTLS',
    server_setup_firewall_failed: 'setup.blocker.firewall',
    host_restart_required: 'setup.blocker.hostRestart',
    firewall_kernel_unavailable: 'setup.blocker.firewallKernel',
    firewall_engine_unavailable: 'setup.blocker.firewallEngine',
    firewall_busy: 'setup.blocker.firewallBusy',
    firewall_status_unknown: 'setup.blocker.firewallUnknown',
    server_setup_step_failed: 'setup.blocker.services',
    firewall_no_ssh_service: 'setup.blocker.ssh',
    firewall_ssh_not_listening: 'setup.blocker.ssh',
    firewall_ssh_unprovable: 'setup.blocker.ssh',
    firewall_no_engine: 'setup.blocker.firewall',
    server_setup_remote_dns_required: 'setup.blocker.remote',
    remote_dns_not_ready: 'setup.blocker.remote',
    server_setup_panel_domain_invalid: 'setup.blocker.panelDomain',
    panel_certificate_contact_email_invalid: 'setup.blocker.contact',
    server_setup_panel_certificate_unmanaged: 'setup.blocker.panelTLS',
    server_setup_panel_renewal_unavailable: 'setup.blocker.renewal',
    server_setup_dns_mode_unsupported: 'setup.blocker.existing',
    server_setup_dns_purpose_requires_local: 'setup.blocker.dnsIdentity',
    server_setup_service_unknown: 'setup.blocker.services',
    server_setup_inventory_unavailable: 'setup.components.inventoryUnknown',
    server_setup_components_required: 'setup.components.chooseRequired',
    server_setup_customization_invalid: 'setup.components.savedUnavailable',
    server_setup_service_unsupported: 'setup.blocker.profile',
    server_setup_dependency_missing: 'setup.blocker.services',
    server_setup_service_conflict: 'setup.blocker.services',
    server_setup_node_version_required: 'setup.blocker.node',
    server_setup_database_invalid: 'setup.blocker.database',
    server_setup_purpose_invalid: 'setup.blocker.profile',
    server_setup_mail_hostname_invalid: 'setup.blocker.mailHostname',
    panel_https_required: 'setup.blocker.panelTLS',
    panel_https_unavailable: 'setup.blocker.panelTLS',
    panel_renewal_required: 'setup.blocker.renewal',
    panel_renewal_unavailable: 'setup.blocker.renewal',
    firewall_required: 'setup.blocker.firewall',
    firewall_unavailable: 'setup.blocker.firewall',
    dns_required: 'setup.blocker.dnsIdentity',
    dns_unavailable: 'setup.blocker.dnsIdentity',
    services_required: 'setup.blocker.services',
    services_unavailable: 'setup.blocker.services',
    mail_tls_required: 'setup.blocker.mailDNS',
    mail_tls_unavailable: 'setup.blocker.mailDNS',
    setup_existing_dns_not_supported: 'setup.blocker.existing',
    existing_dns_not_supported: 'setup.blocker.existing',
    existing_dns_unavailable: 'setup.blocker.existing',
    panel_domain_required: 'setup.blocker.panelDomain',
    panel_dns_not_ready: 'setup.blocker.panelDNS',
    panel_certificate_untrusted: 'setup.blocker.panelTLS',
    panel_certificate_not_ready: 'setup.blocker.panelTLS',
    panel_renewal_not_ready: 'setup.blocker.renewal',
    firewall_not_ready: 'setup.blocker.firewall',
    dns_identity_required: 'setup.blocker.dnsIdentity',
    dns_redundancy_required: 'setup.blocker.redundancy',
    mail_hostname_required: 'setup.blocker.mailHostname',
    mail_dns_not_ready: 'setup.blocker.mailDNS',
    services_not_ready: 'setup.blocker.services',
    profile_not_supported: 'setup.blocker.profile',
    operation_running: 'setup.blocker.operation',
};

export function ServerSetup() {
    const setup = useServerSetup();
    const [chosen, setChosen] = useState<ServerSetupSnapshot | null>(null);
    const [manual, setManual] = useState(false);
    if (!setup?.snapshot) return <ServerSetupShell><Spinner /></ServerSetupShell>;
    if (manual) return <Navigate to="/" replace />;
    const snapshot = chosen || setup.snapshot;
    if (['undecided', 'manual'].includes(snapshot.guidance || '') && !['running', 'waiting', 'ready'].includes(snapshot.status) && !snapshot.extension) {
        return <ServerSetupShell><ServerSetupChoice snapshot={snapshot} onChosen={next => {
            setup.accept(next); setChosen(next); setManual(next.guidance === 'manual');
        }} /></ServerSetupShell>;
    }
    // Beginning or ending an extension of a completed setup remounts the
    // wizard: it opens on the purpose step, or returns to the ready screen.
    // Genisletmenin baslamasi ya da bitmesi sihirbazi yeniden kurar.
    return <SetupWizard key={snapshot.extension ? 'extension' : 'setup'} initial={snapshot} />;
}

const extensionLabels: Record<SetupExtensionChangeKey, TranslationKey> = {
    purpose: 'setup.dashboardReady', panel_domain: 'setup.panelDomain', mail_hostname: 'setup.mailHostname',
    dns_mode: 'setup.extend.field.dnsMode', node_version: 'setup.nodeVersion', database: 'setup.database',
    dns_hosting_management: 'setup.publisher.management',
};
const extensionLiteral = (key: SetupExtensionChangeKey) => ['panel_domain', 'mail_hostname', 'node_version'].includes(key);

// After the owner's check saw every check pass, the server completes setup on
// its next pass, normally within seconds. Past this bound the page stops
// showing it as in progress and says the result is not known (D-024).
// Sahibin basarili kontrolunden sonra tamamlanma; bu sureden sonra sonuc
// bilinmiyor denir, sonsuz bekleme gosterilmez.
const setupFinishingLimit = 60_000;
// The page keeps reading a setup it could not reach every few seconds. Past
// this bound it stops showing that as a short reconnect and says the result is
// not known and what the owner can check (D-024, owner rule 2026-10-14).
// Ulasilamayan kurulum okunmaya devam eder; bu sureden sonra sonuc bilinmiyor denir.
const setupReconnectLimit = 60_000;
// A continuation that no read has confirmed stops being shown as in flight
// after this bound; the button is offered again (the server answers a repeat
// for the same request without starting a second operation).
// Dogrulanmayan devam istegi bu sureden sonra yeniden sunulur.
const enrollmentSettleLimit = 15_000;
const setupChecksPassed = (checks: ServerSetupCheck[]) => checks.some(check => !check.advisory) && checks.every(check => check.state === 'ready' || check.advisory);

function SetupWizard({ initial }: { initial: ServerSetupSnapshot }) {
    const setup = useServerSetup()!;
    const { user } = useAuth();
    const { t } = useSetupI18n();
    const { catalog, failed: catalogFailed, reload: reloadCatalog } = useSetupComponentCatalog(!['running', 'waiting', 'ready'].includes(initial.status));
    const [snapshot, setSnapshot] = useState(initial);
    const [restoredEditor] = useState(() => {
        try { return decodeSetupEditorCheckpoint(sessionStorage.getItem(editorKey(user.username)), initial); }
        catch { return null; }
    });
    // The wizard's own answers start from the saved draft: the main domain (or
    // the zone an earlier setup prepared), two servers (recommended) and the
    // host name's suggestion for the short name. / Ilk degerler.
    const [draft, setDraft] = useState<ServerSetupDraft>(() => restoredEditor?.draft || {
        ...initial.draft,
        main_domain: setupInitialMainDomain(initial.draft) || undefined,
        server_count: initial.draft.server_count ?? 2,
        short_name: initial.draft.short_name ?? (initial.short_name_suggestion || undefined),
    });
    const proposalRef = useRef<SetupProposedNames | null>(null);
    const [componentsOpen, setComponentsOpen] = useState(false);
    const restoreReview = useRef(restoredEditor?.step === 'review');
    const restoredSnapshot = useRef(initial);
    const localIPTouched = useRef(false);
    const detectedIP = setupDetectedIPv4(snapshot.server_ip);
    useEffect(() => {
        if (!detectedIP || localIPTouched.current || !['new', 'legacy', 'draft'].includes(snapshot.status)) return;
        setDraft(previous => previous.local_ip ? previous : { ...previous, local_ip: detectedIP });
    }, [detectedIP, snapshot.status]);
    const dnsNames = setupDNSNames(draft);
    // A second server that joined the first by code (alpha.114): its shared
    // answers come from the join. / Kodla katilmis ikinci sunucu.
    const peer = useSetupPeerLink(!['ready'].includes(initial.status));
    const joinedLink = peer.link;
    const [step, setStep] = useState<Step>(restoredEditor?.step || (initial.status === 'new' || initial.status === 'legacy' || (initial.extension && initial.status === 'draft') || !initial.draft.main_domain ? 'domain' : 'dnshome'));
    const [plan, setPlan] = useState<ServerSetupPlan | null>(null);
    const [execution, setExecution] = useState<ServerSetupExecution | null>(null);
    const [marker, setMarker] = useState<SetupStartMarker | null>(() => {
        try { return decodeSetupMarker(localStorage.getItem(markerKey(user.username))); } catch { return null; }
    });
    const [resolving, setResolving] = useState(true);
    const [busy, setBusy] = useState(false);
    const [acknowledged, setAcknowledged] = useState(false);
    const [remoteVerified, setRemoteVerified] = useState(false);
    const [error, setError] = useState('');
    const [reconnecting, setReconnecting] = useState(false);
    const [reconnectSlow, setReconnectSlow] = useState(false);
    const [enrollmentNotice, setEnrollmentNotice] = useState<'checking' | 'unknown' | null>(null);
    // The mail enrollment continuation the owner pressed, from the press until
    // a read shows that operation moved on (set18 O3): the button stays
    // disabled and says it is checking, so it is not pressed again meanwhile.
    // Basilan devam istegi, islem ilerleyene dek; dugme bu surede kapalidir.
    const [enrollmentSent, setEnrollmentSent] = useState<{ execution: string; step: string; code: string } | null>(null);
    const [completionFailed, setCompletionFailed] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{ checks: ServerSetupCheck[]; failed: boolean } | null>(null);
    const [verifyingRequirements, setVerifyingRequirements] = useState(false);
    // The answer to "Check requirements again" opens under the button, which
    // can be the last thing on the screen: it is brought into view when it
    // arrives (2026-10-16). / Kontrolun cevabi geldiginde gorunur alana alinir.
    const verificationBox = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (verificationResult && !verifyingRequirements) verificationBox.current?.scrollIntoView?.({ block: 'nearest' });
    }, [verificationResult, verifyingRequirements]);
    // checks: the run's own last recorded checks when the owner's check passed.
    // A newer unmet result from the run itself ends the finishing state.
    const [finishingRun, setFinishingRun] = useState<{ checks: string } | null>(null);
    const [finishingSlow, setFinishingSlow] = useState(false);
    // Every check passed on the owner's check; the server is completing setup.
    const finishing = !!finishingRun && !finishingSlow && execution?.status === 'waiting' && execution.phase === 'verification';
    const [manualExit, setManualExit] = useState(false);
    const [extendBusy, setExtendBusy] = useState(false);
    const [extendError, setExtendError] = useState<TranslationKey | null>(null);
    const pendingRef = useRef(false);
    const pollingRef = useRef(false);
    const pollEpoch = useRef(0);
    const alive = useRef(true);
    const heading = useRef<HTMLHeadingElement>(null);
    const markerRef = useRef(marker);
    markerRef.current = marker;
    const accept = useCallback((value: ServerSetupSnapshot) => { setSnapshot(value); setup.accept(value); }, [setup.accept]);
    const failureText = (code: string, message?: string, reason?: string) => {
        const hostingRoot = setupHostingRootBlockerValues(code);
        if (hostingRoot) return t('setup.blocker.hostingRoot', hostingRoot);
        const busy = setupHostBusyGuidance({ code, message, reason });
        if (busy) return `${t(busy.title)}. ${t(busy.body)}`;
        return t(code === 'mail_enrollment_restored' ? 'setup.guide.mailEnrollmentFailed' : mailServiceUnsupported(code) ? 'setup.blocker.mailUnsupported' : codeKey[code.split(':')[0]] || 'setup.blocker.unknown');
    };
    // A guidance line, with the check it names translated into {check}.
    const say = (message: SetupGuidanceText) => t(message.key, message.name ? { ...message.values, check: t(message.name) } : message.values);
    // An open check in the same typed sentence the run's guidance uses (2026-10-16).
    const checkSentence = (check: ServerSetupCheck) => { const line = setupCheckLine(check); return line.text ? say(line.text) : failureText(check.code); };
    // One localized name per component, shared by the step list and the
    // install-failure guidance so both sentences name the same thing.
    // Adım listesi ve kurulum hatası yönlendirmesi aynı yerel adı kullanır.
    const componentLabel = (target: string) => {
        if (['webmail', 'core-mail', 'protected-mail'].includes(target)) return t(target === 'protected-mail' ? 'dashboard.audit.profile.protectedMail' : target === 'core-mail' ? 'dashboard.audit.profile.coreMail' : 'dashboard.audit.profile.webmail');
        if (target === 'cron') return t('setup.component.cron');
        return componentName({ id: target, name: setupComponentName(target) }, t);
    };
    const stepTarget = (kind: string, target: string) => {
        if (kind === 'dns' && ['local', 'external', 'existing'].includes(target)) return t(`setup.dns.${target}` as TranslationKey);
        if (kind === 'mail_profile') return t(target === 'protected-mail' ? 'dashboard.audit.profile.protectedMail' : target === 'core-mail' ? 'dashboard.audit.profile.coreMail' : 'dashboard.audit.profile.webmail');
        return componentLabel(target);
    };

    useEffect(() => {
        heading.current?.focus();
    }, [step]);
    useEffect(() => {
        try {
            if (step === 'progress' || manualExit || snapshot.status === 'ready') {
                sessionStorage.removeItem(editorKey(user.username));
            } else if (['new', 'legacy', 'draft'].includes(snapshot.status)) {
                sessionStorage.setItem(editorKey(user.username), JSON.stringify({
                    version: 1, revision: snapshot.revision, step, draft,
                }));
            }
        } catch { /* Browser storage may be unavailable; normal setup still works. */ }
    }, [step, draft, snapshot.revision, snapshot.status, manualExit, user.username]);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

    async function readExecution() {
        const current = markerRef.current;
        const response = await setupFetch(`/api/v1/setup/operation${current ? `?request_id=${encodeURIComponent(current.request_id)}` : ''}`, { cache: 'no-store' });
        if (!response.ok) throw new Error('operation unavailable');
        const value: unknown = await response.json();
        if (value === null) {
            if (current) throw new Error('unconfirmed start');
            return null;
        }
        const next = decodeSetupExecution(value, current);
        if (!next) throw new Error('operation identity');
        // The completed setup's own run while that setup is being extended
        // (set18 D1): not this wizard's operation, and not a completion to
        // verify. The extension's wizard is shown; a start marker left from
        // that finished run is dropped. Once the extension starts, its own run
        // is read. / Genisletilen kurulumun eski calismasi gosterilmez.
        if (next.superseded) {
            if (current && markerRef.current === current) {
                try { localStorage.removeItem(markerKey(user.username)); } catch { /* the marker only selects what to read */ }
                markerRef.current = null; setMarker(null);
            }
            return null;
        }
        return next;
    }
    const reconcile = useCallback(async () => {
        if (pollingRef.current) return;
        pollingRef.current = true;
        const epoch = pollEpoch.current;
        try {
            const next = await readExecution();
            if (!alive.current || epoch !== pollEpoch.current) return;
            // Rebuild the review after a remount/reload, only after ruling out
            // an accepted execution. Never restore the confirmation checkbox.
            // Kabul edilmis islem yoksa incelemeyi yeniden kur; baslatma
            // onayini geri yukleme.
            if (!next && restoreReview.current) {
                restoreReview.current = false;
                try {
                    const response = await setupFetch('/api/v1/setup/plan', requestOptions({ revision: restoredSnapshot.current.revision }));
                    const reviewed = response.ok ? decodeSetupPlan(await response.json(), restoredSnapshot.current.revision) : null;
                    if (!reviewed || (restoredSnapshot.current.draft.dns_mode === 'existing' && reviewed.can_start
                        && reviewed.remote_dns_connection?.id !== restoredSnapshot.current.draft.remote_dns_connection_id)) throw new Error('review unavailable');
                    if (!alive.current || epoch !== pollEpoch.current) return;
                    setPlan(reviewed); setAcknowledged(false);
                } catch {
                    if (!alive.current || epoch !== pollEpoch.current) return;
                    setStep('dnshome'); setError(t('setup.planFailed'));
                }
            }
            setExecution(next); setResolving(false); setReconnecting(false);
            if (next) {
                setStep('progress');
                if (next.status === 'succeeded') {
                    try {
                        const state = await setup.reload(true);
                        if (!alive.current) return;
                        accept(state);
                        setCompletionFailed(state.status !== 'ready');
                        if (state.status === 'ready') localStorage.removeItem(markerKey(user.username));
                    } catch { if (alive.current) setCompletionFailed(true); }
                }
            }
        } catch {
            if (alive.current && epoch === pollEpoch.current) { setResolving(false); setReconnecting(true); }
        } finally { pollingRef.current = false; }
    }, [setup.reload, accept, user.username]);
    useEffect(() => {
        void reconcile();
        void setup.reload(true).then(value => { if (alive.current) setSnapshot(value); }).catch(() => {});
    }, [reconcile, setup.reload]);
    useEffect(() => {
        if (snapshot.status === 'ready' || (!reconnecting && !marker && !execution)) return;
        if (execution?.status === 'failed' && !reconnecting) return;
        // Read more often while setup is finishing, so completion shows promptly.
        // Only while the tab is visible; shown again, it reads once at once
        // (2026-10-15). Nothing here depends on reading while hidden: the
        // operation runs on the server, its record is kept there, and the
        // 60 s notices only change wording.
        return startVisibleReads(reconcile, finishing ? 1500 : 3000, {
            setInterval: (callback, ms) => window.setInterval(callback, ms),
            clearInterval: (id) => window.clearInterval(id),
        });
    }, [execution, marker, reconnecting, resolving, reconcile, snapshot.status, finishing]);
    // Finishing ends with the run's own next state: it left the final check, or
    // it recorded a newer result with a requirement unmet again.
    // Calisma son kontrolden cikinca ya da yeni bir karsilanmamis sonuc kaydedince biter.
    useEffect(() => {
        if (!finishingRun || !execution) return;
        const waitingFinal = execution.status === 'waiting' && execution.phase === 'verification';
        const checks = execution.checks || [];
        const unmetAgain = waitingFinal && JSON.stringify(checks) !== finishingRun.checks && !setupChecksPassed(checks);
        if (waitingFinal && !unmetAgain) return;
        setFinishingRun(null); setFinishingSlow(false);
        if (unmetAgain) setVerificationResult(null);
    }, [execution, finishingRun]);
    useEffect(() => {
        if (!finishingRun) return;
        const timer = window.setTimeout(() => setFinishingSlow(true), setupFinishingLimit);
        return () => window.clearTimeout(timer);
    }, [finishingRun]);
    // The press is settled by the operation itself: another run, another state
    // of the same step, or the step no longer running (set18 O3).
    // Devam istegi islemin kendi yeni durumuyla sonuclanir.
    useEffect(() => {
        if (!enrollmentSent) return;
        const same = !!execution && execution.id === enrollmentSent.execution && (execution.error?.code || '') === enrollmentSent.code
            && execution.steps.some(item => item.id === enrollmentSent.step && item.status === 'running');
        if (!same) setEnrollmentSent(null);
    }, [execution, enrollmentSent]);
    useEffect(() => {
        if (!enrollmentSent) return;
        const timer = window.setTimeout(() => setEnrollmentSent(null), enrollmentSettleLimit);
        return () => window.clearTimeout(timer);
    }, [enrollmentSent]);
    // Reconnecting is shown as active for a bounded time; the reads go on after it.
    // Yeniden baglanma sinirli sure etkin gosterilir; okumalar sonra da surer.
    useEffect(() => {
        setReconnectSlow(false);
        if (!reconnecting) return;
        const timer = window.setTimeout(() => setReconnectSlow(true), setupReconnectLimit);
        return () => window.clearTimeout(timer);
    }, [reconnecting]);
    // Once the certificate handover is behind this setup, drop its flag from
    // the saved marker: a later restart is then not explained as the handover.
    // Devir geride kalinca isaret kaldirilir; sonraki yeniden baslatma devir sayilmaz.
    useEffect(() => {
        if (!marker?.handover || !handoverSettled(execution)) return;
        const { handover: _handover, ...kept } = marker;
        try { localStorage.setItem(markerKey(user.username), JSON.stringify(kept)); } catch { /* the flag only selects recovery wording */ }
        setMarker(kept);
    }, [execution, marker, user.username]);

    function change<K extends keyof ServerSetupDraft>(key: K, value: ServerSetupDraft[K]) {
        if (key === 'local_ip') localIPTouched.current = true;
        if (key === 'dns_mode' || key === 'remote_dns_connection_id') setRemoteVerified(false);
        setDraft(previous => ({ ...previous, [key]: value, ...(key === 'dns_mode' && value !== 'local' ? { infrastructure_dns: undefined } : {}) })); setPlan(null); setAcknowledged(false); setError('');
    }
    function updateDraft(update: (previous: ServerSetupDraft) => ServerSetupDraft) {
        setDraft(update); setPlan(null); setAcknowledged(false); setError('');
    }
    // DNS home: unticking the DNS role is the external provider; the main DNS
    // server answer follows the role. / DNS yeri ve gorev.
    function chooseDNSMode(mode: ServerSetupDraft['dns_mode']) {
        setRemoteVerified(false);
        updateDraft(previous => {
            const next: ServerSetupDraft = { ...previous, dns_mode: mode };
            if (mode === 'local' && next.dns_role === 'primary') next.main_dns_server = 'this';
            else { delete next.main_dns_server; }
            if (mode !== 'local') delete next.infrastructure_dns;
            return next;
        });
    }
    async function saveDraft(): Promise<ServerSetupSnapshot> {
        // The new flow never sends a typed peer: the second server joins by
        // code; a joined second server keeps what the join gave it.
        const sent = joinedLink ? draft : setupJoinPendingDraft(draft);
        const response = await setupFetch('/api/v1/setup', { ...requestOptions({ revision: snapshot.revision, draft: { ...sent, ...(sent.main_domain ? { main_domain: normalizeSetupDomain(sent.main_domain) } : {}), ...(sent.short_name !== undefined ? { short_name: sent.short_name.trim().toLowerCase() || undefined } : {}), ...(secondaryHosting && !joinFlow ? { dns_hosting_management: hostingDNSManagement, ...(hostingDNSManagement === 'manual' ? { dns_publisher_endpoint: '' } : {}) } : {}) } }), method: 'PUT' });
        if (response.status === 409) throw new Error(t('setup.conflict'));
        if (!response.ok) throw new Error(t('setup.saveFailed'));
        const next = decodeServerSetup(await response.json());
        if (!next) throw new Error(t('setup.loadFailed'));
        if (alive.current) { accept(next); setDraft(next.draft); }
        return next;
    }
    async function next(event: FormEvent) {
        event.preventDefault();
        const problem = stepProblem();
        if (problem) { setError(t(problem)); return; }
        if (pendingRef.current) return;
        pendingRef.current = true; setBusy(true); setError('');
        try {
            const saved = await saveDraft();
            if (!alive.current) return;
            if (step !== lastEdit) {
                const after = steps[steps.indexOf(step) + 1] as Step;
                // Names are proposed from the main domain and the short name
                // when the names step opens; a name the owner typed is kept.
                if (after === 'names' || after === 'dnshome') proposeNames();
                setStep(after);
                return;
            }
            const response = await setupFetch('/api/v1/setup/plan', requestOptions({ revision: saved.revision }));
            if (!response.ok) throw new Error(t('setup.planFailed'));
            const reviewed = decodeSetupPlan(await response.json(), saved.revision);
            if (!reviewed || (saved.draft.dns_mode === 'existing' && reviewed.can_start && reviewed.remote_dns_connection?.id !== saved.draft.remote_dns_connection_id)) throw new Error(t('setup.planFailed'));
            if (alive.current) { setPlan(reviewed); setAcknowledged(false); setStep('review'); }
        } catch (failure) { if (alive.current) setError(failure instanceof Error ? failure.message : t('setup.saveFailed')); }
        finally { pendingRef.current = false; if (alive.current) setBusy(false); }
    }
    async function start() {
        if (!plan?.can_start || !acknowledged || pendingRef.current) return;
        pendingRef.current = true; setBusy(true); setError('');
        let started: SetupStartMarker;
        try {
            started = { request_id: newSetupRequestID(), plan_id: plan.id, panel_domain: draft.panel_domain, ...(plan.steps.some(item => item.kind === 'panel_certificate') ? { handover: true } : {}) };
            localStorage.setItem(markerKey(user.username), JSON.stringify(started));
        } catch { setError(t('setup.storageFailed')); pendingRef.current = false; setBusy(false); return; }
        markerRef.current = started; setMarker(started); setStep('progress');
        try {
            const response = await setupFetch('/api/v1/setup/start', requestOptions({ plan_id: started.plan_id, request_id: started.request_id, confirmed: true }));
            if (!response.ok) {
                const problem = await response.clone().json().catch(() => null);
                if (response.status === 409 && ['server_setup_review_required', 'server_setup_review_stale', 'server_setup_plan_blocked'].includes(problem?.code)) {
                    // These codes are emitted only before an execution is created.
                    // The server checks a previously committed request first.
                    localStorage.removeItem(markerKey(user.username));
                    markerRef.current = null; setMarker(null); setPlan(null); setAcknowledged(false);
                    setStep('dnshome'); setError(t('setup.conflict')); return;
                }
                await reconcile(); return;
            }
            const accepted = decodeSetupExecution(await response.json(), started);
            if (!accepted) { await reconcile(); return; }
            if (alive.current) { setExecution(accepted); setReconnecting(false); }
        } catch { await reconcile(); }
        finally { pendingRef.current = false; if (alive.current) setBusy(false); }
    }
    async function verifyManual() {
        if (pendingRef.current) return;
        pendingRef.current = true; setBusy(true); setError('');
        setVerifyingRequirements(true); setVerificationResult(null); setFinishingRun(null); setFinishingSlow(false);
        try {
            // 'manual' lets a run waiting at its final check finish on its next
            // pass when every check passes; the server, not this page, completes it.
            const latest = await setup.reload('manual');
            if (!alive.current) return;
            accept(latest);
            setVerificationResult({ checks: latest.checks, failed: false });
            if (setupChecksPassed(latest.checks)) setFinishingRun({ checks: JSON.stringify(execution?.checks || []) });
            await reconcile();
        } catch { if (alive.current) setVerificationResult({ checks: [], failed: true }); }
        finally { pendingRef.current = false; if (alive.current) { setBusy(false); setVerifyingRequirements(false); } }
    }
    async function resumeUnconfirmed() {
        const saved = markerRef.current;
        if (!saved || execution || pendingRef.current) { await reconcile(); return; }
        pendingRef.current = true; setBusy(true);
        try {
            // Reuse the already confirmed plan and request. A lost response is
            // never permission to create another installation identity.
            const response = await setupFetch('/api/v1/setup/start', requestOptions({ plan_id: saved.plan_id, request_id: saved.request_id, confirmed: true }));
            if (response.ok) {
                const resumed = decodeSetupExecution(await response.json(), saved);
                if (resumed && alive.current) { setExecution(resumed); setReconnecting(false); }
            }
            await reconcile();
        } catch { if (alive.current) setReconnecting(true); }
        finally { pendingRef.current = false; if (alive.current) setBusy(false); }
    }
    async function continueMailEnrollment(initialRetry = false) {
        const current = initialRetry ? retryableMailEnrollmentHandoff(execution) : continuableMailEnrollment(execution);
        if (!current || !execution || reconnecting || enrollmentSent || pendingRef.current) return;
        pendingRef.current = true; setBusy(true); setError(''); setEnrollmentNotice(null);
        setEnrollmentSent({ execution: execution.id, step: current.id, code: execution.error?.code || '' });
        try {
            const response = await setupFetch(`/api/v1/setup/mail-enrollment/${initialRetry ? 'retry' : 'continue'}`, requestOptions({ execution_id: execution.id, step_id: current.id }));
            const result: unknown = response.ok ? await response.json() : null;
            const accepted = !!result && typeof result === 'object' && 'handoff' in result && result.handoff === 'accepted';
            if (alive.current) setEnrollmentNotice(accepted ? 'checking' : 'unknown');
        } catch { if (alive.current) setEnrollmentNotice('unknown'); }
        finally {
            // A lost POST reply only triggers a read of the same setup execution.
            await reconcile();
            pendingRef.current = false; if (alive.current) setBusy(false);
        }
    }
    // Begin or stop extending a completed setup (POST /api/v1/setup/extend).
    // Only the setup record changes; nothing on the server is installed until
    // the reviewed plan is started. A lost answer is settled by reading the
    // setup again, never by sending the request a second time.
    // Genisletmeyi baslat ya da birak; kayip yanit yeniden okumayla cozulur.
    async function extendSetup(action: 'begin' | 'cancel') {
        if (pendingRef.current) return;
        pendingRef.current = true; setExtendBusy(true); setExtendError(null);
        const settled = (value: ServerSetupSnapshot | null): value is ServerSetupSnapshot => !!value && (action === 'begin' ? value.extension === true : value.status === 'ready' && !value.extension);
        // code: the server's typed refusal; 'unanswered' when no answer came.
        let code = 'unanswered';
        try {
            const response = await setupFetch('/api/v1/setup/extend', requestOptions({ revision: snapshot.revision, action }));
            if (!response.ok) {
                const problem: unknown = await response.json();
                code = problem && typeof problem === 'object' && typeof (problem as { code?: unknown }).code === 'string' ? (problem as { code: string }).code : 'refused';
            } else {
                const next = decodeServerSetup(await response.json());
                // An extension opens on the purpose step, whatever this screen showed.
                if (settled(next)) { pendingRef.current = false; if (action === 'begin') setStep('domain'); accept(next); return; }
            }
        } catch { code = 'unanswered'; }
        let latest: ServerSetupSnapshot | null = null;
        try { latest = await setup.reload(); } catch { latest = null; }
        pendingRef.current = false;
        if (!alive.current) return;
        setExtendBusy(false);
        if (settled(latest) && action === 'begin') setStep('domain');
        if (latest) accept(latest);
        if (settled(latest)) return;
        setExtendError(code === 'server_setup_busy' ? 'setup.extend.busy' : code === 'server_setup_extend_not_ready' ? 'setup.extend.notReady' : action === 'cancel' ? 'setup.extend.stopFailed' : 'setup.extend.failed');
    }
    function editPlan() {
        if (execution && execution.status !== 'failed') return;
        resetForReview();
    }
    function resetForReview() {
        pollEpoch.current++;
        setVerificationResult(null); setFinishingRun(null); setFinishingSlow(false);
        try { localStorage.removeItem(markerKey(user.username)); } catch { /* next start still requires storage */ }
        markerRef.current = null; setMarker(null); setExecution(null); setPlan(null); setAcknowledged(false); setStep('dnshome');
    }
    async function reviseWaiting() {
        if (execution?.status !== 'waiting' || !['verification', 'dns_publisher', 'dns_readiness', 'access_dns', 'primary_dns', 'infrastructure_dns'].includes(execution.phase) || pendingRef.current) return;
        pendingRef.current = true; setBusy(true); setError('');
        try {
            const latest = await setup.reload();
            const response = await setupFetch('/api/v1/setup/revise', requestOptions({ revision: latest.revision, execution_id: execution.id }));
            if (!response.ok) throw new Error(t('setup.reviseBlocked'));
            const revised = decodeServerSetup(await response.json());
            if (!revised || revised.status !== 'draft') throw new Error(t('setup.reviseBlocked'));
            if (alive.current) { accept(revised); setDraft(revised.draft); resetForReview(); }
        } catch (failure) { if (alive.current) setError(failure instanceof Error ? failure.message : t('setup.reviseBlocked')); }
        finally { pendingRef.current = false; if (alive.current) setBusy(false); }
    }
    const waitingVerification = execution?.status === 'waiting' && execution.phase === 'verification';
    // Not offered while setup is finishing: it is the one control that stops the
    // run, and nothing needs to be done then. It returns with the unknown result.
    const canReviseWaiting = !finishing && execution?.status === 'waiting' && ['verification', 'dns_publisher', 'dns_readiness', 'access_dns', 'primary_dns', 'infrastructure_dns'].includes(execution.phase);
    const waitingLicense = execution?.status === 'waiting' && execution.phase === 'license';
    const waitingDNSPrerequisite = execution?.status === 'waiting' && ['access_dns', 'primary_dns', 'infrastructure_dns'].includes(execution.phase);
    const confirmingPrevious = execution?.status === 'running' && execution.error?.code.split(':')[0] === 'server_setup_reconciling';
    // Stopped because its plan was reopened: a known end, not a failure.
    const planReopened = setupPlanReopened(execution);
    const completed = snapshot.status === 'ready';
    const extending = snapshot.extension === true;
    const extensionChanges = extending ? setupExtensionChanges(snapshot.extension_base, draft) : [];
    const extensionValue = (key: SetupExtensionChangeKey, value: string) => !value ? t('setup.extend.none')
        : key === 'purpose' ? t(`setup.purpose.${value}` as TranslationKey)
            : key === 'dns_mode' ? t(`setup.dns.${value}` as TranslationKey)
                : key === 'dns_hosting_management' ? t(value === 'manual' ? 'setup.publisher.manual' : 'setup.publisher.automatic')
                    : key === 'database' ? (value === 'postgresql' ? 'PostgreSQL' : value === 'mariadb' ? 'MariaDB' : value) : value;
    const hasOperation = resolving || !!execution || !!marker || reconnecting;
    const certificateReady = execution?.steps.some(item => item.kind === 'panel_certificate' && item.status === 'succeeded') || execution?.status === 'succeeded';
    const panelURL = certificateReady ? safeSetupPanelURL(execution?.panel_url, marker?.panel_domain || draft.panel_domain) : null;
    // Planned certificate handover (lib/panelHandover.ts): said in advance on
    // another address, and named when the connection drops at that step.
    const currentHost = window.location.hostname;
    const handoverDrop = reconnecting ? plannedHandoverDrop(execution, currentHost) : null;
    const handoverAhead = reconnecting || completed || execution?.status === 'failed' ? null
        : setupHandover(execution ? execution.steps : step === 'review' ? plan?.steps : undefined, currentHost);
    const handoverNotice = handoverAhead?.elsewhere && handoverAhead.phase !== 'done' ? handoverAhead : null;
    const handoverLink = handoverNotice || (handoverDrop?.elsewhere ? handoverDrop : null);
    const handoverURL = handoverLink ? handoverAddress(handoverLink.host, window.location.port, execution?.panel_url) : '';
    // Setup wizard v2 (alpha.111): roles, the optional components step, the
    // names proposed from the main domain and the DNS home.
    const roles = setupDraftRoles(draft, catalog);
    const customized = setupCustomized(draft, catalog);
    const explicitSelection = !!draft.customization || draft.purpose === 'custom';
    const selectedComponents = setupEffectiveComponents(draft, catalog);
    const mailSelected = ['postfix', 'dovecot', 'rspamd', 'roundcube'].some(id => selectedComponents.has(id));
    // Two servers with their own DNS join by code: the first server prepares
    // the list whichever server is the main; a joined second server takes it.
    const joinFlow = !!joinedLink || (draft.dns_mode === 'local' && (draft.server_count ?? 2) === 2);
    const steps: Step[] = ['domain', ...(joinedLink ? [] : ['count' as const]), 'name', 'roles', ...(customized || componentsOpen ? ['components' as const] : []), 'names', 'dnshome',
        ...(!joinedLink && draft.dns_mode === 'local' && (draft.dns_role === 'primary' || joinFlow) && draft.main_domain ? ['records' as const] : []),
        ...(mailSelected && draft.main_domain ? ['mail' as const] : []), 'review', 'progress'];
    // The plan is reviewed when the last editing step is left.
    const lastEdit = steps[steps.indexOf('review') - 1];
    const serverCount = draft.server_count ?? 2;
    const mainDomain = draft.main_domain ? normalizeSetupDomain(draft.main_domain) : '';
    const proposal = setupProposedNames(validSetupDomain(mainDomain) ? mainDomain : '', draft.short_name || '');
    const serverLabel = setupServerLabel(t, draft.short_name);
    const emptySelectionInvalid = selectedComponents.size === 0 && draft.dns_mode !== 'local';
    const isDNS = selectedComponents.size === 0;
    const needsDNSPublisher = ['nginx', 'node', 'postfix', 'roundcube'].some(id => selectedComponents.has(id));
    const localPrimary = draft.dns_mode === 'local' && draft.dns_role === 'primary';
    const panelDNSIP = draft.dns_mode === 'local' ? draft.local_ip : snapshot.server_ip;
    const secondaryHosting = needsDNSPublisher && draft.dns_mode === 'local' && draft.dns_role === 'secondary';
    const hostingDNSManagement = draft.dns_hosting_management || (draft.dns_publisher_endpoint ? 'panel' : 'manual');
    const automaticPublisher = secondaryHosting && hostingDNSManagement === 'panel';
    const publisherEndpoint = remoteDNSEndpoint(draft.dns_publisher_endpoint || '');
    const dnsSelectionError: TranslationKey | null = isDNS && draft.dns_mode !== 'local'
        ? 'setup.components.localDNSRequired'
        : automaticPublisher && !publisherEndpoint && !joinFlow
            ? 'setup.publisher.endpointRequired' : null;
    const fullName = (value: string) => validSetupDomain(value.trim().toLowerCase().replace(/\.$/, ''));
    function stepProblem(): TranslationKey | null {
        switch (step) {
        case 'domain': return joinedLink || validSetupDomain(mainDomain) ? null : 'setup.v2.domain.invalid';
        case 'name': return validSetupShortName((draft.short_name || '').trim().toLowerCase()) ? null : 'setup.v2.name.invalid';
        case 'roles': return roles.web || roles.mail || roles.dns || selectedComponents.size > 0 ? null : 'setup.v2.roles.required';
        case 'components':
            if (!catalog || catalog.inventory_state !== 'ready') return 'setup.components.inventoryUnknown';
            return emptySelectionInvalid && !roles.dns ? 'setup.components.chooseRequired' : null;
        case 'names':
            if (!fullName(draft.panel_domain) || (isMail && !fullName(draft.mail_hostname))) return 'setup.v2.names.invalid';
            if (localPrimary && (!setupNameWithin(draft.panel_domain, mainDomain) || (isMail && !setupNameWithin(draft.mail_hostname, mainDomain)))) return 'setup.v2.names.outsideZoneShort';
            return null;
        case 'dnshome':
            if (draft.dns_mode === 'local' && dnsNames.mismatch) return 'setup.dnsMappingMismatch';
            if (draft.dns_mode === 'local' && setupEngineChoice(catalog, draft.dns_engine, draft.dns_role)?.offered === false) return 'setup.v2.engine.notOffered';
            if (dnsSelectionError) return dnsSelectionError;
            if (draft.dns_mode === 'existing' && !remoteVerified) return 'setup.remote.proof.failed';
            return null;
        default: return null;
        }
    }
    function proposeNames() {
        const current = proposal;
        updateDraft(previous => {
            const next = setupApplyProposedNames(previous, current, proposalRef.current, roles.mail);
            if (next.dns_mode === 'local' && next.dns_role === 'primary') next.main_dns_server = 'this';
            return next;
        });
        proposalRef.current = current;
    }
    const isMail = ['postfix', 'dovecot', 'rspamd', 'roundcube'].some(id => selectedComponents.has(id));
    const isNode = selectedComponents.has('node');
    const nextPath = setupNextPath(snapshot.draft.purpose, explicitSelection ? selectedComponents : undefined);
    const nextLabel = nextPath === '/dns-infrastructure' ? 'setup.nextDNS' : nextPath === '/services' ? 'setup.nextComponents' : isNode ? 'setup.nextApplication' : 'setup.nextWebsite';
    const progressCurrentStep = execution?.steps.find(item => ['running', 'failed'].includes(item.status));
    const observingMailEnrollment = progressCurrentStep?.kind === 'mail_enrollment' && execution?.status === 'running' && ['running', 'rollback', 'unknown', 'not_recorded', 'handoff'].some(reason => execution.error?.code === `server_setup_mail_enrollment_${reason}`);
    const mailEnrollmentToRetry = !reconnecting && retryableMailEnrollmentHandoff(execution);
    const mailEnrollmentToContinue = !reconnecting && continuableMailEnrollment(execution);
    const guidance = execution && !reconnecting && !finishing ? setupExecutionGuidance(execution, componentLabel) : null;
    // A server that was busy is a typed wait, not a failure: its reason leads the
    // screen, with the action beside it, and the generic paragraphs stay out.
    const hostBusy = execution?.status === 'failed' && !reconnecting ? setupHostBusyGuidance(execution.error) : null;
    // The primary created this server's record (2026-10-16): the guidance above says
    // that nothing is needed; the generic 'follow the record instructions' line is not drawn.
    // Birincil kaydi olusturdu: genel 'kaydi izleyin' satiri cizilmez.
    const peerRecordApplied = execution?.phase === 'access_dns' && execution.context?.peer_host_record?.outcome?.status === 'applied';
    // One heading per state. A stopped, waiting or confirming run is named by its
    // guidance, so the block under it does not repeat that as a second heading;
    // only a run in progress keeps "what happens at this step" as the block's label.
    const stateTitle = hostBusy ? hostBusy.title : guidance && (execution?.status !== 'running' || confirmingPrevious) ? guidance.title : null;
    // A run accepted but not yet at its first step names that first step (set18 O1).
    // Kabul edilmis, henuz baslamamis calisma ilk adimini adlandirir.
    const guidanceStepItem = progressCurrentStep || (execution?.status === 'running' && execution.phase === 'queued' ? execution.steps.find(item => item.status === 'pending') : undefined);
    const guidanceStep = guidanceStepItem && !waitingLicense ? t(`setup.kind.${guidanceStepItem.kind}`, { target: stepTarget(guidanceStepItem.kind, guidanceStepItem.target) }) : '';
    const shownChecks = verificationResult && !verificationResult.failed ? verificationResult.checks : execution?.checks || snapshot.checks;
    const progressChecks = shownChecks.filter(check => check.state !== 'ready' && !check.advisory);
    // Reverse DNS is the provider's: an open task, never a wait (2026-10-28).
    const openTasks = shownChecks.filter(check => check.state !== 'ready' && check.advisory);
    const verifiedBlockers = verificationResult?.checks.filter(check => check.state !== 'ready' && !check.advisory) || [];
    const verificationMessage = verifyingRequirements ? 'setup.verifyChecking'
        : !verificationResult ? null
        : verificationResult.failed || verificationResult.checks.length === 0 ? 'setup.verificationFailed'
        : verifiedBlockers.some(check => check.state === 'unknown') ? 'setup.verifyUnknown'
        : verifiedBlockers.length > 0 ? 'setup.verifyStillWaiting'
        : finishing ? 'setup.finishing' : finishingRun && finishingSlow ? 'setup.finishingSlow' : null;

    if (manualExit) return <Navigate to="/" replace />;
    return <ServerSetupShell navigation={!completed ? <ServerSetupSteps steps={steps} current={hasOperation ? 'progress' : step} /> : undefined}>
        <div className="max-w-3xl">
            <h1 ref={heading} tabIndex={-1} className="text-2xl font-semibold leading-tight outline-none focus-visible:outline-none sm:text-3xl">{t(completed ? 'setup.completeTitle' : extending ? 'setup.extend.title' : 'setup.title')}</h1>
            <p className="mt-3 max-w-2xl text-fg-muted">{t(completed ? 'setup.completeHelp' : hasOperation ? 'setup.guide.progressIntro' : extending ? 'setup.extend.intro' : 'setup.intro')}</p>
        </div>
        {completed ? <section className="mt-8 max-w-3xl space-y-6" aria-labelledby="setup-ready-title">
            <div className="flex items-start gap-3"><Check className="mt-1 h-5 w-5 shrink-0 text-success" aria-hidden="true" /><div><h2 id="setup-ready-title" className="text-lg font-semibold">{t('setup.v2.ready', { server: setupServerLabel(t, snapshot.draft.short_name) })}</h2><p className="mt-1 text-sm text-fg-muted">{t('setup.completeServices')}</p></div></div>
            {snapshot.checks.some(check => check.advisory && check.state !== 'ready') && <SetupOpenTasks lines={snapshot.checks.filter(check => check.advisory && check.state !== 'ready').map(checkSentence)} />}
            <Link to={nextPath} className="inline-flex items-center gap-3 rounded-lg bg-primary px-5 py-3 font-semibold text-primary-fg hover:bg-primary/90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary">{t(nextLabel)}<ArrowRight className="h-4 w-4" aria-hidden="true" /></Link>
            {/* What extending does and does not do, said before the action (D-033). */}
            <section aria-labelledby="setup-extend-title" className="border-t border-border pt-6" data-setup-extend="">
                <h2 id="setup-extend-title" className="text-lg font-semibold">{t('setup.extend.title')}</h2>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.extend.help')}</p>
                {extendError && <p role="alert" className="mt-3 max-w-2xl text-sm leading-6 text-fg">{t(extendError)}</p>}
                <Button variant="secondary" className="mt-4" disabled={extendBusy} loading={extendBusy} onClick={() => void extendSetup('begin')}>{t('setup.extend.action')}</Button>
            </section>
        </section> : <div className="mt-6 space-y-6">
            <div className="md:hidden"><ServerSetupSteps steps={steps} current={hasOperation ? 'progress' : step} mobile /></div>
            <div className="min-w-0">
            {error && <p role="alert" className="mb-5 rounded-lg border border-danger/40 bg-danger/5 p-4 text-sm text-danger">{error}</p>}
            {resolving ? <div className="flex items-center gap-3"><Spinner label={t('setup.resuming')} /><p>{t('setup.resuming')}</p></div>
                : step === 'progress' || hasOperation ? <section aria-labelledby="setup-progress-title">
                    <h2 id="setup-progress-title" className="text-xl font-semibold">{t(handoverDrop ? 'setup.handover.dropTitle' : reconnecting ? 'setup.reconnecting' : stateTitle ?? (finishing ? 'setup.finishingTitle' : execution?.status === 'failed' ? 'setup.operationFailed' : waitingLicense ? 'setup.licenseWaiting' : execution?.status === 'waiting' ? 'setup.waiting' : execution?.status === 'succeeded' ? 'setup.verifying' : 'setup.installing'))}</h2>
                    {/* The page reads the operation again by itself while it reconnects: that is
                        shown as active, with motion, and for a bounded time (owner rule 2026-10-14).
                        Sayfa yeniden baglanirken islemi kendisi okur; bu hareketle ve sinirli sure gosterilir. */}
                    {handoverDrop ? <div role="status" className="mt-3 max-w-2xl"><p className="flex items-start gap-3 break-words text-sm leading-6 text-fg-muted" data-setup-reconnecting=""><Loader2 className="mt-1 h-4 w-4 shrink-0 text-primary motion-safe:animate-spin" aria-hidden="true" /><span>{t('setup.handover.drop', { host: handoverDrop.host })}</span></p>{handoverDrop.elsewhere && <SetupHandoverAddress lead={t('setup.handover.dropAddress')} url={handoverURL} />}</div>
                        : reconnecting && (reconnectSlow
                            ? <div role="status" className="mt-3 max-w-2xl rounded-lg border border-border bg-surface p-4"><p className="break-words text-sm leading-6" data-setup-reconnect-slow=""><KeepWords text={t('setup.reconnectSlow')} /></p><p className="mt-1 text-sm text-fg-muted" data-sudo-note="">{t('common.sudoNote')}</p><Button variant="secondary" className="mt-3" onClick={() => window.location.reload()}>{t('setup.reloadPage')}</Button></div>
                            : <p role="status" className="mt-3 flex max-w-2xl items-start gap-3 text-sm leading-6 text-fg-muted" data-setup-reconnecting=""><Loader2 className="mt-1 h-4 w-4 shrink-0 text-primary motion-safe:animate-spin" aria-hidden="true" /><span>{t('setup.uncertain')}</span></p>)}
                    {execution?.status === 'waiting' && execution.phase === 'dns_publisher' && !execution.error?.code.startsWith('server_setup_dns_publisher_') && <SetupDNSPublisher key={execution.id} execution={execution} onBound={reconcile} />}
                    {execution && execution.status !== 'failed' && execution.steps.some(item => item.kind === 'peer_join' && item.status !== 'succeeded') && <SetupJoinCard />}
                    {hostBusy ? <div role="alert" className={`mt-4 rounded-lg border p-4 sm:p-5 ${hostBusy.caution ? 'border-warning-mark/40 bg-warning-mark/10' : 'border-border bg-surface'}`}>
                        <p className="max-w-3xl break-words text-sm leading-6 text-fg">{t(hostBusy.body)}</p>
                        <Button variant="primary" className="mt-4" onClick={editPlan}>{t('setup.revise')}</Button>
                    </div> : guidance && <aside aria-labelledby={stateTitle && !guidanceStep ? 'setup-progress-title' : 'setup-guidance-title'} className="mt-5 rounded-lg border border-border bg-surface p-4 sm:p-5">
                        {stateTitle ? guidanceStep && <h3 id="setup-guidance-title" className="break-words font-semibold">{guidanceStep}</h3>
                            : <><h3 id="setup-guidance-title" className="font-semibold">{t(guidance.title)}</h3>{guidanceStep && <p className="mt-2 break-words text-sm font-medium">{guidanceStep}</p>}</>}
                        <div role={execution?.status === 'failed' && !planReopened ? 'alert' : 'status'} className="mt-2 space-y-2 text-sm leading-6 text-fg-muted first:mt-0">{guidance.messages.map((message, index) => <div key={index} className="space-y-2">
                            <p className="break-words">{say(message)}</p>
                            {/* The open checks follow the sentence that introduces them, in full. */}
                            {index === (guidance.checksAt ?? 0) && guidance.checks && guidance.checks.length > 0 && <ul className="space-y-2 rounded-md border border-border bg-surface-2 px-4 py-3 text-fg" data-setup-checks="">{guidance.checks.map(line => <li key={line.check.id} className="break-words" data-check={line.check.id} data-state={line.check.state}>{line.text ? say(line.text) : failureText(line.check.code)}</li>)}</ul>}
                        </div>)}</div>
                        {canReviseWaiting && <Button variant="secondary" disabled={busy} className="mt-3" onClick={() => void reviseWaiting()}>{t('setup.editPlan')}</Button>}
                        {mailEnrollmentToContinue && <Button variant="secondary" disabled={busy || !!enrollmentSent} loading={!!enrollmentSent} className="mt-3" onClick={() => void continueMailEnrollment()}>{t(enrollmentSent ? 'setup.mailEnrollment.sent' : 'setup.mailEnrollment.continue')}</Button>}
                        {mailEnrollmentToRetry && <Button variant="secondary" disabled={busy || !!enrollmentSent} loading={!!enrollmentSent} className="mt-3" onClick={() => void continueMailEnrollment(true)}>{t(enrollmentSent ? 'setup.mailEnrollment.sent' : 'setup.mailEnrollment.retry')}</Button>}
                        {enrollmentNotice && progressCurrentStep?.kind === 'mail_enrollment' && <p role="status" className="mt-3 text-sm">{t(`setup.mailEnrollment.${enrollmentNotice}`)}</p>}
                        {execution?.context?.infrastructure_dns && ['infrastructure_dns', 'access_dns'].includes(progressCurrentStep?.kind || execution?.phase || '') && <details className="mt-3 text-sm"><summary className="cursor-pointer font-medium text-primary">{t('setup.infrastructure.reviewTitle')}</summary><SetupInfrastructureDNSReview value={execution.context.infrastructure_dns} compact /></details>}
                        {guidance.details.length > 0 && <details className="mt-3 text-sm leading-6"><summary className="cursor-pointer font-medium text-primary">{t('setup.guide.more')}</summary><ul className="mt-2 list-disc space-y-2 pl-5 text-fg-muted">{guidance.details.map((message, index) => <li key={index}>{say(message)}</li>)}</ul></details>}
                    </aside>}
                    {/* The failure colour is for a verified failure only: a busy server, an unmet prerequisite and a result still being confirmed are not one. */}
                    {execution?.error && !['dns_publisher', 'dns_readiness'].includes(execution.phase) && <div role={hostBusy ? undefined : confirmingPrevious || waitingDNSPrerequisite || observingMailEnrollment || planReopened ? 'status' : 'alert'} className={`${hostBusy ? 'mt-3' : 'mt-5'} space-y-2 text-sm`}>{!hostBusy && !peerRecordApplied && (!(confirmingPrevious || observingMailEnrollment || planReopened) || !guidance) && <p className={execution.status === 'failed' ? 'text-danger' : confirmingPrevious || waitingDNSPrerequisite ? 'text-fg-muted' : 'text-fg'}>{failureText(execution.error.code, execution.error.message, execution.error.reason)}</p>}<details><summary className="cursor-pointer text-primary">{t('setup.details')}</summary><p className="mt-2 break-words text-fg-muted">{execution.error.message}</p></details></div>}
                    {/* The action a stopped run names sits with its reason, above the step list. */}
                    {execution?.status === 'failed' && guidance && !hostBusy && <div className="mt-5"><Button variant="primary" onClick={editPlan}>{t('setup.revise')}</Button></div>}
                    {progressChecks.length > 0 && waitingVerification && !guidance?.checks && <ul className="mt-5 list-disc space-y-2 pl-5 text-sm text-fg-muted">{progressChecks.map(check => <li key={check.id} className="break-words">{checkSentence(check)}</li>)}</ul>}
                    {/* After everything that explains the current state, and still above the step list. */}
                    {handoverNotice && <SetupHandoverNotice host={handoverNotice.host} url={handoverURL} />}
                    {openTasks.length > 0 && <SetupOpenTasks lines={openTasks.map(checkSentence)} />}
                    {execution && !certificateReady && execution.status !== 'failed' && <SetupPanelAccessCard ip={snapshot.server_ip || draft.local_ip} port={window.location.port || '2083'} panelDomain={marker?.panel_domain || draft.panel_domain} />}
                    {execution && execution.status !== 'failed' && draft.dns_mode === 'external' && <SetupProviderRecordsCard />}
                    {execution && execution.status !== 'failed' && draft.dns_mode === 'local' && draft.ns1 && draft.ns2 && <SetupRegistrarCard draft={draft} mainDomain={mainDomain} now={execution.phase === 'access_dns' || (draft.dns_role === 'secondary' && execution.status === 'succeeded')} />}
                    <ol className="mt-6 divide-y divide-border" aria-live="polite">
                        {execution?.steps.map(item => {
                            // The step's real state: a stopped run shows nothing in progress, and a
                            // waiting final check says what it waits for (lib/serverSetupGuidance).
                            const status = finishing && item.kind === 'verify' ? 'finishing'
                                // Past the finishing bound every check had passed and completion is
                                // not confirmed: not "Waiting for requirements" (browser inspection
                                // 2026-10-14). Sinirdan sonra: kontroller gecti, tamamlanma dogrulanmadi.
                                : finishingRun && finishingSlow && item.kind === 'verify' && waitingVerification ? 'unconfirmed'
                                    : setupStepState(execution, item, progressChecks);
                            const tone = status === 'failed' ? 'font-medium text-danger' : status === 'waitingOwner' || status === 'stopped' ? 'font-medium text-fg' : 'text-fg-muted';
                            return <li key={item.id} className="flex flex-col items-start justify-between gap-2 py-4 sm:flex-row sm:items-center sm:gap-4" data-step-state={status}><div className="min-w-0"><p className="font-medium">{t(`setup.kind.${item.kind}`, { target: stepTarget(item.kind, item.target) })}</p>{item.qualifier && item.kind !== 'mail_enrollment' && <p className="mt-1 break-words text-sm text-fg-muted">{item.qualifier}</p>}{handoverNotice && item.kind === 'panel_certificate' && <p className="mt-1 text-sm text-fg-muted">{t('setup.handover.stepNote')}</p>}</div><span className={`flex shrink-0 items-center gap-2 text-sm ${tone}`}>{status === 'succeeded' ? <Check className="h-4 w-4 text-success" aria-hidden="true" /> : status === 'running' || status === 'finishing' ? <Loader2 className="h-4 w-4 motion-safe:animate-spin" aria-hidden="true" /> : <Circle className="h-3 w-3" aria-hidden="true" />}{t(setupStepStateLabel[status])}</span></li>; })}
                    </ol>
                    {waitingVerification && !finishing && <p className="mt-5 text-sm text-fg-muted">{t('setup.reviseHelp')}</p>}
                    {completionFailed && <p role="alert" className="mt-5 text-sm text-danger">{t('setup.verificationFailed')}</p>}
                    {panelURL && new URL(panelURL).hostname !== window.location.hostname && <p className="mt-6"><a href={new URL('/setup', panelURL).href} className="font-semibold text-primary underline underline-offset-4">{t('setup.secureAddress')}</a></p>}
                    {waitingLicense && <Link to="/settings?section=license" className="mt-5 inline-flex text-primary underline underline-offset-4">{t('license.activate')}</Link>}
                    <div className="mt-6 flex flex-wrap gap-3">
                        {canReviseWaiting && !guidance && <Button variant="secondary" disabled={busy} onClick={() => void reviseWaiting()}>{t('setup.editPlan')}</Button>}
                        {execution?.status === 'failed' ? !guidance && <Button variant="primary" onClick={editPlan}>{t('setup.revise')}</Button>
                            : waitingVerification ? !finishing && <Button variant="primary" disabled={busy} loading={verifyingRequirements} onClick={() => void verifyManual()}>{t(verifyingRequirements ? 'setup.verifyChecking' : 'setup.verify')}</Button>
                                : (reconnecting || completionFailed) && <Button variant={handoverDrop ? 'secondary' : 'primary'} disabled={busy} onClick={() => void (marker && !execution ? resumeUnconfirmed() : reconcile())}>{t(marker && !execution ? 'setup.resumeConfirmed' : 'setup.reconnect')}</Button>}
                    </div>
                    {waitingVerification && <div role="status" aria-live="polite" aria-atomic="true" className="mt-4 text-sm">
                        {verificationMessage && <div ref={verificationBox} className="scroll-mb-6 rounded-lg border border-border bg-surface p-4">
                            {finishing ? <p className="flex items-start gap-3 font-medium" data-setup-finishing=""><Loader2 className="mt-0.5 h-4 w-4 shrink-0 text-primary motion-safe:animate-spin" aria-hidden="true" /><span>{t(verificationMessage)}</span></p>
                                : <p className={verificationMessage === 'setup.finishingSlow' ? 'max-w-3xl break-words leading-6' : 'font-medium'}>{verificationMessage === 'setup.finishingSlow' ? <KeepWords text={t(verificationMessage)} /> : t(verificationMessage)}</p>}
                            {!verifyingRequirements && verifiedBlockers.length > 0 && <ul className="mt-2 list-disc space-y-2 pl-5 text-fg-muted">{verifiedBlockers.map(check => <li key={check.id} className="break-words" data-check={check.id}>{checkSentence(check)}</li>)}</ul>}
                            {verificationMessage === 'setup.finishingSlow' && <p className="mt-1 text-sm text-fg-muted" data-sudo-note="">{t('common.sudoNote')}</p>}
                            {verificationMessage === 'setup.finishingSlow' && <Button variant="secondary" className="mt-3" onClick={() => window.location.reload()}>{t('setup.reloadPage')}</Button>}
                        </div>}
                    </div>}
                </section> : <form onSubmit={next} className="min-w-0">
                    {step === 'domain' && (joinedLink ? <SetupJoinedSummary link={joinedLink} /> : <>
                        <SetupDomainStep value={draft.main_domain || ''} invalid={!!error && !validSetupDomain(mainDomain)} onChange={value => updateDraft(previous => ({ ...previous, main_domain: value }))} />
                        {!extending && peer.known && <SetupJoinPanel disabled={busy} serverIP={snapshot.server_ip} onJoined={value => { const next = decodeServerSetup(value); peer.refresh(); if (next) { accept(next); setDraft(next.draft); setStep('name'); } }} />}
                    </>)}
                    {step === 'count' && <SetupCountStep value={serverCount} onChange={value => updateDraft(previous => ({ ...previous, server_count: value }))} />}
                    {step === 'name' && <SetupNameStep value={draft.short_name || ''} suggestion={snapshot.short_name_suggestion} mainDomain={mainDomain} invalid={!validSetupShortName((draft.short_name || '').trim().toLowerCase())} onChange={value => updateDraft(previous => ({ ...previous, short_name: value }))} />}
                    {step === 'roles' && <>
                        <SetupRolesStep roles={roles} catalog={catalog} server={serverLabel} serverCount={serverCount} customized={customized} disabled={busy}
                            onRoles={next => { if (catalog) updateDraft(previous => applySetupRoles(previous, next, catalog)); else updateDraft(previous => ({ ...previous, dns_mode: next.dns ? 'local' : previous.dns_mode === 'local' ? 'external' : previous.dns_mode })); }}
                            onCustomize={() => { if (!catalog) return; setComponentsOpen(true); updateDraft(previous => ({ ...previous, purpose: 'custom', customization: previous.customization || { components: setupRolesPreset(roles, catalog) } })); setStep('components'); }}
                            onReset={() => { if (catalog) { setComponentsOpen(false); updateDraft(previous => resetSetupCustomization(previous, catalog)); } }} />
                        {catalogFailed && <p role="alert" className="mt-3 text-sm text-fg-muted">{t('setup.components.loadFailed')} <Button type="button" onClick={reloadCatalog}>{t('common.retry')}</Button></p>}
                    </>}
                    {step === 'components' && (catalog ? <ServerSetupComponents catalog={catalog} selected={draft.customization?.components || []} disabled={busy}
                        versions={draft.customization?.versions || {}}
                        onVersions={versions => updateDraft(previous => ({ ...previous, purpose: 'custom', customization: { components: previous.customization?.components || [], ...(Object.keys(versions).length ? { versions } : {}) } }))}
                        onChange={components => updateDraft(previous => {
                            const versions = Object.fromEntries(Object.entries(previous.customization?.versions || {}).filter(([id]) => setupEffectiveComponents({ ...previous, customization: { components } }, catalog).has(id)));
                            return { ...previous, purpose: 'custom', customization: { components, ...(Object.keys(versions).length ? { versions } : {}) } };
                        })} /> : <div role={catalogFailed ? 'alert' : 'status'} className="space-y-3"><h2 className="text-xl font-semibold">{t('setup.components.title')}</h2><p className="text-sm text-fg-muted">{t(catalogFailed ? 'setup.components.loadFailed' : 'setup.components.loading')}</p>{catalogFailed && <Button type="button" onClick={reloadCatalog}>{t('common.retry')}</Button>}</div>)}
                    {step === 'components' && isNode && <div className="mt-6 max-w-md"><SetupNodeVersion value={draft.node_version} onChange={value => change('node_version', value)} /></div>}
                    {step === 'components' && emptySelectionInvalid && !roles.dns && <p role="alert" className="mt-4 text-sm text-danger">{t('setup.components.chooseRequired')}</p>}
                    {step === 'components' && catalog?.inventory_state === 'unknown' && <div className="mt-4"><Button type="button" disabled={busy} onClick={reloadCatalog}>{t('common.retry')}</Button></div>}
                    {step === 'names' && <fieldset disabled={busy}><SetupNamesStep draft={draft} mail={isMail} web={roles.web} mainDomain={mainDomain} ip={panelDNSIP || snapshot.server_ip || ''} ownDNS={localPrimary} onChange={(key, value) => change(key, value)} /></fieldset>}
                    {step === 'records' && <fieldset disabled={busy}><SetupRecordsStep draft={draft} update={updateDraft} mainDomain={mainDomain}
                        own={[draft.panel_domain, draft.mail_hostname, draft.ns1, draft.ns2, draft.infrastructure_dns?.peer_panel_domain || '', draft.infrastructure_dns?.peer_mail_hostname || ''].filter(Boolean)} /></fieldset>}
                    {step === 'mail' && <fieldset disabled={busy}><SetupMailStep draft={draft} update={updateDraft} mainDomain={mainDomain} ip4={draft.dns_mode === 'local' ? draft.local_ip : snapshot.server_ip || ''} ip6={snapshot.server_ipv6 || ''} /></fieldset>}
                    {step === 'dnshome' && <fieldset disabled={busy}><SetupDNSHomeStep draft={draft} catalog={catalog} serverCount={serverCount} mainDomain={mainDomain} detectedIP={detectedIP}
                        set={(key, value) => { if (key === 'local_ip') localIPTouched.current = true; updateDraft(previous => ({ ...previous, [key]: value })); }}
                        onMode={chooseDNSMode}
                        onMainDNS={main => updateDraft(previous => changeSetupWizardDNSRole(previous, main === 'this' ? 'primary' : 'secondary', proposal))}
                        joined={joinedLink}
                        connection={<ServerSetupDNSConnection value={draft.remote_dns_connection_id} onChange={value => change('remote_dns_connection_id', value)} onValidityChange={setRemoteVerified} />}
                        publisher={secondaryHosting && !joinFlow && <div className="space-y-3">
                            <label className="block" htmlFor="setup-dns_hosting_management"><span className="mb-2 block text-sm font-medium">{t('setup.publisher.management')}</span><select id="setup-dns_hosting_management" name="dns_hosting_management" value={hostingDNSManagement} onChange={event => change('dns_hosting_management', event.target.value as 'manual' | 'panel')} className={`${inputClass} w-full`}>
                                <option value="manual">{t('setup.publisher.manual')}</option>
                                <option value="panel">{t('setup.publisher.automatic')}</option>
                            </select></label>
                            {automaticPublisher ? <>
                                <SetupWizardInput name="dns_publisher_endpoint" label={t('setup.publisher.endpoint')} value={draft.dns_publisher_endpoint || ''} onChange={value => change('dns_publisher_endpoint', value)} placeholder="https://frankfurt.example.com:2083" required />
                                <p className="text-sm leading-6 text-fg-muted">{t('setup.publisher.setupHelp')}</p>
                            </> : <p className="text-sm leading-6 text-fg-muted">{t('setup.publisher.manualHelp')}</p>}
                        </div>} />
                        {dnsSelectionError && <p role="alert" className="mt-4 text-sm leading-6 text-danger">{t(dnsSelectionError)}</p>}
                        {draft.dns_mode === 'local' && dnsNames.mismatch && <div role="alert" className="mt-4 space-y-2 rounded-lg border border-warning-mark/40 bg-warning-mark/10 p-4 text-sm"><p>{t('setup.dnsMappingMismatch')}</p><p className="break-all">{t('setup.savedPeerName')}: {draft.peer_ns || '—'}</p><Button type="button" disabled={!draft[dnsNames.peerKey].trim()} onClick={() => change('peer_ns', draft[dnsNames.peerKey])}>{t('setup.useDisplayedPeer')}</Button></div>}
                    </fieldset>}
                    {step === 'review' && plan && <section aria-labelledby="setup-plan-title">
                        <h2 id="setup-plan-title" className="text-xl font-semibold">{t('setup.reviewTitle')}</h2><p className="mt-3 text-sm leading-6 text-fg-muted">{t('setup.reviewHelp')}</p>
                        {handoverNotice && <SetupHandoverNotice host={handoverNotice.host} url={handoverURL} />}
                        {plan.infrastructure_dns && <SetupInfrastructureDNSReview value={plan.infrastructure_dns} />}
                        {extending && <SetupExtensionReview plan={plan} changes={extensionChanges.map(change => ({ key: change.key, label: t(extensionLabels[change.key]), before: extensionValue(change.key, change.before), after: extensionValue(change.key, change.after), beforeLiteral: extensionLiteral(change.key) && change.before !== '', afterLiteral: extensionLiteral(change.key) && change.after !== '' }))} name={(kind, target) => kind === 'service' ? componentName(catalog?.components.find(row => row.id === target) ?? { id: target, name: stepTarget('service', target) }, t) : t(`setup.kind.${kind}` as TranslationKey, { target: stepTarget(kind, target) })} />}
                        {plan.components && !extending && <div className="mt-5"><h3 className="font-semibold">{t('setup.components.reviewTitle')}</h3><ul className="mt-2 divide-y divide-border">{plan.components.map(item => <li key={item.id} className="flex flex-wrap items-baseline justify-between gap-2 py-2 text-sm"><span>{componentName(catalog?.components.find(row => row.id === item.id) ?? { id: item.id, name: stepTarget('service', item.id) }, t)}</span><span className="text-fg-muted">{t(item.installed ? 'setup.components.keep' : item.required ? 'setup.components.dependency' : 'setup.components.toInstall')}</span></li>)}</ul><p className="mt-3 text-sm text-fg-muted">{t('setup.components.preserve')}</p></div>}
                        <ol className="mt-5 divide-y divide-border">{plan.steps.map(item => <li key={item.id} className="py-4"><p className="font-medium">{t(`setup.kind.${item.kind}`, { target: stepTarget(item.kind, item.target) })}</p>{item.qualifier && item.kind !== 'mail_enrollment' && <p className="mt-1 text-sm text-fg-muted">{item.qualifier}</p>}{handoverNotice && item.kind === 'panel_certificate' && <p className="mt-1 text-sm text-fg-muted">{t('setup.handover.stepNote')}</p>}</li>)}</ol>
                        <dl className="mt-5 space-y-3 rounded-lg bg-surface-2 p-4 text-sm"><div><dt className="font-semibold">{t('setup.firewallReview')}</dt><dd className="mt-1 leading-6 text-fg-muted">{t('setup.firewallHelp')}</dd></div><div><dt className="font-semibold">TCP</dt><dd className="mt-1 break-words tabular-nums">{plan.tcp_ports.join(', ') || t('setup.noPorts')}</dd></div><div><dt className="font-semibold">UDP</dt><dd className="mt-1 break-words tabular-nums">{plan.udp_ports.join(', ') || t('setup.noPorts')}</dd></div><div><dt className="font-semibold">{t('setup.certificateContact')}</dt><dd className="mt-1 break-all">{plan.contact_email}</dd></div>{plan.hostname_change && <div><dt className="font-semibold">{t('setup.hostnameChange')}</dt><dd className="mt-1 break-all">{plan.hostname_change}</dd></div>}</dl>
                        {automaticPublisher && publisherEndpoint && <div className="mt-5 space-y-2 border-t border-border pt-4 text-sm"><p className="font-semibold">{t('setup.publisher.reviewTitle')}</p><p className="break-all">{publisherEndpoint}</p><p className="leading-6 text-fg-muted">{t('setup.publisher.setupHelp')}</p></div>}
                        {plan.remote_dns_connection && <div className="mt-5 rounded-lg border border-border p-4 text-sm"><p className="font-semibold">{t('setup.remote.reviewTitle')}</p><p className="mt-2 break-all">{plan.remote_dns_connection.endpoint}</p><p className="mt-1 break-words text-fg-muted">{plan.remote_dns_connection.nameservers.join(', ')}</p><p className="mt-2 leading-6 text-fg-muted">{t('setup.remote.reviewHelp')}</p></div>}
                        {secondaryHosting && !automaticPublisher && <p className="mt-5 text-sm leading-6 text-fg-muted">{t('setup.publisher.manualHelp')}</p>}
                        {draft.dns_mode === 'external' && <p className="mt-5 text-sm leading-6 text-fg-muted">{t('setup.externalAfter')}</p>}
                        {isMail && <p className="mt-4 text-sm leading-6 text-fg-muted">{t('setup.mailAfter')}</p>}
                        {plan.blockers.length > 0 && <div role="alert" className="mt-5 rounded-lg border border-warning-mark/40 bg-warning-mark/10 p-4"><p className="font-semibold">{t('setup.planBlocked')}</p><ul className="mt-3 list-disc space-y-2 pl-5 text-sm">{plan.blockers.map(code => <li key={code}>{failureText(code)}{code === 'panel_certificate_contact_email_invalid' && <>{' '}<Link to="/account#email" className="inline-flex min-h-11 items-center font-medium text-primary underline underline-offset-4 sm:min-h-0" data-setup-blocker-action={code}>{t('setup.blocker.contactAction')}</Link></>}<details className="mt-1 text-xs text-fg-muted"><summary className="cursor-pointer">{t('setup.details')}</summary><code className="mt-1 block break-words">{code}</code></details></li>)}</ul></div>}
                        <div className="mt-5 rounded-lg border border-border bg-surface p-4 text-sm leading-6" data-setup-untouched="">
                            <h3 className="font-semibold">{t('setup.v2.review.untouched')}</h3>
                            <ul className="mt-2 list-disc space-y-1 pl-5 text-fg-muted">
                                {localPrimary && mainDomain && <li>{t('setup.v2.review.registrar', { domain: mainDomain })}</li>}
                                {joinFlow && !joinedLink && <li>{t('setup.v5.review.second')}</li>}
                                {joinedLink && <li>{t('setup.v5.review.joined', { server: joinedLink.first.short_name || t('setup.v5.joined.firstServer'), ip: joinedLink.first.ipv4 })}</li>}
                                {draft.dns_mode === 'external' && <li>{t('setup.v2.review.provider')}</li>}
                                {roles.web && mainDomain && !(localPrimary && (draft.imported_records || []).some(item => item.decision === 'import')) && <li>{t('setup.v2.review.site', { domain: mainDomain })}</li>}
                                {localPrimary && !!draft.main_domain && (draft.imported_records || []).length > 0 && <li>{t('setup.v3.review.imported', { imported: (draft.imported_records || []).filter(item => item.decision === 'import').length, skipped: (draft.imported_records || []).filter(item => item.decision === 'skip').length })}</li>}
                                {mailSelected && mainDomain && <li>{t(draft.mail_dns?.mx === 'now' || !(draft.imported_records || []).some(item => item.decision === 'import' && item.type === 'MX') ? 'setup.v3.review.mailNow' : 'setup.v3.review.mailLater', { domain: mainDomain, host: draft.mail_hostname })}</li>}
                                <li>{t('setup.v2.review.installed')}</li>
                            </ul>
                        </div>
                        <SetupPanelAccessCard ip={snapshot.server_ip || draft.local_ip} port={window.location.port || '2083'} panelDomain={draft.panel_domain} />
                        {plan.can_start && <label className="mt-6 flex cursor-pointer items-start gap-3 text-sm leading-6"><input type="checkbox" checked={acknowledged} onChange={event => setAcknowledged(event.target.checked)} className="mt-1 h-4 w-4 shrink-0 accent-primary" /><span>{t('setup.confirm')}</span></label>}
                    </section>}
                    <div className="setup-actions sticky bottom-0 z-10 mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-border bg-bg py-4">
                        {step !== 'domain' && <Button type="button" variant="secondary" disabled={busy} onClick={() => { setStep(steps[Math.max(0, steps.indexOf(step) - 1)]); setPlan(null); setAcknowledged(false); setError(''); }}>{t('setup.back')}</Button>}
                        <div className="ml-auto flex flex-wrap items-center justify-end gap-3">
                        {step === 'review' ? <Button type="button" variant="primary" disabled={busy || !plan?.can_start || !acknowledged} onClick={() => void start()}>{t('setup.start')}</Button> : <button type="submit" disabled={busy || (step === 'components' && (!catalog || catalog.inventory_state !== 'ready')) || (step === 'dnshome' && (!!dnsSelectionError || (draft.dns_mode === 'local' && dnsNames.mismatch) || (draft.dns_mode === 'existing' && !remoteVerified)))} className="inline-flex items-center gap-3 rounded-lg bg-primary px-5 py-2.5 font-semibold text-primary-fg hover:bg-primary/90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:opacity-60">{busy ? t('common.loading') : t(step === lastEdit ? 'setup.review' : 'setup.continue')}<ArrowRight className="h-4 w-4" aria-hidden="true" /></button>}
                        </div>
                    </div>
                </form>}
            {!completed && !hasOperation && (extending
                ? <div className="mt-4 flex flex-wrap items-start gap-x-6 gap-y-3" data-setup-extend-stop="">
                    <button type="button" disabled={extendBusy || busy} onClick={() => void extendSetup('cancel')} className="min-h-11 py-2 text-sm font-medium text-primary underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary disabled:cursor-not-allowed disabled:opacity-60">{t('setup.extend.stop')}</button>
                    <p className="min-w-0 flex-1 basis-80 py-2 text-sm leading-6 text-fg-muted">{t('setup.extend.stopHelp')}</p>
                    {extendError && <p role="alert" className="w-full max-w-2xl text-sm leading-6 text-fg">{t(extendError)}</p>}
                </div>
                : <ServerSetupManualAction compact snapshot={snapshot} onChosen={next => { accept(next); setManualExit(true); }} />)}
            </div>
        </div>}
    </ServerSetupShell>;
}

// The review of an extension says what changes against the completed setup:
// the settings that differ, what will be installed, and (folded) what is
// already installed and kept. The step list below it stays complete: those
// are the steps the run takes, checks included.
// Genisletme incelemesi degiseni one koyar; adim listesi eksiksiz kalir.
function SetupExtensionReview({ plan, changes, name }: {
    plan: ServerSetupPlan;
    // A literal value keeps the mono face; "none" is a word and does not (2026-10-16).
    changes: { key: string; label: string; before: string; after: string; beforeLiteral: boolean; afterLiteral: boolean }[];
    name: (kind: string, target: string) => string;
}) {
    const { t } = useSetupI18n();
    const installs = plan.steps.filter(item => ['service', 'runtime', 'mail_profile'].includes(item.kind));
    const kept = (plan.components || []).filter(item => item.installed);
    return <div className="mt-5 space-y-5" data-setup-extension-review="">
        <section aria-labelledby="setup-extension-changes">
            <h3 id="setup-extension-changes" className="font-semibold">{t('setup.extend.changesTitle')}</h3>
            {changes.length > 0 ? <dl className="mt-2 divide-y divide-border border-y border-border">{changes.map(change => <div key={change.key} className="grid gap-1 py-3 text-sm sm:grid-cols-[minmax(0,13rem)_minmax(0,1fr)] sm:gap-4" data-change={change.key}>
                <dt className="text-fg-muted">{change.label}</dt>
                <dd className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1"><span className={`break-all text-fg-muted ${change.beforeLiteral ? 'font-mono' : ''}`}>{change.before}</span><ArrowRight className="h-3.5 w-3.5 shrink-0 text-fg-muted" aria-hidden="true" /><span className="sr-only">→</span><span className={`break-all font-medium text-fg ${change.afterLiteral ? 'font-mono' : ''}`}>{change.after}</span></dd>
            </div>)}</dl> : <p className="mt-2 text-sm leading-6 text-fg-muted">{t('setup.extend.noChanges')}</p>}
        </section>
        <section aria-labelledby="setup-extension-installs">
            <h3 id="setup-extension-installs" className="font-semibold">{t('setup.extend.installTitle')}</h3>
            {installs.length > 0 ? <ul className="mt-2 divide-y divide-border border-y border-border">{installs.map(item => <li key={item.id} className="py-2 text-sm" data-install={item.target}>{name(item.kind, item.target)}</li>)}</ul> : <p className="mt-2 text-sm text-fg-muted">{t('setup.extend.nothingToInstall')}</p>}
        </section>
        {kept.length > 0 && <details className="text-sm">
            <summary className="cursor-pointer font-medium text-primary">{t('setup.extend.keptTitle', { count: kept.length })}</summary>
            <ul className="mt-2 divide-y divide-border">{kept.map(item => <li key={item.id} className="flex flex-wrap items-baseline justify-between gap-2 py-2"><span>{name('service', item.id)}</span><span className="text-fg-muted">{t('setup.components.keep')}</span></li>)}</ul>
        </details>}
    </div>;
}

// The Panel's secure address as a real link, with what to expect there. In the
// advance notice it is where the Panel can also be opened, not a step to take.
function SetupHandoverAddress({ lead, url }: { lead: string; url: string }) {
    const { t } = useSetupI18n();
    return <div className="mt-3 max-w-3xl space-y-1 text-sm leading-6 text-fg-muted">
        <p>{lead} <AddressLink href={`${url}/setup`} address={url} /></p>
        <p>{t('setup.handover.addressHelp')}</p>
    </div>;
}

// Advance notice of the one planned Panel restart, in the wizard's own quiet
// guidance surface and above the step list: what happens, that nobody needs to
// act, where the Panel is reachable afterwards. The step row repeats it in a line.
function SetupHandoverNotice({ host, url }: { host: string; url: string }) {
    const { t } = useSetupI18n();
    return <aside aria-labelledby="setup-handover-title" className="mt-5 rounded-lg border border-border bg-surface p-4 sm:p-5">
        <h3 id="setup-handover-title" className="font-semibold">{t('setup.handover.title')}</h3>
        <p className="mt-2 max-w-3xl break-words text-sm leading-6 text-fg-muted">{t('setup.handover.notice', { host })}</p>
        <SetupHandoverAddress lead={t('setup.handover.address')} url={url} />
    </aside>;
}

function SetupInfrastructureDNSReview({ value, compact = false }: { value: SetupInfrastructureDNSPlan; compact?: boolean }) {
    const { t } = useSetupI18n();
    return <section aria-label={t('setup.infrastructure.reviewTitle')} className="mt-5 space-y-3 text-sm">
        <h3 className="break-words font-semibold">{t('setup.infrastructure.reviewTitle')}: {value.zone}</h3>
        {!compact && <p className="max-w-3xl leading-6 text-fg-muted">{t('setup.infrastructure.reviewHelp')}</p>}
        <table className="w-full table-fixed text-left text-sm"><thead><tr className="border-b border-border"><th scope="col" className="w-[28%] pb-2 pr-3 font-medium">{t('setup.infrastructure.recordName')}</th><th scope="col" className="w-[12%] pb-2 pr-3 font-medium">{t('setup.infrastructure.recordType')}</th><th scope="col" className="w-[40%] pb-2 pr-3 font-medium">{t('setup.infrastructure.recordValue')}</th><th scope="col" className="w-[20%] pb-2 font-medium">{t('setup.infrastructure.recordAction')}</th></tr></thead><tbody className="divide-y divide-border">{value.records.map((item, index) => <tr key={`${item.name}-${item.type}-${index}`}><th scope="row" className="break-all py-3 pr-3 align-top font-normal">{item.name}</th><td className="break-words py-3 pr-3 align-top">{item.type}</td><td className="break-words py-3 pr-3 align-top [overflow-wrap:anywhere]">{item.content}<span className="mt-1 block text-xs text-fg-muted">TTL {item.ttl}</span></td><td className="break-words py-3 align-top">{t(item.action === 'keep' ? 'setup.infrastructure.keep' : 'setup.infrastructure.add')}</td></tr>)}</tbody></table>
        {!compact && <p className="leading-6 text-fg-muted">{t('setup.infrastructure.delegation')}</p>}
    </section>;
}

function SetupDNSPublisher({ execution, onBound }: { execution: ServerSetupExecution; onBound: () => Promise<void> }) {
    const { t } = useSetupI18n();
    const [connection, setConnection] = useState('');
    const [verified, setVerified] = useState(false);
    const [busy, setBusy] = useState(false);
    const [failed, setFailed] = useState(false);
    const [bound, setBound] = useState(false);
    const inFlight = useRef(false);
    const alive = useRef(true);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
    const endpoint = remoteDNSEndpoint(execution.steps.find(item => item.kind === 'dns_publisher')?.target || '');
    async function bind() {
        if (!endpoint || !connection || !verified || inFlight.current) return;
        inFlight.current = true; setBusy(true); setFailed(false);
        try {
            const response = await setupFetch('/api/v1/setup/publisher', requestOptions({ execution_id: execution.id, connection_id: connection }));
            const value = response.ok ? decodeSetupExecution(await response.json()) : null;
            if (!value || value.id !== execution.id || value.plan_id !== execution.plan_id || value.request_id !== execution.request_id) throw new Error('binding');
            if (alive.current) setBound(true);
            await onBound();
        } catch {
            // A lost reply may follow a committed binding. Poll the existing
            // execution; retrying this exact choice never creates another grant.
            if (alive.current) setFailed(true);
            await onBound();
        } finally { inFlight.current = false; if (alive.current) setBusy(false); }
    }
    return <div className="mt-6 space-y-4 border-y border-border py-5" aria-labelledby="setup-publisher-title">
        <h3 id="setup-publisher-title" className="text-lg font-semibold">{t('setup.publisher.waitingTitle')}</h3>
        <p className="text-sm leading-6 text-fg-muted">{t('setup.publisher.waitingHelp')}</p>
        {endpoint ? <>
            <fieldset disabled={busy || bound}><ServerSetupDNSConnection value={connection} requiredEndpoint={endpoint} onChange={id => { setConnection(id); setVerified(false); setFailed(false); }} onValidityChange={setVerified} /></fieldset>
            {failed && <p role="alert" className="text-sm text-danger">{t('setup.publisher.bindFailed')}</p>}
            {bound && <p role="status" className="text-sm text-fg-muted">{t('setup.publisher.bound')}</p>}
            <Button type="button" variant="primary" disabled={!connection || !verified || busy || bound} loading={busy} onClick={() => void bind()}>{t('setup.publisher.continue')}</Button>
        </> : <p role="alert" className="text-sm text-danger">{t('setup.publisher.endpointRequired')}</p>}
    </div>;
}


function SetupNodeVersion({ value, onChange }: { value: string; onChange: (value: string) => void }) {
    const { t } = useSetupI18n();
    const [releases, setReleases] = useState<{ version: string; name: string }[] | null>(null);
    const [failed, setFailed] = useState(false);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        const timeout = window.setTimeout(() => controller.abort(), 20000);
        let active = true;
        setFailed(false); setReleases(null);
        void fetch('/api/v1/runtimes/node/lts', { signal: controller.signal, cache: 'no-store' })
            .then(async response => { if (!response.ok) throw new Error('release list'); return response.json() as Promise<unknown>; })
            .then(body => {
                const list = (body as { releases?: unknown } | null)?.releases;
                if (!Array.isArray(list) || list.length === 0 || list.some(item => typeof item?.version !== 'string' || !/^\d+\.\d+\.\d+$/.test(item.version) || typeof item.name !== 'string')) throw new Error('invalid release list');
                if (active) setReleases(list);
            }).catch(() => { if (active) setFailed(true); })
            .finally(() => window.clearTimeout(timeout));
        return () => { active = false; controller.abort(); window.clearTimeout(timeout); };
    }, [attempt]);
    return <div className="space-y-3">
        <label className="block" htmlFor="setup-node_version"><span className="mb-2 block text-sm font-medium">{t('setup.nodeVersion')}</span><select id="setup-node_version" name="node_version" value={value} onChange={event => onChange(event.target.value)} required disabled={!releases} className={`${inputClass} w-full`}>
            <option value="">{t(releases ? 'setup.nodeChoose' : failed ? 'setup.nodeUnavailable' : 'common.loading')}</option>
            {value && !releases?.some(item => item.version === value) && <option value={value}>{value} — {t('setup.nodeSaved')}</option>}
            {releases?.map(item => <option key={item.version} value={item.version}>Node.js {item.version} · {item.name} LTS</option>)}
        </select></label>
        <p className="text-sm text-fg-muted">{t('setup.nodeHelp')}</p>
        {failed && <p role="alert" className="flex flex-wrap items-center gap-3 text-sm text-danger">{t('setup.nodeUnavailable')}<Button type="button" variant="secondary" onClick={() => setAttempt(value => value + 1)}>{t('common.retry')}</Button></p>}
    </div>;
}
