package checkresult

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIfContextEndedMarksOnlyEndedChecks(t *testing.T) {
	plain := errors.New("exit status 1")
	if got := IfContextEnded(context.Background(), plain); Is(got) || got != plain {
		t.Fatalf("a finished check's failure must stay a failure: %v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if got := IfContextEnded(ctx, plain); !Is(got) || !errors.Is(got, plain) {
		t.Fatalf("a check whose deadline passed is unknown and keeps its cause: %v", got)
	}
	if got := IfContextEnded(context.Background(), context.DeadlineExceeded); !Is(got) {
		t.Fatalf("a deadline error is unknown: %v", got)
	}
	if IfContextEnded(ctx, nil) != nil || Incomplete(nil) != nil {
		t.Fatal("nil stays nil")
	}
	twice := Incomplete(Incomplete(plain))
	if twice.Error() != Incomplete(plain).Error() {
		t.Fatalf("marking twice must not stack: %v", twice)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestNetworkTimeoutsAreUnknown(t *testing.T) {
	if !Is(IfContextEnded(context.Background(), timeoutErr{})) {
		t.Fatal("a network read that timed out is unknown")
	}
}

// alpha.129: a check that could not finish is asked again with pauses; a
// finished answer (success or a verified failure) ends the retry at once.
func TestRetryAsksAgainOnlyWhileTheCheckCannotFinish(t *testing.T) {
	waits := []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	calls := 0
	err := Retry(context.Background(), waits, func() error {
		calls++
		if calls < 3 {
			return Incomplete(errors.New("dpkg-query: deadline"))
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err %v calls %d", err, calls)
	}
	calls = 0
	refused := errors.New("not owned by bind9")
	if err := Retry(context.Background(), waits, func() error { calls++; return refused }); err != refused || calls != 1 {
		t.Fatalf("a verified failure is not retried: %v %d", err, calls)
	}
	calls = 0
	if err := Retry(context.Background(), waits, func() error { calls++; return Incomplete(refused) }); !Is(err) || calls != 4 {
		t.Fatalf("after the last pause the unknown result is returned: %v %d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if err := Retry(ctx, waits, func() error { calls++; return Incomplete(refused) }); !Is(err) || calls != 1 {
		t.Fatalf("an ended context stops the retry: %v %d", err, calls)
	}
}
