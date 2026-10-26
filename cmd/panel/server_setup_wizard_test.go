package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Setup wizard v2 (alpha.111): the catalogue-generated component list, the
// wizard's additive draft answers, version qualifiers, the short-name
// suggestion and the read-only provider-records check.

func catalogChoice(t *testing.T, catalog serverSetupComponentCatalog, id string) serverSetupComponentChoice {
	t.Helper()
	for _, choice := range catalog.Components {
		if choice.ID == id {
			return choice
		}
	}
	t.Fatalf("catalogue component %s missing from the wizard list", id)
	return serverSetupComponentChoice{}
}

func TestServerSetupCatalogListsEveryCatalogueComponent(t *testing.T) {
	host := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroFamily: "debian", ServiceManager: "systemd", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
	catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InventoryReady: true, DNSState: &dnsEngineDBState{}})
	if len(catalog.Components) != len(core.ManagedServices) {
		t.Fatalf("wizard lists %d components, catalogue has %d", len(catalog.Components), len(core.ManagedServices))
	}
	for index, service := range core.ManagedServices {
		if catalog.Components[index].ID != service.ID {
			t.Fatalf("catalogue order lost at %d: %s != %s", index, catalog.Components[index].ID, service.ID)
		}
	}
	want := map[string]string{
		"nginx": "choice", "php-fpm": "choice", "node": "choice", "mariadb": "choice", "postgresql": "choice",
		"postfix": "choice", "roundcube": "choice", "clamav": "components", "wireguard": "choice", "fail2ban": "choice",
		"nftables": "required", "certbot": "required", core.NativeCronServiceID: "automatic", "bind": "dns", "pdns": "dns",
		"apache": "unavailable", "exim": "unavailable", "vsftpd": "unavailable", "netdata": "components", "spamassassin": "components",
	}
	for id, role := range want {
		choice := catalogChoice(t, catalog, id)
		if choice.Wizard != role {
			t.Fatalf("%s: wizard role %q, want %q", id, choice.Wizard, role)
		}
		if role != "choice" && role != "required" && (choice.Supported || choice.Reason == "") {
			t.Fatalf("%s is not a wizard choice but has no reason: %+v", id, choice)
		}
		if role == "choice" && !slices.Contains(serverSetupSelectableComponents, id) {
			t.Fatalf("%s listed as a choice but refused by the draft", id)
		}
	}
	reasons := map[string]string{
		"apache": "server_setup_service_integration:apache", "netdata": "server_setup_repository_required:netdata",
		"spamassassin": "server_setup_component_after_setup:spamassassin", "bind": "server_setup_component_dns_step",
		core.NativeCronServiceID: "server_setup_component_automatic",
	}
	for id, reason := range reasons {
		if got := catalogChoice(t, catalog, id).Reason; got != reason {
			t.Fatalf("%s reason %q, want %q", id, got, reason)
		}
	}
	// Every listed dependency is itself a reviewed wizard component.
	for _, choice := range catalog.Components {
		for _, dependency := range choice.Dependencies {
			if !slices.Contains(serverSetupSelectableComponents, dependency) && !slices.Contains(serverSetupRequiredComponents, dependency) {
				t.Fatalf("%s depends on unreviewed %s", choice.ID, dependency)
			}
		}
	}
	// A component added to the catalogue appears in the wizard by itself.
	saved := core.ManagedServices
	t.Cleanup(func() { core.ManagedServices = saved })
	core.ManagedServices = append(append([]core.ManagedService{}, saved...), core.ManagedService{ID: "future-tool", Name: "Future tool", Category: "web", Kind: core.KindTool})
	if catalogChoice(t, buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InventoryReady: true}), "future-tool").Wizard != "choice" {
		t.Fatal("a new catalogue component did not reach the wizard")
	}
}

