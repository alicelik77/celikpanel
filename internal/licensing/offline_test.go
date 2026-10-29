package licensing

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Seven-day offline window, hourly checks and clock safety (D-039).

// fresh is the service's next receipt at now: refresh after an hour, valid
// offline for seven days, never past the annual expiry.
func fresh(t *testing.T, c Claims, priv ed25519.PrivateKey, now int64) Envelope {
	t.Helper()
	c.IssuedAt = now
	c.RefreshAfter = now + 3600
	c.OfflineUntil = min(now+7*86400, c.ExpiresAt)
	return sign(t, c, priv)
}

func TestOutageHasBoundedGraceBackoffAndRecoversOnUse(t *testing.T) {
	pub, priv, c, e := fixture(t)
	now := time.Unix(c.IssuedAt, 0)
	m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, c.ServerID)
	m.clock = func() time.Time { return now }
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(fresh(t, c, priv, now.Unix()))
	}))
	defer server.Close()
	m.endpoint = server.URL
	// About an hour after the last check (the jitter takes up to a fifth off).
	now = now.Add(47 * time.Minute)
	if !m.CanProvision(context.Background()) || calls != 0 {
		t.Fatal("checked before its time")
	}
	now = now.Add(13 * time.Minute)
	if !m.CanProvision(context.Background()) || calls != 1 {
		t.Fatal("hourly check during an outage must keep the valid receipt", calls)
	}
	for i := 0; i < 100; i++ {
		if !m.CanProvision(context.Background()) {
			t.Fatal("outage closed a valid receipt")
		}
	}
	if calls != 1 {
		t.Fatal("outage retried on every request", calls)
	}
	// The backoff doubles from a minute (each delay shortened by up to a fifth).
	now = now.Add(time.Minute)
	_ = m.CanProvision(context.Background())
	now = now.Add(time.Minute)
	_ = m.CanProvision(context.Background())
	if calls != 2 {
		t.Fatal("second delay did not grow", calls)
	}
	now = now.Add(time.Minute)
	_ = m.CanProvision(context.Background())
	if calls != 3 {
		t.Fatal("retry after the doubled delay missing", calls)
	}
	if s := m.Status(); s.Check != CheckUnreachable || !s.CanProvision || s.State != "active" {
		t.Fatal(s)
	}
	// A retry is allowed at the latest when the receipt lapses; from then on
	// requests wait for a check, retried every RetryInterval.
	now = time.Unix(c.OfflineUntil-10, 0)
	if !m.CanProvision(context.Background()) || calls != 4 {
		t.Fatal("last valid moments", calls)
	}
	now = time.Unix(c.OfflineUntil, 0)
	for i := 0; i < 100; i++ {
		if m.CanProvision(context.Background()) {
			t.Fatal("outage authorized access past the offline window")
		}
	}
	if calls != 5 {
		t.Fatal("lapsed receipt retried on every request", calls)
	}
	if s := m.Status(); s.State != "verification_unavailable" || s.Reason != ReasonOfflineEnded {
		t.Fatal(s)
	}
	now = now.Add(RetryInterval)
	fail = false
	if !m.CanProvision(context.Background()) || calls != 6 {
		t.Fatal("active request failed to recover")
	}
	if s := m.Status(); s.Check != "" || s.OfflineForSeconds != 7*86400 {
		t.Fatal("recovered status", s)
	}
	now = time.Unix(c.ExpiresAt, 0)
	if m.CanProvision(context.Background()) {
		t.Fatal("refresh extended annual term")
	}
}

