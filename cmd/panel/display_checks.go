package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Last-known answers for screens (v0.1.0-alpha.127, owner report 2026-11-06:
// a domain page on a joined server waited ~10 s on "Checking which database
// engines are installed…" and 5–10 s on "Checking where this domain points…").
//
// Both reads proved the whole DNS pair on every page open: the Agent's DNS
// readiness transfers the peer's catalog and asks both servers for the SOA of
// every zone, one query after another (about 6 s between Boston and
// Frankfurt), and a joined server then asked its main DNS server over HTTPS,
// which ran the same proof towards this server again. Several screens of one
// page did it at once, so the reads queued behind each other.
//
// Screens now read those two proofs through this cache:
//   - an answer younger than displayCheckFresh is used as it is;
//   - an older answer is shown at once as what it is — the last verified
//     answer, with its time — while one check runs behind the screen; the
//     response says `rechecking` and the screen reads again shortly;
//   - nothing is kept from a failed check, and no answer is served across a
//     DNS-relevant change made through the panel (invalidateDisplayChecks) or
//     across a change of the durable DNS identity (the cache key).
//
// Only screens read through here. Every change (domain creation, import,
// publication, DNS setup) keeps calling the live proofs, so a last-known
// answer can never authorise a change. Polling cannot start a second check:
// one check per key runs at a time.
//
// Ekranlar icin son bilinen yanitlar: taze yanit oldugu gibi kullanilir;
// eskisi zamaniyla hemen gosterilir ve arkada tek bir kontrol kosar. Hatali
// kontrol saklanmaz; paneldeki DNS degisikligi onbellegi bosaltir. Degisiklik
// yapan her yol canli kanit kullanmaya devam eder.

// displayChecksEnabled is set by the production panel's main. Tests leave it
// off, so every read is live unless a test turns it on.
var displayChecksEnabled atomic.Bool

// displayChecksGeneration moves whenever a DNS-relevant change begins or
// ends; an answer read under an older generation is never served.
var displayChecksGeneration atomic.Uint64

var (
	displayCheckFresh = 30 * time.Second
	// displayCheckKeep bounds how old a last-known answer may be and still be
	// shown while it is checked again; older answers are read before showing.
	displayCheckKeep = 24 * time.Hour
	// displayCheckLimit bounds one check that runs behind the screen.
	displayCheckLimit = 30 * time.Second
	displayCheckClock = time.Now
)

func invalidateDisplayChecks() {
	displayChecksGeneration.Add(1)
}

// The changes after which a screen must not see an earlier DNS answer: DNS,
// DNS setup and server setup, managed services and components, panel
// settings. The joined server's own status question is a read.
var displayCheckDomainDelete = regexp.MustCompile(`^/api/v1/domains/[0-9]+/?$`)

func requestChangesDisplayChecks(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	path := r.URL.Path
	if path == "/api/v1/dns/remote/receiver/status" {
		return false
	}
	for _, prefix := range []string{
		"/api/v1/dns", "/api/v1/pdns/", "/api/v1/setup", "/api/v1/server/",
		"/api/v1/settings/", "/api/v1/service/", "/api/v1/managed-services/",
		"/api/v1/domains/create",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return r.Method == http.MethodDelete && displayCheckDomainDelete.MatchString(path)
}

// displayCheckResult is one check's outcome.
type displayCheckResult[T any] struct {
	value T
	at    time.Time
	err   error
}

type displayCheckFlight[T any] struct {
	generation uint64
	done       chan struct{}
	result     displayCheckResult[T]
}

type displayCheckEntry[T any] struct {
	known      bool
	value      T
	at         time.Time
	generation uint64
	flight     *displayCheckFlight[T]
}

// displayCheck keeps one last-known answer per key.
type displayCheck[T any] struct {
	mu      sync.Mutex
	entries map[string]*displayCheckEntry[T]
}

// displayCheckView is what a screen is told about the answer it got.
type displayCheckView struct {
	CheckedAt  time.Time
	Rechecking bool
}

// get returns the answer for key: fresh, last-known while one check runs
// behind it, or — with no usable answer — the outcome of a check the caller
// waits for (until ctx ends).
func (c *displayCheck[T]) get(
	ctx context.Context, key string, read func(context.Context) (T, error),
) (T, displayCheckView, error) {
	var zero T
	if !displayChecksEnabled.Load() {
		value, err := read(ctx)
		return value, displayCheckView{CheckedAt: displayCheckClock()}, err
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]*displayCheckEntry[T])
	}
	entry := c.entries[key]
	if entry == nil {
		if len(c.entries) >= 32 {
			// Keys follow durable identities; a long-lived process does not
			// keep every identity it ever saw.
			c.entries = make(map[string]*displayCheckEntry[T])
		}
		entry = &displayCheckEntry[T]{}
		c.entries[key] = entry
	}
	generation := displayChecksGeneration.Load()
	now := displayCheckClock()
	usable := entry.known && entry.generation == generation && !entry.at.After(now)
	if usable && now.Sub(entry.at) < displayCheckFresh {
		value, at := entry.value, entry.at
		c.mu.Unlock()
		return value, displayCheckView{CheckedAt: at}, nil
	}
	flight := c.startLocked(entry, generation, read)
	if usable && now.Sub(entry.at) < displayCheckKeep {
		value, at := entry.value, entry.at
		c.mu.Unlock()
		return value, displayCheckView{CheckedAt: at, Rechecking: true}, nil
	}
	c.mu.Unlock()
	select {
	case <-flight.done:
	case <-ctx.Done():
		return zero, displayCheckView{}, ctx.Err()
	}
	if flight.result.err != nil {
		return zero, displayCheckView{}, flight.result.err
	}
	return flight.result.value, displayCheckView{CheckedAt: flight.result.at}, nil
}

