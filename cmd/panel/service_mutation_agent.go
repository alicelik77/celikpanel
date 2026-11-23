package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

const (
	agentMutationRunning    = "running"
	agentMutationCancelling = "cancelling"
	agentMutationOrphaned   = "orphaned"
	agentMutationPending    = "pending"
	agentMutationSucceeded  = "succeeded"
	agentMutationFailed     = "failed"

	errCodeServiceOperationFailed             = "service_operation_failed"
	errCodeServiceOperationHeartbeatUncertain = "service_operation_heartbeat_uncertain"
	agentMutationLeaseExpiredErrorCode        = "service_mutation_lease_expired"

	panelMutationHeartbeatInterval = 5 * time.Second
	panelMutationFinishTimeout     = 15 * time.Second
	panelMutationRecoveryTimeout   = 90 * time.Second
	// The agent may spend two minutes forward-converging a committed Mail TLS
	// plan. Keep a separate end-to-end budget with room for capability checks,
	// snapshot inspection, durable lease publication, and terminal receipt.
	panelMailTLSSyncTimeout = 5 * time.Minute
	// These kinds also make a durable forward-only decision before their RPC
	// response is authoritative. Their exact terminal receipt must outlive the
	// generic 15-second control-RPC reconciliation window.
	panelFirewallCommitReconcileTimeout = 90 * time.Second
	panelDNSCommitReconcileTimeout      = 5 * time.Minute

	dnsEngineSwitchLegacyPublishedPhasePrefix = "commit/dns-engine-switch/v1/published/"
	dnsEngineSwitchFinalizedPhasePrefix       = "commit/dns-engine-switch/v2/finalized/"
)

var waitExpectedAgentMutationTerminalFn = func(
	panel *Panel,
	ctx context.Context,
	identity agentMutationIdentity,
) (*agentMutationJob, error) {
	return panel.waitExpectedAgentMutationTerminal(ctx, identity)
}

func panelMutationTerminalReconcileTimeout(identity agentMutationIdentity) time.Duration {
	switch identity.Kind {
	case "mail_tls_sync":
		return panelMailTLSSyncTimeout
	case "firewall_apply", "firewall_sync":
		return panelFirewallCommitReconcileTimeout
	case "dns_zone_sync":
		return panelDNSCommitReconcileTimeout
	case dnsEngineSwitchKind:
		return dnsEngineSwitchTimeout
	case "dns_cluster_configure":
		return 5 * time.Minute
	default:
		return panelMutationFinishTimeout
	}
}

type agentMutationBinding = transport.ServiceMutationBinding

type agentMutationJob = transport.ServiceMutationJob

type agentMutationResponse = transport.ServiceMutationResponse

type panelMutationContextKey struct{}

var errAgentMutationIdentityMismatch = errors.New(
	"agent service mutation response identity does not match the requested durable operation",
)

var errAgentMutationLeaseLost = errors.New(
	"agent service mutation lease is no longer owned by the requested durable operation",
)

var errAgentMutationTerminalFailed = errors.New(
	"agent service mutation reached a verified terminal failure",
)

var errAgentMutationPublishedReceiptMismatch = errors.New(
	"agent service mutation success lacks its exact canonical published receipt",
)

var errAgentMutationRecoveryRequired = errors.New(
	"agent service mutation requires privileged recovery before it can be committed",
)

var errHostMutationBusy = errors.New(
	"another server change or package-manager task is still running",
)

// hostMutationBusyError is errHostMutationBusy plus the one thing the agent
// knew and the panel used to drop: which of three is blocking the change. It
// unwraps to the sentinel, so everything that tested for the sentinel still
// does; the reason is read only by the classifier that writes the sentence.
//
// hostMutationBusyError, errHostMutationBusy'nin ustune agent'in bildigi ve
// panelin dusurdugu tek seyi ekler: uc seyden hangisinin engelledigini.
type hostMutationBusyError struct {
	reason string
	// holder names the other Agent job that held the lease when the Agent
	// reported it with the refusal (kind and target, never a secret), so a
	// log line can say what kept the host busy (final alpha.113 run 5, D10:
	// "no log names the holder"). Empty when the Agent did not say.
	// Kirayi tutan diger Agent isi (tur ve hedef); bilinmiyorsa bos.
	holder string
}

func (e *hostMutationBusyError) Error() string { return errHostMutationBusy.Error() }

func (e *hostMutationBusyError) Unwrap() error { return errHostMutationBusy }

func serviceMutationResponseError(response agentMutationResponse) error {
	switch response.ErrorCode {
	case transport.HostMutationBusy:
		return &hostMutationBusyError{reason: response.Reason}
	case "":
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	default:
		return fmt.Errorf(
			"agent returned unsupported service mutation error code %q",
			response.ErrorCode,
		)
	}
}

// agentMutationTerminalUncertainError means an exact forward-only payload may
// already have a durable intent but its terminal receipt could not be observed.
// Callers must retain the newly requested database state; rolling it back could
// make the ledger disagree with the plan the agent will finish after recovery.
type agentMutationTerminalUncertainError struct {
	kind string
	err  error
}

func (e *agentMutationTerminalUncertainError) Error() string {
	return "durable " + e.kind + " terminal state is uncertain: " + e.err.Error()
}

func (e *agentMutationTerminalUncertainError) Unwrap() error { return e.err }

