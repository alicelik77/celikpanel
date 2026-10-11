package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A site configuration file the owner changed is kept and named, never
// overwritten by a render (D-031, 10 Oct 2026; D-022; D-024; D-025 invariants
// 1, 2, 4 and 6).
//
// Every render of a site's vhost (start, setting, certificate, hosting change,
// PHP switch, creation, import) goes through the Agent's classifier. A file
// that is not CelikPanel's unchanged text is kept; the render's change is not
// applied to nginx and the operation says so with a typed refusal
// (SITE_CONFIG_OWNER_EDITED, SITE_CONFIG_MISSING, SITE_CONFIG_UNWRITABLE). The
// domain screen reads the file's state and the difference
// (GET /api/v1/domains/{id}/site-config) and offers the owner's three actions:
// keep mine, take CelikPanel's (a dated copy of the owner's file is kept
// beside it), or merge by hand in the file (the Panel's text is held in
// <file>.celikpanel-pending). A removed file is offered "recreate".
//
// Sahibin değiştirdiği site yapılandırma dosyası korunur ve adlandırılır;
// hiçbir üretim onun üzerine yazmaz. İşlem bunu tipli bir retle söyler; alan
// adı ekranı durumu ve farkı okur ve sahibin üç seçeneğini sunar.

const (
	errCodeSiteConfigOwnerEdited   = "SITE_CONFIG_OWNER_EDITED"
	errCodeSiteConfigMissing       = "SITE_CONFIG_MISSING"
	errCodeSiteConfigUnwritable    = "SITE_CONFIG_UNWRITABLE"
	errCodeSiteConfigExists        = "SITE_CONFIG_EXISTS"
	errCodeSiteConfigChanged       = "SITE_CONFIG_CHANGED"
	errCodeSiteConfigNotApplicable = "SITE_CONFIG_NOT_APPLICABLE"
	errCodeSiteConfigNotRead       = "SITE_CONFIG_NOT_READ"
	errCodeSiteConfigNginxRefused  = "SITE_CONFIG_NGINX_REFUSED"

	siteConfigReasonAgentDoesNotReport = "agent_does_not_report"
	siteConfigReasonRenderInput        = "render_input_unavailable"

	siteConfigActionTimeout = 5 * time.Minute
)

// The English originals of the refusals; the screens' sentences, in English
// and Turkish, are in web/src/i18n (err.<CODE>) and in
// docs/OPERATION-GUIDANCE.md.
const (
	siteConfigOwnerEditedMessage = "The site's nginx configuration file is not CelikPanel's unchanged text, so CelikPanel kept it and did not apply this change to it. " +
		"Nothing else was changed. Open the domain's Configuration file section: keep your file, take CelikPanel's text (your file is kept as a dated copy), or merge the two by hand."
	siteConfigMissingMessage = "The site's nginx configuration file is missing, so this change was not applied and the file was not recreated. " +
		"If the file was removed on purpose, nothing needs doing; otherwise open the domain's Configuration file section and choose Recreate."
	siteConfigUnwritableMessage = "CelikPanel could not read or replace the site's nginx configuration file (for example, it is a link or it is locked against changes), so it kept the file as it is and did not apply this change. " +
		"The server administrator checks the file on the server; the reason is shown in the domain's Configuration file section."
	siteConfigExistsMessage = "A configuration file for this name already exists on the server and was not written by CelikPanel, so CelikPanel kept it and did not create the site. " +
		"Nothing was left behind. Move or rename that file on the server if it is no longer used, then create the site again."
	siteConfigChangedMessage = "The configuration file or CelikPanel's text changed after the page showed them, so nothing was done. " +
		"Read the file's state again, then choose again."
	siteConfigNotApplicableMessage = "This choice does not apply to the file as it is now, so nothing was done. Read the file's state again; choose again if you still want it."
	siteConfigNotReadMessage       = "CelikPanel could not read the state of this site's configuration file just now. This does not mean anything is wrong with the file, and nothing was changed. Try again."
	siteConfigNginxRefusedMessage  = "nginx did not accept CelikPanel's text (it refused the configuration or could not reload), so your file was put back and nginx keeps running with it. " +
		"nginx checks every site together, so the cause may be another file; fix what nginx reported, then choose again."
	// The same refusal when putting the previous file back was not confirmed
	// (the Agent said "rollback incomplete", or gave no restore fact).
	siteConfigNginxRefusedUnconfirmedMessage = "nginx did not accept CelikPanel's text (it refused the configuration or could not reload). " +
		"CelikPanel tried to put the previous file back; whether it is in place is not confirmed — the server administrator checks the file on the server. " +
		"nginx checks every site together, so the cause may be another file; fix what nginx reported, then choose again."
)

// siteFileHeldError is a render the Agent did not apply because of the file's
// state. It is a refusal with a reason, never a bare failure.
type siteFileHeldError struct {
	Domain string
	File   transport.SiteFileResult
	// AgentError is the Agent's own error line with the result, if any (for a
	// failed operation whose restore failed it says "rollback incomplete").
	AgentError string
}

func (e *siteFileHeldError) Error() string {
	text := fmt.Sprintf("site configuration of %s not applied: %s (%s %s)",
		e.Domain, e.File.Outcome, e.File.State, e.File.Reason)
	if e.File.Outcome == transport.SiteFileOutcomeFailed {
		text += "; restore " + siteFileRestore(e.File, e.AgentError)
	}
	if e.AgentError != "" {
		text += ": " + boundedAgentDiagnostic(e.AgentError)
	}
	return text
}

