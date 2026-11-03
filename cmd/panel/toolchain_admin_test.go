package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.119: the developer toolchains (Go, Rust) are the administrator's alone.

var toolchainIDs = []string{"go", "rust"}

// The toolchains have no offering id: no plan, no reseller set, no hidden
// list and no store product can name them, and no layer offers them.
func TestToolchainsAreNeverOfferable(t *testing.T) {
	for _, id := range toolchainIDs {
		for _, item := range []string{"component:" + id, "component:" + id + ":1.24", "component:" + id + ":1"} {
			if validOfferingID(item) || validRightsItem(item) {
				t.Fatalf("%s is a valid offering or rights item", item)
			}
			e := &effectiveRights{admin: adminOfferLayer{hidden: map[string]bool{}}, Own: rightsState{Set: allOfferedRightsSet()}}
			if e.offers(item) {
				t.Fatalf("an all-offered subscription is offered %s", item)
			}
		}
		if _, err := applyGrantRevoke(allOfferedRightsSet(), []string{"component:" + id}, nil, nil, false); err == nil {
			t.Fatalf("granting component:%s was accepted", id)
		}
	}
	// A neighbour with a similar shape is still valid (the rule is not a regexp accident).
	for _, item := range []string{"component:nginx", "component:node:22", "component:postgresql:17"} {
		if !validOfferingID(item) {
			t.Fatalf("%s lost its offering id", item)
		}
	}
}

func TestToolchainsAreAdministratorOnlyComponents(t *testing.T) {
	for _, id := range toolchainIDs {
		svc := core.GetManagedServiceByID(id)
		if svc == nil || !svc.AdminOnly || svc.Category != core.DeveloperToolsCategory || svc.Kind != core.KindTool {
			t.Fatalf("%s: %+v", id, svc)
		}
		if !core.IsAdminOnlyComponent(id) {
			t.Fatalf("%s is not administrator-only", id)
		}
	}
	for _, svc := range core.ManagedServices {
		if svc.AdminOnly && svc.Category != core.DeveloperToolsCategory {
			t.Fatalf("%s is administrator-only outside the developer tools category", svc.ID)
		}
		if svc.Category == core.DeveloperToolsCategory && !svc.AdminOnly {
			t.Fatalf("%s sits in the developer tools category without being administrator-only", svc.ID)
		}
	}
	if core.IsAdminOnlyComponent("nginx") || core.IsAdminOnlyComponent("") || core.IsAdminOnlyComponent("unknown") {
		t.Fatal("an ordinary or unknown component is administrator-only")
	}
}

// Every route that installs, removes or lists components is behind the
// administrator-only path rule, and the handlers refuse any other caller.
func TestToolchainRoutesAreAdministratorOnly(t *testing.T) {
	for _, path := range []string{
		"/api/v1/managed-services", "/api/v1/managed-services/scan", "/api/v1/service/install",
		"/api/v1/service/uninstall", "/api/v1/service/install-preview", "/api/v1/service/operation",
		"/api/v1/service/action", "/api/v1/service/profile/install", storeCatalogAdminPath,
	} {
		if !isAdminOnlyPath(path) {
			t.Fatalf("%s is not administrator-only", path)
		}
	}
	for _, id := range toolchainIDs {
		for _, role := range []string{roleCustomer, roleReseller, ""} {
			for _, caller := range []*Caller{{ID: 7, Role: role}, nil} {
				request := httptest.NewRequest(http.MethodPost, "/x", nil)
				if caller != nil {
					request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
				}
				w := httptest.NewRecorder()
				if requireAdministratorForComponent(w, request, id) || w.Code != http.StatusForbidden {
					t.Fatalf("%s as %q: allowed (%d)", id, role, w.Code)
				}
			}
		}
		admin := httptest.NewRequest(http.MethodPost, "/x", nil)
		admin = admin.WithContext(context.WithValue(admin.Context(), callerKey, &Caller{ID: 1, Role: roleAdmin}))
		if !requireAdministratorForComponent(httptest.NewRecorder(), admin, id) {
			t.Fatalf("the administrator was refused %s", id)
		}
	}
	// Ordinary components keep their own rules.
	if !requireAdministratorForComponent(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/x", nil), "nginx") {
		t.Fatal("an ordinary component was refused by the toolchain check")
	}
	// The install and uninstall handlers run the check before any work.
	for _, role := range []string{roleCustomer, roleReseller} {
		body, _ := json.Marshal(map[string]string{"service_id": "go", "request_id": strings.Repeat("a", 32)})
		for name, handle := range map[string]http.HandlerFunc{
			"install":   (&Panel{}).handleServiceInstall,
			"uninstall": (&Panel{}).handleServiceUninstall,
		} {
			request := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(string(body)))
			request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 7, Role: role}))
			w := httptest.NewRecorder()
			handle(w, request)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s as %s: %d %s", name, role, w.Code, w.Body.String())
			}
		}
	}
}

// The store never takes a toolchain as a component of a product.
func TestToolchainsAreNotStoreComponents(t *testing.T) {
	metadata := func() *storeMetadata {
		return &storeMetadata{
			Name:        storeLocalizedText{EN: "Name", TR: "Ad"},
			Description: storeLocalizedText{EN: "Text", TR: "Metin"},
			Icon:        "box", Tags: []string{},
		}
	}
	if err := validateStoreAdminUpdate("hosting", "Vendor", "available", metadata(), []string{"nginx"}, 1, "2026-01-01"); err != nil {
		t.Fatalf("an ordinary component was refused: %v", err)
	}
	for _, id := range toolchainIDs {
		if err := validateStoreAdminUpdate("hosting", "Vendor", "available", metadata(), []string{id}, 1, "2026-01-01"); err == nil {
			t.Fatalf("the store accepted %s as a component", id)
		}
	}
}

// The wizard does not list them either: they are not a server purpose.
func TestToolchainsAreNotInTheSetupWizard(t *testing.T) {
	host := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroFamily: "debian", ServiceManager: "systemd", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
	catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: host, InventoryReady: true, DNSState: &dnsEngineDBState{}})
	for _, choice := range catalog.Components {
		if core.IsAdminOnlyComponent(choice.ID) {
			t.Fatalf("the wizard lists %s", choice.ID)
		}
	}
}
