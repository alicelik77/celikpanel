package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.113 final acceptance, D2: a site added through Add Domain is a static
// site with no PHP-FPM pool and no socket recorded. Switching it to PHP must
// write the site's pool (as site creation does), record the socket, apply the
// vhost with it, and undo all of it when the vhost is refused.

type phpHostingTestAgent struct {
	*hostingTestAgent

	poolMu       sync.Mutex
	versions     []string
	poolExists   bool
	poolState    string // state reported for an existing pool
	poolOutcome  string // forced outcome (held cases)
	poolRequests []transport.SitePoolRequest
	deleted      []core.DeletePHPPoolRequest
	vhostSockets []string
}

func (a *phpHostingTestAgent) ListServiceInstances(
	request *transport.ServiceInstancesRequest,
	reply *transport.ServiceInstancesResponse,
) error {
	if request.ID != "php-fpm" {
		return fmt.Errorf("unexpected service instance ID %q", request.ID)
	}
	for _, version := range a.versions {
		reply.Instances = append(reply.Instances, core.ServiceInstance{Version: version, Managed: true})
	}
	return nil
}

func (a *phpHostingTestAgent) ApplySitePool(
	request *transport.SitePoolRequest,
	reply *transport.ApplySiteFileResponse,
) error {
	a.record("apply-pool", request.Operation)
	a.poolMu.Lock()
	defer a.poolMu.Unlock()
	a.poolRequests = append(a.poolRequests, *request)
	file := transport.SiteFileResult{
		Kind: transport.SiteFileKindPHPPool,
		Path: "/etc/php/" + request.PHPVersion + "/fpm/pool.d/site" + fmtIntForHostingTest(request.SiteID) + ".conf",
	}
	switch {
	case a.poolOutcome != "":
		file.State = a.poolState
		file.Outcome = a.poolOutcome
	case a.poolExists:
		file.State = a.poolState
		file.Outcome = transport.SiteFileOutcomeUnchanged
	default:
		file.State = transport.SiteFileAbsent
		file.Outcome = transport.SiteFileOutcomeWritten
		a.poolExists = true
		a.poolState = transport.SiteFileManagedUnchanged
	}
	reply.File = &file
	return nil
}

func (a *phpHostingTestAgent) DeletePHPPool(
	request core.DeletePHPPoolRequest,
	_ *transport.Empty,
) error {
	a.record("delete-pool", request.Version)
	a.poolMu.Lock()
	defer a.poolMu.Unlock()
	a.deleted = append(a.deleted, request)
	a.poolExists = false
	return nil
}