func payloadBoundMutationTerminalError(
	identity agentMutationIdentity,
	observed *agentMutationJob,
	err error,
) error {
	if err == nil {
		return nil
	}
	switch identity.Kind {
	case "mail_tls_sync", "firewall_apply", "firewall_sync", "dns_zone_sync", "dns_cluster_configure", dnsEngineSwitchKind:
		if observed == nil || !identity.matches(observed) || agentMutationActive(observed.Status) {
			return &agentMutationTerminalUncertainError{kind: identity.Kind, err: err}
		}
	}
	return err
}

func mutationTerminalUncertain(err error) bool {
	var target *agentMutationTerminalUncertainError
	return errors.As(err, &target) || errors.Is(err, errAgentMutationRecoveryRequired)
}

// hostMutationRefusedAsBusy is the one retry decision for work refused because
// the host was busy (first written for deferred startup mail work, upd11 F2;
// shared with setup steps on 2026-10-30): only a busy refusal is retried, and a
// terminal-uncertain mutation never is - it belongs to exact durable recovery.
// An unknown outcome (a lost reply, a transport failure) is not a busy refusal.
// Yalnız meşgul reddi yeniden denenir; sonucu belirsiz bir değişiklik asla.
func hostMutationRefusedAsBusy(err error) bool {
	return err != nil &&
		errors.Is(err, errHostMutationBusy) &&
		!mutationTerminalUncertain(err)
}

// hostMutationBusyHolderName is "<kind> <target>" of the job that held the
// lease, bounded; kinds and targets are service, domain or step names.
func hostMutationBusyHolderName(job *agentMutationJob) string {
	name := strings.TrimSpace(strings.TrimSpace(job.Kind) + " " + strings.TrimSpace(job.Target))
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
}

// hostMutationBusyLogDetail is what a log line adds to a busy refusal: the
// typed reason and, when the Agent named it, the job that held the host.
// Meşgul reddi için günlük ayrıntısı: neden ve biliniyorsa tutan iş.
func hostMutationBusyLogDetail(err error) string {
	var busy *hostMutationBusyError
	if !errors.As(err, &busy) {
		return ""
	}
	detail := "reason=" + busy.reason
	if busy.reason == "" {
		detail = "reason=unnamed"
	}
	if busy.holder != "" {
		detail += " held_by=" + strconv.Quote(busy.holder)
	}
	return detail
}

// hostMutationBusyReason is the typed reason of a busy refusal, or "".
func hostMutationBusyReason(err error) string {
	var busy *hostMutationBusyError
	if errors.As(err, &busy) {
		return busy.reason
	}
	return ""
}

type agentMutationIdentity struct {
	RequestID   string
	OwnerID     string
	Kind        string
	Target      string
	PackageName string
}

func agentMutationIdentityForOperation(
	op serviceOperation,
	ownerID string,
) agentMutationIdentity {
	return agentMutationIdentity{
		RequestID:   op.RequestID,
		OwnerID:     ownerID,
		Kind:        op.Kind,
		Target:      op.ServiceID,
		PackageName: op.PackageName,
	}
}

func (identity agentMutationIdentity) matches(job *agentMutationJob) bool {
	return job != nil &&
		job.RequestID == identity.RequestID &&
		job.OwnerID == identity.OwnerID &&
		job.Kind == identity.Kind &&
		job.Target == identity.Target &&
		job.PackageName == identity.PackageName
}

func validateAgentMutationIdentity(
	job *agentMutationJob,
	identity agentMutationIdentity,
) error {
	if identity.matches(job) {
		return nil
	}
	return errAgentMutationIdentityMismatch
}

