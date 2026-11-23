package membership

// Review of PR #361, finding 5: a burst of sign-ins never runs more password work at once
// than the slots allow; the rest waits its turn and every check still completes.

import (
	"encoding/base64"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordWorkIsBounded(t *testing.T) {
	var active, most atomic.Int32
	passwordWorkHook = func() {
		n := active.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // long enough for the burst to overlap
		active.Add(-1)
	}
	defer func() { passwordWorkHook = nil }()
	// A cheap hash in the stored format, so the burst is quick; a real one would only be slower.
	cheap := "$argon2id$v=19$m=8,t=1,p=1$" + base64.RawStdEncoding.EncodeToString([]byte("saltsaltsaltsalt")) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	var wg sync.WaitGroup
	done := atomic.Int32{}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = verifyPassword("burst-password-1234", cheap)
			done.Add(1)
		}()
	}
	wg.Wait()
	if most.Load() > int32(cap(passwordSlots)) || most.Load() < 2 {
		t.Fatalf("%d checks ran at once; the bound is %d", most.Load(), cap(passwordSlots))
	}
	if done.Load() != 16 {
		t.Fatalf("only %d of 16 checks finished", done.Load())
	}
}

// fillSlots takes every password slot until the returned function frees them.
func fillSlots(t *testing.T) func() {
	t.Helper()
	for i := 0; i < cap(passwordSlots); i++ {
		passwordSlots <- struct{}{}
	}
	return func() {
		for i := 0; i < cap(passwordSlots); i++ {
			<-passwordSlots
		}
	}
}

// Second review of #361, finding 6: the wait for a slot has a deadline and a queue limit,
// answered with the existing refusal (rate_limited).
func TestPasswordQueueHasALimitAndADeadline(t *testing.T) {
	wait, limit := passwordWait, passwordQueueLimit
	defer func() { passwordWait, passwordQueueLimit = wait, limit }()
	passwordWait, passwordQueueLimit = 150*time.Millisecond, 1
	free := fillSlots(t)
	defer free()
	cheap := "$argon2id$v=19$m=8,t=1,p=1$" + base64.RawStdEncoding.EncodeToString([]byte("saltsaltsaltsalt")) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	started := time.Now()
	if _, err := verifyPassword("queued-password-1234", cheap); err != ErrRateLimited || time.Since(started) < 100*time.Millisecond {
		t.Fatalf("a full house must be refused after the deadline: %v after %v", err, time.Since(started))
	}
	// While one caller waits, the queue (limit 1) is full: the next is refused at once.
	go func() { _, _ = verifyPassword("waiting-password-1234", cheap) }()
	time.Sleep(30 * time.Millisecond)
	started = time.Now()
	if _, err := verifyPassword("refused-password-1234", cheap); err != ErrRateLimited || time.Since(started) > 80*time.Millisecond {
		t.Fatalf("a full queue must be refused at once: %v after %v", err, time.Since(started))
	}
}

// Second review of #361, finding 6: a renewal waiting for a password slot holds no write
// lock, so the rest of the service (every request's rate limit, panels' refreshes) goes on.
func TestPasswordWorkHoldsNoWriteLock(t *testing.T) {
	h := newHarness(t)
	if err := h.s.Register("lock@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ConsumeToken(h.lastMail().Token, "verify", "lock-password-12345"); err != nil {
		t.Fatal(err)
	}
	m := must(h.s.Login("lock@example.com", "lock-password-12345"))
	l := must(h.s.Issue(m.ID))
	other := h.open() // a second connection, as another request would have
	free := fillSlots(t)
	renewed := make(chan error, 1)
	go func() { renewed <- h.s.Renew(m.ID, l.ID, "lock-password-12345") }()
	time.Sleep(100 * time.Millisecond) // the renewal now waits for a slot
	started := time.Now()
	err := other.Limit("another-request", 100, 60)
	free()
	if err != nil || time.Since(started) > time.Second {
		t.Fatalf("a write had to wait for the password work: %v after %v", err, time.Since(started))
	}
	if err := <-renewed; err != ErrRenewalNotDue {
		t.Fatalf("renew after the slots were freed: %v", err)
	}
}
