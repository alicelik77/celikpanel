package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.127 lab finding, fixed in alpha.128: changing a site's PHP version
// while nginx is stopped answered INTERNAL after the pool had moved, and the
// site gave 502 until the next vhost write. Now a known-stopped web server
// refuses before anything changes, and a vhost that does not follow the new
// socket moves the pool and the record back.

type phpSwitchTestAgent struct {
	*phpHostingTestAgent
	nginxState string
}

func (a *phpSwitchTestAgent) ServiceHealthV1(req *transport.ServiceHealthRequest, resp *transport.ServiceHealthResponse) error {
	resp.Batched = true
	for _, unit := range req.Units {
		resp.Units = append(resp.Units, transport.ServiceHealthUnit{Unit: unit, LoadState: "loaded", ActiveState: a.nginxState})
	}
	return nil
}

func phpSwitchFixture(t *testing.T, nginxState string) (*Panel, int, *phpSwitchTestAgent) {
	t.Helper()
	panel, domainID := newDomainAliasFixture(t)
	resetHostingTestState(t, panel, domainID, "php")
	var siteID int
	if err := panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = ?`, domainID).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	socket := services.PHPFPMSocketPath("8.2", fmt.Sprintf("site%d", siteID))
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET php_version = '8.2', php_fpm_socket = ? WHERE domain_id = ?`, socket, domainID); err != nil {
		t.Fatal(err)
	}
	base := newPHPHostingTestAgent("8.2", "8.4")
	base.poolExists, base.poolState = true, transport.SiteFileManagedUnchanged
	agent := &phpSwitchTestAgent{phpHostingTestAgent: base, nginxState: nginxState}
	attachPHPSwitchTestAgent(t, panel, agent)
	return panel, domainID, agent
}

func attachPHPSwitchTestAgent(t *testing.T, panel *Panel, agent *phpSwitchTestAgent) {
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

func postPHPVersion(panel *Panel, domainID int, version string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/domains/%d/php", domainID),
		strings.NewReader(`{"php_version":"`+version+`"}`))
	caller := &Caller{ID: 1, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, request)
	return recorder
}

func phpSwitchAnswerCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

func TestPHPVersionChangeRefusesBeforeAnythingWhenNginxIsStopped(t *testing.T) {
	panel, domainID, agent := phpSwitchFixture(t, "inactive")
	recorder := postPHPVersion(panel, domainID, "8.4")
	if recorder.Code != http.StatusConflict || phpSwitchAnswerCode(t, recorder) != errCodeWebServerStopped {
		t.Fatalf("answer %d %s, want 409 %s", recorder.Code, recorder.Body.String(), errCodeWebServerStopped)
	}
	if len(agent.poolRequests) != 0 {
		t.Fatalf("pool requests %#v, want none", agent.poolRequests)
	}
	var version string
	if err := panel.db.GetDB().QueryRow(`SELECT php_version FROM sites WHERE domain_id = ?`, domainID).Scan(&version); err != nil || version != "8.2" {
		t.Fatalf("version %q %v, want 8.2", version, err)
	}
}

func TestPHPVersionChangeMovesBackWhenTheVhostIsRefused(t *testing.T) {
	panel, domainID, agent := phpSwitchFixture(t, "active")
	agent.applyVhost = func(_ *transport.ApplyVhostRequest, resp *transport.ApplyVhostResponse) error {
		resp.Error = "nginx reload failed: nginx.service is not active"
		return nil
	}
	recorder := postPHPVersion(panel, domainID, "8.4")
	if recorder.Code != http.StatusBadGateway || phpSwitchAnswerCode(t, recorder) != errCodePHPVersionUndone {
		t.Fatalf("answer %d %s, want 502 %s", recorder.Code, recorder.Body.String(), errCodePHPVersionUndone)
	}
	if len(agent.poolRequests) != 2 ||
		agent.poolRequests[0].PHPVersion != "8.2" || agent.poolRequests[0].NewPHPVersion != "8.4" ||
		agent.poolRequests[1].PHPVersion != "8.4" || agent.poolRequests[1].NewPHPVersion != "8.2" {
		t.Fatalf("pool requests %#v, want switch 8.2->8.4 then back", agent.poolRequests)
	}
	var version, socket string
	if err := panel.db.GetDB().QueryRow(`SELECT php_version, php_fpm_socket FROM sites WHERE domain_id = ?`, domainID).Scan(&version, &socket); err != nil ||
		version != "8.2" || !strings.Contains(socket, "8.2") {
		t.Fatalf("record %q %q %v, want back on 8.2", version, socket, err)
	}
}

