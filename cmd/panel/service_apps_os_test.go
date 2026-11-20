package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.141: the facts behind "cannot be installed yet" and an app that
// cannot run on this operating system (service_apps_os.go).

// An Agent of an earlier release reports no facts: the screen still gets
// what the app needs (from the catalogue) and is told that what the server
// has was not reported, never an English fragment to put in a sentence.
func TestServiceAppFactsFromAnEarlierAgent(t *testing.T) {
	f := newServiceAppFixture(t)
	f.native.mu.Lock()
	for i := range f.native.probeApps {
		if f.native.probeApps[i].AppID == "n8n" {
			f.native.probeApps[i] = transport.ServiceAppAvailability{AppID: "n8n", Reason: serviceapps.ReasonRuntimeAbsent, Detail: "needs Node.js 24; found none"}
		}
	}
	f.native.mu.Unlock()
	resetServiceAppProbeCache()
	n8n := f.domainApps(t, 9701, "9903")["n8n"]
	if n8n.Reason != serviceapps.ReasonRuntimeAbsent || len(n8n.Needs) != 1 || n8n.Needs[0] != "24" || n8n.FoundKnown || n8n.Found != nil {
		t.Fatalf("n8n from an earlier Agent: %+v", n8n)
	}
	// A deferred app carries no runtime facts.
	if claw := f.domainApps(t, 9701, "9903")["openclaw"]; claw.Needs != nil || claw.FoundKnown {
		t.Fatalf("deferred app: %+v", claw)
	}
}

// Unknown is not unsupported: while the Agent cannot say, nothing is hidden
// from the customer and an earlier offer still counts (the install asks the
// Agent again and refuses then).
func TestServiceAppNotHiddenWhileTheAgentCannotSay(t *testing.T) {
	f := newServiceAppFixture(t)
	f.offer(t, "open-webui")
	f.native.mu.Lock()
	f.native.probeErr = errors.New("agent socket closed")
	f.native.mu.Unlock()
	resetServiceAppProbeCache()
	app, shown := f.domainApps(t, 9703, "9902")["open-webui"]
	if !shown || !app.Offered || app.Reason != serviceapps.ReasonProbeFailed {
		t.Fatalf("open-webui while the Agent cannot say: shown=%v %+v", shown, app)
	}
	if unsupported := f.panel.serviceAppsUnsupportedHere(context.Background()); len(unsupported) != 0 {
		t.Fatalf("unknown counted as unsupported: %v", unsupported)
	}
}

// Nobody can be given it on this server: the administrator's offer layer
// leaves it out and the rights screens do not list it as a candidate.
func TestServiceAppThatCannotRunHereIsNotGiven(t *testing.T) {
	f := newServiceAppFixture(t)
	f.offer(t, "open-webui")
	f.offer(t, "n8n")
	layer, err := f.panel.loadAdminOfferLayer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !layer.offers("app:open-webui") || !layer.offers("app:n8n") {
		t.Fatal("offered apps must be given while they can run here")
	}
	f.setOpenWebUIOnPython313()
	layer, err = f.panel.loadAdminOfferLayer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if layer.offers("app:open-webui") || !layer.offers("app:n8n") {
		t.Fatalf("offer layer: open-webui %v, n8n %v", layer.offers("app:open-webui"), layer.offers("app:n8n"))
	}
	for _, candidate := range f.panel.serviceAppCandidateOfferingsHere(context.Background()) {
		if candidate == "app:open-webui" {
			t.Fatal("an app that cannot run here is still a rights candidate")
		}
	}
}

// The app a domain already holds stays on the account's screen, so it can be
// managed and removed; other apps that cannot run here are left out.
func TestServiceAppHeldByTheDomainStaysVisible(t *testing.T) {
	apps := []serviceAppAppView{
		{App: serviceapps.App{ID: "open-webui"}, Reason: serviceapps.ReasonOSUnsupported},
		{App: serviceapps.App{ID: "n8n"}, Reason: serviceapps.ReasonRuntimeAbsent},
		{App: serviceapps.App{ID: "other"}, Available: true},
	}
	ids := func(list []serviceAppAppView) string {
		out := ""
		for _, app := range list {
			out += app.ID + ","
		}
		return out
	}
	if got := ids(serviceAppsShownToAccount(apps, "")); got != "n8n,other," {
		t.Fatalf("without an install: %s", got)
	}
	if got := ids(serviceAppsShownToAccount(apps, "open-webui")); got != "open-webui,n8n,other," {
		t.Fatalf("with the domain's install: %s", got)
	}
	// The real view keeps it too.
	f := newServiceAppFixture(t)
	f.setOpenWebUIOnPython313()
	f.exec(t, `INSERT INTO service_app_installs (subscription_id, domain_id, site_id, app_id, version, target_version, port, aux_port, state, previous_project_type)
		VALUES (9802, 9902, 9940, 'open-webui', '0.12.5', '0.12.5', 21100, 0, 'running', 'php')`)
	if _, shown := f.domainApps(t, 9703, "9902")["open-webui"]; !shown {
		t.Fatal("the app the domain holds disappeared from the customer's screen")
	}
	if code := f.as(t, 9703, http.MethodGet, "/api/v1/domains/9902/service-apps", "").Code; code != http.StatusOK {
		t.Fatalf("view status %d", code)
	}
}
