package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// alpha.114 lab run 2, E-new-4: a customer domain created with mail on the
// joined server, then deleted while the main DNS server is still confirming
// the removal with its backup. The real create and delete handlers run on
// the joined server; the main DNS server is the real receiver handler stack
// behind the network seam, with only its final paired publication replaced.
func TestJoinedServerDeletionFinishesItselfOnceTheMainConfirmsTheRemoval(t *testing.T) {
	f := newRemoteConsumerIntegrationFixture(t)
	ctx := context.Background()
	const domain = "custb.example.test"

	// The main server applies the removal at once but answers "still
	// confirming with the backup" until its own proof ends.
	confirming, mainDown := false, false
	receiverPublications := 0
	remoteDNSPublishReceived = func(p *Panel, _ context.Context, _ string, deleted bool) error {
		if p != f.receiver {
			return errors.New("joined server attempted local DNS publication")
		}
		receiverPublications++
		if deleted && confirming {
			return errors.New("the backup has not confirmed the removal yet")
		}
		return nil
	}
	stack := f.receiver.requireRemoteDNSMachineAuth(csrfProtect(f.receiver.requireAuth(http.HandlerFunc(f.receiver.handleRemoteDNSMachine))))
	remoteDNSExchange = func(ctx context.Context, endpoint, path, secret string, request, response any) error {
		if mainDown {
			return errors.New("dial tcp: connection refused")
		}
		raw, err := json.Marshal(request)
		if err != nil {
			return err
		}
		r := httptest.NewRequest(http.MethodPost, endpoint+path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		stack.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			return &remoteDNSHTTPError{StatusCode: w.Code, Code: remoteDNSRefusalCode(bytes.NewReader(w.Body.Bytes()))}
		}
		return json.Unmarshal(w.Body.Bytes(), response)
	}

	// This server runs mail, so the new domain gets its mail records on the
	// main DNS server (alpha.114 E6), then its DKIM record.
	if _, err := f.origin.db.GetDB().Exec(`INSERT INTO service_operations (id, kind, service_id, status, phase, started_at, created_at, updated_at) VALUES ('m', 'mail_profile_install', 'web_mail', 'succeeded', 'done', 'x', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	if w := f.create(t, domain, "static"); w.Code != http.StatusOK {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var id int
	if err := f.origin.db.GetDB().QueryRow(`SELECT id FROM domains WHERE name=?`, domain).Scan(&id); err != nil {
		t.Fatal(err)
	}
	f.origin.automaticDKIMPublish(ctx, dkimAutomaticItem{record: dkimAutomaticRecord{DomainID: id, Domain: domain, Trigger: dkimTriggerFirstMailbox, State: dkimAutoPending}, pub: "PUBKEY"})
	served := func(kind string) int {
		var n int
		if err := f.receiver.db.GetDB().QueryRow(`SELECT count(*) FROM pdns_records r JOIN pdns_domains d ON d.id=r.domain_id WHERE d.name=? AND r.type=?`, domain, kind).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if served("MX") != 1 || served("TXT") < 3 {
		t.Fatalf("mail records on the main DNS server: MX=%d TXT=%d", served("MX"), served("TXT"))
	}

	// Delete on the joined server while the main server confirms.
	agent := &domainDeletionRPCAgent{}
	attachDomainDeletionRPCAgent(t, f.origin, agent)
	confirming = true
	w := deleteDomainForSagaTest(t, f.origin, id)
	requireDeletionPendingStage(t, w, "dns_cleanup")
	var answer map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil || answer["reason"] != remoteDNSDeletionConfirmingReason {
		t.Fatalf("pending answer does not say the main server is confirming: %s", w.Body.String())
	}
	if served("MX")+served("TXT")+served("A") != 0 {
		t.Fatal("the main DNS server still serves the removed zone")
	}
	if code, body := readDeletionStatusForTest(t, f.origin, id); code != http.StatusOK ||
		body.Status != domainDeletionPendingStatus || body.Stage != "dns_cleanup" || body.Reason != remoteDNSDeletionConfirmingReason {
		t.Fatalf("status while the main server confirms: %d %+v", code, body)
	}

	// A deletion stuck before this fix has no DNS stage record and no
	// in-memory history: the saved removal itself is the evidence.
	f.origin.clearRemoteDNSDeletionStage(ctx, id)
	f.origin.remoteDNSDeletions().forget(id)
	if _, body := readDeletionStatusForTest(t, f.origin, id); body.Stage != "dns_cleanup" || body.Reason != remoteDNSDeletionConfirmingReason {
		t.Fatalf("a deletion stuck before this release reads %+v", body)
	}

	// The main server is unreachable: typed, and asked again by itself.
	mainDown = true
	f.origin.reconcileRemoteDNSDeletionsOnce(ctx)
	if _, body := readDeletionStatusForTest(t, f.origin, id); body.Reason != remoteDNSDeletionUnreachableReason || body.Status != domainDeletionPendingStatus {
		t.Fatalf("unreachable main server reads %+v", body)
	}
	// Not due yet: a scan sends nothing (no duplicate from polling).
	before := receiverPublications
	mainDown = false
	f.origin.reconcileRemoteDNSDeletionsOnce(ctx)
	if receiverPublications != before {
		t.Fatal("a scan re-sent before its pause ended")
	}

	// The main server's proof ended; the next due attempt gets the receipt
	// and the joined server finishes the deletion by itself.
	confirming = false
	f.origin.remoteDNSDeletions().forget(id)
	f.origin.reconcileRemoteDNSDeletionsOnce(ctx)
	if code, _ := readDeletionStatusForTest(t, f.origin, id); code != http.StatusNoContent {
		t.Fatalf("deletion did not finish by itself: status %d", code)
	}
	var left int
	if err := f.origin.db.GetDB().QueryRow(`SELECT count(*) FROM domains WHERE id=?`, id).Scan(&left); err != nil || left != 0 {
		t.Fatalf("domain row left: %d %v", left, err)
	}
	var generation, applied int64
	var deleted bool
	if err := f.receiver.db.GetDB().QueryRow(`SELECT generation,applied_generation,deleted FROM remote_dns_zone_ownership WHERE zone_name=?`, domain).Scan(&generation, &applied, &deleted); err != nil || !deleted || generation != applied {
		t.Fatalf("main server receipt: %d/%d deleted=%v %v", generation, applied, deleted, err)
	}
	agent.callsMu.Lock()
	sites := agent.deleteSiteCalls
	agent.callsMu.Unlock()
	if sites != 1 {
		t.Fatalf("the site was removed %d times", sites)
	}
	var audited int
	if err := f.origin.db.GetDB().QueryRow(`SELECT count(*) FROM audit_logs WHERE action=?`, "domain.delete:"+domain).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("audit rows for the finished deletion: %d %v", audited, err)
	}
	var stage int
	if err := f.origin.db.GetDB().QueryRow(`SELECT count(*) FROM panel_settings WHERE key=?`, remoteDNSDeletionStageKey(id)).Scan(&stage); err != nil || stage != 0 {
		t.Fatalf("stage record left: %d %v", stage, err)
	}
	// Nothing more happens for the finished deletion.
	before = receiverPublications
	f.origin.reconcileRemoteDNSDeletionsOnce(ctx)
	if receiverPublications != before {
		t.Fatal("a finished deletion was published again")
	}
}

// A main server refusing the removal is a verified refusal: a person acts.
func TestJoinedServerDeletionReasonsAreTyped(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{&remoteDNSPublicationPendingError{cause: &remoteDNSHTTPError{StatusCode: 409, Code: "REMOTE_DNS_PUBLICATION_PENDING"}}, remoteDNSDeletionConfirmingReason},
		{&remoteDNSPublicationPendingError{cause: &remoteDNSHTTPError{StatusCode: 409, Code: "REMOTE_DNS_PUBLICATION_CONFLICT"}}, remoteDNSDeletionRefusedReason},
		{&remoteDNSPublicationPendingError{cause: &remoteDNSHTTPError{StatusCode: 401}}, remoteDNSDeletionRefusedReason},
		{&remoteDNSPublicationPendingError{cause: &remoteDNSHTTPError{StatusCode: 503, Code: "REMOTE_DNS_BUSY"}}, remoteDNSDeletionUnreachableReason},
		{&remoteDNSPublicationPendingError{cause: errors.New("timeout")}, remoteDNSDeletionUnreachableReason},
		{errRemoteDNSNotReady, remoteDNSDeletionUnreachableReason},
		// Lab run 6: the main server got the removal and answered after the
		// timeout while its backup check ran; it is running, so this waits.
		{&remoteDNSPublicationPendingError{cause: &remoteDNSExchangeError{Delivered: true}}, remoteDNSDeletionConfirmingReason},
		{&remoteDNSPublicationPendingError{cause: &remoteDNSExchangeError{}}, remoteDNSDeletionUnreachableReason},
		// Lab run 6: the main panel stopped; the bare exchange failure has a
		// reason too (the 202 answer gave none).
		{&remoteDNSExchangeError{}, remoteDNSDeletionUnreachableReason},
		{fmt.Errorf("publish removal: %w", &remoteDNSExchangeError{}), remoteDNSDeletionUnreachableReason},
	} {
		if got, ok := remoteDNSDeletionReason(tc.err); !ok || got != tc.reason {
			t.Errorf("%v: reason %q, want %q", tc.err, got, tc.reason)
		}
		var exchange *remoteDNSExchangeError
		if errors.As(tc.err, &exchange) && !strings.HasSuffix(tc.err.Error(), "remote DNS HTTPS exchange failed") && tc.err.Error() != errRemoteDNSPublicationPending.Error() {
			t.Errorf("transport detail reached the text: %q", tc.err.Error())
		}
		if !errors.As(tc.err, &exchange) && tc.err.Error() != errRemoteDNSPublicationPending.Error() && !errors.Is(tc.err, errRemoteDNSNotReady) {
			t.Errorf("transport detail reached the text: %q", tc.err.Error())
		}
	}
	if _, ok := remoteDNSDeletionReason(fmt.Errorf("site cleanup failed")); ok {
		t.Error("an unrelated error was classified as a DNS answer")
	}
}