// startLocked joins the check of this generation that is already running, or
// starts one. The check does not belong to the request that started it: a
// closed page must not cancel the answer the next page needs.
func (c *displayCheck[T]) startLocked(
	entry *displayCheckEntry[T], generation uint64, read func(context.Context) (T, error),
) *displayCheckFlight[T] {
	if entry.flight != nil && entry.flight.generation == generation {
		return entry.flight
	}
	flight := &displayCheckFlight[T]{generation: generation, done: make(chan struct{})}
	entry.flight = flight
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), displayCheckLimit)
		value, err := read(ctx)
		cancel()
		at := displayCheckClock()
		flight.result = displayCheckResult[T]{value: value, at: at, err: err}
		c.mu.Lock()
		if entry.flight == flight {
			entry.flight = nil
		}
		// A failed check keeps nothing: the earlier answer stays what it was
		// (last known, with its own time), and the next read checks again.
		if err == nil && generation == displayChecksGeneration.Load() &&
			(!entry.known || generation >= entry.generation) {
			entry.known, entry.value, entry.at, entry.generation = true, value, at, generation
		}
		c.mu.Unlock()
		close(flight.done)
	}()
	return flight
}

// displayChecks holds the screens' last-known DNS answers of one panel.
type displayChecks struct {
	backend displayCheck[dnsBackendRuntimeView]
	remote  displayCheck[remoteDNSAuthority]
}

type dnsBackendRuntimeView struct {
	runtimes       map[transport.DNSEngine]transport.DNSBackendRuntimeState
	port53Conflict bool
}

// dnsPublisherStatusForDisplay is dnsPublisherStatus for screens. The durable
// identity and a running switch are read live; only the Agent's runtime proof
// is last-known, keyed by that identity.
func (p *Panel) dnsPublisherStatusForDisplay(ctx context.Context) (dnsPublisherStatusView, displayCheckView, error) {
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil {
		return dnsPublisherStatusView{}, displayCheckView{}, err
	}
	if state.ActiveEngine == "" || state.CurrentSwitchID != "" {
		return classifyDNSPublisher(state, nil, false), displayCheckView{CheckedAt: displayCheckClock()}, nil
	}
	runtime, view, err := p.displayChecks.backend.get(ctx, fmt.Sprintf("%+v", state),
		func(ctx context.Context) (dnsBackendRuntimeView, error) {
			runtimes, port53Conflict, _, err := p.readDNSBackendRuntime(ctx)
			return dnsBackendRuntimeView{runtimes: runtimes, port53Conflict: port53Conflict}, err
		})
	if err != nil {
		return dnsPublisherStatusView{}, displayCheckView{}, err
	}
	return classifyDNSPublisher(state, runtime.runtimes, runtime.port53Conflict), view, nil
}

// activeDNSPublisherForDisplay is activeDNSPublisher for screens.
func (p *Panel) activeDNSPublisherForDisplay(ctx context.Context) (dnsPublisherIdentity, bool, error) {
	status, _, err := p.dnsPublisherStatusForDisplay(ctx)
	if err != nil {
		return dnsPublisherIdentity{}, false, err
	}
	return status.Identity, status.State == dnsServerStateReady, nil
}

// remoteDNSConnectionReadinessForDisplay is remoteDNSConnectionReadiness for
// screens: the connection row is read live, the main DNS server's answer is
// last-known.
func (p *Panel) remoteDNSConnectionReadinessForDisplay(ctx context.Context, id string) (remoteDNSAuthority, displayCheckView, error) {
	c, err := p.readRemoteDNSConnection(ctx, id)
	if err != nil {
		return remoteDNSAuthority{}, displayCheckView{}, err
	}
	if c.Status != "ready" || !remoteDNSValidSecret(c.credential) {
		return remoteDNSAuthority{}, displayCheckView{}, errRemoteDNSNotReady
	}
	return p.displayChecks.remote.get(ctx, c.ID+"|"+c.Endpoint,
		func(ctx context.Context) (remoteDNSAuthority, error) {
			return p.remoteDNSConnectionReadiness(ctx, id)
		})
}

// olderDisplayCheck combines two answers: the older time, rechecking if
// either is.
func olderDisplayCheck(a, b displayCheckView) displayCheckView {
	out := a
	if out.CheckedAt.IsZero() || (!b.CheckedAt.IsZero() && b.CheckedAt.Before(out.CheckedAt)) {
		out.CheckedAt = b.CheckedAt
	}
	out.Rechecking = a.Rechecking || b.Rechecking
	return out
}

// defaultRemoteDNSReadyForDisplay is remoteDNSConnectionForCreation(ctx, "")
// for screens: whether the saved main DNS server last answered ready.
func (p *Panel) defaultRemoteDNSReadyForDisplay(ctx context.Context) (bool, displayCheckView) {
	id, err := p.defaultRemoteDNSConnectionID(ctx)
	if err != nil || id == "" {
		return false, displayCheckView{}
	}
	authority, view, err := p.remoteDNSConnectionReadinessForDisplay(ctx, id)
	return err == nil && authority.Ready, view
}

// warmDisplayChecks reads the screens' DNS answers once after the start.
func (p *Panel) warmDisplayChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), displayCheckLimit)
	defer cancel()
	if _, err := p.hostingCapsForDisplay(ctx); err != nil {
		log.Printf("first DNS check for the screens did not finish; the first page that needs it checks again: %v", err)
	}
}