func TestServerSetupCatalogVersionsInstalledAndLockedRepositories(t *testing.T) {
	host := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
	catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{
		Host: host, InventoryReady: true, InstalledIDs: []string{"nginx"},
		InstalledVersions: map[string]string{"nginx": "1.26.3"},
		CandidateVersions: map[string]string{"php-fpm": "8.4", "postgresql": "17", "nginx": "9.9"},
		Repos: map[string]serverSetupRepoObservation{
			"sury": {State: "enabled", Packages: []string{"php8.5-fpm", "php8.3-fpm", "php-common", "php8.3-fpm"}},
			"pgdg": {State: "disabled"},
		},
		DNSState: &dnsEngineDBState{},
	})
	nginx := catalogChoice(t, catalog, "nginx")
	if !nginx.Installed || nginx.InstalledVersion != "1.26.3" || nginx.CandidateVersion != "" || len(nginx.Versions) != 0 {
		t.Fatalf("installed component must show its installed version only: %+v", nginx)
	}
	php := catalogChoice(t, catalog, "php-fpm")
	if php.Repo == nil || php.Repo.State != "enabled" || php.CandidateVersion != "8.4" {
		t.Fatalf("php repository/candidate: %+v", php)
	}
	packages := []string{}
	for _, version := range php.Versions {
		packages = append(packages, version.Source+":"+version.Package+":"+version.Version)
	}
	if !slices.Equal(packages, []string{"distribution::8.4", "vendor:php8.3-fpm:8.3", "vendor:php8.5-fpm:8.5"}) {
		t.Fatalf("php versions: %v", packages)
	}
	postgres := catalogChoice(t, catalog, "postgresql")
	if postgres.Repo == nil || postgres.Repo.State != "disabled" || len(postgres.Versions) != 2 || postgres.Versions[1].Available || postgres.Versions[1].Reason != "repository_disabled" || postgres.Versions[1].Package != "" {
		t.Fatalf("a disabled vendor repository must lock its versions without naming a package: %+v", postgres.Versions)
	}
	unknown := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InventoryReady: true})
	if reason := catalogChoice(t, unknown, "php-fpm").Versions[1].Reason; reason != "repository_unknown" {
		t.Fatalf("an unread repository is unknown, not disabled: %s", reason)
	}
	unread := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InventoryReady: false, InstalledIDs: []string{"nginx"}, InstalledVersions: map[string]string{"nginx": "1.26.3"}})
	for _, choice := range unread.Components {
		if choice.Installed || choice.InstalledVersion != "" || choice.Supported {
			t.Fatalf("unverified inventory produced a claim: %+v", choice)
		}
	}
}

func TestServerSetupCatalogDNSEnginesFollowReviewRules(t *testing.T) {
	debian := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
	ubuntu := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "ubuntu", VersionID: "24.04", Architecture: "amd64"}
	engine := func(catalog serverSetupComponentCatalog, id, role string) serverSetupDNSEngineChoice {
		for _, choice := range catalog.DNSEngines {
			if choice.ID == id && choice.Role == role {
				return choice
			}
		}
		t.Fatalf("missing %s/%s", id, role)
		return serverSetupDNSEngineChoice{}
	}
	fresh := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true, DNSState: &dnsEngineDBState{}})
	for _, role := range []string{"main", "backup"} {
		for _, id := range []string{"bind", "pdns"} {
			if !engine(fresh, id, role).Offered {
				t.Fatalf("%s %s refused on a fresh Debian 13: %+v", id, role, engine(fresh, id, role))
			}
		}
	}
	other := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: ubuntu, InventoryReady: true, DNSState: &dnsEngineDBState{}})
	if choice := engine(other, "pdns", "main"); choice.Offered || choice.Reason != "server_setup_pdns_main_untested_system" {
		t.Fatalf("PowerDNS main outside the measured system: %+v", choice)
	}
	if !engine(other, "pdns", "backup").Offered || !engine(other, "bind", "main").Offered {
		t.Fatal("backup PowerDNS or BIND refused on Ubuntu")
	}
	bindActive := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true, InstalledIDs: []string{"bind"}, InstalledVersions: map[string]string{"bind": "9.20"}, DNSState: &dnsEngineDBState{ActiveEngine: transport.DNSEngineBIND, EngineEpoch: 1}})
	for _, role := range []string{"main", "backup"} {
		if choice := engine(bindActive, "pdns", role); choice.Offered || choice.Reason != "server_setup_existing_dns_requires_migration" {
			t.Fatalf("a running BIND must never be switched by setup: %+v", choice)
		}
	}
	if choice := engine(bindActive, "bind", "main"); !choice.Offered || choice.InstalledVersion != "9.20" {
		t.Fatalf("the running BIND: %+v", choice)
	}
	previous := pdnsPairedPrimaryGateOpen
	pdnsPairedPrimaryGateOpen = false
	t.Cleanup(func() { pdnsPairedPrimaryGateOpen = previous })
	closed := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true, DNSState: &dnsEngineDBState{}})
	if choice := engine(closed, "pdns", "main"); choice.Offered || choice.Reason != "pdns_primary_switch_paused" {
		t.Fatalf("closed product gate: %+v", choice)
	}
	unknown := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true})
	if engine(unknown, "bind", "main").Offered {
		t.Fatal("unread DNS state offered an engine")
	}
}

