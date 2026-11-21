package main

import (
	"net/http"
	"strings"
	"testing"
)

// Security release (20 November 2026): Add domain read another account's
// subscription (whether a parent domain exists there and its state) before
// it checked that the caller may use that subscription. A subscription the
// caller may not use is refused first, the same whatever it holds.
func TestNameRuleParentLookupWaitsForTheAccessCheck(t *testing.T) {
	p, agent := nameRuleFixture(t)
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "victim.test"); w.Code != http.StatusOK {
		t.Fatalf("alice: %d %s", w.Code, w.Body.String())
	}
	if _, err := p.db.GetDB().Exec(`UPDATE domains SET status = 'suspended' WHERE name = 'victim.test'`); err != nil {
		t.Fatal(err)
	}
	held := nameRuleCreate(t, p, nameRuleBob, 2, "shop.victim.test")
	free := nameRuleCreate(t, p, nameRuleBob, 2, "unrelated.test")
	for _, w := range []struct {
		name string
		code int
		body string
	}{{"under alice's suspended domain", held.Code, held.Body.String()}, {"unrelated", free.Code, free.Body.String()}} {
		if w.code != http.StatusForbidden || !strings.Contains(w.body, "subscription not found or not permitted") {
			t.Fatalf("bob into alice's subscription, %s: %d %s", w.name, w.code, w.body)
		}
	}
	if got := strings.Join(agent.createdNames(), ","); got != "victim.test" {
		t.Fatalf("site creations: %s", got)
	}
}