// A due check of a still-valid receipt does not hold requests while the
// service is slow or unreachable: they proceed after the short wait, share the
// one refresh in flight, and its result applies when it arrives.
func TestDueCheckOfValidReceiptWaitsBrieflyAndIsShared(t *testing.T) {
	pub, priv, c, e := fixture(t)
	var now atomic.Int64
	now.Store(c.IssuedAt)
	m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, c.ServerID)
	m.clock = func() time.Time { return time.Unix(now.Load(), 0) }
	m.wait = 50 * time.Millisecond
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-release
		_ = json.NewEncoder(w).Encode(fresh(t, c, priv, now.Load()))
	}))
	defer server.Close()
	m.endpoint = server.URL
	now.Add(3600)
	started := time.Now()
	for i := 0; i < 20; i++ {
		if s := m.AccessStatus(context.Background()); !s.CanProvision {
			t.Fatal("slow service closed a valid receipt", s)
		}
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatal("requests were held for the client timeout", elapsed)
	}
	if calls.Load() != 1 {
		t.Fatal("refresh in flight was duplicated", calls.Load())
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for m.Status().VerifiedAt != now.Load() {
		if time.Now().After(deadline) {
			t.Fatal("background refresh result was not applied", m.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A reachable service's refusal (license removed, moved or replaced) applies to
// the very request that discovered it, though the receipt is still valid offline.
func TestReachableRejectionAppliesToDiscoveringRequest(t *testing.T) {
	pub, _, c, e := fixture(t)
	now := time.Unix(c.IssuedAt, 0)
	m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, c.ServerID)
	m.clock = func() time.Time { return now }
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_activation"}`))
	}))
	defer server.Close()
	m.endpoint = server.URL
	now = now.Add(time.Hour)
	if s := m.AccessStatus(context.Background()); s.CanProvision || s.State != "invalid" || s.Observation != ObservationKnown || s.Check != "" {
		t.Fatal("removed or moved license still opened management", s)
	}
}

// The service before this policy signs a one-minute window refreshed after 45
// seconds. A panel with the seven-day policy honours exactly that window.
func TestShortReceiptFromOlderServiceStillWorks(t *testing.T) {
	pub, priv, c, _ := fixture(t)
	now := time.Unix(c.IssuedAt, 0)
	short := func(at int64) Envelope {
		next := c
		next.IssuedAt, next.RefreshAfter, next.OfflineUntil = at, at+45, min(at+60, c.ExpiresAt)
		return sign(t, next, priv)
	}
	m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, c.ServerID)
	m.clock = func() time.Time { return now }
	if err := m.save(short(now.Unix())); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(short(now.Unix()))
	}))
	defer server.Close()
	m.endpoint = server.URL
	if s := m.Status(); !s.CanProvision || s.OfflineUntil != c.IssuedAt+60 {
		t.Fatal(s)
	}
	now = now.Add(45 * time.Second)
	if !m.CanProvision(context.Background()) || calls != 1 || m.Status().OfflineUntil != now.Unix()+60 {
		t.Fatal("short receipt was not renewed in time", calls)
	}
	fail = true
	now = now.Add(45 * time.Second)
	if !m.CanProvision(context.Background()) || calls != 2 {
		t.Fatal("renewal attempt inside the minute", calls)
	}
	now = now.Add(15 * time.Second)
	if m.CanProvision(context.Background()) || calls != 3 {
		t.Fatal("short receipt outlived its signed minute", calls)
	}
}

func TestRefreshAndRetryJitterStayInBounds(t *testing.T) {
	pub, _, c, _ := fixture(t)
	seen := map[int64]bool{}
	for i := 0; i < 16; i++ {
		server := strings.Repeat(string("0123456789abcdef"[i]), 64)
		m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, server)
		at := m.refreshAt(c)
		if at > c.IssuedAt+3600 || at < c.IssuedAt+2880 {
			t.Fatal("hourly check outside 48-60 minutes", at-c.IssuedAt)
		}
		seen[at] = true
		for n, want := range []int64{60, 120, 240, 480, 960, 1800, 1800} {
			if d := m.retryDelay(n+1, c.IssuedAt); d > want || d < want*4/5 {
				t.Fatal("retry delay", n+1, d)
			}
		}
	}
	if len(seen) < 8 {
		t.Fatal("servers are not spread", len(seen))
	}
}