func attachPHPHostingTestAgent(t *testing.T, panel *Panel, agent *phpHostingTestAgent) {
	t.Helper()
	panel.pkgFamilyVal = "apt"
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func newPHPHostingTestAgent(versions ...string) *phpHostingTestAgent {
	agent := &phpHostingTestAgent{hostingTestAgent: &hostingTestAgent{}, versions: versions}
	agent.applyVhost = func(req *transport.ApplyVhostRequest, resp *transport.ApplyVhostResponse) error {
		if req.ProjectType == "php" {
			agent.poolMu.Lock()
			agent.vhostSockets = append(agent.vhostSockets, req.PHPSocket)
			agent.poolMu.Unlock()
		}
		resp.Config = "ok"
		return nil
	}
	return agent
}

// staticSiteAsAddDomainCreatesIt is the row Add Domain leaves: a static site,
// an empty PHP version and an empty socket (site_orchestrator.go).
func staticSiteAsAddDomainCreatesIt(t *testing.T, panel *Panel, domainID int) {
	t.Helper()
	resetHostingTestState(t, panel, domainID, "static")
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET php_version = '', php_fpm_socket = '' WHERE domain_id = ?`, domainID); err != nil {
		t.Fatalf("static site row: %v", err)
	}
}

func TestHostingTransitionStaticToPHPWritesPoolAndRecordsSocket(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent("8.2", "8.4")
	attachPHPHostingTestAgent(t, panel, agent)

	if _, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"}); err != nil {
		t.Fatalf("static -> php: %v", err)
	}
	state := hostingTestState(t, panel, domainID)
	wantSocket := services.PHPFPMSocketPath("8.4", fmt.Sprintf("site%d", state.SiteID))
	if state.ProjectType != "php" || state.PHPVersion.String != "8.4" || state.PHPSocket.String != wantSocket {
		t.Fatalf("state after static -> php = %#v, want php 8.4 with socket %s", state, wantSocket)
	}
	if len(agent.poolRequests) != 1 || agent.poolRequests[0].Operation != transport.SitePoolOperationCreate ||
		agent.poolRequests[0].PHPVersion != "8.4" || agent.poolRequests[0].Username != services.SiteUsername(state.DomainName) {
		t.Fatalf("pool requests = %#v, want one create for PHP 8.4 as the site user", agent.poolRequests)
	}
	if len(agent.vhostSockets) != 1 || agent.vhostSockets[0] != wantSocket {
		t.Fatalf("vhost sockets = %#v, want %s", agent.vhostSockets, wantSocket)
	}
	assertHostingCalls(t, agent.hostingTestAgent, []hostingAgentCall{
		{Method: "apply-pool", ProjectType: transport.SitePoolOperationCreate},
		{Method: "apply-vhost", ProjectType: "php"},
	})

	// Back to static: the pool and its record stay (the site's deletion
	// removes the pool; a return to PHP finds it), nginx stops sending PHP.
	if _, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "static"}); err != nil {
		t.Fatalf("php -> static: %v", err)
	}
	state = hostingTestState(t, panel, domainID)
	if state.ProjectType != "static" || state.PHPVersion.String != "8.4" || state.PHPSocket.String != wantSocket {
		t.Fatalf("state after php -> static = %#v, want static keeping the pool record", state)
	}
	// And to PHP again: the recorded version is kept although 8.4 is still
	// the newest; the existing pool is not created again nor removed.
	agent.versions = []string{"8.4", "8.5"}
	if _, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"}); err != nil {
		t.Fatalf("static -> php again: %v", err)
	}
	state = hostingTestState(t, panel, domainID)
	if state.ProjectType != "php" || state.PHPVersion.String != "8.4" || state.PHPSocket.String != wantSocket {
		t.Fatalf("state after the return to php = %#v", state)
	}
	if len(agent.deleted) != 0 {
		t.Fatalf("pools deleted = %#v, want none", agent.deleted)
	}
}

func TestHostingTransitionStaticToPHPUndoesPoolWhenVhostIsRefused(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent("8.4")
	vhostCalls := 0
	agent.applyVhost = func(_ *transport.ApplyVhostRequest, resp *transport.ApplyVhostResponse) error {
		vhostCalls++
		if vhostCalls == 1 {
			resp.Error = "injected nginx validation failure"
		}
		return nil
	}
	attachPHPHostingTestAgent(t, panel, agent)

	_, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"})
	if !errors.Is(err, errHostingActivation) {
		t.Fatalf("transition error = %v, want restored activation failure", err)
	}
	state := hostingTestState(t, panel, domainID)
	if state.ProjectType != "static" || state.PHPVersion.String != "" || state.PHPSocket.String != "" {
		t.Fatalf("restored state = %#v, want the static row as it was", state)
	}
	assertHostingCalls(t, agent.hostingTestAgent, []hostingAgentCall{
		{Method: "apply-pool", ProjectType: transport.SitePoolOperationCreate},
		{Method: "apply-vhost", ProjectType: "php"},
		{Method: "remove-app"},
		{Method: "apply-vhost", ProjectType: "static"},
		{Method: "delete-pool", ProjectType: "8.4"},
	})
}

func TestHostingTransitionToPHPWithoutPHPFPMChangesNothing(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent()
	attachPHPHostingTestAgent(t, panel, agent)

	_, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"})
	if !errors.Is(err, errHostingPHPMissing) {
		t.Fatalf("transition error = %v, want PHP-FPM missing", err)
	}
	if state := hostingTestState(t, panel, domainID); state.ProjectType != "static" {
		t.Fatalf("project type = %q, want static", state.ProjectType)
	}
	assertHostingCalls(t, agent.hostingTestAgent, nil)
}

func TestHostingTransitionToPHPRefusedPoolChangesNothing(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent("8.4")
	agent.poolState = transport.SiteFileAbsent
	agent.poolOutcome = transport.SiteFileOutcomeFailed
	attachPHPHostingTestAgent(t, panel, agent)

	_, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"})
	var held *siteFileHeldError
	if !errors.As(err, &held) || held.File.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("transition error = %v, want the pool's typed refusal", err)
	}
	if state := hostingTestState(t, panel, domainID); state.ProjectType != "static" || state.PHPSocket.String != "" {
		t.Fatalf("state = %#v, want the static row as it was", state)
	}
	assertHostingCalls(t, agent.hostingTestAgent, []hostingAgentCall{
		{Method: "apply-pool", ProjectType: transport.SitePoolOperationCreate},
	})
}

func TestHostingTransitionToPHPKeepsAndUsesOwnerEditedPool(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent("8.4")
	agent.poolState = transport.SiteFileOwnerEdited
	agent.poolOutcome = transport.SiteFileOutcomeKept
	attachPHPHostingTestAgent(t, panel, agent)

	if _, err := panel.transitionHosting(context.Background(), domainID, hostingSettings{ProjectType: "php"}); err != nil {
		t.Fatalf("static -> php with the owner's pool: %v", err)
	}
	if state := hostingTestState(t, panel, domainID); state.ProjectType != "php" || state.PHPVersion.String != "8.4" {
		t.Fatalf("state = %#v, want php 8.4", state)
	}
	if len(agent.deleted) != 0 {
		t.Fatalf("the owner's pool was deleted: %#v", agent.deleted)
	}
}

func TestHostingTransitionCustomerTakesNewestOfferedPHP(t *testing.T) {
	panel, domainID := newDomainAliasFixture(t)
	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	agent := newPHPHostingTestAgent("8.2", "8.4")
	attachPHPHostingTestAgent(t, panel, agent)
	policy := offeredVersions{Hidden: map[string][]string{"php": {"8.4"}}}

	if _, err := panel.transitionHostingWithPolicy(context.Background(), domainID, hostingSettings{ProjectType: "php"}, &policy); err != nil {
		t.Fatalf("customer static -> php: %v", err)
	}
	if state := hostingTestState(t, panel, domainID); state.PHPVersion.String != "8.2" {
		t.Fatalf("customer's PHP version = %q, want the newest offered 8.2", state.PHPVersion.String)
	}

	staticSiteAsAddDomainCreatesIt(t, panel, domainID)
	none := offeredVersions{Hidden: map[string][]string{"php": {"8.2", "8.4"}}}
	_, err := panel.transitionHostingWithPolicy(context.Background(), domainID, hostingSettings{ProjectType: "php"}, &none)
	if !errors.Is(err, errHostingPHPNotOffered) {
		t.Fatalf("transition error = %v, want no PHP version offered", err)
	}
}

// The switch's refusals carry their own reason, so the screens say that the
// site was not switched (the codes' plain sentences belong to Add Domain).
func TestHostingPHPRefusalsSayTheSiteWasNotSwitched(t *testing.T) {
	for _, code := range []string{errCodePHPRequired, errCodeNoVersionOffered} {
		recorder := httptest.NewRecorder()
		writeHostingPHPRefusal(recorder, code, "/services", "not switched")
		var body apiErrorBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusConflict || body.Code != code || body.Reason != hostingReasonSwitch || body.Vars["name"] != "PHP" {
			t.Fatalf("%s refusal = %d %+v", code, recorder.Code, body)
		}
	}
}
