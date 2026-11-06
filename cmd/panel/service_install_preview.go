package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/transport"
)

// GET /api/v1/service/install-preview (alpha.105, D-034 amendment): what
// installing one component, or a bundle's missing components together,
// would bring, as the package manager's own simulation says, read-only and
// bounded. Notable packages (another PHP version, Apache, another catalogue
// component, a database server) are named so the owner confirms them
// before anything is installed; a chosen PHP version that cannot serve its
// tool is refused with the packages it lacks. A failed or slow read says
// "unknown": the install is still checked again on the server.
//
// Bir bileşeni ya da paketin eksik bileşenlerini kurmanın neyi
// getireceği; salt-okur ve sınırlı. Okunamazsa "bilinmiyor"; kurulum
// sunucuda yine denetlenir.

const (
	serviceInstallPreviewPath = "/api/v1/service/install-preview"
	// alpha.121: a first simulation, at most one refresh of a managed
	// repository's lists and a second simulation (the Agent bounds them).
	serviceInstallPreviewBound = 110 * time.Second
	maxInstallPreviewSteps     = 8
)

var installPreviewPackagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+.\-]{0,127}(:[a-z0-9\-]{1,32})?$`)

type serviceInstallPreviewAnswer struct {
	// State: known, busy (another package operation runs), unsupported,
	// unknown (the read failed or took too long).
	State     string                            `json:"state"`
	Installs  []string                          `json:"installs"`
	Notable   []transport.NotableInstall        `json:"notable"`
	Removes   []string                          `json:"removes"`
	Refusals  []transport.InstallPreviewRefusal `json:"refusals"`
	Requested []string                          `json:"requested"`
	// Distribution (alpha.109): this server's os-release ID ("ubuntu",
	// "debian") so the confirmation names whose version a vendor package
	// replaces. Absent when os-release cannot be read. Additive.
	// Sunucunun dağıtımı; onay, kimin sürümünün değiştirileceğini adıyla söyler.
	Distribution string `json:"distribution,omitempty"`
	// Reason (alpha.121, D-024): why the read said "unknown" (or why the
	// package manager could not answer), as a stable code, the package names
	// and the managed repository it concerns, and one short sentence built
	// from them, never the package manager's own text. Additive.
	// Okumanın neden "bilinmiyor" dediği: kod, paket adları, depo.
	ReasonCode     string   `json:"reason_code,omitempty"`
	ReasonPackages []string `json:"reason_packages,omitempty"`
	ReasonRepo     string   `json:"reason_repo,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	// ListsRefreshed names the repository whose package lists the check
	// refreshed once before answering. Additive.
	ListsRefreshed string `json:"lists_refreshed,omitempty"`
}

// installPreviewReasonCodes are the reasons a screen may be told.
var installPreviewReasonCodes = map[string]string{
	"unmet_dependencies":   "unmet dependencies",
	"package_not_found":    "not offered by the package lists",
	"lists_refresh_failed": "the repository's package lists could not be refreshed",
	"package_manager_busy": "another program holds the package manager",
	"timed_out":            "the package manager did not answer in time",
	"invalid_request":      "the request does not name an installable version",
	"agent_unreachable":    "the server agent did not answer",
	"other":                "the package manager reported an error",
}

// installPreviewReason sanitizes the Agent's reason: a known code only,
// valid package names (at most 8) and a catalogue repository id.
func installPreviewReason(resp transport.ServiceInstallPreviewResponse) (code string, packages []string, repo, sentence string) {
	code = resp.ErrorCode
	if _, ok := installPreviewReasonCodes[code]; !ok {
		code = "other"
	}
	packages = []string{}
	for _, name := range resp.ErrorPackages {
		if len(packages) < 8 && installPreviewPackagePattern.MatchString(name) && !strings.Contains(name, ":") {
			packages = append(packages, name)
		}
	}
	if resp.ErrorRepo != "" && repoCatalogued(resp.ErrorRepo) {
		repo = resp.ErrorRepo
	}
	sentence = installPreviewReasonCodes[code]
	if len(packages) > 0 {
		sentence += ": " + strings.Join(packages, ", ")
	}
	if repo != "" && (code == "package_not_found" || code == "lists_refresh_failed") {
		sentence += " (" + repo + ")"
	}
	return code, packages, repo, sentence
}

// repoCatalogued: id is a managed repository of the catalogue.
func repoCatalogued(id string) bool {
	for i := range core.ManagedServices {
		if repo := core.ManagedServices[i].Repo; repo != nil && repo.ID == id {
			return true
		}
	}
	return false
}

// serverDistributionID is this server's os-release ID, or "".
func serverDistributionID() string {
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return ""
	}
	release, err := hostplatform.ParseOSRelease(data)
	if err != nil {
		return ""
	}
	return release.ID
}

