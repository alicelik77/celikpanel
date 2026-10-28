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
}

func serverSetupPeerBindingKey(executionID string) string {
	return "server_setup_peer_binding:" + executionID
}

func serverSetupPeerBindingFromJoin(join serverSetupJoinRecord, names serverSetupPeerNames) serverSetupPeerBinding {
	binding := serverSetupPeerBinding{JoinRequestID: join.RequestID, PeerIPv4: join.Peer.IPv4, PeerIPv6: join.Peer.IPv6, PeerShortName: join.Peer.ShortName,
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
	if join.Names == nil {
		return false, &serverSetupPeerWait{Code: "server_setup_peer_names_waiting", Message: "The second server joined; waiting for its own setup to start, which sends its names here."}
	}
	if code, message := canonicalServerSetupPeerNames(join.Names, plan.Draft, join); code != "" {
		return false, &serverSetupPeerWait{Code: code, Message: message}
	}
	binding := serverSetupPeerBindingFromJoin(join, *join.Names)
	binding.ExecutionID, binding.PlanID = executionID, plan.ID
	effective := serverSetupEffectiveDraft(plan.Draft, binding)
	if effective.InfrastructureDNS != nil && effective.DNSRole == transport.DNSPairRolePrimary {
		// The zone records are computed now, against the zone as it is, and
		// refused before anything is written if they clash.
		infrastructure, err := p.buildServerSetupInfrastructureDNSPlan(ctx, effective)
		if err != nil {
			var planErr *serverSetupInfrastructureDNSPlanError
			if errors.As(err, &planErr) {
				return false, &serverSetupPeerWait{Code: "server_setup_peer_names_clash", Message: "The second server's names cannot be added to the zone: " + planErr.Message + ". Change them in the second server's plan, or the conflicting record in Domains."}
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

// serverSetupPeerJoinSteps orders a first server's new-flow plan: the
// components that need no DNS first, then the join step, then the DNS steps
// and their waits, then the rest.
func serverSetupPeerJoinSteps(steps []serverSetupPlanStep, lead serverSetupPlanStep) []serverSetupPlanStep {
	early, rest := []serverSetupPlanStep{}, []serverSetupPlanStep{}
	for _, step := range steps {
		if slices.Contains([]string{"service", "runtime", "mail_profile"}, step.Kind) {
			early = append(early, step)
		} else {
			rest = append(rest, step)
		}
	}
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
	RequestID string `json:"request_id"`
	Domain    string `json:"domain"`
	Selector  string `json:"selector"`
	Value     string `json:"value"`
}

// handleServerSetupPeerDKIM: the server that receives the main domain's mail
// sends its DKIM record to the main DNS server, which writes it into the
// setup's zone and publishes the zone. Writing the same value again is the
// same change, so a retry after a lost answer changes nothing twice.
func (p *Panel) handleServerSetupPeerDKIM(w http.ResponseWriter, r *http.Request) {
	var request serverSetupPeerDKIMRequest
	if !decodeRemoteDNSBody(w, r, &request) {
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
	if execution == nil {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_dkim_zone_waiting", "The main DNS server's setup has not prepared the zone yet.", nil)
		return
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	plan, err = p.effectiveServerSetupPlan(ctx, plan, execution.ID)
	if err != nil {
		writeServerError(w, err)
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
	if err := p.upsertTXT(ctx, zoneID, dkimSelector+"._domainkey."+d.MainDomain, value); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := p.syncZoneToDNS(ctx, d.InfrastructureDNS.Zone, false); err != nil {
		// Saved; its publication is checked by the zone's own operation.
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"state": "saved"})
		return
	}
	p.audit(r, "server.setup.peer_dkim", "", 0)
	_ = json.NewEncoder(w).Encode(map[string]string{"state": "published"})
}

// serverSetupPeerTarget is how this server reaches the other one: on the
// second server its link, on the first the address and key fingerprint the
// second sent with its names.
func (p *Panel) serverSetupPeerTarget(ctx context.Context, plan serverSetupPlan, executionID string) (serverSetupLinkTarget, string, error) {
	if link, err := p.readServerSetupLink(ctx); err != nil || link != nil {
		if link == nil {
			return serverSetupLinkTarget{}, "", err
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
	pin, _ := hex.DecodeString(binding.PeerPin)
	return serverSetupLinkTarget{Address: binding.PeerAddress, Pin: pin}, join.LinkID + "." + join.LinkCredential, nil
}

// sendServerSetupPeerDKIM: the backup DNS server that receives the main
// domain's mail sends its DKIM record to the main DNS server.
func (p *Panel) sendServerSetupPeerDKIM(ctx context.Context, plan serverSetupPlan, executionID string, step serverSetupExecutionStep, value string) error {
	target, bearer, err := p.serverSetupPeerTarget(ctx, plan, executionID)
	if err != nil {
		return &serverSetupChildFailure{Code: "server_setup_peer_link_missing", Message: "This server has no setup link to the main DNS server, so its DKIM record cannot be sent. Add it on the main DNS server: Domains → " + plan.Draft.MainDomain + "."}
	}
	request := serverSetupPeerDKIMRequest{RequestID: serverSetupID(step.RequestID, "peer-dkim", "request"), Domain: plan.Draft.MainDomain, Selector: dkimSelector, Value: value}
	var answer struct {
		State string `json:"state"`
	}
	callErr := serverSetupLinkJSON(ctx, target, serverSetupPeerDKIMPath, bearer, request, &answer)
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
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_dkim_zone_waiting":
		return &serverSetupPeerWait{Code: refusal.Code, Message: "The main DNS server has not prepared the main domain's zone yet. Setup sends the DKIM record again by itself."}
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
			code = join.Names.EnrollmentCode
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
