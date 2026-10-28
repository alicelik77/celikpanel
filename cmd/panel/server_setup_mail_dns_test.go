package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Tests never ask the internet: the host's own IPv6 address and the public
// PTR resolvers are replaced for the whole test binary.
func init() {
	setupMailIPv6 = func() string { return "" }
	setupPTRResolvers = func() []setupProviderRecordLookup { return nil }
}

func mailDraftForTest() serverSetupDraft {
	return serverSetupDraft{Purpose: "custom", Customization: &serverSetupCustomization{Components: []string{"postfix"}}, MainDomain: "example.test",
		PanelDomain: "panel.example.test", MailHostname: "mail.example.test", DNSMode: "local", DNSRole: "primary", DNSEngine: "bind",
		NS1: "ns1.example.test", NS2: "ns2.example.test", PeerNS: "ns2.example.test", LocalIP: "203.0.113.10", PeerIP: "198.51.100.20",
		InfrastructureDNS: &serverSetupInfrastructureDNSDraft{Zone: "example.test"}}
}

func desiredByKey(t *testing.T, d serverSetupDraft) map[string]serverSetupInfrastructureDNSRecord {
	t.Helper()
	records, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]serverSetupInfrastructureDNSRecord{}
	for _, record := range records {
		out[record.Name+" "+record.Type+" "+joinTXTSegments(record.Content)] = record
	}
	return out
}

func TestServerSetupDraftImportedRecordsCanonical(t *testing.T) {
	d := mailDraftForTest()
	d.ImportedRecords = []serverSetupImportedRecord{
		{Name: "Example.TEST.", Type: "a", Content: "192.0.2.80", Decision: "import"},
		{Name: "example.test", Type: "A", Content: "192.0.2.80", Decision: "import"},
		{Name: "_dmarc.example.test", Type: "TXT", Content: "v=DMARC1; p=none", Decision: "skip"},
	}
	got, err := canonicalServerSetupDraft(d)
	if err != nil || len(got.ImportedRecords) != 2 || got.ImportedRecords[1].Name != "example.test" || got.ImportedRecords[1].Type != "A" {
		t.Fatalf("canonical imports: %+v %v", got.ImportedRecords, err)
	}
	for _, bad := range []serverSetupImportedRecord{
		{Name: "other.test", Type: "A", Content: "192.0.2.1", Decision: "import"},
		{Name: "example.test", Type: "CNAME", Content: "x.example.test", Decision: "import"},
		{Name: "example.test", Type: "NS", Content: "ns.old.test", Decision: "import"},
		{Name: "www.example.test", Type: "A", Content: "not-an-ip", Decision: "import"},
		{Name: "www.example.test", Type: "A", Content: "192.0.2.1", Decision: ""},
		{Name: "www.example.test", Type: "TXT", Content: "a\nb", Decision: "import"},
	} {
		d.ImportedRecords = []serverSetupImportedRecord{bad}
		if _, err := canonicalServerSetupDraft(d); err == nil {
			t.Fatalf("refused import accepted: %+v", bad)
		}
	}
	d.ImportedRecords = nil
	d.MailDNS = &serverSetupMailDNS{MX: "soon"}
	if _, err := canonicalServerSetupDraft(d); err == nil {
		t.Fatal("invalid MX choice accepted")
	}
}

