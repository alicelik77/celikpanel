package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114 (the owner, 2026-10-31): each server of a pair hosts its own
// customers' web, databases and mail. A server that joined by code may run
// mail for the domains created on it without claiming the main domain's mail,
// which stays on the first server.
// Koda katilan sunucu kendi musterileri icin posta calistirir; ana alan
// adinin postasi ilk sunucuda kalir.

func joinedMailFirstDraft() serverSetupDraft {
	first := joinFirstDraft()
	first.Purpose, first.MailHostname = "web_mail", "mail.example.test"
	return first
}

// joinedMailPair: the first server receives the main domain's mail and keeps
// its zone; the second joined by code and can issue its mail certificate.
func joinedMailPair(t *testing.T) *peerPair {
	t.Helper()
	pair := newPeerPair(t, joinedMailFirstDraft())
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2}
	pair.second.agent.versionCapabilities = &caps
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	link, err := pair.second.panel.readServerSetupLink(context.Background())
	if err != nil || link == nil || !link.Document.First.MainDomainMail || link.Document.First.MailHostname != "mail.example.test" {
		t.Fatalf("the join names the first server's mail: %+v %v", link, err)
	}
	return pair
}

func ownMail(name string) func(*serverSetupDraft) {
	return func(d *serverSetupDraft) { d.Purpose, d.MailHostname = "web_mail", name }
}

func TestServerSetupJoinedServerRunsMailForItsOwnDomains(t *testing.T) {
	pair := joinedMailPair(t)
	_, plan := pair.secondReview(t, ownMail("mail.beta.example.test"))
	if !plan.CanStart || slices.Contains(plan.Blockers, "server_setup_peer_mail_both") {
		t.Fatalf("own mail on the joined server is refused: %v", plan.Blockers)
	}
	// Mail installed with its own host name, access check and certificate;
	// no DKIM step for the main domain on this server.
	want := map[string]string{"mail_profile": "", "mail_certificate": "mail.beta.example.test"}
	for _, step := range plan.Steps {
		if target, ok := want[step.Kind]; ok && (target == "" || target == step.Target) {
			delete(want, step.Kind)
		}
		if step.Kind == "mail_dns" {
			t.Fatalf("the joined server prepares no DKIM of the main domain: %+v", plan.Steps)
		}
	}
	if len(want) != 0 || plan.Steps[0].Kind != "peer_names" {
		t.Fatalf("missing steps %v in %+v", want, plan.Steps)
	}
	accessMail := false
	for _, step := range plan.Steps {
		accessMail = accessMail || (step.Kind == "access_dns" && step.Target == "mail.beta.example.test")
	}
	if !accessMail {
		t.Fatalf("the mail host name is checked and requested from the main DNS server: %+v", plan.Steps)
	}
	// Its names give the first server its mail host name only, never a
	// claim on the main domain's mail.
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	names, err := pair.second.panel.serverSetupOwnNames(context.Background(), plan.Draft, link, strings.Repeat("4", 32))
	if err != nil || names.MailHostname != "mail.beta.example.test" || names.Mail != nil {
		t.Fatalf("names: %+v %v", names, err)
	}
	// The first server takes them; its zone keeps the main domain's mail on
	// its own mail name and adds the second server's mail name.
	join := serverSetupJoinRecord{RequestID: strings.Repeat("a", 32), Peer: serverSetupJoinPeer{IPv4: "198.51.100.20"}}
	names.Address = "https://198.51.100.20:" + itoaTest(panelPort())
	if code, message := canonicalServerSetupPeerNames(&names, pair.plan.Draft, join); code != "" {
		t.Fatalf("the first server refuses the names: %s %s", code, message)
	}
	bound := serverSetupEffectiveDraft(pair.plan.Draft, serverSetupPeerBindingFromJoin(join, names))
	if bound.PeerMail != nil {
		t.Fatalf("no shared main-domain mail answer from the joined server: %+v", bound.PeerMail)
	}
	records, err := serverSetupInfrastructureDNSDesired(bound)
	if err != nil {
		t.Fatal(err)
	}
	mx, spf, mailA := []string{}, "", ""
	for _, record := range records {
		switch {
		case record.Type == "MX" && record.Name == "example.test":
			mx = append(mx, record.Content)
		case record.Type == "TXT" && record.Name == "example.test":
			spf = record.Content
		case record.Type == "A" && record.Name == "mail.beta.example.test":
			mailA = record.Content
		}
	}
	if len(mx) != 1 || !strings.Contains(mx[0], "mail.example.test") || strings.Contains(mx[0], "beta") || strings.Contains(spf, "beta") {
		t.Fatalf("the main domain's mail stays on the first server: mx=%v spf=%q", mx, spf)
	}
	if mailA != "198.51.100.20" {
		t.Fatalf("the second server's mail name points to it: %q (%+v)", mailA, records)
	}
	// The DNS overview on the joined server names the first server as the
	// main domain's receiver.
	in := dnsOverviewInput{Draft: plan.Draft, Peer: dnsOverviewPeer{FromLink: true, Bound: true, ShortName: "alpha", MailHost: "mail.example.test", MainMail: true}}
	if receiver, _, ok := dnsOverviewMailInputs(in); !ok || receiver.Server != "peer" || receiver.Host != "mail.example.test" {
		t.Fatalf("receiver: %+v", receiver)
	}
}

