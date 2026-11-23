package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.145 (alpha.144 lab D1 and O1).

// a145WebmailSendingAgent answers whether webmail's configuration can send.
type a145WebmailSendingAgent struct {
	verifiedAPTAgentRPCFixture
	sending, reason, file string
}

func (a *a145WebmailSendingAgent) WebmailAddressFormsV1(_ *transport.WebmailAddressFormsRequest, resp *transport.WebmailAddressFormsResponse) error {
	resp.Open = []transport.WebmailAddressForm{}
	resp.Sending, resp.SendingReason, resp.SendingFile = a.sending, a.reason, a.file
	return nil
}

// D1: webmail whose configuration still sends without encryption is one
// line in Problems now, for the server owner, naming CelikPanel's text
// beside the file; it goes by itself once the Agent no longer reads it so.
func TestA145WebmailThatCannotSendIsAProblemUntilItCan(t *testing.T) {
	pending := "/var/lib/celikpanel-webmail/config/config.inc.php.celikpanel-pending"
	agent := &a145WebmailSendingAgent{sending: "cannot_send", file: pending}
	p := newDNSPanelForTest(t)
	p.pkgFamilyVal = "apt"
	p.agentClient = a144AgentClient(t, agent)
	problems := p.webmailAddressProblems(context.Background())
	if len(problems) != 1 || problems[0].Kind != "webmail_cannot_send" || problems[0].Key != "webmail_cannot_send" ||
		problems[0].Actor != "server_owner" || problems[0].Severity != "warning" || problems[0].Name != pending ||
		problems[0].Action.Kind != "open" || problems[0].Action.To != "/components/roundcube" {
		t.Fatalf("problems: %+v", problems)
	}
	if _, ok := problemKindOrder["webmail_cannot_send"]; !ok {
		t.Fatal("the kind has no place in the list's order")
	}
	if !p.webmailCannotSend(context.Background()) {
		t.Fatal("the component check does not read that webmail cannot send")
	}
	for _, name := range []string{pending, "/var/lib/celikpanel-webmail/config/config.inc.php"} {
		en, tr := alertLine(pendingAlertMail{kind: "webmail_cannot_send", name: name})
		if !strings.Contains(en, name) || !strings.Contains(tr, name) || !strings.HasPrefix(en, "Webmail cannot send") || !strings.HasPrefix(tr, "Web postası gönderemiyor") {
			t.Fatalf("alert mail: %q / %q", en, tr)
		}
	}
	agent.sending, agent.file = "", ""
	if problems := p.webmailAddressProblems(context.Background()); len(problems) != 0 {
		t.Fatalf("still listed: %+v", problems)
	}
	if p.webmailCannotSend(context.Background()) {
		t.Fatal("still read as cannot send")
	}
}

