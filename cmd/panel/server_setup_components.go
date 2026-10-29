package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A nil customization preserves the exact legacy preset and serialized plan.
// An explicit empty selection is meaningful and never falls back to a preset.
// Versions (alpha.111) is additive and omitted when empty: a component id ->
// the exact package the owner chose instead of the distribution's default
// (php8.3-fpm, postgresql-17). It becomes the service step's Package
// qualifier; the install preflight still requires the vendor repository.
// Versions: bilesen -> secilen tam paket; bossa yazilmaz.
type serverSetupCustomization struct {
	Components []string          `json:"components"`
	Versions   map[string]string `json:"versions,omitempty"`
}

type serverSetupComponentChoice struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Dependencies []string `json:"dependencies"`
	Conflicts    []string `json:"conflicts"`
	Supported    bool     `json:"supported"`
	Installed    bool     `json:"installed"`
	Reason       string   `json:"reason,omitempty"`
	// Additive (alpha.111). Wizard says what the wizard does with the row:
	// "choice" (the owner ticks it), "required" (every setup installs it),
	// "automatic" (prepared only when the selection needs it), "dns" (the
	// DNS step chooses the DNS software), "unavailable" (CelikPanel has no
	// integration yet) or "components" (installs from the Components page,
	// for example after its required vendor repository is enabled).
	// Wizard: sihirbazin satirla ne yaptigi.
	Wizard           string                        `json:"wizard"`
	InstalledVersion string                        `json:"installed_version,omitempty"`
	CandidateVersion string                        `json:"candidate_version,omitempty"`
	Repo             *serverSetupComponentRepo     `json:"repo,omitempty"`
	Versions         []serverSetupComponentVersion `json:"versions,omitempty"`
}

// serverSetupComponentRepo is the vendor repository a component can take
// other versions from, with its state on this server: "enabled", "disabled"
// or "unknown" (not read; never shown as disabled).
type serverSetupComponentRepo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
	State    string `json:"state"`
}

// serverSetupComponentVersion is one version the wizard can offer. The
// distribution's (empty package) is first and recommended. A vendor version is
// available only from an enabled repository; a locked entry carries the reason
// ("repository_disabled", "repository_unknown") and names no package.
type serverSetupComponentVersion struct {
	Package   string `json:"package"`
	Version   string `json:"version,omitempty"`
	Source    string `json:"source"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// serverSetupDNSEngineChoice says whether the DNS step may offer one DNS
// software for one role ("main" = primary, "backup" = secondary) on this
// server, and why not. It mirrors the review's own blockers; the review stays
// the authority.
type serverSetupDNSEngineChoice struct {
	ID               string `json:"id"`
	Role             string `json:"role"`
	Offered          bool   `json:"offered"`
	Reason           string `json:"reason,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	CandidateVersion string `json:"candidate_version,omitempty"`
}

type serverSetupComponentCatalog struct {
	Version            int                          `json:"version"`
	InventoryState     string                       `json:"inventory_state"`
	Presets            map[string][]string          `json:"presets"`
	RequiredComponents []string                     `json:"required_components"`
	Components         []serverSetupComponentChoice `json:"components"`
	// Additive (alpha.111).
	DNSEngines []serverSetupDNSEngineChoice `json:"dns_engines,omitempty"`
	Host       *serverSetupCatalogHost      `json:"host,omitempty"`
}

// serverSetupCatalogHost names the verified system the versions come from.
type serverSetupCatalogHost struct {
	DistroID  string `json:"distro_id,omitempty"`
	VersionID string `json:"version_id,omitempty"`
}

type serverSetupPlanComponent struct {
	ID        string `json:"id"`
	Selected  bool   `json:"selected"`
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
}

var serverSetupRequiredComponents = []string{"nftables", "certbot"}

