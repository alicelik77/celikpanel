package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// serverSetupMailAfterCertificate fails unless the plan obtains the mail host
// certificate before it installs mail (owner, 2 Nov 2026): the mail host
// name's address check, then its certificate, then the mail profile and the
// services that need it, then the main domain's DKIM step and the
// independent renewal.
func serverSetupMailAfterCertificate(t *testing.T, layout, mailHostname string, steps []serverSetupPlanStep) {
	t.Helper()
	kinds := []string{}
	at := map[string][]int{}
	for index, step := range steps {
		kinds = append(kinds, step.Kind+":"+step.Target)
		key := step.Kind
		if step.Kind == "service" && serverSetupMailDependent(step.Target) {
			key = "mail_service"
		}
		if step.Kind == "access_dns" && step.Target == mailHostname {
			key = "mail_access_dns"
		}
		at[key] = append(at[key], index)
	}
	t.Logf("%s: %v", layout, kinds)
	certificate := at["mail_certificate"]
	if len(certificate) != 1 || len(at["mail_profile"]) == 0 || len(at["mail_access_dns"]) != 1 {
		t.Fatalf("%s: plan lacks the mail certificate, profile or address step: %v", layout, kinds)
	}
	if at["mail_access_dns"][0] > certificate[0] {
		t.Errorf("%s: certificate before its address check: %v", layout, kinds)
	}
	for _, key := range []string{"firewall", "panel_certificate", "infrastructure_dns", "dns_readiness", "dns_publisher", "peer_join", "peer_names"} {
		for _, index := range at[key] {
			if index > certificate[0] {
				t.Errorf("%s: %s runs after the mail certificate: %v", layout, key, kinds)
			}
		}
	}
	lastMail := certificate[0]
	for _, key := range []string{"mail_profile", "mail_service"} {
		for _, index := range at[key] {
			if index < certificate[0] {
				t.Errorf("%s: %s runs before the mail certificate: %v", layout, key, kinds)
			}
			lastMail = max(lastMail, index)
		}
	}
	for _, key := range []string{"mail_dns", "mail_enrollment"} {
		for _, index := range at[key] {
			if index < lastMail {
				t.Errorf("%s: %s runs before the mail profile: %v", layout, key, kinds)
			}
		}
	}
	if !serverSetupMailCertificateFirst(steps) {
		t.Errorf("%s: not recognised as certificate first: %v", layout, kinds)
	}
}

func TestServerSetupMailCertificateComesBeforeMailInEveryLayout(t *testing.T) {
	withMail := func(d serverSetupDraft) serverSetupDraft {
		d.Purpose, d.MailHostname = "web_mail", "mail."+d.MainDomain
		return d
	}
	t.Run("first server is the main", func(t *testing.T) {
		draft := withMail(joinFirstDraft())
		_, plan, _ := joinFirstFixture(t, draft)
		serverSetupMailAfterCertificate(t, "first main", draft.MailHostname, plan.Steps)
	})
	t.Run("option A: first server is the backup", func(t *testing.T) {
		draft := withMail(joinFirstDraft())
		draft.MainDNSServer = "peer"
		_, plan, _ := joinFirstFixture(t, draft)
		serverSetupMailAfterCertificate(t, "option A first", draft.MailHostname, plan.Steps)
	})
	t.Run("second server", func(t *testing.T) {
		pair := newPeerPair(t, joinFirstDraft())
		if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
			t.Fatalf("join: %+v", answer)
		}
		_, plan := pair.secondReview(t, func(d *serverSetupDraft) {
			d.Purpose, d.MailHostname = "web_mail", "mail.beta.example.test"
		})
		serverSetupMailAfterCertificate(t, "second", "mail.beta.example.test", plan.Steps)
	})
	t.Run("external DNS", func(t *testing.T) {
		f, state := setupOperationFixture(t)
		caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailHostCertificateStageV1, transport.AgentCapabilityMailTLSSyncV2}
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
		serverSetupMailAfterCertificate(t, "external", draft.MailHostname, plan.Steps)
		if !plan.CanStart {
			t.Fatalf("blocked: %v", plan.Blockers)
		}
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
		serverSetupMailAfterCertificate(t, "standalone", draft.MailHostname, plan.Steps)
	})
	t.Run("mail-only extension", func(t *testing.T) {
		f, _ := setupOperationFixture(t)
		ready := markServerSetupReadyForTest(t, f)
		w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "scope": "mail", "mail_profile": "core-mail", "mail_hostname": "mail.example.test"})
		if w.Code != http.StatusOK {
			t.Fatalf("begin: %d %s", w.Code, w.Body.String())
		}
		state, err := f.panel.loadServerSetup(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Scope != serverSetupExtensionScopeMail {
			t.Fatalf("not a mail-only plan: %v", setupStepKinds(plan))
		}
		serverSetupMailAfterCertificate(t, "mail-only extension", "mail.example.test", plan.Steps)
	})
	t.Run("joined mail-only extension", func(t *testing.T) {
		pair := newJoinedPairWithoutMail(t)
		plan := beginJoinedMailScope(t, pair, joinedReadyForMailScope(t, pair))
		serverSetupMailAfterCertificate(t, "joined mail-only extension", "mail.beta.example.test", plan.Steps)
	})
}

