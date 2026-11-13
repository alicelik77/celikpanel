package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestDNSPeerPendingGuidanceOnlyRecognizesReviewedCodes(t *testing.T) {
	for _, code := range []string{
		transport.DNSPeerPendingEnrollmentRequired,
		transport.DNSPeerPendingEnrollmentChanged,
		transport.DNSPeerPendingInspectionUnknown,
		transport.DNSPeerPendingNativeUnknown,
		transport.DNSPeerPendingJournalUnknown,
		transport.DNSPeerPendingOwnerEditUnknown,
		transport.DNSPeerPendingProofInternal,
		transport.DNSPeerPendingProofTimeout,
		transport.DNSPeerPendingJournalUnresolved,
	} {
		body, ok := dnsPeerPendingAPIError(errors.Join(
			errors.New("untrusted remote output"), &dnsZoneV3PropagationPendingError{Code: code, Exact: true},
		))
		if !ok || body.Code != errCodeDNSPublicationFailed || body.Reason != code ||
			!strings.Contains(body.Error, "same publication") ||
			strings.Contains(body.Error, "untrusted remote output") {
			t.Fatalf("reviewed code %q body=%+v ok=%v", code, body, ok)
		}
	}
	for _, pending := range []*dnsZoneV3PropagationPendingError{
		{}, {Code: "untrusted remote output", Exact: true},
		{Code: transport.DNSPeerPendingNativeUnknown},
	} {
		if body, ok := dnsPeerPendingAPIError(pending); ok || body.Reason != "" {
			t.Fatalf("unreviewed code leaked: body=%+v ok=%v", body, ok)
		}
	}
}

func TestDNSZoneV3PendingGuidanceRequiresExactPendingJob(t *testing.T) {
	code := transport.DNSPeerPendingOwnerEditUnknown
	for _, tc := range []struct {
		job  *agentMutationJob
		want string
	}{
		{nil, ""},
		{&agentMutationJob{Status: agentMutationPending, ErrorCode: "dns_zone_v3_propagation_pending"}, ""},
		{&agentMutationJob{Status: agentMutationSucceeded, ErrorCode: code}, ""},
		{&agentMutationJob{Status: agentMutationPending, ErrorCode: code}, code},
	} {
		if got := dnsZoneV3PendingCodeFromJob(tc.job); got != tc.want {
			t.Fatalf("job=%+v code=%q want=%q", tc.job, got, tc.want)
		}
	}
}

func TestDNSZoneV3PendingRPCCodeIsBounded(t *testing.T) {
	panel := newDNSPanelForTest(t)
	agent := newDNSZoneV3TestAgent()
	attachDNSZoneV3TestAgent(t, panel, agent)
	request := transport.SyncDNSZoneV3Request{
		Engine: transport.DNSEngineBIND, EngineEpoch: 1,
		DesiredGeneration: 3, Domain: "pending-code.example",
		ZoneType: "MASTER",
	}
	code := transport.DNSPeerPendingInspectionUnknown
	agent.syncResponseHook = func(request transport.SyncDNSZoneV3Request,
		response *transport.SyncDNSZoneV3Response) error {
		*response = transport.SyncDNSZoneV3Response{
			RecoveryPending: true, PendingCode: code,
			Engine: request.Engine, EngineEpoch: request.EngineEpoch,
			AppliedGeneration: request.DesiredGeneration,
		}
		return nil
	}
	var response transport.SyncDNSZoneV3Response
	err := panel.callSyncDNSZoneV3(context.Background(), &request, &response)
	var pending *dnsZoneV3PropagationPendingError
	if !errors.As(err, &pending) || pending.Code != code {
		t.Fatalf("exact pending code=%+v err=%v", response, err)
	}
	code = "private peer output"
	response = transport.SyncDNSZoneV3Response{}
	if err := panel.callSyncDNSZoneV3(context.Background(), &request, &response); err == nil ||
		!strings.Contains(err.Error(), "invalid pending") {
		t.Fatalf("unreviewed pending code accepted: response=%+v err=%v", response, err)
	}
	code = ""
	response = transport.SyncDNSZoneV3Response{}
	err = panel.callSyncDNSZoneV3(context.Background(), &request, &response)
	if !errors.As(err, &pending) || pending.Code != "" {
		t.Fatalf("legacy pending code misclassified: response=%+v err=%v", response, err)
	}
}