// parseInstallPreviewSteps reads "step=<id>[:<package>[:<php version>]]".
func parseInstallPreviewSteps(values []string) ([]transport.ServiceInstallPreviewStep, string) {
	if len(values) == 0 || len(values) > maxInstallPreviewSteps {
		return nil, "name one to eight steps"
	}
	steps := make([]transport.ServiceInstallPreviewStep, 0, len(values))
	for _, value := range values {
		parts := strings.Split(strings.TrimSpace(value), ":")
		if len(parts) > 3 {
			return nil, "invalid step"
		}
		step := transport.ServiceInstallPreviewStep{ID: parts[0]}
		if len(parts) > 1 {
			step.Package = parts[1]
		}
		if len(parts) > 2 {
			step.PHPVersion = parts[2]
		}
		svc := core.GetManagedServiceByID(step.ID)
		if svc == nil {
			return nil, "a step does not name a component"
		}
		if managedDNSEngineServiceID(svc.ID) {
			return nil, "the DNS engines are installed from DNS and peer servers"
		}
		if step.Package != "" && (!installPreviewPackagePattern.MatchString(step.Package) || !packagePickOf(svc, step.Package)) {
			return nil, "a step names a package that is not a version of its component"
		}
		if step.PHPVersion != "" && ((svc.ID != "phpmyadmin" && svc.ID != "phppgadmin") || !phpToolVersionPattern.MatchString(step.PHPVersion)) {
			return nil, "a PHP version is chosen only for phpMyAdmin and phpPgAdmin"
		}
		steps = append(steps, step)
	}
	return steps, ""
}

// installPackagePreview is the Agent's read-only simulation (a seam for tests).
var installPackagePreview = func(p *Panel, ctx context.Context, request transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
	var resp transport.ServiceInstallPreviewResponse
	err := p.callAgentContext(ctx, "Agent.ServiceInstallPreviewV1", &request, &resp)
	return resp, err
}

func (p *Panel) handleServiceInstallPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	steps, reason := parseInstallPreviewSteps(r.URL.Query()["step"])
	if reason != "" {
		writeClientError(w, http.StatusBadRequest, reason)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), serviceInstallPreviewBound)
	defer cancel()
	answer := serviceInstallPreviewAnswer{
		State:     "unknown",
		Installs:  []string{},
		Notable:   []transport.NotableInstall{},
		Removes:   []string{},
		Refusals:  []transport.InstallPreviewRefusal{},
		Requested: []string{},
	}
	answer.Distribution = serverDistributionID()
	resp, err := installPackagePreview(p, ctx, transport.ServiceInstallPreviewRequest{Steps: steps})
	if err != nil {
		log.Printf("install preview: %v", err)
		answer.ReasonCode, answer.Reason = "agent_unreachable", installPreviewReasonCodes["agent_unreachable"]
		if ctx.Err() != nil {
			answer.ReasonCode, answer.Reason = "timed_out", installPreviewReasonCodes["timed_out"]
		}
		answer.ReasonPackages = []string{}
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	if repo := resp.ListsRefreshed; repo != "" {
		for _, id := range strings.Split(repo, ",") {
			if !repoCatalogued(id) {
				repo = ""
			}
		}
		answer.ListsRefreshed = repo
	}
	switch resp.State {
	case "known":
		answer.State = "known"
		answer.Installs = nonNilStrings(resp.Installs)
		answer.Removes = nonNilStrings(resp.Removes)
		answer.Requested = nonNilStrings(resp.Requested)
		if resp.Notable != nil {
			answer.Notable = resp.Notable
		}
	case "busy", "unsupported":
		answer.State = resp.State
	default:
		if resp.Error != "" {
			log.Printf("install preview: %s", resp.Error)
		}
		answer.ReasonCode, answer.ReasonPackages, answer.ReasonRepo, answer.Reason = installPreviewReason(resp)
	}
	// A refusal stands whatever the simulation said: it is the catalogue's
	// answer for the chosen PHP version.
	if resp.Refusals != nil {
		answer.Refusals = resp.Refusals
	}
	_ = json.NewEncoder(w).Encode(answer)
}

