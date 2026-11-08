package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Closing remote database access while the panel is still starting
// (alpha.130 lab, candidate 36e09fb38).
//
// During a long "starting" boot (systemd is-system-running = starting, for
// example a cloud-init hang) the panel serves only its start-up handler,
// which answered 503 PANEL_STARTING to every write: nobody could remove an
// address or turn remote access off, so the D5 narrowing never ran. Closing
// must always be possible. The start-up handler therefore lets exactly these
// writes through to the ordinary, ownership-checked handlers:
//
//   - DELETE /api/v1/database-remote/rules/{id}: the rule goes;
//   - PUT /api/v1/database-remote/rules/{id}: only an end that comes no later
//     than the current one;
//   - PUT /api/v1/database-remote/server: only {"offered": false}.
//
// Each saves the change and then only narrows the address filter
// (NarrowDatabaseRemoteFilterV1: no lease, no host identity, never adds);
// the engines' own rules follow at the first full apply after start.
// Anything that would open (adding, a later end, turning on, the name
// lookup agreement, "apply again") keeps the PANEL_STARTING answer.
//
// Panel açılırken yalnız kapatan yazmalar geçer; açan her şey beklemeye
// devam eder.

type databaseRemoteStartingKey struct{}

// databaseRemoteStarting: the request came through the start-up handler.
func databaseRemoteStarting(ctx context.Context) bool {
	starting, _ := ctx.Value(databaseRemoteStartingKey{}).(bool)
	return starting
}

// databaseRemoteClosingRoute: a method and path of the start-up allowlist
// (the handler still refuses what would open).
func databaseRemoteClosingRoute(r *http.Request) bool {
	route, ok := matchDatabaseRemoteRoute(r)
	if !ok {
		return false
	}
	switch {
	case route.kind == "rule" && (r.Method == http.MethodDelete || r.Method == http.MethodPut):
		return true
	case route.kind == "server" && r.Method == http.MethodPut:
		return true
	}
	return false
}

// handleDatabaseRemoteWhileStarting is the start-up handler's entry.
func (p *Panel) handleDatabaseRemoteWhileStarting(w http.ResponseWriter, r *http.Request) {
	if !databaseRemoteClosingRoute(r) {
		writeDatabaseRemoteStartingRefusal(w)
		return
	}
	p.handleDatabaseRemote(w, r.WithContext(context.WithValue(r.Context(), databaseRemoteStartingKey{}, true)))
}

// writeDatabaseRemoteStartingRefusal is the start-up answer for a write
// that would open access.
func writeDatabaseRemoteStartingRefusal(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "3")
	writeCodedError(w, http.StatusServiceUnavailable, "PANEL_STARTING",
		"Panel management is still starting. Removing an address, giving it a sooner end or turning connections from outside off works now; anything that opens access waits until the panel has started.", "")
}

// writeDatabaseRemoteClosedWhileStarting narrows the filter to what is still
// wanted and answers. The change is saved either way; the sync loop applies
// the rest once the panel has started.
func (p *Panel) writeDatabaseRemoteClosedWhileStarting(w http.ResponseWriter, ctx context.Context) {
	closed := false
	if desired, err := p.databaseRemoteDesired(ctx); err != nil {
		log.Printf("database remote access: the wanted rules could not be read while starting: %v", err)
	} else {
		closed = p.narrowDatabaseRemote(ctx, desired)
	}
	p.kickDatabaseRemoteSync()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	note := "The change is saved. The panel is still starting: the address is blocked at the firewall now and its database access is removed when the panel has started."
	if !closed {
		note = "The change is saved. The panel is still starting and the server could not be asked yet; access is closed as soon as it answers."
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "starting": true, "closed_at_filter": closed, "error": note})
}

// databaseRemoteEndIsNotLater: next (stored form, "" = no end) comes no later
// than previous.
func databaseRemoteEndIsNotLater(next, previous string) bool {
	if previous == "" {
		return true
	}
	if next == "" {
		return false
	}
	a, errA := time.Parse(databaseRemoteTimeLayout, next)
	b, errB := time.Parse(databaseRemoteTimeLayout, previous)
	return errA == nil && errB == nil && !a.After(b)
}
