package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

const (
	serviceOperationKindMailProfileInstall   = "mail_profile_install"
	mailProfileProofVersion                  = 1
	mailProfileInstallPath                   = "/api/v1/service/profile/install"
	errCodeMailProfileConfirmationRequired   = "mail_profile_confirmation_required"
	errCodeMailProfileServerHostnameInvalid  = "mail_profile_server_hostname_invalid"
	errCodeMailProfileServerHostnameRequired = "mail_profile_server_hostname_required"

	mailProfileStatusUnknown   = "unknown"
	mailProfileStatusAvailable = "available"
	mailProfileStatusPartial   = "partial"
	mailProfileStatusComplete  = "complete"
	mailProfileStatusBlocked   = "blocked"

	mailProfileAttemptNone       = "none"
	mailProfileAttemptInProgress = "in_progress"
	mailProfileAttemptSucceeded  = "succeeded"
	mailProfileAttemptFailed     = "failed"

	mailProfileFallbackWarning        = "No active trusted Secure Mail certificate was found. Mail submission is using the local self-signed fallback; activate a trusted certificate with Secure Mail enabled."
	mailProfileReconciliationWarning  = "Components are running. Run Repair to verify and reconcile the complete mail profile, including mail TLS and authenticated submission."
	mailProfileServerHostnameMessage  = "The mail hostname must be a fully qualified domain name, such as mail.example.com."
	mailProfileServerHostnameRequired = "This server does not have a fully qualified name yet and CelikPanel has none saved for it. Enter the name the mail server should answer as, such as mail.example.com; the installation gives this server that name."
	mailProfileServerHostnameUnknown  = "This server's own hostname could not be read, so the mail hostname cannot be decided."
)

var errMailProfileServerHostnameInvalid = errors.New("mail profile server hostname is not a canonical FQDN")

// errMailProfileServerHostnameRequired is the refusal that now replaces the
// dead end: the panel holds no fully qualified name for this server and the
// operator was not asked for one. It names a field to fill rather than a fact
// to fix somewhere else.
// errMailProfileServerHostnameRequired, çıkmaz sokağın yerini alan reddir:
// panelin bu sunucu için tam nitelikli bir adı yok ve operatöre sorulmamış.
// Başka bir yerde düzeltilecek bir olguyu değil, doldurulacak bir alanı
// adlandırır.
var errMailProfileServerHostnameRequired = errors.New("mail profile server hostname was not supplied and could not be derived")

// readMailProfileHostname is a test seam around the host fact used by the
// production preflight. It always points at os.Hostname outside tests.
var readMailProfileHostname = os.Hostname

type mailProfileDefinition struct {
	ID          string
	Name        string
	Description string
	Services    []string
}

var mailProfileDefinitions = [...]mailProfileDefinition{
	{
		ID:          core.MailProfileCore,
		Name:        "Core Mail",
		Description: "Postfix SMTP and Dovecot IMAP/POP3 with authenticated submission.",
		Services:    mustMailProfileServiceIDs(core.MailProfileCore),
	},
	{
		ID:          core.MailProfileWebmail,
		Name:        "Webmail",
		Description: "Core mail plus Nginx, PHP-FPM and Roundcube webmail.",
		Services:    mustMailProfileServiceIDs(core.MailProfileWebmail),
	},
	{
		ID:          core.MailProfileProtected,
		Name:        "Spam-Protected Mail",
		Description: "Core mail with Rspamd spam filtering.",
		Services:    mustMailProfileServiceIDs(core.MailProfileProtected),
	},
}

// mailProfileMailComponents are the components that make a mail profile
// installed. Nginx and PHP-FPM serve websites too; with only those present a
// mail profile is "not installed", not "partially installed" (owner
// observation on Boston, alpha.92, 2026-10-16).
// mailProfileMailComponents bir posta profilini kurulu yapan bileşenlerdir.
// Nginx ve PHP-FPM web sitelerine de hizmet eder; yalnız onlar varken posta
// profili "kısmen kurulu" değil "kurulu değil"dir.
var mailProfileMailComponents = map[string]bool{
	"postfix":   true,
	"dovecot":   true,
	"rspamd":    true,
	"roundcube": true,
}

func mustMailProfileServiceIDs(profileID string) []string {
	services, ok := core.MailProfileServiceIDs(profileID)
	if !ok {
		panic("unknown compiled mail profile: " + profileID)
	}
	return services
}

type mailProfileInstallRequest struct {
	ProfileID string `json:"profile_id"`
	RequestID string `json:"request_id"`
	Confirmed bool   `json:"confirmed"`
	// MailHostname is the name the operator gave on the install screen when
	// the panel held no identity to derive one from. It is validated here and
	// saved as the panel's own mail hostname before the operation starts, so a
	// restart mid-install resumes with exactly the name that was confirmed.
	// MailHostname, panelin türetecek bir kimliği olmadığında operatörün
	// kurulum ekranında verdiği addır. Burada doğrulanır ve işlem başlamadan
	// önce panelin kendi posta ana bilgisayar adı olarak kaydedilir; böylece
	// kurulumun ortasında bir yeniden başlatma, tam olarak onaylanan adla
	// devam eder.
	MailHostname string `json:"mail_hostname"`
}

type mailProfileTLSResult struct {
	Configured   bool `json:"configured"`
	SNICount     int  `json:"sni_count"`
	FallbackOnly bool `json:"fallback_only"`
}

type mailProfileProofReceipt struct {
	Success              bool                       `json:"success"`
	ProofVersion         int                        `json:"proof_version"`
	ProfileID            string                     `json:"profile_id"`
	Services             []string                   `json:"services"`
	CompletedServices    []string                   `json:"completed_services"`
	MailTLS              mailProfileTLSProofReceipt `json:"mail_tls"`
	SubmissionConfigured bool                       `json:"submission_configured"`
}