func payloadBoundMutationPublishedPhase(
	identity agentMutationIdentity,
) (string, bool, error) {
	switch identity.Kind {
	case "vpn_peer_sync":
		if identity.Target != "wireguard" ||
			!mutationpayload.ValidVPNPeerSyncQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/vpn-peer-sync/v1/published/" + identity.RequestID + "/" + identity.PackageName, true, nil
	case "firewall_apply", "firewall_sync":
		if identity.Target != "nftables" ||
			!mutationpayload.ValidFirewallApplyQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/firewall-apply/v1/published/" + identity.RequestID + "/" + identity.PackageName, true, nil
	case "mail_tls_sync":
		if identity.Target != "mail-tls" ||
			!mutationpayload.ValidMailTLSSyncQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/mail-tls-sync/v1/published/" + identity.RequestID + "/" + identity.PackageName, true, nil
	case "dns_cluster_configure":
		if identity.Target != "pdns" ||
			!mutationpayload.ValidDNSClusterConfigQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/dns-cluster-config/v1/published/" + identity.RequestID + "/" + identity.PackageName, true, nil
	case "dns_zone_sync":
		canonicalTarget, err := hostname.CanonicalFQDN(identity.Target)
		if err != nil || canonicalTarget != identity.Target {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		switch {
		case mutationpayload.ValidDNSZoneSyncQualifier(identity.PackageName):
			return "commit/dns-zone-sync/v1/published/" + identity.RequestID + "/" + identity.Target + "/" + identity.PackageName, true, nil
		case mutationpayload.ValidDNSZoneSyncV3Qualifier(identity.PackageName):
			return "commit/dns-zone-sync/v3/published/" + identity.RequestID + "/" + identity.Target + "/" + identity.PackageName, true, nil
		default:
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
	case dnsEngineSwitchKind:
		if !transport.ValidDNSEngine(transport.DNSEngine(identity.Target)) ||
			!mutationpayload.ValidDNSEngineSwitchQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return dnsEngineSwitchFinalizedPhasePrefix +
			identity.RequestID + "/" + identity.PackageName, true, nil
	case "panel_certificate_issue":
		canonicalTarget, err := hostname.CanonicalFQDN(identity.Target)
		if err != nil || canonicalTarget != identity.Target ||
			!mutationpayload.ValidPanelCertificateIssueQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/panel-certificate-issue/v1/published/" + identity.RequestID + "/" + identity.Target + "/" + identity.PackageName, true, nil
	case "mail_host_certificate":
		canonicalTarget, err := hostname.CanonicalFQDN(identity.Target)
		if err != nil || canonicalTarget != identity.Target ||
			!mutationpayload.ValidMailHostCertificateQualifier(identity.PackageName) {
			return "", true, errAgentMutationPublishedReceiptMismatch
		}
		return "commit/mail-host-certificate/v1/published/" + identity.RequestID + "/" + identity.Target + "/" + identity.PackageName, true, nil
	default:
		return "", false, nil
	}
}

func validateAgentMutationSucceededReceipt(
	job *agentMutationJob,
	identity agentMutationIdentity,
) error {
	if err := validateAgentMutationIdentity(job, identity); err != nil {
		return err
	}
	if job.Status != agentMutationSucceeded {
		return fmt.Errorf("agent service mutation is not successful: %s", job.Status)
	}
	expected, required, err := payloadBoundMutationPublishedPhase(identity)
	if err != nil {
		return err
	}
	if required && job.Phase != expected {
		if identity.Kind == dnsEngineSwitchKind {
			return fmt.Errorf(
				"%w: %w: got %q, want exact finalized receipt %q",
				errAgentMutationRecoveryRequired,
				errAgentMutationPublishedReceiptMismatch,
				job.Phase,
				expected,
			)
		}
		return fmt.Errorf(
			"%w: got %q, want %q",
			errAgentMutationPublishedReceiptMismatch,
			job.Phase,
			expected,
		)
	}
	return nil
}

func cloneAgentMutationJob(job *agentMutationJob) *agentMutationJob {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func withPanelMutationBinding(ctx context.Context, binding agentMutationBinding) context.Context {
	return context.WithValue(ctx, panelMutationContextKey{}, binding)
}

func panelMutationBinding(ctx context.Context) (agentMutationBinding, error) {
	binding, ok := ctx.Value(panelMutationContextKey{}).(agentMutationBinding)
	if !ok || !validServiceOperationID(binding.MutationRequestID) ||
		!validServiceOperationID(binding.MutationOwnerID) {
		return agentMutationBinding{}, errors.New("durable service mutation binding is missing")
	}
	return binding, nil
}

func agentMutationActive(status string) bool {
	return status == agentMutationRunning ||
		status == agentMutationCancelling ||
		status == agentMutationOrphaned
}

func (p *Panel) beginAgentMutation(
	ctx context.Context,
	op serviceOperation,
	ownerID string,
	resume bool,
) (*agentMutationJob, error) {
	var response agentMutationResponse
	err := p.callAgentContext(ctx, "Agent.BeginServiceMutation", &transport.ServiceMutationBeginRequest{
		RequestID: op.RequestID, OwnerID: ownerID, Kind: op.Kind,
		Target: op.ServiceID, PackageName: op.PackageName, Resume: resume,
	}, &response)
	if err != nil {
		return nil, fmt.Errorf("begin agent service mutation: %w", err)
	}
	if responseErr := serviceMutationResponseError(response); responseErr != nil {
		var busy *hostMutationBusyError
		if errors.As(responseErr, &busy) && response.Job != nil && response.Job.RequestID != op.RequestID {
			busy.holder = hostMutationBusyHolderName(response.Job)
		}
		return response.Job, responseErr
	}
	if response.Job == nil || response.Job.RequestID != op.RequestID ||
		response.Job.OwnerID != ownerID || response.Job.Kind != op.Kind ||
		response.Job.Target != op.ServiceID || response.Job.PackageName != op.PackageName ||
		response.Job.Status != agentMutationRunning {
		return response.Job, errors.New("agent did not grant the requested service mutation lease")
	}
	return response.Job, nil
}

func (p *Panel) heartbeatAgentMutation(
	ctx context.Context,
	binding agentMutationBinding,
	phase string,
) (*agentMutationJob, error) {
	var response agentMutationResponse
	err := p.callAgentContext(ctx, "Agent.HeartbeatServiceMutation", &transport.ServiceMutationHeartbeatRequest{
		RequestID: binding.MutationRequestID,
		OwnerID:   binding.MutationOwnerID,
		Phase:     strings.TrimSpace(phase),
	}, &response)
	if err != nil {
		return response.Job, err
	}
	responseErr := serviceMutationResponseError(response)
	if response.Job != nil &&
		(response.Job.RequestID != binding.MutationRequestID ||
			response.Job.OwnerID != binding.MutationOwnerID) {
		if responseErr != nil {
			return response.Job, errors.Join(errAgentMutationLeaseLost, responseErr)
		}
		return response.Job, errAgentMutationLeaseLost
	}
	if response.Job != nil && response.Job.Status != agentMutationRunning {
		terminalErr := errAgentMutationLeaseLost
		if response.Job.Status == agentMutationFailed &&
			response.Job.ErrorCode != agentMutationLeaseExpiredErrorCode {
			terminalErr = errAgentMutationTerminalFailed
		}
		if responseErr != nil {
			return response.Job, errors.Join(terminalErr, responseErr)
		}
		return response.Job, terminalErr
	}
	if responseErr != nil {
		return response.Job, responseErr
	}
	if response.Job == nil {
		return nil, errors.New("agent service mutation heartbeat did not return a durable lease")
	}
	return response.Job, nil
}

// R-056. "The privileged host operation did not complete." is true of every
// failed host mutation and therefore says nothing about any one of them. It is
// the right answer when the panel genuinely does not know what happened - a
// lost lease, an unverifiable heartbeat - and the wrong one when the host
// already said why. A caller that has the host's own sentence attaches it with
// namedHostOperationFailure, and the ledger records that instead. Nothing else
// changes: the code stays the generic one, because the code is what the panel
// branches on, and only the sentence an operator reads is replaced.
//
// R-056. "Ayricalikli makine islemi tamamlanmadi." her basarisiz mutasyon icin
// dogrudur, bu yuzden hicbiri hakkinda bir sey soylemez. Makine nedenini zaten
// soylediyse yanlis yanittir. Kodu degismez; yalnizca operatorun okudugu cumle
// degisir.
type namedHostOperationError struct {
	sentence string
	cause    error
}

func (e *namedHostOperationError) Error() string { return e.sentence }
func (e *namedHostOperationError) Unwrap() error { return e.cause }

// A sentence that ends up here is written durably into the ledger and shown to
// an operator, and part of it is the host's own words. So it is bounded and
// stripped of anything that could forge a line, and the order is the one R-054
// wrote down and R-055 needed again: what the panel authored comes first, the
// host's reason last, because truncation has to eat the tail and never the
// sentence the operator acts on.
//
// Buraya gelen bir cumle deftere kalici yazilir ve operatore gosterilir; bir
// kismi makinenin kendi sozleridir. Bu yuzden sinirlanir ve satir uydurabilecek
// her seyden arindirilir; sira R-054'un yazdigi siradir.
const namedHostOperationSentenceLimit = 400

func boundedOperatorSentence(sentence string) string {
	sentence = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, sentence)
	sentence = strings.TrimSpace(sentence)
	runes := []rune(sentence)
	if len(runes) > namedHostOperationSentenceLimit {
		return string(runes[:namedHostOperationSentenceLimit]) + "..."
	}
	return sentence
}

// namedHostOperationFailure attaches the sentence an operator should read to a
// failure the host explained. An empty sentence changes nothing.
// namedHostOperationFailure, makinenin acikladigi bir basarisizliga operatorun
// okumasi gereken cumleyi ekler.
func namedHostOperationFailure(sentence string, cause error) error {
	sentence = boundedOperatorSentence(sentence)
	if sentence == "" {
		return cause
	}
	return &namedHostOperationError{sentence: sentence, cause: cause}
}

// namedHostOperationSentence recovers that sentence, if the failure carries
// one. It is bounded where it is recorded, like every other durable reason.
// namedHostOperationSentence, varsa o cumleyi geri kazanir.
func namedHostOperationSentence(err error) string {
	var named *namedHostOperationError
	if errors.As(err, &named) {
		return named.sentence
	}
	return ""
}

func standaloneAgentMutationFailure(
	callErr error,
	heartbeatErr error,
) *serviceOperationFailure {
	if callErr == nil && heartbeatErr == nil {
		return nil
	}
	cause := errors.Join(callErr, heartbeatErr)
	if heartbeatErr != nil {
		if errors.Is(heartbeatErr, errAgentMutationIdentityMismatch) ||
			errors.Is(heartbeatErr, errAgentMutationLeaseLost) {
			return operationFailure(
				errCodeServiceOperationLeaseLost,
				"The privileged host operation lost its agent lease.",
				cause,
			)
		}
		if errors.Is(heartbeatErr, errAgentMutationTerminalFailed) {
			return operationFailure(
				errCodeServiceOperationFailed,
				"The privileged host operation did not complete.",
				cause,
			)
		}
		return operationFailure(
			errCodeServiceOperationHeartbeatUncertain,
			"The privileged host operation's agent lease could not be verified.",
			cause,
		)
	}
	// A certificate child that failed on the clock records that reason in
	// its job (review fix, item 8).
	if failure, ok := certificateClockSkewOperationFailure(callErr); ok {
		return failure
	}
	message := "The privileged host operation did not complete."
	if named := namedHostOperationSentence(callErr); named != "" {
		message = named
	}
	return operationFailure(errCodeServiceOperationFailed, message, cause)
}

func (p *Panel) statusAgentMutation(
	ctx context.Context,
	requestID string,
) (*agentMutationJob, error) {
	job, _, err := p.statusAgentMutationWithHold(ctx, requestID)
	return job, err
}

// statusAgentMutationWithHold also reports whether the agent is refusing every
// durable mutation. Only a caller that loops on the status needs it; everything
// else keeps the simpler signature.
// statusAgentMutationWithHold, ayrıca agent'ın her kalıcı mutasyonu reddedip
// reddetmediğini bildirir. Buna yalnız durumu döngüyle yoklayan bir çağıranın
// ihtiyacı vardır; geri kalan her şey daha yalın imzayı korur.
func (p *Panel) statusAgentMutationWithHold(
	ctx context.Context,
	requestID string,
) (*agentMutationJob, string, error) {
	var response agentMutationResponse
	if err := p.callAgentContext(ctx, "Agent.ServiceMutationStatus", &transport.ServiceMutationStatusRequest{
		RequestID: requestID,
	}, &response); err != nil {
		return response.Job, response.MutationHold, err
	}
	if responseErr := serviceMutationResponseError(response); responseErr != nil {
		return response.Job, response.MutationHold, responseErr
	}
	return response.Job, response.MutationHold, nil
}

// errAgentMutationHeld ends a terminal wait that cannot succeed. A held agent
// refuses heartbeat, finish and cancel alike, so the job it reports is frozen:
// polling it until the caller's own deadline only converts a failure the agent
// already knows about into a long silence and then a broken connection.
// errAgentMutationHeld, başarıya ulaşamayacak bir uç bekleyişi sonlandırır.
// Tutulan bir agent kalp atışını, bitirmeyi ve iptali aynı şekilde reddeder;
// dolayısıyla bildirdiği iş donmuştur. Onu çağıranın kendi son tarihine kadar
// yoklamak, agent'ın zaten bildiği bir arızayı yalnızca uzun bir sessizliğe ve
// ardından kopmuş bir bağlantıya çevirir.
var errAgentMutationHeld = errors.New(
	"the agent is refusing durable mutations; this operation cannot complete",
)

// agentMutationHeldError carries the stable hold code so the HTTP layer can
// name it. It stays errors.Is-compatible with errAgentMutationHeld. The code
// travels as a field rather than only inside the message because a message is
// internal text and must never reach a client (httperr.go), while the code is
// the one thing the client is entitled to see: without it, the S-6 operator got
// a fast but anonymous 502 and still could not tell a held agent from any other
// unverified outcome.
// agentMutationHeldError, HTTP katmanının adlandırabilmesi için kararlı tutulma
// kodunu taşır. errAgentMutationHeld ile errors.Is uyumunu korur. Kod yalnız
// mesajın içinde değil bir alan olarak yolculuk eder; çünkü mesaj iç metindir
// ve bir istemciye asla ulaşmamalıdır (httperr.go), kod ise istemcinin görmeye
// hakkı olan tek şeydir: onsuz S-6 operatörü hızlı ama adsız bir 502 aldı ve
// tutulan bir agent'ı başka herhangi bir doğrulanmamış sonuçtan yine ayırt
// edemedi.
type agentMutationHeldError struct {
	Hold string
}

func (e *agentMutationHeldError) Error() string {
	return errAgentMutationHeld.Error() + " (" + e.Hold + ")"
}

func (e *agentMutationHeldError) Is(target error) bool {
	return target == errAgentMutationHeld
}

func (p *Panel) cancelAgentMutation(
	ctx context.Context,
	job *agentMutationJob,
	code, message string,
) error {
	if job == nil || !validServiceOperationID(job.RequestID) ||
		!validServiceOperationID(job.OwnerID) {
		return errors.New("agent mutation has no reusable durable owner identity")
	}
	var response agentMutationResponse
	if err := p.callAgentContext(ctx, "Agent.CancelServiceMutation", &transport.ServiceMutationCancelRequest{
		RequestID: job.RequestID, ExpectedOwner: job.OwnerID,
		Reason: "panel_restart_reconcile", FailureCode: code, FailureMessage: message,
	}, &response); err != nil {
		return err
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	return nil
}

func (p *Panel) finishAgentMutation(
	binding agentMutationBinding,
	success bool,
	failure *serviceOperationFailure,
) (*agentMutationJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), panelMutationFinishTimeout)
	defer cancel()
	return p.finishAgentMutationContext(ctx, binding, success, failure)
}

func (p *Panel) finishAgentMutationContext(
	ctx context.Context,
	binding agentMutationBinding,
	success bool,
	failure *serviceOperationFailure,
) (*agentMutationJob, error) {
	request := transport.ServiceMutationFinishRequest{
		RequestID: binding.MutationRequestID,
		OwnerID:   binding.MutationOwnerID,
		Success:   success,
	}
	if failure != nil {
		request.FailureCode = failure.Code
		request.Message = failure.Message
	}
	var response agentMutationResponse
	if err := p.callAgentContext(
		ctx,
		"Agent.FinishServiceMutation",
		&request,
		&response,
	); err != nil {
		return response.Job, err
	}
	if response.Error != "" {
		return response.Job, errors.New(response.Error)
	}
	if response.Job == nil ||
		response.Job.RequestID != binding.MutationRequestID ||
		response.Job.OwnerID != binding.MutationOwnerID ||
		agentMutationActive(response.Job.Status) {
		return response.Job, errors.New("agent did not durably finish the service mutation")
	}
	return response.Job, nil
}

// The heartbeat's two bounds (alpha.145 lab, D1). Variables only so tests
// can make them short; production never changes them.
var (
	// panelOperationHeartbeatInterval: how often a durable operation's
	// worker renews its Agent lease.
	panelOperationHeartbeatInterval = newTunableDuration(panelMutationHeartbeatInterval)
	// panelHeartbeatUncertainWindow: how long after the last renewal the
	// Agent answered (counted from when that renewal was sent) the worker may
	// go on while renewals stay unanswered (a timeout, a broken connection,
	// an answer that proves nothing). A renewal may wait for the whole time
	// that remains of the window, and an unanswered one is asked again after
	// panelHeartbeatRetryPause; the worker stops at the window's end at the
	// latest: 5 s before the Agent's 20-second lease
	// (serviceMutationLeaseDuration) can end. A definite answer stops it at
	// once.
	panelHeartbeatUncertainWindow = newTunableDuration(15 * time.Second)
	// panelHeartbeatRetryPause: the pause before an unanswered renewal is
	// asked again (bounded by what remains of the window).
	panelHeartbeatRetryPause = newTunableDuration(time.Second)
)

// errAgentMutationRenewalsUnanswered: the Agent answered no renewal for the
// whole window, so the worker stopped before the lease could end unseen.
var errAgentMutationRenewalsUnanswered = errors.New(
	"agent service mutation renewals were not answered before the lease's safety window ended",
)

// agentMutationHeartbeatDefinite reports whether a failed renewal is the
// Agent's definite answer that this worker no longer owns a running lease:
// the lease is lost or held by another owner, or the job ended. Anything
// else (no answer in time, a broken connection, an answer without the job)
// is uncertain, and one uncertain renewal is not a lost Agent (D1).
// Kesin yanıt: kira kaybedildi, başka sahibin ya da iş bitti. Gerisi belirsiz.
func agentMutationHeartbeatDefinite(err error) bool {
	return errors.Is(err, errAgentMutationLeaseLost) ||
		errors.Is(err, errAgentMutationTerminalFailed) ||
		errors.Is(err, errAgentMutationIdentityMismatch)
}

// startAgentMutationHeartbeat renews a durable operation's lease; since is
// when the lease was asked for.
func (p *Panel) startAgentMutationHeartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	binding agentMutationBinding,
	since time.Time,
) func() error {
	stopObserved := p.startObservedAgentMutationHeartbeatSince(
		ctx,
		cancel,
		binding,
		nil,
		panelOperationHeartbeatInterval.get(),
		since,
	)
	return func() error {
		_, err := stopObserved()
		return err
	}
}