// packagePickOf: the package is a version pick of the component (its
// repository's package pattern matches the whole name).
func packagePickOf(svc *core.ManagedService, pkg string) bool {
	if svc == nil || svc.Repo == nil || svc.Repo.PackagePattern == "" {
		return false
	}
	re, err := regexp.Compile(svc.Repo.PackagePattern)
	if err != nil {
		return false
	}
	return re.FindString(pkg) == pkg
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// notableRefusalDetail is the one line a refused install says where only a
// line fits (Server setup's guidance, bounded to 180 characters): what it
// would also bring, and where the owner can see and confirm it.
func notableRefusalDetail(notable []transport.NotableInstall) string {
	names := make([]string, 0, 2)
	for _, item := range notable {
		if len(names) == 2 {
			break
		}
		names = append(names, item.Package)
	}
	more := ""
	if len(notable) > len(names) {
		more = fmt.Sprintf(" (+%d more)", len(notable)-len(names))
	}
	return fmt.Sprintf("Not installed: it would also bring %s%s. Install it from Components to see and confirm them.", strings.Join(names, ", "), more)
}

// vendorRefusalDetail says a refused vendor replacement (alpha.107, D-038
// §12): which distribution packages a vendor repository would have replaced,
// that nothing was installed, and what the administrator does.
func vendorRefusalDetail(notable []transport.NotableInstall) string {
	names := []string{}
	repo := ""
	for _, item := range notable {
		if item.Reason == "vendor_replaces_distribution" && len(names) < 4 {
			names = append(names, item.Package)
			repo = item.Component
		}
	}
	return fmt.Sprintf("Not installed: the %s repository would have replaced the distribution's %s with its own builds. Confine the repository on its card on the component's page, then try again.", repo, strings.Join(names, ", "))
}

// serviceInstallOperationData is what an install keeps in its operation
// record so a resumed install runs as it was asked: the tool's PHP version
// and the packages the owner confirmed.
type serviceInstallOperationData struct {
	PHPVersion        string   `json:"install_php_version,omitempty"`
	ConfirmedInstalls []string `json:"install_confirmed,omitempty"`
}

func encodeServiceInstallOperationData(req serviceInstallRequest) string {
	if req.PHPVersion == "" && len(req.ConfirmedInstalls) == 0 {
		return ""
	}
	data, err := json.Marshal(serviceInstallOperationData{PHPVersion: req.PHPVersion, ConfirmedInstalls: req.ConfirmedInstalls})
	if err != nil {
		return ""
	}
	return string(data)
}

// restoreServiceInstallChoices puts a recorded install's choices back on a
// resumed request; anything that does not validate is dropped (the install
// then keeps the check with nothing confirmed).
func restoreServiceInstallChoices(req *serviceInstallRequest, raw string) {
	if raw == "" {
		return
	}
	var data serviceInstallOperationData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return
	}
	candidate := *req
	candidate.PHPVersion, candidate.ConfirmedInstalls = data.PHPVersion, data.ConfirmedInstalls
	if validServiceInstallChoices(candidate) != "" {
		return
	}
	*req = candidate
}

// bindChosenWebmailPHP moves webmail to the PHP version the owner chose in
// its install window (alpha.124). The bind proves it there; true when it did
// (nothing more to prove). A bind that fails leaves webmail where it was and
// is carried in the result as php_tool_bind, so the finished install says
// which version it runs on and why.
// Web postasını kurulum penceresinde seçilen PHP sürümüne taşır.
func (p *Panel) bindChosenWebmailPHP(ctx context.Context, binding transport.ServiceMutationBinding, version string, result serviceOperationResult) bool {
	if version == "" {
		return false
	}
	var bound transport.BindPHPToolResponse
	if err := p.callAgentContext(ctx, "Agent.BindPHPToolV1", &transport.BindPHPToolRequest{
		ServiceMutationBinding: binding,
		Tool:                   "webmail",
		Version:                version,
	}, &bound); err != nil {
		log.Printf("bind webmail to PHP %s after install: %v", version, err)
		result["php_tool_bind"] = map[string]any{"tool": "webmail", "version": version, "bound": false}
		return false
	}
	if bound.Error != "" || !bound.Bound {
		log.Printf("bind webmail to PHP %s after install: %s", version, bound.Error)
		result["php_tool_bind"] = map[string]any{"tool": "webmail", "version": version, "bound": false,
			"previous": bound.Previous, "code": bound.ErrorCode, "detail": bound.Detail}
		return false
	}
	result["php_tool_bind"] = map[string]any{"tool": "webmail", "version": version, "bound": true}
	return true
}

// provePHPToolAfterInstall asks the Agent to prove a PHP tool just
// installed (alpha.105). The install itself stands either way: a tool
// proved nowhere stays installed and its row reads "Not ready" with the
// cause, which the result carries too. An Agent that has no proof (older)
// or does not answer leaves the result without it.
func (p *Panel) provePHPToolAfterInstall(ctx context.Context, binding transport.ServiceMutationBinding, tool string, firstInstall bool, result serviceOperationResult) {
	var proof transport.ProvePHPToolResponse
	if err := p.callAgentContext(ctx, "Agent.ProvePHPToolV1", &transport.ProvePHPToolRequest{
		ServiceMutationBinding: binding,
		Tool:                   tool,
		AllowFallback:          firstInstall,
		AfterInstall:           true,
	}, &proof); err != nil {
		log.Printf("prove %s after install: %v", tool, err)
		return
	}
	if proof.Error != "" {
		log.Printf("prove %s after install: %s", tool, proof.Error)
	}
	result["php_tool_proof"] = proof
}