// serverSetupComponentWizardRole decides, from the catalogue alone, what the
// wizard does with a component (owner decision 2026-10-26: the wizard's list
// comes from the same catalogue as the Components page). A component added to
// the catalogue becomes a choice by itself unless the catalogue says why not:
// DNS software has its own ownership workflow, cron is prepared only when a
// site needs it, an integration-pending service stays closed, and a service
// that exists only in a vendor repository installs from Components, where
// that repository is reviewed and enabled. Mail's core services remain
// installed only by the audited mail profiles (buildServerSetupPlan).
// Sihirbazin bir bilesenle ne yaptigi yalniz katalogdan cikar.
func serverSetupComponentWizardRole(service *core.ManagedService) string {
	switch {
	case service == nil:
		return "unavailable"
	case slices.Contains(serverSetupRequiredComponents, service.ID):
		return "required"
	case service.ID == core.NativeCronServiceID:
		return "automatic"
	case managedDNSEngineService(service) || service.Category == "dns":
		return "dns"
	case service.InstallDisabledReason != "":
		return "unavailable"
	case service.Repo != nil && service.Repo.Required:
		return "components"
	case !serverSetupHelperUnitsObservable(service):
		return "components"
	default:
		return "choice"
	}
}

// serverSetupHelperUnitsObservable: setup's final check proves every helper
// unit of a chosen service running (server_setup_readiness.go), so a helper
// the service scanner cannot see (SpamAssassin's spamass-milter today) would
// leave setup waiting for ever. Such a service installs from Components.
// Tarayicinin goremedigi yardimci birimli servis Bilesenler'den kurulur.
func serverSetupHelperUnitsObservable(service *core.ManagedService) bool {
	for _, helper := range service.HelperUnits {
		if core.ServiceForUnit(helper) == nil && core.HelperUnitOwner(helper) != service {
			return false
		}
	}
	return true
}

func serverSetupSelectableFromCatalogue() []string {
	ids := []string{}
	for index := range core.ManagedServices {
		if serverSetupComponentWizardRole(&core.ManagedServices[index]) == "choice" {
			ids = append(ids, core.ManagedServices[index].ID)
		}
	}
	return ids
}

// The components the wizard can install, generated from the catalogue.
var serverSetupSelectableComponents = serverSetupSelectableFromCatalogue()

func serverSetupPresetComponents(draft serverSetupDraft) []string {
	switch draft.Purpose {
	case "web":
		return []string{"nginx", "php-fpm", "mariadb"}
	case "web_mail":
		return []string{"nginx", "php-fpm", "mariadb", "postfix", "dovecot", "roundcube", "rspamd"}
	case "application":
		ids := []string{"nginx", "node"}
		if draft.Database != "" {
			ids = append(ids, draft.Database)
		}
		return ids
	default:
		return []string{}
	}
}

const serverSetupMaxVersionChoices = 8

func canonicalServerSetupCustomization(value *serverSetupCustomization) (*serverSetupCustomization, error) {
	if value == nil {
		return nil, nil
	}
	if len(value.Components) > len(serverSetupSelectableComponents) {
		return nil, errors.New("too many setup components")
	}
	ids := append([]string{}, value.Components...)
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if !slices.Contains(serverSetupSelectableComponents, id) {
			return nil, errors.New("unsupported setup component")
		}
		ids[i] = id
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	result := &serverSetupCustomization{Components: ids}
	if len(value.Versions) > serverSetupMaxVersionChoices {
		return nil, errors.New("too many setup version choices")
	}
	for id, pkg := range value.Versions {
		id, pkg = strings.TrimSpace(id), strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		if !serverSetupVersionPackageAllowed(id, pkg) {
			return nil, errors.New("unsupported setup version choice")
		}
		if result.Versions == nil {
			result.Versions = map[string]string{}
		}
		result.Versions[id] = pkg
	}
	return result, nil
}

// serverSetupVersionPackageAllowed accepts only a package the component's own
// vendor repository pattern names (the Components page's rule): never an
// arbitrary package name.
func serverSetupVersionPackageAllowed(id, pkg string) bool {
	service := core.GetManagedServiceByID(id)
	if service == nil || !slices.Contains(serverSetupSelectableComponents, id) || service.Repo == nil || strings.TrimSpace(service.Repo.PackagePattern) == "" || len(pkg) > 64 {
		return false
	}
	pattern, err := regexp.Compile(service.Repo.PackagePattern)
	return err == nil && pattern.FindString(pkg) == pkg
}

// serverSetupVersionChoicesSelected refuses a version for a component the
// resolved selection does not contain.
func serverSetupVersionChoicesSelected(draft serverSetupDraft) error {
	if draft.Customization == nil || len(draft.Customization.Versions) == 0 {
		return nil
	}
	resolved, err := serverSetupResolvedComponents(draft)
	if err != nil {
		return err
	}
	for id := range draft.Customization.Versions {
		if !slices.Contains(resolved, id) {
			return errors.New("a version was chosen for a component that is not selected")
		}
	}
	return nil
}

