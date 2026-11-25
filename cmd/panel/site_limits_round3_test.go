package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a third fix round: the Panel's side of what the third review of PR
// #349 found (N2 a move whose put-back did not complete, N3 the kernel's
// values, W1 the customer's state).

func round3Card(t *testing.T, f rightsFixture, handler http.Handler, user int) siteLimitsView {
	t.Helper()
	w := siteLimitsCall(t, f, handler, user, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
	wantStatus(t, w, http.StatusOK, "")
	var view siteLimitsView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

// TestTheCardShowsWhatTheKernelHolds (N3): a program runs in the slice and
// the file is CelikPanel's; the limits are "in force" only when the slice's
// cgroup holds them. Written but not held is "pending" with the kernel's
// values as the ones in force; a cgroup that was not read is "unknown",
// never "yes"; the customer reads the neutral state for both (W1).
func TestTheCardShowsWhatTheKernelHolds(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	reading := siteLimitsStatus(siteID, 0, 0)
	reading.Sites[0].Named = true
	reading.Sites[0].Units[0].Placement, reading.Sites[0].Units[0].Slice = transport.SiteUnitInSlice, reading.Sites[0].Slice
	reading.Sites[0].Live = transport.SiteSliceLimitsRead{MemoryMB: 256, Tasks: 64, CPUPercent: 50}
	reading.Sites[0].Kernel = transport.SiteSliceLimitsRead{MemoryMB: 3072, Tasks: 512, CPUPercent: 200}
	reading.Sites[0].KernelState = transport.SiteSlicePending
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	handler := siteLimitsMux(f)
	admin := round3Card(t, f, handler, authzMatrixAdminID)
	if admin.Applies != siteLimitsPending || admin.Live == nil || admin.Live.MemoryMB != 3072 || admin.Live.Tasks != 512 || admin.MemoryCurrentMB != 0 {
		t.Fatalf("written, not in force: %+v live=%+v", admin.Applies, admin.Live)
	}
	if view := round3Card(t, f, handler, authzMatrixCustomerID); view.Applies != siteLimitsNotYet || view.Live != nil {
		t.Fatalf("the customer: %+v", view)
	}
	reading.Sites[0].KernelState = transport.SiteSliceKernelUnknown
	if view := round3Card(t, f, handler, authzMatrixAdminID); view.Applies != siteLimitsUnknown || view.Live != nil {
		t.Fatalf("an unread cgroup: %+v", view)
	}
	// A program runs in the slice and no kernel state came: not "in force".
	reading.Sites[0].KernelState = ""
	if view := round3Card(t, f, handler, authzMatrixAdminID); view.Applies != siteLimitsUnknown {
		t.Fatalf("no kernel reading: %q", view.Applies)
	}
	reading.Sites[0].KernelState = transport.SiteSliceInForce
	reading.Sites[0].Kernel = reading.Sites[0].Live
	if view := round3Card(t, f, handler, authzMatrixCustomerID); view.Applies != siteLimitsApply || view.Live == nil || view.Live.MemoryMB != 256 {
		t.Fatalf("in force: %+v", view)
	}
	// Only stopped programs (the slice does not run): systemd applies the file
	// when one starts.
	reading.Sites[0].KernelState = ""
	reading.Sites[0].Units[0].Placement = transport.SiteUnitStopped
	if view := round3Card(t, f, handler, authzMatrixAdminID); view.Applies != siteLimitsApply {
		t.Fatalf("stopped programs: %q", view.Applies)
	}
}

// TestAKeptMoveIsOneLineAndCanBeTriedAgain (N2): while the Agent keeps a
// move's record, the site's card says so (the program left and its state),
// Problems now has ONE administrator line opening the site, and "Try again"
// asks the Agent to run the put-back only (no units, no new move); once the
// record is gone the line goes.
func TestAKeptMoveIsOneLineAndCanBeTriedAgain(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	oldRead, oldMove, oldNeeds := readSiteSlicesCall, moveSiteSliceCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { readSiteSlicesCall, moveSiteSliceCall, siteLimitReadNeedsAgent = oldRead, oldMove, oldNeeds })
	siteLimitReadNeedsAgent = false
	oldStart := startSiteLimitRead
	startSiteLimitRead = func(func()) {}
	t.Cleanup(func() { startSiteLimitRead = oldStart })
	unit := "celikapp-" + itoa(siteID) + ".service"
	reading := siteLimitsStatus(siteID, 0, 0)
	reading.Sites[0].MoveInterrupted = &transport.SiteSliceMoveRecovery{At: "2026-11-23T08:00:00Z", Kept: true, Units: map[string]string{unit: transport.SiteMoveFailed}}
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	handler := siteLimitsMux(f)
	admin := round3Card(t, f, handler, authzMatrixAdminID)
	if admin.MoveInterrupted == nil || !admin.MoveInterrupted.Kept || admin.MoveInterrupted.Units[unit] != transport.SiteMoveFailed {
		t.Fatalf("the card: %+v", admin.MoveInterrupted)
	}
	if view := round3Card(t, f, handler, authzMatrixCustomerID); view.MoveInterrupted != nil {
		t.Fatal("the customer reads the administrator's move record")
	}
	count := func() int {
		n := 0
		for _, p := range f.panel.siteLimitProblems(ctx, time.Now()) {
			if p.Kind == "site_move_kept" {
				n++
				if p.Actor != "administrator" || p.Action.To != "/domains/customer.matrix.test" {
					t.Fatalf("line: %+v", p)
				}
			}
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("%d lines for the kept move", n)
	}
	var got *transport.MoveSiteSliceRequest
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		got = req
		return transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{}}, nil
	}
	w := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"retry-0001","recover":true}`)
	wantStatus(t, w, http.StatusOK, "")
	if got == nil || !got.Recover || len(got.Units) != 0 || got.Site.SiteID != 0 {
		t.Fatalf("try again asked: %+v", got)
	}
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"retry-0002","recover":true}`), http.StatusForbidden, "")
	// The record is gone: the card's next reading clears the line.
	reading.Sites[0].MoveInterrupted = &transport.SiteSliceMoveRecovery{At: "2026-11-23T08:05:00Z", Units: map[string]string{unit: transport.SiteMovePutBack}}
	_ = round3Card(t, f, handler, authzMatrixAdminID)
	if n := count(); n != 0 {
		t.Fatalf("%d lines after the put-back", n)
	}
}
