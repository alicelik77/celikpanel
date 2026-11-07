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
