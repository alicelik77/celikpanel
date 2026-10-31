package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The setup answer names this installation. A server wiped and installed
// again is another installation, so a browser start marker from the earlier
// one can be recognised and dropped.
func TestServerSetupAnswerCarriesAStableInstallationIdentity(t *testing.T) {
	first := newServiceOperationTestFixture(t)
	read := func(f serviceOperationTestFixture) string {
		w := httptest.NewRecorder()
		f.panel.handleServerSetup(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPath, "", f.userID))
		var state serverSetupState
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &state) != nil {
			t.Fatalf("setup not readable: %d %s", w.Code, w.Body.String())
		}
		return state.InstallationID
	}
	id := read(first)
	if !serverSetupInstallationPattern.MatchString(id) {
		t.Fatalf("no installation identity: %q", id)
	}
	if again := read(first); again != id {
		t.Fatalf("the identity changed between reads: %q then %q", id, again)
	}
	other := newServiceOperationTestFixture(t)
	if reinstalled := read(other); !serverSetupInstallationPattern.MatchString(reinstalled) || reinstalled == id {
		t.Fatalf("a separate installation shares the identity: %q and %q", id, reinstalled)
	}
}
