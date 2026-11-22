package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.143 lab items 3 and 5 (alpha.144).

// Item 5: "1 PostgreSQL databases" was one database; the alert mail says it
// in the singular too, the Turkish line keeps its singular noun.
func TestA144OnePostgreSQLDatabaseIsSaidInTheSingular(t *testing.T) {
	for _, kind := range []string{"postgresql_databases_open", "postgresql_databases_unreadable"} {
		en, tr := alertLine(pendingAlertMail{kind: kind, value: 1, name: "atolye_db"})
		if strings.Contains(en, "databases") || !strings.Contains(en, "atolye_db") || strings.Contains(tr, "bunlara") || strings.Contains(tr, "okunamadılar") {
			t.Fatalf("%s, one: %q / %q", kind, en, tr)
		}
		en, _ = alertLine(pendingAlertMail{kind: kind, value: 2, name: "a, b"})
		if !strings.Contains(en, "databases") {
			t.Fatalf("%s, two: %q", kind, en)
		}
	}
}

// Item 3: the kind of cause in the database server's message.
func TestA144PGFailureKindNamesTheCause(t *testing.T) {
	for detail, want := range map[string]string{
		"ERROR:  canceling statement due to lock timeout":            "in_use",
		"ERROR: could not obtain lock on relation":                   "in_use",
		"ERROR:  canceling statement due to statement timeout":       "timeout",
		"server closed the connection unexpectedly":                  "connection",
		"FATAL: terminating connection due to administrator command": "connection",
		"ERROR:  permission denied for database x":                   "rights",
		`ERROR:  role "pastane_dr_test_q3_ek" does not exist`:        "missing",
		"ERROR:  syntax error at or near":                            "refused",
		"":                                                           "refused",
	} {
		if got := pgPublicFailureKind(detail); got != want {
			t.Errorf("%q: %s, want %s", detail, got, want)
		}
	}
}

// Item 3: a verified failed close answers with the kind of cause as its
// reason and the server's message quoted; the list names the kind too.
func TestA144FailedCloseAnswersTheKindOfCause(t *testing.T) {
	f := pgPublicFixture(t)
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		out.Detail = "ERROR:  canceling statement due to lock timeout"
		return nil
	}
	rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	var body apiErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusBadGateway || body.Code != errCodePGPublicFailed ||
		body.Reason != "in_use" || len(body.Details) != 1 || !strings.Contains(body.Details[0], "lock timeout") {
		t.Fatalf("failed close: %d %s", rec.Code, rec.Body.String())
	}
	read := pgPublicPost(t, f.visibleDatabasesFixture, "admin", nil)
	var view pgPublicView
	if err := json.Unmarshal(read.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	entry := pgPublicEntryOf(view.Open, "foreign_db")
	if entry == nil || entry.FailureCode != "server" || entry.FailureKind != "in_use" {
		t.Fatalf("the list's entry: %+v", entry)
	}
}

// a144WebmailFormsAgent answers which webmail address files are still open.
type a144WebmailFormsAgent struct {
	verifiedAPTAgentRPCFixture
	open []transport.WebmailAddressForm
}

func (a *a144WebmailFormsAgent) WebmailAddressFormsV1(_ *transport.WebmailAddressFormsRequest, resp *transport.WebmailAddressFormsResponse) error {
	resp.Open = a.open
	return nil
}

// Review of PR #355, finding 2: a webmail address file the start could not
// bring to the current form is a Problems-now line with why, and goes once
// the Agent no longer lists it.
func TestA144WebmailAddressInItsEarlierFormIsAProblemUntilItIsNot(t *testing.T) {
	agent := &a144WebmailFormsAgent{open: []transport.WebmailAddressForm{
		{Host: "webmail.example.test", State: "owner_edited", File: "/etc/nginx/conf.d/celikpanel-webmail-origin-webmail.example.test.conf.celikpanel-pending"},
		{Host: "mail2.example.test", State: "test_failed", File: "/etc/nginx/conf.d/broken.conf"},
	}}
	p := newDNSPanelForTest(t)
	p.pkgFamilyVal = "apt"
	p.agentClient = a144AgentClient(t, agent)
	problems := p.webmailAddressProblems(context.Background())
	if len(problems) != 2 || problems[0].Kind != "webmail_address_kept" || problems[0].Subject != "webmail.example.test" || !strings.HasSuffix(problems[0].Name, ".celikpanel-pending") ||
		problems[1].Kind != "webmail_address_refused" || problems[1].Name != "/etc/nginx/conf.d/broken.conf" {
		t.Fatalf("problems: %+v", problems)
	}
	if en, tr := alertLine(pendingAlertMail{kind: "webmail_address_refused", subject: "mail2.example.test", name: "/etc/nginx/conf.d/broken.conf"}); !strings.Contains(en, "broken.conf") || !strings.Contains(tr, "broken.conf") {
		t.Fatalf("alert mail: %q / %q", en, tr)
	}
	agent.open = nil
	if problems := p.webmailAddressProblems(context.Background()); len(problems) != 0 {
		t.Fatalf("still listed: %+v", problems)
	}
}
