package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// D-031 second step (2026-10-11): the site's PHP-FPM pool file and its
// application unit file follow the vhost's rule. A pool save, a PHP version
// switch or a hosting change that would rewrite a file the owner changed is
// answered with a typed refusal whose reason is the file's kind (php_pool,
// app_unit) and whose detail is its state; the domain's Configuration file
// page lists every file of the site with its state and actions; each action
// names its file (kind).
//
// D-031 ikinci adım: sitenin PHP-FPM havuz dosyası ve uygulama birimi dosyası
// vhost ile aynı kurala uyar; ret, gerekçe olarak dosyanın türünü taşır.

const (
	errCodeSiteConfigServiceRefused = "SITE_CONFIG_SERVICE_REFUSED"
	// siteConfigReasonNotRead: the Agent was asked for one of the site's
	// other files and did not answer; the page shows that file's state as
	// unknown and the others as read.
	siteConfigReasonNotRead = "not_read"
)

// The English originals of the pool's and the unit's refusals; the screens'
// sentences are in web/src/i18n (err.<CODE>.php_pool, err.<CODE>.app_unit).
var otherSiteFileMessages = map[string]map[string]string{
	transport.SiteFileKindPHPPool: {
		errCodeSiteConfigOwnerEdited: "The site's PHP-FPM pool file is not CelikPanel's unchanged text, so CelikPanel kept it and did not apply this change to it. " +
			"Nothing else was changed. Open the domain's Configuration file section: keep your file, take CelikPanel's text (your file is kept as a dated copy), or merge the two by hand.",
		errCodeSiteConfigMissing: "The site's PHP-FPM pool file is missing, so this change was not applied and the file was not recreated. " +
			"Open the domain's Configuration file section and choose Recreate.",
		errCodeSiteConfigUnwritable: "CelikPanel could not read or replace the site's PHP-FPM pool file (for example, it is a link or it is locked against changes), so it kept the file as it is and did not apply this change. " +
			"The server administrator checks the file on the server; the reason is shown in the domain's Configuration file section.",
		errCodeSiteConfigExists: "A PHP-FPM pool file for this site already exists for the PHP version you chose and was not left by CelikPanel's switch, so CelikPanel kept it and did not switch. " +
			"Nothing was changed. Move or rename that file on the server if it is no longer used, then switch again.",
		errCodeSiteConfigServiceRefused: "PHP-FPM did not accept CelikPanel's pool text (its configuration check or reload failed), so the previous pool file was put back and PHP-FPM keeps running with it. " +
			"Nothing else was changed. The server administrator reads PHP-FPM's message in the Panel's log, fixes the cause, then tries again.",
		errCodeSiteConfigServiceRefused + siteConfigRestoreUnconfirmedSuffix: "PHP-FPM did not accept CelikPanel's pool text (its configuration check or reload failed). " +
			"CelikPanel tried to put the previous file back; whether it is in place is not confirmed — the server administrator checks the file on the server. " +
			"PHP-FPM's message and what failed while putting the file back are in the Panel's log; fix the cause, then try again.",
	},
	transport.SiteFileKindAppUnit: {
		errCodeSiteConfigOwnerEdited: "The site's application unit file is not CelikPanel's unchanged text, so CelikPanel kept it and did not apply this change to it. " +
			"Nothing else was changed. Open the domain's Configuration file section: keep your file, take CelikPanel's text (your file is kept as a dated copy), or merge the two by hand.",
		errCodeSiteConfigMissing: "The site's application unit file is missing, so this change was not applied and the file was not recreated. " +
			"Open the domain's Configuration file section and choose Recreate.",
		errCodeSiteConfigUnwritable: "CelikPanel could not read or replace the site's application unit file (for example, it is a link or it is locked against changes), so it kept the file as it is and did not apply this change. " +
			"The server administrator checks the file on the server; the reason is shown in the domain's Configuration file section.",
		errCodeSiteConfigExists: "An application unit file for this site already exists and was not written by CelikPanel, so CelikPanel kept it and did not apply this change. " +
			"Nothing was changed. Open the domain's Configuration file section.",
		errCodeSiteConfigServiceRefused: "systemd did not take CelikPanel's unit text (its reload failed), so the previous unit file was put back. " +
			"Nothing else was changed. The server administrator reads systemd's message in the Panel's log, fixes the cause, then tries again.",
		errCodeSiteConfigServiceRefused + siteConfigRestoreUnconfirmedSuffix: "systemd did not take CelikPanel's unit text (its reload failed). " +
			"CelikPanel tried to put the previous file back; whether it is in place is not confirmed — the server administrator checks the file on the server. " +
			"systemd's message and what failed while putting the file back are in the Panel's log; fix the cause, then try again.",
	},
}