// Resolve catalogue role dependencies to the supported setup provider. Existing
// competing providers are never replaced: the shared conflict check blocks the
// reviewed plan instead. Mail's complete lifecycle adds the paired mailbox
// services even though the individual package catalogue does not require them.
func serverSetupComponentDependencies(id string) []string {
	dependencies := []string{}
	managed := core.GetManagedServiceByID(id)
	if managed == nil {
		return dependencies
	}
	for _, dependency := range managed.Requires {
		switch dependency {
		case "web-server":
			dependency = "nginx"
		case "smtp-server":
			dependency = "postfix"
		case "imap-server":
			dependency = "dovecot"
		}
		dependencies = append(dependencies, dependency)
	}
	switch id {
	case "postfix":
		dependencies = append(dependencies, "dovecot")
	case "dovecot":
		dependencies = append(dependencies, "postfix")
	}
	slices.Sort(dependencies)
	return slices.Compact(dependencies)
}

// serverSetupMailDependent reports a generic service whose catalogue
// requirement is a mail server (SpamAssassin): it can be installed only after
// the mail profile that installs Postfix and Dovecot.
// Posta sunucusuna ihtiyac duyan genel servis; posta profilinden sonra kurulur.
func serverSetupMailDependent(id string) bool {
	if slices.Contains([]string{"postfix", "dovecot", "rspamd", "roundcube"}, id) {
		return false
	}
	managed := core.GetManagedServiceByID(id)
	if managed == nil {
		return false
	}
	for _, requirement := range managed.Requires {
		if stringIn(requirement, "smtp-server", "imap-server", "postfix", "dovecot") {
			return true
		}
	}
	return false
}