// startObservedAgentMutationHeartbeat retains an exact terminal success for a
// standalone mutation. The agent can durably publish success before the
// original mutating RPC response reaches the panel.
//
// One slow or lost renewal does not stop the worker (alpha.145 lab, D1: the
// first failed ping cancelled a webmail update whose Agent step ran on). The
// worker is stopped on the Agent's definite answer, or when no renewal has
// been answered for panelHeartbeatUncertainWindow since the last answered
// one was sent. A renewal may wait for the whole time that remains of the
// window (a slow answer that comes in time keeps the step), an unanswered
// one is asked again promptly, and the first one is scheduled from when the
// lease was asked for; so the stop comes, at the latest, at the window's
// end, before the Agent's 20-second lease can end. The same rule
// governs every standalone mutation (firewall, DNS, mail TLS, certificates,
// VPN, webmail). The Agent's own lease rules are unchanged.
// Tek bir geç ya da kayıp yenileme işçiyi durdurmaz; kesin yanıt ya da son
// yanıtlanan yenilemeden beri süren pencere durdurur, kira bitmeden önce.
func (p *Panel) startObservedAgentMutationHeartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	binding agentMutationBinding,
	expected *agentMutationIdentity,
	interval time.Duration,
) func() (*agentMutationJob, error) {
	return p.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, expected, interval, time.Now())
}

