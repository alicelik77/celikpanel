package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G27): the subscription's value wins over the plan's and the
// server's; only the administrator changes it; a change is applied at once.
func TestLogRetentionFollowsTheChainAndAppliesAtOnce(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	var sent [][]transport.SiteLogRetentionItem
	savedRun, savedStatus := siteLogRetentionCall, siteLogStatusCall
	t.Cleanup(func() { siteLogRetentionCall, siteLogStatusCall = savedRun, savedStatus })
	siteLogRetentionCall = func(_ *Panel, _ context.Context, items []transport.SiteLogRetentionItem) (transport.SiteLogRetentionResponse, error) {
		sent = append(sent, items)
		return transport.SiteLogRetentionResponse{Removed: map[string]int{}}, nil
	}
	siteLogStatusCall = func(*Panel, context.Context, string) (transport.SiteLogStatusResponse, error) {
		return transport.SiteLogStatusResponse{System: transport.SiteLogSystemRule{Found: true, Frequency: "daily", Rotate: 14}}, nil
	}
	read := func(caller *Caller) logRetentionView {
		recorder := featureRequest(p, domainID, caller, http.MethodGet, "logs/retention", "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %d %s", recorder.Code, recorder.Body.String())
		}
		var view logRetentionView
		_ = json.Unmarshal(recorder.Body.Bytes(), &view)
		return view
	}
	if view := read(owner); view.Source != "system" || view.MayChange || !view.System.Found {
		t.Fatalf("fresh: %+v", view)
	}
	recorder := featureRequest(p, domainID, owner, http.MethodPost, "logs/retention", `{"scope":"server","keep_days":7}`)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer change: %d", recorder.Code)
	}
	admin := &Caller{ID: 1, Role: roleAdmin}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "logs/retention", `{"scope":"server","keep_days":30}`)
	if recorder.Code != http.StatusOK || len(sent) != 1 || sent[0][0].KeepDays != 30 {
		t.Fatalf("server: %d %s %+v", recorder.Code, recorder.Body.String(), sent)
	}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "logs/retention", `{"scope":"subscription","keep_days":5}`)
	if view := read(owner); recorder.Code != http.StatusOK || view.KeepDays != 5 || view.Source != "subscription" || view.ServerDays != 30 {
		t.Fatalf("subscription: %+v", view)
	}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "logs/retention", `{"scope":"subscription","keep_days":0}`)
	if view := read(owner); view.KeepDays != 30 || view.Source != "server" {
		t.Fatalf("cleared: %+v", view)
	}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "logs/retention", `{"scope":"server","keep_days":99999}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("out of range: %d", recorder.Code)
	}
}
