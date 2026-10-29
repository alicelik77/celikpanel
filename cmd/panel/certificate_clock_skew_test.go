package main

import (
	"errors"
	"strings"
	"testing"

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
