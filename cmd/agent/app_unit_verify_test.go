package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
)

// systemd-analyze verify that timed out, could not be launched, was killed or
// failed without a word is a check that could not be run (the application is
// neither restarted nor blamed on the unit); only a non-zero exit that printed
// something is the unit being refused (alpha.83, twelfth-record correction).
func TestAppUnitVerifyOutcomeSortsRefusalFromAnUnknownResult(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to produce a real exit status")
	}
	exitWith := func(script string) (error, []byte) {
		out, runErr := exec.Command(shell, "-c", script).CombinedOutput()
		return runErr, out
	}

	if got := appUnitVerifyOutcome(nil, nil, []byte("anything")); got != nil {
		t.Fatalf("a passing check: %v", got)
	}

	// Refused: a non-zero exit with the check's words.
	runErr, out := exitWith("echo \"Unknown key name 'Bogus' in section 'Service'\"; exit 1")
	got := appUnitVerifyOutcome(nil, runErr, out)
	if got == nil || errors.Is(got, services.ErrServiceCheckUnavailable) || !strings.Contains(got.Error(), "Unknown key name") {
		t.Fatalf("a refusal was sorted as %v", got)
	}

	// Unknown: a non-zero exit that said nothing.
	runErr, out = exitWith("exit 1")
	if got := appUnitVerifyOutcome(nil, runErr, out); !errors.Is(got, services.ErrServiceCheckUnavailable) {
		t.Fatalf("a silent failure was sorted as %v", got)
	}

	// Unknown: a timeout, even with words printed so far.
	if got := appUnitVerifyOutcome(context.DeadlineExceeded, errors.New("signal: killed"), []byte("partial")); !errors.Is(got, services.ErrServiceCheckUnavailable) {
		t.Fatalf("a timeout was sorted as %v", got)
	}

	// Unknown: it could not be launched at all.
	_, launchErr := exec.Command("/nonexistent/systemd-analyze").CombinedOutput()
	if got := appUnitVerifyOutcome(nil, launchErr, nil); !errors.Is(got, services.ErrServiceCheckUnavailable) {
		t.Fatalf("a launch failure was sorted as %v", got)
	}
}