func TestServerSetupCatalogEndpointIsReadOnly(t *testing.T) {
	f, _ := setupOperationFixture(t)
	attachSetupComponentsAgent(t, f)
	seedInstalledServices(f.agent, "nginx")
	w := httptest.NewRecorder()
	f.panel.handleServerSetupComponents(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPath+"/components", "", f.userID))
	var catalog serverSetupComponentCatalog
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Components) != len(core.ManagedServices) || len(catalog.DNSEngines) != 4 {
		t.Fatalf("catalog: %d %s", w.Code, w.Body.String())
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("catalogue read mutated the host")
	}
}

func TestServerSetupDraftWizardAnswersCanonical(t *testing.T) {
	base := serverSetupDraft{Purpose: "web", DNSMode: "local", DNSRole: "primary", MainDomain: " Example.TEST ", ShortName: " Frankfurt ", ServerCount: 2, MainDNSServer: "this",
		InfrastructureDNS: &serverSetupInfrastructureDNSDraft{Zone: "other.test", PeerPanelDomain: "boston.example.test"}}
	got, err := canonicalServerSetupDraft(base)
	if err != nil || got.MainDomain != "example.test" || got.ShortName != "frankfurt" || got.InfrastructureDNS == nil || got.InfrastructureDNS.Zone != "example.test" || got.InfrastructureDNS.PeerPanelDomain != "boston.example.test" {
		t.Fatalf("main domain must be the zone of the local main DNS server: %+v %+v %v", got, got.InfrastructureDNS, err)
	}
	fresh := base
	fresh.InfrastructureDNS = nil
	if got, err := canonicalServerSetupDraft(fresh); err != nil || got.InfrastructureDNS == nil || got.InfrastructureDNS.Zone != "example.test" {
		t.Fatalf("the zone is not an unticked option any more: %+v %v", got.InfrastructureDNS, err)
	}
	for _, change := range []func(*serverSetupDraft){
		func(d *serverSetupDraft) { d.DNSRole = "secondary"; d.MainDNSServer = "" },
		func(d *serverSetupDraft) { d.DNSMode = "external"; d.MainDNSServer = "" },
		func(d *serverSetupDraft) { d.DNSMode = "existing"; d.MainDNSServer = "" },
	} {
		draft := base
		change(&draft)
		if got, err := canonicalServerSetupDraft(draft); err != nil || got.InfrastructureDNS != nil {
			t.Fatalf("a backup or remote DNS home prepared a zone: %+v %v", got.InfrastructureDNS, err)
		}
	}
	for _, invalid := range []func(*serverSetupDraft){
		func(d *serverSetupDraft) { d.MainDomain = "not a domain" },
		func(d *serverSetupDraft) { d.ShortName = "two.labels" },
		func(d *serverSetupDraft) { d.ShortName = "-bad" },
		func(d *serverSetupDraft) { d.ServerCount = 3 },
		func(d *serverSetupDraft) { d.MainDNSServer = "peer" },
	} {
		draft := base
		invalid(&draft)
		if _, err := canonicalServerSetupDraft(draft); err == nil {
			t.Fatalf("invalid wizard answer accepted: %+v", draft)
		}
	}
	// A draft without a main domain is not touched (every draft before alpha.111).
	old := serverSetupDraft{Purpose: "web", DNSMode: "local", DNSRole: "primary", InfrastructureDNS: &serverSetupInfrastructureDNSDraft{Zone: "zone.test"}}
	if got, err := canonicalServerSetupDraft(old); err != nil || got.InfrastructureDNS.Zone != "zone.test" {
		t.Fatalf("old draft changed: %+v %v", got.InfrastructureDNS, err)
	}
	old.InfrastructureDNS = nil
	if got, _ := canonicalServerSetupDraft(old); got.InfrastructureDNS != nil {
		t.Fatal("old draft got a zone it never asked for")
	}
}

