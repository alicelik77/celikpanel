package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// The saved mail host name can be corrected from the mail profile dialog. A
// request refused because another operation holds the server leaves the saved
// name unchanged; the admitted request saves it and the run uses it.
// Kayıtlı posta adı düzeltilebilir; meşgul reddi adı değiştirmez.
func TestMailHostnameCorrectionIsSavedOnlyWhenAdmitted(t *testing.T) {
	fixture, _ := newMailProfileTestFixture(t)
	ctx := context.Background()
	if err := fixture.panel.setSetting(ctx, settingMailHostname, "mail.old.test"); err != nil {
		t.Fatal(err)
	}

	fixture.panel.serviceMutationMu.Lock()
	recorder, queued := postMailProfileWithHostname(t, fixture, core.MailProfileCore, mustServiceOperationRequestID(t), "mail.new.test")
	fixture.panel.serviceMutationMu.Unlock()
	if recorder.Code != http.StatusConflict || queued != nil {
		t.Fatalf("busy request: %d %s", recorder.Code, recorder.Body.String())
	}
	var refusal struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &refusal); err != nil || refusal.Code == "" {
		t.Fatalf("busy refusal is not typed: %s", recorder.Body.String())
	}
	if saved := fixture.panel.setting(ctx, settingMailHostname); saved != "mail.old.test" {
		t.Fatalf("a refused request changed the saved name to %q", saved)
	}

	// Control: the same request, admitted, saves the corrected name.
	recorder, queued = postMailProfileWithHostname(t, fixture, core.MailProfileCore, mustServiceOperationRequestID(t), "Mail.New.Test.")
	if recorder.Code != http.StatusAccepted || queued == nil {
		t.Fatalf("admitted request: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForServiceOperation(t, fixture.panel, fixture.userID, queued.ID, serviceOperationSucceeded)
	if saved := fixture.panel.setting(ctx, settingMailHostname); saved != "mail.new.test" {
		t.Fatalf("corrected name not saved: %q", saved)
	}
}

// On a secondary DNS server the mail identity guidance names where the record
// is created (the primary, or the DNS provider) and the exact record, instead
// of sending the owner to a DNS screen that cannot create one here.
// İkincil DNS sunucusunda kaydın nerede ve tam olarak nasıl oluşturulacağı.
func TestMailProfileDNSGuidanceOnASecondaryNamesThePrimaryAndTheRecord(t *testing.T) {
	f := newSecondaryHostingFixture(t)
	f.wait(t)
	ctx := context.Background()
	panel := f.f.panel
	if mode, err := panel.setupDNSManagementMode(ctx); err != nil || mode != setupDNSModeLocal {
		t.Fatalf("fixture mode: %q %v", mode, err)
	}
	for key, value := range map[string]string{settingDNSPeerNS: "ns1.example.test", settingDNSPeerIP: "192.0.2.1", settingMailHostname: "mail.boston.example.test"} {
		if err := panel.setSetting(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	guidance := panel.mailProfileDNSGuidanceFor(ctx, false)
	if guidance == nil || guidance.Reason != mailProfileDNSGuidanceSecondary || guidance.PrimaryNameserver != "ns1.example.test" || guidance.PrimaryIP != "192.0.2.1" || guidance.RecordName != "mail.boston.example.test" || guidance.RecordType != "A" {
		t.Fatalf("secondary guidance: %+v", guidance)
	}
	// The value is this server's address as reviewed in setup (LocalIP).
	if guidance.RecordValue != "192.0.2.2" {
		t.Fatalf("record value: %+v", guidance)
	}
	// No authorized DNS connection to the primary: the record is added by
	// hand. With one to the reviewed primary endpoint, setup asks for it
	// (2026-10-16, decision 3).
	if !guidance.PeerRecordChannel {
		t.Fatalf("the fixture's connection to the primary not seen: %+v", guidance)
	}
	if _, err := panel.db.GetDB().Exec(`DELETE FROM remote_dns_connections`); err != nil {
		t.Fatal(err)
	}
	if guidance = panel.mailProfileDNSGuidanceFor(ctx, false); guidance == nil || guidance.PeerRecordChannel {
		t.Fatalf("channel without a connection: %+v", guidance)
	}
	if _, err := panel.db.GetDB().Exec(`INSERT INTO remote_dns_connections(id,endpoint,label,status,nameservers_json,created_at,credential,enrollment_code) VALUES(?,?,?,?,?,?,?,?)`,
		strings.Repeat("e", 32), f.plan.Draft.DNSPublisherEndpoint, "primary", "ready", "[]", "2026-10-16T00:00:00Z", strings.Repeat("f", 64), ""); err != nil {
		t.Fatal(err)
	}
	if guidance = panel.mailProfileDNSGuidanceFor(ctx, false); guidance == nil || !guidance.PeerRecordChannel {
		t.Fatalf("channel not seen: %+v", guidance)
	}
	// Controls: a ready identity has no guidance, and neither has a server
	// whose hosting records are made elsewhere (external mode).
	if got := panel.mailProfileDNSGuidanceFor(ctx, true); got != nil {
		t.Fatalf("ready identity: %+v", got)
	}
	if err := panel.saveSetupDNSManagementMode(ctx, setupDNSModeExternal); err != nil {
		t.Fatal(err)
	}
	if got := panel.mailProfileDNSGuidanceFor(ctx, false); got != nil {
		t.Fatalf("external mode: %+v", got)
	}
}

func TestMailProfileDNSGuidanceIsAbsentOnAStandaloneServer(t *testing.T) {
	fixture, _ := newMailProfileTestFixture(t)
	if got := fixture.panel.mailProfileDNSGuidanceFor(context.Background(), false); got != nil {
		t.Fatalf("standalone server: %+v", got)
	}
}
