package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The step records where the certificate went (Staged answer), and says
// unknown when completion was proven from the job without a reply.
func TestServerSetupMailCertificatePlacement(t *testing.T) {
	for name, tc := range map[string]struct {
		response transport.IssueMailHostCertificateResponse
		want     string
	}{
		"staged":  {transport.IssueMailHostCertificateResponse{Issued: true, Staged: true}, serverSetupMailCertificateStaged},
		"applied": {transport.IssueMailHostCertificateResponse{Issued: true}, serverSetupMailCertificateApplied},
	} {
		if got := serverSetupMailCertificatePlacement(tc.response); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
	raw, err := json.Marshal(serverSetupExecutionStep{MailCertificate: serverSetupMailCertificateStaged})
	if err != nil || !strings.Contains(string(raw), `"mail_certificate":"staged"`) {
		t.Fatalf("step json: %s %v", raw, err)
	}
	raw, _ = json.Marshal(serverSetupExecutionStep{})
	if strings.Contains(string(raw), "mail_certificate") {
		t.Fatalf("empty placement is written: %s", raw)
	}
}

// A succeeded job with a lost reply completes the step with placement unknown.
func TestServerSetupMailCertificateRecoveredPlacementIsUnknown(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	plan.ContactEmail = "admin@example.test"
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-mail_certificate", Kind: "mail_certificate", Target: "mail.example.test"}, RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	commitment, err := mutationpayload.CanonicalMailHostCertificate(step.Target, plan.ContactEmail, plan.BuildCommit)
	if err != nil {
		t.Fatal(err)
	}
	identity := agentMutationIdentityForOperation(serviceOperation{RequestID: step.RequestID, Kind: "mail_host_certificate", ServiceID: step.Target, PackageName: commitment.Qualifier}, step.OwnerID)
	phase, _, err := payloadBoundMutationPublishedPhase(identity)
	if err != nil {
		t.Fatal(err)
	}
	f.agent.mu.Lock()
	f.agent.mutationJobs[step.RequestID] = &ServiceOperationMutationJob{RequestID: step.RequestID, OwnerID: step.OwnerID, Kind: "mail_host_certificate", Target: step.Target, PackageName: commitment.Qualifier,
		Status: agentMutationSucceeded, Phase: phase}
	f.agent.mu.Unlock()
	done, err := f.panel.runServerSetupMailCertificate(context.Background(), plan, &step)
	if err != nil || !done || step.MailCertificate != serverSetupMailCertificateUnknown {
		t.Fatalf("done=%v err=%v placement=%q", done, err, step.MailCertificate)
	}
	// A placement recorded by an earlier pass is kept.
	step.MailCertificate = serverSetupMailCertificateStaged
	if done, err := f.panel.runServerSetupMailCertificate(context.Background(), plan, &step); err != nil || !done || step.MailCertificate != serverSetupMailCertificateStaged {
		t.Fatalf("kept: done=%v err=%v placement=%q", done, err, step.MailCertificate)
	}
}

func TestMailProfileTLSWarningFollowsTheHostCertificate(t *testing.T) {
	for value, want := range map[string]string{
		transport.MailHostCertificateManaged: mailProfileHostCertificateWarning,
		transport.MailHostCertificateDefault: mailProfileFallbackWarning,
		"":                                   mailProfileFallbackWarning,
	} {
		if got := mailProfileTLSWarning(mailProfileHostCertificateState(value)); got != want {
			t.Errorf("%q: %q", value, got)
		}
	}
	if mailProfileHostCertificateState("anything") != "" {
		t.Fatal("an unknown Agent answer was kept")
	}
}