// siteConfigRestoreUnconfirmedSuffix keys the sentence of a service refusal
// whose restore was not confirmed (restore: unconfirmed).
const siteConfigRestoreUnconfirmedSuffix = ".unconfirmed"

// The sentences of a confirmed put-back after which the service was not
// reloaded (set12 cell RP1), keyed "<service>[.<service_reason>]" per kind;
// "%s" is the check's line (only an administrator's answer names it). The
// screens' sentences are in the screen catalogue (web/src/i18n/screens/server,
// siteConfig.service.<kind>.<service>[.<service_reason>]).
var siteFileServiceMessages = map[string]map[string]string{
	transport.SiteFileKindPHPPool: {
		"not_reloaded.config_refused_elsewhere": "The previous file is back. PHP-FPM was not reloaded, because its configuration is refused by another file — " +
			"PHP-FPM keeps running with the configuration it loaded before. Fix what the check names (%s), then reload PHP-FPM yourself.",
		"not_reloaded": "The previous file is back. PHP-FPM was not reloaded, because its configuration is refused by another file — " +
			"PHP-FPM keeps running with the configuration it loaded before. The server administrator fixes what PHP-FPM's check names, then reloads PHP-FPM.",
		"not_running": "The previous file is back. PHP-FPM was not running before this change, so CelikPanel neither reloaded nor started it. " +
			"Start PHP-FPM yourself, then choose again.",
	},
	transport.SiteFileKindAppUnit: {
		"not_reloaded.config_refused_elsewhere": "The previous unit file is back and systemd has read it again. The application was not restarted, because systemd-analyze verify refuses the unit — " +
			"the application keeps running as it was started. Fix what the check names (%s), then restart the application yourself.",
		"not_reloaded.check_unavailable": "The previous unit file is back and systemd has read it again. The application was not restarted, because this server has no systemd-analyze to check the unit first — " +
			"the application keeps running as it was started. Restart the application yourself once you have checked the unit.",
		"not_reloaded": "The previous unit file is back and systemd has read it again. The application was not restarted, because the unit's check did not pass — " +
			"the application keeps running as it was started. The server administrator fixes what the check names, then restarts the application.",
		"not_running": "The previous unit file is back and systemd has read it again. The application was not running before this change, so CelikPanel did not start it. " +
			"Start it yourself when it should run.",
	},
	transport.SiteFileKindNginxVhost: {
		"not_reloaded.config_refused_elsewhere": "The previous file is back. nginx was not reloaded, because its configuration is refused by another file — " +
			"nginx keeps running with the configuration it loaded before. Fix what the check names (%s), then reload nginx yourself.",
		"not_reloaded": "The previous file is back. nginx was not reloaded, because its configuration is refused by another file — " +
			"nginx keeps running with the configuration it loaded before. The server administrator fixes what nginx -t names, then reloads nginx.",
		"not_running": "The previous file is back. nginx was not running before this change, so CelikPanel neither reloaded nor started it. " +
			"Start nginx yourself, then choose again.",
	},
}

// siteFileServiceActive is the answer's service_active: the Agent's
// `systemctl is-active` after the operation, "unknown" when it gave none.
func siteFileServiceActive(file transport.SiteFileResult) string {
	switch file.ServiceActive {
	case transport.SiteFileServiceActiveTrue, transport.SiteFileServiceActiveFalse:
		return file.ServiceActive
	}
	return transport.SiteFileServiceActiveUnknown
}