// siteFileRestore is what the Panel says of a failed operation's restore:
// "confirmed" only when the Agent reports it confirmed and its error does not
// say the rollback was incomplete; an Agent that gives no restore fact (an
// older one) is "unconfirmed" (2026-10-11).
func siteFileRestore(file transport.SiteFileResult, agentError string) string {
	if file.Restore == transport.SiteFileRestoreConfirmed && !strings.Contains(agentError, "rollback incomplete") {
		return transport.SiteFileRestoreConfirmed
	}
	return transport.SiteFileRestoreUnconfirmed
}

// siteFileAgentDetails is the Agent's own line for an administrator.
func siteFileAgentDetails(agentError string) []string {
	if line := boundedAgentDiagnostic(agentError); line != "" {
		return []string{line}
	}
	return nil
}

func classifySiteFileError(err error) (agentRPCPlatformErrorClassification, bool) {
	var exists *services.SiteConfigExistsError
	if errors.As(err, &exists) {
		return agentRPCPlatformErrorClassification{
			Status: http.StatusConflict, Code: errCodeSiteConfigExists, Message: siteConfigExistsMessage,
		}, true
	}
	var held *siteFileHeldError
	if !errors.As(err, &held) {
		// An older pool RPC answers only with an error's text; its tokens
		// name the kept file (services.SiteFileHeldError).
		var serverErr rpc.ServerError
		if !errors.As(err, &serverErr) {
			return agentRPCPlatformErrorClassification{}, false
		}
		file, ok := services.ParseSiteFileHeldError(string(serverErr))
		if !ok {
			return agentRPCPlatformErrorClassification{}, false
		}
		held = &siteFileHeldError{File: file}
	}
	if kind := held.File.Kind; kind == transport.SiteFileKindPHPPool || kind == transport.SiteFileKindAppUnit {
		return classifyOtherSiteFileHeld(held.File, held.AgentError), true
	}
	switch held.File.Outcome {
	case transport.SiteFileOutcomeKept:
		return agentRPCPlatformErrorClassification{
			Status: http.StatusConflict, Code: errCodeSiteConfigOwnerEdited,
			Message: siteConfigOwnerEditedMessage, Reason: held.File.State,
		}, true
	case transport.SiteFileOutcomeMissing:
		return agentRPCPlatformErrorClassification{
			Status: http.StatusConflict, Code: errCodeSiteConfigMissing, Message: siteConfigMissingMessage,
		}, true
	default:
		reason := held.File.Reason
		if reason == "" {
			reason = held.File.State
		}
		return agentRPCPlatformErrorClassification{
			Status: http.StatusConflict, Code: errCodeSiteConfigUnwritable,
			Message: siteConfigUnwritableMessage, Reason: reason,
		}, true
	}
}

func siteFileReleaseLabel() string {
	commit := strings.TrimSpace(buildCommit)
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return strings.TrimSpace(buildVersion + " " + commit)
}

func siteFileApplied(outcome string) bool {
	switch outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeUnchanged,
		transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		return true
	}
	return false
}

// describeSiteFile is the journal's words for one site's result.
func describeSiteFile(file transport.SiteFileResult) string {
	text := file.Outcome
	switch file.Outcome {
	case transport.SiteFileOutcomeKept:
		text = "kept (" + strings.ReplaceAll(file.State, "_", "-") + "); CelikPanel's text held in " + file.PendingPath
		if file.PendingPath == "" {
			text = "kept (" + strings.ReplaceAll(file.State, "_", "-") + ")"
		}
	case transport.SiteFileOutcomeRefused:
		text = "kept, not replaced (" + file.State + " " + file.Reason + ")"
	case transport.SiteFileOutcomeMissing:
		text = "missing, not recreated"
	}
	if file.AdoptedFrom != "" {
		text += "; adopted the text of " + file.AdoptedFrom
	}
	if file.BackupPath != "" {
		text += "; owner's file kept as " + file.BackupPath
	}
	if file.Detail != "" {
		text += "; " + boundedAgentDiagnostic(file.Detail)
	}
	return text
}

