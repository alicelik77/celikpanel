package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The removal dialogs name what the package manager would take (alpha.104,
// after the 2026-10-23 Frankfurt removal of PHP-FPM 8.4 that also purged
// phpMyAdmin): the Agent's read-only simulation is carried as it answered,
// one version can be asked for, and a read that fails says "unknown".

func stubRemovalPreview(t *testing.T, answer func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error)) *[]transport.ServiceRemovalPreviewRequest {
	t.Helper()
	restore := removalPackagePreview
	t.Cleanup(func() { removalPackagePreview = restore })
	var asked []transport.ServiceRemovalPreviewRequest
	removalPackagePreview = func(_ *Panel, _ context.Context, request transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		asked = append(asked, request)
		return answer(request)
	}
	return &asked
}

func decodeRemovalImpact(t *testing.T, panel *Panel, query string) serviceRemovalImpactResponse {
	t.Helper()
	recorder := removalImpactRequest(panel, http.MethodGet, query)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", query, recorder.Code, recorder.Body.String())
	}
	var impact serviceRemovalImpactResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &impact); err != nil {
		t.Fatal(err)
	}
	return impact
}

func TestRemovalImpactForOnePHPVersionNamesCollateralToolsAndTheDefault(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	seedRemovalScanCache(t, panel, "nginx", "php-fpm", "phpmyadmin", "phppgadmin", "roundcube")
	asked := stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{
			State:         "checked",
			PackageFamily: "apt",
			Requested:     []string{"php8.4-fpm", "php8.4-mysql"},
			Remove:        []string{"php8.4-fpm", "php8.4-mysql", "php8.4-readline"},
			Collateral: []transport.PackageRemovalCollateral{
				{Package: "php-fpm", Reason: "component", Component: "php-fpm"},
				{Package: "phpmyadmin", Reason: "component", Component: "phpmyadmin"},
				{Package: "php-mysql", Reason: "reverse_dependency"},
			},
			BoundTools: []string{"phppgadmin", "webmail"},
		}, nil
	})
	impact := decodeRemovalImpact(t, panel, "?service_id=php-fpm&package=php8.4-fpm")
	if len(*asked) != 1 || (*asked)[0] != (transport.ServiceRemovalPreviewRequest{ID: "php-fpm", Package: "php8.4-fpm"}) {
		t.Fatalf("preview asked = %+v", *asked)
	}
	if impact.Package != "php8.4-fpm" || impact.Version != "8.4" || !slices.Equal(impact.Packages, []string{"php8.4-fpm"}) || impact.AllVersions {
		t.Fatalf("impact = %+v", impact)
	}
	if impact.Hosted == nil || impact.Hosted.Kind != "php_sites" || impact.Hosted.Count != 0 || impact.Hosted.BlocksRemoval {
		t.Fatalf("hosted = %+v", impact.Hosted)
	}
	if len(impact.RequiredBy) != 0 || len(impact.Bundles) != 0 {
		t.Fatalf("a version removal named component-wide effects: %+v", impact)
	}
	sim := impact.Simulation
	if sim == nil || sim.State != "checked" || !sim.Refuses || !sim.DefaultRuntime {
		t.Fatalf("simulation = %+v", sim)
	}
	if len(sim.Collateral) != 3 || !slices.Equal(sim.BoundTools, []string{"phppgadmin", "webmail"}) || !slices.Equal(sim.Remove, []string{"php8.4-fpm", "php8.4-mysql", "php8.4-readline"}) {
		t.Fatalf("simulation = %+v", sim)
	}
}

func TestRemovalImpactSimulationStates(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	seedRemovalScanCache(t, panel, "nginx", "redis", "roundcube", "php-fpm")

	// A clean removal: named, not refused.
	stubRemovalPreview(t, func(request transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{State: "checked", Remove: []string{"redis-server", "libjemalloc2"}}, nil
	})
	if sim := decodeRemovalImpact(t, panel, "?service_id=redis").Simulation; sim == nil || sim.State != "checked" || sim.Refuses || sim.DefaultRuntime || len(sim.Remove) != 2 {
		t.Fatalf("clean simulation = %+v", sim)
	}

	// Webmail is a tree, not a package: no simulation is asked.
	asked := stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{}, errors.New("must not be asked")
	})
	if impact := decodeRemovalImpact(t, panel, "?service_id=roundcube"); impact.Simulation != nil || len(*asked) != 0 {
		t.Fatalf("roundcube simulation = %+v asked=%v", impact.Simulation, *asked)
	}

	// The read failed or timed out: unknown, never "refuses".
	stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{}, context.DeadlineExceeded
	})
	if sim := decodeRemovalImpact(t, panel, "?service_id=nginx").Simulation; sim == nil || sim.State != "unknown" || sim.Refuses || len(sim.Collateral) != 0 {
		t.Fatalf("failed simulation = %+v", sim)
	}

	// Another package operation was running: busy; tools still named.
	stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{State: "busy", BoundTools: []string{"webmail"}}, nil
	})
	sim := decodeRemovalImpact(t, panel, "?service_id=php-fpm").Simulation
	if sim == nil || sim.State != "busy" || !sim.Refuses || !slices.Equal(sim.BoundTools, []string{"webmail"}) {
		t.Fatalf("busy simulation = %+v", sim)
	}

	// A package that is not a version of the component is refused.
	if recorder := removalImpactRequest(panel, http.MethodGet, "?service_id=php-fpm&package=nginx"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad package: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestUninstallRefusalCodesCarryTheirEvidence(t *testing.T) {
	code, details, ok := uninstallRefusalCode(transport.UninstallServiceResponse{
		Error:      "removing php8.4-fpm would also remove packages it did not name: phpmyadmin; nothing was changed",
		ErrorCode:  errCodeRemovalWouldRemoveOthers,
		Collateral: []transport.PackageRemovalCollateral{{Package: "phpmyadmin", Reason: "component", Component: "phpmyadmin"}, {Package: "php-mysql", Reason: "reverse_dependency"}},
	})
	if !ok || code != errCodeRemovalWouldRemoveOthers || !slices.Equal(details, []string{"phpmyadmin", "php-mysql"}) {
		t.Fatalf("collateral refusal = %q %v %v", code, details, ok)
	}
	code, details, ok = uninstallRefusalCode(transport.UninstallServiceResponse{ErrorCode: errCodeRemovalToolsBound, BoundTools: []string{"phpmyadmin", "webmail"}})
	if !ok || code != errCodeRemovalToolsBound || !slices.Equal(details, []string{"phpMyAdmin", "Roundcube"}) {
		t.Fatalf("tools refusal = %q %v %v", code, details, ok)
	}
	if _, _, ok := uninstallRefusalCode(transport.UninstallServiceResponse{Error: "unknown service"}); ok {
		t.Fatal("an untyped refusal was given a code")
	}
}