func TestServerSetupBuildWizardVersionsAndRefusals(t *testing.T) {
	f, state := setupOperationFixture(t)
	state.Draft.Purpose = "custom"
	state.Draft.Customization = &serverSetupCustomization{Components: []string{"nginx", "php-fpm"}, Versions: map[string]string{"php-fpm": "php8.3-fpm"}}
	var err error
	state.Draft, err = canonicalServerSetupDraft(state.Draft)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range plan.Steps {
		if step.Kind == "service" && step.Target == "php-fpm" {
			found = step.Qualifier == "php8.3-fpm"
		}
		if step.Kind == "service" && step.Target == "nginx" && step.Qualifier != "" {
			t.Fatalf("a version leaked to another component: %+v", step)
		}
	}
	if !found {
		t.Fatalf("chosen version is not the service step's package: %+v", plan.Steps)
	}
	plain := customSetupPlanForTest(t, f, state, "nginx", "php-fpm")
	if plain.ID == plan.ID {
		t.Fatal("a version choice did not change the reviewed plan")
	}
	for _, versions := range []map[string]string{{"php-fpm": "nginx"}, {"php-fpm": "php8.3-fpm; rm -rf /"}, {"nginx": "nginx-full"}, {"mariadb": "postgresql-17"}} {
		draft := state.Draft
		draft.Customization = &serverSetupCustomization{Components: []string{"nginx", "php-fpm"}, Versions: versions}
		if _, err := canonicalServerSetupDraft(draft); err == nil {
			t.Fatalf("arbitrary package accepted: %v", versions)
		}
	}
	draft := state.Draft
	draft.Customization = &serverSetupCustomization{Components: []string{"nginx"}, Versions: map[string]string{"postgresql": "postgresql-17"}}
	if _, err := canonicalServerSetupDraft(draft); err == nil {
		t.Fatal("a version for an unselected component was accepted")
	}
	draft.Customization = &serverSetupCustomization{Components: []string{"roundcube"}, Versions: map[string]string{"php-fpm": "php8.3-fpm", "postgresql": ""}}
	if got, err := canonicalServerSetupDraft(draft); err != nil || len(got.Customization.Versions) != 1 {
		t.Fatalf("a dependency's version must be choosable and an empty one dropped: %+v %v", got.Customization, err)
	}

	state.Draft.MainDNSServer = "this"
	state.Draft.ServerCount = 1
	refused, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || !slices.Contains(refused.Blockers, "server_setup_main_dns_server_mismatch") || refused.CanStart {
		t.Fatalf("main DNS server answer without local DNS: %v %v", refused.Blockers, err)
	}
	state.Draft.DNSMode, state.Draft.MainDNSServer = "local", ""
	local, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || !slices.Contains(local.Blockers, "server_setup_local_dns_requires_two_servers") {
		t.Fatalf("one server with its own DNS must say why it is refused: %v %v", local.Blockers, err)
	}
}

