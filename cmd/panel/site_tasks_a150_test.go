package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// TestOnePressGivesTheProgramsOnlyWhatIsLeft (alpha.150: one press, one
// deadline): the programs part of a press gets what the site's scheduled
// tasks left of the press's 11 minutes, never more; when nothing (less than
// a second) is left it is not started and the press says the time ran out,
// naming each program for the next press.
func TestOnePressGivesTheProgramsOnlyWhatIsLeft(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	oldMove, oldSlice, oldNow := moveSiteTasksCall, moveSiteSliceCall, siteMovePressNow
	t.Cleanup(func() { moveSiteTasksCall, moveSiteSliceCall, siteMovePressNow = oldMove, oldSlice, oldNow })
	clock := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	siteMovePressNow = func() time.Time { return clock }
	var tasksTook time.Duration
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		clock = clock.Add(tasksTook)
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}, nil
	}
	var budgets []int
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		budgets = append(budgets, req.BudgetSeconds)
		return transport.MoveSiteSliceResponse{Done: true}, nil
	}
	handler := siteLimitsMux(f)
	for i, c := range []struct {
		took   time.Duration
		budget int // 0: the programs part is not started
	}{
		{0, 660},
		{4*time.Minute + 500*time.Millisecond, 419},
		{10*time.Minute + 59*time.Second, 1},
		{10*time.Minute + 59*time.Second + 500*time.Millisecond, 0},
		{11 * time.Minute, 0},
		{20 * time.Minute, 0},
	} {
		tasksTook, budgets = c.took, nil
		move := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"press-00`+itoa(i)+`"}`)
		wantStatus(t, move, http.StatusOK, "")
		var resp transport.MoveSiteSliceResponse
		if err := json.Unmarshal(move.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if c.budget == 0 {
			if len(budgets) != 0 {
				t.Fatalf("%v: the programs part started with nothing left (%v)", c.took, budgets)
			}
			if resp.Done || !resp.TimeUp || resp.Reason == nil || resp.Reason.Code != "time_up" || len(resp.Units) == 0 {
				t.Fatalf("%v: %s", c.took, move.Body.String())
			}
			for _, u := range resp.Units {
				if u.Outcome != transport.SiteMoveNotTried {
					t.Fatalf("%v: %+v", c.took, u)
				}
			}
			continue
		}
		if len(budgets) != 1 || budgets[0] != c.budget {
			t.Fatalf("%v: the programs part got %v seconds, want %d", c.took, budgets, c.budget)
		}
	}
}

// TestTheUpdatesRefusalNamesATasksChange (R2b fix round): while a tasks
// move, undo or rebuild holds the move's claim, the update's refusal says a
// site is being moved or its scheduled tasks are being changed, not
// "programs".
func TestTheUpdatesRefusalNamesATasksChange(t *testing.T) {
	writer := httptest.NewRecorder()
	writePanelUpdateSiteMoveRunning(writer)
	if body := writer.Body.String(); !strings.Contains(body, "its scheduled tasks are being changed") || strings.Contains(body, "programs") {
		t.Fatalf("refusal %s", body)
	}
}