func TestServerSetupJoinedServerMailNameMustBeItsOwn(t *testing.T) {
	pair := joinedMailPair(t)
	for _, taken := range []string{"mail.example.test", "panel.example.test", "example.test", "ns1.example.test", "ns2.example.test"} {
		_, plan := pair.secondReview(t, ownMail(taken))
		if plan.CanStart || !slices.Contains(plan.Blockers, "server_setup_peer_mail_hostname_taken") {
			t.Fatalf("%s: %v", taken, plan.Blockers)
		}
	}
	// Without mail the name is not looked at.
	if _, plan := pair.secondReview(t, func(d *serverSetupDraft) { d.MailHostname = "mail.example.test" }); slices.Contains(plan.Blockers, "server_setup_peer_mail_hostname_taken") {
		t.Fatalf("no mail, no refusal: %v", plan.Blockers)
	}
}

// The owner asked for mail on the panel's own name (boston.example.com for
// both): the panel listens on its own port with its own certificate lineage,
// webmail is nginx on 443, and the mail certificate is a separate lineage;
// one A record serves both names.
func TestServerSetupJoinedServerMailOnItsPanelName(t *testing.T) {
	pair := joinedMailPair(t)
	_, plan := pair.secondReview(t, ownMail("beta.example.test"))
	if !plan.CanStart {
		t.Fatalf("mail on the panel name: %v", plan.Blockers)
	}
	if !serverSetupPlanHasStep(plan, "mail_certificate") || serverSetupPlanHasStep(plan, "mail_dns") {
		t.Fatalf("steps: %+v", plan.Steps)
	}
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	names, err := pair.second.panel.serverSetupOwnNames(context.Background(), plan.Draft, link, strings.Repeat("4", 32))
	if err != nil || names.MailHostname != "" || names.Mail != nil || names.PanelDomain != "beta.example.test" {
		t.Fatalf("the panel name's record serves the mail name; an earlier first server is not sent it twice: %+v %v", names, err)
	}
	// A first server of this release also accepts the same name twice.
	join := serverSetupJoinRecord{RequestID: strings.Repeat("a", 32), Peer: serverSetupJoinPeer{IPv4: "198.51.100.20"}}
	names.MailHostname, names.Address = names.PanelDomain, "https://198.51.100.20:"+itoaTest(panelPort())
	if code, message := canonicalServerSetupPeerNames(&names, pair.plan.Draft, join); code != "" {
		t.Fatalf("same name twice refused: %s %s", code, message)
	}
	if _, err := serverSetupInfrastructureDNSDesired(serverSetupEffectiveDraft(pair.plan.Draft, serverSetupPeerBindingFromJoin(join, names))); err != nil {
		t.Fatalf("one A record for both names: %v", err)
	}
}

