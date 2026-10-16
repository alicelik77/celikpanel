package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Gap 4 of the second server as a full peer (2026-10-16): the primary's plan
// can carry the other server's mail host name and prepares it as an A record
// to the peer address. Each refusal is typed; the controls show that a valid
// name is accepted and an empty one adds nothing.
func TestServerSetupInfrastructureDNSPreparesThePeerMailHost(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := infrastructureDNSTestDraft()
	d.InfrastructureDNS.PeerMailHostname = "mail.boston.example.test"
	plan, err := p.buildServerSetupInfrastructureDNSPlan(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range plan.Records {
		if r.Name == "mail.boston.example.test" {
			found++
			if r.Type != "A" || r.Content != d.PeerIP || r.Action != "add" {
				t.Fatalf("peer mail record=%+v", r)
			}
		}
	}
	if found != 1 {
		t.Fatalf("peer mail host prepared %d times", found)
	}
	// Control: without the field no record names the peer's mail host.
	d.InfrastructureDNS.PeerMailHostname = ""
	plan, err = p.buildServerSetupInfrastructureDNSPlan(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range plan.Records {
		if r.Name == "mail.boston.example.test" {
			t.Fatalf("an empty field prepared %+v", r)
		}
	}
}

func TestServerSetupInfrastructureDNSRefusesAPeerMailHostThatIsNotThePeers(t *testing.T) {
	for name, scenario := range map[string]struct {
		host string
		code string
	}{
		"panel":   {"frankfurt.example.test", "infrastructure_dns_peer_mail_own_name"},
		"apex":    {"example.test", "infrastructure_dns_peer_mail_own_name"},
		"ns":      {"ns1.example.test", "infrastructure_dns_peer_mail_own_name"},
		"own":     {"mail.frankfurt.example.test", "infrastructure_dns_peer_mail_own_name"},
		"outside": {"mail.boston.other.test", "infrastructure_dns_peer_mail_outside_zone"},
	} {
		t.Run(name, func(t *testing.T) {
			p := newDNSPanelForTest(t)
			d := infrastructureDNSTestDraft()
			d.Purpose = "web_mail"
			d.MailHostname = "mail.frankfurt.example.test"
			d.InfrastructureDNS.PeerMailHostname = scenario.host
			_, err := p.buildServerSetupInfrastructureDNSPlan(context.Background(), d)
			var review *serverSetupInfrastructureDNSPlanError
			if !errors.As(err, &review) || review.Code != scenario.code {
				t.Fatalf("host %s: err=%v", scenario.host, err)
			}
		})
	}
}

func TestServerSetupDraftCanonicalizesThePeerMailHost(t *testing.T) {
	d := defaultServerSetupDraft()
	d.InfrastructureDNS = &serverSetupInfrastructureDNSDraft{Zone: " Example.Test ", PeerMailHostname: " Mail.Boston.Example.Test. "}
	got, err := canonicalServerSetupDraft(d)
	if err != nil {
		t.Fatal(err)
	}
	if got.InfrastructureDNS.PeerMailHostname != "mail.boston.example.test" {
		t.Fatalf("canonical=%q", got.InfrastructureDNS.PeerMailHostname)
	}
	d.InfrastructureDNS.PeerMailHostname = "not a host"
	if _, err := canonicalServerSetupDraft(d); err == nil {
		t.Fatal("an invalid peer mail host name was accepted")
	}
	// An older plan without the field keeps its JSON (and so its identity).
	raw, _ := json.Marshal(serverSetupInfrastructureDNSDraft{Zone: "example.test"})
	if string(raw) != `{"zone":"example.test"}` {
		t.Fatalf("older identity changed: %s", raw)
	}
}