type mailProfileTLSProofReceipt struct {
	Configured   bool  `json:"configured"`
	SNICount     *int  `json:"sni_count"`
	FallbackOnly *bool `json:"fallback_only"`
}

// MailProfileResponse is derived entirely on the server. Clients choose one
// profile id; they can never supply or reorder the privileged service plan.
type MailProfileResponse struct {
	Verified            bool     `json:"verified"`
	LatestAttemptStatus string   `json:"latest_attempt_status"`
	LatestAttemptError  string   `json:"latest_attempt_error,omitempty"`
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Services            []string `json:"services"`
	Status              string   `json:"status"`
	Available           bool     `json:"available"`
	BlockedReason       string   `json:"blocked_reason,omitempty"`
	Warning             string   `json:"warning,omitempty"`
	// BlockedCode/BlockedVars and WarningCode type the two
	// sentences above (additive, 2026-10-14), as the setup checks' code and
	// vars do: a page says them in its own language and keeps the English
	// text only for a code it does not know. The text stays for older pages.
	// Yukarıdaki iki cümlenin türlü karşılığı; sayfa kendi dilinde söyler.
	BlockedCode mailProfileTextCode `json:"blocked_code,omitempty"`
	BlockedVars map[string]string   `json:"blocked_vars,omitempty"`
	WarningCode mailProfileTextCode `json:"warning_code,omitempty"`
}

// mailProfileTextCode names one blocked or warning sentence of a mail
// profile. Every constant of this type must be in mailProfileTextCodes
// (TestMailProfileTextCodesAreListed), and web/tests/mail-profile-text.test.mjs
// reads that list from this file and requires an English and a Turkish
// sentence for each, so a new code cannot reach a page without words.
// Bir posta profilinin engel ya da uyarı cümlesinin adı; her biri listede ve
// iki dilde sözü olmak zorundadır.
type mailProfileTextCode string

const (
	mailProfileBlockedStateUnverified     mailProfileTextCode = "state_unverified"
	mailProfileBlockedStateIncomplete     mailProfileTextCode = "state_incomplete"
	mailProfileBlockedCatalogueIncomplete mailProfileTextCode = "catalogue_incomplete"
	mailProfileBlockedHostnameUnknown     mailProfileTextCode = "hostname_unknown"
	mailProfileBlockedDNSIdentity         mailProfileTextCode = "dns_identity_not_ready"
	mailProfileBlockedNotInstallable      mailProfileTextCode = "component_not_installable"
	mailProfileBlockedConflict            mailProfileTextCode = "component_conflict"
	mailProfileBlockedRequires            mailProfileTextCode = "component_requires"
	mailProfileWarningRepairToVerify      mailProfileTextCode = "repair_to_verify"
	mailProfileWarningWebmailUnverified   mailProfileTextCode = "webmail_unverified"
	mailProfileWarningRoundcubeNotReady   mailProfileTextCode = "roundcube_not_ready"
	mailProfileWarningPartlyPresent       mailProfileTextCode = "partly_present"
)

// mailProfilePlatformName names a package ecosystem for the typed
// component_not_installable sentence ({platform}); "" when the platform is not
// verified, and the sentence then names no platform. The names are product
// names, the same in every language.
// mailProfilePlatformName, türlü cümle için paket ekosistemini adlandırır;
// platform doğrulanmamışsa boştur.
func mailProfilePlatformName(family string) string {
	switch family {
	case "apt":
		return "Debian/Ubuntu (apt)"
	case "pacman":
		return "Arch Linux (pacman)"
	case "dnf":
		return "Fedora/RHEL (dnf)"
	}
	return ""
}

// mailProfileTextCodes is every code above: the list the web test reads.
var mailProfileTextCodes = []mailProfileTextCode{
	mailProfileBlockedStateUnverified,
	mailProfileBlockedStateIncomplete,
	mailProfileBlockedCatalogueIncomplete,
	mailProfileBlockedHostnameUnknown,
	mailProfileBlockedDNSIdentity,
	mailProfileBlockedNotInstallable,
	mailProfileBlockedConflict,
	mailProfileBlockedRequires,
	mailProfileWarningRepairToVerify,
	mailProfileWarningWebmailUnverified,
	mailProfileWarningRoundcubeNotReady,
	mailProfileWarningPartlyPresent,
}

// The English sentences of the codes that carry no values. None asks the
// owner to scan: the panel checks the components by itself (D-032).
const (
	mailProfileStateUnverifiedText     = "The service state is not verified yet; the next component check verifies it."
	mailProfileStateIncompleteText     = "The service state that was read is incomplete; the next component check reads it again."
	mailProfileCatalogueIncompleteText = "The server profile catalogue is incomplete."
	mailProfileWebmailUnverifiedText   = "Webmail runtime readiness is not verified yet; the next component check verifies it."
	mailProfileRoundcubeNotReadyText   = "Roundcube is installed, but its Unix-socket runtime is not ready."
	mailProfilePartlyPresentText       = "Some components are already present. Installing this profile will reconcile the full plan without removing completed services."
)

// mailProfileHostBlockedCode types the host-wide blocked sentence handed to
// mailProfilesView; that sentence is always one of this package's constants.
func mailProfileHostBlockedCode(text string) mailProfileTextCode {
	switch text {
	case mailProfileServerHostnameUnknown:
		return mailProfileBlockedHostnameUnknown
	case mailProfileDNSIdentityMessage:
		return mailProfileBlockedDNSIdentity
	}
	return ""
}

