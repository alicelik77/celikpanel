package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestValidateIssuedCertificateLineageAcceptsOnlyTheDomainsNativeLineage(t *testing.T) {
	own := "celikpanel-site-42-00112233445566778899aabb"
	for _, replacement := range []bool{false, true} {
		if err := validateIssuedCertificateLineage("example.test", 42, replacement, own); err != nil {
			t.Fatalf("replacement=%v: %v", replacement, err)
		}
		if err := validateIssuedCertificateLineage("example.test", 43, replacement, own); err == nil {
			t.Fatalf("replacement=%v: another domain's lineage was accepted", replacement)
		}
	}
}

func TestCleanupTargetOfANativeLineageNamesNoSnapshot(t *testing.T) {
	full, key, chain := transport.NativeSiteLineagePaths("celikpanel-site-42-00112233445566778899aabb")
	target := certificateCleanupTarget{Domain: "example.test", CertPath: full, KeyPath: key, ChainPath: chain}
	if got := target.snapshotPath(); got != "" {
		t.Fatalf("snapshot path = %q: deleting it would make the Agent refuse the lineage cleanup", got)
	}
}

func TestDecideNativeObservation(t *testing.T) {
	recorded := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	row := nativeSiteRow{ExpiresAt: recorded.Format(time.RFC3339), RenewalStatus: ""}
	state := transport.SiteCertificateRenewalStateResponse{State: transport.SiteRenewalScheduled, NotAfter: recorded, NotBefore: recorded.Add(-90 * 24 * time.Hour)}

	got := decideNativeObservation(row, state)
	if got.Renewed || got.Status != nativeRenewalStatus(transport.SiteRenewalScheduled) {
		t.Fatalf("healthy = %+v", got)
	}

	renewed := state
	renewed.NotBefore = recorded.Add(-30 * 24 * time.Hour)
	renewed.NotAfter = recorded.Add(60 * 24 * time.Hour)
	renewed.LastDeploy = &transport.SiteRenewalDeploy{CompletedAt: renewed.NotBefore.Add(time.Minute), Nginx: "reloaded"}
	got = decideNativeObservation(row, renewed)
	if !got.Renewed || !got.ExpiresAt.Equal(renewed.NotAfter) || got.Status != nativeRenewalStatus(transport.SiteRenewalRenewed) ||
		!got.AttemptAt.Equal(renewed.LastDeploy.CompletedAt) {
		t.Fatalf("renewed = %+v", got)
	}
	// It stays "renewed" while the schedule is healthy.
	got = decideNativeObservation(nativeSiteRow{ExpiresAt: renewed.NotAfter.Format(time.RFC3339),
		RenewalStatus: nativeRenewalStatus(transport.SiteRenewalRenewed)}, renewed)
	if got.Renewed || got.Status != "" {
		t.Fatalf("second reading = %+v", got)
	}

	failed := state
	failed.State = transport.SiteRenewalFailed
	failed.LastFailed = &transport.SiteRenewalFailure{At: recorded.Add(-20 * 24 * time.Hour), Detail: "connection refused"}
	got = decideNativeObservation(row, failed)
	if got.Status != nativeRenewalStatus(transport.SiteRenewalFailed) || !got.AttemptAt.Equal(failed.LastFailed.At) {
		t.Fatalf("failed = %+v", got)
	}

	unknown := state
	unknown.State = transport.SiteRenewalUnknown
	if got = decideNativeObservation(nativeSiteRow{ExpiresAt: row.ExpiresAt, RenewalStatus: nativeRenewalStatus(transport.SiteRenewalFailed)}, unknown); got.Status != "" {
		t.Fatalf("unknown overwrote a known failure: %+v", got)
	}
	for _, outbox := range []string{sslPendingActivation, sslPendingDependents, sslWaitingForOwner} {
		got = decideNativeObservation(nativeSiteRow{ExpiresAt: row.ExpiresAt, RenewalStatus: outbox}, renewed)
		if got.Status != "" || !got.Renewed {
			t.Fatalf("%s: %+v", outbox, got)
		}
	}
}

