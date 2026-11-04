package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.121: what the owner hit after updating Boston and Frankfurt.

// B: "could not be checked" says why, sanitized (D-024).
func TestAnUnansweredInstallPreviewSaysWhyWithoutApt(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{
			State:         "failed",
			Error:         "APT simulation failed: exit status 100: E: Unable to correct problems, you have held broken packages.",
			ErrorCode:     "unmet_dependencies",
			ErrorPackages: []string{"postgresql-common", "libpq5", "bad name;rm", "x:amd64"},
		}, nil
	})
	recorder := installPreviewRequest(panel, http.MethodGet, "?step=postgresql:postgresql-18")
	var answer serviceInstallPreviewAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.State != "unknown" || answer.ReasonCode != "unmet_dependencies" ||
		!reflect.DeepEqual(answer.ReasonPackages, []string{"postgresql-common", "libpq5"}) ||
		answer.Reason != "unmet dependencies: postgresql-common, libpq5" {
		t.Fatalf("answer = %+v", answer)
	}
	if strings.Contains(recorder.Body.String(), "held broken") || strings.Contains(recorder.Body.String(), "APT simulation") {
		t.Fatalf("apt's own text reached the screen: %s", recorder.Body.String())
	}

	// An unknown code is "other"; a repository outside the catalogue is dropped.
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "failed", ErrorCode: "secret_thing", ErrorRepo: "evil"}, nil
	})
	recorder = installPreviewRequest(panel, http.MethodGet, "?step=postgresql:postgresql-18")
	answer = serviceInstallPreviewAnswer{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	if answer.ReasonCode != "other" || answer.ReasonRepo != "" {
		t.Fatalf("answer = %+v", answer)
	}

	// A package the refreshed repository still does not offer names it.
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "failed", ErrorCode: "package_not_found", ErrorPackages: []string{"postgresql-18"}, ErrorRepo: "pgdg", ListsRefreshed: "pgdg"}, nil
	})
	recorder = installPreviewRequest(panel, http.MethodGet, "?step=postgresql:postgresql-18")
	answer = serviceInstallPreviewAnswer{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	if answer.ReasonCode != "package_not_found" || answer.ReasonRepo != "pgdg" || answer.ListsRefreshed != "pgdg" ||
		answer.Reason != "not offered by the package lists: postgresql-18 (pgdg)" {
		t.Fatalf("answer = %+v", answer)
	}

	// The agent did not answer.
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{}, errors.New("dial unix: no such file")
	})
	recorder = installPreviewRequest(panel, http.MethodGet, "?step=postgresql:postgresql-18")
	answer = serviceInstallPreviewAnswer{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	if answer.State != "unknown" || answer.ReasonCode != "agent_unreachable" {
		t.Fatalf("answer = %+v", answer)
	}
}

