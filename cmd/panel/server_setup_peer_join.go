package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The first server's join step (alpha.114). Its plan was reviewed without
// the second server: non-DNS components first, then peer_join, which waits
// until the second server joined and sent its names. The step then binds
// the second server to this exact execution, once and immutably (the same
// INSERT OR IGNORE pattern as the DNS publisher binding): its address, names
// and, when this server keeps the zone, the zone records computed and
// re-checked against the zone at binding time. Every later step runs on the
// plan completed by the binding (serverSetupBoundPlan); the reviewed plan
// itself never changes.
// Ilk sunucunun katilma adimi: ikinci sunucu bu calismaya bir kez baglanir.

type serverSetupPeerBinding struct {
	ExecutionID       string                            `json:"execution_id"`
	PlanID            string                            `json:"plan_id"`
	JoinRequestID     string                            `json:"join_request_id"`
	PeerIPv4          string                            `json:"peer_ipv4"`
	PeerIPv6          string                            `json:"peer_ipv6,omitempty"`
	PeerShortName     string                            `json:"peer_short_name,omitempty"`
	PeerPanelDomain   string                            `json:"peer_panel_domain"`
	PeerMailHostname  string                            `json:"peer_mail_hostname,omitempty"`
	PeerMail          *serverSetupPeerMail              `json:"peer_mail,omitempty"`
	PeerAddress       string                            `json:"peer_address"`
	PeerPin           string                            `json:"peer_pin"`
	PeerDNSEngine     string                            `json:"peer_dns_engine"`
	PublisherEndpoint string                            `json:"publisher_endpoint,omitempty"`
	InfrastructureDNS *serverSetupInfrastructureDNSPlan `json:"infrastructure_dns,omitempty"`
	// JoinDigest is the digest of the join document the second server
	// received (shared answers, names, mail): an extension that would change
	// it needs the second server too. Additive; older bindings have none.
	JoinDigest string `json:"join_digest,omitempty"`
}

func serverSetupPeerBindingKey(executionID string) string {
	return "server_setup_peer_binding:" + executionID
}

func serverSetupPeerBindingFromJoin(join serverSetupJoinRecord, names serverSetupPeerNames) serverSetupPeerBinding {
	binding := serverSetupPeerBinding{JoinRequestID: join.RequestID, JoinDigest: join.Digest, PeerIPv4: join.Peer.IPv4, PeerIPv6: join.Peer.IPv6, PeerShortName: join.Peer.ShortName,
		PeerPanelDomain: names.PanelDomain, PeerMailHostname: names.MailHostname, PeerMail: names.Mail, PeerAddress: names.Address, PeerPin: names.Pin,
		PeerDNSEngine: names.DNSEngine, PublisherEndpoint: names.PublisherEndpoint}
	if names.ShortName != "" {
		binding.PeerShortName = names.ShortName
	}
	return binding
}

// serverSetupEffectiveDraft completes the reviewed draft with the bound
// second server: its address, and as main DNS server the zone's names for
// it and its mail choice; as backup DNS server the main's publishing
// endpoint. Only the binding's values are added.
func serverSetupEffectiveDraft(d serverSetupDraft, binding serverSetupPeerBinding) serverSetupDraft {
	d.PeerIP = binding.PeerIPv4
	if d.DNSRole == transport.DNSPairRoleSecondary {
		d.PeerNS, d.DNSPublisherEndpoint = d.NS1, binding.PublisherEndpoint
		return d
	}
	d.PeerNS = d.NS2
	if d.InfrastructureDNS != nil {
		selection := *d.InfrastructureDNS
		selection.PeerPanelDomain, selection.PeerMailHostname = "", ""
		if serverSetupDNSNameWithin(binding.PeerPanelDomain, selection.Zone) {
			selection.PeerPanelDomain = binding.PeerPanelDomain
		}
		if serverSetupDNSNameWithin(binding.PeerMailHostname, selection.Zone) {
			selection.PeerMailHostname = binding.PeerMailHostname
		}
		d.InfrastructureDNS = &selection
	}
	if binding.PeerMail != nil && !serverSetupHasMail(d) {
		mail := *binding.PeerMail
		d.PeerMail = &mail
	}
	return d
}

// serverSetupBoundPlan is the reviewed plan completed by its binding. It
// keeps the reviewed identity; peerBound marks it for the steps that check
// the identity against the plan's own content.
func serverSetupBoundPlan(plan serverSetupPlan, binding serverSetupPeerBinding) serverSetupPlan {
	plan.Draft = serverSetupEffectiveDraft(plan.Draft, binding)
	plan.InfrastructureDNS = binding.InfrastructureDNS
	plan.peerBound = true
	return plan
}

