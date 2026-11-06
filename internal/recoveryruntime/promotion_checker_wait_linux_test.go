//go:build linux

package recoveryruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// a125 lab: alpha.124 -> alpha.125 failed at the recovery runtime promotion
// while Debian's midnight apt-listchanges ran; the promotion's Agent checker
// refused with "the host package manager is active" and was read only once more.
func stubPromotionSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	previous := promotionSleep
	promotionSleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { promotionSleep = previous })
	return &slept
}

func scriptedChecker(answers ...string) (func(context.Context) ([]byte, error), *int) {
	calls := 0
	return func(context.Context) ([]byte, error) {
		answer := answers[len(answers)-1]
		if calls < len(answers) {
			answer = answers[calls]
		}
		calls++
		if answer == "" {
			return nil, nil
		}
		return []byte(answer), errors.New("exit status 1")
	}, &calls
}

const packageManagerRefusal = "Recovery agent check: service mutation state is not idle: the host package manager is active\n"

func TestPromotionCheckerWaitsForAFinishingPackageTask(t *testing.T) {
	slept := stubPromotionSleep(t)
	run, calls := scriptedChecker(packageManagerRefusal, packageManagerRefusal, packageManagerRefusal, "")
	if err := runPromotionChecker(func() error { return nil }, run); err != nil {
		t.Fatalf("a package task that ended within the bound must not fail the promotion: %v", err)
	}
	if *calls != 4 || len(*slept) != 3 || (*slept)[0] != promotionCheckerTransientPoll {
		t.Fatalf("calls=%d slept=%v", *calls, *slept)
	}
}

func TestPromotionCheckerWaitsForARunningAgentOperation(t *testing.T) {
	stubPromotionSleep(t)
	run, calls := scriptedChecker("Recovery agent check: service mutation state is not idle: privileged mutation 3f2a is active\n", "")
	if err := runPromotionChecker(func() error { return nil }, run); err != nil || *calls != 2 {
		t.Fatalf("err=%v calls=%d", err, *calls)
	}
}

func TestPromotionCheckerStopsWaitingAtTheBound(t *testing.T) {
	slept := stubPromotionSleep(t)
	run, calls := scriptedChecker(packageManagerRefusal)
	err := runPromotionChecker(func() error { return nil }, run)
	if err == nil {
		t.Fatalf("a package task still running after the bound must refuse")
	}
	var waited time.Duration
	for _, d := range *slept {
		waited += d
	}
	// The bound of transient waits, then the one ordinary re-read.
	if waited != promotionCheckerTransientWait+promotionCheckerRetryDelay {
		t.Fatalf("waited %v, calls %d", waited, *calls)
	}
}

func TestPromotionCheckerOtherRefusalIsReadOnceMore(t *testing.T) {
	slept := stubPromotionSleep(t)
	run, calls := scriptedChecker("Recovery agent check: service mutation ledger is not canonical\n")
	if err := runPromotionChecker(func() error { return nil }, run); err == nil {
		t.Fatal("a non-transient refusal must refuse")
	}
	if *calls != 2 || len(*slept) != 1 || (*slept)[0] != promotionCheckerRetryDelay {
		t.Fatalf("calls=%d slept=%v", *calls, *slept)
	}
}

func TestPromotionCheckerReprovesTheBoundaryAroundEveryRead(t *testing.T) {
	stubPromotionSleep(t)
	run, _ := scriptedChecker(packageManagerRefusal, "")
	boundary := 0
	changed := errors.New("boundary changed")
	err := runPromotionChecker(func() error {
		boundary++
		if boundary == 3 {
			return changed
		}
		return nil
	}, run)
	if !errors.Is(err, changed) {
		t.Fatalf("a boundary that changed while waiting must stop the promotion, got %v", err)
	}
}

func TestPromotionCheckerOutputIsBounded(t *testing.T) {
	output := &promotionCheckerOutput{limit: 8}
	for i := 0; i < 3; i++ {
		if n, err := output.Write([]byte("abcdef")); n != 6 || err != nil {
			t.Fatalf("write %d %v", n, err)
		}
	}
	if string(output.data) != "abcdefab" {
		t.Fatalf("kept %q", output.data)
	}
}