// startObservedAgentMutationHeartbeatSince: since is when the lease was asked
// for (the Agent granted it at or after that moment).
func (p *Panel) startObservedAgentMutationHeartbeatSince(
	ctx context.Context,
	cancel context.CancelFunc,
	binding agentMutationBinding,
	expected *agentMutationIdentity,
	interval time.Duration,
	since time.Time,
) func() (*agentMutationJob, error) {
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	var observedTerminal *agentMutationJob
	var observedErr error
	// stopCtx ends a renewal in flight when the caller stops the heartbeat,
	// so stopping never waits for a slow renewal.
	stopCtx, stopCancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		window := panelHeartbeatUncertainWindow.get()
		retry := panelHeartbeatRetryPause.get()
		// lastAnswered is when the last renewal the Agent answered was SENT
		// (at first: before the lease was asked for). The Agent renewed the
		// lease at or after that moment, so its lease lasts at least until
		// lastAnswered + 20 s; the worker stops by lastAnswered + window.
		lastAnswered := since
		var lastErr error
		stopForWindow := func() {
			observedErr = errors.Join(errAgentMutationRenewalsUnanswered, lastErr)
			// D1 review: the one line that says why and when the worker
			// stopped (no secret: a transport error or the Agent's refusal).
			log.Printf("agent mutation %s: worker stopped: no lease renewal answered for %v (window %v); last renewal error: %v",
				binding.MutationRequestID, time.Since(lastAnswered).Round(time.Millisecond), window, lastErr)
			cancel()
		}
		// The first renewal is scheduled from when the lease was asked for,
		// so a slow lease grant never pushes it past the window.
		first := interval - time.Since(since)
		if first < 0 {
			first = 0
		}
		timer := time.NewTimer(first)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-timer.C:
				if stopCtx.Err() != nil {
					return
				}
				sent := time.Now()
				remaining := window - sent.Sub(lastAnswered)
				if remaining <= 0 {
					stopForWindow()
					return
				}
				// A renewal may wait for the whole time that remains of the
				// window: one slow answer that still comes in time keeps the
				// step (D1 review, finding 1).
				pingCtx, pingCancel := context.WithTimeout(stopCtx, remaining)
				job, err := p.heartbeatAgentMutation(pingCtx, binding, "")
				pingCancel()
				if stopCtx.Err() != nil {
					return
				}
				if expected != nil && job != nil {
					if identityErr := validateAgentMutationIdentity(job, *expected); identityErr != nil {
						observedErr = identityErr
						cancel()
						return
					}
					if job.Status == agentMutationSucceeded {
						if receiptErr := validateAgentMutationSucceededReceipt(job, *expected); receiptErr != nil {
							observedErr = receiptErr
							cancel()
							return
						}
						observedTerminal = cloneAgentMutationJob(job)
						return
					}
				}
				if err != nil {
					if !agentMutationHeartbeatDefinite(err) {
						// Uncertain: asked again promptly, bounded by what
						// remains of the window.
						lastErr = err
						if left := window - time.Since(lastAnswered); left > 0 {
							// Never the window's end itself: one more renewal
							// is still asked inside it.
							next := retry
							if next >= left {
								next = left / 2
							}
							timer.Reset(next)
							continue
						}
						stopForWindow()
						return
					}
					observedErr = err
					cancel()
					return
				}
				lastAnswered, lastErr = sent, nil
				timer.Reset(interval)
			}
		}
	}()
	return func() (*agentMutationJob, error) {
		once.Do(func() {
			stopCancel()
			close(stop)
			// Wait for an in-flight heartbeat before the caller commits a
			// terminal result. Otherwise a late heartbeat failure can race a
			// successful FinishServiceMutation call.
			// Çağıran terminal sonucu kaydetmeden önce devam eden heartbeat'i
			// bekle. Aksi halde geç gelen heartbeat hatası başarılı
			// FinishServiceMutation çağrısıyla yarışabilir.
			<-done
		})
		return cloneAgentMutationJob(observedTerminal), observedErr
	}
}

