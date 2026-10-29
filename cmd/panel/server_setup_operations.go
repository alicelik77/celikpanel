package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/licensing"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

const serverSetupPlanVersion = 1

type serverSetupPlanStep struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Qualifier string `json:"qualifier,omitempty"`
}

type serverSetupRemoteDNSConnection struct {
	ID          string   `json:"id"`
	Endpoint    string   `json:"endpoint"`
	Nameservers []string `json:"nameservers"`
}

type serverSetupPlan struct {
	InfrastructureDNS   *serverSetupInfrastructureDNSPlan `json:"infrastructure_dns,omitempty"`
	ServerIP            string                            `json:"server_ip,omitempty"`
	ID                  string                            `json:"id"`
	Version             int                               `json:"version"`
	Revision            int                               `json:"revision"`
	Purpose             string                            `json:"purpose"`
	Steps               []serverSetupPlanStep             `json:"steps"`
	Blockers            []string                          `json:"blockers"`
	CanStart            bool                              `json:"can_start"`
	TCPPorts            []int                             `json:"tcp_ports"`
	UDPPorts            []int                             `json:"udp_ports"`
	PreserveSSH         bool                              `json:"preserve_ssh"`
	PersistFirewall     bool                              `json:"persist_firewall"`
	HostnameChange      string                            `json:"hostname_change,omitempty"`
	RemoteDNSConnection *serverSetupRemoteDNSConnection   `json:"remote_dns_connection,omitempty"`
	Draft               serverSetupDraft                  `json:"draft"`
	BuildCommit         string                            `json:"build_commit"`
	ContactEmail        string                            `json:"contact_email"`
	Actor               serviceOperationActor             `json:"actor"`
	Components          []serverSetupPlanComponent        `json:"components,omitempty"`
	// PackageUpdates (Beta-1 fix): the distribution point updates of already
	// installed packages that this plan's component installs would also
	// bring, as the package manager's simulation said at review. The review
	// lists them ("Distribution packages also updated") and the one setup
	// confirmation covers them, so a setup step does not refuse them.
	// PackageUpdatesState is "known" or "unknown" (the simulation could not
	// be read). Neither is part of the plan identity: the start check
	// compares the list itself (serverSetupPackageUpdatesCovered).
	// Kurulumun ayrıca güncelleyeceği dağıtım paketleri; tek onay kapsar.
	PackageUpdates      []transport.NotableInstall `json:"package_updates,omitempty"`
	PackageUpdatesState string                     `json:"package_updates_state,omitempty"`
	// PackageUpdatesDigest names the list (and its state) exactly as stored
	// for this plan; the start sends back the one its page showed, so an
	// update the owner never saw is never confirmed (review fix, 2026-10-29).
	PackageUpdatesDigest string `json:"package_updates_digest,omitempty"`
	// peerBound marks a plan completed by its second-server binding
	// (serverSetupBoundPlan); it is never stored or hashed.
	peerBound bool
}

type serverSetupExecutionStep struct {
	serverSetupPlanStep
	Status                      string `json:"status"`
	RequestID                   string `json:"request_id"`
	OwnerID                     string `json:"owner_id"`
	OperationID                 string `json:"operation_id,omitempty"`
	EnrollmentDispatchAttempted bool   `json:"enrollment_dispatch_attempted,omitempty"`
}

type serverSetupExecution struct {
	ID        string                     `json:"id"`
	RequestID string                     `json:"request_id"`
	PlanID    string                     `json:"plan_id"`
	Status    string                     `json:"status"`
	Phase     string                     `json:"phase"`
	Steps     []serverSetupExecutionStep `json:"steps"`
	Error     *serviceOperationError     `json:"error,omitempty"`
	Checks    []serverSetupCheck         `json:"checks,omitempty"`
	PanelURL  string                     `json:"panel_url,omitempty"`
}

var serverSetupRunners sync.Map

// serverSetupWakes holds one buffered (size 1) channel per panel. The owner's
// explicit "Check requirements again", once every check passes, pokes it so a
// runner waiting at the final verification makes its next pass now instead of
// after the rest of its 20 s sleep. A poke never starts a runner or a mutation:
// the runner alone completes setup, through its usual single pass.
// Sahibin acik yeniden kontrolu bekleyen calisanin uykusunu kisaltir; hicbir
// calisan ya da degisiklik baslatmaz, kurulumu yalniz calisan tamamlar.
var serverSetupWakes sync.Map

// serverSetupWaitingInterval is the unattended re-check cadence of a waiting run.
const serverSetupWaitingInterval = 20 * time.Second

func (p *Panel) serverSetupWakeChannel() chan struct{} {
	value, _ := serverSetupWakes.LoadOrStore(p, make(chan struct{}, 1))
	return value.(chan struct{})
}

// wakeServerSetupRunner is non-blocking: a pending poke is enough, a second
// one is dropped.
func (p *Panel) wakeServerSetupRunner() {
	select {
	case p.serverSetupWakeChannel() <- struct{}{}:
	default:
	}
}

// sleepServerSetupRunner waits for the interval or a poke, whichever is first.
func (p *Panel) sleepServerSetupRunner(interval time.Duration) {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-p.serverSetupWakeChannel():
	}
}

// wakeServerSetupVerification pokes the runner only when the latest run waits
// at its final verification and the owner's explicit check just saw every
// check pass. Any other state is left to the runner's own cadence.
func (p *Panel) wakeServerSetupVerification(ctx context.Context, checks []serverSetupCheck) {
	if !serverSetupChecksReady(checks) {
		return
	}
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil || execution == nil || execution.Status != "waiting" || execution.Phase != "verification" {
		return
	}
	p.wakeServerSetupRunner()
}

