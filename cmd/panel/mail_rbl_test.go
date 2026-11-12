package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133: a blocklist that refused the question (Spamhaus's
// 127.255.255.254 to a public resolver) was shown as "this server is on a
// spam blocklist" on three screens. A refusal is now "could not check" on
// all three and never colours the domain; an older Agent's bare "listed"
// is never trusted as a listing.

func newRBL(zone, outcome, reason string, sublists ...string) transport.RBLResult {
	return transport.RBLResult{Zone: zone, Listed: outcome == transport.RBLListed, Outcome: outcome, Reason: reason, Sublists: sublists}
}

func TestRBLViewThreeOutcomesAndOlderAgent(t *testing.T) {
	cases := []struct {
		name    string
		results []transport.RBLResult
		want    rblSummary
		listed  []bool
	}{
		{"all clean", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLNotListed, ""),
			newRBL("bl.spamcop.net", transport.RBLNotListed, ""),
		}, rblSummary{Status: "ok", Code: "clean"}, []bool{false, false}},
		{"public resolver refusal is could-not-check", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLNotChecked, transport.RBLReasonPublicResolver),
			newRBL("bl.spamcop.net", transport.RBLNotListed, ""),
			newRBL("b.barracudacentral.org", transport.RBLNotChecked, transport.RBLReasonLookupFailed),
		}, rblSummary{Status: "unknown", Code: transport.RBLReasonPublicResolver, Items: []string{"Spamhaus", "Barracuda"}}, []bool{false, false, false}},
		{"too many queries", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLNotChecked, transport.RBLReasonTooManyQueries),
		}, rblSummary{Status: "unknown", Code: transport.RBLReasonTooManyQueries, Items: []string{"Spamhaus"}}, []bool{false}},
		{"spam listing names the list and its parts", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLListed, "", "sbl", "pbl"),
			newRBL("bl.spamcop.net", transport.RBLListed, ""),
		}, rblSummary{Status: "fail", Code: transport.RBLListed, Items: []string{"Spamhaus (SBL, PBL)", "SpamCop"}}, []bool{true, true}},
		{"policy entry only", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLListed, "", "pbl"),
			newRBL("bl.spamcop.net", transport.RBLNotChecked, transport.RBLReasonLookupFailed),
		}, rblSummary{Status: "fail", Code: "policy", Items: []string{"Spamhaus (PBL)"}}, []bool{true, false}},
		{"spam beside policy: spam first, both named", []transport.RBLResult{
			newRBL("zen.spamhaus.org", transport.RBLListed, "", "pbl"),
			newRBL("b.barracudacentral.org", transport.RBLListed, ""),
		}, rblSummary{Status: "fail", Code: transport.RBLListed, Items: []string{"Barracuda", "Spamhaus (PBL)"}}, []bool{true, true}},
		{"older agent: its listed is never trusted", []transport.RBLResult{
			{Zone: "zen.spamhaus.org", Listed: true},
			{Zone: "dnsbl.sorbs.net", Listed: false},
		}, rblSummary{Status: "unknown", Code: transport.RBLReasonOlderAgent, Items: []string{"Spamhaus", "dnsbl.sorbs.net"}}, []bool{false, false}},
		{"a newer outcome or reason is never guessed", []transport.RBLResult{
			{Zone: "zen.spamhaus.org", Listed: true, Outcome: "suspended", Reason: "new_reason"},
		}, rblSummary{Status: "unknown", Code: transport.RBLReasonUnknownAnswer, Items: []string{"Spamhaus"}}, []bool{false}},
		{"no list answered", nil, rblSummary{Status: "unknown"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			view := rblView(transport.CheckRBLResponse{IP: "203.0.113.7", Results: c.results})
			if !reflect.DeepEqual(view.Summary, c.want) {
				t.Fatalf("summary = %+v, want %+v", view.Summary, c.want)
			}
			for i, list := range view.Results {
				if list.Listed != c.listed[i] || list.Listed != (list.Outcome == transport.RBLListed) {
					t.Fatalf("result %d = %+v, want listed %v", i, list, c.listed[i])
				}
				if list.Outcome == transport.RBLNotChecked && list.Reason == "" {
					t.Fatalf("result %d could not be checked without a reason: %+v", i, list)
				}
			}
		})
	}
}

