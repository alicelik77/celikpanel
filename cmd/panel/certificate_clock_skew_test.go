package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 third re-run N2/N4: a clock behind the certificate authority is one
// coded reason for every certificate the setup issues, with the seconds, the
// owner's command and how to go on; another failure keeps its own answer.
func TestCertificateClockSkewReachesTheStepForEveryCertificate(t *testing.T) {
	cause := &panelCertificateChildTerminalFailure{code: "service_operation_failed", message: "failed", cause: &certificateClockSkewError{Seconds: 200}}
	code, message := panelCertificateSagaFailure(cause)
	if code != certificateClockSkewCode || !strings.Contains(message, "200 seconds behind") || !strings.Contains(message, "timedatectl set-ntp true") || !strings.Contains(message, "Review a revised plan") {
		t.Fatalf("panel certificate: %s %q", code, message)
	}
	if code, _ := panelCertificateSagaFailure(&panelCertificateChildTerminalFailure{cause: errors.New("certbot failed")}); code != "panel_certificate_issue_failed" {
		t.Fatalf("another panel certificate failure: %s", code)
	}
	err := serverSetupMailCertificateResponseError(transport.IssueMailHostCertificateResponse{Error: "not valid yet", ErrorCode: transport.CertificateClockSkewErrorCode, ClockSkewSeconds: 4})
	var failure *serverSetupChildFailure
	if !errors.As(err, &failure) || failure.Code != certificateClockSkewCode || !strings.Contains(failure.Message, "4 seconds behind") {
		t.Fatalf("mail certificate: %v", err)
	}
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{Kind: "mail_certificate"}}
	if got := serverSetupFailureForStep(step, err); got.Code != certificateClockSkewCode {
		t.Fatalf("the step's code: %+v", got)
	}
	if err := serverSetupMailCertificateResponseError(transport.IssueMailHostCertificateResponse{Issued: true}); err != nil {
		t.Fatalf("an issued certificate: %v", err)
	}
}

// Review fix (item 8): after a lost reply only the child's job is read
// back. The job records the clock code (the finish of a failed call keeps
// it), and both the panel certificate saga and the mail certificate step
// map it again from the job.
func TestCertificateClockSkewSurvivesALostReply(t *testing.T) {
	failure := standaloneAgentMutationFailure(&certificateClockSkewError{Seconds: 200}, nil)
	if failure == nil || failure.Code != certificateClockSkewCode || !strings.Contains(failure.Message, "200 seconds behind") {
		t.Fatalf("the job's recorded failure: %+v", failure)
	}
	// The saga reads the job back: only its code and sentence remain.
	read := &panelCertificateChildTerminalFailure{code: failure.Code, message: failure.Message, cause: errors.New("persisted privileged child failed")}
	if code, message := panelCertificateSagaFailure(read); code != certificateClockSkewCode || !strings.Contains(message, "200 seconds behind") {
		t.Fatalf("panel certificate after a lost reply: %s %q", code, message)
	}
	// A recorded sentence that is not the panel's own is not taken.
	if _, ok := certificateClockSkewRecorded(certificateClockSkewCode, "something else"); ok {
		t.Fatal("a foreign sentence was taken")
	}
	// The mail certificate step reads the same job back.
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	plan.ContactEmail = "admin@example.test"
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-mail_certificate", Kind: "mail_certificate", Target: "mail.example.test"}, RequestID: strings.Repeat("3", 32), OwnerID: strings.Repeat("4", 32)}
	commitment, err := mutationpayload.CanonicalMailHostCertificate(step.Target, plan.ContactEmail, plan.BuildCommit)
	if err != nil {
		t.Fatal(err)
	}
	f.agent.mu.Lock()
	f.agent.mutationJobs[step.RequestID] = &ServiceOperationMutationJob{RequestID: step.RequestID, OwnerID: step.OwnerID, Kind: "mail_host_certificate", Target: step.Target, PackageName: commitment.Qualifier,
		Status: agentMutationFailed, ErrorCode: failure.Code, ErrorMessage: failure.Message}
	f.agent.mu.Unlock()
	_, err = f.panel.runServerSetupMailCertificate(context.Background(), plan, step)
	var child *serverSetupChildFailure
	if !errors.As(err, &child) || child.Code != certificateClockSkewCode {
		t.Fatalf("mail certificate after a lost reply: %v", err)
	}
}