func (p *Panel) waitExpectedAgentMutationTerminal(
	ctx context.Context,
	identity agentMutationIdentity,
) (*agentMutationJob, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, hold, err := p.statusAgentMutationWithHold(ctx, identity.RequestID)
		if hold != "" {
			// Stop immediately. Nothing about this job can change while the
			// agent is held, and the operator is waiting on the other end of
			// this request.
			// Hemen dur. Agent tutulurken bu işle ilgili hiçbir şey
			// değişemez ve bu isteğin öbür ucunda operatör bekliyor.
			return job, &agentMutationHeldError{Hold: hold}
		}
		if job != nil {
			if identityErr := validateAgentMutationIdentity(job, identity); identityErr != nil {
				return job, identityErr
			}
			if job.Status == agentMutationSucceeded {
				if receiptErr := validateAgentMutationSucceededReceipt(job, identity); receiptErr != nil {
					return job, receiptErr
				}
				return job, nil
			}
			if !agentMutationActive(job.Status) {
				return job, err
			}
		}
		if err != nil {
			return job, err
		}
		if job == nil {
			return nil, errors.New("agent service mutation status did not return the requested durable operation")
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Panel) finishExpectedAgentMutation(
	binding agentMutationBinding,
	identity agentMutationIdentity,
	success bool,
	failure *serviceOperationFailure,
) (*agentMutationJob, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		panelMutationTerminalReconcileTimeout(identity),
	)
	defer cancel()
	return p.finishExpectedAgentMutationWithin(
		ctx, binding, identity, success, failure,
	)
}

