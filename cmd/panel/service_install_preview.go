package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
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
	serviceInstallPreviewPath  = "/api/v1/service/install-preview"
	serviceInstallPreviewBound = 30 * time.Second
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
	resp, err := installPackagePreview(p, ctx, transport.ServiceInstallPreviewRequest{Steps: steps})
	if err != nil {
		log.Printf("install preview: %v", err)
		_ = json.NewEncoder(w).Encode(answer)
		return
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
	}, &proof); err != nil {
		log.Printf("prove %s after install: %v", tool, err)
		return
	}
	if proof.Error != "" {
		log.Printf("prove %s after install: %s", tool, proof.Error)
	}
	result["php_tool_proof"] = proof
}