func TestMailHealthRBLRefusalNeverColoursTheCard(t *testing.T) {
	refused := transport.CheckRBLResponse{IP: "203.0.113.7", Results: []transport.RBLResult{
		newRBL("zen.spamhaus.org", transport.RBLNotChecked, transport.RBLReasonPublicResolver),
		newRBL("bl.spamcop.net", transport.RBLNotListed, ""),
		newRBL("b.barracudacentral.org", transport.RBLNotListed, ""),
	}}
	check := mailHealthRBLCheck(refused, nil)
	if check.Status != "unknown" || check.Detail != transport.RBLReasonPublicResolver || !reflect.DeepEqual(check.Items, []string{"Spamhaus"}) {
		t.Fatalf("rbl = %+v", check)
	}
	fine := []healthCheck{{ID: "ptr", Status: "ok"}, {ID: "spf", Status: "ok"}}
	if got := mailHealthOverall(append(fine, check)); got != "ok" {
		t.Fatalf("overall with a refused list = %q, want ok", got)
	}
	if got := mailHealthOverall(append(fine, mailHealthRBLCheck(transport.CheckRBLResponse{}, fmt.Errorf("agent down")))); got != "ok" {
		t.Fatalf("overall with an unreadable blocklist check = %q, want ok", got)
	}
	// Another check that could not be read still says so.
	if got := mailHealthOverall(append(fine, check, healthCheck{ID: "port25", Status: "unknown"})); got != "warn" {
		t.Fatalf("overall with port 25 unknown = %q, want warn", got)
	}
	listed := mailHealthRBLCheck(transport.CheckRBLResponse{IP: "203.0.113.7", Results: []transport.RBLResult{
		newRBL("zen.spamhaus.org", transport.RBLListed, "", "xbl"),
	}}, nil)
	if listed.Status != "fail" || listed.Detail != transport.RBLListed || mailHealthOverall(append(fine, listed)) != "fail" {
		t.Fatalf("listed = %+v", listed)
	}
	clean := mailHealthRBLCheck(transport.CheckRBLResponse{IP: "203.0.113.7", Results: []transport.RBLResult{
		newRBL("zen.spamhaus.org", transport.RBLNotListed, ""),
	}}, nil)
	if clean.Status != "ok" || clean.Detail != "" {
		t.Fatalf("clean = %+v", clean)
	}
	// An older Agent's "listed" (possibly a refusal) never turns the card red.
	old := mailHealthRBLCheck(transport.CheckRBLResponse{IP: "203.0.113.7", Results: []transport.RBLResult{{Zone: "zen.spamhaus.org", Listed: true}}}, nil)
	if old.Status != "unknown" || old.Detail != transport.RBLReasonOlderAgent {
		t.Fatalf("older agent = %+v", old)
	}
}

func TestDomainHealthMailBlocklistLines(t *testing.T) {
	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: authzMatrixCustomerID}
	cases := []struct {
		name  string
		rbl   healthCheck
		state string
		code  string
		items []string
	}{
		{"refused list keeps the domain fine", healthCheck{ID: "rbl", Status: "unknown", Detail: transport.RBLReasonPublicResolver, Items: []string{"Spamhaus"}},
			"ok", "complete", nil},
		{"unreadable check keeps the domain fine", healthCheck{ID: "rbl", Status: "unknown"}, "ok", "complete", nil},
		{"spam listing names the list", healthCheck{ID: "rbl", Status: "fail", Detail: transport.RBLListed, Items: []string{"SpamCop"}},
			"problem", "listed", []string{"SpamCop"}},
		{"policy entry is said as one", healthCheck{ID: "rbl", Status: "fail", Detail: "policy", Items: []string{"Spamhaus (PBL)"}},
			"problem", "listed_policy", []string{"Spamhaus (PBL)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fixture := newAuthzMatrixFixture(t)
			previous := displayChecksEnabled.Load()
			displayChecksEnabled.Store(true)
			t.Cleanup(func() { displayChecksEnabled.Store(previous) })
			key := fmt.Sprintf("%d|%s|%s", authzMatrixCustomerDomainID, "customer.matrix.test", setupDNSModeLocal)
			if _, _, err := fixture.panel.displayChecks.ready().mailHealth.get(context.Background(), key,
				func(context.Context) (mailHealthReport, error) {
					return mailHealthReport{Checks: []healthCheck{
						{ID: "spf", Status: "ok"}, {ID: "dkim", Status: "ok"}, {ID: "dmarc", Status: "ok"},
						{ID: "port25", Status: "ok"}, {ID: "ptr", Status: "ok"}, c.rbl,
					}}, nil
				}); err != nil {
				t.Fatal(err)
			}
			for _, caller := range []*Caller{admin, customer} {
				_, body := readDomainHealth(t, fixture, caller, authzMatrixCustomerDomainID)
				mail := body.check("mail")
				if mail == nil || mail.State != c.state || mail.Code != c.code || !reflect.DeepEqual(mail.Items, c.items) {
					t.Fatalf("role %s: mail = %+v, want %s/%s %v", caller.Role, mail, c.state, c.code, c.items)
				}
				if c.state == "problem" {
					wantKind := "contact"
					if caller.Role == roleAdmin {
						wantKind = "tab"
					}
					if mail.Fix == nil || mail.Fix.Kind != wantKind {
						t.Fatalf("role %s: fix = %+v, want %s", caller.Role, mail.Fix, wantKind)
					}
				}
			}
		})
	}
}

