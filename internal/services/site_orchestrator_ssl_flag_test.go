package services

import (
	"context"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// "Enable SSL" on Add domain asks for a certificate AFTER the site exists. The
// site itself is created without one (the creation vhost is rendered with SSL
// off), so the flag that means "a certificate is active and served" must stay
// false: it used to follow the requested type, and lists then reported SSL on
// for sites that never had a certificate.
func TestCreateSiteWithRequestedSSLDoesNotClaimACertificate(t *testing.T) {
	agent := &SiteOrchestratorTestAgent{
		CreateResponse: transport.CreateSiteResponse{
			Success:   true,
			PHPSocket: "/run/php/celikpanel-ssl.sock",
		},
	}
	orchestrator, database, subscriptionID := newSiteOrchestratorFixture(t, agent)
	req := createSiteTestRequest(subscriptionID, "wants-ssl.example.test")
	req.SSLType = "letsencrypt"

	response, err := orchestrator.CreateSite(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var sslEnabled bool
	if err := database.GetDB().QueryRow(
		"SELECT COALESCE(ssl_enabled, 0) FROM sites WHERE id = ?",
		response.SiteID,
	).Scan(&sslEnabled); err != nil {
		t.Fatal(err)
	}
	if sslEnabled {
		t.Fatal("ssl_enabled is true for a site created without any certificate")
	}
	var certificates int
	if err := database.GetDB().QueryRow(
		"SELECT COUNT(*) FROM ssl_certificates WHERE domain_id = ?",
		response.DomainID,
	).Scan(&certificates); err != nil {
		t.Fatal(err)
	}
	if certificates != 0 {
		t.Fatalf("site creation recorded %d certificates", certificates)
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.CreateCalls) != 1 {
		t.Fatalf("agent create calls = %d, want 1", len(agent.CreateCalls))
	}
}