// siteFileServiceSentence is the sentence of a confirmed put-back after which
// the service was deliberately not reloaded, without and with the check's
// line (the second only when there is a line); "" for any other answer.
func siteFileServiceSentence(file transport.SiteFileResult, restore string) (string, string) {
	if restore != transport.SiteFileRestoreConfirmed {
		return "", ""
	}
	messages := siteFileServiceMessages[file.Kind]
	if file.Kind == "" {
		messages = siteFileServiceMessages[transport.SiteFileKindNginxVhost]
	}
	switch file.Service {
	case transport.SiteFileServiceNotRunning:
		return messages["not_running"], ""
	case transport.SiteFileServiceNotReloaded:
		plain := messages["not_reloaded"]
		if specific, ok := messages["not_reloaded."+file.ServiceReason]; ok {
			if !strings.Contains(specific, "%s") {
				return specific, ""
			}
			if file.ServiceCheck != "" {
				return plain, fmt.Sprintf(specific, boundedAgentDiagnostic(file.ServiceCheck))
			}
		}
		return plain, ""
	}
	return "", ""
}

// classifyOtherSiteFileHeld is the typed refusal of a pool or unit the rule
// did not write.
func classifyOtherSiteFileHeld(file transport.SiteFileResult, agentError string) agentRPCPlatformErrorClassification {
	classification := agentRPCPlatformErrorClassification{Status: http.StatusConflict, Reason: file.Kind}
	switch {
	case file.Outcome == transport.SiteFileOutcomeKept:
		classification.Code = errCodeSiteConfigOwnerEdited
		classification.Detail = file.State
	case file.Outcome == transport.SiteFileOutcomeMissing:
		classification.Code = errCodeSiteConfigMissing
	case file.Outcome == transport.SiteFileOutcomeRefused && file.Reason == transport.SiteFileReasonNotApplicable:
		classification.Code = errCodeSiteConfigExists
		classification.Detail = file.State
	case file.Outcome == transport.SiteFileOutcomeRefused && file.Reason == transport.SiteFileReasonChanged:
		return agentRPCPlatformErrorClassification{
			Status: http.StatusConflict, Code: errCodeSiteConfigChanged, Message: siteConfigChangedMessage,
		}
	case file.Outcome == transport.SiteFileOutcomeFailed:
		classification.Status = http.StatusBadGateway
		classification.Code = errCodeSiteConfigServiceRefused
		classification.Detail = file.Reason
		// Whether the previous file is back is said, never assumed: the
		// Agent's "rollback incomplete" (or no restore fact at all) is
		// "unconfirmed", with the Agent's line for an administrator.
		classification.Restore = siteFileRestore(file, agentError)
		classification.AdminDetails = siteFileAgentDetails(agentError)
		// What happened to the service after the put-back, and whether it
		// runs now (set12 cell RP1).
		classification.Service, classification.ServiceReason = file.Service, file.ServiceReason
		classification.ServiceActive = siteFileServiceActive(file)
		if plain, withLine := siteFileServiceSentence(file, classification.Restore); plain != "" {
			classification.Message = plain
			if withLine != "" {
				line := boundedAgentDiagnostic(file.ServiceCheck)
				classification.AdminMessage = withLine
				classification.AdminVars = map[string]string{"line": line}
				classification.AdminDetails = append([]string{line}, classification.AdminDetails...)
			}
			return classification
		}
		if classification.Restore != transport.SiteFileRestoreConfirmed {
			classification.Message = otherSiteFileMessages[file.Kind][errCodeSiteConfigServiceRefused+siteConfigRestoreUnconfirmedSuffix]
			return classification
		}
	default:
		classification.Code = errCodeSiteConfigUnwritable
		classification.Detail = file.Reason
		if classification.Detail == "" {
			classification.Detail = file.State
		}
	}
	classification.Message = otherSiteFileMessages[file.Kind][classification.Code]
	return classification
}

// siteFileSite is what the site configuration needs to name a site's files.
type siteFileSite struct {
	SiteID      int
	DomainID    int
	Domain      string
	ProjectType string
	PHPVersion  string
}