func serverSetupResolvedComponents(draft serverSetupDraft) ([]string, error) {
	selected := serverSetupPresetComponents(draft)
	if draft.Customization != nil {
		canonical, err := canonicalServerSetupCustomization(draft.Customization)
		if err != nil {
			return nil, err
		}
		selected = canonical.Components
	}
	result := []string{}
	seen := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if seen[id] {
			return nil
		}
		if !slices.Contains(serverSetupSelectableComponents, id) && !slices.Contains(serverSetupRequiredComponents, id) {
			return errors.New("unsupported setup dependency")
		}
		seen[id] = true
		for _, dependency := range serverSetupComponentDependencies(id) {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		result = append(result, id)
		return nil
	}
	for _, id := range selected {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func serverSetupHasComponent(draft serverSetupDraft, id string) bool {
	resolved, err := serverSetupResolvedComponents(draft)
	return err == nil && slices.Contains(resolved, id)
}

// serverSetupNeedsNativeCron reports whether the reviewed profile hosts sites,
// whose Scheduled tasks screen writes native user crontabs. Those profiles
// prepare the platform's own cron (cron on Debian/Ubuntu, cronie on Arch)
// through the ordinary component step; the Agent installs it only when no
// cron implementation is present and otherwise leaves the owner's untouched
// (D-022). A DNS-only profile hosts no site and gets no cron change.
//
// serverSetupNeedsNativeCron, incelenen profilin site barındırıp
// barındırmadığını bildirir; site Zamanlanmış görevler ekranı yerel kullanıcı
// crontab'larına yazar. Bu profiller platformun kendi cron'unu olağan bileşen
// adımıyla hazırlar; Agent onu yalnız hiçbir cron yokken kurar (D-022).
func serverSetupNeedsNativeCron(draft serverSetupDraft) bool {
	if draft.Customization == nil {
		return stringIn(draft.Purpose, "web", "web_mail", "application")
	}
	return serverSetupHasComponent(draft, "nginx")
}

func serverSetupMailProfileIDs(draft serverSetupDraft) []string {
	if draft.Customization == nil {
		if draft.Purpose == "web_mail" {
			return []string{core.MailProfileWebmail, core.MailProfileProtected}
		}
		return nil
	}
	resolved, err := serverSetupResolvedComponents(draft)
	if err != nil || !slices.Contains(resolved, "postfix") {
		return nil
	}
	profiles := []string{}
	if slices.Contains(resolved, "roundcube") {
		profiles = append(profiles, core.MailProfileWebmail)
	}
	if slices.Contains(resolved, "rspamd") {
		profiles = append(profiles, core.MailProfileProtected)
	}
	if len(profiles) == 0 {
		profiles = append(profiles, core.MailProfileCore)
	}
	return profiles
}

func serverSetupNeedsDNSPublisher(draft serverSetupDraft) bool {
	if draft.Customization == nil {
		return draft.Purpose != "dns"
	}
	for _, id := range []string{"nginx", "node", "postfix", "roundcube"} {
		if serverSetupHasComponent(draft, id) {
			return true
		}
	}
	return false
}

// serverSetupRepoObservation is one vendor repository as the Agent reported
// it: "enabled" (with the versioned packages it offers now), "disabled" or
// "unknown".
type serverSetupRepoObservation struct {
	State    string
	Packages []string
}

// serverSetupCatalogInputs is everything the catalogue reads from the host.
// Each read is optional: a missing answer leaves its value unknown, never
// "absent" (D-036).
type serverSetupCatalogInputs struct {
	Host              core.ManagedServiceHostProfile
	InstalledIDs      []string
	InventoryReady    bool
	InstalledVersions map[string]string
	CandidateVersions map[string]string
	Repos             map[string]serverSetupRepoObservation
	DNSState          *dnsEngineDBState
	// OwnDNSEngine: the active engine is this setup's own recorded install
	// (serverSetupOwnDNSEngine), not foreign DNS needing a migration.
	OwnDNSEngine bool
	// PDNSCandidate is the Agent's read of the PowerDNS build a fresh paired
	// primary would install (nil when not read).
	PDNSCandidate *transport.PDNSFreshPrimaryCandidateResponse
}

func (p *Panel) handleServerSetupComponents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	_ = json.NewEncoder(w).Encode(buildServerSetupComponentCatalogFrom(p.readServerSetupCatalogInputs(ctx)))
}

// readServerSetupCatalogInputs performs read-only Agent and database reads.
// Nothing here installs, enables or writes anything.
// Yalniz okuma: hicbir sey kurulmaz, acilmaz, yazilmaz.
func (p *Panel) readServerSetupCatalogInputs(ctx context.Context) serverSetupCatalogInputs {
	inputs := serverSetupCatalogInputs{Host: p.managedServiceHostProfile(), InstalledVersions: map[string]string{}, CandidateVersions: map[string]string{}, Repos: map[string]serverSetupRepoObservation{}}
	inventoryErr := p.callAgentContext(ctx, "Agent.InstalledServiceIDsStrict", &transport.Empty{}, &inputs.InstalledIDs)
	var nodeVersions transport.NodeVersionsResponse
	if inventoryErr == nil {
		inventoryErr = p.callAgentContext(ctx, "Agent.ListNodeVersions", &transport.Empty{}, &nodeVersions)
		if inventoryErr == nil && len(nodeVersions.Installed) > 0 {
			inputs.InstalledIDs = append(inputs.InstalledIDs, "node")
			inputs.InstalledVersions["node"] = strings.TrimPrefix(nodeVersions.Installed[0], "v")
		}
	}
	inputs.InventoryReady = inventoryErr == nil
	if !inputs.InventoryReady {
		return inputs
	}
	if state, err := readDNSEngineDBState(ctx, p.db.GetDB()); err == nil {
		inputs.DNSState = &state
		inputs.OwnDNSEngine, _ = p.serverSetupOwnDNSEngine(ctx, state)
		if state.ActiveEngine == "" && serverSetupPDNSMainSystemTested(inputs.Host) {
			inputs.PDNSCandidate = p.readServerSetupPDNSCandidate(ctx)
		}
	}
	for id, observed := range p.readPackageVersionsForScan(ctx, nil, time.Now()) {
		if _, short, _ := packageVersionView(observed.Packages); short != "" {
			inputs.InstalledVersions[id] = short
		}
	}
	// The distribution's version of each component that would be installed,
	// within one bounded budget; what is not read stays unknown.
	candidates, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	for index := range core.ManagedServices {
		service := &core.ManagedServices[index]
		role := serverSetupComponentWizardRole(service)
		if (role != "choice" && role != "dns") || slices.Contains(inputs.InstalledIDs, service.ID) || len(service.Packages[inputs.Host.PackageFamily]) == 0 || candidates.Err() != nil {
			continue
		}
		var version string
		if err := p.callAgentContext(candidates, "Agent.ServiceCandidateVersion", &transport.InstallServiceRequest{ID: service.ID}, &version); err == nil {
			if short := upstreamPackageVersion(panelPackageVersion(version)); short != "" {
				inputs.CandidateVersions[service.ID] = short
			}
		}
	}
	for index := range core.ManagedServices {
		service := &core.ManagedServices[index]
		if serverSetupComponentWizardRole(service) != "choice" || service.Repo == nil || strings.TrimSpace(service.Repo.PackagePattern) == "" || inputs.Host.PackageFamily != "apt" {
			continue
		}
		if _, read := inputs.Repos[service.Repo.ID]; read {
			continue
		}
		inputs.Repos[service.Repo.ID] = p.readServerSetupRepoObservation(ctx, service.Repo.ID)
	}
	return inputs
}

// readServerSetupRepoObservation reads one vendor repository's state and, when
// it is enabled, the packages it offers. A failed read is "unknown".
// Yalniz okuma: deponun durumu ve sundugu paketler.
func (p *Panel) readServerSetupRepoObservation(ctx context.Context, repoID string) serverSetupRepoObservation {
	observation := serverSetupRepoObservation{State: "unknown"}
	var status transport.RepoStatusResponse
	if err := p.callAgentContext(ctx, "Agent.RepoStatus", &transport.EnableRepoRequest{RepoID: repoID}, &status); err == nil && status.Error == "" {
		observation.State = "disabled"
		if status.Enabled {
			var packages transport.RepoPackagesResponse
			if err := p.callAgentContext(ctx, "Agent.RepoPackages", &transport.RepoPackagesRequest{RepoID: repoID}, &packages); err == nil && packages.Error == "" {
				observation = serverSetupRepoObservation{State: "enabled", Packages: packages.Packages}
			} else {
				observation.State = "unknown"
			}
		}
	}
	return observation
}

// buildServerSetupComponentCatalog keeps the established call shape.
func buildServerSetupComponentCatalog(host core.ManagedServiceHostProfile, installedIDs []string, inventoryReady bool) serverSetupComponentCatalog {
	return buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InstalledIDs: installedIDs, InventoryReady: inventoryReady})
}