type nativeRenewalAgent struct {
	*serviceOperationTestAgent
	mu    sync.Mutex
	state transport.SiteCertificateRenewalStateResponse
	err   error
	calls []transport.SiteCertificateRenewalStateRequest
	// autoRenew records SetSiteCertificateAutoRenewV1 calls.
	autoRenew []transport.SetSiteCertificateAutoRenewRequest
	// retire records RetireSiteCertificateLineagesV1 calls; retireOutcome is
	// what it answers for each lineage ("" retired).
	retire        []transport.RetireSiteCertificateLineagesRequest
	retireOutcome string
}

func (a *nativeRenewalAgent) RetireSiteCertificateLineagesV1(req *transport.RetireSiteCertificateLineagesRequest, resp *transport.RetireSiteCertificateLineagesResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.retire = append(a.retire, *req)
	outcome := a.retireOutcome
	if outcome == "" {
		outcome = transport.SiteLineageRetired
	}
	for _, lineage := range req.Lineages {
		resp.Results = append(resp.Results, transport.RetireSiteLineageResult{LineageName: lineage.LineageName, Outcome: outcome, Detail: "/etc/nginx/sites-enabled/own.conf"})
	}
	return nil
}

func (a *nativeRenewalAgent) SiteCertificateRenewalStateV1(req *transport.SiteCertificateRenewalStateRequest, resp *transport.SiteCertificateRenewalStateResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, *req)
	if a.err != nil {
		return a.err
	}
	*resp = a.state
	return nil
}

func (a *nativeRenewalAgent) SetSiteCertificateAutoRenewV1(req *transport.SetSiteCertificateAutoRenewRequest, resp *transport.SetSiteCertificateAutoRenewResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.autoRenew = append(a.autoRenew, *req)
	resp.Changed = true
	return nil
}

// alpha.134 lab: a replaced native lineage is removed with certbot's own
// means once nothing serves it, never switched off and left behind; nothing
// is asked while the active certificate is not served yet.
func TestObserveNativeSiteCertificatesRetiresReplacedLineages(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, "native-retire.example", "static")
	replaced := "celikpanel-site-" + strconv.Itoa(domainID) + "-ffffffffffffffffffffffff"
	current := "celikpanel-site-" + strconv.Itoa(domainID) + "-eeeeeeeeeeeeeeeeeeeeeeee"
	insert := func(lineage, status, renewal string) {
		t.Helper()
		full, key, chain := transport.NativeSiteLineagePaths(lineage)
		if _, err := panel.db.GetDB().Exec(`
			INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, chain_path, lineage_name,
				acme_provider_id, issued_at, expires_at, auto_renew, status, renewal_status)
			VALUES (?, 'letsencrypt', ?, ?, ?, ?, 'zerossl', '2026-09-01T00:00:00Z', '2026-12-01T00:00:00Z', true, ?, ?)`,
			domainID, full, key, chain, lineage, status, renewal); err != nil {
			t.Fatal(err)
		}
	}
	insert(replaced, "revoked", "")
	insert(current, "active", sslPendingActivation)
	agent := &nativeRenewalAgent{serviceOperationTestAgent: newServiceOperationTestAgent()}
	attachStartupVhostBatchAgent(t, panel, agent)
	openLineageRetireGate(t, panel)

	// The new certificate is not served yet: the old one may still be.
	panel.retireSupersededNativeLineages(t.Context())
	agent.mu.Lock()
	waited := len(agent.retire)
	agent.mu.Unlock()
	if waited != 0 {
		t.Fatalf("asked to remove a lineage while the active certificate waits for its activation")
	}

	if _, err := panel.db.GetDB().Exec(`UPDATE ssl_certificates SET renewal_status = '' WHERE lineage_name = ?`, current); err != nil {
		t.Fatal(err)
	}
	panel.runDueCertRenewals()
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.retire) != 1 {
		t.Fatalf("retire calls = %+v", agent.retire)
	}
	call := agent.retire[0]
	if call.DomainID != domainID || call.SubscriptionID != subscriptionID || call.ActiveLineage != current ||
		len(call.Lineages) != 1 || call.Lineages[0].LineageName != replaced ||
		call.Lineages[0].ACMEServer != "https://acme.zerossl.com/v2/DV90" || strings.TrimSpace(call.ExpectedBuildCommit) != strings.TrimSpace(buildCommit) {
		t.Fatalf("retire call = %+v", call)
	}
	if len(agent.autoRenew) != 0 {
		t.Fatalf("a replaced lineage was switched off instead of removed: %+v", agent.autoRenew)
	}
}