// mailProfileServiceName is a catalogue service's display name, or the token
// itself (a requirement may name a role rather than a service).
func mailProfileServiceName(id string) string {
	if managed := core.GetManagedServiceByID(id); managed != nil {
		return managed.Name
	}
	return id
}

type mailProfileAttemptProof struct {
	Status   string
	Error    string
	Verified bool
}

// latestMailProfileAttemptProofs reads only the newest durable attempt for
// each profile. The current scan still has to prove every component ready
// before a profile may expose Verified; a receipt alone never makes a stopped
// profile healthy.
func (p *Panel) latestMailProfileAttemptProofs(ctx context.Context) (map[string]mailProfileAttemptProof, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT service_id, status, COALESCE(result_json, ''), COALESCE(error_message, '')
		FROM service_operations
		WHERE kind=?
		ORDER BY started_at DESC, updated_at DESC, created_at DESC, id DESC`,
		serviceOperationKindMailProfileInstall,
	)
	if err != nil {
		return nil, fmt.Errorf("read verified mail profiles: %w", err)
	}
	defer rows.Close()

	proofs := make(map[string]mailProfileAttemptProof, len(mailProfileDefinitions))
	seen := make(map[string]bool, len(mailProfileDefinitions))
	for rows.Next() {
		var profileID, status, resultJSON, errorMessage string
		if err := rows.Scan(&profileID, &status, &resultJSON, &errorMessage); err != nil {
			return nil, fmt.Errorf("decode verified mail profile: %w", err)
		}
		if seen[profileID] {
			continue
		}
		if profile, known := mailProfileByID(profileID); known {
			seen[profileID] = true
			proof := mailProfileAttemptProof{}
			switch status {
			case serviceOperationQueued, serviceOperationRunning:
				proof.Status = mailProfileAttemptInProgress
			case serviceOperationSucceeded:
				proof.Status = mailProfileAttemptSucceeded
				proof.Verified = mailProfileReceiptVerified(profile, resultJSON)
			case serviceOperationFailed:
				proof.Status = mailProfileAttemptFailed
				proof.Error = strings.TrimSpace(errorMessage)
			default:
				// The database status constraint should make this impossible. If
				// storage is corrupted, fail closed as an unresolved attempt.
				proof.Status = mailProfileAttemptInProgress
			}
			proofs[profileID] = proof
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read verified mail profiles: %w", err)
	}
	return proofs, nil
}

// mailProfileReceiptVerified accepts only the versioned proof emitted after
// every server-owned profile phase completed. Legacy or malformed success
// rows remain useful history, but cannot silently acquire today's stronger
// meaning and mark the Dashboard journey done.
func mailProfileReceiptVerified(profile mailProfileDefinition, resultJSON string) bool {
	var receipt mailProfileProofReceipt
	if err := json.Unmarshal([]byte(resultJSON), &receipt); err != nil {
		return false
	}
	return receipt.Success &&
		receipt.ProofVersion == mailProfileProofVersion &&
		receipt.ProfileID == profile.ID &&
		slices.Equal(receipt.Services, profile.Services) &&
		slices.Equal(receipt.CompletedServices, profile.Services) &&
		receipt.SubmissionConfigured &&
		receipt.MailTLS.Configured &&
		receipt.MailTLS.SNICount != nil &&
		*receipt.MailTLS.SNICount >= 0 &&
		receipt.MailTLS.FallbackOnly != nil &&
		*receipt.MailTLS.FallbackOnly == (*receipt.MailTLS.SNICount == 0)
}

func mailProfileByID(id string) (mailProfileDefinition, bool) {
	for _, profile := range mailProfileDefinitions {
		if profile.ID != id {
			continue
		}
		profile.Services = append([]string(nil), profile.Services...)
		return profile, true
	}
	return mailProfileDefinition{}, false
}

func allMailProfiles() []mailProfileDefinition {
	profiles := make([]mailProfileDefinition, 0, len(mailProfileDefinitions))
	for _, definition := range mailProfileDefinitions {
		profile, _ := mailProfileByID(definition.ID)
		profiles = append(profiles, profile)
	}
	return profiles
}

// handleMailProfileInstall creates one durable operation and one privileged
// agent lease for the complete, server-owned plan.
func (p *Panel) handleMailProfileInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	caller := currentCaller(r)
	if caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}

	var request mailProfileInstallRequest
	if err := decodeServiceOperationJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	request.ProfileID = strings.TrimSpace(request.ProfileID)
	if _, ok := mailProfileByID(request.ProfileID); !ok {
		writeClientError(w, http.StatusBadRequest, "unknown mail profile")
		return
	}
	if !request.Confirmed {
		writeCodedError(w, http.StatusBadRequest, errCodeMailProfileConfirmationRequired,
			"confirm the reviewed mail profile plan before starting", "")
		return
	}
	if !validServiceOperationID(request.RequestID) {
		writeClientError(w, http.StatusBadRequest, "invalid request_id")
		return
	}
	// A supplied mail hostname is validated before anything starts, so the
	// operator learns it is unusable in the dialog they typed it in rather
	// than from a failed operation minutes later. It is saved only once the
	// operation is admitted (below): a refusal because a setup run or another
	// operation is in progress leaves the saved name as it was (2026-10-16,
	// the mail host name can be corrected from this dialog).
	// Verilen ad hiçbir şey başlamadan önce doğrulanır; yalnız işlem kabul
	// edildikten sonra kaydedilir. Meşgul reddi kayıtlı adı değiştirmez.
	suppliedHostname := ""
	if supplied := strings.TrimSpace(request.MailHostname); supplied != "" {
		canonical, err := hostname.CanonicalFQDN(supplied)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest,
				errCodeMailProfileServerHostnameInvalid,
				mailProfileServerHostnameMessage, "")
			return
		}
		suppliedHostname = canonical
	}

	existing, found, err := p.idempotentServiceOperation(
		r.Context(), request.RequestID, serviceOperationKindMailProfileInstall, request.ProfileID, "",
	)
	if err != nil {
		if errors.Is(err, errServiceOperationRequestConflict) {
			writeServiceOperationRequestConflict(w)
			return
		}
		writeServerError(w, err)
		return
	}
	if found {
		writeAcceptedServiceOperation(w, existing)
		return
	}
	if err := p.authorizeAgentRPCContext(r.Context(), "Agent.SyncMailTLSV2"); err != nil {
		writeAgentError(w, err, "mail profile Mail TLS V2 platform preflight")
		return
	}
	// Fail closed before taking the process slot or creating the outer row: an
	// older agent must never install the mail stack and discover only afterward
	// that it cannot commit the required direct Mail TLS child.
	if err := p.requireMailTLSSyncV2Agent(r.Context()); err != nil {
		writeAgentError(w, err, "mail profile Mail TLS V2 preflight")
		return
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

	previousHostname := p.setting(r.Context(), settingMailHostname)
	if suppliedHostname != "" && suppliedHostname != previousHostname {
		if err := p.setSetting(r.Context(), settingMailHostname, suppliedHostname); err != nil {
			writeServerError(w, err)
			return
		}
	}
	actor := captureServiceOperationActor(r)
	op, err := p.createServiceOperationRequest(
		r.Context(), serviceOperationKindMailProfileInstall, request.ProfileID, "", request.RequestID, actor,
	)
	if err != nil && suppliedHostname != "" && suppliedHostname != previousHostname {
		// Not admitted after all: the name that was saved for it goes back.
		if restoreErr := p.setSetting(r.Context(), settingMailHostname, previousHostname); restoreErr != nil {
			log.Printf("mail hostname not restored after a refused profile request: %v", restoreErr)
		}
	}
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

	p.launchServiceOperation(
		op,
		actor,
		mailProfilePhase(request.ProfileID, "preflight"),
		"mail.profile.install:"+request.ProfileID,
		"mail.profile.install.failed:"+request.ProfileID,
		release,
		func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runMailProfileInstall(ctx, request.ProfileID, advance)
		},
	)
	releaseInHandler = false
	writeAcceptedServiceOperation(w, op)
}

func mailProfilePhase(profileID string, parts ...string) string {
	phase := "profile/" + profileID
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			phase += "/" + part
		}
	}
	return phase
}

func newMailProfileResult(profile mailProfileDefinition) serviceOperationResult {
	return serviceOperationResult{
		"success":               false,
		"proof_version":         mailProfileProofVersion,
		"profile_id":            profile.ID,
		"services":              append([]string(nil), profile.Services...),
		"completed_services":    []string{},
		"mail_tls":              mailProfileTLSResult{},
		"submission_configured": false,
		"warnings":              []string{},
	}
}

func mailProfileInstallFailure(cause error) *serviceOperationFailure {
	if errors.Is(cause, errMailProfileDNSIdentityNotReady) {
		return operationFailure(
			errCodeMailProfileDNSIdentityNotReady,
			mailProfileDNSIdentityMessage,
			cause,
		)
	}
	if errors.Is(cause, errMailProfileServerHostnameRequired) {
		return operationFailure(
			errCodeMailProfileServerHostnameRequired,
			mailProfileServerHostnameRequired,
			cause,
		)
	}
	if errors.Is(cause, errMailProfileServerHostnameInvalid) {
		return operationFailure(
			errCodeMailProfileServerHostnameInvalid,
			mailProfileServerHostnameMessage,
			cause,
		)
	}
	if failure := platformServiceOperationFailure(cause); failure != nil {
		return failure
	}
	// A sub-step refused by the Agent because the host was busy keeps that
	// typed cause and its reason sentence instead of the generic profile code
	// (upd8 F1: the profile's mail TLS sub-step was refused while a package task
	// ran, and the owner saw only mail_profile_install_failed).
	// Ana makine meşgul olduğu için reddedilen alt adım tipli nedenini korur.
	if classification, ok := classifyHostMutationError(cause); ok {
		return operationFailure(classification.Code, classification.Message, cause)
	}
	return operationFailure(
		"mail_profile_install_failed",
		"The mail profile could not be installed and verified.",
		cause,
	)
}

// mailTLSSyncFailure is the outcome of the mail TLS sub-step that runs after
// the mail profile installed. A busy host there has its own code: the profile
// did run, so it is never taken for a child refused at admission (review
// fix, 2026-10-29). Every other cause keeps the profile's classification.
const errCodeMailTLSSyncHostBusy = "mail_tls_sync_host_busy"

func mailTLSSyncFailure(cause error) *serviceOperationFailure {
	if _, ok := classifyHostMutationError(cause); ok {
		return operationFailure(errCodeMailTLSSyncHostBusy, "The mail server was installed, but setting up its certificates could not run because another change was running on this server. Nothing else was changed. You, the owner: once the other change has finished, choose Review a revised plan; the installed mail server is kept.", cause)
	}
	return mailProfileInstallFailure(cause)
}

func (p *Panel) runMailProfileInstall(
	ctx context.Context,
	profileID string,
	advance func(string) error,
) (serviceOperationResult, *serviceOperationFailure) {
	profile, ok := mailProfileByID(profileID)
	if !ok {
		return serviceOperationResult{"success": false},
			mailProfileInstallFailure(errors.New("unknown mail profile"))
	}
	result := newMailProfileResult(profile)
	if err := advance(mailProfilePhase(profile.ID, "preflight")); err != nil {
		return result, operationAdvanceFailure(err)
	}
	if _, err := p.preflightMailProfileInstall(ctx, profile); err != nil {
		return result, mailProfileInstallFailure(err)
	}
	binding, err := panelMutationBinding(ctx)
	if err != nil {
		return result, mailProfileInstallFailure(err)
	}

	// Mail identity belongs to Postfix and the mail TLS snapshot, not the OS.
	// Persist it before installing services so recovery and certificate renewal
	// use the same identity even when the system hostname is a short name.
	desiredHostname, err := p.resolveMailProfileHostname(ctx)
	if err != nil {
		return result, mailProfileInstallFailure(err)
	}
	if err := p.setSetting(ctx, settingMailHostname, desiredHostname); err != nil {
		return result, mailProfileInstallFailure(fmt.Errorf("save mail identity: %w", err))
	}
	result["mail_hostname"] = desiredHostname

	completed := make([]string, 0, len(profile.Services))
	for _, serviceID := range profile.Services {
		if err := advance(mailProfilePhase(profile.ID, serviceID, "installing")); err != nil {
			return result, operationAdvanceFailure(err)
		}
		serviceAdvance := func(phase string) error {
			return advance(mailProfilePhase(profile.ID, serviceID, phase))
		}
		_, failure := p.runServiceInstall(ctx, serviceInstallRequest{
			ServiceID: serviceID,
			RequestID: binding.MutationRequestID,
		}, serviceAdvance)
		if failure != nil {
			if failure.Cause != nil {
				failure.Cause = fmt.Errorf("install profile service %s: %w", serviceID, failure.Cause)
			}
			return result, failure
		}
		completed = append(completed, serviceID)
		result["completed_services"] = append([]string(nil), completed...)
	}

	mutationRequest := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
	if err := advance(mailProfilePhase(profile.ID, "mail-stack")); err != nil {
		return result, operationAdvanceFailure(err)
	}
	var mailStack transport.ConfigureMailStackResponse
	if err := p.callAgentContext(ctx, "Agent.ConfigureMailStack", &mutationRequest, &mailStack); err != nil {
		return result, mailProfileInstallFailure(fmt.Errorf("final mail stack configuration: %w", err))
	}
	if mailStack.Error != "" {
		return result, mailProfileInstallFailure(fmt.Errorf("final mail stack configuration: %s", mailStack.Error))
	}
	if !mailStack.Configured {
		return result, mailProfileInstallFailure(errors.New("agent did not confirm final mail stack configuration"))
	}

	if err := advance(mailProfilePhase(profile.ID, "submission")); err != nil {
		return result, operationAdvanceFailure(err)
	}
	var submission transport.ConfigureMailSubmissionResponse
	if err := p.callAgentContext(ctx, "Agent.ConfigureMailSubmission", &mutationRequest, &submission); err != nil {
		return result, mailProfileInstallFailure(fmt.Errorf("mail submission configuration: %w", err))
	}
	if submission.Error != "" {
		return result, mailProfileInstallFailure(fmt.Errorf("mail submission configuration: %s", submission.Error))
	}
	if !submission.Configured {
		return result, mailProfileInstallFailure(errors.New("agent did not confirm mail submission configuration"))
	}
	result["submission_configured"] = true

	// DKIM signing belongs to a working mail service (30 Oct 2026): OpenDKIM
	// is installed and wired here, so mail is signed without a first manual
	// "Generate DKIM key", and every domain gets its key once the profile is
	// done (queueAutomaticDKIMForAllDomains below). A failure here does not
	// fail the profile (mail is delivered either way); it is kept in the
	// result and every domain's card shows it through its automatic record.
	// DKIM imzalama çalışan posta hizmetinin parçasıdır; hata profili
	// düşürmez, sonuçta ve her alan adının kartında görünür.
	if err := advance(mailProfilePhase(profile.ID, "dkim-signing")); err != nil {
		return result, operationAdvanceFailure(err)
	}
	var signing transport.ConfigureDKIMSigningResponse
	signingErr := p.callAgentContext(ctx, "Agent.ConfigureDKIMSigning", &mutationRequest, &signing)
	switch {
	case signingErr != nil:
		result["dkim_signing"] = false
		result["dkim_signing_error"] = dkimAutomaticLine(signingErr.Error())
	case signing.Error != "" || !signing.Configured:
		result["dkim_signing"] = false
		result["dkim_signing_error"] = dkimAutomaticLine(signing.Error)
	default:
		result["dkim_signing"] = true
	}
	if result["dkim_signing"] == false {
		log.Printf("[mail-profile] %s: DKIM signing was not configured: %v", profile.ID, result["dkim_signing_error"])
	}

	if err := advance(mailProfilePhase(profile.ID, "verifying")); err != nil {
		return result, operationAdvanceFailure(err)
	}
	services, err := p.scanManagedServices(ctx)
	if err != nil {
		return result, mailProfileInstallFailure(fmt.Errorf("final mail profile scan: %w", err))
	}
	if err := verifyMailProfileReady(profile, services); err != nil {
		return result, mailProfileInstallFailure(err)
	}
	if err := p.verifyMailProfileRuntimeProof(ctx, profile); err != nil {
		return result, mailProfileInstallFailure(err)
	}

	// Existing domains now have mail here: their keys, signing and records
	// follow in the background once this operation releases the host.
	p.queueAutomaticDKIMForAllDomains(context.WithoutCancel(ctx))

	result["success"] = true
	return result, nil
}

// preflightMailProfileInstall proves the whole plan before the first package
// or portable-application mutation. Requirements are checked against the
// projected final set, never incrementally against client-supplied ordering.
func (p *Panel) preflightMailProfileInstall(
	ctx context.Context,
	profile mailProfileDefinition,
) ([]ManagedServiceResponse, error) {
	if _, err := panelMutationBinding(ctx); err != nil {
		return nil, err
	}
	if err := p.requireMailProfileDNSIdentity(ctx); err != nil {
		return nil, err
	}
	if err := p.validateMailProfileHostAndCatalog(ctx, profile); err != nil {
		return nil, err
	}
	services, err := p.scanManagedServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail profile preflight scan: %w", err)
	}
	installed := make(map[string]bool, len(services)+len(profile.Services))
	observed := make(map[string]bool, len(services))
	for _, service := range services {
		observed[service.ID] = true
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	projected := make(map[string]bool, len(installed)+len(profile.Services))
	for id, present := range installed {
		projected[id] = present
	}
	for _, id := range profile.Services {
		if !observed[id] {
			return nil, fmt.Errorf("fresh scan did not return catalogue service %s", id)
		}
		projected[id] = true
	}
	for _, id := range profile.Services {
		managed := core.GetManagedServiceByID(id)
		if taken := core.SeatTakenBy(managed, projected); taken != "" {
			return nil, fmt.Errorf("%s cannot be installed while %s occupies the same service role", managed.Name, taken)
		}
		if missing := core.RequirementsMissing(managed, projected); len(missing) > 0 {
			return nil, fmt.Errorf("%s requires %s outside this profile", managed.Name, strings.Join(missing, ", "))
		}
	}
	return services, nil
}

func (p *Panel) validateMailProfileHostAndCatalog(ctx context.Context, profile mailProfileDefinition) error {
	if err := p.authorizeAgentRPCContext(ctx, "Agent.SyncMailTLSV2"); err != nil {
		return err
	}
	if err := p.requireMailTLSSyncV2Agent(ctx); err != nil {
		return err
	}
	if _, err := p.resolveMailProfileHostname(ctx); err != nil {
		return err
	}
	family := p.packageFamily()
	for _, id := range profile.Services {
		managed := core.GetManagedServiceByID(id)
		if managed == nil {
			return fmt.Errorf("mail profile contains unknown managed service %s", id)
		}
		if reason := core.ManagedServiceInstallDisabledReason(managed, family); reason != "" {
			return fmt.Errorf("%s: %s", managed.Name, reason)
		}
		if err := p.preflightManagedServiceRepository(ctx, managed, family, ""); err != nil {
			return err
		}
	}
	return nil
}

// resolveMailProfileHostname is the single answer to "what will the mail
// server be called". Everything downstream — the preflight, the catalogue and
// the install itself — asks this one function, so the screen can never promise
// a name the install would not use.
// resolveMailProfileHostname, "posta sunucusunun adı ne olacak" sorusunun tek
// yanıtıdır. Aşağıdaki her şey — ön kontrol, katalog ve kurulumun kendisi — bu
// tek işleve sorar; böylece ekran, kurulumun kullanmayacağı bir adı asla vaat
// edemez.
func (p *Panel) resolveMailProfileHostname(ctx context.Context) (string, error) {
	// Setup carries the exact reviewed identity through recovery; later settings
	// edits cannot redirect an already authorized mail installation.
	if reviewed, ok := ctx.Value(serverSetupMailHostnameKey{}).(string); ok {
		canonical, err := hostname.CanonicalFQDN(reviewed)
		if err != nil || canonical != reviewed {
			return "", errMailProfileServerHostnameInvalid
		}
		return canonical, nil
	}
	if _, err := readMailProfileHostname(); err != nil {
		return "", fmt.Errorf("read mail profile server hostname: %w", err)
	}
	identity := p.mailHostnameIdentity(ctx)
	if identity.Hostname == "" {
		return "", errMailProfileServerHostnameRequired
	}
	return identity.Hostname, nil
}

// mailProfileHostBlockedReason exposes the safe, actionable hostname gate in
// the read-only catalogue. A server without a fully qualified name is no
// longer blocked: the install asks for a separate mail identity. The
// catalogue only fails closed when this server's own hostname cannot be read
// at all, because then nothing about the name can be decided.
// mailProfileHostBlockedReason, salt-okunur katalogdaki güvenli ve eyleme
// dönük ana bilgisayar adı kapısını gösterir. Tam nitelikli adı olmayan bir
// sunucu artık engellenmez: kurulum ekranı ayrı bir posta kimliği ister.
// Katalog yalnız bu sunucunun kendi adı hiç okunamadığında kapanır; çünkü o
// zaman ad hakkında hiçbir şeye karar verilemez.
func mailProfileHostBlockedReason() string {
	if _, err := readMailProfileHostname(); err != nil {
		return mailProfileServerHostnameUnknown
	}
	return ""
}

func verifyMailProfileReady(profile mailProfileDefinition, services []ManagedServiceResponse) error {
	for _, serviceID := range profile.Services {
		managed := core.GetManagedServiceByID(serviceID)
		if managed == nil {
			return fmt.Errorf("final scan contains unknown profile service %s", serviceID)
		}
		ready := false
		for _, service := range services {
			if service.ID != serviceID || !service.Installed() {
				continue
			}
			ready = managed.Kind == core.KindTool ||
				strings.HasPrefix(strings.ToLower(strings.TrimSpace(service.Status)), "active")
			break
		}
		if !ready {
			return fmt.Errorf("final scan did not find %s in its required state", serviceID)
		}
	}
	return nil
}

func (p *Panel) verifyMailProfileRuntimeProof(ctx context.Context, profile mailProfileDefinition) error {
	if profile.ID != "webmail" {
		return nil
	}
	ready, proven, err := p.cachedWebmailReadinessProof(ctx)
	if err != nil {
		return fmt.Errorf("read final webmail readiness proof: %w", err)
	}
	if !proven || !ready {
		return errors.New("Roundcube webmail did not pass its final Unix-socket readiness probe")
	}
	return nil
}

// reconstructSucceededMailProfileResult closes the narrow crash window after
// the agent durably committed success but before the panel stored its result.
// The agent terminal state proves all mutation steps completed; current fresh
// reads reconstruct the truthful profile membership and fallback warning.
func (p *Panel) reconstructSucceededMailProfileResult(
	ctx context.Context,
	profile mailProfileDefinition,
) (serviceOperationResult, error) {
	if err := p.validateMailProfileHostAndCatalog(ctx, profile); err != nil {
		return nil, err
	}
	services, err := p.scanManagedServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("recover mail profile scan: %w", err)
	}
	if err := verifyMailProfileReady(profile, services); err != nil {
		return nil, err
	}
	if err := p.verifyMailProfileRuntimeProof(ctx, profile); err != nil {
		return nil, err
	}

	mailTLSSyncMu.Lock()
	_, sni, err := p.loadMailTLSSnapshotLocked(ctx, 0)
	mailTLSSyncMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("reconstruct mail TLS result: %w", err)
	}
	result := newMailProfileResult(profile)
	result["completed_services"] = append([]string(nil), profile.Services...)
	result["mail_tls"] = mailProfileTLSResult{
		Configured:   true,
		SNICount:     len(sni),
		FallbackOnly: len(sni) == 0,
	}
	result["submission_configured"] = true
	result["recovered"] = true
	result["success"] = true
	if len(sni) == 0 {
		result["warnings"] = []string{mailProfileFallbackWarning}
	}
	return result, nil
}

// componentPlanBlock is the blocking decision shared by the mail profiles and
// the ready bundles (alpha.94): each component in check, in order, must be
// installable on this package family, must not take a seat that another
// component of the projected set holds, and must find its requirements in
// that projected set (what is installed plus the whole plan). The first
// refusal wins and is returned as the English sentence, its typed code and
// values. Empty means nothing blocks. The mail profiles check every component
// of their plan, exactly as before; a bundle checks only what it would install.
// componentPlanBlock, posta profilleri ile hazır paketlerin ortak engel
// kararıdır; ilk ret kazanır, boş sonuç engel yok demektir.
func componentPlanBlock(check []string, projected map[string]bool, packageFamily string) (string, mailProfileTextCode, map[string]string) {
	for _, id := range check {
		managed := core.GetManagedServiceByID(id)
		if managed == nil {
			return mailProfileCatalogueIncompleteText, mailProfileBlockedCatalogueIncomplete, nil
		}
		if kind, reason := core.ManagedServiceInstallBlock(managed, packageFamily); reason != "" {
			vars := map[string]string{"component": managed.Name, "kind": string(kind)}
			// The old text named the platform through its reason; the typed
			// sentence names it by a value both languages can show.
			// Eski metin platformu gerekçesiyle anıyordu; türlü cümle onu
			// iki dilin de gösterebileceği bir değerle adlandırır.
			if platform := mailProfilePlatformName(packageFamily); platform != "" {
				vars["platform"] = platform
			}
			return managed.Name + ": " + reason, mailProfileBlockedNotInstallable, vars
		}
		if taken := core.SeatTakenBy(managed, projected); taken != "" {
			return fmt.Sprintf("%s conflicts with installed %s.", managed.Name, taken),
				mailProfileBlockedConflict,
				map[string]string{"component": managed.Name, "conflicting": taken}
		}
		if missing := core.RequirementsMissing(managed, projected); len(missing) > 0 {
			names := make([]string, 0, len(missing))
			for _, token := range missing {
				names = append(names, mailProfileServiceName(token))
			}
			return fmt.Sprintf("%s requires %s outside this profile.", managed.Name, strings.Join(missing, ", ")),
				mailProfileBlockedRequires,
				map[string]string{"component": managed.Name, "required": strings.Join(names, ", ")}
		}
	}
	return "", "", nil
}

// componentReady is one component's readiness on a scan, as the final check
// of a mail profile reads it: installed, and active unless it is a tool.
// componentReady, bir bileşenin taramadaki hazır oluşudur.
func componentReady(id string, services []ManagedServiceResponse) bool {
	return verifyMailProfileReady(mailProfileDefinition{Services: []string{id}}, services) == nil
}

func mailProfilesView(
	services []ManagedServiceResponse,
	verified bool,
	packageFamily string,
	hostBlockedReason string,
	webmailReady bool,
	webmailProven bool,
	profileAttemptProofs map[string]mailProfileAttemptProof,
) []MailProfileResponse {
	byID := make(map[string]ManagedServiceResponse, len(services))
	installed := make(map[string]bool, len(services))
	for _, service := range services {
		byID[service.ID] = service
		if service.Installed() {
			installed[service.ID] = true
		}
	}

	profiles := make([]MailProfileResponse, 0, len(mailProfileDefinitions))
	for _, definition := range allMailProfiles() {
		proof, attempted := profileAttemptProofs[definition.ID]
		if !attempted {
			proof.Status = mailProfileAttemptNone
		}
		view := MailProfileResponse{
			ID:                  definition.ID,
			Name:                definition.Name,
			Description:         definition.Description,
			Services:            append([]string(nil), definition.Services...),
			LatestAttemptStatus: proof.Status,
			LatestAttemptError:  proof.Error,
			Status:              mailProfileStatusUnknown,
			Available:           false,
		}
		if !verified {
			view.BlockedReason = mailProfileStateUnverifiedText
			view.BlockedCode = mailProfileBlockedStateUnverified
			profiles = append(profiles, view)
			continue
		}
		if hostBlockedReason != "" {
			view.Status = mailProfileStatusBlocked
			view.BlockedReason = hostBlockedReason
			view.BlockedCode = mailProfileHostBlockedCode(hostBlockedReason)
			profiles = append(profiles, view)
			continue
		}

		projected := make(map[string]bool, len(installed)+len(definition.Services))
		for id, present := range installed {
			projected[id] = present
		}
		incompleteObservation := false
		for _, id := range definition.Services {
			if _, ok := byID[id]; !ok {
				incompleteObservation = true
			}
			projected[id] = true
		}
		if incompleteObservation {
			view.BlockedReason = mailProfileStateIncompleteText
			view.BlockedCode = mailProfileBlockedStateIncomplete
			profiles = append(profiles, view)
			continue
		}

		blockedReason, blockedCode, blockedVars := componentPlanBlock(definition.Services, projected, packageFamily)
		if blockedReason != "" {
			view.Status = mailProfileStatusBlocked
			view.BlockedReason = blockedReason
			view.BlockedCode = blockedCode
			view.BlockedVars = blockedVars
			profiles = append(profiles, view)
			continue
		}

		anyInstalled := false
		anyPresent := false
		allReady := true
		for _, id := range definition.Services {
			service := byID[id]
			if service.Installed() {
				anyPresent = true
				if mailProfileMailComponents[id] {
					anyInstalled = true
				}
			}
			if !componentReady(id, services) {
				allReady = false
			}
		}
		webmailWarning := ""
		var webmailWarningCode mailProfileTextCode
		if definition.ID == "webmail" && allReady {
			switch {
			case !webmailProven:
				allReady = false
				webmailWarning, webmailWarningCode = mailProfileWebmailUnverifiedText, mailProfileWarningWebmailUnverified
			case !webmailReady:
				allReady = false
				webmailWarning, webmailWarningCode = mailProfileRoundcubeNotReadyText, mailProfileWarningRoundcubeNotReady
			}
		}
		view.Available = true
		switch {
		case allReady:
			view.Status = mailProfileStatusComplete
			view.Verified = proof.Verified
			if !view.Verified {
				// A service scan can prove component membership and runtime state, but
				// only the bound profile operation proves its final MailStack, TLS and
				// submission reconciliations. Keep that distinction visible instead of
				// over-claiming a complete mail setup from component state alone.
				view.Warning = mailProfileReconciliationWarning
				view.WarningCode = mailProfileWarningRepairToVerify
			}
		case anyInstalled:
			view.Status = mailProfileStatusPartial
			if webmailWarning != "" {
				view.Warning, view.WarningCode = webmailWarning, webmailWarningCode
			} else {
				view.Warning, view.WarningCode = mailProfilePartlyPresentText, mailProfileWarningPartlyPresent
			}
		default:
			view.Status = mailProfileStatusAvailable
			if anyPresent {
				// Only web-server parts (Nginx, PHP-FPM) are here: the profile is
				// not installed, but the note about reusing them stays.
				// Burada yalnız web sunucusu parçaları var: profil kurulu değil,
				// ama onları yeniden kullanma notu kalır.
				view.Warning, view.WarningCode = mailProfilePartlyPresentText, mailProfileWarningPartlyPresent
			}
		}
		profiles = append(profiles, view)
	}
	inheritSupersetMailProfileProof(profiles)
	return profiles
}

// inheritSupersetMailProfileProof marks a complete but unproven profile as
// verified when another profile that contains every one of its components is
// itself verified on the same scan. Webmail and Spam-Protected Mail run the
// same Postfix and Dovecot reconciliation (MailStack, mail TLS, authenticated
// submission) as Core Mail; their bound receipt proves that work, so asking
// the owner to repair Core Mail afterwards was a false alarm (owner
// observation on Frankfurt, alpha.92). Nothing else is relaxed: component
// state alone still proves nothing, and a profile whose own latest attempt is
// running or failed keeps its own state and warning.
//
// inheritSupersetMailProfileProof, tamamlanmış ama kanıtı olmayan bir profili,
// onun bütün bileşenlerini içeren başka bir profil aynı taramada doğrulanmışsa
// doğrulanmış sayar. Web posta ve Spam Korumalı Posta, Temel Posta ile aynı
// Postfix ve Dovecot uzlaştırmasını yapar; bağlı makbuzları bu işi kanıtlar.
// Başka hiçbir kural gevşemez: yalnız bileşen durumu yine kanıt değildir;
// kendi son denemesi süren ya da başarısız olan profil kendi durumunu korur.
func inheritSupersetMailProfileProof(profiles []MailProfileResponse) {
	for i := range profiles {
		candidate := &profiles[i]
		if candidate.Status != mailProfileStatusComplete || candidate.Verified ||
			candidate.WarningCode != mailProfileWarningRepairToVerify {
			continue
		}
		switch candidate.LatestAttemptStatus {
		case mailProfileAttemptInProgress, mailProfileAttemptFailed:
			continue
		}
		for j := range profiles {
			superset := profiles[j]
			if i == j || superset.Status != mailProfileStatusComplete || !superset.Verified ||
				!mailProfileServicesContain(superset.Services, candidate.Services) {
				continue
			}
			candidate.Verified = true
			candidate.Warning = ""
			candidate.WarningCode = ""
			break
		}
	}
}

func mailProfileServicesContain(superset, subset []string) bool {
	for _, id := range subset {
		if !slices.Contains(superset, id) {
			return false
		}
	}
	return true
}