func (p *Panel) finishExpectedAgentMutationWithin(
	ctx context.Context,
	binding agentMutationBinding,
	identity agentMutationIdentity,
	success bool,
	failure *serviceOperationFailure,
) (*agentMutationJob, error) {
	finishCtx, cancelFinish := context.WithTimeout(
		ctx, panelMutationFinishTimeout,
	)
	terminal, finishErr := p.finishAgentMutationContext(
		finishCtx, binding, success, failure,
	)
	cancelFinish()
	if terminal != nil {
		if identityErr := validateAgentMutationIdentity(terminal, identity); identityErr != nil {
			return terminal, payloadBoundMutationTerminalError(
				identity, terminal, identityErr,
			)
		}
		if terminal.Status == agentMutationSucceeded {
			if receiptErr := validateAgentMutationSucceededReceipt(terminal, identity); receiptErr != nil {
				return terminal, receiptErr
			}
			return terminal, nil
		}
	}
	if finishErr == nil {
		return terminal, nil
	}
	reconciled, statusErr := waitExpectedAgentMutationTerminalFn(
		p, ctx, identity,
	)
	if reconciled != nil && identity.matches(reconciled) &&
		reconciled.Status == agentMutationSucceeded {
		if receiptErr := validateAgentMutationSucceededReceipt(reconciled, identity); receiptErr != nil {
			return reconciled, receiptErr
		}
		return reconciled, nil
	}
	if statusErr != nil {
		return reconciled, payloadBoundMutationTerminalError(identity, reconciled, fmt.Errorf(
			"finish durable agent mutation: %w",
			errors.Join(
				finishErr,
				fmt.Errorf("reconcile terminal status: %w", statusErr),
			),
		))
	}
	return reconciled, payloadBoundMutationTerminalError(
		identity, reconciled,
		fmt.Errorf("finish durable agent mutation: %w", finishErr),
	)
}

