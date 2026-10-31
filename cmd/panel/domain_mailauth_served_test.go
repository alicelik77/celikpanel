package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Final alpha.113 acceptance run 4: the DKIM card said "pending" ("In zone —
// waiting for DNS") while the key signed, the record was in the zone and
// both DNS servers answered it; only the panel host's public lookup could
// not read the name. The card now says what is true: this server's DNS
// serves the record (served), a differing public answer is still pending,
// and an own answer that cannot be read claims nothing new.

func TestMailAuthDKIMStatusSaysServedWhenOnlyThePublicLookupCannotConfirm(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "site.example")
	id := f.domain(t, "site.example", "local", 0, true)
	f.queue(t, id, "site.example", dkimTriggerDomainCreated)
	f.pass(t)
	dkimName := "celik._domainkey.site.example"
	want := dkimRecommended(f.agent.keys["site.example"])
	if got := f.txt(t, dkimName); len(got) != 1 || got[0] != want {
		t.Fatalf("DKIM TXT not in the zone: %q", got)
	}

	originalLive, originalOwn := mailAuthLiveTXT, mailAuthOwnTXT
	t.Cleanup(func() { mailAuthLiveTXT, mailAuthOwnTXT = originalLive, originalOwn })

	type lookup struct {
		value    string
		resolved bool
	}
	for _, tc := range []struct {
		name    string
		live    lookup
		own     []string
		ownOK   bool
		status  string
		askedOf bool // whether this server's DNS is asked for the DKIM name
	}{
		{"public lookup cannot read the name, own DNS serves it", lookup{"", false}, []string{"unrelated", want}, true, "served", true},
		{"own answer unreadable", lookup{"", false}, nil, false, "pending", true},
		{"own DNS answers without the record", lookup{"", false}, []string{"v=DKIM1; k=rsa; p=OTHER"}, true, "pending", true},
		{"public answer differs: still spreading", lookup{"v=DKIM1; k=rsa; p=OLD", true}, []string{want}, true, "pending", false},
		{"public answer matches", lookup{want, true}, []string{want}, true, "ok", false},
	} {
		asked := map[string]bool{}
		mailAuthLiveTXT = func(_ context.Context, name, prefix string) (string, bool) {
			if name == dkimName {
				return tc.live.value, tc.live.resolved
			}
			return "", false
		}
		mailAuthOwnTXT = func(_ context.Context, name string) ([]string, bool) {
			asked[name] = true
			if name == dkimName {
				return tc.own, tc.ownOK
			}
			return nil, false
		}
		recorder := httptest.NewRecorder()
		f.p.handleMailAuth(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/auth", nil), id)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", tc.name, recorder.Code, recorder.Body.String())
		}
		var view mailAuthStatus
		if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.DKIM.Status != tc.status {
			t.Fatalf("%s: dkim.status=%q, want %q (signed=%v)", tc.name, view.DKIM.Status, tc.status, view.DKIMSigned)
		}
		if !view.DKIMSigned {
			t.Fatalf("%s: the key is not reported signed", tc.name)
		}
		if asked[dkimName] != tc.askedOf {
			t.Fatalf("%s: own DNS asked=%v, want %v", tc.name, asked[dkimName], tc.askedOf)
		}
	}
}

// Only a pending record the public lookup could not read is asked of this
// server's DNS; a missing record or a readable public answer is not.
func TestMailAuthServedAsksOnlyForAnUnreadPendingRecord(t *testing.T) {
	record := mailAuthRecord{Name: "celik._domainkey.x.example", Recommended: "v=DKIM1; p=A", Status: "missing"}
	originalOwn := mailAuthOwnTXT
	t.Cleanup(func() { mailAuthOwnTXT = originalOwn })
	mailAuthOwnTXT = func(context.Context, string) ([]string, bool) {
		t.Fatal("own DNS asked for a record that is not pending")
		return nil, false
	}
	markServedMailAuthRecord(context.Background(), &record, func(string) bool { return true })
	record.Status, record.Resolved = "pending", true
	markServedMailAuthRecord(context.Background(), &record, func(string) bool { return true })
	if record.Status != "pending" {
		t.Fatalf("status=%q", record.Status)
	}
}