// The owner's Boston: joined with websites and DNS only on alpha.113, set up
// completely, then updated; mail is added later by reopening setup. The
// extension installs mail without sending names again over the ended link
// and without any record of the main domain.
func TestServerSetupJoinedServerAddsMailLater(t *testing.T) {
	pair := joinedMailPair(t)
	state, plan := pair.secondReview(t, nil)
	if !plan.CanStart || serverSetupHasMail(plan.Draft) {
		t.Fatalf("web and DNS: %v", plan.Blockers)
	}
	db := pair.second.database.GetDB()
	// The completed setup: ready, its link ended (it reported to the main).
	if _, err := db.Exec(`UPDATE server_setup_state SET status='ready', completed_at='2026-10-31T09:00:00Z' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := pair.second.panel.revokeServerSetupLinkSide(context.Background(), serverSetupLinkRoleSecond); err != nil {
		t.Fatal(err)
	}
	// Reopen setup (Server -> Server setup) and add mail with a mail profile.
	state, err := pair.second.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	extended, err := serverSetupDraftWithMailProfile(state.Draft, core.MailProfileWebmail)
	if err != nil {
		t.Fatal(err)
	}
	extended.MailHostname = "mail.beta.example.test"
	if _, err := db.Exec(`UPDATE server_setup_state SET status='draft' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	state, _ = pair.second.panel.loadServerSetup(context.Background())
	state, err = pair.second.panel.saveServerSetupDraft(context.Background(), state.Revision, extended)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = pair.second.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: pair.second.userID})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanStart || !serverSetupHasMail(plan.Draft) || serverSetupPlanHasStep(plan, "mail_dns") || !serverSetupPlanHasStep(plan, "mail_certificate") {
		t.Fatalf("adding mail later: %v %+v", plan.Blockers, plan.Steps)
	}
	// The names step of an extension sends nothing: the link ended, and the
	// first server took this server's names when its setup ran.
	execution := pair.startSecond(t, plan)
	before := pair.requests.Load()
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, execution.Steps[0]); !done || err != nil || execution.Steps[0].Kind != "peer_names" {
		t.Fatalf("extension names step: %v %v", done, err)
	}
	if pair.requests.Load() != before {
		t.Fatal("an extension sent something over the ended link")
	}
	// A first setup whose link ended still says what to do (unchanged).
	if _, err := db.Exec(`UPDATE server_setup_state SET completed_at='' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if kind, code := peerOutcomeCode(func() error {
		_, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, execution.Steps[0])
		return err
	}()); kind != "failed" || code != "server_setup_peer_link_revoked" {
		t.Fatalf("first setup over an ended link: %s %s", kind, code)
	}
}

// Option A: the joined server is the main DNS server and the first server
// receives the main domain's mail. The main may run its own mail; the main
// domain's records keep naming the first server's mail.
func TestServerSetupJoinedMainDNSServerRunsItsOwnMail(t *testing.T) {
	first := joinedMailFirstDraft()
	first.MainDNSServer = "peer"
	pair := newPeerPair(t, first)
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2}
	pair.second.agent.versionCapabilities = &caps
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" || answer.Setup.Draft.PeerMail == nil {
		t.Fatalf("join: %+v", answer)
	}
	_, plan := pair.secondReview(t, ownMail("mail2.example.test"))
	if !plan.CanStart || serverSetupPlanHasStep(plan, "mail_dns") {
		t.Fatalf("own mail on the main DNS server: %v %+v", plan.Blockers, plan.Steps)
	}
	if serverSetupMainDomainMailHere(plan.Draft) || serverSetupNeedsMailDNSStep(plan.Draft) {
		t.Fatal("the main domain's mail is not received here")
	}
	if plan.InfrastructureDNS == nil {
		t.Fatal("the main DNS server prepares its zone")
	}
	mx, mail2 := []string{}, false
	for _, record := range plan.InfrastructureDNS.Records {
		if record.Type == "MX" && record.Name == "example.test" {
			mx = append(mx, record.Content)
		}
		if record.Type == "TXT" && record.Name == "example.test" && strings.Contains(record.Content, "mail2") {
			t.Fatalf("the main domain's SPF names only the first server's mail: %q", record.Content)
		}
		mail2 = mail2 || (record.Type == "A" && record.Name == "mail2.example.test")
	}
	if len(mx) != 1 || !strings.Contains(mx[0], "mail.example.test") || strings.Contains(mx[0], "mail2") || !mail2 {
		t.Fatalf("records: mx=%v mail2=%v %+v", mx, mail2, plan.InfrastructureDNS.Records)
	}
}

// The first server's own behaviour is unchanged: with the main domain's mail
// it still writes MX, SPF and DMARC for its own mail name and prepares DKIM.
func TestServerSetupFirstServerMainDomainMailUnchanged(t *testing.T) {
	d, err := canonicalServerSetupDraft(joinedMailFirstDraft())
	if err != nil {
		t.Fatal(err)
	}
	host, _, ok := serverSetupMainDomainMail(d)
	if !ok || host != "mail.example.test" || !serverSetupMainDomainMailHere(d) || !serverSetupNeedsMailDNSStep(d) {
		t.Fatalf("first server: %q %v", host, ok)
	}
	if code := serverSetupJoinedMailHostnameBlocker(d, serverSetupJoinDocument{MainDomain: "example.test"}); code != "" {
		t.Fatalf("own names: %s", code)
	}
}
