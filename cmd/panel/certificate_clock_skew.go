package main

import (
	"errors"
	"regexp"
	"strconv"
)

// A certificate the setup issued is not valid yet by this server's clock
// (Beta-1 third re-run N2/N4, 2026-10-29). With the clock 200 s behind, the
// panel certificate step said only panel_certificate_issue_failed and the
// mail certificate step the generic server_setup_step_failed. The Agent now
// codes it (transport.CertificateClockSkewErrorCode with the seconds) for
// every certificate the setup issues, and the step says it (D-024: the clock
// is N s behind; who = owner; next = turn on time sync or correct the clock;
// then = review a revised plan, which issues it again).
//
// Sunucu saati geride: sertifika henüz geçerli değil; sahip saati düzeltir.

const certificateClockSkewCode = "server_setup_certificate_clock_skew"

type certificateClockSkewError struct{ Seconds int }

func (e *certificateClockSkewError) Error() string {
	return "the issued certificate is not valid yet by this server's clock"
}

func (e certificateClockSkewError) message() string {
	return "This server's clock is " + strconv.Itoa(e.Seconds) + " seconds behind the certificate authority's, so the certificate it just issued is not valid yet here and was not put to use. Nothing else was changed. You, the owner: turn on time synchronisation (`timedatectl set-ntp true`) or correct the clock; then choose Review a revised plan to issue it again."
}

func certificateClockSkewFrom(err error) (certificateClockSkewError, bool) {
	var skew *certificateClockSkewError
	if errors.As(err, &skew) && skew.Seconds > 0 {
		return *skew, true
	}
	// A terminal certificate child keeps the call's own error as its cause,
	// or, read back after a lost reply, the code its job recorded.
	var child *panelCertificateChildTerminalFailure
	if errors.As(err, &child) {
		if child.cause != nil && errors.As(child.cause, &skew) && skew.Seconds > 0 {
			return *skew, true
		}
		return certificateClockSkewRecorded(child.code, child.message)
	}
	var failure *serverSetupChildFailure
	if errors.As(err, &failure) {
		return certificateClockSkewRecorded(failure.Code, failure.Message)
	}
	return certificateClockSkewError{}, false
}

var certificateClockSkewSeconds = regexp.MustCompile(`^This server's clock is ([0-9]{1,8}) seconds behind`)

// certificateClockSkewRecorded reads the reason back from a job's recorded
// code and the panel-authored sentence (review fix, item 8: a lost reply
// kept only the job). The sentence is made again from the seconds alone.
func certificateClockSkewRecorded(code, message string) (certificateClockSkewError, bool) {
	if code != certificateClockSkewCode {
		return certificateClockSkewError{}, false
	}
	match := certificateClockSkewSeconds.FindStringSubmatch(message)
	if match == nil {
		return certificateClockSkewError{}, false
	}
	seconds, err := strconv.Atoi(match[1])
	if err != nil || seconds <= 0 {
		return certificateClockSkewError{}, false
	}
	return certificateClockSkewError{Seconds: seconds}, true
}

// certificateClockSkewOperationFailure: the code and sentence a certificate
// child's job records when its call failed on the clock, so a later read of
// the job (after a lost reply) still says it.
func certificateClockSkewOperationFailure(err error) (*serviceOperationFailure, bool) {
	skew, ok := certificateClockSkewFrom(err)
	if !ok {
		return nil, false
	}
	return operationFailure(certificateClockSkewCode, skew.message(), err), true
}