// What was kept is said to an administrator, and to nobody else (nobody
// else can act on it).
func TestRetireAfterIssueTellsOnlyAnAdministratorWhatWasKept(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, "native-kept.example", "static")
	replaced := "celikpanel-site-" + strconv.Itoa(domainID) + "-dddddddddddddddddddddddd"
	full, key, chain := transport.NativeSiteLineagePaths(replaced)
	if _, err := panel.db.GetDB().Exec(`
		INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, chain_path, lineage_name,
			issued_at, expires_at, auto_renew, status)
		VALUES (?, 'letsencrypt', ?, ?, ?, ?, '2026-09-01T00:00:00Z', '2026-12-01T00:00:00Z', true, 'revoked')`,
		domainID, full, key, chain, replaced); err != nil {
		t.Fatal(err)
	}
	agent := &nativeRenewalAgent{serviceOperationTestAgent: newServiceOperationTestAgent(), retireOutcome: transport.SiteLineageInUse}
	attachStartupVhostBatchAgent(t, panel, agent)
	openLineageRetireGate(t, panel)
	if got := panel.retireAfterIssue(t.Context(), &Caller{ID: 2, Role: roleCustomer}, domainID); got != nil {
		t.Fatalf("customer answer = %v", got)
	}
	got := panel.retireAfterIssue(t.Context(), &Caller{ID: 1, Role: roleAdmin}, domainID)
	if got["outcome"] != transport.SiteLineageInUse || got["lineage"] != replaced || got["detail"] != "/etc/nginx/sites-enabled/own.conf" {
		t.Fatalf("administrator answer = %v", got)
	}
	// Kept for an update's rollback copy, or waiting behind the update gate,
	// is ordinary operation that ends by itself: no warning.
	agent.retireOutcome = transport.SiteLineageHeldBySnapshot
	if got := panel.retireAfterIssue(t.Context(), &Caller{ID: 1, Role: roleAdmin}, domainID); got != nil {
		t.Fatalf("held for a rollback copy: answer = %v", got)
	}
	panel.startedAt.Store(time.Now().UnixNano())
	if got := panel.retireAfterIssue(t.Context(), &Caller{ID: 1, Role: roleAdmin}, domainID); got != nil {
		t.Fatalf("waiting behind the gate: answer = %v", got)
	}
	panel.startedAt.Store(time.Now().Add(-time.Hour).UnixNano())
	for _, outcome := range []string{transport.SiteLineageOwnerChanged, transport.SiteLineageFailed, transport.SiteLineageNotChecked} {
		agent.retireOutcome = outcome
		if got := panel.retireAfterIssue(t.Context(), &Caller{ID: 1, Role: roleAdmin}, domainID); got["outcome"] != outcome {
			t.Fatalf("%s: answer = %v", outcome, got)
		}
	}
	agent.retireOutcome = transport.SiteLineageRetired
	if got := panel.retireAfterIssue(t.Context(), &Caller{ID: 1, Role: roleAdmin}, domainID); got != nil {
		t.Fatalf("removed: answer = %v", got)
	}
}

func TestRenewalSettingOfANativeCertificateWritesCertbotsOwnSetting(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, "native-setting.example", "static")
	lineage := "celikpanel-site-" + strconv.Itoa(domainID) + "-00112233445566778899aabb"
	full, key, chain := transport.NativeSiteLineagePaths(lineage)
	if _, err := panel.db.GetDB().Exec(`
		INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, chain_path, lineage_name,
			issued_at, expires_at, auto_renew, status)
		VALUES (?, 'letsencrypt', ?, ?, ?, ?, '2026-10-01T00:00:00Z', '2026-12-30T00:00:00Z', true, 'active')`,
		domainID, full, key, chain, lineage); err != nil {
		t.Fatal(err)
	}
	agent := &nativeRenewalAgent{serviceOperationTestAgent: newServiceOperationTestAgent()}
	attachStartupVhostBatchAgent(t, panel, agent)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/domains/x/ssl/renewal", strings.NewReader(`{"auto_renew": false}`))
	panel.handleSSLRenewalSetting(recorder, request, domainID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	agent.mu.Lock()
	calls := append([]transport.SetSiteCertificateAutoRenewRequest(nil), agent.autoRenew...)
	agent.mu.Unlock()
	if len(calls) != 1 || calls[0].LineageName != lineage || calls[0].AutoRenew {
		t.Fatalf("calls = %+v", calls)
	}
	var autoRenew bool
	if err := panel.db.GetDB().QueryRow(`SELECT auto_renew FROM ssl_certificates WHERE domain_id = ?`, domainID).Scan(&autoRenew); err != nil || autoRenew {
		t.Fatalf("ledger auto_renew = %v %v", autoRenew, err)
	}
}