func TestPHPVersionChangeStillWorksWithNginxRunning(t *testing.T) {
	panel, domainID, agent := phpSwitchFixture(t, "active")
	recorder := postPHPVersion(panel, domainID, "8.4")
	if recorder.Code != http.StatusOK {
		t.Fatalf("answer %d %s", recorder.Code, recorder.Body.String())
	}
	if len(agent.poolRequests) != 1 || len(agent.vhostSockets) != 1 || !strings.Contains(agent.vhostSockets[0], "8.4") {
		t.Fatalf("pool %#v vhost %#v", agent.poolRequests, agent.vhostSockets)
	}
}

// alpha.128: the site's PHP settings within its plan's limits.

type phpLimitsTestAgent struct {
	*phpSwitchTestAgent
	own          transport.SitePHPSettings
	processes    int
	nginxOutcome string
	nginxSent    []transport.SiteSettingsIncludeRequest
}

func (a *phpLimitsTestAgent) SitePHPSettingsV1(req *transport.SitePHPSettingsRequest, resp *transport.SitePHPSettingsResponse) error {
	for _, site := range req.Sites {
		resp.Sites = append(resp.Sites, transport.SitePHPSettingsState{SiteID: site.SiteID, PoolState: transport.SiteFileManagedUnchanged,
			Own: a.own, Processes: a.processes, Server: transport.SitePHPServerValues{MemoryLimit: "128M", MaxExecutionTime: "30"}})
	}
	return nil
}

func (a *phpLimitsTestAgent) ApplySiteSettingsIncludeV1(req *transport.SiteSettingsIncludeRequest, resp *transport.SiteSettingsIncludeResponse) error {
	a.nginxSent = append(a.nginxSent, *req)
	resp.Outcome = a.nginxOutcome
	if resp.Outcome == "" {
		resp.Outcome = transport.SiteSettingsIncludeWritten
	}
	if resp.Outcome == transport.SiteSettingsIncludeRefused {
		resp.Restored, resp.Detail, resp.Error = true, "nginx: [emerg] invalid value", "nginx refused site-settings.conf, the previous file was put back"
	}
	return nil
}

func phpLimitsFixture(t *testing.T) (*Panel, int, *phpLimitsTestAgent) {
	t.Helper()
	panel, domainID, base := phpSwitchFixture(t, "active")
	agent := &phpLimitsTestAgent{phpSwitchTestAgent: base, processes: 5}
	db := panel.db.GetDB()
	var subID int
	if err := db.QueryRow(`SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO service_plans (name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES ('Small', 0, 0, 0, 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	planID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', ?, 'php.memory_limit_mb', 256), ('plan', ?, 'php.max_children', 8)`, planID, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE subscriptions SET plan_id = ?, name = 'Customer' WHERE id = ?`, planID, subID); err != nil {
		t.Fatal(err)
	}
	attachPHPLimitsTestAgent(t, panel, agent)
	return panel, domainID, agent
}

func attachPHPLimitsTestAgent(t *testing.T, panel *Panel, agent *phpLimitsTestAgent) {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
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

func callPHPLimits(panel *Panel, domainID int, method, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, fmt.Sprintf("/api/v1/domains/%d/php/settings", domainID), strings.NewReader(body))
	caller := &Caller{ID: 1, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	panel.handleDomainPHPLimits(recorder, request, domainID)
	return recorder
}

func TestSitePHPSettingsStayWithinThePlanAndShowWhoSetTheLimit(t *testing.T) {
	panel, domainID, agent := phpLimitsFixture(t)
	read := callPHPLimits(panel, domainID, http.MethodGet, "")
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"set_by":"plan"`) || !strings.Contains(read.Body.String(), `"set_by_name":"Small"`) {
		t.Fatalf("read %d %s", read.Code, read.Body.String())
	}
	above := callPHPLimits(panel, domainID, http.MethodPut, `{"memory_mb":512}`)
	if above.Code != http.StatusConflict || phpSwitchAnswerCode(t, above) != errCodeSettingAboveLimit || len(agent.poolRequests) != 0 {
		t.Fatalf("above the plan: %d %s pool=%d", above.Code, above.Body.String(), len(agent.poolRequests))
	}
	saved := callPHPLimits(panel, domainID, http.MethodPut, `{"memory_mb":256,"time_s":120,"upload_mb":64,"processes":6,"display_errors":"off"}`)
	if saved.Code != http.StatusOK {
		t.Fatalf("save %d %s", saved.Code, saved.Body.String())
	}
	if len(agent.poolRequests) != 1 || agent.poolRequests[0].Operation != transport.SitePoolOperationSettings ||
		agent.poolRequests[0].PHPSettings.MemoryMB != 256 || !agent.poolRequests[0].PHPLimits.Known || agent.poolRequests[0].PHPLimits.MemoryMB != 256 {
		t.Fatalf("pool requests %+v", agent.poolRequests)
	}
	if len(agent.nginxSent) != 1 || agent.nginxSent[0].UploadMB != 64 || agent.nginxSent[0].TimeoutS != 130 {
		t.Fatalf("nginx %+v", agent.nginxSent)
	}
}

