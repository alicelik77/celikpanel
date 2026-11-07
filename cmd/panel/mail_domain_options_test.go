package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestDomainMailOptionsBuildTheMapAndRefuseBesideACatchAll(t *testing.T) {
	f := newOutgoingFixture(t)
	var sent [][]transport.MailDomainOptionsEntry
	saved := domainOptionsAgent
	t.Cleanup(func() { domainOptionsAgent = saved; domainOptionsApply.hash = "" })
	domainOptionsApply.hash = ""
	domainOptionsAgent = func(_ *Panel, _ context.Context, entries []transport.MailDomainOptionsEntry) (transport.SetMailDomainOptionsResponse, error) {
		sent = append(sent, entries)
		return transport.SetMailDomainOptionsResponse{Applied: true}, nil
	}
	ctx := context.Background()
	// Unused: nothing is sent.
	if _, called, err := f.p.applyDomainMailOptions(ctx, false); called || err != nil {
		t.Fatalf("unused options reached the server: %v %v", called, err)
	}
	if _, err := f.db.Exec(`INSERT INTO email_forwardings (domain_id, source, destination) VALUES (9801, 'sales@example.com', 'a@example.com')`); err != nil {
		t.Fatal(err)
	}
	customer := &Caller{ID: 9602, Role: roleCustomer}
	put := func(domainID int, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.p.handleDomainMailOptions(w, outgoingRequest(http.MethodPut, "/", body, customer), domainID)
		return w
	}
	if w := put(9801, `{"incoming_off":false,"unknown_reply":"No such person here. See example.com/contact"}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if w := put(9802, `{"incoming_off":true,"unknown_reply":""}`); w.Code != http.StatusOK {
		t.Fatalf("save send-only: %d %s", w.Code, w.Body.String())
	}
	last := sent[len(sent)-1]
	var lines []string
	for _, e := range last {
		lines = append(lines, e.Key+" "+e.Action)
	}
	got := strings.Join(lines, "|")
	for _, want := range []string{
		"a@example.com DUNNO", "new@example.com DUNNO", "sales@example.com DUNNO", "postmaster@example.com DUNNO",
		"example.com 550 5.1.1 No such person here. See example.com/contact",
		"example.org " + domainOptionsSendOnlyReply,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("map lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, "b@example.org DUNNO") {
		t.Fatalf("a send-only domain lists addresses: %s", got)
	}
	// Unchanged: nothing sent again.
	n := len(sent)
	if _, called, _ := f.p.applyDomainMailOptions(ctx, false); called || len(sent) != n {
		t.Fatal("an unchanged map was sent again")
	}
	// Accented text and a reply beside a catch-all are refused.
	if w := put(9801, `{"incoming_off":false,"unknown_reply":"Böyle biri yok"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("accented reply: %d", w.Code)
	}
	if _, err := f.db.Exec(`INSERT INTO mail_catch_all (domain_id, destination) VALUES (9801, 'a@example.com')`); err != nil {
		t.Fatal(err)
	}
	if w := put(9801, `{"incoming_off":false,"unknown_reply":"Nobody"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), errCodeMailOptionsCatchAll) {
		t.Fatalf("beside a catch-all: %d %s", w.Code, w.Body.String())
	}
	w := httptest.NewRecorder()
	f.p.handleDomainMailOptions(w, outgoingRequest(http.MethodGet, "/", "", customer), 9801)
	if !strings.Contains(w.Body.String(), `"catch_all":true`) || !strings.Contains(w.Body.String(), `"unknown_reply":"No such person here.`) {
		t.Fatalf("get: %s", w.Body.String())
	}
}