func serverSetupID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func (p *Panel) serverSetupExecutionActive(ctx context.Context) (bool, error) {
	var count int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM server_setup_executions WHERE status IN ('running','waiting')`).Scan(&count)
	return count != 0, err
}

// serverSetupExecutionMutating reports whether a setup execution holds, or is
// about to make, a change on this host. A service action, an install and a
// removal are refused (`409 server_setup_busy`) while it does.
//
// The rule (11 Oct 2026): an execution is mutating while its row is `running`,
// except when the step it is at is the public address check (`access_dns`).
// That step only asks public resolvers whether a hostname resolves here; it
// starts no ACME order and edits nothing (server_setup_access_dns.go). A setup
// that waits for a DNS record is `waiting`, and every 20 s the runner claims
// the row (`running`) to ask the resolvers again
// (claimServerSetupDNSRetry). Measured on Debian 13 and Ubuntu 24.04: during
// those moments a Stop or a Start on the Services page was refused, although
// nothing was being changed and the wait itself can last until the owner's
// DNS record exists.
//
// A row that cannot be read as an execution counts as mutating: the refusal
// is the safe answer. The other DNS waits (`primary_dns`,
// `infrastructure_dns`) are not exempted here; their re-check can publish
// native DNS records.
//
// serverSetupExecutionMutating, bir kurulum yürütmesinin bu sunucuda bir
// değişiklik tutup tutmadığını söyler. Kural: satır `running` iken yürütme
// değişiklik yapıyor sayılır; bulunduğu adım yalnızca genel adres denetimi
// (`access_dns`) ise sayılmaz, çünkü o adım yalnızca genel çözümleyicilere
// sorar ve hiçbir şeyi değiştirmez. Okunamayan satır değişiklik yapıyor sayılır.
func (p *Panel) serverSetupExecutionMutating(ctx context.Context) (bool, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT execution_json FROM server_setup_executions WHERE status='running'`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		if !serverSetupOnlyChecksPublicAddress(raw) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// serverSetupOnlyChecksPublicAddress reports whether the step a running
// execution is at is the public address check: the first step that has not
// succeeded, which is the step the runner works on (advanceServerSetupExecution).
func serverSetupOnlyChecksPublicAddress(raw string) bool {
	var execution serverSetupExecution
	if err := json.Unmarshal([]byte(raw), &execution); err != nil || len(execution.Steps) == 0 {
		return false
	}
	for _, step := range execution.Steps {
		if step.Kind == "verify" || step.Status == "succeeded" {
			continue
		}
		return step.Kind == "access_dns"
	}
	// Every step succeeded and the row is still running: the final
	// verification is being recorded. Not exempted.
	return false
}

func (p *Panel) latestServerSetupExecution(ctx context.Context) (*serverSetupExecution, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT execution_json FROM server_setup_executions ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var execution serverSetupExecution
	if err := json.Unmarshal([]byte(raw), &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (p *Panel) loadServerSetupPlan(ctx context.Context, id string) (serverSetupPlan, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT plan_json FROM server_setup_plans WHERE id=?`, id).Scan(&raw)
	if err != nil {
		return serverSetupPlan{}, err
	}
	var plan serverSetupPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return plan, err
	}
	if plan.Version != serverSetupPlanVersion || plan.ID != id || serverSetupPlanIdentity(plan) != id {
		return plan, errors.New("saved setup plan identity is invalid")
	}
	return plan, nil
}

func serverSetupPlanIdentity(plan serverSetupPlan) string {
	plan.ID = ""
	plan.PackageUpdates, plan.PackageUpdatesState, plan.PackageUpdatesDigest = nil, "", ""
	// An unread package-update list holds the start (CanStart false) but is
	// not part of what was reviewed: the same plan read again is the same plan.
	plan.CanStart = len(plan.Blockers) == 0
	encoded, _ := json.Marshal(plan)
	return serverSetupID(string(encoded))
}

func (p *Panel) handleServerSetupPlan(w http.ResponseWriter, r *http.Request) {
	if !serverSetupAdmin(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Revision int `json:"revision"`
	}
	if err := decodeServiceOperationJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.Revision != request.Revision {
		writeCodedError(w, http.StatusConflict, "server_setup_review_stale", "Setup changed. Review the current plan.", "")
		return
	}
	plan, err := p.buildServerSetupPlan(r.Context(), state, captureServiceOperationActor(r))
	if err != nil {
		writeServerError(w, err)
		return
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		writeServerError(w, err)
		return
	}
	// The same plan reviewed again keeps its identity. Its stored list is
	// replaced only while it is still unread (P5: a refreshed review reads
	// it), never once read and never for a plan a run started from. The
	// answer is the stored plan: the list the page shows is the one the
	// start compares (review fix, 2026-10-29).
	_, err = p.db.GetDB().ExecContext(r.Context(), `INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET plan_json=excluded.plan_json
		WHERE COALESCE(json_extract(server_setup_plans.plan_json,'$.package_updates_state'),'')='unknown'
		AND NOT EXISTS (SELECT 1 FROM server_setup_executions WHERE plan_id=excluded.id)`, plan.ID, plan.Revision, string(encoded), plan.Actor.UserID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		writeServerError(w, err)
		return
	}
	stored, err := p.loadServerSetupPlan(r.Context(), plan.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	stored.PackageUpdatesDigest = serverSetupPackageUpdatesDigest(stored)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stored)
}

func serverSetupAdmin(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	caller := currentCaller(r)
	if caller == nil || caller.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

// setupPDNSPairedPrimaryBlocker applies pdnsPairedPrimaryBlocker to a setup
// draft that would install PowerDNS as the paired primary on a server with no
// active DNS engine. A server whose active engine differs from the draft is
// already refused as a migration; one that matches installs nothing.
func setupPDNSPairedPrimaryBlocker(draft serverSetupDraft, state dnsEngineDBState) string {
	if draft.DNSMode != setupDNSModeLocal || draft.DNSRole != transport.DNSPairRolePrimary ||
		state.ActiveEngine != "" {
		return ""
	}
	code, _ := pdnsPairedPrimaryBlocker(dnsEngineSnapshot{
		Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRolePrimary,
		EngineEpoch: state.EngineEpoch,
	}, transport.DNSEngine(draft.DNSEngine), "install")
	return code
}

// serverSetupExecutionStateReplaceable: no step after the join step failed
// or runs an operation, and the run holds no error other than the join's own
// waits or a wait of a step that has started nothing yet, so saying the
// join's state replaces nothing the owner must see.
//
// Beta-1 re-run: the first server's run waited at primary_dns (its DNS step
// had not admitted its install) while its second server's answers had
// changed; the run kept saying primary_dns and only the peer card said the
// change. That wait cannot end before the join is fixed, so the join's state
// is the current one and is said at the top. A step with an operation (a
// DNS switch, a component install) or a known failure is never replaced.
func (p *Panel) serverSetupExecutionStateReplaceable(ctx context.Context, execution serverSetupExecution) bool {
	after, waitingStep := false, false
	for _, step := range execution.Steps {
		if step.Kind == "peer_join" {
			after = true
			continue
		}
		if !after {
			continue
		}
		if step.Status == "failed" {
			return false
		}
		if step.Status == "running" {
			if !p.serverSetupStepWaitsWithoutChild(ctx, execution, step) {
				return false
			}
			waitingStep = true
		}
	}
	return execution.Error == nil || waitingStep || stringIn(execution.Error.Code, "server_setup_peer_join_changed", "server_setup_peer_join_reopen_required", "server_setup_peer_names_waiting", "server_setup_peer_join_waiting")
}

// serverSetupStepWaitsWithoutChild: the run waits at a step that has not
// admitted anything: a DNS step whose switch does not exist, or a component
// step before its operation.
func (p *Panel) serverSetupStepWaitsWithoutChild(ctx context.Context, execution serverSetupExecution, step serverSetupExecutionStep) bool {
	if execution.Status != "waiting" || step.OperationID != "" {
		return false
	}
	switch step.Kind {
	case "dns":
		status, err := p.serverSetupDNSOperationStatus(ctx, step.RequestID)
		return err == nil && status == "missing"
	case "service", "runtime", "mail_profile":
		return p.serverSetupStepChildMissing(ctx, step)
	}
	return false
}

// serverSetupStepChildMissing: no operation exists under the step's own
// request identity. The step's OperationID can lag behind the operation (it
// is saved after the operation is created), so the operation is looked up by
// the identity it was created with. A lookup that fails is not "missing":
// the step then counts as having its operation, and nothing is replaced.
// Review fix (item 8, 2026-10-29).
func (p *Panel) serverSetupStepChildMissing(ctx context.Context, step serverSetupExecutionStep) bool {
	if step.OperationID != "" {
		return false
	}
	_, err := p.serviceOperationByRequestID(ctx, step.RequestID)
	return errors.Is(err, sql.ErrNoRows)
}

// serverSetupPDNSMainCandidateCode reads, read-only, the PowerDNS build this
// server would install and applies the catalogue's rule to it.
func (p *Panel) serverSetupPDNSMainCandidateCode(ctx context.Context) string {
	return serverSetupPDNSMainCandidateBlocker(p.readServerSetupPDNSCandidate(ctx))
}

func (p *Panel) buildServerSetupPlan(ctx context.Context, state serverSetupState, actor serviceOperationActor) (serverSetupPlan, error) {
	draft := state.Draft
	extensionChangesJoin := false
	if serverSetupJoinPending(draft) {
		// A setup that succeeded with its second server bound (an extension,
		// Beta-1 fix): the plan reuses that binding and has no new join step,
		// unless it would change what the second server received.
		bound, err := p.serverSetupSucceededPeerBinding(ctx)
		if err != nil {
			return serverSetupPlan{Draft: draft}, err
		}
		if bound != nil {
			reviewed := bound.JoinDigest
			if reviewed == "" {
				var join serverSetupJoinRecord
				if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil {
					return serverSetupPlan{Draft: draft}, err
				} else if found && join.RequestID == bound.JoinRequestID {
					reviewed = join.Digest
				}
			}
			extensionChangesJoin = reviewed != "" && reviewed != serverSetupJoinDocumentDigest(serverSetupJoinDocumentFor(draft))
			draft = serverSetupEffectiveDraft(draft, *bound)
		}
	}
	plan := serverSetupPlan{Version: serverSetupPlanVersion, Revision: state.Revision, Purpose: draft.Purpose, Draft: draft, Actor: actor,
		BuildCommit: strings.TrimSpace(buildCommit), Steps: []serverSetupPlanStep{}, Blockers: []string{}, TCPPorts: []int{panelPort(), 80}, UDPPorts: []int{}, PreserveSSH: true, PersistFirewall: true}
	addBlocker := func(code string) {
		if !slices.Contains(plan.Blockers, code) {
			plan.Blockers = append(plan.Blockers, code)
		}
	}
	addStep := func(kind, target, qualifier string) {
		plan.Steps = append(plan.Steps, serverSetupPlanStep{ID: fmt.Sprintf("%02d-%s", len(plan.Steps)+1, kind), Kind: kind, Target: target, Qualifier: qualifier})
	}
	if extensionChangesJoin {
		// Verified prerequisite: the second server has the earlier shared
		// answers, names and mail choice; this release cannot change them
		// on both after pairing.
		addBlocker("server_setup_extension_needs_second_server")
	}
	if canonical, err := hostname.CanonicalFQDN(draft.PanelDomain); err != nil || canonical != draft.PanelDomain {
		addBlocker("server_setup_panel_domain_invalid")
	}
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT email FROM users WHERE id=?`, actor.UserID).Scan(&plan.ContactEmail); err != nil {
		return plan, err
	}
	plan.ContactEmail = strings.TrimSpace(plan.ContactEmail)
	// The same validator as the certificate request the plan will make
	// (v0.1.0-alpha.99): what passes here is what the request accepts.
	if _, err := mutationpayload.CanonicalContactEmail(plan.ContactEmail); err != nil {
		addBlocker("panel_certificate_contact_email_invalid")
	}
	if _, _, blocked := panelCertificateManagementBlocker(); blocked {
		addBlocker("server_setup_panel_certificate_unmanaged")
	}
	if !setupDNSModeSupported(draft.DNSMode) {
		addBlocker("server_setup_dns_mode_unsupported")
	}

	if draft.DNSMode == setupDNSModeExisting {
		authority, err := p.remoteDNSConnectionReadiness(ctx, draft.RemoteDNSConnectionID)
		if err != nil || !authority.Ready {
			addBlocker("server_setup_remote_dns_unavailable")
		} else {
			var endpoint string
			err = p.db.GetDB().QueryRowContext(ctx, `SELECT endpoint FROM remote_dns_connections WHERE id=? AND status='ready'`, draft.RemoteDNSConnectionID).Scan(&endpoint)
			if err != nil {
				return plan, err
			}
			plan.RemoteDNSConnection = &serverSetupRemoteDNSConnection{ID: draft.RemoteDNSConnectionID, Endpoint: endpoint, Nameservers: append([]string(nil), authority.Nameservers...)}
			slices.Sort(plan.RemoteDNSConnection.Nameservers)
		}
	}
	if draft.Purpose == "dns" && draft.DNSMode != "local" {
		addBlocker("server_setup_dns_purpose_requires_local")
	}
	joinPending := serverSetupJoinPending(draft)
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return plan, err
	}
	if link != nil && !serverSetupJoinedDraftMatches(draft, *link) {
		// A joined second server keeps the answers the first server gave.
		addBlocker("server_setup_peer_join_answers_changed")
	}
	if draft.DNSMode == "local" && joinPending {
		// The second server joins by code after this plan starts: its
		// address and names are bound then (peer_join). Here only this
		// server's own identity is checked.
		if _, valid := canonicalIPv4(draft.LocalIP); !valid || !validDNSHostname(draft.NS1) || !validDNSHostname(draft.NS2) || draft.NS1 == draft.NS2 || !transport.ValidDNSEngine(transport.DNSEngine(draft.DNSEngine)) {
			addBlocker("server_setup_dns_identity_required")
		}
		if state, stateErr := readDNSEngineDBState(ctx, p.db.GetDB()); stateErr != nil {
			return plan, stateErr
		} else if state.ActiveEngine != "" {
			// This setup's own recorded engine is not "existing DNS" (P9).
			own, ownErr := p.serverSetupOwnDNSEngine(ctx, state)
			switch {
			case ownErr != nil:
				return plan, ownErr
			case !own:
				addBlocker("server_setup_existing_dns_requires_migration")
			case !serverSetupOwnDNSMatchesJoinDraft(draft, state):
				addBlocker(serverSetupDNSInstalledDiffersCode)
			}
		} else if code := setupPDNSPairedPrimaryBlocker(draft, state); code != "" {
			addBlocker(code)
		} else if draft.DNSRole == transport.DNSPairRolePrimary && transport.DNSEngine(draft.DNSEngine) == transport.DNSEnginePowerDNS && !serverSetupPDNSMainSystemTested(p.managedServiceHostProfile()) {
			addBlocker("server_setup_pdns_main_untested_system")
		} else if draft.DNSRole == transport.DNSPairRolePrimary && transport.DNSEngine(draft.DNSEngine) == transport.DNSEnginePowerDNS {
			if code := p.serverSetupPDNSMainCandidateCode(ctx); code != "" {
				addBlocker(code)
			}
		}
	} else if draft.DNSMode == "local" {
		request, local, identityErr := setupDNSIdentity(draft)
		if identityErr != nil {
			addBlocker("server_setup_dns_identity_required")
		} else {
			state, stateErr := readDNSEngineDBState(ctx, p.db.GetDB())
			if stateErr != nil {
				return plan, stateErr
			}
			if state.ActiveEngine != "" && !setupDNSDraftMatchesState(draft, request, local, state) {
				own, ownErr := p.serverSetupOwnDNSEngine(ctx, state)
				if ownErr != nil {
					return plan, ownErr
				}
				if own {
					addBlocker(serverSetupDNSInstalledDiffersCode)
				} else {
					addBlocker("server_setup_existing_dns_requires_migration")
				}
			}
			// Setup installs the first engine only. The paired PowerDNS
			// primary is decided by the same policy as the DNS engine card,
			// here at review time, before any DNS identity is saved.
			if code := setupPDNSPairedPrimaryBlocker(draft, state); code != "" {
				addBlocker(code)
			} else if draft.DNSRole == transport.DNSPairRolePrimary && state.ActiveEngine == "" &&
				transport.DNSEngine(draft.DNSEngine) == transport.DNSEnginePowerDNS &&
				!serverSetupPDNSMainSystemTested(p.managedServiceHostProfile()) {
				// Same rule as the catalogue: refused at review, not mid-run.
				addBlocker("server_setup_pdns_main_untested_system")
			} else if draft.DNSRole == transport.DNSPairRolePrimary && state.ActiveEngine == "" &&
				transport.DNSEngine(draft.DNSEngine) == transport.DNSEnginePowerDNS {
				if code := p.serverSetupPDNSMainCandidateCode(ctx); code != "" {
					addBlocker(code)
				}
			}
			if serverSetupSecondaryHosting(draft) {
				endpoint, err := canonicalRemoteDNSEndpoint(draft.DNSPublisherEndpoint)
				if err != nil || endpoint != draft.DNSPublisherEndpoint {
					addBlocker("server_setup_dns_publisher_endpoint_required")
				}
			}
		}
	}
	var installedIDs []string
	if err := p.callAgentContext(ctx, "Agent.InstalledServiceIDsStrict", &transport.Empty{}, &installedIDs); err != nil {
		return plan, err
	}
	var existingFirewall FirewallStatusResp
	if err := p.callAgentContext(ctx, "Agent.FirewallStatus", &transport.Empty{}, &existingFirewall); err != nil {
		return plan, err
	}
	if existingFirewall.Error != "" {
		// A host condition the Agent reported is a plan blocker with a stable
		// code and the next action, never an internal error (D-024).
		blocker := setupFirewallStatusBlocker(existingFirewall)
		log.Printf("setup plan blocked by firewall status %s: %s", blocker, boundedSetupHostReason(existingFirewall.Error))
		addBlocker(blocker)
	}
	plan.TCPPorts = append(plan.TCPPorts, existingFirewall.TCPPorts...)
	plan.TCPPorts = append(plan.TCPPorts, existingFirewall.SSHPorts...)
	plan.UDPPorts = append(plan.UDPPorts, existingFirewall.UDPPorts...)
	installed := make(map[string]bool, len(installedIDs))
	for _, id := range installedIDs {
		installed[id] = true
	}
	if draft.Customization != nil && serverSetupHasComponent(draft, "node") {
		var versions transport.NodeVersionsResponse
		if err := p.callAgentContext(ctx, "Agent.ListNodeVersions", &transport.Empty{}, &versions); err != nil {
			return plan, err
		}
		for _, version := range versions.Installed {
			if strings.TrimPrefix(version, "v") == strings.TrimPrefix(draft.NodeVersion, "v") {
				installed["node"] = true
			}
		}
	}
	planned := make(map[string]bool, len(installed))
	for id := range installed {
		planned[id] = true
	}
	host := p.managedServiceHostProfile()
	if draft.DNSMode == "local" {
		managed := core.GetManagedServiceByID(draft.DNSEngine)
		if managed == nil {
			addBlocker("server_setup_dns_engine_unsupported")
		} else if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
			addBlocker("server_setup_dns_engine_unsupported")
		}
	}
	addService := func(id string) {
		// A version the owner chose (alpha.111) is the step's exact package;
		// a draft without one keeps its empty qualifier and its identity.
		qualifier := ""
		if draft.Customization != nil {
			qualifier = draft.Customization.Versions[id]
		}
		managed := core.GetManagedServiceByID(id)
		if managed == nil {
			addBlocker("server_setup_service_unknown")
			return
		}
		if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
			addBlocker("server_setup_service_unsupported:" + id)
		}
		if missing := core.RequirementsMissing(managed, planned); len(missing) > 0 {
			addBlocker("server_setup_dependency_missing:" + id)
		}
		if taken := core.SeatTakenBy(managed, planned); taken != "" {
			addBlocker("server_setup_service_conflict:" + id + ":" + taken)
		}
		if qualifier != "" && !installed[id] {
			// A vendor version needs its repository enabled and offering
			// that package now: refused at review (unmet prerequisite or
			// unknown), never discovered mid-run.
			observation := serverSetupRepoObservation{State: "unknown"}
			if managed.Repo != nil {
				observation = p.readServerSetupRepoObservation(ctx, managed.Repo.ID)
			}
			switch {
			case observation.State == "unknown":
				addBlocker("server_setup_version_repository_unknown:" + id)
			case observation.State != "enabled" || !slices.Contains(observation.Packages, qualifier):
				addBlocker("server_setup_version_repository_required:" + id)
			}
		}
		if !installed[id] {
			addStep("service", id, qualifier)
		}
		planned[id] = true
	}
	// The wizard's own answers (alpha.111) add refusals only to drafts that
	// carry them; an older draft has neither and keeps its review.
	// Sihirbazin yanitlari yalniz onlari tasiyan taslaga ret ekler.
	if (draft.MainDNSServer == "this" && (draft.DNSMode != setupDNSModeLocal || draft.DNSRole != transport.DNSPairRolePrimary)) ||
		(draft.MainDNSServer == "peer" && (draft.DNSMode != setupDNSModeLocal || draft.DNSRole != transport.DNSPairRoleSecondary || draft.ServerCount != 2)) {
		addBlocker("server_setup_main_dns_server_mismatch")
	}
	addStep("dns", draft.DNSMode, "")
	mailDependent := []string{}
	if draft.Customization != nil {
		resolved, err := serverSetupResolvedComponents(draft)
		if err != nil {
			addBlocker("server_setup_service_unknown")
		}
		if len(resolved) == 0 && !(draft.Purpose == "custom" && draft.DNSMode == "local") && draft.Purpose != "dns" {
			addBlocker("server_setup_components_required")
		}
		for _, id := range resolved {
			plan.Components = append(plan.Components, serverSetupPlanComponent{ID: id, Selected: slices.Contains(draft.Customization.Components, id), Required: !slices.Contains(draft.Customization.Components, id), Installed: installed[id]})
			switch id {
			case "postfix", "dovecot", "rspamd", "roundcube":
				// These components are installed only by the audited mail profile.
			case "node":
				if !nodeSemverRe.MatchString(draft.NodeVersion) {
					addBlocker("server_setup_node_version_required")
				}
				managed := core.GetManagedServiceByID(id)
				if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
					addBlocker("server_setup_service_unsupported:" + id)
				}
				if len(core.RequirementsMissing(managed, planned)) != 0 {
					addBlocker("server_setup_dependency_missing:" + id)
				}
				addStep("runtime", "node", draft.NodeVersion)
				planned[id] = true
			default:
				// A service that needs a mail server (SpamAssassin) follows
				// the mail profile that installs Postfix and Dovecot.
				if serverSetupMailDependent(id) {
					mailDependent = append(mailDependent, id)
					continue
				}
				addService(id)
			}
		}
		for _, id := range serverSetupRequiredComponents {
			plan.Components = append(plan.Components, serverSetupPlanComponent{ID: id, Required: true, Installed: installed[id]})
		}
		if serverSetupNeedsNativeCron(draft) {
			plan.Components = append(plan.Components, serverSetupPlanComponent{ID: core.NativeCronServiceID, Required: true, Installed: installed[core.NativeCronServiceID]})
		}
	} else {
		switch draft.Purpose {
		case "web", "web_mail":
			addService("nginx")
			addService("php-fpm")
			addService("mariadb")
		case "application":
			addService("nginx")
			if !nodeSemverRe.MatchString(draft.NodeVersion) {
				addBlocker("server_setup_node_version_required")
			}
			if managed := core.GetManagedServiceByID("node"); managed != nil {
				if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
					addBlocker("server_setup_service_unsupported:node")
				}
			}
			addStep("runtime", "node", draft.NodeVersion)
			if draft.Database != "" && draft.Database != "none" {
				if draft.Database != "mariadb" && draft.Database != "postgresql" {
					addBlocker("server_setup_database_invalid")
				} else {
					addService(draft.Database)
				}
			}
		case "dns":
		default:
			addBlocker("server_setup_purpose_invalid")
		}
	}
	if serverSetupHostsSites(draft) {
		if code := serverSetupHostingRootBlockerCode(); code != "" {
			addBlocker(code)
		}
	}
	mailProfiles := serverSetupMailProfileIDs(draft)
	if len(mailProfiles) > 0 {
		if canonical, err := hostname.CanonicalFQDN(draft.MailHostname); err != nil || canonical != draft.MailHostname {
			addBlocker("server_setup_mail_hostname_invalid")
		}
		for _, profileID := range mailProfiles {
			profile, _ := mailProfileByID(profileID)
			for _, id := range profile.Services {
				managed := core.GetManagedServiceByID(id)
				if _, reason := core.ManagedServiceInstallBlockForHost(managed, host); reason != "" {
					addBlocker("server_setup_service_unsupported:" + id)
				}
				if draft.Customization != nil && len(core.RequirementsMissing(managed, planned)) > 0 {
					addBlocker("server_setup_dependency_missing:" + id)
				}
				if taken := core.SeatTakenBy(managed, planned); taken != "" {
					addBlocker("server_setup_service_conflict:" + id + ":" + taken)
				}
				planned[id] = true
			}
			addStep("mail_profile", profileID, "")
		}
	}
	for _, id := range mailDependent {
		addService(id)
	}
	// alpha.112: the main domain's DKIM key, signing and record follow the
	// mail profile that installs the mail server.
	if serverSetupNeedsMailDNSStep(draft) {
		addStep("mail_dns", draft.MainDomain, "")
	} else if serverSetupPeerMailDNSStep(draft, link != nil && link.Document.First.MainDomainMail) && (link != nil || joinPending) {
		addStep("mail_dns", draft.MainDomain, "")
	}
	if serverSetupNeedsNativeCron(draft) {
		addService(core.NativeCronServiceID)
	}
	addService("nftables")
	addService("certbot")
	for id := range planned {
		if service := core.GetManagedServiceByID(id); service != nil {
			for _, port := range service.FirewallPorts {
				if port.Proto == "udp" {
					plan.UDPPorts = append(plan.UDPPorts, port.Port)
				} else {
					plan.TCPPorts = append(plan.TCPPorts, port.Port)
				}
			}
		}
	}
	if draft.DNSMode == "local" {
		plan.TCPPorts = append(plan.TCPPorts, 53)
		plan.UDPPorts = append(plan.UDPPorts, 53)
	}
	slices.Sort(plan.TCPPorts)
	plan.TCPPorts = slices.Compact(plan.TCPPorts)
	slices.Sort(plan.UDPPorts)
	plan.UDPPorts = slices.Compact(plan.UDPPorts)
	addStep("firewall", "enable_and_persist", "")
	plan.ServerIP, _ = canonicalIPv4(serverPrimaryIP())
	if draft.DNSMode == setupDNSModeLocal {
		plan.ServerIP, _ = canonicalIPv4(draft.LocalIP)
	}
	if plan.ServerIP == "" {
		addBlocker("server_setup_panel_dns_address_unavailable")
	}
	if draft.InfrastructureDNS != nil {
		var infrastructureErr error
		reviewed := draft
		if joinPending {
			// The records are computed again when the second server is bound;
			// here they are checked with a stand-in address for it.
			reviewed = serverSetupEffectiveDraft(draft, serverSetupPeerBinding{PeerIPv4: serverSetupJoinStandInPeer(draft.LocalIP)})
		}
		plan.InfrastructureDNS, infrastructureErr = p.buildServerSetupInfrastructureDNSPlan(ctx, reviewed)
		if infrastructureErr != nil {
			var reviewError *serverSetupInfrastructureDNSPlanError
			if !errors.As(infrastructureErr, &reviewError) {
				return plan, infrastructureErr
			}
			// A typed refusal keeps its own code (the peer's mail host name,
			// 2026-10-16); every other review error stays the conflict.
			// Tipli ret kendi kodunu korur; digerleri cakisma olarak kalir.
			code := "infrastructure_dns_conflict"
			if strings.HasPrefix(reviewError.Code, "infrastructure_dns_peer_mail_") || reviewError.Code == "infrastructure_dns_imported_clash" {
				code = reviewError.Code
			}
			addBlocker("server_setup_" + code + ":" + reviewError.Message)
		}
		if plan.InfrastructureDNS != nil {
			addStep("infrastructure_dns", plan.InfrastructureDNS.Zone, "")
			if joinPending {
				plan.InfrastructureDNS = nil
			}
		}
	}
	addStep("access_dns", draft.PanelDomain, plan.ServerIP)
	cert := currentPanelCert()
	certificateRequired := serverSetupPanelCertificateChangesRequired(cert, draft.PanelDomain, plan.Steps)
	if !certificateRequired {
		var renewal transport.PanelRenewalReadinessResponse
		if err := p.callAgentContext(ctx, "Agent.PanelRenewalReadiness", &transport.PanelRenewalReadinessRequest{Domain: draft.PanelDomain}, &renewal); err != nil {
			addBlocker("server_setup_panel_renewal_unavailable")
		} else {
			certificateRequired = !renewal.Ready
		}
	}
	if certificateRequired {
		addStep("panel_certificate", draft.PanelDomain, "")
	}
	if len(mailProfiles) > 0 {
		var agent transport.AgentVersionResponse
		if err := p.callAgentContext(ctx, "Agent.Version", &transport.Empty{}, &agent); err != nil {
			return plan, err
		}
		if err := requireKnownAgentCapabilities(agent.Capabilities, transport.AgentCapabilityMailHostCertificateV1); err != nil {
			addBlocker("server_setup_mail_certificate_unavailable")
		}
		addStep("access_dns", draft.MailHostname, plan.ServerIP)
		addStep("mail_certificate", draft.MailHostname, "")
		var renewal transport.MailEnrollmentPreviewResponse
		renewalErr := p.callAgentContext(ctx, "Agent.MailEnrollmentPreviewV1", &transport.MailEnrollmentSourceRequest{ExpectedBuildCommit: plan.BuildCommit}, &renewal)
		generation, previewErr := setupMailEnrollmentPlanGeneration(renewal, plan.BuildCommit)
		if renewalErr == nil && renewal.State == "waiting" && renewal.Reason == "mail_enrollment_native_busy" {
			addBlocker("server_setup_mail_enrollment_busy")
		} else if renewalErr != nil || previewErr != nil {
			addBlocker("server_setup_mail_enrollment_unavailable")
		} else if generation != "" {
			addStep("mail_enrollment", "mail-renewal", generation)
		}
	}
	addStep("verify", draft.Purpose, "")
	plan.Steps = serverSetupDNSBootstrapSteps(draft, plan.Steps)
	switch {
	case joinPending && draft.DNSMode == setupDNSModeLocal:
		plan.Steps = serverSetupPeerJoinSteps(plan.Steps, serverSetupPlanStep{Kind: "peer_join", Target: "second"})
	case link != nil:
		plan.Steps = serverSetupPeerJoinSteps(plan.Steps, serverSetupPlanStep{Kind: "peer_names", Target: link.Address})
	}
	plan.CanStart = len(plan.Blockers) == 0
	if plan.CanStart {
		plan.PackageUpdates, plan.PackageUpdatesState = p.serverSetupPackageUpdates(ctx, plan.Steps)
		// Beta-1 re-run P5: the one confirmation covers a list the owner has
		// read. While it is unread (the package manager was busy) setup does
		// not start; refreshing the review reads it again.
		if plan.PackageUpdatesState == "unknown" {
			plan.CanStart = false
		}
	}
	plan.ID = serverSetupPlanIdentity(plan)
	plan.PackageUpdatesDigest = serverSetupPackageUpdatesDigest(plan)
	return plan, nil
}