func TestSitePHPSettingsPutThePoolBackWhenNginxRefuses(t *testing.T) {
	panel, domainID, agent := phpLimitsFixture(t)
	agent.own = transport.SitePHPSettings{MemoryMB: 128}
	agent.nginxOutcome = transport.SiteSettingsIncludeRefused
	refused := callPHPLimits(panel, domainID, http.MethodPut, `{"memory_mb":256,"upload_mb":64}`)
	if refused.Code != http.StatusBadGateway || phpSwitchAnswerCode(t, refused) != errCodeSiteSettingsNginxRefuse {
		t.Fatalf("refusal %d %s", refused.Code, refused.Body.String())
	}
	if len(agent.poolRequests) != 2 || agent.poolRequests[1].PHPSettings.MemoryMB != 128 || agent.poolRequests[1].PHPSettings.Processes != 5 {
		t.Fatalf("the pool was not written back: %+v", agent.poolRequests)
	}
}

func TestLoweredLimitNoticesTheSitesAboveItOnce(t *testing.T) {
	panel, domainID, agent := phpLimitsFixture(t)
	agent.own = transport.SitePHPSettings{MemoryMB: 256}
	ctx := context.Background()
	db := panel.db.GetDB()
	var planID, subID, ownerID int
	if err := db.QueryRow(`SELECT s.plan_id, s.id, s.owner_id FROM subscriptions s JOIN domains d ON d.subscription_id = s.id WHERE d.id = ?`, domainID).Scan(&planID, &subID, &ownerID); err != nil {
		t.Fatal(err)
	}
	preview := rightsPreview{}
	if err := panel.previewLoweredCeilings(ctx, &preview, planID, []string{settingPHPMemoryMB}, map[string]int{settingPHPMemoryMB: 128}); err != nil {
		t.Fatal(err)
	}
	if preview.SitesAboveCount != 1 || len(preview.clamped) != 1 || preview.clamped[0].Value != 256 || preview.clamped[0].Limit != 128 {
		t.Fatalf("preview %+v clamped %+v", preview, preview.clamped)
	}
	panel.storeClampNotices(ctx, preview.clamped, settingSetByPlan, "Small")
	panel.storeClampNotices(ctx, preview.clamped, settingSetByPlan, "Small")
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rights_notices WHERE kind = ? AND acknowledged_at IS NULL`, noticeSiteLimitClamped).Scan(&rows); err != nil || rows == 0 {
		t.Fatalf("notices %d %v", rows, err)
	}
	var perPerson int
	if err := db.QueryRow(`SELECT MAX(n) FROM (SELECT COUNT(*) AS n FROM rights_notices WHERE kind = ? GROUP BY user_id)`, noticeSiteLimitClamped).Scan(&perPerson); err != nil || perPerson != 1 {
		t.Fatalf("a person got the notice %d times: %v", perPerson, err)
	}
	// The site screen says the value is clamped where it is shown.
	agent.own = transport.SitePHPSettings{MemoryMB: 512}
	read := callPHPLimits(panel, domainID, http.MethodGet, "")
	if !strings.Contains(read.Body.String(), `"clamped":true`) {
		t.Fatalf("view does not mark the clamped value: %s", read.Body.String())
	}
}