func TestServerSetupBuildMailRecordsFollowTheOwnersChoices(t *testing.T) {
	d := mailDraftForTest()
	records := desiredByKey(t, d)
	for _, key := range []string{"example.test MX mail.example.test", "example.test TXT v=spf1 mx a:mail.example.test ~all", "_dmarc.example.test TXT v=DMARC1; p=none; rua=mailto:dmarc@example.test"} {
		if record, ok := records[key]; !ok || record.Origin != "mail" {
			t.Fatalf("missing mail record %q: %v", key, records)
		}
	}
	if records["example.test MX mail.example.test"].Priority != 10 {
		t.Fatal("MX preference lost")
	}
	// An old mail server stays the MX until the owner says now; SPF is merged.
	d.ImportedRecords = []serverSetupImportedRecord{
		{Name: "example.test", Type: "MX", Content: "mx.old.test", Priority: 5, Decision: "import"},
		{Name: "example.test", Type: "TXT", Content: "v=spf1 include:_spf.old.test ~all", Decision: "import"},
		{Name: "example.test", Type: "TXT", Content: "google-site-verification=abc", Decision: "import"},
		{Name: "_dmarc.example.test", Type: "TXT", Content: "v=DMARC1; p=reject", Decision: "import"},
		{Name: "www.example.test", Type: "CNAME", Content: "example.test", Decision: "import"},
		{Name: "old.example.test", Type: "A", Content: "192.0.2.9", Decision: "skip"},
	}
	records = desiredByKey(t, d)
	if _, ok := records["example.test MX mail.example.test"]; ok {
		t.Fatal("MX moved without the owner")
	}
	if records["example.test MX mx.old.test"].Origin != "imported" {
		t.Fatal("old MX not kept")
	}
	if _, ok := records["example.test TXT v=spf1 include:_spf.old.test a:mail.example.test ~all"]; !ok {
		t.Fatalf("SPF not merged: %v", records)
	}
	if _, ok := records["example.test TXT v=spf1 include:_spf.old.test ~all"]; ok {
		t.Fatal("two SPF policies would be published")
	}
	for _, key := range []string{"example.test TXT google-site-verification=abc", "_dmarc.example.test TXT v=DMARC1; p=reject", "www.example.test CNAME example.test"} {
		if records[key].Origin != "imported" {
			t.Fatalf("imported record missing: %s", key)
		}
	}
	if _, ok := records["old.example.test A 192.0.2.9"]; ok {
		t.Fatal("a skipped record was written")
	}
	d.MailDNS = &serverSetupMailDNS{MX: "now", DMARC: "add"}
	records = desiredByKey(t, d)
	if _, ok := records["example.test MX mx.old.test"]; ok {
		t.Fatal("moving mail now must replace the old MX")
	}
	if _, ok := records["example.test MX mail.example.test"]; !ok {
		t.Fatal("MX not moved")
	}
	if _, ok := records["_dmarc.example.test TXT v=DMARC1; p=reject"]; ok {
		t.Fatal("DMARC add must replace the old one")
	}
	// An imported address may not take a name the setup gives to a server.
	d.ImportedRecords = []serverSetupImportedRecord{{Name: "mail.example.test", Type: "A", Content: "192.0.2.55", Decision: "import"}}
	_, err := serverSetupInfrastructureDNSDesired(d)
	var planErr *serverSetupInfrastructureDNSPlanError
	if !errors.As(err, &planErr) || planErr.Code != "infrastructure_dns_imported_clash" {
		t.Fatalf("clash not refused: %v", err)
	}
	// Without mail only the imports are written; a backup DNS server writes none.
	d = mailDraftForTest()
	d.Customization = &serverSetupCustomization{Components: []string{"nginx"}}
	d.ImportedRecords = []serverSetupImportedRecord{{Name: "example.test", Type: "A", Content: "192.0.2.80", Decision: "import"}}
	records = desiredByKey(t, d)
	if _, ok := records["example.test MX mail.example.test"]; ok {
		t.Fatal("mail records without mail")
	}
	if records["example.test A 192.0.2.80"].Origin != "imported" {
		t.Fatal("import missing")
	}
	if !serverSetupNeedsMailDNSStep(mailDraftForTest()) {
		t.Fatal("DKIM step missing for the main DNS server")
	}
	backup := mailDraftForTest()
	backup.DNSRole, backup.InfrastructureDNS = "secondary", nil
	if serverSetupNeedsMailDNSStep(backup) {
		t.Fatal("a backup DNS server cannot publish the main domain's DKIM")
	}
}