// serverSetupPackageUpdatesDigest: the package-update list and its state as
// one value, order-independent.
func serverSetupPackageUpdatesDigest(plan serverSetupPlan) string {
	lines := make([]string, 0, len(plan.PackageUpdates))
	for _, item := range plan.PackageUpdates {
		lines = append(lines, item.Package+"="+item.From+"="+item.To)
	}
	slices.Sort(lines)
	return serverSetupID(append([]string{"package-updates", plan.PackageUpdatesState}, lines...)...)
}

// serverSetupPackageUpdateBound bounds the review's read-only simulation.
const serverSetupPackageUpdateBound = 20 * time.Second

// serverSetupPackageUpdatesUnreadCode: the review could not read which
// installed distribution packages the plan's installs would also update, so
// it cannot be confirmed yet (D-024: an unmet prerequisite; who: the panel;
// next: wait or refresh the review; then: confirm and start).
const (
	serverSetupPackageUpdatesUnreadCode    = "server_setup_distribution_updates_unread"
	serverSetupPackageUpdatesUnreadMessage = "CelikPanel could not read yet which installed distribution packages this plan's installs would also update (the package manager may be busy), so setup cannot start on this review. Nobody needs to fix anything: wait a moment and refresh the review; it reads the list again without changing the server. Then confirm it and start setup."
)

