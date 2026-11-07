package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129 lab (Debian 12): BIND was stopped while setup waited in its
// verification. Setup said "open DNS and peer servers" and that page said
// "server setup is still running; open Server setup". The DNS page now says
// the real reason and the one next action.
func TestDNSOverviewSaysTheStoppedEngineInsteadOfSetupRunning(t *testing.T) {
	in := dnsOverviewInput{
		Draft:         serverSetupDraft{MainDomain: "example.test", DNSMode: "local"},
		Running:       true,
		EngineStopped: transport.DNSEngineBIND,
	}
	got := dnsOverviewSentenceFor(in, dnsOverview{})
	if got.Code != "dns_overview_engine_stopped" || got.State != "failed" || got.Vars["unit"] != "named" || got.Vars["engine"] != "BIND" ||
		!strings.Contains(got.Next, "systemctl start named") || strings.Contains(got.Next, "Server setup") {
		t.Fatalf("stopped engine sentence: %+v", got)
	}
	in.EngineStopped = transport.DNSEnginePowerDNS
	if got := dnsOverviewSentenceFor(in, dnsOverview{}); got.Vars["unit"] != "pdns" {
		t.Fatalf("PowerDNS unit: %+v", got)
	}
	in.EngineStopped = ""
	if got := dnsOverviewSentenceFor(in, dnsOverview{}); got.Code != "dns_overview_setup_running" {
		t.Fatalf("a running engine during setup keeps the setup sentence: %+v", got)
	}
}

func TestDNSEngineEntryStoppedNeedsAVerifiedReading(t *testing.T) {
	stopped := dnsEngineEntry{ID: transport.DNSEngineBIND, Installed: true, Managed: true, Status: "installed_standby"}
	if !dnsEngineEntryStopped(transport.DNSEngineBIND, stopped) {
		t.Fatal("installed, managed, not running is stopped")
	}
	unread := dnsEngineEntry{ID: transport.DNSEngineBIND, Status: "available", DetailCode: "readiness_unavailable"}
	if dnsEngineEntryStopped(transport.DNSEngineBIND, unread) {
		t.Fatal("an unread reading is unknown, never stopped")
	}
	running := stopped
	running.Running = true
	if dnsEngineEntryStopped(transport.DNSEngineBIND, running) || dnsEngineEntryStopped(transport.DNSEnginePowerDNS, stopped) || dnsEngineEntryStopped("", stopped) {
		t.Fatal("running, not the active engine, or no active engine is not stopped")
	}
}
