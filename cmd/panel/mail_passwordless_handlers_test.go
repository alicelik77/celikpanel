package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestPasswordlessMailViewPlacesSendersOnSites(t *testing.T) {
	report := transport.PasswordlessMailReportResponse{
		Read:         true,
		CoveredSince: "2026-10-10T08:00:00Z",
		LocalSenders: []transport.PasswordlessLocalSender{
			{UID: 0, User: "root", System: true, Messages: 9},
			{UID: 5000, User: "vmail", System: true, Messages: 2},
			{UID: 1001, User: "shop_example", Messages: 3, Last: "2026-10-11T00:00:00Z"},
			{UID: 1002, User: "bob", Messages: 1},
			{UID: 1003, Messages: 1},
		},
		LoopbackSenders: []transport.PasswordlessLoopbackSender{
			{Sender: "Contact@Shop.Example", Messages: 2, Last: "2026-10-12T00:00:00Z"},
			{Sender: "no-reply@panel.example", Messages: 4},
			{Sender: "x@elsewhere.example", Messages: 1},
		},
	}
	domains := []passwordlessDomainRow{{id: 7, name: "shop.example"}, {id: 8, name: "quiet.example"}}
	view := buildPasswordlessMailView(report, domains, "no-reply@panel.example")
	if view.State != "found" || view.WindowDays != 30 || view.CoveredSince != report.CoveredSince {
		t.Fatalf("%+v", view)
	}
	if len(view.Sites) != 1 {
		t.Fatalf("sites %+v", view.Sites)
	}
	shop := view.Sites[0]
	if shop.DomainID != 7 || shop.Messages != 5 || len(shop.Ways) != 2 || shop.Last != "2026-10-12T00:00:00Z" {
		t.Fatalf("shop %+v", shop)
	}
	// root, vmail and the panel's own sender are not listed; the others are.
	if len(view.Others) != 3 || view.Others[0].Label != "bob" || view.Others[1].Label != "uid 1003" ||
		view.Others[2].Label != "x@elsewhere.example" {
		t.Fatalf("others %+v", view.Others)
	}
}

func TestPasswordlessMailViewUnknownAndNone(t *testing.T) {
	unknown := buildPasswordlessMailView(transport.PasswordlessMailReportResponse{Error: "Postfix's log could not be read: x",
		Enforcement: &transport.PasswordlessMailEnforcement{State: "closed"}}, nil, "")
	if unknown.State != "unknown" || unknown.Reason == "" || unknown.Sites == nil || unknown.Others == nil ||
		unknown.Enforcement == nil || unknown.Enforcement.State != "closed" {
		t.Fatalf("%+v", unknown)
	}
	none := buildPasswordlessMailView(transport.PasswordlessMailReportResponse{Read: true,
		LocalSenders: []transport.PasswordlessLocalSender{{UID: 0, User: "root", System: true, Messages: 1}}}, nil, "")
	if none.State != "none" {
		t.Fatalf("%+v", none)
	}
}

type passwordlessPanelAgent struct {
	report transport.PasswordlessMailReportResponse
	sent   []transport.SendAccountMailRequest
}

func (a *passwordlessPanelAgent) PasswordlessMailReportV1(_ *transport.Empty, response *transport.PasswordlessMailReportResponse) error {
	*response = a.report
	return nil
}

// HostPlatform: a verified Debian identity, so the host-mutation policy lets
// Agent.SendAccountMailV1 through as on a real server.
func (a *passwordlessPanelAgent) HostPlatform(_ *transport.Empty, out *transport.HostPlatformResponse) error {
	*out = debianPolicyTestIdentity()
	return nil
}

func (a *passwordlessPanelAgent) SendAccountMailV1(request *transport.SendAccountMailRequest, response *transport.SendAccountMailResponse) error {
	a.sent = append(a.sent, *request)
	*response = transport.SendAccountMailResponse{Sent: true}
	return nil
}

func newPasswordlessPanel(t *testing.T, agent *passwordlessPanelAgent) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role)
		VALUES (6301, 'mail-admin', 'x', 'mail-admin@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, max_email_accounts, status)
		VALUES (6302, 6301, 'Mail subscription', 50, 'active');
		INSERT INTO domains (id, subscription_id, name, status)
		VALUES (6303, 6302, 'shop.example', 'active'), (6304, 6302, 'blog.example', 'active');
		INSERT INTO email_accounts (id, domain_id, address, password_hash, quota_mb)
		VALUES (6401, 6304, 'noreply@blog.example', 'x', 100);
	`); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
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
	return &Panel{db: database, agentClient: transport.NewReconnectingClientWithContextConnector(client, connector)}
}

func TestPasswordlessMailHandlerShowsSitesAndTheirSendingMailbox(t *testing.T) {
	agent := &passwordlessPanelAgent{report: transport.PasswordlessMailReportResponse{
		Read:         true,
		LocalSenders: []transport.PasswordlessLocalSender{{UID: 1001, User: "shop_example", Messages: 2}},
		LoopbackSenders: []transport.PasswordlessLoopbackSender{
			{Sender: "contact@blog.example", Messages: 1, Refused: 1},
		},
		Enforcement: &transport.PasswordlessMailEnforcement{State: "closed"},
	}}
	panel := newPasswordlessPanel(t, agent)
	request := httptest.NewRequest(http.MethodGet, passwordlessMailPath, nil)
	response := httptest.NewRecorder()
	panel.handlePasswordlessMail(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	var view passwordlessMailView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.State != "found" || len(view.Sites) != 2 || view.Enforcement == nil || view.Enforcement.State != "closed" {
		t.Fatalf("%+v", view)
	}
	if view.Sites[0].Domain != "blog.example" || view.Sites[0].SenderMailbox != "noreply@blog.example" || view.Sites[0].Refused != 1 ||
		view.Sites[1].Domain != "shop.example" || view.Sites[1].SenderMailbox != "" {
		t.Fatalf("%+v", view.Sites)
	}
}

// Account mail no longer relays through 127.0.0.1:25 without a password: the
// Agent hands it to sendmail (D-046).
func TestAccountMailIsHandedToTheAgent(t *testing.T) {
	agent := &passwordlessPanelAgent{}
	panel := newPasswordlessPanel(t, agent)
	saved := accountMailSend
	t.Cleanup(func() { accountMailSend = saved })
	accountMailSend = nil
	setup := accountMailSetup{accountMailStatus: accountMailStatus{Available: true}, baseURL: "https://panel.example:1923",
		from: "no-reply@panel.example", domain: "panel.example", host: "panel.example"}
	if err := panel.sendAccountMail(context.Background(), setup, "user@example.org",
		accountMailContent{kind: accountMailReset, lang: "en", username: "alice", link: "https://panel.example:1923/reset#x"}); err != nil {
		t.Fatal(err)
	}
	if len(agent.sent) != 1 || agent.sent[0].From != "no-reply@panel.example" || agent.sent[0].To != "user@example.org" ||
		!strings.Contains(string(agent.sent[0].Message), "To: <user@example.org>") {
		t.Fatalf("%+v", agent.sent)
	}
}
