// Package checkresult names one kind of result: a check that could not
// finish (its deadline passed, it was cancelled). Its answer is unknown. It
// is neither "absent" nor "not ours" nor "failed", and it must never be the
// reason to stop, disable, empty or remove a running native service
// (D-024, constitution: an unknown result is never read as a verified one).
// The caller keeps what runs, says that checking could not finish, and
// checks again later.
//
// Bir kontrolün bitememesi (süre doldu, iptal edildi) bilinmeyen sonuçtur:
// "yok", "bizim değil" ya da "başarısız" değildir; çalışan bir hizmeti
// durdurmanın veya kaldırmanın gerekçesi olamaz.
package checkresult

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// ErrIncomplete marks an error whose check did not finish.
var ErrIncomplete = errors.New("the check could not finish, so its result is unknown")

// Incomplete marks err as an unfinished check. A nil err stays nil.
func Incomplete(err error) error {
	if err == nil || errors.Is(err, ErrIncomplete) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrIncomplete, err)
}

// IfContextEnded marks err as an unfinished check when ctx has ended (its
// deadline passed or it was cancelled): a command killed by that context
// reports a failure that says nothing about what it was asked. Otherwise err
// is returned as it is.
func IfContextEnded(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return Incomplete(err)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, os.ErrDeadlineExceeded) {
		return Incomplete(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Incomplete(err)
	}
	return err
}

// RetryWaits are the pauses between the attempts of Retry: a check that
// could not finish is asked again a few times, more slowly each time, before
// its unknown result is returned (the server is usually just busy: installs
// running, alpha.129 lab).
var RetryWaits = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

// Retry runs check and, while its result is an unfinished check and ctx has
// not ended, waits the next of waits and runs it again. A finished result
// (nil or a verified failure) is returned at once; after the last wait the
// unknown result is returned as it is. check gets its own deadline each time
// (callers bound one attempt, not the whole retry).
func Retry(ctx context.Context, waits []time.Duration, check func() error) error {
	err := check()
	for _, wait := range waits {
		if !Is(err) || ctx == nil || ctx.Err() != nil {
			return err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
		err = check()
	}
	return err
}

// Is reports whether err is (or wraps) an unfinished check.
func Is(err error) bool { return errors.Is(err, ErrIncomplete) }