// buildServerSetupComponentCatalogFrom lists every catalogue component, in
// catalogue order, with what the wizard does with it and why.
func buildServerSetupComponentCatalogFrom(inputs serverSetupCatalogInputs) serverSetupComponentCatalog {
	host, installedIDs, inventoryReady := inputs.Host, inputs.InstalledIDs, inputs.InventoryReady
	catalog := serverSetupComponentCatalog{Version: 1, InventoryState: "ready", Presets: map[string][]string{}, RequiredComponents: append([]string{}, serverSetupRequiredComponents...), Components: []serverSetupComponentChoice{}}
	if host.DistroID != "" {
		catalog.Host = &serverSetupCatalogHost{DistroID: host.DistroID, VersionID: host.VersionID}
	}
	if !inventoryReady {
		catalog.InventoryState = "unknown"
	}
	for _, purpose := range []string{"web", "web_mail", "application", "dns", "custom"} {
		catalog.Presets[purpose] = serverSetupPresetComponents(serverSetupDraft{Purpose: purpose, Database: "mariadb"})
	}
	installed := map[string]bool{}
	if inventoryReady {
		for _, installedID := range installedIDs {
			installed[installedID] = true
		}
	}
	for index := range core.ManagedServices {
		managed := &core.ManagedServices[index]
		id := managed.ID
		role := serverSetupComponentWizardRole(managed)
		choice := serverSetupComponentChoice{ID: id, Name: managed.Name, Category: managed.Category, Dependencies: serverSetupComponentDependencies(id), Conflicts: []string{}, Installed: installed[id], Supported: true, Wizard: role}
		if role != "choice" && role != "required" {
			// Only reviewed setup choices carry dependencies the wizard resolves.
			choice.Dependencies = []string{}
		}
		if installed[id] {
			choice.InstalledVersion = inputs.InstalledVersions[id]
		} else {
			choice.CandidateVersion = inputs.CandidateVersions[id]
		}
		for _, other := range core.ManagedServices {
			if managed.ConflictGroup != "" && managed.ConflictGroup == other.ConflictGroup && id != other.ID {
				choice.Conflicts = append(choice.Conflicts, other.ID)
			}
		}
		slices.Sort(choice.Conflicts)
		switch role {
		case "dns":
			choice.Supported, choice.Reason = false, "server_setup_component_dns_step"
		case "automatic":
			choice.Supported, choice.Reason = false, "server_setup_component_automatic"
		case "unavailable":
			choice.Supported, choice.Reason = false, "server_setup_service_integration:"+id
		case "components":
			choice.Supported, choice.Reason = false, "server_setup_component_after_setup:"+id
			if managed.Repo != nil && managed.Repo.Required {
				choice.Reason = "server_setup_repository_required:" + id
			}
		default:
			resolved, _ := serverSetupResolvedComponents(serverSetupDraft{Customization: &serverSetupCustomization{Components: []string{id}}})
			if role == "required" {
				resolved = []string{id}
			}
			for _, dependency := range resolved {
				managedDependency := core.GetManagedServiceByID(dependency)
				if occupied := core.SeatTakenBy(managedDependency, installed); occupied != "" {
					choice.Supported, choice.Reason = false, "server_setup_service_conflict:"+dependency+":"+occupied
					break
				}
				if _, reason := core.ManagedServiceInstallBlockForHost(managedDependency, host); reason != "" {
					choice.Supported, choice.Reason = false, "server_setup_service_unsupported:"+dependency
					break
				}
			}
			if role == "choice" && !installed[id] {
				choice.Repo, choice.Versions = serverSetupComponentVersions(managed, choice.CandidateVersion, host, inputs.Repos)
			}
		}
		if !inventoryReady {
			choice.Supported, choice.Reason, choice.Installed = false, "server_setup_inventory_unavailable", false
			choice.InstalledVersion = ""
		}
		catalog.Components = append(catalog.Components, choice)
	}
	catalog.DNSEngines = serverSetupDNSEngineChoices(inputs)
	return catalog
}