func TestObserveNativeSiteCertificatesRecordsWhatTheOperatingSystemDid(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, "native-renewal.example", "static")
	lineage := "celikpanel-site-" + strconv.Itoa(domainID) + "-00112233445566778899aabb"
	full, key, chain := transport.NativeSiteLineagePaths(lineage)
	recorded := time.Now().UTC().Add(20 * 24 * time.Hour).Truncate(time.Second)
	result, err := panel.db.GetDB().Exec(`
		INSERT INTO ssl_certificates (
			domain_id, type, cert_path, key_path, chain_path, lineage_name, acme_provider_id,
			issuer, subject, issued_at, expires_at, auto_renew, secure_mail, renewal_status, status
		) VALUES (?, 'letsencrypt', ?, ?, ?, ?, 'zerossl', 'ZeroSSL', 'native-renewal.example',
		          ?, ?, true, false, '', 'active')`,
		domainID, full, key, chain, lineage,
		recorded.Add(-70*24*time.Hour).Format(time.RFC3339), recorded.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	certID, _ := result.LastInsertId()
	agent := &nativeRenewalAgent{serviceOperationTestAgent: newServiceOperationTestAgent()}
	newNotAfter := recorded.Add(60 * 24 * time.Hour)
	agent.state = transport.SiteCertificateRenewalStateResponse{
		State: transport.SiteRenewalScheduled, LineageName: lineage,
		NotBefore: recorded.Add(-30 * 24 * time.Hour), NotAfter: newNotAfter,
	}
	attachStartupVhostBatchAgent(t, panel, agent)

	read := func() (string, string, string) {
		var expires, status, attempt string
		if err := panel.db.GetDB().QueryRow(`
			SELECT expires_at, COALESCE(renewal_status, ''), COALESCE(last_renewal_attempt, '')
			FROM ssl_certificates WHERE id = ?`, certID).Scan(&expires, &status, &attempt); err != nil {
			t.Fatal(err)
		}
		return expires, status, attempt
	}

	panel.runDueCertRenewals()
	expires, status, attempt := read()
	if expires != newNotAfter.Format(time.RFC3339) || status != "os_renewed" || attempt == "" {
		t.Fatalf("after the OS renewal: expires=%s status=%s attempt=%s", expires, status, attempt)
	}
	agent.mu.Lock()
	if len(agent.calls) != 1 || agent.calls[0].LineageName != lineage || agent.calls[0].DomainID != domainID ||
		agent.calls[0].SubscriptionID != subscriptionID || agent.calls[0].ACMEServer != "https://acme.zerossl.com/v2/DV90" {
		t.Fatalf("calls = %+v", agent.calls)
	}
	agent.state.State = transport.SiteRenewalFailed
	agent.state.Detail = "connection refused"
	agent.state.LastFailed = &transport.SiteRenewalFailure{At: time.Now().UTC().Truncate(time.Second), Detail: "connection refused"}
	agent.mu.Unlock()

	panel.runDueCertRenewals()
	_, status, _ = read()
	if status != "os_failed" {
		t.Fatalf("status = %s", status)
	}

	// The dashboard lists it whatever the days, as the administrator's.
	recorder := httptest.NewRecorder()
	panel.handleDashboard(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))
	var dashboard dashboardExtras
	if err := json.NewDecoder(recorder.Body).Decode(&dashboard); err != nil {
		t.Fatal(err)
	}
	if len(dashboard.ExpiringCerts) != 1 || dashboard.ExpiringCerts[0].RenewalProblem != transport.SiteRenewalFailed ||
		dashboard.ExpiringCerts[0].DomainID != domainID {
		t.Fatalf("dashboard = %+v", dashboard.ExpiringCerts)
	}

	// An unreadable state records nothing: the known failure stays.
	agent.mu.Lock()
	agent.err = errors.New("agent restarting")
	agent.mu.Unlock()
	panel.runDueCertRenewals()
	if _, status, _ = read(); status != "os_failed" {
		t.Fatalf("an unknown reading changed the state to %s", status)
	}
}