func TestServerSetupCatalogMailDependentServicesFollowMailProfile(t *testing.T) {
	if !serverSetupMailDependent("spamassassin") || serverSetupMailDependent("rspamd") || serverSetupMailDependent("nginx") || serverSetupMailDependent("roundcube") {
		t.Fatal("mail-dependent classification is wrong")
	}
	saved := core.ManagedServices
	t.Cleanup(func() {
		core.ManagedServices = saved
		serverSetupSelectableComponents = serverSetupSelectableFromCatalogue()
	})
	core.ManagedServices = append(append([]core.ManagedService{}, saved...), core.ManagedService{ID: "mail-addon", Name: "Mail add-on", Category: "email", Kind: core.KindService, Requires: []string{"smtp-server"}})
	serverSetupSelectableComponents = serverSetupSelectableFromCatalogue()
	f, state := setupOperationFixture(t)
	capabilities := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
	f.agent.versionCapabilities = &capabilities
	state.Draft.Purpose = "custom"
	state.Draft.MailHostname = "mail.example.test"
	plan := customSetupPlanForTest(t, f, state, "mail-addon")
	profile, addon := -1, -1
	for index, step := range plan.Steps {
		if step.Kind == "mail_profile" && profile < 0 {
			profile = index
		}
		if step.Kind == "service" && step.Target == "mail-addon" {
			addon = index
		}
	}
	if profile < 0 || addon < profile || slices.ContainsFunc(plan.Blockers, func(code string) bool { return strings.HasPrefix(code, "server_setup_dependency_missing") }) {
		t.Fatalf("a service needing the mail server must follow the mail profile: %+v %v", plan.Steps, plan.Blockers)
	}
}

func TestServerSetupCatalogShortNameSuggestion(t *testing.T) {
	for host, want := range map[string]string{
		"frankfurt.celikhost.com": "frankfurt", "Boston": "boston", "web-1.example.com": "web-1", "istanbul2": "istanbul2",
		"srv612345.hstgr.cloud": "", "vps-1a2b3c": "", "ip-10-0-0-1.ec2.internal": "", "localhost": "", "localhost.localdomain": "",
		"ubuntu": "", "debian": "", "ubuntu-s-1vcpu-1gb-fra1-01": "", "debian-12-x64": "", "vmi1234567": "", "static.42.provider.example": "",
		"a1b2c3d4e5f6": "", "1234": "", "node1": "", "mail.example.com": "", "ns1": "", "x": "", "averyveryverylonghostnamelabel": "",
		"ab12cd34": "", "s1": "", "": "",
	} {
		if got := serverSetupShortNameSuggestion(host); got != want {
			t.Fatalf("%q -> %q, want %q", host, got, want)
		}
	}
	previous := setupHostname
	t.Cleanup(func() { setupHostname = previous })
	setupHostname = func() (string, error) { return "frankfurt.example.test", nil }
	f := newServiceOperationTestFixture(t)
	w := httptest.NewRecorder()
	f.panel.handleServerSetup(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPath, "", f.userID))
	var state serverSetupState
	if json.Unmarshal(w.Body.Bytes(), &state) != nil || state.ShortNameSuggestion != "frankfurt" {
		t.Fatalf("suggestion not offered: %s", w.Body.String())
	}
	setupHostname = func() (string, error) { return "", errors.New("no host name") }
	w = httptest.NewRecorder()
	f.panel.handleServerSetup(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPath, "", f.userID))
	if strings.Contains(w.Body.String(), "short_name_suggestion") {
		t.Fatalf("an unreadable host name produced a suggestion: %s", w.Body.String())
	}
}
