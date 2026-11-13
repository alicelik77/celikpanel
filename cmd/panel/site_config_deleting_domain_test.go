package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab (13 Nov 2026): the row of sil7.test, waiting for the rest of
// its deletion, carried "Configuration file missing" under its name, because
// the deletion had removed the file on purpose and the next panel start
// recorded it as missing. A domain that is being deleted shows only its
// deletion state; a domain whose file is really missing keeps its badge.
// Silinmeyi bekleyen alan adi yalniz silme durumunu gosterir; dosyasi gercekten
// eksik olan alan adi rozetini korur.

func markDomainDeletingForTest(t *testing.T, panel *Panel, domainID int) {
	t.Helper()
	database := panel.db.GetDB()
	if _, err := database.Exec(`INSERT INTO domain_deletion_operations (domain_id, previous_status) VALUES (?, 'active')`, domainID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE domains SET status = 'pending' WHERE id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
}

func TestADomainWaitingForItsDeletionShowsNoSiteConfigBadge(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	missing := addStartupHostedDomain(t, panel, subscriptionID, "really-missing.example", "static")
	deleting := addStartupHostedDomain(t, panel, subscriptionID, "sil7.example", "static")
	// The panel start finds both files gone and records them as missing.
	agent := &siteFileTestAgent{batch: func(request *transport.ApplyVhostsRequest, response *transport.ApplyVhostsResponse) {
		for _, vhost := range request.Vhosts {
			item := transport.SiteFileBatchItem{DomainID: vhost.DomainID, SiteID: vhost.SiteID}
			item.File = fileResult(vhost.Domain, transport.SiteFileAbsent, transport.SiteFileOutcomeMissing)
			response.Items = append(response.Items, item)
		}
		response.Counts = &transport.SiteFileCounts{Missing: len(request.Vhosts)}
	}}
	attachStartupVhostBatchAgent(t, panel, agent)
	if _, err := panel.reconcileHostedVhostsAtStartupDetailed(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	summaries, err := panel.domainSiteConfigSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, domainID := range []int{missing, deleting} {
		if summaries[domainID].State != transport.SiteFileMissingState {
			t.Fatalf("domain %d before its deletion: %+v", domainID, summaries[domainID])
		}
	}

	markDomainDeletingForTest(t, panel, deleting)
	summaries, err = panel.domainSiteConfigSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary, ok := summaries[deleting]; ok {
		t.Fatalf("the domain waiting for its deletion still has a site configuration summary: %+v", summary)
	}
	if siteConfigFor(summaries, deleting) != nil {
		t.Fatal("the list row of the deleting domain would carry a badge")
	}
	if summaries[missing].State != transport.SiteFileMissingState {
		t.Fatalf("the really missing file lost its badge: %+v", summaries[missing])
	}
	// The ledger is unchanged: the evidence of what the start found stays.
	if row := ledgerRow(t, panel, deleting); row.State != transport.SiteFileMissingState {
		t.Fatalf("the deleting domain's ledger row changed: %+v", row)
	}
}

// The usage worker does not measure the disk of a site whose domain is being
// deleted (it logged "could not be measured" for it every five minutes).
func TestTheDiskOfADomainBeingDeletedIsNotMeasured(t *testing.T) {
	agent := &siteUsageTestAgent{}
	p := newSiteUsageTestPanel(t, agent, nil)
	database := p.db.GetDB()
	if _, err := database.Exec(`INSERT INTO domain_deletion_operations (domain_id, previous_status) VALUES (7, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE domains SET status = 'pending' WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	if err := p.measureDueSiteDisks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := agent.calls.Load(); calls != 0 {
		t.Fatalf("measured the deleting domain's site %d times", calls)
	}
	// The same site, not being deleted, is measured.
	if _, err := database.Exec(`DELETE FROM domain_deletion_operations WHERE domain_id = 7`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE domains SET status = 'active' WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	if err := p.measureDueSiteDisks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := agent.calls.Load(); calls != 1 {
		t.Fatalf("measurements of the live site = %d, want 1", calls)
	}
}

// The home page lists no certificate of a domain being deleted.
func TestTheHomePageListsNoCertificateOfADomainBeingDeleted(t *testing.T) {
	p, domainID, certificateID := newSSLStateFixture(t)
	expiresAt := time.Now().UTC().Add(10 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := p.db.GetDB().Exec(`UPDATE ssl_certificates SET expires_at = ? WHERE id = ?`, expiresAt, certificateID); err != nil {
		t.Fatal(err)
	}
	read := func() dashboardExtras {
		t.Helper()
		recorder := httptest.NewRecorder()
		p.handleDashboard(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))
		var response dashboardExtras
		if recorder.Code != http.StatusOK || json.NewDecoder(recorder.Body).Decode(&response) != nil {
			t.Fatalf("dashboard: %d", recorder.Code)
		}
		return response
	}
	if got := read().ExpiringCerts; len(got) != 1 {
		t.Fatalf("before the deletion: %#v", got)
	}
	markDomainDeletingForTest(t, p, domainID)
	if got := read().ExpiringCerts; len(got) != 0 {
		t.Fatalf("the deleting domain's certificate is a home page item: %#v", got)
	}
}
