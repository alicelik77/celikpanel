package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The first-login tour's state (alpha.119, migration 062): every signed-in
// person reads and writes only their own row, through the real middleware
// chain; no row is "pending" at version 0; a "Log in as" session cannot use
// up the person's tour; malformed bodies change nothing.
// İlk giriş turu durumu: herkes yalnız kendi satırını okur ve yazar.

type guideStateFixture struct {
	authzMatrixFixture
	handler http.Handler
}

func newGuideStateFixture(t *testing.T) *guideStateFixture {
	t.Helper()
	base := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &base)
	mux := http.NewServeMux()
	mux.HandleFunc(guideStatePath, base.panel.handleGuideState)
	return &guideStateFixture{authzMatrixFixture: base, handler: csrfProtect(base.panel.requireAuth(mux))}
}

func (f *guideStateFixture) send(t *testing.T, token, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, guideStatePath, strings.NewReader(body))
	request.Host = "panel.example.test"
	request.Header.Set("Origin", "https://panel.example.test")
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeGuideStateView(t *testing.T, recorder *httptest.ResponseRecorder) guideStateView {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var view guideStateView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return view
}

func TestGuideStateReadsPendingWithoutARowAndKeepsEachPersonsOwn(t *testing.T) {
	f := newGuideStateFixture(t)
	for _, id := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, authzMatrixAdditionalUserID} {
		view := decodeGuideStateView(t, f.send(t, f.tokens[id], http.MethodGet, ""))
		if view != (guideStateView{TourVersion: 0, TourState: "pending"}) {
			t.Fatalf("user %d without a row read %+v", id, view)
		}
	}

	saved := decodeGuideStateView(t, f.send(t, f.tokens[authzMatrixCustomerID], http.MethodPut, `{"tour_version":1,"tour_state":"completed"}`))
	if saved != (guideStateView{TourVersion: 1, TourState: "completed"}) {
		t.Fatalf("PUT answered %+v", saved)
	}
	// A team member writes their own row, not their customer's.
	decodeGuideStateView(t, f.send(t, f.tokens[authzMatrixAdditionalUserID], http.MethodPut, `{"tour_version":1,"tour_state":"skipped"}`))

	want := map[int]guideStateView{
		authzMatrixCustomerID:       {TourVersion: 1, TourState: "completed"},
		authzMatrixAdditionalUserID: {TourVersion: 1, TourState: "skipped"},
		authzMatrixResellerID:       {TourVersion: 0, TourState: "pending"},
		authzMatrixAdminID:          {TourVersion: 0, TourState: "pending"},
	}
	for id, expected := range want {
		if got := decodeGuideStateView(t, f.send(t, f.tokens[id], http.MethodGet, "")); got != expected {
			t.Fatalf("user %d reads %+v, want %+v", id, got, expected)
		}
	}

	// A later version replaces the row in place.
	again := decodeGuideStateView(t, f.send(t, f.tokens[authzMatrixCustomerID], http.MethodPut, `{"tour_version":2,"tour_state":"skipped"}`))
	if again != (guideStateView{TourVersion: 2, TourState: "skipped"}) {
		t.Fatalf("second PUT answered %+v", again)
	}
	var rows int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM user_guide_state`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("rows = %d (%v), want 2", rows, err)
	}
}

func TestGuideStateRefusesMalformedBodiesAndChangesNothing(t *testing.T) {
	f := newGuideStateFixture(t)
	token := f.tokens[authzMatrixCustomerID]
	for _, body := range []string{
		``,
		`{}`,
		`{"tour_version":1}`,
		`{"tour_version":1,"tour_state":"done"}`,
		`{"tour_version":-1,"tour_state":"skipped"}`,
		`{"tour_version":1001,"tour_state":"skipped"}`,
		`{"tour_version":1,"tour_state":"skipped","user_id":9701}`,
		`{"tour_version":1,"tour_state":"skipped"} {"tour_version":2,"tour_state":"skipped"}`,
		`{"tour_version":"1","tour_state":"skipped"}`,
	} {
		if recorder := f.send(t, token, http.MethodPut, body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status %d, want 400", body, recorder.Code)
		}
	}
	if view := decodeGuideStateView(t, f.send(t, token, http.MethodGet, "")); view.TourState != "pending" {
		t.Fatalf("a refused body changed the state: %+v", view)
	}
	if recorder := f.send(t, token, http.MethodPost, `{"tour_version":1,"tour_state":"skipped"}`); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: status %d, want 405", recorder.Code)
	}
}

func TestGuideStateNeedsASessionAndIsNotUsedUpByLogInAs(t *testing.T) {
	f := newGuideStateFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		if recorder := f.send(t, "", method, `{"tour_version":1,"tour_state":"skipped"}`); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: status %d, want 401", method, recorder.Code)
		}
	}
	token, err := f.panel.sessions.CreateImpersonation(context.Background(), authzMatrixCustomerID, authzMatrixAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if view := decodeGuideStateView(t, f.send(t, token, http.MethodGet, "")); view.TourState != "pending" {
		t.Fatalf("impersonated read %+v", view)
	}
	if recorder := f.send(t, token, http.MethodPut, `{"tour_version":1,"tour_state":"skipped"}`); recorder.Code != http.StatusConflict {
		t.Fatalf("impersonated PUT: status %d, want 409", recorder.Code)
	}
	if view := decodeGuideStateView(t, f.send(t, f.tokens[authzMatrixCustomerID], http.MethodGet, "")); view.TourState != "pending" {
		t.Fatalf("Log in as used up the tour: %+v", view)
	}
}

func TestGuideStateRowGoesWithItsUser(t *testing.T) {
	f := newGuideStateFixture(t)
	db := f.panel.db.GetDB()
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, email, role, parent_id, status)
		VALUES (9799, 'guide-leaver', 'x', 'guide-leaver@example.test', 'customer', NULL, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_guide_state (user_id, tour_version, tour_state) VALUES (9799, 1, 'completed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = 9799`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_guide_state WHERE user_id = 9799`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("the deleted user's row stayed: %d (%v)", rows, err)
	}
}