func (p *Panel) loadSiteFileSite(ctx context.Context, domainID int) (siteFileSite, error) {
	site := siteFileSite{DomainID: domainID}
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.id, d.name, COALESCE(s.project_type, 'php'), COALESCE(s.php_version, '')
		FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE s.domain_id = ? AND COALESCE(s.project_type, 'php') <> 'dnsonly'`, domainID).
		Scan(&site.SiteID, &site.Domain, &site.ProjectType, &site.PHPVersion)
	return site, err
}

// otherKinds are the site's files besides the vhost: the pool of a PHP site,
// the unit of a Node site.
func (site siteFileSite) otherKinds() []string {
	switch site.ProjectType {
	case "php":
		if services.ValidatePHPVersion(site.PHPVersion) == nil {
			return []string{transport.SiteFileKindPHPPool}
		}
	case "node":
		return []string{transport.SiteFileKindAppUnit}
	}
	return nil
}

func (p *Panel) recordedSiteFileSHA256(ctx context.Context, siteID int, kind string) string {
	record, found, err := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, kind)
	if err != nil || !found {
		return ""
	}
	return record.BodySHA256
}

func (p *Panel) sitePoolRequest(ctx context.Context, site siteFileSite, operation string) transport.SitePoolRequest {
	return transport.SitePoolRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		SiteID:              site.SiteID,
		Username:            services.SiteUsername(site.Domain),
		PHPVersion:          site.PHPVersion,
		Operation:           operation,
		RecordedSHA256:      p.recordedSiteFileSHA256(ctx, site.SiteID, transport.SiteFileKindPHPPool),
	}
}

// hostingAppRequest is the unit request of a Node site's hosting state.
func hostingAppRequest(state hostingRuntimeState) *transport.AppApplyRequest {
	return &transport.AppApplyRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		SiteID:              state.SiteID,
		Description:         state.DomainName,
		WorkDir:             state.DocumentRoot,
		Command:             state.StartCommand.String,
		Port:                int(state.AppPort.Int64),
		NodeVersion:         state.RuntimeVersion.String,
		RunAsUser:           "www-data",
	}
}

func (p *Panel) appUnitRequest(ctx context.Context, domainID int) (*transport.AppApplyRequest, hostingRuntimeState, error) {
	state, err := p.loadHostingRuntimeState(ctx, domainID)
	if err != nil {
		return nil, state, err
	}
	if state.ProjectType != "node" || !state.AppPort.Valid || !state.StartCommand.Valid || !state.RuntimeVersion.Valid {
		return nil, state, errors.New("incomplete node runtime state")
	}
	request := hostingAppRequest(state)
	request.RecordedSHA256 = p.recordedSiteFileSHA256(ctx, state.SiteID, transport.SiteFileKindAppUnit)
	return request, state, nil
}

// recordOtherSiteFile stores and logs a pool's or unit's result; nil is an
// Agent that did not report one (the state is unknown, never "unchanged").
func (p *Panel) recordOtherSiteFile(ctx context.Context, site siteFileSite, kind, trigger string, file *transport.SiteFileResult, unknownReason string) {
	if file == nil || file.Path == "" {
		log.Printf("site configuration %s %s (%s): state unknown (%s)", site.Domain, kind, trigger, unknownReason)
		return
	}
	log.Printf("site configuration %s %s (%s): %s", site.Domain, kind, trigger, describeSiteFile(*file))
	if err := services.RecordSiteFileResult(ctx, p.db.GetDB(), site.SiteID, site.DomainID, *file, siteFileReleaseLabel(), 0); err != nil {
		log.Printf("site configuration %s %s: record the result in the ledger: %v", site.Domain, kind, err)
	}
}

// siteFileOutcomeHeld turns a result the rule did not apply into the typed
// refusal, carrying the Agent's error line (a failed operation's "rollback
// incomplete" decides restore: unconfirmed); an applied result with an Agent
// error is that error.
func siteFileOutcomeHeld(domain string, file transport.SiteFileResult, agentError string) error {
	switch file.Outcome {
	case transport.SiteFileOutcomeKept, transport.SiteFileOutcomeMissing, transport.SiteFileOutcomeRefused,
		transport.SiteFileOutcomeFailed:
		return &siteFileHeldError{Domain: domain, File: file, AgentError: agentError}
	}
	if agentError != "" {
		return errors.New(agentError)
	}
	return nil
}

// applySitePool sends one pool operation and records its result.
func (p *Panel) applySitePool(ctx context.Context, site siteFileSite, request transport.SitePoolRequest) (*transport.SiteFileResult, error) {
	var response transport.ApplySiteFileResponse
	if err := p.callAgentContext(ctx, "Agent.ApplySitePool", &request, &response); err != nil {
		return nil, err
	}
	p.recordOtherSiteFile(ctx, site, transport.SiteFileKindPHPPool, request.Operation, response.File, siteConfigReasonAgentDoesNotReport)
	if response.File == nil {
		if response.Error != "" {
			return nil, errors.New(response.Error)
		}
		return nil, errors.New("the Agent did not report the pool file's state")
	}
	return response.File, siteFileOutcomeHeld(site.Domain, *response.File, response.Error)
}

// applyAppUnit sends one unit operation and records its result. An older
// Agent reports no File: its error, if any, stands.
func (p *Panel) applyAppUnit(ctx context.Context, site siteFileSite, request *transport.AppApplyRequest) (*transport.SiteFileResult, error) {
	var response transport.AppApplyResponse
	if err := p.callAgentContext(ctx, "Agent.ApplyAppUnit", request, &response); err != nil {
		return nil, fmt.Errorf("apply application unit: %w", err)
	}
	trigger := request.Trigger
	if trigger == "" {
		trigger = transport.SiteFileTriggerChange
	}
	p.recordOtherSiteFile(ctx, site, transport.SiteFileKindAppUnit, trigger, response.File, siteConfigReasonAgentDoesNotReport)
	if response.File == nil {
		if response.Error != "" {
			return nil, fmt.Errorf("apply application unit rejected: %s", response.Error)
		}
		return nil, nil
	}
	if err := siteFileOutcomeHeld(site.Domain, *response.File, response.Error); err != nil {
		return response.File, err
	}
	if response.Error != "" {
		return response.File, fmt.Errorf("apply application unit rejected: %s", response.Error)
	}
	return response.File, nil
}

// fileView is the page's shape for one file the Agent read.
func fileView(base siteConfigView, file transport.SiteFileResult, diff string, truncated bool) siteConfigView {
	view := base
	view.Path, view.State, view.Reason, view.Detail = file.Path, file.State, file.Reason, file.Detail
	view.AdoptedFrom, view.IncludeDir = file.AdoptedFrom, file.IncludeDir
	view.FileSHA256, view.RenderSHA256, view.PendingPath = file.FileSHA256, file.RenderSHA256, file.PendingPath
	view.Diff, view.DiffTruncated = diff, truncated
	view.SystemError, view.Protection = file.SystemError, file.Protection
	view.heldRender = file.State == transport.SiteFileManagedUnchanged && file.WrittenSHA256 != file.RenderSHA256
	view.Actions = siteConfigActions(file.State)
	return view
}

// inspectOtherSiteFile asks the Agent for a pool's or unit's state and
// difference. With observe (the page's read) the ledger records what was
// read when it differs from the row (so the Domains row agrees with the
// page) and a refused replacement is checked again; nothing is written on
// the server.
func (p *Panel) inspectOtherSiteFile(ctx context.Context, site siteFileSite, kind string, observe bool) (siteConfigView, *transport.SiteFileResult, error) {
	view := siteConfigView{DomainID: site.DomainID, Domain: site.Domain, Kind: kind, Actions: []string{}}
	var response transport.InspectSiteFileResponse
	var method string
	var err error
	switch kind {
	case transport.SiteFileKindPHPPool:
		method = "Agent.InspectSitePool"
		request := p.sitePoolRequest(ctx, site, transport.SitePoolOperationInspect)
		err = p.callAgentContext(ctx, method, &request, &response)
	case transport.SiteFileKindAppUnit:
		method = "Agent.InspectAppUnit"
		request, _, requestErr := p.appUnitRequest(ctx, site.DomainID)
		if requestErr != nil {
			return view, nil, fmt.Errorf("%s: %w", siteConfigReasonRenderInput, requestErr)
		}
		err = p.callAgentContext(ctx, method, request, &response)
	default:
		return view, nil, fmt.Errorf("unknown site file kind %q", kind)
	}
	if err != nil {
		if agentRPCMethodUnavailable(err, method) {
			view.State = transport.SiteFileStateUnknown
			view.Reason = siteConfigReasonAgentDoesNotReport
			if observe {
				p.recordSiteFileUnread(ctx, site, kind, siteConfigReasonAgentDoesNotReport, "")
			}
			return view, nil, nil
		}
		return view, nil, err
	}
	if response.Error != "" {
		return view, nil, errors.New(response.Error)
	}
	file := response.File
	view = fileView(view, file, response.Diff, response.DiffTruncated)
	if observe {
		p.observeOtherSiteFile(ctx, site, view, file)
	}
	return p.siteConfigViewFor(ctx, site.SiteID, view), &file, nil
}

// observeOtherSiteFile records the page's read of a pool or unit when the row
// says otherwise. A refused replacement is decided by recheckWriteRefusal; a
// protection_removed row on the same bytes stays as it is.
func (p *Panel) observeOtherSiteFile(ctx context.Context, site siteFileSite, view siteConfigView, file transport.SiteFileResult) {
	if file.Path == "" {
		return
	}
	record, found, err := services.LoadSiteFileRecord(ctx, p.db.GetDB(), site.SiteID, file.Kind)
	if err != nil {
		return
	}
	if found && record.Path == file.Path {
		if services.SiteFileWriteRefused(record) {
			p.recheckWriteRefusal(ctx, site.SiteID, view)
			return
		}
		if record.FileSHA256 == file.FileSHA256 && (record.State == services.SiteFileStoredState(file) ||
			record.StateReason == services.SiteFileReasonProtectionRemoved) && record.PendingPath == file.PendingPath {
			return
		}
	}
	if err := services.RecordSiteFileResult(ctx, p.db.GetDB(), site.SiteID, site.DomainID, file, siteFileReleaseLabel(), 0); err != nil {
		log.Printf("site configuration %s %s: record the read: %v", site.Domain, file.Kind, err)
	}
}

// siteFileReadErrorClass names why a read of a site file got no answer:
// "timeout" (the Agent did not answer in time), "render_input_unavailable"
// (the Panel could not prepare the request), "agent_unreachable" (the Agent
// could not be asked) or "agent_error" (it answered with an error).
func siteFileReadErrorClass(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "deadline exceeded") || strings.Contains(text, "timed out"):
		return "timeout"
	case strings.HasPrefix(text, siteConfigReasonRenderInput):
		return siteConfigReasonRenderInput
	case strings.Contains(text, "connection refused") || strings.Contains(text, "no such file or directory") ||
		strings.Contains(text, "connection is shut down") || strings.Contains(text, "unexpected EOF"):
		return "agent_unreachable"
	}
	return "agent_error"
}

// recordSiteFileUnread stores a read that got no answer in the ledger, so the
// Domains row and its badge say what the page says (set12 cell U).
func (p *Panel) recordSiteFileUnread(ctx context.Context, site siteFileSite, kind, reason, class string) {
	// A read that timed out has used up the request's context; the record
	// gets its own short one.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	changed, err := services.RecordSiteFileUnread(ctx, p.db.GetDB(), site.SiteID, kind, reason, class, 0)
	if err != nil {
		log.Printf("site configuration %s %s: record the unread state: %v", site.Domain, kind, err)
		return
	}
	if changed {
		log.Printf("site configuration %s %s: recorded as not read (%s %s)", site.Domain, kind, reason, class)
	}
}

// withOtherSiteFiles adds every file of the site to the vhost's answer, and
// the worst of their states (services.SiteFileWorstState).
func (p *Panel) withOtherSiteFiles(ctx context.Context, domainID int, view siteConfigView) siteConfigView {
	vhost := view
	vhost.Files = nil
	files := []siteConfigView{vhost}
	if site, err := p.loadSiteFileSite(ctx, domainID); err == nil {
		for _, kind := range site.otherKinds() {
			other, _, err := p.inspectOtherSiteFile(ctx, site, kind, true)
			if err != nil {
				log.Printf("site configuration %s %s: could not read its state: %v", site.Domain, kind, err)
				class := siteFileReadErrorClass(err)
				other = siteConfigView{
					DomainID: site.DomainID, Domain: site.Domain, Kind: kind,
					State: transport.SiteFileStateUnknown, Reason: siteConfigReasonNotRead, ReadError: class,
					Actions: []string{siteConfigActionRecheck},
				}
				// The Domains row and its badge read the ledger: the failed
				// read is recorded there (set12 cell U).
				p.recordSiteFileUnread(ctx, site, kind, siteConfigReasonNotRead, class)
			}
			files = append(files, other)
		}
	}
	states := make([]string, len(files))
	for index, file := range files {
		states[index] = file.State
	}
	shown, index := services.SiteFileWorstState(states...)
	view.Files = files
	view.OverallState = shown
	if index >= 0 {
		view.OverallKind = files[index].Kind
	}
	return view
}

func siteConfigViewKind(view siteConfigView) string {
	if view.Kind == "" {
		return transport.SiteFileKindNginxVhost
	}
	return view.Kind
}

func writeSiteConfigView(w http.ResponseWriter, view siteConfigView) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

// handleOtherSiteFileAction is keep, take, recreate and apply on the site's
// pool or unit (the POST routes with "kind"). The rules are the vhost's: every
// choice is bound to the file the page showed; "take" keeps a dated copy of
// the owner's file and is answered unwritable before anything is written
// while a read-only measurement finds the file protected.
func (p *Panel) handleOtherSiteFileAction(w http.ResponseWriter, r *http.Request, domainID int, action string, body siteConfigActionRequest) {
	kind := body.Kind
	if action == "keep" && body.FileSHA256 == "" {
		writeClientError(w, http.StatusBadRequest, "file_sha256 is required")
		return
	}
	if (action == "take" || action == siteConfigActionApply) && (body.FileSHA256 == "" || body.RenderSHA256 == "") {
		writeClientError(w, http.StatusBadRequest, "file_sha256 and render_sha256 are required")
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), siteConfigActionTimeout)
	defer cancel()
	site, err := p.loadSiteFileSite(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "this domain has no hosted site")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !containsString(site.otherKinds(), kind) {
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
		return
	}
	view, inspected, err := p.inspectOtherSiteFile(ctx, site, kind, false)
	if err == nil && inspected == nil {
		err = errors.New("the Agent did not report the file's state")
	}
	if err != nil {
		writeSiteConfigNotRead(w, site.Domain, err)
		return
	}
	switch action {
	case "keep":
		if !containsString(view.Actions, "keep") {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
			return
		}
		if inspected.FileSHA256 != body.FileSHA256 {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
			return
		}
		observed := *inspected
		observed.Outcome = transport.SiteFileOutcomeKept
		if err := services.RecordSiteFileResult(ctx, p.db.GetDB(), site.SiteID, domainID, observed, siteFileReleaseLabel(), 0); err != nil {
			writeServerError(w, err)
			return
		}
		if err := services.RecordSiteFileDecision(ctx, p.db.GetDB(), site.SiteID, kind, inspected.Path,
			services.SiteFileDecisionKeepMine, inspected.FileSHA256, currentCaller(r).ID, 0); err != nil {
			writeServerError(w, err)
			return
		}
		log.Printf("site configuration %s %s: the owner chose to keep the file (sha256 %s)", site.Domain, kind, inspected.FileSHA256)
		p.audit(r, "domain.site_config.keep", "domain", domainID)
		writeSiteConfigView(w, p.siteConfigViewFor(ctx, site.SiteID, view))
		return
	case siteConfigActionApply:
		if inspected.FileSHA256 != body.FileSHA256 || inspected.RenderSHA256 != body.RenderSHA256 {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
			return
		}
		if !containsString(view.Actions, siteConfigActionApply) {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
			return
		}
	case "take":
		if inspected.FileSHA256 == body.FileSHA256 && siteConfigProtected(inspected.Protection) {
			refused := *inspected
			refused.Outcome = transport.SiteFileOutcomeRefused
			refused.Reason = transport.SiteFileReasonWriteRefused
			refused.Detail = "not attempted: the file is protected (" + inspected.Protection + ")"
			p.recordOtherSiteFile(ctx, site, kind, transport.SiteFileTriggerTake, &refused, "")
			writeServerError(w, &siteFileHeldError{Domain: site.Domain, File: refused})
			return
		}
	case "recreate":
		if inspected.State != transport.SiteFileAbsent {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
			return
		}
	}

	var file *transport.SiteFileResult
	switch kind {
	case transport.SiteFileKindPHPPool:
		operation := map[string]string{
			"take": transport.SitePoolOperationTake, "recreate": transport.SitePoolOperationRecreate,
			siteConfigActionApply: transport.SitePoolOperationApply,
		}[action]
		request := p.sitePoolRequest(ctx, site, operation)
		request.ExpectedFileSHA256, request.ExpectedRenderSHA256 = body.FileSHA256, body.RenderSHA256
		file, err = p.applySitePool(ctx, site, request)
	case transport.SiteFileKindAppUnit:
		request, _, requestErr := p.appUnitRequest(ctx, domainID)
		if requestErr != nil {
			writeSiteConfigNotRead(w, site.Domain, requestErr)
			return
		}
		request.Trigger = map[string]string{
			"take": transport.SiteFileTriggerTake, "recreate": transport.SiteFileTriggerRecreate,
			siteConfigActionApply: transport.SiteFileTriggerChange,
		}[action]
		request.ExpectedFileSHA256, request.ExpectedRenderSHA256 = body.FileSHA256, body.RenderSHA256
		file, err = p.applyAppUnit(ctx, site, request)
	}
	if file == nil {
		if err == nil {
			err = errors.New("the Agent did not report the file's state")
		}
		writeSiteConfigNotRead(w, site.Domain, err)
		return
	}
	switch {
	case file.Outcome == transport.SiteFileOutcomeRefused && file.Reason == transport.SiteFileReasonChanged,
		file.Outcome == transport.SiteFileOutcomeKept && action == siteConfigActionApply:
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
		return
	case file.Outcome == transport.SiteFileOutcomeKept && action == "recreate":
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
		return
	case !services.SiteFileApplied(file.Outcome) || err != nil:
		writeServerErrorFor(w, r, err)
		return
	}
	decision := map[string]string{"take": services.SiteFileDecisionTake, "recreate": services.SiteFileDecisionRecreate}[action]
	if decision != "" {
		if err := services.RecordSiteFileDecision(ctx, p.db.GetDB(), site.SiteID, kind,
			file.Path, decision, file.FileSHA256, currentCaller(r).ID, 0); err != nil {
			log.Printf("site configuration %s %s: record the decision: %v", site.Domain, kind, err)
		}
	}
	p.audit(r, "domain.site_config."+action, "domain", domainID)
	result := siteConfigView{
		DomainID: domainID, Domain: site.Domain, Kind: kind,
		Path: file.Path, State: transport.SiteFileManagedUnchanged, IncludeDir: file.IncludeDir,
		FileSHA256: file.FileSHA256, RenderSHA256: file.RenderSHA256,
		Actions: []string{}, Outcome: file.Outcome, BackupPath: file.BackupPath,
	}
	writeSiteConfigView(w, p.siteConfigViewFor(ctx, site.SiteID, result))
}

// otherSiteConfigSummary is one file's row for the Domains list.
type siteConfigSummaryRow struct {
	kind, state, adopted, decision, decisionSHA, fileSHA, reason string
}