func TestServerSetupBuildMergeSPF(t *testing.T) {
	for old, want := range map[string]string{
		"":                                 "v=spf1 mx a:mail.example.test ~all",
		"v=spf1 -all":                      "v=spf1 a:mail.example.test -all",
		"v=spf1 include:x.test ?all":       "v=spf1 include:x.test a:mail.example.test ?all",
		"v=spf1 a:mail.example.test ~all":  "v=spf1 a:mail.example.test ~all",
		"v=spf1 mx redirect=_spf.old.test": "v=spf1 mx a:mail.example.test redirect=_spf.old.test",
	} {
		if got := mergeServerSetupSPF(old, "mail.example.test"); got != want {
			t.Fatalf("%q -> %q, want %q", old, got, want)
		}
	}
}

func TestServerSetupCatalogPublicRecordsReadKnownNamesOnly(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	asked := map[string]bool{}
	answers := map[string]func() ([]string, error){
		"example.test/A":         func() ([]string, error) { return []string{"192.0.2.80"}, nil },
		"example.test/MX":        func() ([]string, error) { return []string{"10 mx.old.test"}, nil },
		"www.example.test/CNAME": func() ([]string, error) { return []string{"example.test"}, nil },
		"www.example.test/A":     func() ([]string, error) { return []string{"192.0.2.80"}, nil },
		"mail.example.test/A":    func() ([]string, error) { return nil, errors.New("timeout") },
	}
	previous := setupPublicRecordResolvers
	t.Cleanup(func() { setupPublicRecordResolvers = previous })
	setupPublicRecordResolvers = func() []setupProviderRecordLookup {
		lookup := func(_ context.Context, name string, qtype uint16) ([]string, error) {
			calls.Add(1)
			kind := ""
			for label, value := range serverSetupRecordTypes {
				if value == qtype {
					kind = label
				}
			}
			mu.Lock()
			asked[name] = true
			mu.Unlock()
			if answer, ok := answers[name+"/"+kind]; ok {
				return answer()
			}
			return nil, &net.DNSError{IsNotFound: true}
		}
		return []setupProviderRecordLookup{lookup}
	}
	extras, err := serverSetupPublicExtras("shop, blog.example.test", "example.test")
	if err != nil || !slices.Equal(extras, []string{"blog.example.test", "shop.example.test"}) {
		t.Fatalf("extras: %v %v", extras, err)
	}
	if _, err := serverSetupPublicExtras("a b", "example.test"); err == nil {
		t.Fatal("invalid extra accepted")
	}
	p := &Panel{}
	got := p.serverSetupPublicRecords(context.Background(), "example.test", extras, time.Now())
	state := map[string]string{}
	for _, record := range got.Records {
		state[record.Name+"/"+record.Type] = record.State
	}
	if state["example.test/A"] != "found" || state["example.test/MX"] != "found" || state["mail.example.test/A"] != "unknown" || state["webmail.example.test/A"] != "none" {
		t.Fatalf("states: %v", state)
	}
	if _, ok := state["www.example.test/A"]; ok {
		t.Fatal("an alias keeps only its CNAME")
	}
	for name := range asked {
		if !serverSetupDNSNameWithin(name, "example.test") {
			t.Fatalf("asked a name outside the domain: %s", name)
		}
	}
	before := calls.Load()
	p.serverSetupPublicRecords(context.Background(), "example.test", extras, time.Now())
	if calls.Load() == before {
		t.Fatal("an answer with an unread name was kept, so Try again would not ask again")
	}
	answers["mail.example.test/A"] = func() ([]string, error) { return []string{"192.0.2.55"}, nil }
	p.serverSetupPublicRecords(context.Background(), "example.test", extras, time.Now())
	before = calls.Load()
	p.serverSetupPublicRecords(context.Background(), "example.test", extras, time.Now())
	if calls.Load() != before {
		t.Fatal("a complete read within a minute asked the internet again")
	}
	f, state0 := setupOperationFixture(t)
	w := httptest.NewRecorder()
	f.panel.handleServerSetupPublicRecords(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPublicRecordsPath, "", f.userID))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_main_domain_required") {
		t.Fatalf("no main domain: %d %s", w.Code, w.Body.String())
	}
	state0.Draft.MainDomain = "example.test"
	if _, err := f.panel.saveServerSetupDraft(context.Background(), state0.Revision, state0.Draft); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	f.panel.handleServerSetupPublicRecords(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPublicRecordsPath+"?extra=shop", "", f.userID))
	var value serverSetupPublicRecords
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &value) != nil || value.Domain != "example.test" || !slices.Equal(value.Extras, []string{"shop.example.test"}) {
		t.Fatalf("endpoint: %d %s", w.Code, w.Body.String())
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("reading public records mutated the host")
	}
}

