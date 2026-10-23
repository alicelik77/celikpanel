package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

func (a *serviceOperationTestAgent) InstalledPackageVersionsV1(
	_ *transport.Empty,
	resp *transport.InstalledPackageVersionsResponse,
) error {
	a.mu.Lock()
	hook := a.packageVersionsHook
	a.mu.Unlock()
	if hook != nil {
		hook()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.packageVersionCalls++
	if a.packageVersions == nil {
		*resp = transport.InstalledPackageVersionsResponse{Family: "apt", State: transport.PackageVersionsUnsupported}
		return nil
	}
	*resp = *a.packageVersions
	return nil
}

func TestInstalledPackageVersionsRPCPolicyIsABoundedRead(t *testing.T) {
	policy, err := agentRPCPolicyForMethod("Agent.InstalledPackageVersionsV1")
	if err != nil {
		t.Fatal(err)
	}
	if policy.effect != agentRPCEffectRead || policy.capability != "" || policy.timeout != agentRPCQuickReadTimeout {
		t.Fatalf("InstalledPackageVersionsV1 policy = %+v, want a quick read with no host capability", policy)
	}
	// The read is admitted on every family, including one that may not mutate.
	for _, family := range []string{"apt", "pacman", "dnf", ""} {
		identity := agentRPCHostIdentity{host: core.ManagedServiceHostProfile{PackageFamily: family}}
		if err := authorizeAgentRPCPolicyForHost("Agent.InstalledPackageVersionsV1", policy, identity); err != nil {
			t.Fatalf("family %q: the version read was refused: %v", family, err)
		}
	}
	// No mutation by a similar name is admitted.
	for _, method := range []string{"Agent.InstalledPackageVersions", "Agent.InstallPackageVersionsV1", "Agent.SetPackageVersionsV1"} {
		if _, err := agentRPCPolicyForMethod(method); err == nil {
			t.Fatalf("%s has a policy; only the reviewed read may", method)
		}
	}
}

func TestUpstreamPackageVersion(t *testing.T) {
	for full, want := range map[string]string{
		"1.26.3-3+deb13u1":     "1.26.3",
		"1:10.11.13-0+deb13u1": "10.11.13",
		"3.10.4-1~deb13u1":     "3.10.4",
		"1.24.0-2ubuntu7.5":    "1.24.0",
		"2.3.21.1+dfsg1-3":     "2.3.21.1",
		"1.28.0-1":             "1.28.0",
		"2:1.2.3-1":            "1.2.3",
		"17.6":                 "17.6",
		"17+278":               "17",
		"git20240101-1":        "git20240101-1",
		// Real Debian and Ubuntu strings of phpMyAdmin and friends (alpha.101).
		"4:5.2.2+dfsg-1":                      "5.2.2",
		"1:5.2.1+dfsg-3":                      "5.2.1",
		"5.2.2-really5.2.1-1":                 "5.2.1",
		"2:4.1.0+really4.0.9-2":               "4.0.9",
		"4:5.2.2-really5.2.2+20250121+dfsg-1": "5.2.2",
		"1:2.0~really1.9.3-1":                 "1.9.3",
		// alpha.104: the exact strings of the owner's servers. Debian 13
		// (trixie, apt-cache policy phpmyadmin in a Debian 13 guest and
		// Frankfurt's apt history) has "really" followed by "+dfsg", not by
		// a version; Ubuntu 24.04 (noble, Launchpad) has none.
		"4:5.2.2-really+dfsg-1+deb13u1": "5.2.2",
		"4:5.2.1+dfsg-3":                "5.2.1",
		"7.14.7+dfsg-1":                 "7.14.7",
		"1:1.2-really~bpo1-1":           "1.2",
		"3:7.2":                         "7.2",
		"7:4.4.2-1":                     "4.4.2",
		"1.2.3~rc1-1":                   "1.2.3",
		"2.9.14~dfsg-1ubuntu0.1":        "2.9.14",
		"0~20240101-1":                  "0",
		// "really" that is not a "really" version is left alone.
		"1.0.really-1":  "1.0.really",
		"1.0+reallyx-1": "1.0",
		"really5.2.1-1": "really5.2.1-1",
	} {
		if got := upstreamPackageVersion(full); got != want {
			t.Errorf("upstreamPackageVersion(%q) = %q, want %q", full, got, want)
		}
	}
}

func TestPackageVersionViewKeepsUnknownUnknown(t *testing.T) {
	v := func(pairs ...string) []transport.InstalledPackageVersion {
		out := []transport.InstalledPackageVersion{}
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, transport.InstalledPackageVersion{Package: pairs[i], Version: pairs[i+1]})
		}
		return out
	}
	full, short, differ := packageVersionView(v("nginx", "1.26.3-3+deb13u1"))
	if full != "1.26.3-3+deb13u1" || short != "1.26.3" || differ {
		t.Fatalf("one package: %q %q %v", full, short, differ)
	}
	full, short, differ = packageVersionView(v("dovecot-imapd", "1:2.4.1+dfsg1-6", "dovecot-pop3d", "1:2.4.1+dfsg1-6"))
	if full != "1:2.4.1+dfsg1-6" || short != "2.4.1" || differ {
		t.Fatalf("agreeing packages: %q %q %v", full, short, differ)
	}
	full, short, differ = packageVersionView(v("spamassassin", "4.0.1-1", "spamass-milter", "0.4.0-2"))
	if full != "" || short != "" || !differ {
		t.Fatalf("differing packages must say several versions, not one: %q %q %v", full, short, differ)
	}
	for _, packages := range [][]transport.InstalledPackageVersion{
		nil,
		v("nginx", ""),
		v("nginx", "1.26.3", "nginx-common", ""),
		v("nginx", "1.0;rm -rf"),
		v("nginx", "1."+string(make([]byte, 80))),
	} {
		if full, short, differ := packageVersionView(packages); full != "" || short != "" || differ {
			t.Fatalf("%v: unknown became %q %q %v", packages, full, short, differ)
		}
	}
}