// A clock set back cannot stretch the offline window: not while the panel runs
// (monotonic time), not across a restart (the persisted high-water mark), and
// not before the receipt's own issue time.
func TestClockSetBackDoesNotExtendOfflineWindow(t *testing.T) {
	pub, priv, c, e := fixture(t)
	file := filepath.Join(t.TempDir(), "license.json")
	now := time.Unix(c.IssuedAt, 0)
	service := time.Unix(c.IssuedAt, 0)
	clock := func() time.Time { return now }
	manager := func() *Manager {
		m, err := New(file, pub, c.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		m.clock = clock
		return m
	}
	m := manager()
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(fresh(t, c, priv, service.Unix()))
	}))
	defer server.Close()
	m.endpoint = server.URL
	if s := m.Status(); !s.CanProvision {
		t.Fatal(s)
	}
	// Within the tolerated skew before issue the receipt opens; beyond it not.
	now = time.Unix(c.IssuedAt-60, 0)
	if s := manager().Status(); !s.CanProvision {
		t.Fatal("small clock skew closed the panel", s)
	}
	now = time.Unix(c.IssuedAt-int64(ClockSkew/time.Second)-1, 0)
	if s := manager().Status(); s.CanProvision || s.State != "verification_unavailable" || s.Reason != ReasonClockBehind || s.Observation != ObservationUnavailable {
		t.Fatal("clock before issue opened the panel", s)
	}

	// Running panel: two days in, an outage check records the moment; then the
	// clock goes back a day.
	now = time.Unix(c.IssuedAt+2*86400, 0)
	if !m.CanProvision(context.Background()) {
		t.Fatal(m.Status())
	}
	now = time.Unix(c.IssuedAt+86400, 0)
	if s := m.Status(); s.CanProvision || s.Reason != ReasonClockBehind {
		t.Fatal("clock set back kept the running panel open", s)
	}
	// Restart with the clock still set back: the persisted mark refuses it.
	restarted := manager()
	restarted.endpoint = server.URL
	if s := restarted.Status(); s.CanProvision || s.Reason != ReasonClockBehind {
		t.Fatal("restart with the clock set back reopened the window", s)
	}
	// The service answers, but this clock is more than the skew behind it: its
	// fresh receipt is refused and the status names the clock as the cause.
	fail = false
	service = time.Unix(c.IssuedAt+2*86400+600, 0)
	if restarted.CanProvision(context.Background()) || restarted.Status().Check != CheckClockBehind {
		t.Fatal("receipt from the future accepted", restarted.Status())
	}
	// With the clock corrected the next check restores management.
	now = service
	if err := restarted.Refresh(context.Background(), true); err != nil || !restarted.Status().CanProvision {
		t.Fatal("corrected clock did not recover", err, restarted.Status())
	}
	if s := restarted.Status(); s.Check != "" || s.Reason != "" || s.VerifiedAt != service.Unix() {
		t.Fatal(s)
	}
}

// A refresh that succeeds while the clock was set back (the service reached
// with a clock within the skew) starts the high-water marks again from it.
func TestSuccessfulCheckResetsClockMarks(t *testing.T) {
	pub, priv, c, e := fixture(t)
	now := time.Unix(c.IssuedAt+3*86400, 0)
	m, _ := New(filepath.Join(t.TempDir(), "license.json"), pub, c.ServerID)
	m.clock = func() time.Time { return now }
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(fresh(t, c, priv, now.Unix()))
	}))
	defer server.Close()
	m.endpoint = server.URL
	if !m.Status().CanProvision {
		t.Fatal(m.Status())
	}
	// The clock was a day ahead and is corrected: management waits for one
	// check, which the reachable service answers at once.
	now = time.Unix(c.IssuedAt+2*86400, 0)
	if s := m.AccessStatus(context.Background()); !s.CanProvision || s.VerifiedAt != now.Unix() {
		t.Fatal("corrected clock with a reachable service did not recover", s)
	}
}