func TestServerSetupCatalogProviderMailRowsAndIPv6PTR(t *testing.T) {
	d := mailDraftForTest()
	d.DNSMode = "external"
	rows := serverSetupProviderMailRows(d, "")
	if len(rows) != 4 || rows[3].State != "pending" || rows[3].Value != "" {
		t.Fatalf("mail rows: %+v", rows)
	}
	state, _, value := classifyServerSetupMailRecord(rows[1], [][]string{{"google-site-verification=x", "v=spf1 include:_spf.old.test ~all"}}, 0)
	if state != "different" || value != "v=spf1 include:_spf.old.test a:mail.example.test ~all" {
		t.Fatalf("SPF row must ask for the merged policy: %s %s", state, value)
	}
	if state, _, _ := classifyServerSetupMailRecord(rows[0], [][]string{{"10 mail.example.test"}}, 0); state != "ok" {
		t.Fatal("MX here is ok")
	}
	d.MailDNS = &serverSetupMailDNS{MX: "later"}
	if state, _, _ := classifyServerSetupMailRecord(serverSetupProviderMailRows(d, "")[0], [][]string{{"10 mx.old.test"}}, 0); state != "kept" {
		t.Fatalf("an old MX kept by choice is not a problem: %s", state)
	}
	if state, _, _ := classifyServerSetupMailRecord(rows[2], nil, 1); state != "unknown" {
		t.Fatal("a failed lookup must be unknown")
	}

	if ipv6ReverseName(net.ParseIP("2001:db8::1")) != "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa" {
		t.Fatal("reverse name")
	}
	previousIP, previousResolvers := setupMailIPv6, setupPTRResolvers
	t.Cleanup(func() { setupMailIPv6, setupPTRResolvers = previousIP, previousResolvers })
	if _, applies := setupMailIPv6Check(context.Background(), "mail.example.test"); applies {
		t.Fatal("no IPv6 address, no check")
	}
	setupMailIPv6 = func() string { return "2001:db8:100::10" }
	answer := func(values []string, err error) func() []setupProviderRecordLookup {
		return func() []setupProviderRecordLookup {
			return []setupProviderRecordLookup{func(_ context.Context, name string, qtype uint16) ([]string, error) {
				if qtype != dnswire.TypePTR || !strings.HasSuffix(name, ".ip6.arpa") {
					t.Errorf("wrong question %s %d", name, qtype)
				}
				return values, err
			}}
		}
	}
	setupPTRResolvers = answer(nil, errors.New("timeout"))
	if check, _ := setupMailIPv6Check(context.Background(), "mail.example.test"); check.State != "unknown" {
		t.Fatalf("no answer must be unknown: %+v", check)
	}
	setupPTRResolvers = answer([]string{"static.10.provider.example"}, nil)
	if check, _ := setupMailIPv6Check(context.Background(), "mail.example.test"); check.State != "action_required" || check.Reason != "reverse_dns6_mismatch" || check.Vars["ptr"] != "static.10.provider.example" {
		t.Fatalf("mismatch: %+v", check)
	}
	setupPTRResolvers = answer([]string{"mail.example.test."}, nil)
	if check, _ := setupMailIPv6Check(context.Background(), "mail.example.test"); check.State != "ready" {
		t.Fatalf("ready: %+v", check)
	}
}