// serverSetupPackageUpdates asks the package manager, read-only, which
// installed distribution packages the plan's component installs would also
// update within this release. Anything else notable is not listed here: a
// setup step still refuses it as before. A failed or busy read is "unknown".
func (p *Panel) serverSetupPackageUpdates(ctx context.Context, steps []serverSetupPlanStep) ([]transport.NotableInstall, string) {
	preview := p.serverSetupInstallPreview
	if preview == nil {
		preview = func(ctx context.Context, request transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
			return installPackagePreview(p, ctx, request)
		}
	}
	var batch []transport.ServiceInstallPreviewStep
	for _, step := range steps {
		if step.Kind == "service" && !managedDNSEngineServiceID(step.Target) && core.GetManagedServiceByID(step.Target) != nil {
			batch = append(batch, transport.ServiceInstallPreviewStep{ID: step.Target, Package: step.Qualifier})
		}
	}
	if len(batch) == 0 {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, serverSetupPackageUpdateBound)
	defer cancel()
	updates := []transport.NotableInstall{}
	seen := map[string]bool{}
	for start := 0; start < len(batch); start += maxInstallPreviewSteps {
		end := min(start+maxInstallPreviewSteps, len(batch))
		resp, err := preview(ctx, transport.ServiceInstallPreviewRequest{Steps: batch[start:end]})
		if err != nil || resp.State != "known" {
			return nil, "unknown"
		}
		for _, item := range resp.Notable {
			if item.PointUpdate && !seen[item.Package] {
				seen[item.Package] = true
				updates = append(updates, transport.NotableInstall{Package: item.Package, Reason: item.Reason, From: item.From, To: item.To, PointUpdate: true})
			}
		}
	}
	slices.SortFunc(updates, func(a, b transport.NotableInstall) int { return strings.Compare(a.Package, b.Package) })
	return updates, "known"
}

// serverSetupAcceptedPointUpdates is what a setup step tells the Agent: the
// reviewed point updates as exact "package=version", none when the review
// could not read them.
func serverSetupAcceptedPointUpdates(plan serverSetupPlan) []string {
	if plan.PackageUpdatesState != "known" {
		return nil
	}
	accepted := make([]string, 0, len(plan.PackageUpdates))
	for _, item := range plan.PackageUpdates {
		accepted = append(accepted, item.Package+"="+item.To)
	}
	return accepted
}

// serverSetupPointUpdatesWait asks, read-only, what this component's install
// would update now. A point update the review did not list (or any, when the
// review could not read them) is a verified state for the owner; a read that
// failed is unknown and asked again. Nothing is installed in either case.
func (p *Panel) serverSetupPointUpdatesWait(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) *serverSetupPeerWait {
	updates, state := p.serverSetupPackageUpdates(ctx, []serverSetupPlanStep{step.serverSetupPlanStep})
	if state == "" {
		return nil
	}
	if state != "known" {
		return &serverSetupPeerWait{Code: "server_setup_distribution_updates_unread", Message: "CelikPanel could not read which distribution packages this component's install would also update, so nothing was installed. Nobody needs to act yet: setup asks again by itself, or you can choose Review the updates on this page to read and confirm them now; setup then continues from this step."}
	}
	listed := map[string]bool{}
	for _, item := range serverSetupAcceptedPointUpdates(plan) {
		listed[item] = true
	}
	for _, item := range updates {
		if !listed[item.Package+"="+item.To] {
			return &serverSetupPeerWait{Code: "server_setup_distribution_updates_changed", Message: "New distribution updates appeared since the review (" + item.Package + " " + item.To + "), so nothing was installed. You, the owner: choose Review the updates on this page and confirm them; setup then continues from this step, and nothing already done is repeated."}
		}
	}
	return nil
}

// serverSetupPackageUpdatesCovered: at start the reviewed list still covers
// what the package manager would update now. A newer read that names a
// package the review did not list needs a new review; an unread one does not
// (each step's install is checked again on the server).
func serverSetupPackageUpdatesCovered(reviewed, fresh serverSetupPlan) bool {
	if fresh.PackageUpdatesState != "known" {
		return true
	}
	listed := map[string]bool{}
	for _, item := range reviewed.PackageUpdates {
		listed[item.Package+"\x00"+item.To] = true
	}
	for _, item := range fresh.PackageUpdates {
		if !listed[item.Package+"\x00"+item.To] {
			return false
		}
	}
	return true
}