// siteIDForDomain reads a hosted site's identity.
func (p *Panel) siteIDForDomain(ctx context.Context, domainID int) (int, string, error) {
	var siteID int
	var name string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE s.domain_id = ? AND COALESCE(s.project_type, 'php') <> 'dnsonly'`, domainID).Scan(&siteID, &name)
	return siteID, name, err
}

// recordedVhostSHA256 is the body digest the ledger last recorded; "" when
// the Panel never wrote the file under the header (or the ledger was lost).
func (p *Panel) recordedVhostSHA256(ctx context.Context, siteID int) string {
	record, found, err := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, transport.SiteFileKindNginxVhost)
	if err != nil || !found {
		return ""
	}
	return record.BodySHA256
}

// recordSiteFile stores and logs one site's result. A nil result is an Agent
// that did not report one: the state is unknown, never "unchanged".
func (p *Panel) recordSiteFile(ctx context.Context, siteID, domainID int, domain, trigger string, file *transport.SiteFileResult, unknownReason string) {
	if file == nil {
		path := services.SiteVhostPath(domain)
		log.Printf("site configuration %s (%s): state unknown (%s)", domain, trigger, unknownReason)
		if err := services.RecordSiteFileUnknown(ctx, p.db.GetDB(), siteID, domainID, path, unknownReason, 0); err != nil {
			log.Printf("site configuration %s: record the unknown state: %v", domain, err)
		}
		return
	}
	log.Printf("site configuration %s (%s): %s", domain, trigger, describeSiteFile(*file))
	if file.Path == "" {
		return
	}
	p.recordObservedSiteFile(ctx, siteID, domainID, domain, *file)
}

// recordObservedSiteFile stores a result and lets it end a certificate's
// waiting state (D-031 step 1b).
func (p *Panel) recordObservedSiteFile(ctx context.Context, siteID, domainID int, domain string, file transport.SiteFileResult) error {
	previous, _, _ := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, transport.SiteFileKindNginxVhost)
	if err := services.RecordSiteFileResult(ctx, p.db.GetDB(), siteID, domainID, file, siteFileReleaseLabel(), 0); err != nil {
		log.Printf("site configuration %s: record the result in the ledger: %v", domain, err)
		return err
	}
	p.observeSiteFileCertificate(ctx, siteID, domainID, previous.StateReason, file)
	return nil
}

// applySiteFileRender is the one path of every single-site render.
func (p *Panel) applySiteFileRender(ctx context.Context, req applyVhostRPCRequest, trigger string) (*transport.SiteFileResult, error) {
	req.FileTrigger = trigger
	req.RecordedSHA256 = p.recordedVhostSHA256(ctx, req.SiteID)
	var resp transport.ApplyVhostResponse
	if err := p.callAgentContext(ctx, "Agent.ApplyVhost", &req, &resp); err != nil {
		return nil, err
	}
	p.recordSiteFile(ctx, req.SiteID, req.DomainID, req.Domain, trigger, resp.File, siteConfigReasonAgentDoesNotReport)
	if resp.File != nil {
		switch resp.File.Outcome {
		case transport.SiteFileOutcomeKept, transport.SiteFileOutcomeMissing, transport.SiteFileOutcomeRefused:
			return resp.File, &siteFileHeldError{Domain: req.Domain, File: *resp.File}
		}
	}
	if resp.Error != "" {
		return resp.File, errors.New(resp.Error)
	}
	return resp.File, nil
}

// siteConfigView is the domain screen's answer.
type siteConfigView struct {
	DomainID      int                   `json:"domain_id"`
	Domain        string                `json:"domain"`
	Kind          string                `json:"kind"`
	Path          string                `json:"path,omitempty"`
	State         string                `json:"state"`
	Reason        string                `json:"reason,omitempty"`
	Detail        string                `json:"detail,omitempty"`
	AdoptedFrom   string                `json:"adopted_from,omitempty"`
	IncludeDir    string                `json:"include_dir,omitempty"`
	Enabled       string                `json:"enabled,omitempty"`
	FileSHA256    string                `json:"file_sha256,omitempty"`
	RenderSHA256  string                `json:"render_sha256,omitempty"`
	PendingPath   string                `json:"pending_path,omitempty"`
	Diff          string                `json:"diff,omitempty"`
	DiffTruncated bool                  `json:"diff_truncated,omitempty"`
	Actions       []string              `json:"actions"`
	Decision      *siteConfigDecision   `json:"decision,omitempty"`
	Ledger        *siteConfigLedgerView `json:"ledger,omitempty"`
	Outcome       string                `json:"outcome,omitempty"`
	BackupPath    string                `json:"backup_path,omitempty"`
	// D-031 step 1b: the Panel's own include directory and the line a kept
	// file needs for certificate validation; Validation is whether a
	// certificate operation could validate without changing a kept file.
	ManagedDir     string `json:"managed_dir,omitempty"`
	ManagedInclude string `json:"managed_include,omitempty"`
	Validation     string `json:"validation,omitempty"`
	ChallengeFile  string `json:"challenge_file,omitempty"`
	// PendingReason is the ledger's certificate reason ("certificate",
	// "certificate_validation"); Certificate says what it is about.
	PendingReason string                 `json:"pending_reason,omitempty"`
	Certificate   *siteConfigCertificate `json:"certificate,omitempty"`
	// ResolvedReason is the certificate reason this read ended
	// ("certificate_validation": the probe found the kept file ready), so the
	// page can say that the certificate can be requested again. Additive.
	ResolvedReason string `json:"resolved_reason,omitempty"`
	// ValidationName and ValidationStatus: the first name nginx did not
	// serve the probe under, and the HTTP status it gave (probe reads only).
	ValidationName   string `json:"validation_name,omitempty"`
	ValidationStatus int    `json:"validation_status,omitempty"`
	// set10 scenario g, additive. SystemError: the system's error name a
	// replacement of the file gets now ("EPERM"), measured read-only by the
	// Agent; Protection: what refuses it (transport.SiteFileProtection*).
	SystemError string `json:"system_error,omitempty"`
	Protection  string `json:"protection,omitempty"`
	// FileState, additive (2026-10-11): what the file's bytes are when State
	// is a higher fact about the same bytes (services.SiteFileShownState):
	// "owner_edited", "foreign" or "unknown_origin" under "unwritable" — the
	// owner's file whose replacement ("take") the server refused.
	FileState string `json:"file_state,omitempty"`
	// Files, additive (D-031 second step, 2026-10-11): every file CelikPanel
	// writes for the site, each in this same shape with its own kind and
	// actions: the vhost (the top-level fields repeat it, for a client that
	// reads one file), the PHP-FPM pool of a PHP site, the application unit
	// of a Node site. OverallState is the worst of their states
	// (services.SiteFileWorstState); OverallKind names the file that decided it.
	Files        []siteConfigView `json:"files,omitempty"`
	OverallState string           `json:"overall_state,omitempty"`
	OverallKind  string           `json:"overall_kind,omitempty"`

	certificateReferenced bool
	// heldRender: the file is CelikPanel's unchanged text, but not this
	// render's (its declared digest differs, or it has no header).
	heldRender bool
}

type siteConfigDecision struct {
	Kind      string `json:"kind"`
	DecidedAt string `json:"decided_at"`
	// Current: the file is still the one the decision was made on.
	Current bool `json:"current"`
}

type siteConfigLedgerView struct {
	WrittenRelease string `json:"written_release,omitempty"`
	WrittenAt      string `json:"written_at,omitempty"`
	ObservedAt     string `json:"observed_at,omitempty"`
	BackupPath     string `json:"backup_path,omitempty"`
}

func unixToAPI(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.Unix(value, 0).UTC().Format(time.RFC3339)
}

// siteConfigActionRecheck is the page's "check again": it reads this answer
// again (GET; nothing is written). It is offered where only a change on the
// server can resolve the state.
const siteConfigActionRecheck = "recheck"

// siteConfigActionApply writes CelikPanel's held text for this one site now
// (POST .../site-config/apply). It is offered only on CelikPanel's unchanged
// text whose last replacement was refused and whose protection a later "check
// again" found removed (ledger reason protection_removed), while a render is
// held; the next start or saved change would write it as well.
const siteConfigActionApply = "apply"

func siteConfigActions(state string) []string {
	switch state {
	case transport.SiteFileOwnerEdited, transport.SiteFileForeign, transport.SiteFileUnknownOrigin:
		return []string{"keep", "take", "merge"}
	case transport.SiteFileAbsent:
		return []string{"recreate"}
	case transport.SiteFileUnreadable, transport.SiteFileUnwritable:
		return []string{siteConfigActionRecheck}
	}
	return []string{}
}

// presentLedgerWriteRefusal answers the ledger's last observation when the
// last render or "take" could not replace the file (set10 scenario g): the
// Agent's read finds the file readable, but the ledger and the domains list
// say its replacement was refused. While the file on disk is still the one
// the refusal was recorded on, both facts hold for the same bytes and the
// read answers the higher one (services.SiteFileShownState), so the page, the
// list row and the badge agree: CelikPanel's unchanged text is "unwritable";
// the owner's file whose "take" was refused is "unwritable" with FileState
// naming what the file is, and the owner's choices stay offered ("take" is
// answered SITE_CONFIG_UNWRITABLE while the protection stands). The system
// error and the protection are the Agent's read-only measurement of now; the
// difference is what CelikPanel wanted to write.
func presentLedgerWriteRefusal(view *siteConfigView, record services.SiteFileRecord) {
	if view.Outcome != "" || !services.SiteFileWriteRefused(record) {
		return
	}
	if record.FileSHA256 == "" || record.FileSHA256 != view.FileSHA256 {
		return
	}
	shown := services.SiteFileShownState(view.State, transport.SiteFileUnwritable)
	if shown == view.State {
		return
	}
	actions := siteConfigActions(shown)
	if siteFileKeptState(view.State) {
		view.FileState = view.State
		actions = append(siteConfigActions(view.State), siteConfigActionRecheck)
	}
	view.State = shown
	view.Reason = transport.SiteFileReasonWriteRefused
	view.Actions = actions
}

// presentProtectionRemoved offers "apply" on CelikPanel's unchanged text
// whose refusal a "check again" ended (reason protection_removed), while the
// file is the one that read measured and a render is held.
func presentProtectionRemoved(view *siteConfigView, record services.SiteFileRecord) {
	if view.Outcome != "" || view.State != transport.SiteFileManagedUnchanged || !view.heldRender ||
		record.StateReason != services.SiteFileReasonProtectionRemoved ||
		record.FileSHA256 == "" || record.FileSHA256 != view.FileSHA256 {
		return
	}
	view.Reason = services.SiteFileReasonProtectionRemoved
	view.Actions = []string{siteConfigActionApply}
}

func siteFileKeptState(state string) bool {
	switch state {
	case transport.SiteFileOwnerEdited, transport.SiteFileForeign, transport.SiteFileUnknownOrigin:
		return true
	}
	return false
}

// recheckWriteRefusal is the domain screen's read ("check again") on a site
// whose ledger row is a refused replacement. It records what the read found,
// so the domains list row agrees with the page:
//   - the same bytes, measured unprotected ("none"): CelikPanel's unchanged
//     text is stored managed_unchanged with the reason protection_removed (its
//     held text is written at the next start or saved change, or by "apply");
//     the owner's file is stored as what it is (owner-edited, …), its choices
//     open again;
//   - the same bytes, still protected or not measured: the refusal stands;
//     only the observation time is stored;
//   - other bytes: the file as it is read now (the refusal was about bytes
//     that are gone). Never a write of the file.
func (p *Panel) recheckWriteRefusal(ctx context.Context, siteID int, view siteConfigView) {
	kind := siteConfigViewKind(view)
	record, found, err := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, kind)
	if err != nil || !found || !services.SiteFileWriteRefused(record) || view.Path == "" || view.Path != record.Path {
		return
	}
	state, reason, fileSHA := record.State, record.StateReason, record.FileSHA256
	switch {
	case record.FileSHA256 != "" && view.FileSHA256 == record.FileSHA256:
		if view.Protection == transport.SiteFileProtectionNone {
			switch {
			case view.State == transport.SiteFileManagedUnchanged:
				state, reason = view.State, services.SiteFileReasonProtectionRemoved
			case siteFileKeptState(view.State):
				state, reason = view.State, ""
			}
		}
	default:
		switch {
		case view.State == transport.SiteFileAbsent:
			state, reason = transport.SiteFileMissingState, ""
		case view.State == transport.SiteFileManagedUnchanged || siteFileKeptState(view.State):
			state, reason = view.State, ""
		case view.State == transport.SiteFileUnreadable:
			state, reason = view.State, view.Reason
		default:
			return
		}
		fileSHA = view.FileSHA256
	}
	changed, err := services.RecordSiteFileRecheck(ctx, p.db.GetDB(), siteID, kind,
		record.Path, state, reason, fileSHA, 0)
	if err != nil {
		log.Printf("site configuration %s: record the check: %v", view.Domain, err)
		return
	}
	if changed && (state != record.State || reason != record.StateReason) {
		log.Printf("site configuration %s: checked again: %s (%s), no longer a refused replacement (protection %q)",
			view.Domain, strings.ReplaceAll(state, "_", "-"), reason, view.Protection)
	}
}

func (p *Panel) siteConfigViewFor(ctx context.Context, siteID int, view siteConfigView) siteConfigView {
	kind := siteConfigViewKind(view)
	record, found, err := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, kind)
	// A pool row of another PHP version's path is not this file's.
	if err != nil || !found || (kind != transport.SiteFileKindNginxVhost && view.Path != "" && record.Path != view.Path) {
		return view
	}
	presentLedgerWriteRefusal(&view, record)
	presentProtectionRemoved(&view, record)
	view.Ledger = &siteConfigLedgerView{
		WrittenRelease: record.WrittenRelease, WrittenAt: unixToAPI(record.WrittenAt),
		ObservedAt: unixToAPI(record.ObservedAt), BackupPath: record.BackupPath,
	}
	if record.Decision != "" {
		view.Decision = &siteConfigDecision{
			Kind: record.Decision, DecidedAt: unixToAPI(record.DecidedAt),
			Current: record.DecisionFileSHA256 != "" && record.DecisionFileSHA256 == view.FileSHA256,
		}
	}
	if kind == transport.SiteFileKindNginxVhost && services.SiteFileCertificateReason(record.StateReason) {
		view.PendingReason = record.StateReason
		view.Certificate = p.siteConfigCertificateFor(ctx, view.DomainID, record.StateReason, view.certificateReferenced)
	}
	return view
}

// inspectSiteConfig asks the Agent for the file's state and the difference.
// Without probe it writes nothing, on the server or in the Panel's database.
// With probe (the domain screen's read), a ledger row whose last observation
// is a refused replacement records what the read found (recheckWriteRefusal).
// With probe, and only while the ledger says the file stopped a certificate
// ("certificate_validation"), the Agent measures the validation (a probe file
// in the challenge root, removed again; CelikPanel's challenge file published
// when absent) and a measured "ready" ends that reason (D-031 step 1b).
func (p *Panel) inspectSiteConfig(ctx context.Context, domainID int, probe bool) (siteConfigView, int, *transport.SiteFileResult, error) {
	siteID, domain, err := p.siteIDForDomain(ctx, domainID)
	if err != nil {
		return siteConfigView{}, 0, nil, err
	}
	view := siteConfigView{DomainID: domainID, Domain: domain, Kind: transport.SiteFileKindNginxVhost, Actions: []string{}}
	req, err := p.buildVhostRequest(ctx, domainID, nil)
	if err != nil {
		return view, siteID, nil, fmt.Errorf("%s: %w", siteConfigReasonRenderInput, err)
	}
	req.FileTrigger = transport.SiteFileTriggerChange
	req.RecordedSHA256 = p.recordedVhostSHA256(ctx, siteID)
	if probe {
		record, found, recordErr := services.LoadSiteFileRecord(ctx, p.db.GetDB(), siteID, transport.SiteFileKindNginxVhost)
		req.ProbeValidation = recordErr == nil && found && record.StateReason == services.SiteFileReasonCertificateValidation
	}
	var resp transport.InspectSiteFileResponse
	if err := p.callAgentContext(ctx, "Agent.InspectSiteFile", &req, &resp); err != nil {
		if agentRPCMethodUnavailable(err, "Agent.InspectSiteFile") {
			view.State = transport.SiteFileStateUnknown
			view.Reason = siteConfigReasonAgentDoesNotReport
			return p.siteConfigViewFor(ctx, siteID, view), siteID, nil, nil
		}
		return view, siteID, nil, err
	}
	if resp.Error != "" {
		return view, siteID, nil, errors.New(resp.Error)
	}
	file := resp.File
	view.Path, view.State, view.Reason, view.Detail = file.Path, file.State, file.Reason, file.Detail
	view.AdoptedFrom, view.IncludeDir, view.Enabled = file.AdoptedFrom, file.IncludeDir, file.Enabled
	view.FileSHA256, view.RenderSHA256, view.PendingPath = file.FileSHA256, file.RenderSHA256, file.PendingPath
	view.Diff, view.DiffTruncated = resp.Diff, resp.DiffTruncated
	view.ManagedDir, view.ManagedInclude = file.ManagedDir, file.ManagedInclude
	view.Validation, view.ChallengeFile = file.Validation, file.ChallengeFile
	view.certificateReferenced = file.CertificateReferenced
	view.ValidationName, view.ValidationStatus = file.ValidationName, file.ValidationStatus
	view.SystemError, view.Protection = file.SystemError, file.Protection
	view.heldRender = file.State == transport.SiteFileManagedUnchanged && file.WrittenSHA256 != file.RenderSHA256
	view.Actions = siteConfigActions(file.State)
	if req.ProbeValidation && file.Validation == transport.SiteFileValidationReady &&
		p.endCertificateValidationReason(ctx, siteID, domainID, domain) {
		view.ResolvedReason = services.SiteFileReasonCertificateValidation
	}
	if probe {
		// The domain screen's read is its "check again".
		p.recheckWriteRefusal(ctx, siteID, view)
	}
	return p.siteConfigViewFor(ctx, siteID, view), siteID, &file, nil
}

type siteConfigActionRequest struct {
	FileSHA256   string `json:"file_sha256"`
	RenderSHA256 string `json:"render_sha256"`
	// Kind names the file (D-031 second step): nginx_vhost (the default when
	// absent), php_pool or app_unit.
	Kind string `json:"kind,omitempty"`
}

func validSiteConfigDigest(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

// handleDomainSiteConfig serves GET site-config and the three POST actions.
// The file's path and the difference name this server's files, so every one
// of them is for an administrator.
func (p *Panel) handleDomainSiteConfig(w http.ResponseWriter, r *http.Request, domainID int, action string) {
	caller := currentCaller(r)
	if caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeClientError(w, http.StatusForbidden, "administrator access is required")
		return
	}
	switch action {
	case "":
		p.handleSiteConfigRead(w, r, domainID)
	case "keep":
		p.handleSiteConfigKeep(w, r, domainID)
	case "take", "recreate", siteConfigActionApply:
		p.handleSiteConfigRender(w, r, domainID, action)
	default:
		http.NotFound(w, r)
	}
}

func writeSiteConfigNotRead(w http.ResponseWriter, domain string, err error) {
	log.Printf("[502] site configuration %s: could not read its state: %v", domain, err)
	writeCodedError(w, http.StatusBadGateway, errCodeSiteConfigNotRead, siteConfigNotReadMessage, "")
}

func (p *Panel) handleSiteConfigRead(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx, cancel := context.WithTimeout(r.Context(), agentRPCStandardReadTimeout)
	defer cancel()
	view, _, _, err := p.inspectSiteConfig(ctx, domainID, true)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "this domain has no hosted site")
		return
	}
	if err != nil {
		writeSiteConfigNotRead(w, view.Domain, err)
		return
	}
	view = p.withOtherSiteFiles(ctx, domainID, view)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

func decodeSiteConfigAction(w http.ResponseWriter, r *http.Request) (siteConfigActionRequest, bool) {
	var body siteConfigActionRequest
	if r.ContentLength != 0 {
		if err := decodeServiceConfigJSON(w, r, &body); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return body, false
		}
	}
	switch body.Kind {
	case "":
		body.Kind = transport.SiteFileKindNginxVhost
	case transport.SiteFileKindNginxVhost, transport.SiteFileKindPHPPool, transport.SiteFileKindAppUnit:
	default:
		writeClientError(w, http.StatusBadRequest, "kind is nginx_vhost, php_pool or app_unit")
		return body, false
	}
	if (body.FileSHA256 != "" && !validSiteConfigDigest(body.FileSHA256)) ||
		(body.RenderSHA256 != "" && !validSiteConfigDigest(body.RenderSHA256)) {
		writeClientError(w, http.StatusBadRequest, "file_sha256 and render_sha256 are 64 lowercase hexadecimal characters")
		return body, false
	}
	return body, true
}

// handleSiteConfigKeep records "keep mine". The file is not touched, not even
// its header: rewriting the header would make the owner's text look like the
// Panel's unchanged text, and the next render would replace it.
func (p *Panel) handleSiteConfigKeep(w http.ResponseWriter, r *http.Request, domainID int) {
	body, ok := decodeSiteConfigAction(w, r)
	if !ok {
		return
	}
	if body.Kind != transport.SiteFileKindNginxVhost {
		p.handleOtherSiteFileAction(w, r, domainID, "keep", body)
		return
	}
	if body.FileSHA256 == "" {
		writeClientError(w, http.StatusBadRequest, "file_sha256 is required")
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), siteConfigActionTimeout)
	defer cancel()
	view, siteID, file, err := p.inspectSiteConfig(ctx, domainID, false)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "this domain has no hosted site")
		return
	}
	if err != nil {
		writeSiteConfigNotRead(w, view.Domain, err)
		return
	}
	if file == nil || !containsString(view.Actions, "keep") {
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
		return
	}
	if file.FileSHA256 != body.FileSHA256 {
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
		return
	}
	observed := *file
	observed.Outcome = transport.SiteFileOutcomeKept
	if err := p.recordObservedSiteFile(ctx, siteID, domainID, view.Domain, observed); err != nil {
		writeServerError(w, err)
		return
	}
	if err := services.RecordSiteFileDecision(ctx, p.db.GetDB(), siteID, transport.SiteFileKindNginxVhost,
		file.Path, services.SiteFileDecisionKeepMine, file.FileSHA256, currentCaller(r).ID, 0); err != nil {
		writeServerError(w, err)
		return
	}
	log.Printf("site configuration %s: the owner chose to keep the file (sha256 %s)", view.Domain, file.FileSHA256)
	p.audit(r, "domain.site_config.keep", "domain", domainID)
	view = p.siteConfigViewFor(ctx, siteID, view)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

// siteConfigProtected says whether a read-only measurement found something
// that refuses replacing the file now ("none" and "not measured" do not).
func siteConfigProtected(protection string) bool {
	return protection != "" && protection != transport.SiteFileProtectionNone
}

// handleSiteConfigRender is "take CelikPanel's", "recreate" and "apply".
//
// "apply" writes CelikPanel's held text for this one site on CelikPanel's own
// unchanged text, after a "check again" found the protection that refused it
// removed. It is the render of a settings change (the Agent's "change"
// trigger, managed_unchanged branch): no dated copy is made, because the file
// is CelikPanel's own earlier text (its header's digest matches its body), so
// nothing of the owner's is in it; if nginx refuses the text, the previous
// bytes are put back exactly, as for every render. It records no owner
// decision.
//
// "take" on a file a read-only measurement finds protected is answered
// SITE_CONFIG_UNWRITABLE before anything is written (no dated copy is left
// behind for a replacement the server would refuse); the refusal is recorded.
func (p *Panel) handleSiteConfigRender(w http.ResponseWriter, r *http.Request, domainID int, action string) {
	body, ok := decodeSiteConfigAction(w, r)
	if !ok {
		return
	}
	if body.Kind != transport.SiteFileKindNginxVhost {
		p.handleOtherSiteFileAction(w, r, domainID, action, body)
		return
	}
	if action != "recreate" && (body.FileSHA256 == "" || body.RenderSHA256 == "") {
		writeClientError(w, http.StatusBadRequest, "file_sha256 and render_sha256 are required")
		return
	}
	trigger := transport.SiteFileTriggerRecreate
	switch action {
	case "take":
		trigger = transport.SiteFileTriggerTake
	case siteConfigActionApply:
		trigger = transport.SiteFileTriggerChange
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), siteConfigActionTimeout)
	defer cancel()
	siteID, domain, err := p.siteIDForDomain(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "this domain has no hosted site")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if action != "recreate" {
		view, _, inspected, err := p.inspectSiteConfig(ctx, domainID, false)
		if err != nil {
			writeSiteConfigNotRead(w, domain, err)
			return
		}
		if action == siteConfigActionApply {
			if inspected != nil && (inspected.FileSHA256 != body.FileSHA256 || inspected.RenderSHA256 != body.RenderSHA256) {
				writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
				return
			}
			if inspected == nil || !containsString(view.Actions, siteConfigActionApply) {
				writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
				return
			}
		}
		if action == "take" && inspected != nil && inspected.FileSHA256 == body.FileSHA256 &&
			siteConfigProtected(inspected.Protection) {
			refused := *inspected
			refused.Outcome = transport.SiteFileOutcomeRefused
			refused.Reason = transport.SiteFileReasonWriteRefused
			refused.Detail = "not attempted: the file is protected (" + inspected.Protection + ")"
			p.recordSiteFile(ctx, siteID, domainID, domain, trigger, &refused, "")
			// The same answer as a replacement the Agent's write refused.
			writeServerError(w, &siteFileHeldError{Domain: domain, File: refused})
			return
		}
	}
	req, err := p.buildVhostRequest(ctx, domainID, nil)
	if err != nil {
		writeSiteConfigNotRead(w, domain, err)
		return
	}
	req.ExpectedFileSHA256 = body.FileSHA256
	req.ExpectedRenderSHA256 = body.RenderSHA256
	file, err := p.applySiteFileRender(ctx, req, trigger)
	if file == nil {
		if err == nil {
			err = errors.New("the Agent did not report the file's state")
		}
		writeSiteConfigNotRead(w, domain, err)
		return
	}
	switch {
	case file.Outcome == transport.SiteFileOutcomeRefused && file.Reason == transport.SiteFileReasonChanged:
		writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
		return
	case file.Outcome == transport.SiteFileOutcomeFailed &&
		(file.Reason == transport.SiteFileReasonNginxRefused || file.Reason == transport.SiteFileReasonReloadFailed):
		agentError := ""
		if err != nil {
			agentError = err.Error()
		}
		restore := siteFileRestore(*file, agentError)
		body := apiErrorBody{Error: siteConfigNginxRefusedMessage, Code: errCodeSiteConfigNginxRefused, Reason: file.Reason, Restore: restore}
		if restore != transport.SiteFileRestoreConfirmed {
			body.Error = siteConfigNginxRefusedUnconfirmedMessage
		}
		if file.Detail != "" {
			body.Details = []string{boundedAgentDiagnostic(file.Detail)}
		}
		if restore != transport.SiteFileRestoreConfirmed && agentError != "" {
			// The Agent's own line (this route is an administrator's).
			body.Details = append(body.Details, siteFileAgentDetails(agentError)...)
		}
		log.Printf("[502] site configuration %s: %s: %v", domain, trigger, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(body)
		return
	case !siteFileApplied(file.Outcome):
		if file.Outcome == transport.SiteFileOutcomeKept && action == siteConfigActionApply {
			// The file changed after the read: kept, never written.
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigChanged, siteConfigChangedMessage, "")
			return
		}
		if file.Outcome == transport.SiteFileOutcomeKept && action == "recreate" {
			writeCodedError(w, http.StatusConflict, errCodeSiteConfigNotApplicable, siteConfigNotApplicableMessage, "")
			return
		}
		writeServerError(w, err)
		return
	}
	decision := ""
	switch action {
	case "take":
		decision = services.SiteFileDecisionTake
	case "recreate":
		decision = services.SiteFileDecisionRecreate
	}
	if decision != "" {
		if err := services.RecordSiteFileDecision(ctx, p.db.GetDB(), siteID, transport.SiteFileKindNginxVhost,
			file.Path, decision, file.FileSHA256, currentCaller(r).ID, 0); err != nil {
			log.Printf("site configuration %s: record the decision: %v", domain, err)
		}
	}
	p.audit(r, "domain.site_config."+action, "domain", domainID)
	view := siteConfigView{
		DomainID: domainID, Domain: domain, Kind: transport.SiteFileKindNginxVhost,
		Path: file.Path, State: transport.SiteFileManagedUnchanged, IncludeDir: file.IncludeDir,
		Enabled: file.Enabled, FileSHA256: file.FileSHA256, RenderSHA256: file.RenderSHA256,
		Actions: []string{}, Outcome: file.Outcome, BackupPath: file.BackupPath,
		ManagedDir: file.ManagedDir, ManagedInclude: file.ManagedInclude, ChallengeFile: file.ChallengeFile,
	}
	view = p.siteConfigViewFor(ctx, siteID, view)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// domainSiteConfigSummary is the domains list's field for an administrator.
type domainSiteConfigSummary struct {
	State       string `json:"state"`
	AdoptedFrom string `json:"adopted_from,omitempty"`
	// KeptByChoice: the owner chose "keep mine" for the file as it is now.
	KeptByChoice bool `json:"kept_by_choice,omitempty"`
	// PendingReason is a certificate reason (D-031 step 1b), when there is one.
	PendingReason string `json:"pending_reason,omitempty"`
	// Kind names the file whose state is shown when it is not the vhost
	// (D-031 second step: php_pool, app_unit). Additive.
	Kind string `json:"kind,omitempty"`
}

// domainSiteConfigSummaries answers, per domain, the worst state of the
// site's files (services.SiteFileWorstState, applied to each file's presented
// state and then across the files, the vhost first on a tie): the vhost
// always, the pool of a PHP site, the unit of a Node site, each from its
// newest ledger row.
func (p *Panel) domainSiteConfigSummaries(ctx context.Context) (map[int]domainSiteConfigSummary, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT m.domain_id, m.kind, m.state, m.adopted_from, m.decision, m.decision_file_sha256,
		       m.file_sha256, m.state_reason, COALESCE(s.project_type, 'php')
		FROM managed_site_files m LEFT JOIN sites s ON s.id = m.site_id
		WHERE m.kind IN (?, ?, ?) ORDER BY m.id`,
		transport.SiteFileKindNginxVhost, transport.SiteFileKindPHPPool, transport.SiteFileKindAppUnit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	newest := make(map[int]map[string]siteConfigSummaryRow)
	projectTypes := make(map[int]string)
	var order []int
	for rows.Next() {
		var domainID int
		var row siteConfigSummaryRow
		var projectType string
		if err := rows.Scan(&domainID, &row.kind, &row.state, &row.adopted, &row.decision, &row.decisionSHA,
			&row.fileSHA, &row.reason, &projectType); err != nil {
			return nil, err
		}
		if newest[domainID] == nil {
			newest[domainID] = make(map[string]siteConfigSummaryRow)
			order = append(order, domainID)
		}
		newest[domainID][row.kind] = row
		projectTypes[domainID] = projectType
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	summaries := make(map[int]domainSiteConfigSummary, len(order))
	for _, domainID := range order {
		kinds := []string{transport.SiteFileKindNginxVhost}
		switch projectTypes[domainID] {
		case "php":
			kinds = append(kinds, transport.SiteFileKindPHPPool)
		case "node":
			kinds = append(kinds, transport.SiteFileKindAppUnit)
		}
		var present []siteConfigSummaryRow
		var states []string
		for _, kind := range kinds {
			row, ok := newest[domainID][kind]
			if !ok {
				continue
			}
			present = append(present, row)
			// A refused replacement is "unwritable", as the domain screen
			// answers it (set10 scenario g).
			states = append(states, services.SiteFilePresentedState(row.state, row.reason))
		}
		shown, index := services.SiteFileWorstState(states...)
		if index < 0 {
			continue
		}
		row := present[index]
		summary := domainSiteConfigSummary{
			State: shown, AdoptedFrom: row.adopted,
			KeptByChoice: row.decision == services.SiteFileDecisionKeepMine && row.decisionSHA != "" && row.decisionSHA == row.fileSHA,
		}
		if row.kind == transport.SiteFileKindNginxVhost {
			if services.SiteFileCertificateReason(row.reason) {
				summary.PendingReason = row.reason
			}
		} else {
			summary.Kind = row.kind
		}
		summaries[domainID] = summary
	}
	return summaries, nil
}

func siteConfigFor(summaries map[int]domainSiteConfigSummary, domainID int) *domainSiteConfigSummary {
	summary, ok := summaries[domainID]
	if !ok {
		return nil
	}
	return &summary
}