func TestPackageVersionsFromReplyKeepsPreviousOnlyWhenBusy(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	previous := []serviceObservation{{
		ID: "nginx", IsInstalled: true,
		PackageVersions:       []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3+deb13u1"}},
		PackageVersionsReadAt: "2026-10-21T08:00:00Z",
	}}
	read := packageVersionsFromReply(transport.InstalledPackageVersionsResponse{
		Family: "apt", State: transport.PackageVersionsRead,
		Components: []transport.ComponentPackageVersions{
			{ServiceID: "nginx", Packages: []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.4-1"}}},
			{ServiceID: "not-a-component", Packages: []transport.InstalledPackageVersion{{Package: "x", Version: "1"}}},
			{ServiceID: "certbot", Packages: []transport.InstalledPackageVersion{{Package: "certbot", Version: "bad version"}}},
		},
	}, previous, now)
	if got := read["nginx"]; got.Kept || got.ReadAt != "2026-10-21T12:00:00Z" || got.Packages[0].Version != "1.26.4-1" {
		t.Fatalf("read nginx = %+v", got)
	}
	if _, listed := read["not-a-component"]; listed {
		t.Fatal("a component outside the catalogue was kept")
	}
	if got := read["certbot"]; got.Packages[0].Version != "" {
		t.Fatalf("an unreadable version was kept: %+v", got)
	}

	busy := packageVersionsFromReply(transport.InstalledPackageVersionsResponse{State: transport.PackageVersionsBusy}, previous, now)
	if got := busy["nginx"]; !got.Kept || got.ReadAt != "2026-10-21T08:00:00Z" || got.Packages[0].Version != "1.26.3-3+deb13u1" {
		t.Fatalf("busy must keep the previous versions, marked: %+v", got)
	}
	for _, state := range []string{transport.PackageVersionsFailed, transport.PackageVersionsUnsupported, ""} {
		if got := packageVersionsFromReply(transport.InstalledPackageVersionsResponse{State: state, Error: "x"}, previous, now); len(got) != 0 {
			t.Fatalf("state %q kept versions: %+v", state, got)
		}
	}
}

func TestCatalogViewPackageVersionsAndOldCacheRows(t *testing.T) {
	// A row written before alpha.97: no package fields at all.
	old := `{"observations":[{"id":"nginx","is_installed":true,"status":"active (running)","unit":"nginx"}]}`
	decoded, err := decodeScanCacheSnapshot(old)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range catalogView(decoded.Observations, "apt") {
		if service.PackageVersion != "" || service.PackageVersionShort != "" || service.PackageVersionsDiffer ||
			len(service.PackageVersions) != 0 || service.PackageVersionKept {
			t.Fatalf("%s: an old cache row produced a version: %+v", service.ID, service)
		}
	}
	// The legacy array format decodes as unknown too.
	legacy := `[{"id":"nginx","is_installed":true,"status":"active (running)","versions":[]}]`
	if decoded, err := decodeScanCacheSnapshot(legacy); err != nil {
		t.Fatal(err)
	} else if view := catalogView(decoded.Observations, "apt"); serviceByID(view, "nginx").PackageVersion != "" {
		t.Fatal("legacy row produced a version")
	}

	nginx := []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3+deb13u1"}}
	php := []transport.InstalledPackageVersion{{Package: "php8.4-fpm", Version: "8.4.11-1"}}
	view := catalogView([]serviceObservation{
		{ID: "nginx", IsInstalled: true, PackageVersions: nginx, PackageVersionsReadAt: "2026-10-21T08:00:00Z", PackageVersionsKept: true},
		{ID: "php-fpm", IsInstalled: true, PackageVersions: php, PackageVersionsReadAt: "2026-10-21T08:00:00Z"},
		{ID: "certbot", IsInstalled: false, PackageVersions: []transport.InstalledPackageVersion{{Package: "certbot", Version: "2.11.0-1"}}},
	}, "apt")
	got := serviceByID(view, "nginx")
	if got.PackageVersion != "1.26.3-3+deb13u1" || got.PackageVersionShort != "1.26.3" || !got.PackageVersionKept ||
		got.PackageVersionReadAt != "2026-10-21T08:00:00Z" || !reflect.DeepEqual(got.PackageVersions, nginx) {
		t.Fatalf("nginx view = %+v", got)
	}
	if got := serviceByID(view, "php-fpm"); got.PackageVersion != "" || len(got.PackageVersions) != 0 {
		t.Fatalf("a multi-version runtime was given a package version: %+v", got)
	}
	if got := serviceByID(view, "certbot"); got.PackageVersion != "" {
		t.Fatalf("an absent component was given a version: %+v", got)
	}
	if got := serviceByID(view, "postfix"); got.PackageVersion != "" {
		t.Fatalf("an unobserved component was given a version: %+v", got)
	}
}

func serviceByID(services []ManagedServiceResponse, id string) ManagedServiceResponse {
	for _, service := range services {
		if service.ID == id {
			return service
		}
	}
	return ManagedServiceResponse{}
}

// The version travels from the Agent through the scan into the cache and out
// of both managed-services answers; a busy package manager on the next scan
// keeps it, marked; a failed read on the scan after that drops it.
func TestPackageVersionsPassThroughScanCacheAndAnswers(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	f.agent.mu.Lock()
	f.agent.installed["nginx"] = true
	f.agent.active["nginx"] = true
	f.agent.installed["certbot"] = true
	f.agent.packageVersions = &transport.InstalledPackageVersionsResponse{
		Family: "apt", State: transport.PackageVersionsRead,
		Components: []transport.ComponentPackageVersions{
			{ServiceID: "nginx", Packages: []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3+deb13u1"}}},
			{ServiceID: "certbot", Packages: []transport.InstalledPackageVersion{{Package: "certbot", Version: "4.0.0-2"}}},
		},
	}
	f.agent.mu.Unlock()

	services, err := f.panel.scanManagedServices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := serviceByID(services, "nginx"); got.PackageVersion != "1.26.3-3+deb13u1" || got.PackageVersionShort != "1.26.3" || got.PackageVersionKept {
		t.Fatalf("scan answer nginx = %+v", got)
	}
	if f.agent.packageVersionCalls != 1 {
		t.Fatalf("one scan made %d version reads, want 1", f.agent.packageVersionCalls)
	}

	read := func() managedServicesPayload {
		recorder := httptest.NewRecorder()
		f.panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET = %d %s", recorder.Code, recorder.Body.String())
		}
		var payload managedServicesPayload
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	cached := read()
	if got := serviceByID(cached.Services, "certbot"); got.PackageVersion != "4.0.0-2" || got.PackageVersionShort != "4.0.0" {
		t.Fatalf("cached answer certbot = %+v", got)
	}
	firstReadAt := serviceByID(cached.Services, "nginx").PackageVersionReadAt
	if firstReadAt == "" {
		t.Fatal("the read time was not kept")
	}

	f.agent.mu.Lock()
	f.agent.packageVersions = &transport.InstalledPackageVersionsResponse{Family: "apt", State: transport.PackageVersionsBusy}
	f.agent.mu.Unlock()
	if _, err := f.panel.scanManagedServices(context.Background()); err != nil {
		t.Fatal(err)
	}
	kept := serviceByID(read().Services, "nginx")
	if kept.PackageVersion != "1.26.3-3+deb13u1" || !kept.PackageVersionKept || kept.PackageVersionReadAt != firstReadAt {
		t.Fatalf("a busy package manager must keep the previous version, marked: %+v", kept)
	}

	f.agent.mu.Lock()
	f.agent.packageVersions = &transport.InstalledPackageVersionsResponse{Family: "apt", State: transport.PackageVersionsFailed, Error: "the package database query failed"}
	f.agent.mu.Unlock()
	if _, err := f.panel.scanManagedServices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := serviceByID(read().Services, "nginx"); got.PackageVersion != "" || got.PackageVersionKept {
		t.Fatalf("a failed read must leave the version unknown: %+v", got)
	}
}

// The page's own scan reads the versions before it takes the Panel's mutation
// lease: the read is read-only and takes no lock, so the lease is not held
// while the package database answers (alpha.97 review).
func TestPageScanReadsPackageVersionsOutsideTheMutationLease(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	leaseFree := false
	f.agent.mu.Lock()
	f.agent.installed["nginx"] = true
	f.agent.packageVersions = &transport.InstalledPackageVersionsResponse{Family: "apt", State: transport.PackageVersionsRead,
		Components: []transport.ComponentPackageVersions{{ServiceID: "nginx", Packages: []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3"}}}}}
	f.agent.packageVersionsHook = func() {
		if f.panel.serviceMutationMu.TryLock() {
			f.panel.serviceMutationMu.Unlock()
			leaseFree = true
		}
	}
	f.agent.mu.Unlock()
	recorder := httptest.NewRecorder()
	f.panel.handleManagedServicesScan(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/managed-services/scan", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("scan = %d %s", recorder.Code, recorder.Body.String())
	}
	if !leaseFree {
		t.Fatal("the mutation lease was held while the package database was asked")
	}
	if f.agent.packageVersionCalls != 1 {
		t.Fatalf("one page scan made %d version reads, want 1", f.agent.packageVersionCalls)
	}
	var payload managedServicesPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := serviceByID(payload.Services, "nginx"); got.PackageVersion != "1.26.3-3" {
		t.Fatalf("nginx = %+v", got)
	}
}

// A reading kept for more than a day says so; a fresh keep does not.
func TestKeptReadingOlderThanADayIsMarkedLong(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	nginx := []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3"}}
	for readAt, want := range map[string]bool{"2026-10-21T08:00:00Z": false, "2026-10-20T11:00:00Z": true} {
		kept := packageVersionsFromReply(transport.InstalledPackageVersionsResponse{State: transport.PackageVersionsBusy},
			[]serviceObservation{{ID: "nginx", IsInstalled: true, PackageVersions: nginx, PackageVersionsReadAt: readAt}}, now)
		if got := kept["nginx"]; !got.Kept || got.KeptLong != want {
			t.Fatalf("read at %s: %+v, want kept long %v", readAt, got, want)
		}
		view := catalogView([]serviceObservation{{ID: "nginx", IsInstalled: true, PackageVersions: nginx, PackageVersionsReadAt: readAt,
			PackageVersionsKept: true, PackageVersionsKeptLong: want}}, "apt")
		if got := serviceByID(view, "nginx"); got.PackageVersionKeptLong != want {
			t.Fatalf("read at %s: view %+v", readAt, got)
		}
	}
}

// alpha.97 -> alpha.96 (a downgrade without the pre-update snapshot): the
// additive fields stay in the cache row. alpha.96 has two decoders of that row
// and they disagree, so both are mirrored here with alpha.96's observation
// shape: the Components page's (decodeScanCacheSnapshot, lenient
// encoding/json) reads it; the Store's (strictScanObservations,
// DisallowUnknownFields) refuses it, so alpha.96's Store says
// component_state_invalid until alpha.96's next scan writes the row again
// without the fields. alpha.97's own strict Store decoder accepts the row.
func TestAlpha96DecodersAndACacheRowWrittenByAlpha97(t *testing.T) {
	type alpha96Observation struct {
		ID                    string                 `json:"id"`
		IsInstalled           bool                   `json:"is_installed"`
		Status                string                 `json:"status"`
		Unit                  string                 `json:"unit,omitempty"`
		ConfigFiles           []core.ConfigFile      `json:"config_files,omitempty"`
		Instances             []core.ServiceInstance `json:"instances,omitempty"`
		Versions              []string               `json:"versions,omitempty"`
		InstalledRepoPackages []string               `json:"installed_repo_packages,omitempty"`
	}
	row, err := json.Marshal(scanCacheDoc{Observations: []serviceObservation{{
		ID: "nginx", IsInstalled: true, Status: "active (running)",
		PackageVersions:       []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3"}},
		PackageVersionsReadAt: "2026-10-21T08:00:00Z", PackageVersionsKept: true, PackageVersionsKeptLong: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Observations json.RawMessage `json:"observations"`
	}
	if err := json.Unmarshal(row, &envelope); err != nil {
		t.Fatal(err)
	}
	var old []alpha96Observation
	if err := json.Unmarshal(envelope.Observations, &old); err != nil {
		t.Fatalf("alpha.96 refuses an alpha.97 row: %v", err)
	}
	if len(old) != 1 || old[0].ID != "nginx" || !old[0].IsInstalled {
		t.Fatalf("decoded %+v", old)
	}

	// alpha.96's Store decoder (08faa91e5 cmd/panel/store_backend.go,
	// strictScanObservations): one document, unknown fields refused.
	var strict struct {
		Observations []alpha96Observation `json:"observations"`
		WebmailReady *bool                `json:"webmail_ready,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(row))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strict); err == nil {
		t.Fatal("alpha.96's strict Store decoder accepted an alpha.97 row; the documented component_state_invalid gap would not exist")
	}
	// alpha.97's own strict Store decoder knows the fields.
	if observations, err := strictScanObservations(string(row)); err != nil || len(observations) != 1 {
		t.Fatalf("alpha.97's Store decoder refused its own row: %v", err)
	}
}