// serverSetupComponentVersions lists the distribution's version and, for a
// component with a vendor repository, either that repository's versions
// (enabled) or one locked entry saying it is not enabled (the owner enables
// it on the Components page; setup never enables a repository).
func serverSetupComponentVersions(managed *core.ManagedService, candidate string, host core.ManagedServiceHostProfile, repos map[string]serverSetupRepoObservation) (*serverSetupComponentRepo, []serverSetupComponentVersion) {
	if managed.Repo == nil || strings.TrimSpace(managed.Repo.PackagePattern) == "" || host.PackageFamily != "apt" {
		return nil, nil
	}
	observation, read := repos[managed.Repo.ID]
	if !read || observation.State == "" {
		observation.State = "unknown"
	}
	repo := &serverSetupComponentRepo{ID: managed.Repo.ID, Name: managed.Repo.Name, Required: managed.Repo.Required, State: observation.State}
	versions := []serverSetupComponentVersion{{Package: "", Version: candidate, Source: "distribution", Available: true}}
	switch observation.State {
	case "enabled":
		packages := append([]string(nil), observation.Packages...)
		slices.Sort(packages)
		for _, pkg := range slices.Compact(packages) {
			if len(versions) > serverSetupMaxVersionChoices*2 {
				break
			}
			if serverSetupVersionPackageAllowed(managed.ID, pkg) {
				versions = append(versions, serverSetupComponentVersion{Package: pkg, Version: serverSetupPackageVersionLabel(pkg), Source: "vendor", Available: true})
			}
		}
	case "disabled":
		versions = append(versions, serverSetupComponentVersion{Source: "vendor", Reason: "repository_disabled"})
	default:
		versions = append(versions, serverSetupComponentVersion{Source: "vendor", Reason: "repository_unknown"})
	}
	return repo, versions
}

var serverSetupPackageVersionDigits = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// serverSetupPackageVersionLabel is the version a versioned package name
// carries ("php8.3-fpm" -> "8.3", "postgresql-17" -> "17").
func serverSetupPackageVersionLabel(pkg string) string {
	return serverSetupPackageVersionDigits.FindString(pkg)
}

// serverSetupPDNSMainSystemTested reports whether PowerDNS as the main (paired
// primary) DNS server was measured on this system: Debian 13 amd64 only. The
// catalogue and the plan review use the same rule.
// PowerDNS ana sunucu olarak yalniz olculen sistemde (Debian 13 amd64).
func serverSetupPDNSMainSystemTested(host core.ManagedServiceHostProfile) bool {
	return host.DistroID == "debian" && host.VersionID == "13" && host.Architecture == "amd64"
}