// openLineageRetireGate passes the gate a removal waits behind (the one the
// database-rights rewrite uses): the update to this build is verified, the
// start-up quiet time has passed, the server is quiet.
func openLineageRetireGate(t *testing.T, panel *Panel) {
	t.Helper()
	savedPhase, savedQuiet, savedCommit := databaseGrantFormsUpdatePhase, databaseGrantFormsHostQuiet, databaseGrantFormsCommit
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "succeeded", true }
	databaseGrantFormsHostQuiet = func(*Panel, context.Context) bool { return true }
	databaseGrantFormsCommit = func() string { return strings.Repeat("a", 40) }
	panel.startedAt.Store(time.Now().Add(-time.Hour).UnixNano())
	lineageOutcomeAudited.Range(func(key, _ any) bool { lineageOutcomeAudited.Delete(key); return true })
	t.Cleanup(func() {
		databaseGrantFormsUpdatePhase, databaseGrantFormsHostQuiet, databaseGrantFormsCommit = savedPhase, savedQuiet, savedCommit
	})
}

// replacedLineageFixture is a domain with one replaced native lineage and a
// fake Agent that answers retireOutcome.
func replacedLineageFixture(t *testing.T, name string) (*Panel, int, string, *nativeRenewalAgent) {
	t.Helper()
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, name, "static")
	replaced := "celikpanel-site-" + strconv.Itoa(domainID) + "-cccccccccccccccccccccccc"
	full, key, chain := transport.NativeSiteLineagePaths(replaced)
	if _, err := panel.db.GetDB().Exec(`
		INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, chain_path, lineage_name,
			issued_at, expires_at, auto_renew, status)
		VALUES (?, 'letsencrypt', ?, ?, ?, ?, '2026-09-01T00:00:00Z', '2026-12-01T00:00:00Z', true, 'revoked')`,
		domainID, full, key, chain, replaced); err != nil {
		t.Fatal(err)
	}
	agent := &nativeRenewalAgent{serviceOperationTestAgent: newServiceOperationTestAgent()}
	attachStartupVhostBatchAgent(t, panel, agent)
	return panel, domainID, replaced, agent
}

// Removing is irreversible: it waits for a verified update, the start-up
// quiet time and a quiet server, through the database-rights helper.
func TestRetireReplacedNativeLineagesWaitsBehindTheUpdateGate(t *testing.T) {
	panel, domainID, _, agent := replacedLineageFixture(t, "native-gate.example")
	openLineageRetireGate(t, panel)
	for name, shut := range map[string]func(){
		"just started":           func() { panel.startedAt.Store(time.Now().UnixNano()) },
		"update not verified":    func() { databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "running", true } },
		"update state not known": func() { databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "", false } },
		"server busy":            func() { databaseGrantFormsHostQuiet = func(*Panel, context.Context) bool { return false } },
	} {
		openLineageRetireGate(t, panel)
		shut()
		results, err := panel.retireReplacedNativeLineages(t.Context(), domainID)
		var held *lineageRetireHeld
		if results != nil || !errors.As(err, &held) {
			t.Fatalf("%s: results=%v err=%v", name, results, err)
		}
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.retire) != 0 {
		t.Fatalf("asked although held: %+v", agent.retire)
	}
}