func TestServerSetupBuildMailDNSStepFollowsTheMailProfile(t *testing.T) {
	f, state := setupOperationFixture(t)
	capabilities := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
	f.agent.versionCapabilities = &capabilities
	state.Draft.MainDomain = "example.test"
	state.Draft.MailHostname = "mail.example.test"
	plan := customSetupPlanForTest(t, f, state, "postfix")
	profile, dkim := -1, -1
	for index, step := range plan.Steps {
		if step.Kind == "mail_profile" && profile < 0 {
			profile = index
		}
		if step.Kind == "mail_dns" {
			dkim = index
			if step.Target != "example.test" {
				t.Fatalf("mail DNS step target: %+v", step)
			}
		}
	}
	if profile < 0 || dkim <= profile {
		t.Fatalf("the DKIM step must follow the mail profile: %+v", plan.Steps)
	}
	state.Draft.MainDomain = ""
	plan = customSetupPlanForTest(t, f, state, "postfix")
	for _, step := range plan.Steps {
		if step.Kind == "mail_dns" {
			t.Fatal("without a main domain there is no main-domain mail step")
		}
	}
}

func TestServerSetupBuildMailDNSSigningIdentityIsTheSteps(t *testing.T) {
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-mail_dns", Kind: "mail_dns"}, RequestID: serverSetupID("e", "09-mail_dns", "request"), OwnerID: serverSetupID("e", "09-mail_dns", "owner")}
	first, again := serverSetupMailDNSSigningIdentity(step), serverSetupMailDNSSigningIdentity(step)
	if first.RequestID != again.RequestID || first.OwnerID != again.OwnerID || !validServiceOperationID(first.RequestID) || !validServiceOperationID(first.OwnerID) {
		t.Fatalf("signing identity must be fixed per step: %+v %+v", first, again)
	}
	other := step
	other.RequestID = serverSetupID("f", "09-mail_dns", "request")
	if serverSetupMailDNSSigningIdentity(other).RequestID == first.RequestID || first.RequestID == step.RequestID {
		t.Fatal("signing identity must belong to its own step and differ from the step's")
	}
}

func TestServerSetupBuildReverseDNSIsAnOpenTaskNotAWait(t *testing.T) {
	health := transport.MailHealthResponse{ServerIP: "203.0.113.42", Myhostname: "mail.example.test", HostnameFQDN: true, PTR: "static.provider.example.", PTRAligned: false, FCrDNS: false}
	ptr := setupMailIdentityCheck(health, nil, "mail.example.test")
	if ptr.State != "action_required" || !ptr.Advisory {
		t.Fatalf("a provider PTR must be an open task: %+v", ptr)
	}
	ready := serverSetupCheck{ID: "services", State: "ready", Code: "ready"}
	if !serverSetupChecksReady([]serverSetupCheck{ready, ptr}) {
		t.Fatal("setup waited for reverse DNS")
	}
	health.Myhostname, health.HostnameFQDN = "debian", false
	if name := setupMailIdentityCheck(health, nil, "mail.example.test"); name.Advisory || serverSetupChecksReady([]serverSetupCheck{ready, name}) {
		t.Fatalf("a wrong mail name must still block: %+v", name)
	}
	if serverSetupChecksReady([]serverSetupCheck{ptr}) {
		t.Fatal("open tasks alone never complete setup")
	}
}