func (p *Panel) handleServerSetupStart(w http.ResponseWriter, r *http.Request) {
	if !serverSetupAdmin(w, r, http.MethodPost) {
		return
	}
	if !p.requireSubsystemOperational(w, degradedSubsystemServiceOperations) {
		return
	}
	var request struct {
		PlanID    string `json:"plan_id"`
		Confirmed bool   `json:"confirmed"`
		RequestID string `json:"request_id"`
		// PackageUpdatesDigest: the package-update list the page showed.
		PackageUpdatesDigest string `json:"package_updates_digest"`
	}
	if err := decodeServiceOperationJSON(w, r, &request); err != nil || !request.Confirmed || !validServiceOperationID(request.RequestID) || !validServiceOperationID(request.PlanID) {
		writeClientError(w, http.StatusBadRequest, "confirm the exact reviewed plan and provide a valid request_id")
		return
	}
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	var previous string
	err := p.db.GetDB().QueryRowContext(r.Context(), `SELECT execution_json FROM server_setup_executions WHERE request_id=?`, request.RequestID).Scan(&previous)
	if err == nil {
		var execution serverSetupExecution
		if json.Unmarshal([]byte(previous), &execution) != nil {
			writeServerError(w, errors.New("invalid saved setup operation"))
			return
		}
		if execution.PlanID != request.PlanID {
			writeCodedError(w, http.StatusConflict, "server_setup_request_conflict", "Request identity belongs to another setup plan.", "")
			return
		}
		if execution.Status == "running" || execution.Status == "waiting" {
			p.launchServerSetupExecution()
		}
		writeServerSetupExecution(w, execution, http.StatusAccepted)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeServerError(w, err)
		return
	}
	if err := p.requireServerSetupAdmission(r.Context()); err != nil {
		writeServerSetupAdmissionError(w, err, "license_required", "An active license is required to start setup.", "")
		return
	}
	plan, err := p.loadServerSetupPlan(r.Context(), request.PlanID)
	if err != nil {
		writeCodedError(w, http.StatusConflict, "server_setup_review_required", "Review the setup plan first.", "")
		return
	}
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.Revision != plan.Revision || plan.BuildCommit != strings.TrimSpace(buildCommit) || plan.Actor.UserID != currentCaller(r).ID {
		writeCodedError(w, http.StatusConflict, "server_setup_review_stale", "Setup changed. Review the current plan.", "")
		return
	}
	if request.PackageUpdatesDigest != serverSetupPackageUpdatesDigest(plan) {
		// Another review (a reload, a second tab) replaced the list after
		// this page read it: the owner confirms only a list they read.
		writeCodedError(w, http.StatusConflict, "server_setup_distribution_updates_changed_before_start", "The distribution updates this setup would also bring changed after you read the review, so setup did not start. You, the owner: read the review again; then confirm it and start setup.", "")
		return
	}
	if plan.PackageUpdatesState == "unknown" {
		// The same refusal the review's disabled button shows: a client
		// cannot start on a list nobody could read (P5).
		writeCodedError(w, http.StatusConflict, serverSetupPackageUpdatesUnreadCode, serverSetupPackageUpdatesUnreadMessage, "")
		return
	}
	fresh, err := p.buildServerSetupPlan(r.Context(), state, plan.Actor)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if fresh.ID != plan.ID || !serverSetupPackageUpdatesCovered(plan, fresh) {
		writeCodedError(w, http.StatusConflict, "server_setup_review_stale", "Server requirements changed. Review the current plan.", "")
		return
	}
	if !plan.CanStart {
		writeCodedError(w, http.StatusConflict, "server_setup_plan_blocked", "Resolve the plan requirements before starting.", "")
		return
	}
	active, err := p.serverSetupExecutionActive(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if active {
		writeCodedError(w, http.StatusConflict, "server_setup_busy", "A setup operation is already active.", "")
		return
	}
	execution := serverSetupExecution{ID: request.RequestID, RequestID: request.RequestID, PlanID: plan.ID, Status: "running", Phase: "queued", Steps: []serverSetupExecutionStep{}, PanelURL: fmt.Sprintf("https://%s:%d/", plan.Draft.PanelDomain, panelPort())}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "pending", RequestID: serverSetupID(execution.ID, step.ID, "request"), OwnerID: serverSetupID(execution.ID, step.ID, "owner")})
	}
	encoded, _ := json.Marshal(execution)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `UPDATE server_setup_state SET status='running',updated_at=? WHERE id=1 AND revision=? AND status IN ('new','legacy','draft','ready')`, now, plan.Revision)
	if err != nil {
		writeServerError(w, err)
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if changed != 1 {
		writeCodedError(w, http.StatusConflict, "server_setup_review_stale", "Setup changed. Review the current plan.", "")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, execution.ID, execution.RequestID, plan.ID, execution.Status, string(encoded), now, now)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditServiceOperation(r.Context(), captureServiceOperationActor(r), "server.setup.start:"+plan.Purpose)
	p.launchServerSetupExecution()
	writeServerSetupExecution(w, execution, http.StatusAccepted)
}

func writeServerSetupExecution(w http.ResponseWriter, execution serverSetupExecution, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(execution)
}

