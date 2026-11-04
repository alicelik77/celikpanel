package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Remove together (alpha.123, D-045). When the components that require the
// one being removed are leaf components nothing else uses (phpPgAdmin when
// PostgreSQL goes; Roundcube and Rspamd when Postfix goes), the removal runs
// on the server as ONE durable operation: the settings turned off first, then
// every dependent, then the target, each step checked against a fresh scan.
// It replaces the browser loop of alpha.118, where a closed tab left the
// dependents behind. One Agent lease covers it (kind service_remove_together,
// target = the component, qualifier = the canonical member list), and the
// Agent accepts only members the dependency map says require the target.
//
// Resume: every step is idempotent (a component already gone is skipped), so
// an interrupted operation runs the same plan again; the blockers are checked
// again first, so nothing that became used in the meantime is removed.
//
// Birlikte kaldırma: bağımlı yaprak bileşenler ve hedef, sunucuda tek kalıcı
// işlemde, önce bağımlılar, her adım taze taramayla denetlenerek kaldırılır.

const (
	serviceOperationKindRemoveTogether = "service_remove_together"
	errCodeRemovalNeedsTogether        = "REMOVAL_NEEDS_TOGETHER"
	errCodeRemovalStillInstalled       = "removal_still_installed"
	errCodeRemovalBlockedOnResume      = "removal_blocked"
)

type removeTogetherData struct {
	Settings []string `json:"settings,omitempty"`
}

func encodeRemoveTogetherData(plan removalPlan) string {
	data := removeTogetherData{}
	for _, item := range plan.Together {
		if strings.HasPrefix(item, "setting:") {
			data.Settings = append(data.Settings, item)
		}
	}
	raw, _ := json.Marshal(data)
	return string(raw)
}

func decodeRemoveTogetherData(raw string) removeTogetherData {
	var data removeTogetherData
	_ = json.Unmarshal([]byte(raw), &data)
	kept := []string{}
	for _, setting := range data.Settings {
		if setting == core.TogetherMalwareSchedule {
			kept = append(kept, setting)
		}
	}
	data.Settings = kept
	return data
}

// writeRemovalPlanRefusal answers a removal the plan refuses: every blocker
// by name in Details, and the plan itself so the screen can link each one.
func writeRemovalPlanRefusal(w http.ResponseWriter, code, message string, plan removalPlan) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message, "code": code, "details": plan.BlockerLines(), "plan": plan,
	})
}

// startRemoveTogether queues the durable operation and hands it the
// mutation lock (release). It answers 202 with the operation.
func (p *Panel) startRemoveTogether(w http.ResponseWriter, r *http.Request, serviceID, requestID string, plan removalPlan, release func()) bool {
	if requestID != "" && !validServiceOperationID(requestID) {
		writeClientError(w, http.StatusBadRequest, "invalid request_id")
		return false
	}
	qualifier := core.RemoveTogetherQualifier(plan.TogetherComponents())
	if requestID != "" {
		existing, found, err := p.idempotentServiceOperation(r.Context(), requestID, serviceOperationKindRemoveTogether, serviceID, qualifier)
		if errors.Is(err, errServiceOperationRequestConflict) {
			writeServiceOperationRequestConflict(w)
			return false
		}
		if err != nil {
			writeServerError(w, err)
			return false
		}
		if found {
			writeAcceptedServiceOperation(w, existing)
			return false
		}
	}
	actor := captureServiceOperationActor(r)
	op, err := p.createServiceOperationRequestWithState(r.Context(), serviceOperationKindRemoveTogether,
		serviceID, qualifier, requestID, actor, "queued", encodeRemoveTogetherData(plan))
	switch {
	case errors.Is(err, errServiceOperationBusy):
		writeServiceOperationBusy(w)
		return false
	case errors.Is(err, errServiceOperationReplay):
		writeAcceptedServiceOperation(w, op)
		return false
	case errors.Is(err, errServiceOperationRequestConflict):
		writeServiceOperationRequestConflict(w)
		return false
	case err != nil:
		writeServerError(w, err)
		return false
	}
	p.launchServiceOperation(op, actor, "removing",
		"service.uninstall.together:"+serviceID+pkgSuffix(qualifier),
		"service.uninstall.together.failed:"+serviceID+pkgSuffix(qualifier),
		release, p.removeTogetherRunner(op))
	writeAcceptedServiceOperation(w, op)
	return true
}