// D1: the webmail profile (and its bundle) never says installed and verified
// while the last component check read that it cannot send; the reading is
// kept with the component record.
func TestA145WebmailProfileIsNotCompleteWhileItCannotSend(t *testing.T) {
	profiles := []MailProfileResponse{
		{ID: "core", Status: mailProfileStatusComplete, Verified: true},
		{ID: "webmail", Status: mailProfileStatusComplete, Verified: true},
	}
	same := withWebmailSending(append([]MailProfileResponse(nil), profiles...), false)
	if same[1].Status != mailProfileStatusComplete || !same[1].Verified {
		t.Fatalf("changed without a reason: %+v", same[1])
	}
	got := withWebmailSending(profiles, true)
	if got[1].Status != mailProfileStatusPartial || got[1].Verified || got[1].WarningCode != mailProfileWarningWebmailCannotSend ||
		!strings.HasPrefix(got[1].Warning, "Webmail cannot send") {
		t.Fatalf("webmail: %+v", got[1])
	}
	if got[0].Status != mailProfileStatusComplete || !got[0].Verified {
		t.Fatalf("another profile changed: %+v", got[0])
	}
	listed := false
	for _, code := range mailProfileTextCodes {
		listed = listed || code == mailProfileWarningWebmailCannotSend
	}
	if !listed {
		t.Fatal("the code is not in the list the web test reads")
	}
	ready := true
	data, err := json.Marshal(scanCacheDoc{Observations: []serviceObservation{{ID: "roundcube", IsInstalled: true}}, WebmailReady: &ready, WebmailCannotSend: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := decodeScanCacheSnapshot(string(data))
	if err != nil || !snapshot.WebmailCannotSend || !snapshot.WebmailReady {
		t.Fatalf("kept with the record: %+v %v", snapshot, err)
	}
	old, err := decodeScanCacheSnapshot(`{"observations":[{"id":"roundcube"}],"webmail_ready":true}`)
	if err != nil || old.WebmailCannotSend {
		t.Fatalf("a record from before alpha.145: %+v %v", old, err)
	}
}

// O1: a held change published once the earlier publication completed
// carries the alias records that waited; their wait ends then, not at the
// next hourly round, and the zone no longer says they wait. Another zone's
// wait is not touched.
func TestA145AliasWaitEndsWhenTheHeldPublicationCompletes(t *testing.T) {
	panel, agent, clock, _ := newPendingPublicationPanel(t, "a-alias.example", "b-alias.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })
	ctx := context.Background()
	if err := panel.syncZoneToDNS(ctx, "a-alias.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	// The alias add saved its record in b-alias.example; its publication was held.
	var held *dnsPublicationHeldError
	if err := panel.syncZoneToDNS(ctx, "b-alias.example", false); !errors.As(err, &held) {
		t.Fatalf("held err=%v", err)
	}
	panel.markAliasZoneUnpublished(ctx, "b-alias.example")
	panel.markAliasZoneUnpublished(ctx, "other-alias.example")
	zoneSays := func() bool {
		rec := httptest.NewRecorder()
		panel.handleGetDNSZone(rec, httptest.NewRequest("GET", "/api/v1/domains/2/dns/zone", nil), "b-alias.example")
		var zone DNSZone
		if err := json.Unmarshal(rec.Body.Bytes(), &zone); err != nil || zone.Name != "b-alias.example" {
			t.Fatalf("zone answer %d %s: %v", rec.Code, rec.Body.String(), err)
		}
		return zone.AliasesWaiting
	}
	if !zoneSays() {
		t.Fatal("the zone does not say its alias records wait")
	}

	backupDown = false
	agent.syncResponseHook = nil
	clock.Advance(dnsPendingPublicationMaxDelay)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if state, err := readDNSZoneSyncState(ctx, panel.db.GetDB(), "b-alias.example"); err != nil || state.Status != "applied" {
		t.Fatalf("held change not published: %+v %v", state, err)
	}
	waiting, err := panel.aliasZonesUnpublished(ctx)
	if err != nil || waiting["b-alias.example"] || !waiting["other-alias.example"] {
		t.Fatalf("waits after the held publication: %v %v", waiting, err)
	}
	if zoneSays() {
		t.Fatal("the zone still says its alias records wait")
	}
}

// Review of PR #357, finding 2: CelikPanel's own earlier text whose rewrite
// did not happen is the same line with its own words: the journal and a
// restart, never a hand edit.
func TestA145FailedRewriteIsNamedWithTheRestartNotAHandEdit(t *testing.T) {
	path := "/var/lib/celikpanel-webmail/config/config.inc.php"
	agent := &a145WebmailSendingAgent{sending: "cannot_send", reason: "rewrite_failed", file: path}
	p := newDNSPanelForTest(t)
	p.pkgFamilyVal = "apt"
	p.agentClient = a144AgentClient(t, agent)
	problems := p.webmailAddressProblems(context.Background())
	if len(problems) != 1 || problems[0].Kind != "webmail_cannot_send" || problems[0].Subject != "rewrite_failed" || problems[0].Name != path || problems[0].Actor != "server_owner" {
		t.Fatalf("problems: %+v", problems)
	}
	en, tr := alertLine(pendingAlertMail{kind: "webmail_cannot_send", subject: "rewrite_failed", name: path})
	for _, line := range []string{en, tr} {
		if !strings.Contains(line, "sudo systemctl restart celikpanel-agent") || !strings.Contains(line, "sudo journalctl -u celikpanel-agent") || strings.Contains(line, "smtp_host") {
			t.Fatalf("alert mail: %q", line)
		}
	}
	if !strings.Contains(en, "Do not edit the file by hand") || !strings.Contains(tr, "elle düzenlemeyin") {
		t.Fatalf("alert mail: %q / %q", en, tr)
	}
}

// Review of PR #357, finding 1: the owner without CelikPanel's text beside
// the file is given the two lines exactly as CelikPanel writes them.
func TestA145TheOwnerIsGivenCelikPanelsTwoSendingLines(t *testing.T) {
	source, err := os.ReadFile("../agent/webmail_sending.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{webmailSendingHostLine, webmailSendingOptionsLine} {
		if !strings.Contains(string(source), "\n"+line+"\n") {
			t.Fatalf("not CelikPanel's rendered line: %s", line)
		}
	}
	en, tr := alertLine(pendingAlertMail{kind: "webmail_cannot_send", name: "/var/lib/celikpanel-webmail/config/config.inc.php"})
	for _, line := range []string{en, tr} {
		if !strings.Contains(line, webmailSendingHostLine) || !strings.Contains(line, webmailSendingOptionsLine) {
			t.Fatalf("alert mail without the two lines: %q", line)
		}
	}
}

// Review of PR #357, finding 3: a zone whose alias publication was deferred
// behind its own pending publication (the backup catching up) is settled
// when that publication completes, by the hourly round's own step (a fresh
// publication of the zone's current ledger), not an hour later.
func TestA145AliasWaitIsSettledWhenThePendingPublicationCompletes(t *testing.T) {
	panel, agent, clock, _ := newPendingPublicationPanel(t, "d-alias.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })
	ctx := context.Background()
	if err := panel.syncZoneToDNS(ctx, "d-alias.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("deferred err=%v", err)
	}
	panel.markAliasZoneUnpublished(ctx, "d-alias.example")
	panel.markAliasZoneUnpublished(ctx, "other-alias.example")
	panel.reconcilePendingDNSPublicationsOnce(ctx) // discovers the pending publication, not due yet
	if leaseCount(t, panel) != 1 {
		t.Fatal("the deferred publication holds no lease")
	}

	backupDown = false
	agent.syncResponseHook = nil
	clock.Advance(dnsPendingPublicationMaxDelay)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if state, err := readDNSZoneSyncState(ctx, panel.db.GetDB(), "d-alias.example"); err != nil || state.Status != "applied" {
		t.Fatalf("pending publication not completed: %+v %v", state, err)
	}
	waiting, err := panel.aliasZonesUnpublished(ctx)
	if err != nil || waiting["d-alias.example"] || !waiting["other-alias.example"] {
		t.Fatalf("waits after the pending publication completed: %v %v", waiting, err)
	}
	// Settled by publishing the zone's current ledger again, not by a blind clear.
	syncs, _ := pendingAgentCalls(agent)
	if len(syncs) != 2 || syncs[1].Domain != "d-alias.example" {
		t.Fatalf("publications: %+v", syncs)
	}
}

// alpha.145 (security): webmail's local server in an earlier form is a
// variant of the address lines (no address): kept by the owner, or its
// rewrite put back; the alert mail says it.
func TestA145LocalServerInItsEarlierFormIsAProblemUntilItIsNot(t *testing.T) {
	agent := &a144WebmailFormsAgent{open: []transport.WebmailAddressForm{
		{Host: "", State: "owner_edited", File: "/etc/nginx/conf.d/celikpanel-webmail.conf.celikpanel-pending"},
	}}
	p := newDNSPanelForTest(t)
	p.pkgFamilyVal = "apt"
	p.agentClient = a144AgentClient(t, agent)
	problems := p.webmailAddressProblems(context.Background())
	if len(problems) != 1 || problems[0].Kind != "webmail_address_kept" || problems[0].Subject != "" || problems[0].Actor != "server_owner" ||
		problems[0].Key != "webmail_address_kept:" || !strings.HasSuffix(problems[0].Name, ".celikpanel-pending") {
		t.Fatalf("problems: %+v", problems)
	}
	for _, item := range []pendingAlertMail{
		{kind: "webmail_address_kept", name: "/etc/nginx/conf.d/celikpanel-webmail.conf.celikpanel-pending"},
		{kind: "webmail_address_refused", name: "/etc/nginx/conf.d/broken.conf"},
	} {
		en, tr := alertLine(item)
		if !strings.Contains(en, "local server") || !strings.Contains(tr, "yerel sunucusu") || !strings.Contains(en, item.name) || strings.Contains(en, "address") {
			t.Fatalf("alert mail: %q / %q", en, tr)
		}
	}
	agent.open = nil
	if problems := p.webmailAddressProblems(context.Background()); len(problems) != 0 {
		t.Fatalf("still listed: %+v", problems)
	}
}