func (p *Panel) handleServerSetupOperation(w http.ResponseWriter, r *http.Request) {
	if !serverSetupAdmin(w, r, http.MethodGet) {
		return
	}
	var execution *serverSetupExecution
	var err error
	if requestID := r.URL.Query().Get("request_id"); requestID != "" {
		if !validServiceOperationID(requestID) {
			writeClientError(w, http.StatusBadRequest, "invalid request_id")
			return
		}
		var raw string
		err = p.db.GetDB().QueryRowContext(r.Context(), `SELECT execution_json FROM server_setup_executions WHERE request_id=?`, requestID).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else if err == nil {
			execution = &serverSetupExecution{}
			err = json.Unmarshal([]byte(raw), execution)
		}
	} else {
		execution, err = p.latestServerSetupExecution(r.Context())
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	response := p.serverSetupOperationResponse(r.Context(), execution)
	if response != nil && execution.Status == "succeeded" {
		// An unreadable setup state leaves the flag out: the page then
		// reads the setup itself, as it did before the flag existed.
		if state, err := p.loadServerSetup(r.Context()); err == nil && serverSetupRunSuperseded(state, *execution) {
			response.Superseded = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// serverSetupRunSuperseded: a succeeded run while the completed setup is being
// extended and the extension has not started (set18 D1, 2026-10-16). The run
// belongs to the setup that was completed; it is neither the extension's run
// nor a completion still to be verified. Starting the extension moves the
// state to running, so the flag never covers the extension's own run.
// Genisletilen tamamlanmis kurulumun basarili calismasi: genisletmenin degil,
// tamamlanmis kurulumun calismasidir; dogrulanacak bir tamamlanma degildir.
func serverSetupRunSuperseded(state serverSetupState, execution serverSetupExecution) bool {
	return execution.Status == "succeeded" && state.Extension && state.Status == "draft"
}

func (p *Panel) persistServerSetupExecution(ctx context.Context, execution serverSetupExecution) error {
	encoded, err := json.Marshal(execution)
	if err != nil {
		return err
	}
	result, err := p.db.GetDB().ExecContext(ctx, `UPDATE server_setup_executions SET status=?,execution_json=?,updated_at=? WHERE id=? AND status IN ('running','waiting')`, execution.Status, string(encoded), time.Now().UTC().Format(time.RFC3339Nano), execution.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("setup execution lost active state")
	}
	return nil
}

// Called once after the existing service/DNS startup reconciliation. Reads and
// browser refreshes never launch work; only a committed start or restart does.
func (p *Panel) resumeServerSetupExecutions() { p.launchServerSetupExecution() }

func (p *Panel) launchServerSetupExecution() {
	if _, loaded := serverSetupRunners.LoadOrStore(p, true); loaded {
		return
	}
	// A poke left over from an earlier run must not cut this run's first wait.
	select {
	case <-p.serverSetupWakeChannel():
	default:
	}
	go func() {
		lastID := ""
		defer func() { p.finishServerSetupRunner(lastID) }()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("server setup runner stopped: %v", recovered)
			}
		}()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			execution, err := p.latestServerSetupExecution(ctx)
			if err != nil || execution == nil || (execution.Status != "running" && execution.Status != "waiting") {
				cancel()
				return
			}
			lastID = execution.ID
			plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
			if err != nil {
				cancel()
				log.Printf("server setup saved plan: %v", err)
				return
			}
			cancel()
			progressed, err := p.advanceServerSetupExecution(plan, execution)
			if err != nil {
				log.Printf("server setup execution %s: %v", execution.ID, err)
				return
			}
			if execution.Status == "failed" || execution.Status == "succeeded" {
				return
			}
			if !progressed {
				if execution.Status == "waiting" {
					p.sleepServerSetupRunner(serverSetupWaitingInterval)
				} else {
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()
}

func (p *Panel) advanceServerSetupExecution(plan serverSetupPlan, execution *serverSetupExecution) (bool, error) {
	if err := validateServerSetupExecution(plan, *execution); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	reviewed := plan
	plan, err := p.effectiveServerSetupPlan(ctx, reviewed, execution.ID)
	if err != nil {
		return false, err
	}
	if plan.peerBound && execution.Status != "succeeded" {
		wait, unbound, err := p.reconcileServerSetupStaleBinding(ctx, reviewed, execution)
		if err != nil {
			return false, err
		}
		if wait != nil {
			// The run does not go on with answers the second server
			// replaced. A step that is running, or a known failure, keeps
			// its own phase and error (D-024); the setup link's state says
			// the change (GET /api/v1/setup/peer: changed).
			if !p.serverSetupExecutionStateReplaceable(ctx, *execution) {
				return false, nil
			}
			execution.Status, execution.Phase = "waiting", "peer_join"
			execution.Error = &serviceOperationError{Code: wait.Code, Message: wait.Message}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if unbound {
			plan = reviewed
		}
	}
	if execution.Status == "waiting" && stringIn(execution.Phase, "primary_dns", "infrastructure_dns", "access_dns") {
		if err := p.claimServerSetupDNSRetry(ctx, plan, execution); err != nil {
			return false, err
		}
	}
	for index := range execution.Steps {
		step := &execution.Steps[index]
		if step.Kind == "verify" {
			continue
		}
		if step.Status == "succeeded" {
			continue
		}
		execution.Phase = step.ID
		if step.Status == "pending" {
			step.Status = "running"
			execution.Status = "running"
			if err := p.persistServerSetupExecution(ctx, *execution); err != nil {
				return false, err
			}
		}
		var done bool
		var err error
		if step.Kind == "peer_join" {
			done, err = p.runServerSetupPeerJoin(ctx, reviewed, execution.ID)
			if done && err == nil {
				plan, err = p.effectiveServerSetupPlan(ctx, reviewed, execution.ID)
			}
		} else if step.Kind == "dns_publisher" {
			done, err = p.runServerSetupDNSPublisher(ctx, plan, execution.ID)
			if errors.Is(err, errServerSetupConflict) {
				// A join replaced under this run: a verified stop with what
				// the owner does, never the generic step failure (P8).
				if rejoined, rejoinErr := p.serverSetupRejoinedUnderRun(ctx, reviewed, execution.ID); rejoinErr == nil && rejoined {
					err = serverSetupPlanChangedByRejoin()
				}
			}
		} else if step.Kind == "mail_enrollment" {
			done, err = p.runServerSetupMailEnrollment(ctx, reviewed, execution, index)
		} else if step.Kind == "service" {
			// The updates the owner confirmed during this run join the
			// reviewed list for its component installs (P5).
			stepPlan, readErr := p.withServerSetupRunPackageUpdates(ctx, plan, execution.ID)
			if readErr != nil {
				return false, readErr
			}
			done, err = p.runServerSetupStep(ctx, stepPlan, step)
		} else {
			done, err = p.runServerSetupStep(ctx, plan, step)
		}
		var peerWait *serverSetupPeerWait
		if errors.As(err, &peerWait) {
			// A wait with its own reason (the join, the names, the link):
			// the step stays running and is asked again (D-024).
			execution.Status = "waiting"
			execution.Phase = step.Kind
			execution.Error = &serviceOperationError{Code: peerWait.Code, Message: peerWait.Message}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		var enrollmentWait *serverSetupMailEnrollmentWait
		if errors.As(err, &enrollmentWait) {
			execution.Status = "running"
			execution.Error = &serviceOperationError{Code: enrollmentWait.Code, Message: enrollmentWait.Message}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}

		var enrollmentFailure *serverSetupChildFailure
		if step.Kind == "mail_enrollment" && errors.As(err, &enrollmentFailure) && enrollmentFailure.Code == "mail_enrollment_restored" {
			// Exact terminal inverse evidence is a known outcome. An unrelated service
			// observation must not turn it back into an unknown/reconciling diagnosis.
			step.Status = "failed"
			execution.Status = "failed"
			execution.Error = &serviceOperationError{Code: enrollmentFailure.Code, Message: enrollmentFailure.Message}
			if persistErr := p.persistServerSetupExecution(ctx, *execution); persistErr != nil {
				return false, persistErr
			}
			p.releaseFailedServerSetupDraft(ctx, plan)
			return false, nil
		}

		if errors.Is(err, errServerSetupAccessDNSRequired) || errors.Is(err, errServerSetupPrimaryDNSRequired) || errors.Is(err, errServerSetupInfrastructureDNSWaiting) {
			execution.Status = "waiting"
			execution.Phase = step.Kind
			if errors.Is(err, errServerSetupPrimaryDNSRequired) {
				execution.Phase = "primary_dns"
			}
			execution.Error = serverSetupDNSWaitMessage(plan, *step, err)
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServerSetupInfrastructureDNSUnknown) {
			execution.Status = "running"
			execution.Error = &serviceOperationError{Code: "server_setup_infrastructure_dns_unknown", Message: "The DNS publication result is being checked using its saved operation. Do not start it again."}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServerSetupDNSPublisherRequired) || errors.Is(err, errServerSetupDNSReadinessRequired) {
			execution.Status = "waiting"
			execution.Phase = step.Kind
			execution.Error = &serviceOperationError{Code: "server_setup_" + step.Kind + "_required", Message: "Finish the DNS connection shown in this setup flow to continue. Completed operations and existing services are preserved."}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServerSetupLicenseRequired) {
			execution.Status = "waiting"
			execution.Phase = "license"
			execution.Error = &serviceOperationError{Code: "license_required", Message: "Activate the license to continue the remaining setup steps. Existing services keep running."}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServerSetupLicenseUnverified) {
			// Unknown, not "license required": the step waits and the runner
			// rechecks through the same refresh every 20 s.
			execution.Status = "waiting"
			execution.Error = serverSetupLicenseUnverifiedFailure(err)
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		// The Agent's verified terminal or held result for the exact DNS
		// request is shown as that result, not as an unknown (D-024). The
		// step stays unreleased: the Panel has not recorded a rollback, so a
		// new plan could not start anyway. The runner keeps rereading the
		// same request every 20 s; reading never starts a mutation.
		var agentFailed *serverSetupDNSAgentFailedError
		if step.Kind == "dns" && errors.As(err, &agentFailed) {
			execution.Status = "waiting"
			execution.Error = serverSetupDNSAgentFailedMessage(agentFailed)
			log.Printf("server setup step %s: %v", step.ID, err)
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if step.Kind == "dns" && errors.Is(err, errServerSetupDNSAgentRunning) {
			execution.Status = "running"
			execution.Error = nil
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServerSetupDNSReconciliationRequired) {
			execution.Status = "running"
			now := time.Now()
			since := time.Time{}
			if step.Kind == "dns" {
				since = p.serverSetupDNSUnknownSince(ctx, execution.ID, step.RequestID, now)
			}
			execution.Error = serverSetupDNSReconcilingError(step.RequestID, since, now)
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		if errors.Is(err, errServiceOperationBusy) {
			return false, nil
		}
		if err != nil {
			// A failed transport reply cannot prove that the admitted child did
			// not commit. Keep its exact identity and reconcile before a retry.
			active, observeErr := p.activeServiceOperation(ctx)
			job, agentErr := p.statusAgentMutation(ctx, "")
			if observeErr != nil || agentErr != nil || active != nil || (job != nil && agentMutationActive(job.Status)) {
				execution.Error = &serviceOperationError{Code: "server_setup_reconciling", Message: "The previous operation is being reconciled. Its saved identity will be checked before continuing."}
				return false, p.persistServerSetupExecution(ctx, *execution)
			}
			step.Status = "failed"
			execution.Status = "failed"
			execution.Error = serverSetupFailureForStep(*step, err)
			log.Printf("server setup step %s failed: %v", step.ID, err)
			if persistErr := p.persistServerSetupExecution(ctx, *execution); persistErr != nil {
				return false, persistErr
			}
			p.releaseFailedServerSetupDraft(ctx, plan)
			return false, nil
		}
		execution.Status = "running"
		execution.Error = nil
		if !done {
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		step.Status = "succeeded"
		execution.Error = nil
		if err := p.persistServerSetupExecution(ctx, *execution); err != nil {
			return false, err
		}
		return true, nil
	}
	// Final verification is evidence, not an installation child. Normalize older
	// receipts that marked this row successful before the readiness checks.
	// Son doğrulama kurulum işi değil, kanıttır. Eski erken başarı kaydını düzelt.
	for index := range execution.Steps {
		if execution.Steps[index].Kind == "verify" {
			execution.Steps[index].Status = "pending"
		}
	}
	execution.Phase = "verification"
	if err := p.persistServerSetupExecution(ctx, *execution); err != nil {
		return false, err
	}
	checks, err := p.serverSetupCompletionChecks(ctx, plan.Draft)
	if err != nil {
		return false, err
	}
	execution.Checks = checks
	if !serverSetupChecksReady(checks) {
		execution.Status = "waiting"
		return false, p.persistServerSetupExecution(ctx, *execution)
	}
	if err := p.completeServerSetupExecution(ctx, plan.Revision, execution.ID); err != nil {
		var wait *serverSetupPeerWait
		if errors.As(err, &wait) {
			// Not complete, with its reason; the runner checks again.
			execution.Status = "waiting"
			execution.Error = &serviceOperationError{Code: wait.Code, Message: wait.Message}
			return false, p.persistServerSetupExecution(ctx, *execution)
		}
		return false, err
	}
	for index := range execution.Steps {
		if execution.Steps[index].Kind == "verify" {
			execution.Steps[index].Status = "succeeded"
		}
	}
	execution.Status = "succeeded"
	execution.Phase = "complete"
	execution.Error = nil
	return true, p.persistServerSetupExecution(ctx, *execution)
}

func (p *Panel) runServerSetupStep(ctx context.Context, plan serverSetupPlan, step *serverSetupExecutionStep) (bool, error) {
	switch step.Kind {
	case "peer_names":
		if err := p.requireServerSetupAdmission(ctx); err != nil {
			return false, err
		}
		return p.runServerSetupPeerNames(ctx, plan, *step)
	case "access_dns":
		return p.runServerSetupAccessDNS(ctx, plan, *step)
	case "infrastructure_dns":
		return p.runServerSetupInfrastructureDNS(ctx, plan, *step)
	case "dns_readiness":
		if err := p.requireServerSetupAdmission(ctx); err != nil {
			return false, err
		}
		if serverSetupManualSecondaryHosting(plan.Draft) {
			ready, err := p.serverSetupManualSecondaryHostingReadiness(ctx, plan.Draft)
			if err != nil || !ready {
				return false, errServerSetupDNSReadinessRequired
			}
			return true, nil
		}
		if serverSetupStandaloneDNS(plan.Draft) {
			// One server keeping its own DNS: its engine and both public
			// nameserver names, no pair and no publishing connection.
			if ready, err := p.setupDNSModeReadiness(ctx, setupDNSModeLocal); err != nil || !ready {
				return false, errServerSetupDNSReadinessRequired
			}
			if ready, err := p.setupDNSNameserverReadiness(ctx); err != nil || !ready {
				return false, errServerSetupDNSReadinessRequired
			}
			return true, nil
		}
		if _, err := p.remoteDNSLocalAuthority(ctx); err != nil {
			return false, errServerSetupDNSReadinessRequired
		}
		return true, nil
	case "dns":
		if plan.Draft.DNSMode == "external" {
			if err := p.requireServerSetupAdmission(ctx); err != nil {
				return false, err
			}
			return true, p.saveSetupDNSManagementMode(ctx, "external")
		}

		if plan.Draft.DNSMode == setupDNSModeExisting {
			if err := p.requireServerSetupAdmission(ctx); err != nil {
				return false, err
			}
			if plan.RemoteDNSConnection == nil || plan.RemoteDNSConnection.ID != plan.Draft.RemoteDNSConnectionID {
				return false, errors.New("reviewed remote DNS identity is missing")
			}
			authority, err := p.remoteDNSConnectionReadiness(ctx, plan.Draft.RemoteDNSConnectionID)
			if err != nil || !authority.Ready {
				return false, errors.New("reviewed remote DNS authority is unavailable")
			}
			names := append([]string(nil), authority.Nameservers...)
			slices.Sort(names)
			var endpoint string
			if err := p.db.GetDB().QueryRowContext(ctx, `SELECT endpoint FROM remote_dns_connections WHERE id=? AND status='ready'`, plan.Draft.RemoteDNSConnectionID).Scan(&endpoint); err != nil {
				return false, err
			}
			if endpoint != plan.RemoteDNSConnection.Endpoint || !slices.Equal(names, plan.RemoteDNSConnection.Nameservers) {
				return false, errors.New("remote DNS identity changed after review")
			}
			return true, p.saveSetupRemoteDNSConnection(ctx, plan.Draft.RemoteDNSConnectionID)
		}
		state, err := p.serverSetupDNSOperationStatus(ctx, step.RequestID)
		if err != nil {
			return false, setupDNSReconciliationError(err)
		}
		if state == "missing" {
			if err := p.requireServerSetupAdmission(ctx); err != nil {
				return false, err
			}
		}
		return true, p.startServerSetupDNS(ctx, plan.Draft, step.RequestID, plan.Actor)
	case "mail_certificate":
		return p.runServerSetupMailCertificate(ctx, plan, *step)
	case "mail_dns":
		return p.runServerSetupMailDNS(ctx, plan, *step)
	case "firewall":
		return p.runServerSetupFirewall(ctx, plan, *step)
	case "service", "runtime", "mail_profile", "panel_certificate":
		op, err := p.ensureServerSetupChild(ctx, plan, *step)
		if err != nil {
			return false, err
		}
		step.OperationID = op.ID
		switch op.Status {
		case serviceOperationSucceeded:
			return true, nil
		case serviceOperationFailed:
			if op.Error != nil {
				return false, &serverSetupChildFailure{Code: op.Error.Code, Message: op.Error.Message,
					Component: op.Error.Component, Step: op.Error.Step, Detail: op.Error.Detail,
					Reason: op.Error.Reason}
			}
			return false, errors.New("setup child failed")
		default:
			return false, nil
		}
	default:
		return false, errors.New("unknown persisted setup step")
	}
}

func (p *Panel) ensureServerSetupChild(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (serviceOperation, error) {
	kind := serviceOperationKindInstall
	switch step.Kind {
	case "runtime":
		kind = serviceOperationKindRuntimeInstall
	case "mail_profile":
		kind = serviceOperationKindMailProfileInstall
	case "panel_certificate":
		kind = serviceOperationKindPanelCertificate
	}
	qualifier := step.Qualifier
	var certificate mutationpayload.PanelCertificateIssueCommitment
	if step.Kind == "panel_certificate" {
		var err error
		certificate, err = mutationpayload.CanonicalPanelCertificateIssue(plan.Draft.PanelDomain, plan.ContactEmail, tlsDir(), plan.BuildCommit)
		if err != nil {
			return serviceOperation{}, err
		}
		qualifier = certificate.Qualifier
	}
	prior, found, err := p.idempotentServiceOperation(ctx, step.RequestID, kind, step.Target, qualifier)
	if err != nil || found {
		return prior, err
	}
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return serviceOperation{}, err
	}
	if plan.BuildCommit != "" && plan.BuildCommit != strings.TrimSpace(buildCommit) {
		return serviceOperation{}, errServerSetupBuildChanged
	}
	if step.Kind == "service" {
		// Before a component's first install the package manager is asked
		// again: a point update the review did not list is never accepted.
		if wait := p.serverSetupPointUpdatesWait(ctx, plan, step); wait != nil {
			return serviceOperation{}, wait
		}
	}
	if _, degraded := p.subsystemDegraded(degradedSubsystemServiceOperations); degraded {
		return serviceOperation{}, errors.New("service operation recovery is required")
	}
	if !p.serviceMutationMu.TryLock() {
		return serviceOperation{}, errServiceOperationBusy
	}
	release := p.serviceMutationMu.Unlock
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()
	if active, err := p.activeServiceOperation(ctx); err != nil {
		return serviceOperation{}, err
	} else if active != nil {
		return serviceOperation{}, errServiceOperationBusy
	}
	job, err := p.statusAgentMutation(ctx, "")
	if err != nil {
		return serviceOperation{}, err
	}
	if job != nil && agentMutationActive(job.Status) {
		return serviceOperation{}, errServiceOperationBusy
	}
	if step.Kind == "panel_certificate" {
		if _, _, blocked := panelCertificateManagementBlocker(); blocked {
			return serviceOperation{}, errors.New("panel certificate path is unmanaged")
		}
		if err := p.requireMatchingAgentBuild(ctx); err != nil {
			return serviceOperation{}, err
		}
		if err := p.requirePanelCertificateSagaAgentCapabilities(ctx); err != nil {
			return serviceOperation{}, err
		}
		for _, method := range []string{"Agent.ApplyFirewallV2", "Agent.IssuePanelCertificateV2"} {
			if err := p.authorizeAgentRPCContext(ctx, method); err != nil {
				return serviceOperation{}, err
			}
		}
		identity := serviceOperation{RequestID: step.RequestID, Kind: kind, ServiceID: step.Target, PackageName: qualifier, Status: serviceOperationQueued, Phase: panelCertificatePhaseQueued}
		data, err := canonicalPanelCertificateSagaData(identity, newPanelCertificateSagaData(certificate))
		if err != nil {
			return serviceOperation{}, err
		}
		op, err := p.createServiceOperationRequestWithState(ctx, kind, step.Target, qualifier, step.RequestID, plan.Actor, panelCertificatePhaseQueued, data)
		if errors.Is(err, errServiceOperationReplay) {
			return op, nil
		}
		if err != nil {
			return serviceOperation{}, err
		}
		p.launchPanelCertificateSaga(op, plan.Actor, release)
		transferred = true
		return op, nil
	}
	if step.Kind == "mail_profile" {
		if err := p.authorizeAgentRPCContext(ctx, "Agent.SyncMailTLSV2"); err != nil {
			return serviceOperation{}, err
		}
		if err := p.requireMailTLSSyncV2Agent(ctx); err != nil {
			return serviceOperation{}, err
		}
		if err := p.setSetting(ctx, settingMailHostname, plan.Draft.MailHostname); err != nil {
			return serviceOperation{}, err
		}
	}
	operationData := ""
	if step.Kind == "mail_profile" {
		raw, _ := json.Marshal(serverSetupMailChild{MailHostname: plan.Draft.MailHostname})
		operationData = string(raw)
	}
	op, err := p.createServiceOperationRequestWithState(ctx, kind, step.Target, qualifier, step.RequestID, plan.Actor, "queued", operationData)
	if errors.Is(err, errServiceOperationReplay) {
		return op, nil
	}
	if err != nil {
		return serviceOperation{}, err
	}
	var runner serviceOperationRunner
	switch step.Kind {
	case "service":
		runner = func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			// The setup's one confirmation covers exactly the distribution
			// point updates its review listed; every other notable install
			// is still refused by the Agent.
			return p.runServiceInstall(ctx, serviceInstallRequest{ServiceID: step.Target, Package: step.Qualifier, RequestID: step.RequestID, AcceptedPointUpdates: serverSetupAcceptedPointUpdates(plan)}, advance)
		}
	case "runtime":
		runner = func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runNodeInstall(ctx, step.Qualifier, advance)
		}
	case "mail_profile":
		runner = func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runMailProfileInstall(context.WithValue(ctx, serverSetupMailHostnameKey{}, plan.Draft.MailHostname), step.Target, advance)
		}
	default:
		return serviceOperation{}, errors.New("unknown service setup child")
	}
	p.launchServiceOperation(op, plan.Actor, "queued", "server.setup.child:"+step.Target, "server.setup.child.failed:"+step.Target, release, runner)
	transferred = true
	return op, nil
}

func (p *Panel) runServerSetupFirewall(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (bool, error) {
	commitment, err := mutationpayload.CanonicalFirewallApply(true, true, plan.TCPPorts, plan.UDPPorts)
	if err != nil {
		return false, err
	}
	job, err := p.statusAgentMutation(ctx, step.RequestID)
	if err != nil {
		return false, err
	}
	if job != nil && agentMutationActive(job.Status) {
		return false, errServiceOperationBusy
	}
	if job != nil && job.Status == agentMutationFailed {
		return false, errors.New("interrupted firewall operation requires a new reviewed attempt")
	}
	if job != nil && job.Status == agentMutationSucceeded {
		identity := agentMutationIdentityForOperation(serviceOperation{RequestID: step.RequestID, Kind: "firewall_apply", ServiceID: "nftables", PackageName: commitment.Qualifier}, step.OwnerID)
		if err := validateAgentMutationSucceededReceipt(job, identity); err != nil {
			return false, err
		}
		return true, nil
	}
	if !p.serviceMutationMu.TryLock() {
		return false, errServiceOperationBusy
	}
	defer p.serviceMutationMu.Unlock()
	if active, err := p.activeServiceOperation(ctx); err != nil {
		return false, err
	} else if active != nil {
		return false, errServiceOperationBusy
	}
	panelFirewallMu.Lock()
	defer panelFirewallMu.Unlock()
	var current FirewallStatusResp
	if err := p.callAgentContext(ctx, "Agent.FirewallStatus", &transport.Empty{}, &current); err != nil {
		return false, err
	}
	if current.Error != "" {
		if current.ErrorCode == transport.FirewallStatusHostRestartRequired {
			return false, fmt.Errorf("%w: %s", errServerSetupHostRestartRequired, boundedSetupHostReason(current.Error))
		}
		return false, errors.New(current.Error)
	}
	if !current.EngineAvailable {
		return false, errFirewallNoEngine
	}
	if err := firewallSSHDiscoveryError(current.SSHDiscoveryReason); err != nil {
		return false, err
	}
	if len(current.SSHPorts) == 0 {
		return false, errFirewallSSHUnprovable
	}
	currentTCP, currentUDP, err := p.desiredFirewallPorts(80)
	if err != nil {
		return false, err
	}
	for _, port := range append(currentTCP, current.TCPPorts...) {
		if !slices.Contains(plan.TCPPorts, port) && !slices.Contains(current.SSHPorts, port) {
			return false, errors.New("firewall requirements changed after plan review")
		}
	}
	for _, port := range append(currentUDP, current.UDPPorts...) {
		if !slices.Contains(plan.UDPPorts, port) {
			return false, errors.New("firewall requirements changed after plan review")
		}
	}

	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	response, err := p.applyCanonicalFirewallV2Identity(ctx, "firewall_apply", commitment, step.RequestID, step.OwnerID)
	if err != nil {
		return false, err
	}
	if response.Error != "" || !response.Enabled {
		return false, errors.New("firewall application was not verified")
	}
	return true, nil
}

// A new review is permitted after a proven terminal failure only. If the agent
// cannot be observed, preserve the running draft: uncertainty is not authority
// to start a second mutation or to change the plan beneath an interrupted one.
func (p *Panel) releaseFailedServerSetupDraft(ctx context.Context, plan serverSetupPlan) {
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	active, err := p.activeServiceOperation(ctx)
	if err != nil || active != nil {
		return
	}
	job, err := p.statusAgentMutation(ctx, "")
	if err != nil || (job != nil && agentMutationActive(job.Status)) {
		return
	}
	_, err = p.db.GetDB().ExecContext(ctx, `UPDATE server_setup_state SET status='draft',updated_at=? WHERE id=1 AND revision=? AND status='running'`, time.Now().UTC().Format(time.RFC3339Nano), plan.Revision)
	if err != nil {
		log.Printf("release failed setup draft: %v", err)
	}
}

type serverSetupMailHostnameKey struct{}
type serverSetupMailChild struct {
	MailHostname string `json:"setup_mail_hostname"`
}

func decodeServerSetupMailChild(op serviceOperation) (string, error) {
	if op.OperationData == "" {
		return "", nil
	}
	var child serverSetupMailChild
	if err := json.Unmarshal([]byte(op.OperationData), &child); err != nil {
		return "", err
	}
	canonical, err := hostname.CanonicalFQDN(child.MailHostname)
	if err != nil || canonical != child.MailHostname {
		return "", errors.New("saved setup mail hostname is invalid")
	}
	return canonical, nil
}

func validateServerSetupExecution(plan serverSetupPlan, execution serverSetupExecution) error {
	if execution.PlanID != plan.ID || execution.ID != execution.RequestID || !validServiceOperationID(execution.ID) || len(execution.Steps) != len(plan.Steps) {
		return errors.New("saved setup execution does not match its reviewed plan")
	}
	for index, step := range execution.Steps {
		if step.serverSetupPlanStep != plan.Steps[index] || step.RequestID != serverSetupID(execution.ID, step.ID, "request") || step.OwnerID != serverSetupID(execution.ID, step.ID, "owner") {
			return errors.New("saved setup child does not match its reviewed identity")
		}
		if step.EnrollmentDispatchAttempted && (step.Kind != "mail_enrollment" || step.Status == "pending") {
			return errors.New("saved enrollment dispatch does not match its reviewed step")
		}
		if !slices.Contains([]string{"pending", "running", "succeeded", "failed"}, step.Status) {
			return errors.New("saved setup child status is invalid")
		}
	}
	return nil
}

// serverSetupChildFailure carries a child's code and message and, for an
// install failure, the component, install step and bounded host line that the
// wizard shows as the cause and next action (D-024). Reason is the typed
// cause of a HOST_MUTATION_BUSY refusal, when known.
type serverSetupChildFailure struct{ Code, Message, Component, Step, Detail, Reason string }

func (e *serverSetupChildFailure) Error() string { return e.Code + ": " + e.Message }

func serverSetupFailureForStep(step serverSetupExecutionStep, cause error) *serviceOperationError {
	var child *serverSetupChildFailure
	if errors.As(cause, &child) {
		return &serviceOperationError{Code: child.Code, Message: child.Message,
			Component: child.Component, Step: child.Step, Detail: child.Detail,
			Reason: child.Reason}
	}
	switch {
	case errors.Is(cause, errServerSetupHostRestartRequired):
		return &serviceOperationError{Code: transport.FirewallStatusHostRestartRequired, Message: setupHostRestartRequiredMessage}
	case errors.Is(cause, errServerSetupDNSRolledBack):
		message := "The DNS engine installation did not complete and CelikPanel undid it: no DNS engine is running on this server, and any packages it installed were kept stopped for the next attempt. Nothing else was changed, and no restart is needed. The server administrator chooses Review a revised plan: keep this DNS software to try the installation again as a new one, or choose the other DNS software in the plan; setup then continues from the new plan."
		if sentence := namedHostOperationSentence(cause); sentence != "" {
			message += " Reason: " + sentence
		}
		return &serviceOperationError{Code: "server_setup_dns_rolled_back", Message: message}
	case errors.Is(cause, errServerSetupInfrastructureDNSChanged):
		return &serviceOperationError{Code: "server_setup_infrastructure_dns_changed", Message: "The DNS zone or its ownership changed after review. Existing records were preserved. Review the infrastructure records again before continuing."}
	case errors.Is(cause, errServerSetupBuildChanged):
		return &serviceOperationError{Code: "server_setup_build_changed", Message: "The panel version changed after this setup plan was reviewed. Completed operations are preserved. Review and confirm a new plan before starting the remaining setup steps."}
	case errors.Is(cause, errFirewallNoSSHService):
		return &serviceOperationError{Code: "firewall_no_ssh_service", Message: "No SSH service is available. Configure a verified management access path before enabling the firewall."}
	case errors.Is(cause, errFirewallSSHNotListening):
		return &serviceOperationError{Code: "firewall_ssh_not_listening", Message: "SSH is installed but has no verified listening port. Restore SSH access before retrying."}
	case errors.Is(cause, errFirewallSSHUnprovable):
		return &serviceOperationError{Code: "firewall_ssh_unprovable", Message: "The SSH access port could not be verified. Review server access before retrying."}
	case errors.Is(cause, errFirewallNoEngine):
		return &serviceOperationError{Code: "firewall_no_engine", Message: "The firewall engine is unavailable. Review the component installation result."}
	}
	// The Agent refused the step's mutation because the host was busy: keep
	// that typed cause and its reason sentence rather than the step's generic
	// code (upd8 F1: 05-firewall showed only server_setup_firewall_failed).
	// Ajan ana makine meşgul olduğu için reddettiyse tipli neden korunur.
	if classification, ok := classifyHostMutationError(cause); ok {
		return &serviceOperationError{Code: classification.Code, Message: classification.Message,
			Reason: classification.Reason}
	}
	switch step.Kind {
	case "infrastructure_dns":
		return &serviceOperationError{Code: "server_setup_infrastructure_dns_failed", Message: "The infrastructure DNS records could not be published and verified. Review the DNS operation and native DNS service before reviewing a new setup plan."}
	case "access_dns":
		return &serviceOperationError{Code: "server_setup_access_dns_failed", Message: "The reviewed address check could not be completed. Review the panel or mail hostname and server address in this setup plan."}
	case "dns":
		return &serviceOperationError{Code: "server_setup_dns_failed", Message: "DNS setup requires attention. Verify the local address, nameserver identities, independent peer and the DNS operation result."}
	case "panel_certificate":
		return &serviceOperationError{Code: "server_setup_panel_certificate_failed", Message: "The panel certificate could not be verified. Check the panel domain DNS and HTTP validation access, then review the certificate operation."}
	case "firewall":
		return &serviceOperationError{Code: "server_setup_firewall_failed", Message: "Firewall requirements changed or application could not be verified. Review management access and the current service ports before retrying."}
	default:
		return &serviceOperationError{Code: "server_setup_step_failed", Message: "The setup step could not be completed. Review its component operation before retrying. Existing services are preserved."}
	}
}

var errServerSetupLicenseRequired = errors.New("setup requires an active license")

// errServerSetupHostRestartRequired: the Agent proved the running kernel
// cannot load netfilter until the server is restarted.
var errServerSetupHostRestartRequired = errors.New("the server must be restarted before its firewall can be checked")

const setupHostRestartRequiredMessage = "This server was updated and must be restarted before its firewall can be checked. " +
	"Restart the server, then open setup again; your draft continues and nothing was changed."

// setupFirewallStatusBlocker turns the Agent's firewall status error into a
// plan blocker. The restart case is fully described by its code. The other
// classified codes carry the Agent's reason, bounded and on one line, behind
// the code (the wizard shows the part before ':' as text and the whole code
// under Technical details). An older Agent without a code is unknown.
func setupFirewallStatusBlocker(status FirewallStatusResp) string {
	switch status.ErrorCode {
	case transport.FirewallStatusHostRestartRequired:
		return transport.FirewallStatusHostRestartRequired
	case transport.FirewallStatusKernelUnavailable, transport.FirewallStatusEngineUnavailable,
		transport.FirewallStatusBusy:
		return status.ErrorCode + ":" + boundedSetupHostReason(status.Error)
	default:
		return transport.FirewallStatusUnknown + ":" + boundedSetupHostReason(status.Error)
	}
}

// boundedSetupHostReason is the first line of a host-reported reason with
// control characters removed and bounded, for logs and plan blockers.
func boundedSetupHostReason(reason string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(reason), "\n")
	line = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, line)), " ")
	if runes := []rune(line); len(runes) > 180 {
		line = string(runes[:180]) + "..."
	}
	if line == "" {
		return "no reason reported"
	}
	return line
}

var errServerSetupBuildChanged = errors.New("setup build changed; review the remaining plan")

// requireServerSetupAdmission judges the license exactly as the HTTP access
// gate does (panelLicenseStatus: licensing.Manager.AccessStatus, which runs
// the same synchronous refresh under the same one-minute rule before reading
// the status). The background setup runner has no browser request to refresh
// the check for it; reading Status() alone let a stale but valid check look
// like a missing license (pair2 finding P-C). License policy is unchanged:
// the same function, verification, interval and failure semantics apply.
// An unreadable or unverifiable status is not a missing or invalid license
// (D-024, D-025): it is reported as unknown, not as license_required.
//
// Kurulum çalıştırıcısı lisansı HTTP erişim kapısıyla aynı biçimde (aynı
// eşzamanlı yenileme ve aynı bir dakika kuralıyla) değerlendirir. Okunamayan
// durum "lisans gerekli" değil, bilinmeyen durum olarak bildirilir.
func (p *Panel) requireServerSetupAdmission(ctx context.Context) error {
	status := p.panelLicenseStatus(ctx)
	if status.CanProvision {
		return nil
	}
	if status.Observation != licensing.ObservationKnown {
		return &serverSetupLicenseUnverifiedError{State: status.State}
	}
	return errServerSetupLicenseRequired
}

// errServerSetupLicenseUnverified: the license status could not be read or
// currently verified. It does not prove the license missing or invalid.
var errServerSetupLicenseUnverified = errors.New("setup could not verify the license status")

type serverSetupLicenseUnverifiedError struct{ State string }

func (e *serverSetupLicenseUnverifiedError) Error() string {
	return errServerSetupLicenseUnverified.Error() + ": " + e.State
}

func (e *serverSetupLicenseUnverifiedError) Unwrap() error { return errServerSetupLicenseUnverified }

// serverSetupLicenseUnverifiedFailure keeps the access gate's stable codes.
func serverSetupLicenseUnverifiedFailure(err error) *serviceOperationError {
	var unverified *serverSetupLicenseUnverifiedError
	if errors.As(err, &unverified) && unverified.State == "verification_unavailable" {
		return &serviceOperationError{Code: errCodeLicenseVerificationUnavailable, Message: "The CelikPanel license could not be verified just now. This does not mean the license is missing or invalid. Setup continues by itself as soon as verification succeeds; if this persists, the server administrator can open License settings and check again. Existing services keep running."}
	}
	return &serviceOperationError{Code: errCodeLicenseStatusUnavailable, Message: "The CelikPanel license status could not be read just now. This does not mean the license is missing or invalid. Setup continues by itself as soon as the status can be read; if this persists, the server administrator can open License settings and check again. Existing services keep running."}
}

// writeServerSetupAdmissionError answers a setup request that was not
// admitted: an unknown license status keeps the access gate's 503 codes, a
// known inactive license keeps the caller's license_required response.
func writeServerSetupAdmissionError(w http.ResponseWriter, err error, code, message, location string) {
	if errors.Is(err, errServerSetupLicenseUnverified) {
		failure := serverSetupLicenseUnverifiedFailure(err)
		w.Header().Set("Cache-Control", "no-store")
		writeCodedError(w, http.StatusServiceUnavailable, failure.Code, failure.Message, "")
		return
	}
	writeCodedError(w, http.StatusForbidden, code, message, location)
}

// A new request may have committed between the old runner's terminal read and
// its defer. Recheck after releasing the per-panel runner token and hand off
// only to a different durable request, never spin on the same persistent error.
func (p *Panel) finishServerSetupRunner(lastID string) {
	serverSetupRunners.Delete(p)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	next, err := p.latestServerSetupExecution(ctx)
	if err == nil && next != nil && next.ID != lastID && (next.Status == "running" || next.Status == "waiting") {
		p.launchServerSetupExecution()
	}
}

func (p *Panel) runServerSetupMailCertificate(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (bool, error) {
	commitment, err := mutationpayload.CanonicalMailHostCertificate(step.Target, plan.ContactEmail, plan.BuildCommit)
	if err != nil {
		return false, err
	}
	op := serviceOperation{RequestID: step.RequestID, Kind: "mail_host_certificate", ServiceID: step.Target, PackageName: commitment.Qualifier}
	identity := agentMutationIdentityForOperation(op, step.OwnerID)
	job, err := p.statusAgentMutation(ctx, step.RequestID)
	if err != nil {
		return false, err
	}
	if job != nil {
		if err := validateAgentMutationIdentity(job, identity); err != nil {
			return false, err
		}
		if job.Status == agentMutationSucceeded {
			return true, validateAgentMutationSucceededReceipt(job, identity)
		}
		if agentMutationActive(job.Status) {
			return false, nil
		}
		return false, errors.New("mail host certificate operation failed; review a new attempt")
	}
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	if plan.BuildCommit != strings.TrimSpace(buildCommit) {
		return false, errServerSetupBuildChanged
	}
	if !p.serviceMutationMu.TryLock() {
		return false, errServiceOperationBusy
	}
	defer p.serviceMutationMu.Unlock()
	if active, err := p.activeServiceOperation(ctx); err != nil {
		return false, err
	} else if active != nil {
		return false, errServiceOperationBusy
	}
	if err := p.requireMatchingAgentBuild(ctx); err != nil {
		return false, err
	}
	var agent transport.AgentVersionResponse
	if err := p.callAgentContext(ctx, "Agent.Version", &transport.Empty{}, &agent); err != nil {
		return false, err
	}
	if err := requireKnownAgentCapabilities(agent.Capabilities, transport.AgentCapabilityMailHostCertificateV1); err != nil {
		return false, err
	}
	if err := p.authorizeAgentRPCContext(ctx, "Agent.IssueMailHostCertificateV1"); err != nil {
		return false, err
	}
	p.auditServiceOperation(ctx, plan.Actor, "server_setup_mail_certificate_requested")
	err = p.withStandaloneAgentMutationIdentity(ctx, op, step.OwnerID, func(bound context.Context, binding agentMutationBinding) error {
		var response transport.IssueMailHostCertificateResponse
		req := transport.IssueMailHostCertificateRequest{ServiceMutationBinding: binding, Domain: commitment.Domain, Email: commitment.Email, ExpectedBuildCommit: commitment.ExpectedBuildCommit}
		if err := p.callAgentContext(bound, "Agent.IssueMailHostCertificateV1", &req, &response); err != nil {
			return err
		}
		if response.Error != "" || !response.Issued {
			return errors.New("mail host certificate publication was not verified")
		}
		return nil
	})
	return err == nil, err
}