func TestServerSetupDraftSkippedRecordsNeverTrapTheOwner(t *testing.T) {
	d := mailDraftForTest()
	bad := []serverSetupImportedRecord{
		{Name: "example.test", Type: "MX", Content: "", Decision: "skip"},
		{Name: "example.test", Type: "TXT", Content: `say "hi" \ there`, Decision: "skip"},
		{Name: "example.test", Type: "CAA", Content: `0 tbs "x"`, Decision: "skip"},
		{Name: "example.test", Type: "TXT", Content: strings.Repeat("x", 5000), Decision: "skip"},
	}
	d.ImportedRecords = bad
	got, err := canonicalServerSetupDraft(d)
	if err != nil || len(got.ImportedRecords) != 3 {
		t.Fatalf("skipped values must never refuse the draft (an oversized one is dropped): %+v %v", got.ImportedRecords, err)
	}
	for _, record := range bad[:2] {
		record.Decision = "import"
		d.ImportedRecords = []serverSetupImportedRecord{record}
		if _, err := canonicalServerSetupDraft(d); err == nil {
			t.Fatalf("an unpublishable import accepted: %+v", record)
		}
	}
	d.ImportedRecords = []serverSetupImportedRecord{
		{Name: "example.test", Type: "TXT", Content: "v=spf1 -all", Decision: "import"},
		{Name: "example.test", Type: "TXT", Content: "v=spf1 mx -all", Decision: "import"},
	}
	if _, err := canonicalServerSetupDraft(d); err == nil {
		t.Fatal("two SPF policies imported")
	}
	marked := []serverSetupPublicRecord{
		{Name: "example.test", Type: "MX", Values: []string{"0 ", "10 mx.old.test"}, State: "found"},
		{Name: "example.test", Type: "TXT", Values: []string{"v=spf1 -all", "v=spf1 mx -all", `a "quoted" one`}, State: "found"},
	}
	markServerSetupImportable(marked, "example.test")
	if marked[0].NotImportable["0 "] != "content_invalid" || marked[0].NotImportable["10 mx.old.test"] != "" ||
		marked[1].NotImportable["v=spf1 mx -all"] != "one_per_name" || marked[1].NotImportable[`a "quoted" one`] != "content_invalid" || marked[1].NotImportable["v=spf1 -all"] != "" {
		t.Fatalf("importability marks: %+v %+v", marked[0].NotImportable, marked[1].NotImportable)
	}
}

func TestServerSetupBuildImportedRecordsKeepTheZoneValid(t *testing.T) {
	clash := func(records ...serverSetupImportedRecord) error {
		d := mailDraftForTest()
		d.ImportedRecords = records
		_, err := serverSetupInfrastructureDNSDesired(d)
		return err
	}
	for label, records := range map[string][]serverSetupImportedRecord{
		"CNAME beside A":          {{Name: "www.example.test", Type: "CNAME", Content: "example.test", Decision: "import"}, {Name: "www.example.test", Type: "A", Content: "192.0.2.1", Decision: "import"}},
		"CNAME at the DMARC name": {{Name: "_dmarc.example.test", Type: "CNAME", Content: "dmarc.old.test", Decision: "import"}},
		"imported DKIM key":       {{Name: dkimSelector + "._domainkey.example.test", Type: "TXT", Content: "v=DKIM1; p=abc", Decision: "import"}},
	} {
		var planErr *serverSetupInfrastructureDNSPlanError
		if err := clash(records...); !errors.As(err, &planErr) || planErr.Code != "infrastructure_dns_imported_clash" {
			t.Fatalf("%s not refused: %v", label, err)
		}
	}
	d := mailDraftForTest()
	d.ImportedRecords = []serverSetupImportedRecord{{Name: "example.test", Type: "CAA", Content: `0 ISSUE letsencrypt.org`, Decision: "import"}}
	records := desiredByKey(t, d)
	if _, ok := records[`example.test CAA 0 issue "letsencrypt.org"`]; !ok {
		t.Fatalf("imports are written in the renderer's own form: %v", records)
	}
	// Against a zone that already has a different SPF, DMARC or MX: refused.
	for label, existing := range map[string]serverSetupInfrastructureDNSStoredRecord{
		"SPF":            {Name: "example.test", Type: "TXT", Content: `"v=spf1 include:other.test -all"`},
		"DMARC":          {Name: "_dmarc.example.test", Type: "TXT", Content: `"v=DMARC1; p=reject"`},
		"MX":             {Name: "example.test", Type: "MX", Content: "mx.old.test", Priority: 5},
		"A at the alias": {Name: "www.example.test", Type: "A", Content: "192.0.2.1"},
	} {
		d := mailDraftForTest()
		d.ImportedRecords = []serverSetupImportedRecord{{Name: "www.example.test", Type: "CNAME", Content: "example.test", Decision: "import"}}
		if label != "A at the alias" {
			d.ImportedRecords = nil
		}
		_, err := buildServerSetupInfrastructureDNSAgainstSnapshot(d, serverSetupInfrastructureDNSSnapshot{Zone: "example.test", ZoneID: 1, Records: []serverSetupInfrastructureDNSStoredRecord{existing}})
		var planErr *serverSetupInfrastructureDNSPlanError
		if !errors.As(err, &planErr) || planErr.Code != "infrastructure_dns_imported_clash" {
			t.Fatalf("existing %s not refused: %v", label, err)
		}
	}
	other := mailDraftForTest()
	if _, err := buildServerSetupInfrastructureDNSAgainstSnapshot(other, serverSetupInfrastructureDNSSnapshot{Zone: "example.test", ZoneID: 1, Records: []serverSetupInfrastructureDNSStoredRecord{{Name: "example.test", Type: "TXT", Content: `"google-site-verification=x"`}}}); err != nil {
		t.Fatalf("an unrelated TXT is not a second SPF: %v", err)
	}
}