// A server that already has Postfix or Dovecot keeps the earlier order: its
// accepted mail configuration is updated by the mail profile first, and the
// certificate is then applied to the running services. Such a plan does not
// need the Agent's staging capability.
func TestServerSetupMailCertificateKeepsProfileFirstWhenMailIsInstalled(t *testing.T) {
	for _, service := range []string{"postfix", "dovecot"} {
		t.Run(service, func(t *testing.T) {
			f, state := setupOperationFixture(t)
			caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2}
			f.agent.versionCapabilities = &caps
			f.agent.installed[service] = true
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
			if serverSetupMailCertificateFirst(plan.Steps) {
				t.Fatalf("certificate first with %s installed: %v", service, setupStepKinds(plan))
			}
			profile, certificate := -1, -1
			for index, step := range plan.Steps {
				switch step.Kind {
				case "mail_profile":
					profile = index
				case "mail_certificate":
					certificate = index
				}
			}
			if profile < 0 || certificate < profile {
				t.Fatalf("order: %v", setupStepKinds(plan))
			}
			if slices.Contains(plan.Blockers, "server_setup_mail_certificate_unavailable") {
				t.Fatalf("the earlier order needs no staging capability: %v", plan.Blockers)
			}
		})
	}
}

// An Agent without the staging capability (a Panel and Agent from different
// releases) cannot issue before mail exists: the review is blocked with the
// existing guidance instead of failing mid-run.
func TestServerSetupMailCertificateFirstRequiresStagingAgent(t *testing.T) {
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
	if !serverSetupMailCertificateFirst(plan.Steps) || plan.CanStart || !slices.Contains(plan.Blockers, "server_setup_mail_certificate_unavailable") {
		t.Fatalf("missing staging capability: can_start=%v blockers=%v steps=%v", plan.CanStart, plan.Blockers, setupStepKinds(plan))
	}
}

// Plans persisted by an earlier release keep their own order and identity:
// they are read back and validated as stored, never reordered.
func TestServerSetupEarlierProfileFirstPlanIsNotReordered(t *testing.T) {
	f, state := setupOperationFixture(t)
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailHostCertificateStageV1, transport.AgentCapabilityMailTLSSyncV2}
	f.agent.versionCapabilities = &caps
	draft := state.Draft
	draft.Purpose, draft.MainDomain, draft.MailHostname = "web_mail", "example.test", "mail.example.test"
	saved, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	plan := saveSetupPlanForTest(t, f, saved)
	// The same plan in the earlier order, as a release before 2 Nov 2026
	// stored it.
	earlier := plan
	earlier.Steps = slices.Clone(plan.Steps)
	certificate := slices.IndexFunc(earlier.Steps, func(step serverSetupPlanStep) bool { return step.Kind == "mail_certificate" })
	profile := slices.IndexFunc(earlier.Steps, func(step serverSetupPlanStep) bool { return step.Kind == "mail_profile" })
	if certificate < 0 || profile < certificate {
		t.Fatalf("new plan order: %v", setupStepKinds(plan))
	}
	moved := earlier.Steps[profile]
	earlier.Steps = slices.Delete(earlier.Steps, profile, profile+1)
	earlier.Steps = slices.Insert(earlier.Steps, certificate, moved)
	for index := range earlier.Steps {
		earlier.Steps[index].ID = fmt.Sprintf("%02d-%s", index+1, earlier.Steps[index].Kind)
	}
	earlier.ID = serverSetupPlanIdentity(earlier)
	if earlier.ID == plan.ID || serverSetupMailCertificateFirst(earlier.Steps) {
		t.Fatalf("earlier plan not distinct: %v", earlier.Steps)
	}
	raw, _ := json.Marshal(earlier)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, earlier.ID, earlier.Revision, string(raw), f.userID, "2026-11-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	loaded, err := f.panel.loadServerSetupPlan(context.Background(), earlier.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded.Steps, earlier.Steps) || serverSetupMailCertificateFirst(loaded.Steps) {
		t.Fatalf("stored order changed: %v", loaded.Steps)
	}
}

