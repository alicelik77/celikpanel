package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// serverSetupMailAfterDNSIdentity fails when a mail profile or a service that
// needs a mail server would run before this server's DNS identity is ready:
// after every DNS step and wait (dns, the readiness or publisher wait, the
// join or names step). Other components may run before the waits.
func serverSetupMailAfterDNSIdentity(t *testing.T, layout string, steps []serverSetupPlanStep) {
	t.Helper()
	identity, mail := -1, 0
	for index, step := range steps {
		if slices.Contains([]string{"dns", "dns_readiness", "dns_publisher", "peer_join", "peer_names"}, step.Kind) {
			identity = index
		}
	}
	kinds := []string{}
	for index, step := range steps {
		kinds = append(kinds, step.Kind+":"+step.Target)
		if step.Kind == "mail_profile" || (step.Kind == "service" && serverSetupMailDependent(step.Target)) {
			mail++
			if index < identity {
				t.Errorf("%s: %s runs before the DNS identity is ready (step %d of %d): %v", layout, step.Kind, index, identity, kinds)
			}
		}
	}
	if mail == 0 {
		t.Fatalf("%s: the plan has no mail step: %v", layout, kinds)
	}
}

// Beta-1 VM cells a and b: in the join flow mail_profile ran before
// peer_join and the DNS steps and failed with
// mail_profile_dns_identity_not_ready. On both servers and in every layout
// the mail profile and everything that needs it follow the DNS identity.
func TestServerSetupMailStepsFollowTheDNSIdentityInEveryLayout(t *testing.T) {
	withMail := func(d serverSetupDraft) serverSetupDraft {
		d.Purpose, d.MailHostname = "web_mail", "mail."+d.MainDomain
		return d
	}
	t.Run("first server is the main", func(t *testing.T) {
		_, plan, _ := joinFirstFixture(t, withMail(joinFirstDraft()))
		if !serverSetupPlanHasStep(plan, "peer_join") {
			t.Fatalf("not a join plan: %+v", plan.Steps)
		}
		serverSetupMailAfterDNSIdentity(t, "first main", plan.Steps)
	})
	t.Run("option A: first server is the backup", func(t *testing.T) {
		first := withMail(joinFirstDraft())
		first.MainDNSServer = "peer"
		_, plan, _ := joinFirstFixture(t, first)
		if !serverSetupPlanHasStep(plan, "peer_join") || !serverSetupPlanHasStep(plan, "dns_publisher") {
			t.Fatalf("not an Option A join plan: %+v", plan.Steps)
		}
		serverSetupMailAfterDNSIdentity(t, "option A first", plan.Steps)
	})
	t.Run("second server", func(t *testing.T) {
		pair := newPeerPair(t, joinFirstDraft())
		if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
			t.Fatalf("join: %+v", answer)
		}
		_, plan := pair.secondReview(t, func(d *serverSetupDraft) {
			d.Purpose, d.MailHostname = "web_mail", "mail.beta.example.test"
		})
		if !serverSetupPlanHasStep(plan, "peer_names") {
			t.Fatalf("not a second-server plan: %+v", plan.Steps)
		}
		serverSetupMailAfterDNSIdentity(t, "second", plan.Steps)
	})
	t.Run("external DNS", func(t *testing.T) {
		f, state := setupOperationFixture(t)
		caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2}
		f.agent.versionCapabilities = &caps
		draft := state.Draft
		draft.Purpose, draft.MainDomain, draft.MailHostname = "web_mail", "example.test", "mail.example.test"
		saved, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.panel.buildServerSetupPlan(context.Background(), saved, serviceOperationActor{UserID: f.userID})
		if err != nil {
			t.Fatal(err)
		}
		serverSetupMailAfterDNSIdentity(t, "external", plan.Steps)
	})
	t.Run("one server with its own DNS", func(t *testing.T) {
		f, state := setupOperationFixture(t)
		draft := withMail(standaloneSetupDraftForTest())
		t.Setenv("CELIKPANEL_SERVER_IP", draft.LocalIP)
		saved, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.panel.buildServerSetupPlan(context.Background(), saved, serviceOperationActor{UserID: f.userID})
		if err != nil {
			t.Fatal(err)
		}
		serverSetupMailAfterDNSIdentity(t, "standalone", plan.Steps)
	})
	// A service that needs a mail server (SpamAssassin) stays with the mail
	// profile; components that need no DNS still run before the join.
	steps := []serverSetupPlanStep{{Kind: "service", Target: "nginx"}, {Kind: "service", Target: "mariadb"}, {Kind: "dns", Target: "local"}, {Kind: "dns_readiness", Target: "local"},
		{Kind: "runtime", Target: "node"}, {Kind: "mail_profile", Target: "full"}, {Kind: "service", Target: "spamassassin"}, {Kind: "verify", Target: "web_mail"}}
	ordered := serverSetupPeerJoinSteps(steps, serverSetupPlanStep{Kind: "peer_join", Target: "second"})
	serverSetupMailAfterDNSIdentity(t, "synthetic", ordered)
	got := []string{}
	for _, step := range ordered {
		got = append(got, step.Kind+":"+step.Target)
	}
	if want := "service:nginx service:mariadb runtime:node peer_join:second dns:local dns_readiness:local mail_profile:full service:spamassassin verify:web_mail"; strings.Join(got, " ") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
}