// serverSetupPDNSMainCandidateBlocker applies the Agent's own rules to the
// PowerDNS build this server would install, so the wizard offers PowerDNS as
// the main DNS server only when the DNS step will accept it: Debian's archive
// build of the tested series (dnsengineartifact), from Debian's own archive.
// An unread candidate stays unknown, not refused: the Agent reads it again
// before any effect.
// Sihirbazin sundugu yapi, DNS adiminin kabul ettigi kuraldan gelir.
func serverSetupPDNSMainCandidateBlocker(candidate *transport.PDNSFreshPrimaryCandidateResponse) string {
	switch {
	case candidate == nil || candidate.State != "known":
		return ""
	case !dnsengineartifact.PDNSFreshPrimaryVersionInTestedSeries(candidate.Version):
		return "server_setup_pdns_main_untested_version"
	case !candidate.Official:
		return "server_setup_pdns_main_not_debian_archive"
	}
	return ""
}

// readServerSetupPDNSCandidate asks the Agent, read-only (nil when it cannot
// answer).
func (p *Panel) readServerSetupPDNSCandidate(ctx context.Context) *transport.PDNSFreshPrimaryCandidateResponse {
	var candidate transport.PDNSFreshPrimaryCandidateResponse
	if err := p.callAgentContext(ctx, "Agent.PDNSFreshPrimaryCandidateV1", &transport.Empty{}, &candidate); err != nil {
		return nil
	}
	return &candidate
}

// serverSetupDNSEngineChoices applies the review's DNS rules per software and
// role: a server runs one DNS software and an active one is never switched by
// setup; PowerDNS as the main (paired primary) server is offered only by the
// product gate, only on a server without DNS software and only on the
// measured system (Debian 13 amd64, the Agent's own preflight).
// DNS yazilimi secenekleri incelemenin kurallariyla.
func serverSetupDNSEngineChoices(inputs serverSetupCatalogInputs) []serverSetupDNSEngineChoice {
	choices := []serverSetupDNSEngineChoice{}
	for _, engine := range []transport.DNSEngine{transport.DNSEngineBIND, transport.DNSEnginePowerDNS} {
		managed := core.GetManagedServiceByID(string(engine))
		for _, role := range []string{"main", "backup"} {
			choice := serverSetupDNSEngineChoice{ID: string(engine), Role: role, Offered: true}
			if inputs.InventoryReady && slices.Contains(inputs.InstalledIDs, string(engine)) {
				choice.InstalledVersion = inputs.InstalledVersions[string(engine)]
			} else {
				choice.CandidateVersion = inputs.CandidateVersions[string(engine)]
			}
			_, blocked := core.ManagedServiceInstallBlockForHost(managed, inputs.Host)
			switch {
			case !inputs.InventoryReady || inputs.DNSState == nil:
				choice.Offered, choice.Reason = false, "server_setup_inventory_unavailable"
			case managed == nil || blocked != "":
				choice.Offered, choice.Reason = false, "server_setup_dns_engine_unsupported"
			case inputs.DNSState.ActiveEngine != "" && inputs.DNSState.ActiveEngine != engine && inputs.OwnDNSEngine:
				choice.Offered, choice.Reason = false, serverSetupDNSInstalledDiffersCode
			case inputs.DNSState.ActiveEngine != "" && inputs.DNSState.ActiveEngine != engine:
				choice.Offered, choice.Reason = false, "server_setup_existing_dns_requires_migration"
			case role == "main" && engine == transport.DNSEnginePowerDNS && inputs.DNSState.ActiveEngine == "":
				code, _ := pdnsPairedPrimaryBlocker(dnsEngineSnapshot{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRolePrimary, EngineEpoch: inputs.DNSState.EngineEpoch}, engine, "install")
				if code != "" {
					choice.Offered, choice.Reason = false, code
				} else if !serverSetupPDNSMainSystemTested(inputs.Host) {
					choice.Offered, choice.Reason = false, "server_setup_pdns_main_untested_system"
				} else if code := serverSetupPDNSMainCandidateBlocker(inputs.PDNSCandidate); code != "" {
					choice.Offered, choice.Reason = false, code
				}
			}
			choices = append(choices, choice)
		}
	}
	return choices
}
