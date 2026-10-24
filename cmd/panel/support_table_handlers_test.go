package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.107 (D-038 §11, §5, §14): every signed-in person reads the table's
// statuses for this server; a team member is not told the server's release;
// a site's PHP change to a version without security fixes needs the
// person's confirmation.

func withSupportServer(t *testing.T, osRelease string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(osRelease), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath, oldNow := osReleasePath, supportNow
	osReleasePath = path
	supportNow = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { osReleasePath, supportNow = oldPath, oldNow })
}

func TestTheSupportTableIsTheServersAnswerForEverySignedInPerson(t *testing.T) {
	withSupportServer(t, "ID=ubuntu\nVERSION_ID=\"22.04\"\nVERSION_CODENAME=jammy\n")
	panel := &Panel{}
	read := func(caller *Caller) supportTableView {
		request := httptest.NewRequest(http.MethodGet, supportTablePath, nil)
		request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
		recorder := httptest.NewRecorder()
		panel.handleSupportTable(recorder, request)
		var view supportTableView
		if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
			t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
		}
		return view
	}
	customer := read(&Caller{ID: 2, Role: roleCustomer})
	if customer.Distro != "ubuntu-22.04" || customer.Statuses["php"]["8.1"].Status != "supported" || customer.Statuses["php"]["8.1"].Provider != "distribution" {
		t.Fatalf("customer = %+v", customer.Statuses["php"]["8.1"])
	}
	if customer.Statuses["node"]["20"].Status != "unsupported" || customer.Checked == "" {
		t.Fatalf("node 20 = %+v", customer.Statuses["node"]["20"])
	}
	team := read(&Caller{ID: 3, Role: core.EffectiveRoleAdditionalUser, AccountType: core.AccountTypeAdditionalUser, CustomerID: 2})
	if team.Distro != "" || team.DistroName != "" || team.Statuses["php"]["8.1"].Status != "supported" || team.Statuses["php"]["8.1"].Note != "" {
		t.Fatalf("team member = %+v / %+v", team.Distro, team.Statuses["php"]["8.1"])
	}
}

func TestASitesPHPChangeToAnUnsupportedVersionNeedsConfirmation(t *testing.T) {
	withSupportServer(t, "ID=debian\nVERSION_ID=\"13\"\nVERSION_CODENAME=trixie\n")
	if support, refuse := phpChangeSupport("8.4", "8.1", false); !refuse || support.Status != "unsupported" {
		t.Fatalf("unconfirmed 8.1: %+v %v", support, refuse)
	}
	if _, refuse := phpChangeSupport("8.4", "8.1", true); refuse {
		t.Fatal("a confirmed choice was refused")
	}
	if _, refuse := phpChangeSupport("8.1", "8.1", false); refuse {
		t.Fatal("keeping the current version asked for confirmation")
	}
	if _, refuse := phpChangeSupport("8.4", "8.3", false); refuse {
		t.Fatal("a supported version asked for confirmation")
	}
}