func (p *Panel) readServerSetupPeerBinding(ctx context.Context, plan serverSetupPlan, executionID string) (*serverSetupPeerBinding, error) {
	var binding serverSetupPeerBinding
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupPeerBindingKey(executionID), &binding)
	if err != nil || !found {
		return nil, err
	}
	if binding.ExecutionID != executionID || binding.PlanID != plan.ID {
		return nil, errors.New("saved second server binding does not match its reviewed execution")
	}
	return &binding, nil
}

// effectiveServerSetupPlan is the plan the steps run on: the reviewed plan,
// completed by its binding once the second server is bound.
func (p *Panel) effectiveServerSetupPlan(ctx context.Context, plan serverSetupPlan, executionID string) (serverSetupPlan, error) {
	if !serverSetupPlanHasStep(plan, "peer_join") {
		return plan, nil
	}
	binding, err := p.readServerSetupPeerBinding(ctx, plan, executionID)
	if err != nil || binding == nil {
		return plan, err
	}
	return serverSetupBoundPlan(plan, *binding), nil
}

// runServerSetupPeerJoin waits for the second server and binds it. Each
// wait names its reason and who acts (D-024); reading never changes the
// join, and the binding is written once.
func (p *Panel) runServerSetupPeerJoin(ctx context.Context, plan serverSetupPlan, executionID string) (bool, error) {
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	if binding, err := p.readServerSetupPeerBinding(ctx, plan, executionID); err != nil || binding != nil {
		return binding != nil, err
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		return false, err
	}
	if !found {
		return false, &serverSetupPeerWait{Code: "server_setup_peer_join_waiting", Message: "Waiting for the second server to join. Create a join code here, then enter this server's address and the code in the second server's setup."}
	}
	if join.Digest != serverSetupJoinDocumentDigest(serverSetupJoinDocumentFor(plan.Draft)) {
		return false, &serverSetupPeerWait{Code: "server_setup_peer_join_changed", Message: "This server's answers changed after the second server joined. Create a new join code here and join the second server again."}
	}
	if join.NamesChanged {
		// The stored names are the ones the second server replaced: binding
		// them would bind answers it no longer has (Beta-1 P4).
		return false, serverSetupPeerJoinChangedWait()
	}
	if join.Names == nil {
		return false, &serverSetupPeerWait{Code: "server_setup_peer_names_waiting", Message: "The second server joined; waiting for its own setup to start, which sends its names here."}
	}
	names, err := p.openServerSetupPeerNames(*join.Names)
	if err != nil {
		return false, err
	}
	if code, message := canonicalServerSetupPeerNames(&names, plan.Draft, join); code != "" {
		return false, &serverSetupPeerWait{Code: code, Message: message}
	}
	binding := serverSetupPeerBindingFromJoin(join, names)
	binding.ExecutionID, binding.PlanID = executionID, plan.ID
	effective := serverSetupEffectiveDraft(plan.Draft, binding)
	if effective.InfrastructureDNS != nil && effective.DNSRole == transport.DNSPairRolePrimary {
		// The zone records are computed now, against the zone as it is, and
		// refused before anything is written if they clash.
		infrastructure, err := p.buildServerSetupInfrastructureDNSPlan(ctx, effective)
		if err != nil {
			var planErr *serverSetupInfrastructureDNSPlanError
			if errors.As(err, &planErr) {
				return false, &serverSetupPeerWait{Code: "server_setup_peer_names_clash", Message: "The second server's names cannot be added to the main domain's zone (" + planErr.Code + "). Change them in the second server's plan, or the conflicting record in Domains."}
			}
			return false, err
		}
		binding.InfrastructureDNS = infrastructure
	}
	encoded, _ := json.Marshal(binding)
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO panel_settings(key,value)
		SELECT ?,? WHERE EXISTS (SELECT 1 FROM server_setup_executions WHERE id=? AND plan_id=? AND status IN ('running','waiting'))`,
		serverSetupPeerBindingKey(executionID), string(encoded), executionID, plan.ID); err != nil {
		return false, err
	}
	saved, err := p.readServerSetupPeerBinding(ctx, plan, executionID)
	if err != nil || saved == nil {
		return false, errors.Join(errors.New("second server binding was not saved"), err)
	}
	return true, nil
}

// The second server's answers changed after a run bound them (Beta-1 P4):
// who acts is the owner, the next action is a new join code here entered on
// the second server, and setup then continues by itself.
const serverSetupPeerJoinChangedMessage = "The second server's answers changed after it joined this setup. Create a new join code here and enter it on the second server; setup then continues by itself."

// serverSetupPeerJoinReopenMessage: the same change after this run already
// went on with the earlier answers: the run cannot take new ones.
const serverSetupPeerJoinReopenMessage = "The second server's answers changed after this setup run already used them. Reopen this server's plan and start it again; then create a new join code here and enter it on the second server."

func serverSetupPeerJoinChangedWait() *serverSetupPeerWait {
	return &serverSetupPeerWait{Code: "server_setup_peer_join_changed", Message: serverSetupPeerJoinChangedMessage}
}

// serverSetupPlanChangedByRejoin: the run's second server joined again with
// other answers after this run used its earlier binding, so the run stops
// with this verified state (D-024: who = owner; next = review the plan
// again; then setup continues from the revised plan, completed host changes
// kept). Beta-1 re-run P8.
func serverSetupPlanChangedByRejoin() error {
	return &serverSetupChildFailure{Code: "server_setup_plan_changed_by_rejoin", Message: "The second server joined this setup again after this run had used its earlier join, so this run stopped. Completed host changes are kept. You, the owner: choose Review a revised plan here; setup then continues from the revised plan with the new join."}
}

// serverSetupRejoinedUnderRun: the join this run is bound to is no longer
// the current join (a newer join replaced it), or its names changed.
func (p *Panel) serverSetupRejoinedUnderRun(ctx context.Context, reviewed serverSetupPlan, executionID string) (bool, error) {
	if !serverSetupPlanHasStep(reviewed, "peer_join") {
		return false, nil
	}
	binding, err := p.readServerSetupPeerBinding(ctx, reviewed, executionID)
	if err != nil || binding == nil {
		return false, err
	}
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil || !found {
		return false, err
	}
	return serverSetupBindingStale(join, *binding), nil
}

// serverSetupBindingStale: the join a run was bound to was replaced by a
// newer one, or the second server sent other names after the binding.
func serverSetupBindingStale(join serverSetupJoinRecord, binding serverSetupPeerBinding) bool {
	return join.RequestID != binding.JoinRequestID || join.NamesChanged
}

// serverSetupBindingUnused: nothing after the run's join step has used the
// binding yet. Every later step is still pending, or is the DNS step waiting
// for the main DNS server before its install began (no DNS child exists).
func (p *Panel) serverSetupBindingUnused(ctx context.Context, execution serverSetupExecution) (bool, error) {
	after := false
	for _, step := range execution.Steps {
		if step.Kind == "peer_join" {
			after = true
			continue
		}
		if !after || step.Status == "pending" {
			continue
		}
		if step.Kind == "dns" && step.Status == "running" {
			status, err := p.serverSetupDNSOperationStatus(ctx, step.RequestID)
			if err != nil {
				return false, err
			}
			if status == "missing" {
				continue
			}
		}
		return false, nil
	}
	return after, nil
}

// serverSetupSucceededPeerBinding is the binding of a setup run that
// succeeded, if any: after it, a second server's answers are final.
func (p *Panel) serverSetupSucceededPeerBinding(ctx context.Context) (*serverSetupPeerBinding, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT s.value FROM panel_settings s JOIN server_setup_executions e ON s.key='server_setup_peer_binding:'||e.id
		WHERE e.status='succeeded' ORDER BY e.updated_at DESC, e.id DESC LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var binding serverSetupPeerBinding
	if err := json.Unmarshal([]byte(raw), &binding); err != nil || !validServiceOperationID(binding.ExecutionID) {
		return nil, errors.New("saved second server binding is invalid")
	}
	return &binding, nil
}

// reconcileServerSetupStaleBinding handles a run whose binding went stale
// before the run succeeded: it waits for the owner's new join, or, once a
// newer join replaced the one it was bound to and nothing used the binding,
// it unbinds and runs its join step again with the new join. A run that
// already used the binding is told to be reopened. Reading changes nothing
// unless the newer join is there (D-024: polling never mutates otherwise).
func (p *Panel) reconcileServerSetupStaleBinding(ctx context.Context, reviewed serverSetupPlan, execution *serverSetupExecution) (*serverSetupPeerWait, bool, error) {
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	binding, err := p.readServerSetupPeerBinding(ctx, reviewed, execution.ID)
	if err != nil || binding == nil {
		return nil, false, err
	}
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil || !found || !serverSetupBindingStale(join, *binding) {
		return nil, false, err
	}
	unused, err := p.serverSetupBindingUnused(ctx, *execution)
	if err != nil {
		return nil, false, err
	}
	if !unused {
		return &serverSetupPeerWait{Code: "server_setup_peer_join_reopen_required", Message: serverSetupPeerJoinReopenMessage}, false, nil
	}
	if join.RequestID == binding.JoinRequestID {
		return serverSetupPeerJoinChangedWait(), false, nil
	}
	// A newer join is here: this run's unused binding is withdrawn and its
	// join step binds the new join (or waits for its names).
	for index := range execution.Steps {
		if execution.Steps[index].Kind == "peer_join" {
			execution.Steps[index].Status = "running"
		}
	}
	if err := p.persistServerSetupExecution(ctx, *execution); err != nil {
		return nil, false, err
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupPeerBindingKey(execution.ID)); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

// serverSetupPeerJoinSteps orders a joined plan: the components that need
// no DNS first, then the join step, then the DNS steps and their waits, then
// the rest. The mail profile and every service that needs a mail server
// (SpamAssassin) need this server's DNS identity (mailProfileDNSIdentityReady),
// which exists only after the DNS waits: they stay in the rest, after them
// (Beta-1 VM cells a and b failed with mail_profile_dns_identity_not_ready).
func serverSetupPeerJoinSteps(steps []serverSetupPlanStep, lead serverSetupPlanStep) []serverSetupPlanStep {
	early, rest, mail := []serverSetupPlanStep{}, []serverSetupPlanStep{}, []serverSetupPlanStep{}
	for _, step := range steps {
		switch {
		case step.Kind == "mail_profile" || (step.Kind == "service" && serverSetupMailDependent(step.Target)):
			mail = append(mail, step)
		case step.Kind == "runtime" || step.Kind == "service":
			early = append(early, step)
		default:
			rest = append(rest, step)
		}
	}
	// The mail steps go right after the last DNS step or wait, whatever the
	// order they came in, keeping their own order.
	at := 0
	for index, step := range rest {
		if slices.Contains([]string{"dns", "dns_readiness", "dns_publisher"}, step.Kind) {
			at = index + 1
		}
	}
	rest = slices.Concat(rest[:at:at], mail, rest[at:])
	ordered := []serverSetupPlanStep{}
	if lead.Kind == "peer_names" {
		ordered = append(append(append(ordered, lead), early...), rest...)
	} else {
		ordered = append(append(append(ordered, early...), lead), rest...)
	}
	for index := range ordered {
		ordered[index].ID = fmt.Sprintf("%02d-%s", index+1, ordered[index].Kind)
	}
	return ordered
}

// --- DKIM over the link ------------------------------------------------------

type serverSetupPeerDKIMRequest struct {
	Protocol  int    `json:"protocol"`
	RequestID string `json:"request_id"`
	Domain    string `json:"domain"`
	Selector  string `json:"selector"`
	Value     string `json:"value"`
}

// handleServerSetupPeerDKIM: the server that receives the main domain's mail
// sends its DKIM record to the main DNS server, which writes it into the
// setup's zone and publishes the zone. Writing the same value again is the
// same change, so a retry after a lost answer changes nothing twice.
func (p *Panel) handleServerSetupPeerDKIM(w http.ResponseWriter, r *http.Request, role string) {
	var request serverSetupPeerDKIMRequest
	if !decodeServerSetupLinkBody(w, r, &request) || refuseServerSetupLinkProtocol(w, request.Protocol) {
		return
	}
	ctx := r.Context()
	value := strings.TrimSpace(request.Value)
	if !validServiceOperationID(request.RequestID) || request.Selector != dkimSelector || len(value) > 2048 || !strings.HasPrefix(value, "v=DKIM1;") || !strings.Contains(value, "p=") || strings.ContainsAny(value, "\"\\\x00\r\n") {
		writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_peer_dkim_invalid", "The DKIM record is not valid.", nil)
		return
	}
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	// The gate is the live link credential (checked before this handler) and
	// this server's own setup running or waiting: the link is a setup
	// channel, not a standing record interface. Either server can be the
	// main DNS server, so being bound is not the gate: a second server that
	// is the main (Option A) has no binding step at all.
	// On the first server the record comes from the second server's joined
	// address, exactly as its ready report does (review fix, item 5).
	if role == serverSetupLinkRoleFirst {
		record, err := p.readServerSetupPeerRecord(ctx, role)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if source, known := p.serverSetupJoinSourceIP(r); !record.Found || !known || source.String() != record.PeerSource {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_peer_dkim_source", "This DKIM record does not come from the second server's joined address; it is not written.", nil)
			return
		}
	}
	// A main whose run finished keeps its link open until the backup reports
	// ready (P7): while that link is live, the backup's DKIM for the bound
	// join is still taken (Beta-1 third re-run N3).
	linkLive := false
	if execution != nil && execution.Status == "succeeded" {
		record, err := p.readServerSetupPeerRecord(ctx, role)
		if err != nil {
			writeServerError(w, err)
			return
		}
		linkLive = record.Found && record.Live && record.ReadyAt == "" && record.RetiredAt == ""
	}
	if execution == nil || (!stringIn(execution.Status, "running", "waiting") && !linkLive) {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_not_running", "The main DNS server's setup is not running; the DKIM record is added there in Domains.", nil)
		return
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	switch {
	case serverSetupPlanHasStep(plan, "peer_join"):
		// The first server: its plan names the other server once bound.
		plan, err = p.effectiveServerSetupPlan(ctx, plan, execution.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if !plan.peerBound {
			writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_not_bound", "The main DNS server's setup has not taken this server in yet; it does once it has this server's names. Setup sends the record again by itself.", nil)
			return
		}
	case serverSetupPlanHasStep(plan, "peer_names"):
		// The second server: the join gave its plan the other server.
	default:
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_not_running", "The main DNS server's setup does not run with this server; the DKIM record is added there in Domains.", nil)
		return
	}
	d := plan.Draft
	if d.DNSMode != setupDNSModeLocal || d.DNSRole != transport.DNSPairRolePrimary || d.InfrastructureDNS == nil || d.InfrastructureDNS.Zone != d.MainDomain || request.Domain != d.MainDomain {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_not_main", "This server does not keep the main domain's zone.", nil)
		return
	}
	if serverSetupHasMail(d) || d.PeerMail == nil {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_main_signs", "The main domain's mail is received by the main DNS server itself; the other server's DKIM record is not written.", nil)
		return
	}
	var zoneID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name=?`, d.InfrastructureDNS.Zone).Scan(&zoneID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_zone_waiting", "The main DNS server's setup has not prepared the zone yet.", nil)
			return
		}
		writeServerError(w, err)
		return
	}
	changed, err := p.writeServerSetupPeerDKIM(ctx, d.InfrastructureDNS.Zone, int64(zoneID), dkimSelector+"._domainkey."+d.MainDomain, value)
	if errors.Is(err, errServerSetupPeerDKIMConflict) {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_conflict", "The zone already has a different DKIM record at this name that setup did not write. Change or remove it in Domains on the main DNS server, then review the other server's plan again.", nil)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	published := false
	if changed {
		p.audit(r, "server.setup.peer_dkim", "", 0)
		published = p.syncZoneToDNS(ctx, d.InfrastructureDNS.Zone, false) == nil
	} else if state, err := readDNSZoneSyncState(ctx, p.db.GetDB(), d.InfrastructureDNS.Zone); err == nil {
		// The same record again: nothing changes and nothing is published
		// again; the answer says whether the zone already carries it.
		published = state.Status == "applied" && state.AppliedGeneration == state.DesiredGeneration
	}
	if !published {
		// Saved; its publication is checked by the zone's own operation.
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"protocol": serverSetupLinkProtocol, "state": "saved"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"protocol": serverSetupLinkProtocol, "state": "published"})
}