func TestDNSZoneV3DeferredErrorRetainsExactPendingCode(t *testing.T) {
	code := transport.DNSPeerPendingNativeUnknown
	err := &dnsAgentPublicationError{Err: errors.Join(
		errDNSZoneV3PropagationDeferred,
		&dnsZoneV3PropagationPendingError{Code: code, Exact: true},
		errors.New("private peer output"),
	)}
	body, ok := dnsPeerPendingAPIError(err)
	if !ok || body.Reason != code ||
		strings.Contains(body.Error, "private peer output") {
		t.Fatalf("deferred pending code lost or leaked: body=%+v ok=%v", body, ok)
	}
}

func TestDomainDeletionPendingReportsReviewedExactReason(t *testing.T) {
	panel := newDNSPanelForTest(t)
	for _, tc := range []struct {
		name       string
		stage      string
		cause      error
		wantReason string
	}{
		{"reviewed", "dns_cleanup", &dnsZoneV3PropagationPendingError{Code: transport.DNSPeerPendingNativeUnknown, Exact: true}, transport.DNSPeerPendingNativeUnknown},
		{"unreconciled", "dns_cleanup", &dnsZoneV3PropagationPendingError{Code: transport.DNSPeerPendingNativeUnknown}, ""},
		{"generic", "dns_cleanup", errors.New("private peer output"), ""},
		{"other stage", "site_cleanup", &dnsZoneV3PropagationPendingError{Code: transport.DNSPeerPendingNativeUnknown}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/3", nil)
			panel.writeDomainDeletionPending(w, r, 3, "gone.example", tc.stage, tc.cause)
			if w.Code != http.StatusAccepted {
				t.Fatalf("status=%d", w.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != domainDeletionPendingStatus || body["reason"] != tc.wantReason ||
				strings.Contains(body["message"], "private peer output") {
				t.Fatalf("pending body=%+v", body)
			}
		})
	}
}

func TestDNSPeerPendingGuidanceSplitsReviewedInspectorDetail(t *testing.T) {
	reason, detail, message, ok := dnsPeerPendingGuidance("dns_peer_inspection_unknown:named_unavailable")
	if !ok || reason != transport.DNSPeerPendingInspectionUnknown || detail != "named_unavailable" ||
		!strings.HasPrefix(message, dnsPeerPendingEnglish(reason)+" The secondary's inspector reported") {
		t.Fatalf("composite code: %q %q %q %t", reason, detail, message, ok)
	}
	reason, detail, message, ok = dnsPeerPendingGuidance(transport.DNSPeerPendingCatalogTransferRefused)
	if !ok || reason != transport.DNSPeerPendingCatalogTransferRefused || detail != "" ||
		!strings.Contains(message, "allow-transfer") || !strings.Contains(message, "127.0.0.1 and ::1") ||
		!strings.Contains(message, "checks this same publication again by itself") {
		t.Fatalf("typed refusal: %q %q %q %t", reason, detail, message, ok)
	}
	for _, code := range []string{"dns_peer_inspection_unknown:raw", "dns_peer_native_unknown:named_unavailable", "nope"} {
		if _, _, _, ok := dnsPeerPendingGuidance(code); ok {
			t.Fatalf("unreviewed code %q accepted", code)
		}
	}
	body, ok := dnsPeerPendingAPIError(&dnsZoneV3PropagationPendingError{Code: "dns_peer_inspection_unknown:catalog_malformed", Exact: true})
	if !ok || body.Reason != transport.DNSPeerPendingInspectionUnknown || body.Detail != "catalog_malformed" {
		t.Fatalf("API body lost the reviewed detail: %+v", body)
	}
}

// The PowerDNS inspector's unreviewed-configuration token has its own
// sentence naming both reviewed shapes; the reason text still comes first.
func TestDNSPeerPendingGuidanceNamesPowerDNSConfigUnreviewed(t *testing.T) {
	reason, detail, message, ok := dnsPeerPendingGuidance("dns_peer_inspection_unknown:config_unreviewed")
	want := dnsPeerPendingEnglish(transport.DNSPeerPendingInspectionUnknown) +
		" The secondary's inspector reported that PowerDNS is not running with a configuration it recognises (the panel's own or the documented panel-free one)."
	if !ok || reason != transport.DNSPeerPendingInspectionUnknown || detail != "config_unreviewed" || message != want {
		t.Fatalf("config_unreviewed: %q %q %q %t", reason, detail, message, ok)
	}
	body, ok := dnsPeerPendingAPIError(&dnsZoneV3PropagationPendingError{Code: "dns_peer_inspection_unknown:config_unreviewed", Exact: true})
	if !ok || body.Reason != transport.DNSPeerPendingInspectionUnknown || body.Detail != "config_unreviewed" || body.Error != want {
		t.Fatalf("API body lost the reviewed detail: %+v", body)
	}
}

// pair5 P5-1: the Agent could not run its own proof. The text must not claim
// an owner change; it names the owner, the log command, that a retry waits
// for a fixed Agent, and that the deletion stays pending.
func TestDNSPeerProofInternalGuidanceIsNotAnOwnerEdit(t *testing.T) {
	reason, detail, message, ok := dnsPeerPendingGuidance(transport.DNSPeerPendingProofInternal)
	if !ok || reason != transport.DNSPeerPendingProofInternal || detail != "" {
		t.Fatalf("internal code: %q %q %t", reason, detail, ok)
	}
	for _, want := range []string{
		"could not run its own check of the secondary",
		"No change by either server's owner was found",
		"server owner",
		"sudo journalctl -u celikpanel-agent | grep peer",
		"Retrying does not help until the CelikPanel Agent is updated",
		"“Retry this deletion”",
		"same publication",
		"deletion stays pending and DNS answers are unaffected",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("internal guidance lacks %q: %s", want, message)
		}
	}
	if strings.Contains(message, "evidence changed") || strings.Contains(message, "reconcile") {
		t.Fatalf("internal guidance claims an owner change: %s", message)
	}
	_, _, ownerEdit, _ := dnsPeerPendingGuidance(transport.DNSPeerPendingOwnerEditUnknown)
	if !strings.Contains(ownerEdit, "compare the catalog zone and zone serials on both servers") ||
		!strings.Contains(ownerEdit, "evidence changed during verification") {
		t.Fatalf("owner-edit guidance lost its meaning or action: %s", ownerEdit)
	}
}

