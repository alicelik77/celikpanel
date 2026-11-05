package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// Install at component level (alpha.124, D-045 follow-up). The install window
// asks one question, the mirror of removal's: what does this component need?
// The answer is the component dependency map (internal/core/
// component_rules.go) against what is installed here, said per requirement:
//
//   - installed: a component that fills it is already on this server;
//   - together: it is missing and can be installed here on its own, so the
//     install installs it first, in the same durable operation;
//   - unavailable: it is missing and cannot be installed from this window,
//     with the reason and where it is done instead (the mail setup, another
//     component in the same role, a system this component is not offered on).
//
// Low-level packages are not part of this answer; the package manager's own
// simulation (install-preview) is read for what needs the owner's consent (a
// repository change).
//
// "Install together" runs on the server as ONE durable operation
// (service_install_together): one Agent lease whose qualifier is the member
// list, requirements first, each install checked by a fresh scan, the plan
// checked again before any step and on resume. A failure leaves the earlier
// successful installs in place and says which they are.
//
// Bileşen düzeyinde kurulum: pencere tek soru sorar, bu bileşene ne gerekir?
// Kurulu, birlikte kurulacak ya da burada kurulamaz (nedeni ve nerede
// yapıldığıyla). Birlikte kurulum sunucuda tek kalıcı işlemdir.

const (
	serviceOperationKindInstallTogether = "service_install_together"
	serviceInstallPlanPath              = "/api/v1/service/install-plan"

	installNeedInstalled   = "installed"
	installNeedTogether    = "together"
	installNeedUnavailable = "unavailable"
	installNeedUnknown     = "unknown"

	// Why a missing requirement is not installed from this window.
	installNeedReasonMailSetup  = "mail_setup"  // mail engines come with the mail setup
	installNeedReasonNotOffered = "not_offered" // not offered on this system
	installNeedReasonConflict   = "conflict"    // another component holds its role
	installNeedReasonElsewhere  = "elsewhere"   // installed from its own place (runtimes, DNS)

	errCodeInstallPlanChanged = "INSTALL_PLAN_CHANGED"
	errCodeInstallBlocked     = "INSTALL_REQUIREMENT_UNAVAILABLE"
)

