package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestPasswordlessMailViewPlacesSendersOnSites(t *testing.T) {
	report := transport.PasswordlessMailReportResponse{
		Read:         true,
		CoveredSince: "2026-10-10T08:00:00Z",
		LocalSenders: []transport.PasswordlessLocalSender{
			{UID: 0, User: "root", System: true, Messages: 9},
			{UID: 5000, User: "vmail", System: true, Messages: 2},
			{UID: 1001, User: "shop_example", Messages: 3, Last: "2026-10-11T00:00:00Z"},
			{UID: 1002, User: "bob", Messages: 1},
			{UID: 1003, Messages: 1},
		},
		LoopbackSenders: []transport.PasswordlessLoopbackSender{
			{Sender: "Contact@Shop.Example", Messages: 2, Last: "2026-10-12T00:00:00Z"},
			{Sender: "no-reply@panel.example", Messages: 4},
			{Sender: "x@elsewhere.example", Messages: 1},
		},
	}
	domains := []passwordlessDomainRow{{id: 7, name: "shop.example"}, {id: 8, name: "quiet.example"}}
	view := buildPasswordlessMailView(report, domains, "no-reply@panel.example")
	if view.State != "found" || view.WindowDays != 30 || view.CoveredSince != report.CoveredSince {
		t.Fatalf("%+v", view)
	}
	if len(view.Sites) != 1 {
		t.Fatalf("sites %+v", view.Sites)
	}
	shop := view.Sites[0]
	if shop.DomainID != 7 || shop.Messages != 5 || len(shop.Ways) != 2 || shop.Last != "2026-10-12T00:00:00Z" {
		t.Fatalf("shop %+v", shop)
	}
	// root, vmail and the panel's own sender are not listed; the others are.
	if len(view.Others) != 3 || view.Others[0].Label != "bob" || view.Others[1].Label != "uid 1003" ||
		view.Others[2].Label != "x@elsewhere.example" {
		t.Fatalf("others %+v", view.Others)
	}
}

func TestPasswordlessMailViewUnknownAndNone(t *testing.T) {
	unknown := buildPasswordlessMailView(transport.PasswordlessMailReportResponse{Error: "Postfix's log could not be read: x"}, nil, "")
	if unknown.State != "unknown" || unknown.Reason == "" || unknown.Sites == nil || unknown.Others == nil {
		t.Fatalf("%+v", unknown)
	}
	none := buildPasswordlessMailView(transport.PasswordlessMailReportResponse{Read: true,
		LocalSenders: []transport.PasswordlessLocalSender{{UID: 0, User: "root", System: true, Messages: 1}}}, nil, "")
	if none.State != "none" {
		t.Fatalf("%+v", none)
	}
}