// E: a busy refusal whose holder is the start-up work after an update says so.
func TestABusyRefusalNamesTheStartUpWorkAfterAnUpdate(t *testing.T) {
	started := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	job := &agentMutationJob{Kind: "mail_tls_sync", Target: "postfix", Status: agentMutationRunning, StartedAt: started.Add(20 * time.Second)}
	if got := serviceOperationBusyReason(job, started, started.Add(time.Minute)); got != serviceOperationBusyReasonStartup {
		t.Fatalf("right after start = %q", got)
	}
	if got := serviceOperationBusyReason(job, started, started.Add(panelStartupBusyWindow+time.Second)); got != "" {
		t.Fatalf("long after start = %q", got)
	}
	older := &agentMutationJob{Kind: "install", Status: agentMutationRunning, StartedAt: started.Add(-10 * time.Minute)}
	if got := serviceOperationBusyReason(older, started, started.Add(time.Minute)); got != "" {
		t.Fatalf("a job older than this start = %q", got)
	}
	recorder := httptest.NewRecorder()
	writeServiceOperationBusyReason(recorder, serviceOperationBusyReasonStartup)
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || body.Code != errCodeServiceOperationBusy || body.Reason != serviceOperationBusyReasonStartup || !strings.Contains(body.Error, "start-up") {
		t.Fatalf("body = %+v", body)
	}
	recorder = httptest.NewRecorder()
	writeServiceOperationBusy(recorder)
	body = apiErrorBody{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Reason != "" || body.Error != "another package operation is already in progress" {
		t.Fatalf("plain busy = %+v", body)
	}
}

type hostnameResolver struct {
	addresses []string
	err       error
}

func (r hostnameResolver) LookupHost(context.Context, string) ([]string, error) {
	return r.addresses, r.err
}

// D: the server's own host name in public DNS, read-only.
func TestTheServersHostNameIsCheckedInPublicDNS(t *testing.T) {
	nx := &net.DNSError{Err: "DNS name does not exist", Name: "frankfurt.example.test", IsNotFound: true}
	for _, c := range []struct {
		name      string
		resolvers []hostResolver
		want      string
	}{
		{"missing", []hostResolver{hostnameResolver{err: nx}, hostnameResolver{err: nx}}, serverHostnameMissing},
		{"resolves", []hostResolver{hostnameResolver{addresses: []string{"192.0.2.10"}}, hostnameResolver{addresses: []string{"192.0.2.10"}}}, serverHostnameResolves},
		{"elsewhere", []hostResolver{hostnameResolver{addresses: []string{"198.51.100.7"}}, hostnameResolver{err: nx}}, serverHostnameOtherAddress},
		{"half", []hostResolver{hostnameResolver{err: nx}, hostnameResolver{err: errors.New("i/o timeout")}}, serverHostnameUnknown},
	} {
		got, _ := classifyServerHostname(context.Background(), "frankfurt.example.test", "192.0.2.10", "", c.resolvers)
		if got != c.want {
			t.Fatalf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestAMissingHostNameInAHostedZoneProposesItsRecord(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupInfrastructureZone(t, p)
	zone, err := p.serverHostnameHostedZone(context.Background(), "web1.example.test", "192.0.2.10", "2001:db8::10")
	if err != nil || zone == nil {
		t.Fatalf("zone = %+v err = %v", zone, err)
	}
	if zone.Name != "example.test" || zone.Kind != "infrastructure" || zone.RecordsPath != "/api/v1/dns/infrastructure-zones/example.test/records" ||
		zone.EditorPath != "/domains/infrastructure/example.test" || len(zone.Missing) != 2 ||
		zone.Missing[0] != (serverHostnameRecord{Name: "web1.example.test", Type: "A", Content: "192.0.2.10", TTL: serverHostnameRecordTTL}) ||
		zone.Missing[1].Type != "AAAA" {
		t.Fatalf("zone = %+v", zone)
	}
	// A name the zone already has is said, never added twice.
	zone, err = p.serverHostnameHostedZone(context.Background(), "frankfurt.example.test", "192.0.2.10", "")
	if err != nil || zone == nil || len(zone.Missing) != 0 || len(zone.Present) != 1 || !strings.HasPrefix(zone.Present[0], "A ") {
		t.Fatalf("present = %+v err = %v", zone, err)
	}
	// A zone this panel does not serve: nothing to offer.
	if zone, err := p.serverHostnameHostedZone(context.Background(), "host.elsewhere.test", "192.0.2.10", ""); err != nil || zone != nil {
		t.Fatalf("elsewhere = %+v err = %v", zone, err)
	}
}

func TestTheHostNameCheckIsCachedAndNeverAsksTwiceAtOnce(t *testing.T) {
	p := newDNSPanelForTest(t)
	restoreName, restoreResolvers, restoreNow := localServerName, setupDNSPublicResolvers, serverHostnameDNSNow
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	asked := 0
	localServerName = func() (string, error) { return "web1.example.test", nil }
	setupDNSPublicResolvers = func() []hostResolver {
		asked++
		return []hostResolver{hostnameResolver{err: &net.DNSError{Err: "no such host", IsNotFound: true}}}
	}
	serverHostnameDNSNow = func() time.Time { return now }
	t.Cleanup(func() {
		localServerName, setupDNSPublicResolvers, serverHostnameDNSNow = restoreName, restoreResolvers, restoreNow
		serverHostnameDNSCache.Lock()
		serverHostnameDNSCache.at, serverHostnameDNSCache.key = time.Time{}, ""
		serverHostnameDNSCache.Unlock()
	})
	first := p.serverHostnameDNS(context.Background(), false)
	if first.State != serverHostnameMissing || first.Hostname != "web1.example.test" {
		t.Fatalf("first = %+v", first)
	}
	p.serverHostnameDNS(context.Background(), false)
	p.serverHostnameDNS(context.Background(), true)
	if asked != 1 {
		t.Fatalf("asked %d times within the cache", asked)
	}
	now = now.Add(serverHostnameDNSRecheck + time.Second)
	p.serverHostnameDNS(context.Background(), true)
	if asked != 2 {
		t.Fatalf("a recheck after %s asked %d times", serverHostnameDNSRecheck, asked)
	}
}