var errServerSetupPeerDKIMConflict = errors.New("a different DKIM record setup did not write exists")

// writeServerSetupPeerDKIM writes the other server's DKIM record into the
// setup's zone as a record setup wrote (the written list), replaces only a
// record setup wrote earlier, refuses one the owner wrote, and reports
// whether anything changed.
func (p *Panel) writeServerSetupPeerDKIM(ctx context.Context, zone string, zoneID int64, name, value string) (bool, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	content := splitTXTContent(value)
	rows, err := tx.QueryContext(ctx, `SELECT id,content FROM pdns_records WHERE domain_id=? AND name=? AND type='TXT'`, zoneID, name)
	if err != nil {
		return false, err
	}
	type existing struct {
		id      int64
		content string
	}
	found := []existing{}
	for rows.Next() {
		var row existing
		if err := rows.Scan(&row.id, &row.content); err != nil {
			rows.Close()
			return false, err
		}
		found = append(found, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	var written serverSetupInfrastructureDNSWritten
	if _, err := readServerSetupJSONSetting(ctx, tx, serverSetupInfrastructureDNSWrittenKey(zone), &written); err != nil {
		return false, err
	}
	if written.ZoneID != zoneID {
		written = serverSetupInfrastructureDNSWritten{ZoneID: zoneID}
	}
	setupWrote := func(stored string) bool {
		for _, record := range written.Records {
			if record.Name == name && record.Type == "TXT" && joinTXTSegments(record.Content) == joinTXTSegments(stored) {
				return true
			}
		}
		return false
	}
	switch {
	case len(found) == 1 && joinTXTSegments(found[0].content) == value:
		return false, nil
	case len(found) > 1 || (len(found) == 1 && !setupWrote(found[0].content)):
		return false, errServerSetupPeerDKIMConflict
	case len(found) == 1:
		if _, err := tx.ExecContext(ctx, `UPDATE pdns_records SET content=?,disabled=0 WHERE id=?`, content, found[0].id); err != nil {
			return false, err
		}
		written.Records = slices.DeleteFunc(written.Records, func(record serverSetupInfrastructureDNSWrittenRecord) bool {
			return record.Name == name && record.Type == "TXT"
		})
	default:
		if _, err := tx.ExecContext(ctx, `INSERT INTO pdns_records(domain_id,name,type,content,ttl) VALUES(?,?,'TXT',?,3600)`, zoneID, name, content); err != nil {
			return false, err
		}
	}
	written.Records = append(written.Records, serverSetupInfrastructureDNSWrittenRecord{Name: name, Type: "TXT", Content: content})
	if err := writeServerSetupJSONSetting(ctx, tx, serverSetupInfrastructureDNSWrittenKey(zone), written); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// serverSetupPeerDKIMPresent: the other server's DKIM record is in the zone.
// The main DNS server's completion waits for it when the other server
// receives the main domain's mail, so its link stays open until then.
func (p *Panel) serverSetupPeerDKIMPresent(ctx context.Context, d serverSetupDraft) (bool, error) {
	var count int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM pdns_records r JOIN pdns_domains z ON z.id=r.domain_id WHERE z.name=? AND r.name=? AND r.type='TXT' AND COALESCE(r.disabled,0)=0`,
		d.InfrastructureDNS.Zone, dkimSelector+"._domainkey."+d.MainDomain).Scan(&count)
	return count > 0, err
}

// revokeServerSetupLink ends this server's side of the setup link once its
// setup completed: the link credential and the join code are removed, so
// neither a join (nor its replay) nor a link call is accepted afterwards.
// The join's other records stay as evidence.
// Kurulum tamamlaninca baglanti kimligi ve katilma kodu kaldirilir.
func (p *Panel) revokeServerSetupLink(ctx context.Context) error {
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupJoinCodeKey); err != nil {
		return err
	}
	if err := p.revokeServerSetupLinkSide(ctx, serverSetupLinkRoleFirst); err != nil {
		return err
	}
	return p.revokeServerSetupLinkSide(ctx, serverSetupLinkRoleSecond)
}

// revokeServerSetupLinkSide ends one role's side of the link. It keeps an
// authenticated tombstone: a keyed hash of the credential that side accepted
// (never the credential), so a later call with exactly that credential is
// answered "the link ended" (server_setup_peer_link_ended) rather than with
// a bare refusal (review fix, item 1, 2026-10-29). Idempotent.
func (p *Panel) revokeServerSetupLinkSide(ctx context.Context, role string) error {
	switch role {
	case serverSetupLinkRoleFirst:
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil || !found || (join.LinkCredentialHash == "" && join.PeerCredential == "") {
			return err
		}
		if join.LinkCredentialHash != "" {
			join.EndedCredentialHash = join.LinkCredentialHash
		}
		join.LinkCredentialHash, join.PeerCredential = "", ""
		return writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join)
	case serverSetupLinkRoleSecond:
		link, err := p.readServerSetupLink(ctx)
		if err != nil || link == nil || link.LinkCredential == "" {
			return err
		}
		link.EndedCredentialHash = p.serverSetupMAC("link", serverSetupReverseCredential(link.LinkCredential))
		link.LinkCredential = ""
		return p.writeServerSetupLink(ctx, p.db.GetDB(), *link)
	}
	return errors.New("unknown setup link role")
}

// serverSetupPeerTarget is how this server reaches the other one: on the
// second server its link, on the first the address and key fingerprint the
// second sent with its names.
func (p *Panel) serverSetupPeerTarget(ctx context.Context, plan serverSetupPlan, executionID string) (serverSetupLinkTarget, string, error) {
	if link, err := p.readServerSetupLink(ctx); err != nil || link != nil {
		if link == nil {
			return serverSetupLinkTarget{}, "", err
		}
		if link.LinkCredential == "" {
			return serverSetupLinkTarget{}, "", errServerSetupPeerLinkRevoked
		}
		return link.target(), link.bearer(), nil
	}
	if executionID == "" {
		execution, err := p.latestServerSetupExecution(ctx)
		if err != nil || execution == nil {
			return serverSetupLinkTarget{}, "", errors.Join(errors.New("no setup link to the other server"), err)
		}
		executionID = execution.ID
	}
	binding, err := p.readServerSetupPeerBinding(ctx, plan, executionID)
	if err != nil || binding == nil {
		return serverSetupLinkTarget{}, "", errors.Join(errors.New("no setup link to the other server"), err)
	}
	var join serverSetupJoinRecord
	if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil || !found {
		return serverSetupLinkTarget{}, "", errors.Join(errors.New("no setup link to the other server"), err)
	}
	if join.PeerCredential == "" && join.LinkCredentialHash == "" {
		return serverSetupLinkTarget{}, "", errServerSetupPeerLinkRevoked
	}
	pin, _ := hex.DecodeString(binding.PeerPin)
	credential, err := p.openServerSetupSecret(join.PeerCredential)
	if err != nil || credential == "" {
		return serverSetupLinkTarget{}, "", errors.Join(errors.New("no setup link credential for the other server"), err)
	}
	return serverSetupLinkTarget{Address: binding.PeerAddress, Pin: pin}, join.LinkID + "." + credential, nil
}

// serverSetupIsSecond: this server joined another (it keeps a link record).
func (p *Panel) serverSetupIsSecond(ctx context.Context) bool {
	link, err := p.readServerSetupLink(ctx)
	return err == nil && link != nil
}

// sendServerSetupPeerDKIM: the backup DNS server that receives the main
// domain's mail sends its DKIM record to the main DNS server.
func (p *Panel) sendServerSetupPeerDKIM(ctx context.Context, plan serverSetupPlan, executionID string, step serverSetupExecutionStep, value string) error {
	target, bearer, err := p.serverSetupPeerTarget(ctx, plan, executionID)
	if errors.Is(err, errServerSetupPeerLinkRevoked) {
		return serverSetupPeerLinkRevoked(p.serverSetupIsSecond(ctx))
	}
	if err != nil {
		return &serverSetupChildFailure{Code: "server_setup_peer_link_missing", Message: "This server has no setup link to the main DNS server, so its DKIM record cannot be sent. Add it on the main DNS server: Domains → " + plan.Draft.MainDomain + "."}
	}
	request := serverSetupPeerDKIMRequest{Protocol: serverSetupLinkProtocol, RequestID: serverSetupID(step.RequestID, "peer-dkim", "request"), Domain: plan.Draft.MainDomain, Selector: dkimSelector, Value: value}
	var answer struct {
		State string `json:"state"`
	}
	callErr := p.serverSetupLinkJSON(ctx, target, serverSetupPeerDKIMPath, bearer, request, &answer)
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil && answer.State == "published":
		return nil
	case callErr == nil:
		return &serverSetupPeerWait{Code: "server_setup_peer_dkim_publishing", Message: "The main DNS server saved this server's DKIM record and is publishing it. Setup checks again by itself."}
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_main_signs":
		// Both servers have mail: the main DNS server receives and signs the
		// main domain's mail itself; this server's mail serves its own
		// domains, so there is nothing to publish for it.
		return nil
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_version_incompatible":
		return serverSetupPeerVersionWait()
	// Not yet: the main's setup has not started, has not taken this server
	// in, or has no zone yet. A wait (D-024); the same request is sent again.
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_not_running":
		return &serverSetupPeerWait{Code: refusal.Code, Message: "The main DNS server's setup is not running. Nothing to do here if it is about to start; otherwise start or resume setup on the main DNS server. This server sends its DKIM record again by itself."}
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_not_bound":
		return &serverSetupPeerWait{Code: refusal.Code, Message: "The main DNS server's setup has not taken this server in yet; it does once it has this server's names (check its join step if it waits there). Setup sends the DKIM record again by itself."}
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_zone_waiting":
		return &serverSetupPeerWait{Code: refusal.Code, Message: "The main DNS server has not prepared the main domain's zone yet. Setup sends the DKIM record again by itself."}
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_link_unauthorized":
		return serverSetupPeerLinkRevoked(p.serverSetupIsSecond(ctx))
	// A definite refusal: the owner's own record is in the way.
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_conflict":
		return &serverSetupChildFailure{Code: refusal.Code, Message: "The main DNS server already has a different DKIM record at this name that setup did not write. Change or remove it there in Domains → " + plan.Draft.MainDomain + ", then review this plan again."}
	case errors.As(callErr, &refusal):
		return &serverSetupChildFailure{Code: refusal.Code, Message: "The main DNS server refused this server's DKIM record (" + refusal.Code + ")."}
	case errors.Is(callErr, errServerSetupLinkUnreachable), errors.Is(callErr, errServerSetupLinkPinMismatch):
		return &serverSetupPeerWait{Code: "server_setup_peer_dkim_unreachable", Message: "The main DNS server cannot be reached to send this server's DKIM record. Setup asks again by itself."}
	default:
		return &serverSetupPeerWait{Code: "server_setup_peer_dkim_unknown", Message: "The main DNS server's answer was not received. Setup sends the same record again; nothing is done twice."}
	}
}

// serverSetupJoinStandInPeer is a documentation address standing in for the
// second server while the plan is reviewed before it joined.
func serverSetupJoinStandInPeer(local string) string {
	if local == "192.0.2.254" {
		return "192.0.2.253"
	}
	return "192.0.2.254"
}

// autoServerSetupDNSPublisher makes the publishing connection of a joined
// backup DNS server with the enrollment code the join handed over, and binds
// it to this execution once (the administrator's confirmation is not needed:
// the join was that consent, D-021 Q5). Until the main DNS server accepts
// (its pair ready, its nameservers public) this is a wait.
func (p *Panel) autoServerSetupDNSPublisher(ctx context.Context, plan serverSetupPlan, executionID string) (*serverSetupDNSPublisherBinding, error) {
	endpoint, code := plan.Draft.DNSPublisherEndpoint, ""
	if link, err := p.readServerSetupLink(ctx); err != nil {
		return nil, err
	} else if link != nil && link.Document.PublisherEndpoint == endpoint {
		code = link.EnrollmentCode
	} else if link == nil {
		var join serverSetupJoinRecord
		if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil {
			return nil, err
		} else if found && join.Names != nil && join.Names.PublisherEndpoint == endpoint {
			opened, err := p.openServerSetupSecret(join.Names.EnrollmentCode)
			if err != nil {
				return nil, err
			}
			code = opened
		}
	}
	if code == "" || !remoteDNSValidSecret(code) || !serverSetupSecondaryHosting(plan.Draft) {
		return nil, nil
	}
	waiting := &serverSetupPeerWait{Code: "server_setup_dns_publisher_waiting", Message: "Waiting for the main DNS server to accept this server's publishing connection. It does once its DNS pair is ready and its nameservers are public. Setup asks again by itself."}
	label := "CelikPanel setup"
	if plan.Draft.ShortName != "" {
		label += ": " + plan.Draft.ShortName
	}
	id, err := p.prepareRemoteDNSConnection(ctx, endpoint, code, label)
	if err != nil {
		return nil, err
	}
	connection, err := p.resumeRemoteDNSConnection(ctx, id)
	if err != nil {
		// A definite refusal by the main DNS server (the code expired, was
		// withdrawn or used by another identity) is a verified failure with
		// its next action; only a transport failure, a main not ready yet or
		// an unreadable answer is a wait.
		var refused *remoteDNSHTTPError
		if errors.As(err, &refused) && refused.StatusCode >= 400 && refused.StatusCode < 500 && refused.Code != "REMOTE_DNS_AUTHORITY_NOT_READY" && refused.StatusCode != http.StatusTooManyRequests && refused.StatusCode != http.StatusRequestTimeout {
			return nil, &serverSetupChildFailure{Code: "server_setup_dns_publisher_refused", Message: "The main DNS server refused this server's publishing connection: its enrollment code expired, was withdrawn or was already used. On the main DNS server create a new join code, join this server again, then review this plan again."}
		}
		return nil, waiting
	}
	binding, err := p.serverSetupDNSPublisherProof(ctx, plan.Draft, connection.ID)
	if err != nil {
		return nil, waiting
	}
	if ready, err := p.serverSetupLocalSecondaryReadiness(ctx, plan.Draft); err != nil || !ready {
		return nil, &serverSetupPeerWait{Code: "server_setup_dns_publisher_local_waiting", Message: "This backup DNS server waits until it serves the main DNS server's list and both nameserver names are public. Setup asks again by itself."}
	}
	binding.ExecutionID, binding.PlanID = executionID, plan.ID
	encoded, _ := json.Marshal(binding)
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO panel_settings(key,value)
		SELECT ?,? WHERE EXISTS (SELECT 1 FROM server_setup_executions WHERE id=? AND plan_id=? AND status IN ('running','waiting'))`,
		serverSetupDNSPublisherKey(executionID), string(encoded), executionID, plan.ID); err != nil {
		return nil, err
	}
	return p.readServerSetupDNSPublisher(ctx, plan, executionID)
}
