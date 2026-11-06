package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// displayCheckTestClock enables the cache and drives its clock for one test.
type displayCheckTestTime struct {
	mu  *sync.Mutex
	now *time.Time
}

func (c displayCheckTestTime) get() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return *c.now }
func (c displayCheckTestTime) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.now = c.now.Add(d)
}

func displayCheckTestClock(t *testing.T) displayCheckTestTime {
	t.Helper()
	now := time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	previousClock, previousEnabled := displayCheckClock, displayChecksEnabled.Load()
	displayCheckClock = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	displayChecksEnabled.Store(true)
	t.Cleanup(func() {
		displayCheckClock = previousClock
		displayChecksEnabled.Store(previousEnabled)
	})
	return displayCheckTestTime{mu: &mu, now: &now}
}

func TestDisplayCheckServesFreshAnswerWithoutAskingAgain(t *testing.T) {
	displayCheckTestClock(t)
	var c displayCheck[int]
	var reads atomic.Int32
	read := func(context.Context) (int, error) { return int(reads.Add(1)), nil }
	for i := 0; i < 3; i++ {
		value, view, err := c.get(context.Background(), "k", read)
		if err != nil || value != 1 || view.Rechecking {
			t.Fatalf("read %d = %d %+v %v, want the first answer, not rechecking", i, value, view, err)
		}
	}
	if reads.Load() != 1 {
		t.Fatalf("checks = %d, want 1", reads.Load())
	}
}

func TestDisplayCheckShowsLastKnownWhileOneCheckRunsAgain(t *testing.T) {
	now := displayCheckTestClock(t)
	var c displayCheck[int]
	var reads atomic.Int32
	release := make(chan struct{})
	read := func(context.Context) (int, error) {
		n := int(reads.Add(1))
		if n > 1 {
			<-release
		}
		return n, nil
	}
	if value, _, err := c.get(context.Background(), "k", read); err != nil || value != 1 {
		t.Fatalf("first read = %d %v", value, err)
	}
	first := now.get()
	now.advance(displayCheckFresh + time.Second)

	// Several screens at once: each is answered at once with the last known
	// answer and its time; one check runs.
	for i := 0; i < 5; i++ {
		value, view, err := c.get(context.Background(), "k", read)
		if err != nil || value != 1 || !view.Rechecking || !view.CheckedAt.Equal(first) {
			t.Fatalf("stale read %d = %d %+v %v, want last known with its time, rechecking", i, value, view, err)
		}
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		value, view, err := c.get(context.Background(), "k", read)
		if err == nil && value == 2 && !view.Rechecking {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the check behind the screen never replaced the answer: %d %+v %v", value, view, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reads.Load() != 2 {
		t.Fatalf("checks = %d, want 2 (one behind the screen, not one per read)", reads.Load())
	}
}

func TestDisplayCheckKeepsNothingFromAFailedCheck(t *testing.T) {
	displayCheckTestClock(t)
	var c displayCheck[int]
	var reads atomic.Int32
	failing := errors.New("agent unavailable")
	read := func(context.Context) (int, error) {
		if reads.Add(1) == 1 {
			return 0, failing
		}
		return 7, nil
	}
	if _, _, err := c.get(context.Background(), "k", read); !errors.Is(err, failing) {
		t.Fatalf("first read error = %v, want the check's own failure", err)
	}
	if value, view, err := c.get(context.Background(), "k", read); err != nil || value != 7 || view.Rechecking {
		t.Fatalf("second read = %d %+v %v, want a new check", value, view, err)
	}
}

func TestDisplayCheckIsReadAgainAfterADNSChange(t *testing.T) {
	displayCheckTestClock(t)
	var c displayCheck[int]
	var reads atomic.Int32
	read := func(context.Context) (int, error) { return int(reads.Add(1)), nil }
	_, _, _ = c.get(context.Background(), "k", read)
	invalidateDisplayChecks()
	value, view, err := c.get(context.Background(), "k", read)
	if err != nil || value != 2 || view.Rechecking {
		t.Fatalf("read after a change = %d %+v %v, want a waited, new check", value, view, err)
	}
}

func TestDisplayCheckTooOldAnswerIsNotShown(t *testing.T) {
	now := displayCheckTestClock(t)
	var c displayCheck[int]
	var reads atomic.Int32
	read := func(context.Context) (int, error) { return int(reads.Add(1)), nil }
	_, _, _ = c.get(context.Background(), "k", read)
	now.advance(displayCheckKeep + time.Second)
	if value, view, err := c.get(context.Background(), "k", read); err != nil || value != 2 || view.Rechecking {
		t.Fatalf("read of a too-old answer = %d %+v %v, want a waited, new check", value, view, err)
	}
}

func TestDisplayCheckOffReadsLive(t *testing.T) {
	previous := displayChecksEnabled.Load()
	displayChecksEnabled.Store(false)
	t.Cleanup(func() { displayChecksEnabled.Store(previous) })
	var c displayCheck[int]
	var reads atomic.Int32
	read := func(context.Context) (int, error) { return int(reads.Add(1)), nil }
	_, _, _ = c.get(context.Background(), "k", read)
	_, _, _ = c.get(context.Background(), "k", read)
	if reads.Load() != 2 {
		t.Fatalf("checks = %d, want every read live while off", reads.Load())
	}
}

func TestRequestChangesDisplayChecks(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/v1/dns/engine", false},
		{"POST", "/api/v1/dns/engine/switch", true},
		{"POST", "/api/v1/dns/remote/receiver/status", false},
		{"POST", "/api/v1/dns/remote/receiver/publish", true},
		{"POST", "/api/v1/setup/components", true},
		{"POST", "/api/v1/service/install", true},
		{"POST", "/api/v1/managed-services/pdns/action", true},
		{"PUT", "/api/v1/settings/dns-setup", true},
		{"POST", "/api/v1/domains/create", true},
		{"DELETE", "/api/v1/domains/12", true},
		{"POST", "/api/v1/domains/12/mail/accounts", false},
		{"POST", "/api/v1/databases/", false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if got := requestChangesDisplayChecks(r); got != tc.want {
			t.Errorf("%s %s = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