// A lineage a site row still names (its file, its key) is never asked about.
func TestRetireReplacedNativeLineagesSkipsALineageASiteStillNames(t *testing.T) {
	panel, domainID, replaced, agent := replacedLineageFixture(t, "native-site-row.example")
	openLineageRetireGate(t, panel)
	_, key, _ := transport.NativeSiteLineagePaths(replaced)
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET ssl_key_path = ? WHERE domain_id = ?`, key, domainID); err != nil {
		t.Fatal(err)
	}
	if results, err := panel.retireReplacedNativeLineages(t.Context(), domainID); results != nil || err != nil {
		t.Fatalf("results=%v err=%v", results, err)
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.retire) != 0 {
		t.Fatalf("asked about a lineage a site names: %+v", agent.retire)
	}
}

// The owner can read each removal and each kept outcome in the audit log,
// once per change of outcome, not at every pass.
func TestRetireReplacedNativeLineagesWritesTheAuditLogOncePerChange(t *testing.T) {
	panel, domainID, replaced, agent := replacedLineageFixture(t, "native-audit.example")
	openLineageRetireGate(t, panel)
	rows := func() []string {
		t.Helper()
		list, err := panel.db.GetDB().Query(`SELECT action || ' ' || COALESCE(details, '') FROM audit_logs WHERE action LIKE 'ssl.lineage.%' AND resource_id = ? ORDER BY id`, domainID)
		if err != nil {
			t.Fatal(err)
		}
		defer list.Close()
		var out []string
		for list.Next() {
			var row string
			_ = list.Scan(&row)
			out = append(out, row)
		}
		return out
	}
	agent.retireOutcome = transport.SiteLineageInUse
	for i := 0; i < 3; i++ {
		if _, err := panel.retireReplacedNativeLineages(t.Context(), domainID); err != nil {
			t.Fatal(err)
		}
	}
	// A restart forgets what it wrote; the audit log itself is read back.
	lineageOutcomeAudited.Delete(replaced)
	_, _ = panel.retireReplacedNativeLineages(t.Context(), domainID)
	agent.retireOutcome = transport.SiteLineageRetired
	_, _ = panel.retireReplacedNativeLineages(t.Context(), domainID)
	got := rows()
	if len(got) != 2 || !strings.HasPrefix(got[0], "ssl.lineage.kept_in_use:native-audit.example ") ||
		!strings.Contains(got[0], `"lineage":"`+replaced+`"`) || !strings.Contains(got[0], `"file":"/etc/nginx/sites-enabled/own.conf"`) ||
		!strings.HasPrefix(got[1], "ssl.lineage.removed:native-audit.example ") {
		t.Fatalf("audit rows = %q", got)
	}
}

// alpha.134 lab run 6: the pass at start is always held (start-up quiet
// time) and the next one came 12 hours later. A held pass is asked again by
// the hourly round of the database-rights rewrite once the same gate opens:
// once, and not again while nothing waits.
func TestHeldLineagePassIsAskedAgainWhenTheGateOpens(t *testing.T) {
	panel, domainID, _, agent := replacedLineageFixture(t, "native-waiting.example")
	openLineageRetireGate(t, panel)
	nativeLineageRetireWaiting.Store(false)
	t.Cleanup(func() { nativeLineageRetireWaiting.Store(false) })
	calls := func() int {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		return len(agent.retire)
	}

	started := time.Now()
	panel.startedAt.Store(started.UnixNano())
	if _, err := panel.retireReplacedNativeLineages(t.Context(), domainID); err == nil || !nativeLineageRetireWaiting.Load() {
		t.Fatalf("the pass at start was not held and marked waiting: %v", err)
	}
	// Still inside the quiet time: the round asks nothing.
	panel.retireWaitingNativeLineages(t.Context(), started)
	if calls() != 0 || !nativeLineageRetireWaiting.Load() {
		t.Fatalf("asked while the gate is closed: calls=%d", calls())
	}
	// The gate opens: the next round asks once.
	opened := time.Now().Add(-time.Hour)
	panel.startedAt.Store(opened.UnixNano())
	panel.retireWaitingNativeLineages(t.Context(), opened)
	if calls() != 1 || nativeLineageRetireWaiting.Load() {
		t.Fatalf("after the gate opened: calls=%d waiting=%v", calls(), nativeLineageRetireWaiting.Load())
	}
	panel.retireWaitingNativeLineages(t.Context(), opened)
	if calls() != 1 {
		t.Fatalf("asked again although nothing waits: calls=%d", calls())
	}
	// The round itself asks it.
	source, err := os.ReadFile("database_grant_forms.go")
	if err != nil || !strings.Contains(string(source), "p.retireWaitingNativeLineages(ctx, startedAt)") {
		t.Fatalf("the hourly round does not ask the waiting pass: %v", err)
	}
}