type installRequirement struct {
	// Need is the requirement as the map says it: a component id or a role
	// ("web-server").
	Need string `json:"need"`
	// Component fills it: the installed provider, the one to install, or
	// (unavailable) the one that would.
	Component string `json:"component,omitempty"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	// Holder: for a conflict, the installed component that holds the role.
	Holder string `json:"holder,omitempty"`
	// For: the component that needs it ("" for the target itself).
	For string `json:"for,omitempty"`
}

type installPlan struct {
	Target   string               `json:"target"`
	Requires []installRequirement `json:"requires"`
	// Together are installed first, in this order, then the target.
	Together []string `json:"together"`
	// Blocked: a requirement cannot be installed from here (or is unknown).
	Blocked bool `json:"blocked"`
}

// installPlanHost is what the plan reads about this server.
type installPlanHost struct {
	Installed map[string]bool
	// Known is false when this server's components were never observed.
	Known bool
	// Blocked says why a component is not offered here ("" when it is).
	Blocked func(id string) string
}

func mailSeatOrEngine(token string) bool {
	switch token {
	case core.SeatSMTPServer, core.SeatIMAPServer, "postfix", "exim", "dovecot":
		return true
	}
	return false
}

// planComponentInstall answers "what does target need?" from the map. Pure:
// the caller gives the installed set and the host's offers.
func planComponentInstall(target string, host installPlanHost) installPlan {
	plan := installPlan{Target: target, Requires: []installRequirement{}, Together: []string{}}
	seen := map[string]bool{}
	var visit func(id string, depth int)
	visit = func(id string, depth int) {
		if depth > 4 {
			return
		}
		forID := ""
		if id != target {
			forID = id
		}
		for _, need := range core.RuleFor(id).Requires {
			key := forID + "|" + need
			if seen[key] {
				continue
			}
			seen[key] = true
			requirement := installRequirement{Need: need, For: forID}
			providers := []string{need}
			if core.GetManagedServiceByID(need) == nil {
				providers = core.SeatProviders(need)
			}
			for _, provider := range providers {
				if host.Installed[provider] || slices.Contains(plan.Together, provider) {
					requirement.Component, requirement.State = provider, installNeedInstalled
					if !host.Installed[provider] {
						requirement.State = installNeedTogether
					}
					break
				}
			}
			if requirement.State == "" && !host.Known {
				requirement.State = installNeedUnknown
				plan.Blocked = true
			}
			if requirement.State == "" {
				requirement.State, requirement.Component, requirement.Reason = installNeedUnavailable, providers[0], installNeedReasonElsewhere
				for _, provider := range providers {
					svc := core.GetManagedServiceByID(provider)
					switch {
					case svc == nil:
						continue
					case mailSeatOrEngine(need) || mailSeatOrEngine(provider):
						requirement.Component, requirement.Reason = provider, installNeedReasonMailSetup
					case host.Blocked != nil && host.Blocked(provider) != "":
						if requirement.Reason == installNeedReasonElsewhere {
							requirement.Component, requirement.Reason = provider, installNeedReasonNotOffered
						}
						continue
					case core.SeatTakenBy(svc, host.Installed) != "":
						requirement.Component, requirement.Reason = provider, installNeedReasonConflict
						for _, other := range core.SeatProviders(svc.ConflictGroup) {
							if host.Installed[other] {
								requirement.Holder = other
							}
						}
						continue
					case core.InstallableTogether(provider):
						requirement.State, requirement.Component, requirement.Reason = installNeedTogether, provider, ""
					default:
						continue
					}
					break
				}
				if requirement.State == installNeedTogether {
					visit(requirement.Component, depth+1)
					if !slices.Contains(plan.Together, requirement.Component) {
						plan.Together = append(plan.Together, requirement.Component)
					}
				} else {
					plan.Blocked = true
				}
			}
			plan.Requires = append(plan.Requires, requirement)
		}
	}
	visit(target, 0)
	return plan
}

// TogetherQualifier is the plan's member list as the lease names it.
func (plan installPlan) TogetherQualifier() string {
	return core.InstallTogetherQualifier(plan.Together)
}

// UnavailableLines name what cannot be installed here, for a refusal.
func (plan installPlan) UnavailableLines() []string {
	lines := []string{}
	for _, requirement := range plan.Requires {
		if requirement.State == installNeedUnavailable || requirement.State == installNeedUnknown {
			lines = append(lines, componentDisplayName(requirement.Component)+": "+requirement.Reason)
		}
	}
	return lines
}

// componentInstallPlan reads this server: the stored component record, or a
// fresh scan when fresh is set or there is none.
func (p *Panel) componentInstallPlan(ctx context.Context, target string, fresh bool) (installPlan, error) {
	var installed map[string]bool
	if !fresh {
		installed = p.cachedInstalledSet(ctx)
	}
	if installed == nil {
		services, err := p.scanManagedServices(ctx)
		if err != nil {
			return installPlan{}, err
		}
		installed = map[string]bool{}
		for _, service := range services {
			if service.Installed() {
				installed[service.ID] = true
			}
		}
	}
	hostProfile := p.managedServiceHostProfile()
	return planComponentInstall(target, installPlanHost{
		Installed: installed,
		Known:     true,
		Blocked: func(id string) string {
			_, reason := core.ManagedServiceInstallBlockForHost(core.GetManagedServiceByID(id), hostProfile)
			return reason
		},
	}), nil
}

// handleServiceInstallPlan: GET /api/v1/service/install-plan?id=<component>.
// Read only; the install window draws it.
func (p *Panel) handleServiceInstallPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if core.GetManagedServiceByID(id) == nil {
		writeClientError(w, http.StatusBadRequest, "unknown managed service")
		return
	}
	plan, err := p.componentInstallPlan(r.Context(), id, false)
	if err != nil {
		writeServerError(w, fmt.Errorf("read components for the install plan: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"plan": plan})
}

// writeInstallPlanRefusal answers an install the plan refuses or that names
// another member list than the plan's, with the plan so the window redraws.
func writeInstallPlanRefusal(w http.ResponseWriter, code, message string, plan installPlan) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message, "code": code, "details": plan.UnavailableLines(), "plan": plan,
	})
}

// startInstallTogether checks the owner's member list against the plan and
// queues the durable operation. It answers the request itself.
func (p *Panel) startInstallTogether(w http.ResponseWriter, r *http.Request, req serviceInstallRequest) {
	if req.Package != "" {
		writeClientError(w, http.StatusBadRequest, "install_together installs the distribution's version; a version package is chosen on its own install")
		return
	}
	qualifier := core.InstallTogetherQualifier(req.InstallTogether)
	if _, err := core.ParseInstallTogetherQualifier(req.ServiceID, qualifier); err != nil {
		writeClientError(w, http.StatusBadRequest, "install_together does not name requirements of this component")
		return
	}
	for _, member := range req.InstallTogether {
		if !requireAdministratorForComponent(w, r, member) {
			return
		}
	}
	existing, found, err := p.idempotentServiceOperation(r.Context(), req.RequestID, serviceOperationKindInstallTogether, req.ServiceID, qualifier)
	if errors.Is(err, errServiceOperationRequestConflict) {
		writeServiceOperationRequestConflict(w)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if found {
		writeAcceptedServiceOperation(w, existing)
		return
	}
	if managedDNSEngineServiceID(req.ServiceID) {
		writeDNSEngineWorkflowRequired(w)
		return
	}
	for _, id := range append(append([]string{}, req.InstallTogether...), req.ServiceID) {
		phpVersion := ""
		if id == req.ServiceID {
			phpVersion = req.PHPVersion
		}
		if p.refuseWithdrawnInstall(w, r, id, "", phpVersion) {
			return
		}
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	releaseInHandler := true
	defer func() {
		if releaseInHandler {
			release()
		}
	}()
	plan, err := p.componentInstallPlan(r.Context(), req.ServiceID, true)
	if err != nil {
		writeServerError(w, fmt.Errorf("read components before installing: %w", err))
		return
	}
	if plan.Blocked {
		writeInstallPlanRefusal(w, errCodeInstallBlocked,
			"A requirement of this component cannot be installed here, so nothing was installed.", plan)
		return
	}
	if plan.TogetherQualifier() != qualifier {
		writeInstallPlanRefusal(w, errCodeInstallPlanChanged,
			"What this component needs changed since the window was opened, so nothing was installed. Look at the list again and confirm it.", plan)
		return
	}
	actor := captureServiceOperationActor(r)
	op, err := p.createServiceOperationRequestWithState(r.Context(), serviceOperationKindInstallTogether,
		req.ServiceID, qualifier, req.RequestID, actor, "queued", encodeServiceInstallOperationData(req))
	switch {
	case errors.Is(err, errServiceOperationBusy):
		writeServiceOperationBusy(w)
		return
	case errors.Is(err, errServiceOperationReplay):
		writeAcceptedServiceOperation(w, op)
		return
	case errors.Is(err, errServiceOperationRequestConflict):
		writeServiceOperationRequestConflict(w)
		return
	case err != nil:
		writeServerError(w, err)
		return
	}
	order := append([]string{}, plan.Together...)
	p.launchServiceOperation(op, actor, "installing",
		"service.install.together:"+req.ServiceID+pkgSuffix(qualifier),
		"service.install.together.failed:"+req.ServiceID+pkgSuffix(qualifier),
		release, func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runInstallTogether(ctx, req, order, advance)
		})
	releaseInHandler = false
	writeAcceptedServiceOperation(w, op)
}

// installTogetherRunner rebuilds the run from the operation row (resume).
func (p *Panel) installTogetherRunner(op serviceOperation) (serviceOperationRunner, error) {
	members, err := core.ParseInstallTogetherQualifier(op.ServiceID, op.PackageName)
	if err != nil {
		return nil, err
	}
	req := serviceInstallRequest{ServiceID: op.ServiceID, RequestID: op.RequestID, InstallTogether: members}
	restoreServiceInstallChoices(&req, op.OperationData)
	return func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
		return p.runInstallTogether(ctx, req, members, advance)
	}, nil
}

// runInstallTogether: the plan again, each missing requirement, the target;
// every install is the ordinary one (preflight, Agent, configuration, scan).
func (p *Panel) runInstallTogether(ctx context.Context, req serviceInstallRequest, order []string, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
	installed := []string{}
	result := func(success bool) serviceOperationResult {
		return serviceOperationResult{"success": success, "installed_together": append([]string{}, installed...),
			"target": req.ServiceID, "together": req.InstallTogether}
	}
	kept := func(failure *serviceOperationFailure) *serviceOperationFailure {
		if failure != nil && len(installed) > 0 {
			names := make([]string, 0, len(installed))
			for _, id := range installed {
				names = append(names, componentDisplayName(id))
			}
			failure.Message = strings.TrimSpace(failure.Message + " Installed before this step and kept: " + strings.Join(names, ", ") + ".")
		}
		return failure
	}
	// The plan is read again with a fresh scan: a requirement that became
	// unavailable, or a new one nobody confirmed, stops the run before any
	// install (D-045).
	plan, err := p.componentInstallPlan(ctx, req.ServiceID, true)
	if err != nil {
		return result(false), operationFailure(errCodeServiceStateRefreshFailed, "The server's components could not be read, so nothing was installed. Try again.", err)
	}
	if plan.Blocked {
		return result(false), operationFailure(strings.ToLower(errCodeInstallBlocked),
			"A requirement of "+componentDisplayName(req.ServiceID)+" cannot be installed here now ("+strings.Join(plan.UnavailableLines(), ", ")+"), so nothing was installed. Open the install window again for the reason.", nil)
	}
	for _, id := range plan.Together {
		if !slices.Contains(req.InstallTogether, id) {
			return result(false), operationFailure(strings.ToLower(errCodeInstallPlanChanged),
				componentDisplayName(req.ServiceID)+" now also needs "+componentDisplayName(id)+", which was not confirmed, so nothing was installed. Open the install window again and confirm the list.", nil)
		}
	}
	for _, id := range order {
		if !slices.Contains(plan.Together, id) {
			// Installed meanwhile (a resumed run, or by hand): nothing to do.
			continue
		}
		step := func(phase string) error { return advance("installing:" + id) }
		if err := step(""); err != nil {
			return result(false), operationAdvanceFailure(err)
		}
		_, failure := p.runServiceInstall(ctx, serviceInstallRequest{
			ServiceID: id, RequestID: req.RequestID, CheckInstalls: true, ConfirmedInstalls: req.ConfirmedInstalls,
		}, step)
		if failure != nil {
			return result(false), kept(failure)
		}
		installed = append(installed, id)
	}
	if err := advance("installing:" + req.ServiceID); err != nil {
		return result(false), kept(operationAdvanceFailure(err))
	}
	targetResult, failure := p.runServiceInstall(ctx, req, func(string) error { return advance("installing:" + req.ServiceID) })
	if failure != nil {
		out := result(false)
		for key, value := range targetResult {
			if key != "success" {
				out[key] = value
			}
		}
		return out, kept(failure)
	}
	out := result(true)
	for key, value := range targetResult {
		out[key] = value
	}
	out["installed_together"] = append([]string{}, installed...)
	return out, nil
}