// --- "Check now": the manual check over a real net/rpc (gob) connection,
// with a current Agent and with an Agent older than alpha.133.

type rblTestAgent struct{ reply transport.CheckRBLResponse }

func (a *rblTestAgent) CheckRBL(_ *transport.Empty, reply *transport.CheckRBLResponse) error {
	*reply = a.reply
	return nil
}

// The answer shape of an Agent older than alpha.133.
type RBLTestOldResult struct {
	Zone   string
	Listed bool
	Detail string
}
type RBLTestOldResponse struct {
	IP      string
	Results []RBLTestOldResult
	Error   string
}
type rblTestOldAgent struct{ reply RBLTestOldResponse }

func (a *rblTestOldAgent) CheckRBL(_ *transport.Empty, reply *RBLTestOldResponse) error {
	*reply = a.reply
	return nil
}

func newRBLTestPanel(t *testing.T, agent any) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
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
	t.Cleanup(func() { _ = client.Close() })
	return &Panel{agentClient: transport.NewReconnectingClientWithContextConnector(client, connector), db: database}
}

func readManualRBL(t *testing.T, panel *Panel) rblReportView {
	t.Helper()
	recorder := httptest.NewRecorder()
	panel.handleMailRBL(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/rbl", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var view rblReportView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestManualRBLCheckRefusalIsNeverListed(t *testing.T) {
	panel := newRBLTestPanel(t, &rblTestAgent{reply: transport.CheckRBLResponse{IP: "203.0.113.7", Results: []transport.RBLResult{
		newRBL("zen.spamhaus.org", transport.RBLNotChecked, transport.RBLReasonPublicResolver),
		newRBL("bl.spamcop.net", transport.RBLNotListed, ""),
		newRBL("b.barracudacentral.org", transport.RBLNotListed, ""),
	}}})
	view := readManualRBL(t, panel)
	if view.IP != "203.0.113.7" || len(view.Results) != 3 {
		t.Fatalf("view = %+v", view)
	}
	for _, list := range view.Results {
		if list.Listed {
			t.Fatalf("%s listed: %+v", list.Zone, list)
		}
	}
	if spamhaus := view.Results[0]; spamhaus.Outcome != transport.RBLNotChecked || spamhaus.Reason != transport.RBLReasonPublicResolver || spamhaus.Name != "Spamhaus" {
		t.Fatalf("spamhaus = %+v", spamhaus)
	}
	if view.Summary.Status != "unknown" || view.Summary.Code != transport.RBLReasonPublicResolver {
		t.Fatalf("summary = %+v", view.Summary)
	}
}

func TestManualRBLCheckOlderAgentListedIsNotTrusted(t *testing.T) {
	panel := newRBLTestPanel(t, &rblTestOldAgent{reply: RBLTestOldResponse{IP: "203.0.113.7", Results: []RBLTestOldResult{
		{Zone: "zen.spamhaus.org", Listed: true, Detail: "open resolver"},
		{Zone: "bl.spamcop.net"},
	}}})
	view := readManualRBL(t, panel)
	for _, list := range view.Results {
		if list.Listed || list.Outcome != transport.RBLNotChecked || list.Reason != transport.RBLReasonOlderAgent || list.Detail != "" {
			t.Fatalf("older agent answer = %+v", list)
		}
	}
	if view.Summary.Status != "unknown" || view.Summary.Code != transport.RBLReasonOlderAgent {
		t.Fatalf("summary = %+v", view.Summary)
	}
}