// dns_peer_owner_edit_unknown carries the check that differed as a reviewed
// detail: the reason and its "same publication" retry text are unchanged,
// and every check has its own sentence.
func TestDNSPeerPendingGuidanceSplitsOwnerEditCheck(t *testing.T) {
	base := dnsPeerPendingEnglish(transport.DNSPeerPendingOwnerEditUnknown)
	for _, check := range []string{
		transport.DNSPeerOwnerEditCheckOperationAttempt, transport.DNSPeerOwnerEditCheckEngineState,
		transport.DNSPeerOwnerEditCheckActiveEngine, transport.DNSPeerOwnerEditCheckNativeBinding,
		transport.DNSPeerOwnerEditCheckDeletionReceipt, transport.DNSPeerOwnerEditCheckProducerCatalog,
		transport.DNSPeerOwnerEditCheckCatalogProbe, transport.DNSPeerOwnerEditCheckAuthority,
		transport.DNSPeerOwnerEditCheckTransferObserved, transport.DNSPeerOwnerEditCheckZoneAnswered,
	} {
		sentence := dnsPeerOwnerEditDetailEnglish(check)
		code := transport.DNSPeerPendingOwnerEditUnknownWithDetail(check)
		reason, detail, message, ok := dnsPeerPendingGuidance(code)
		if !ok || reason != transport.DNSPeerPendingOwnerEditUnknown || detail != check ||
			!strings.HasPrefix(sentence, "What differed: ") || message != base+" "+sentence ||
			!strings.Contains(message, "same publication") {
			t.Fatalf("%s: %q %q %q %t", check, reason, detail, message, ok)
		}
		body, ok := dnsPeerPendingAPIError(&dnsZoneV3PropagationPendingError{Code: code, Exact: true})
		if !ok || body.Reason != transport.DNSPeerPendingOwnerEditUnknown || body.Detail != check || body.Error != message {
			t.Fatalf("%s: API body %+v", check, body)
		}
		if got := dnsZoneV3PendingCodeFromJob(&agentMutationJob{Status: agentMutationPending, ErrorCode: code}); got != code {
			t.Fatalf("%s: saved job code %q", check, got)
		}
	}
	// An inspector reason never rides on the owner-edit code, nor the reverse.
	for _, code := range []string{
		"dns_peer_owner_edit_unknown:named_unavailable",
		"dns_peer_inspection_unknown:producer_catalog",
	} {
		if _, _, _, ok := dnsPeerPendingGuidance(code); ok {
			t.Fatalf("cross-reason detail accepted: %q", code)
		}
	}
	if sentence := dnsPeerPendingDetailEnglish(transport.DNSPeerPendingNativeUnknown, "producer_catalog"); sentence != "" {
		t.Fatalf("detail sentence for a reason without details: %q", sentence)
	}
}