func TestServerSetupBuildForwardDNSStillBlocksAndExternalMXDefaultsToLater(t *testing.T) {
	health := transport.MailHealthResponse{ServerIP: "203.0.113.42", Myhostname: "mail.example.test", HostnameFQDN: true, PTR: "mail.example.test.", PTRAligned: true, FCrDNS: false}
	check := setupMailIdentityCheck(health, nil, "mail.example.test")
	if check.Reason != "forward_dns_mismatch" || check.Advisory {
		t.Fatalf("a mail host whose address does not reach this server must block: %+v", check)
	}
	d := mailDraftForTest()
	d.DNSMode = "external"
	if rows := serverSetupProviderMailRows(d, ""); rows[0].Kind != "mx-later" {
		t.Fatalf("an unanswered MX choice at a provider is later: %+v", rows[0])
	}
	d.MailDNS = &serverSetupMailDNS{MX: "now"}
	if rows := serverSetupProviderMailRows(d, ""); rows[0].Kind != "mx" {
		t.Fatal("now must ask for the new MX")
	}
}

func TestServerSetupBuildMailDNSRunsAfterTheZoneIsWritten(t *testing.T) {
	d := mailDraftForTest()
	d.Customization = &serverSetupCustomization{Components: []string{"nginx", "postfix"}}
	steps := []serverSetupPlanStep{{Kind: "dns", Target: "local"}, {Kind: "service", Target: "nginx"}, {Kind: "mail_profile", Target: "core-mail"}, {Kind: "mail_dns", Target: "example.test"},
		{Kind: "service", Target: "nftables"}, {Kind: "firewall"}, {Kind: "infrastructure_dns", Target: "example.test"}, {Kind: "access_dns", Target: d.PanelDomain}, {Kind: "verify"}}
	ordered := serverSetupDNSBootstrapSteps(d, steps)
	zone, dkim := -1, -1
	for index, step := range ordered {
		if step.Kind == "infrastructure_dns" {
			zone = index
		}
		if step.Kind == "mail_dns" {
			dkim = index
		}
	}
	if zone < 0 || dkim < zone {
		t.Fatalf("the DKIM record needs the zone first: %+v", ordered)
	}
}