func TestServerSetupMailCertificateFirstOrderHelper(t *testing.T) {
	step := func(kind string) serverSetupPlanStep { return serverSetupPlanStep{Kind: kind} }
	for name, tc := range map[string]struct {
		steps []serverSetupPlanStep
		want  bool
	}{
		"certificate first": {[]serverSetupPlanStep{step("access_dns"), step("mail_certificate"), step("mail_profile"), step("mail_dns")}, true},
		"profile first":     {[]serverSetupPlanStep{step("mail_profile"), step("access_dns"), step("mail_certificate")}, false},
		"no profile":        {[]serverSetupPlanStep{step("mail_certificate"), step("verify")}, false},
		"no certificate":    {[]serverSetupPlanStep{step("mail_profile"), step("verify")}, false},
	} {
		if got := serverSetupMailCertificateFirst(tc.steps); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

// A staged certificate (published while no mail service was installed) is
// the reviewed outcome only for a plan that installs mail after it.
func TestServerSetupMailCertificateStagedOutcome(t *testing.T) {
	staged := transport.IssueMailHostCertificateResponse{Issued: true, Staged: true}
	applied := transport.IssueMailHostCertificateResponse{Issued: true}
	if err := serverSetupMailCertificateStagedError(staged, true); err != nil {
		t.Fatalf("certificate-first plan refused a staged certificate: %v", err)
	}
	if err := serverSetupMailCertificateStagedError(applied, true); err != nil {
		t.Fatalf("certificate-first plan refused an applied certificate: %v", err)
	}
	if err := serverSetupMailCertificateStagedError(applied, false); err != nil {
		t.Fatalf("profile-first plan refused an applied certificate: %v", err)
	}
	if err := serverSetupMailCertificateStagedError(staged, false); err == nil {
		t.Fatal("profile-first plan accepted a certificate no mail service uses")
	}
}

// The joined-plan order keeps the mail steps after the certificate when the
// plan puts the certificate first, and after the DNS waits otherwise.
func TestServerSetupPeerJoinStepsKeepCertificateFirst(t *testing.T) {
	steps := []serverSetupPlanStep{{Kind: "service", Target: "nginx"}, {Kind: "dns", Target: "local"}, {Kind: "dns_readiness", Target: "local"},
		{Kind: "access_dns", Target: "mail.example.test"}, {Kind: "mail_certificate", Target: "mail.example.test"},
		{Kind: "mail_profile", Target: "full"}, {Kind: "service", Target: "spamassassin"}, {Kind: "mail_dns", Target: "example.test"},
		{Kind: "verify", Target: "web_mail"}}
	ordered := serverSetupPeerJoinSteps(steps, serverSetupPlanStep{Kind: "peer_join", Target: "second"})
	got := []string{}
	for _, step := range ordered {
		got = append(got, step.Kind+":"+step.Target)
	}
	want := "service:nginx peer_join:second dns:local dns_readiness:local access_dns:mail.example.test mail_certificate:mail.example.test mail_profile:full service:spamassassin mail_dns:example.test verify:web_mail"
	if strings.Join(got, " ") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
}