// pair6 P6-1, corrected in alpha.133 (incident of 12 Nov 2026): a step of
// the proof ran out of its time before the peer's answer was accepted. The
// zone is already removed on this server, so the text never says nothing was
// changed; it says CelikPanel checks again by itself and when the
// administrator is told, and the detail names the side that ran out of time.
// A customer-visible text names no port or command.
func TestDNSPeerProofTimeoutGuidanceNamesRetryAndChecks(t *testing.T) {
	for code, side := range map[string]string{
		transport.DNSPeerPendingProofTimeout:                                                      "",
		transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck):   "this server's own part of the check that ran out of time, not the backup DNS server",
		transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange): "backup DNS server's answer that did not come in time",
	} {
		reason, detail, message, ok := dnsPeerPendingGuidance(code)
		if !ok || reason != transport.DNSPeerPendingProofTimeout || (side == "") != (detail == "") {
			t.Fatalf("%s: %q %q %t", code, reason, detail, ok)
		}
		for _, want := range []string{
			"The domain is removed from DNS on this server",
			"not confirmed yet, because the check did not finish in time",
			"Nobody needs to do anything now",
			"re-checks this same publication by itself",
			"finishes the deletion by itself",
			"the server's administrator gets a notification",
			side,
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("%s guidance lacks %q: %s", code, want, message)
			}
		}
		for _, not := range []string{"Nothing was changed", "DNS answers are unaffected", "SSH", "port",
			"overloaded", "journalctl", "evidence changed", "Retrying does not help"} {
			if strings.Contains(message, not) {
				t.Fatalf("%s guidance says %q: %s", code, not, message)
			}
		}
	}
	// alpha.133 (lab of 12 Nov 2026): the Agent now finishes an interrupted
	// change of its check record by itself, so a re-check can finish this:
	// why, nobody acts, how it continues; no jargon, no command, no button
	// that is not on the screen.
	_, _, journal, _ := dnsPeerPendingGuidance(transport.DNSPeerPendingJournalUnknown)
	for _, want := range []string{
		"This domain is removed from DNS on this server",
		"because this server could not complete its last check",
		"Nobody needs to do anything",
		"re-checks this same publication by itself, at most every 5 minutes",
	} {
		if !strings.Contains(journal, want) {
			t.Fatalf("journal guidance lacks %q: %s", want, journal)
		}
	}
	for _, not := range []string{"challenge", "journal", "nonce", "proof", "journalctl", "Retry this deletion"} {
		if strings.Contains(journal, not) {
			t.Fatalf("journal guidance says %q: %s", not, journal)
		}
	}
	if !dnsPeerPendingRetriesAutomatically(transport.DNSPeerPendingJournalUnknown) {
		t.Fatal("journal_unknown is no longer re-checked automatically")
	}
}

// alpha.133: a check record the Agent cannot prove. Who acts (the server
// administrator) and the one next step first; it is not re-checked by
// itself and the text never says nobody needs to act.
func TestDNSPeerJournalUnresolvedGuidanceNamesTheAdministrator(t *testing.T) {
	reason, detail, message, ok := dnsPeerPendingGuidance(transport.DNSPeerPendingJournalUnresolved)
	if !ok || reason != transport.DNSPeerPendingJournalUnresolved || detail != "" {
		t.Fatalf("unresolved: %q %q %t", reason, detail, ok)
	}
	for _, want := range []string{
		"cannot sort out by itself", "Who acts: this server's administrator", "if that is not you, tell them",
		"no longer checks this every few minutes", "same publication",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("unresolved guidance lacks %q: %s", want, message)
		}
	}
	if strings.Contains(message, "sudo") || strings.Contains(message, "/var/lib") {
		t.Fatalf("a command or private path outside the administrator's Details: %s", message)
	}
	if strings.Contains(message, "Nobody needs") || dnsPeerPendingRetriesAutomatically(transport.DNSPeerPendingJournalUnresolved) {
		t.Fatalf("unresolved promises automatic progress: %s", message)
	}
	if !strings.Contains(dnsPeerPendingEnglish(transport.DNSPeerPendingJournalUnknown), "Nobody needs") {
		t.Fatal("the self-repairing reason lost its waiting sentence")
	}
}