// removeTogetherRunner rebuilds the run from the operation row (also used
// to resume after a restart).
func (p *Panel) removeTogetherRunner(op serviceOperation) serviceOperationRunner {
	return func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
		members := []string{}
		if op.PackageName != "" {
			parsed, err := core.ParseRemoveTogetherQualifier(op.PackageName)
			if err != nil {
				return serviceOperationResult{"success": false}, operationFailure("removal_plan_invalid", "The recorded removal plan is not valid; nothing more was removed.", err)
			}
			members = parsed
		}
		data := decodeRemoveTogetherData(op.OperationData)
		return p.runRemoveTogether(ctx, op.ServiceID, members, data.Settings, advance)
	}
}

// runRemoveTogether: settings, dependents, target; each removal checked.
func (p *Panel) runRemoveTogether(ctx context.Context, target string, members, settings []string, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
	removed := []string{}
	result := func(success bool) serviceOperationResult {
		return serviceOperationResult{"success": success, "removed": append([]string{}, removed...), "target": target, "together": members}
	}
	svc := core.GetManagedServiceByID(target)
	if svc == nil {
		return result(false), operationFailure("removal_plan_invalid", "The component to remove is not known.", nil)
	}
	// The plan is checked again: a site, a database or another use that
	// appeared since the confirmation refuses the rest (D-045).
	services, err := p.scanManagedServices(ctx)
	if err != nil {
		return result(false), operationFailure(errCodeServiceStateRefreshFailed, "The server's components could not be read, so nothing was removed. Try again.", err)
	}
	installed := map[string]bool{}
	for _, service := range services {
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	plan, err := computeRemovalPlan(ctx, p.db.GetDB(), svc, "", installed, p.removalHostFacts(ctx, target))
	if err != nil {
		return result(false), operationFailure("removal_plan_unreadable", "What requires this component could not be read, so nothing was removed. Try again.", err)
	}
	if len(plan.Blockers) > 0 {
		return result(false), operationFailure(errCodeRemovalBlockedOnResume,
			"Something now requires this component ("+strings.Join(plan.BlockerLines(), ", ")+"), so nothing more was removed. Move or remove it first, then remove again.", nil)
	}
	for _, setting := range settings {
		if setting != core.TogetherMalwareSchedule {
			continue
		}
		if err := advance("setting:malware_schedule"); err != nil {
			return result(false), operationAdvanceFailure(err)
		}
		schedule := p.readMalwareSchedule(ctx)
		if schedule.Enabled {
			schedule.Enabled = false
			if err := p.writeMalwareSchedule(ctx, schedule); err != nil {
				return result(false), operationFailure("setting_failed", "The scheduled malware scan could not be turned off, so nothing was removed. Try again.", err)
			}
		}
	}
	for _, id := range append(append([]string{}, members...), target) {
		if err := advance("removing:" + id); err != nil {
			return result(false), operationAdvanceFailure(err)
		}
		if !installed[id] {
			// Already gone (a resumed run, or removed by hand): nothing to do.
			removed = append(removed, id)
			p.withdrawAfterRemoval(ctx, id, "")
			continue
		}
		if failure := p.removeComponentInOperation(ctx, id); failure != nil {
			return result(false), failure
		}
		services, err := p.scanManagedServices(ctx)
		if err != nil {
			return result(false), operationFailure(errCodeServiceStateRefreshFailed,
				"The server's components could not be read after removing "+componentDisplayName(id)+", so whether it is gone is not known. The operation stops here; open Components to see the state, then remove again.", err)
		}
		if verifyManagedServiceInstalled(services, id) {
			return result(false), operationFailure(errCodeRemovalStillInstalled,
				componentDisplayName(id)+" is still installed after its removal, so nothing more was removed. Look at its page for the reason, then remove again.", nil)
		}
		installed[id] = false
		removed = append(removed, id)
		p.withdrawAfterRemoval(ctx, id, "")
	}
	return result(true), nil
}

// withdrawAfterRemoval takes the removed component's offering items out of
// every rights set (plans, reseller narrowings, custom sets).
func (p *Panel) withdrawAfterRemoval(ctx context.Context, serviceID, version string) int64 {
	n, err := withdrawRemovedOfferings(ctx, p.db.GetDB(), serviceID, version)
	if err != nil {
		log.Printf("removal: offerings of %s%s could not be withdrawn: %v", serviceID, pkgSuffix(version), err)
		return 0
	}
	if n > 0 {
		log.Printf("removal: %d offering entries of %s%s withdrawn", n, serviceID, pkgSuffix(version))
	}
	return n
}

// removeComponentInOperation removes one whole component under the running
// operation's Agent lease, with the follow-up wiring the single removal does
// (mail filter chain, database tools, webmail address).
func (p *Panel) removeComponentInOperation(ctx context.Context, id string) *serviceOperationFailure {
	name := componentDisplayName(id)
	if id == "roundcube" {
		err := p.withStandaloneAgentMutation(ctx, "service_uninstall", "roundcube", "", func(callCtx context.Context, binding agentMutationBinding) error {
			request := transport.WebmailMutationRequest{ServiceMutationBinding: binding}
			var removed transport.RemoveRoundcubeResponse
			if err := p.callAgentContext(callCtx, "Agent.RemoveRoundcube", &request, &removed); err != nil {
				return err
			}
			if removed.Error != "" {
				return errors.New(removed.Error)
			}
			var webmail transport.ConfigureWebmailResponse
			if err := p.callAgentContext(callCtx, "Agent.ConfigureWebmail", &request, &webmail); err != nil {
				return err
			}
			if webmail.Error != "" {
				return errors.New(webmail.Error)
			}
			return nil
		})
		if err != nil {
			return operationFailure("removal_failed", name+" could not be removed, so nothing more was removed: "+auditReason(err.Error()), err)
		}
		if err := p.clearWebmailAddress(ctx); err != nil {
			log.Printf("forget webmail address after roundcube removal: %v", err)
		}
		return nil
	}
	var resp transport.UninstallServiceResponse
	err := p.withStandaloneAgentMutation(ctx, "service_uninstall", id, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.InstallServiceRequest{ServiceMutationBinding: binding, ID: id}
		if err := p.callAgentContext(callCtx, "Agent.UninstallService", &request, &resp); err != nil {
			return err
		}
		if resp.Error != "" && !resp.MutationApplied {
			return errors.New(resp.Error)
		}
		return nil
	})
	if err != nil {
		code := "removal_failed"
		if refusal, _, ok := uninstallRefusalCode(resp); ok {
			code = strings.ToLower(refusal)
		}
		return operationFailure(code, name+" could not be removed, so nothing more was removed: "+auditReason(err.Error()), err)
	}
	if svc := core.GetManagedServiceByID(id); uninstallRewiresMailFilters(svc) {
		var wire transport.WireMailFiltersResponse
		if err := p.withStandaloneAgentMutation(ctx, "mail_filter_wire", id, "", func(callCtx context.Context, binding agentMutationBinding) error {
			request := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
			if err := p.callAgentContext(callCtx, "Agent.WireMailFilters", &request, &wire); err != nil {
				return err
			}
			if wire.Error != "" {
				return errors.New(wire.Error)
			}
			return nil
		}); err != nil {
			return operationFailure("mail_filter_sync_failed", fmt.Sprintf("%s was removed, but the mail filter chain could not be written again: %s", name, auditReason(err.Error())), err)
		}
	}
	if uninstallReconfiguresDatabaseTools(id) {
		var tools transport.ConfigureDBToolsResponse
		if err := p.withStandaloneAgentMutation(ctx, "dbtools_configure", id, "", func(callCtx context.Context, binding agentMutationBinding) error {
			request := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
			if err := p.callAgentContext(callCtx, "Agent.ConfigureDBTools", &request, &tools); err != nil {
				return err
			}
			if tools.Error != "" {
				return errors.New(tools.Error)
			}
			return nil
		}); err != nil {
			log.Printf("db tools configure after %s removal: %v", id, err)
		}
	}
	return nil
}