// withStandaloneAgentMutation gives a synchronous privileged RPC the same
// durable host lease used by asynchronous installs. If the panel dies, startup
// recovery sees the unmatched agent job and waits for its active step to stop
// before admitting another mutation. It intentionally does not acquire
// serviceMutationMu: HTTP handlers and composite runners own that process lock
// before entering component-specific locks, preventing inverse lock order.
// withStandaloneAgentMutation, eşzamanlı ayrıcalıklı RPC'ye asenkron
// kurulumlarla aynı kalıcı makine kirasını verir. Panel ölürse başlangıç
// kurtarması eşleşmeyen agent işini görür ve başka değişiklik kabul etmeden
// önce etkin adımın durmasını bekler.
func (p *Panel) withStandaloneAgentMutation(
	ctx context.Context,
	kind, target, packageName string,
	call func(context.Context, agentMutationBinding) error,
) error {
	if binding, err := panelMutationBinding(ctx); err == nil {
		return call(ctx, binding)
	}
	requestID, err := newServiceOperationID()
	if err != nil {
		return fmt.Errorf("create durable mutation request identity: %w", err)
	}
	ownerID, err := newServiceOperationID()
	if err != nil {
		return fmt.Errorf("create durable mutation owner identity: %w", err)
	}
	op := serviceOperation{
		RequestID:   requestID,
		Kind:        strings.TrimSpace(kind),
		ServiceID:   strings.TrimSpace(target),
		PackageName: strings.TrimSpace(packageName),
	}
	return p.withStandaloneAgentMutationIdentity(ctx, op, ownerID, call)
}

// withStandaloneAgentMutationIdentity runs the ordinary standalone lifecycle
// with caller-selected, already persisted identities. It is used by composite
// panel operations whose direct child must be correlated exactly after a
// process restart. The public wrapper above remains the only path that may
// reuse an existing bound context.
func (p *Panel) withStandaloneAgentMutationIdentity(
	ctx context.Context,
	op serviceOperation,
	ownerID string,
	call func(context.Context, agentMutationBinding) error,
) error {
	return p.withStandaloneAgentMutationIdentityMode(
		ctx, op, ownerID, false, call,
	)
}

func (p *Panel) withResumedStandaloneAgentMutationIdentity(
	ctx context.Context,
	op serviceOperation,
	ownerID string,
	call func(context.Context, agentMutationBinding) error,
) error {
	return p.withStandaloneAgentMutationIdentityMode(
		ctx, op, ownerID, true, call,
	)
}

func (p *Panel) withStandaloneAgentMutationIdentityMode(
	ctx context.Context,
	op serviceOperation,
	ownerID string,
	resume bool,
	call func(context.Context, agentMutationBinding) error,
) error {
	if _, err := panelMutationBinding(ctx); err == nil {
		return errors.New("preselected standalone mutation cannot reuse a bound context")
	}
	if !validServiceOperationID(op.RequestID) || !validServiceOperationID(ownerID) {
		return errors.New("invalid preselected standalone mutation identity")
	}
	op.Kind = strings.TrimSpace(op.Kind)
	op.ServiceID = strings.TrimSpace(op.ServiceID)
	op.PackageName = strings.TrimSpace(op.PackageName)
	identity := agentMutationIdentityForOperation(op, ownerID)
	beginCtx, beginCancel := context.WithTimeout(ctx, panelMutationFinishTimeout)
	leaseAskedAt := time.Now()
	job, err := p.beginAgentMutation(beginCtx, op, ownerID, resume)
	beginCancel()
	if err != nil {
		return err
	}
	binding := agentMutationBinding{
		MutationRequestID: op.RequestID,
		MutationOwnerID:   ownerID,
	}
	deadline := job.DeadlineAt
	if deadline.IsZero() {
		deadline = time.Now().Add(45 * time.Minute)
	}
	workerCtx, cancelWorker := context.WithDeadline(ctx, deadline)
	boundCtx := withPanelMutationBinding(workerCtx, binding)
	stopHeartbeat := p.startObservedAgentMutationHeartbeatSince(
		workerCtx,
		cancelWorker,
		binding,
		&identity,
		panelOperationHeartbeatInterval.get(),
		leaseAskedAt,
	)

	callErr := call(boundCtx, binding)
	heartbeatTerminal, heartbeatErr := stopHeartbeat()
	cancelWorker()
	if callErr == nil && heartbeatErr != nil {
		callErr = fmt.Errorf("agent service mutation heartbeat: %w", heartbeatErr)
	}
	failure := standaloneAgentMutationFailure(callErr, heartbeatErr)
	if heartbeatTerminal != nil {
		if receiptErr := validateAgentMutationSucceededReceipt(heartbeatTerminal, identity); receiptErr != nil {
			return receiptErr
		}
		return nil
	}
	heartbeatIdentityMismatch := errors.Is(
		heartbeatErr,
		errAgentMutationIdentityMismatch,
	)
	var terminal *agentMutationJob
	var finishErr error
	if resume {
		terminal, finishErr = p.finishExpectedAgentMutationWithin(
			ctx, binding, identity, callErr == nil, failure,
		)
	} else {
		terminal, finishErr = p.finishExpectedAgentMutation(
			binding, identity, callErr == nil, failure,
		)
	}
	if finishErr != nil {
		return finishErr
	}
	if heartbeatIdentityMismatch {
		return heartbeatErr
	}
	if terminal != nil && terminal.Status == agentMutationSucceeded {
		return nil
	}
	if callErr == nil && (terminal == nil || terminal.Status != agentMutationSucceeded) {
		status := "missing"
		if terminal != nil {
			status = terminal.Status
		}
		return fmt.Errorf("agent committed standalone mutation with status %s", status)
	}
	return callErr
}

func (p *Panel) waitAgentMutationTerminal(
	ctx context.Context,
	requestID string,
) (*agentMutationJob, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := p.statusAgentMutation(ctx, requestID)
		if err != nil {
			return nil, err
		}
		if job == nil || !agentMutationActive(job.Status) {
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-ticker.C:
		}
	}
}
