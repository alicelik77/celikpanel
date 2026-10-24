package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
)

// alpha.107 (D-038 §16): every decision records what it was about.

func TestAnInstallsAuditDetailsNameWhatWasConfirmedAndTheVersionsSupport(t *testing.T) {
	withSupportServer(t, "ID=debian\nVERSION_ID=\"13\"\n")
	op := serviceOperation{Kind: "service_install", ServiceID: "php-fpm", PackageName: "php8.1-fpm",
		OperationData: `{"install_php_version":"","install_confirmed":["php8.1-common"]}`}
	details := operationAuditDetails(op, serviceOperationResult{"version": "8.1.34"})
	if details["component"] != "php-fpm" || details["package"] != "php8.1-fpm" || details["version"] != "8.1" ||
		details["support"] != "unsupported" || details["confirmed_unsupported"] != true || details["installed_version"] != "8.1.34" {
		t.Fatalf("details = %v", details)
	}
	if confirmed, _ := details["confirmed_installs"].([]string); len(confirmed) != 1 || confirmed[0] != "php8.1-common" {
		t.Fatalf("confirmed = %v", details["confirmed_installs"])
	}
	node := operationAuditDetails(serviceOperation{Kind: "runtime_install", ServiceID: "node", PackageName: "22.14.0"}, nil)
	if node["version"] != "22.14.0" || node["support"] != "supported" {
		t.Fatalf("node = %v", node)
	}
}

func TestAServiceOperationsAuditRowCarriesItsDetails(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	ctx := withAuditDetails(context.Background(), map[string]any{"component": "postgresql", "package": "postgresql-17"})
	panel.auditServiceOperation(ctx, serviceOperationActor{UserID: userID}, "service.install:postgresql")
	var details sql.NullString
	if err := database.GetDB().QueryRow(`SELECT details FROM audit_logs WHERE action='service.install:postgresql'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details.String, `"package":"postgresql-17"`) {
		t.Fatalf("details = %q", details.String)
	}
}

func TestASitesNodeVersionChangeIsAudited(t *testing.T) {
	withSupportServer(t, "ID=debian\nVERSION_ID=\"13\"\n")
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	request := serviceStateAtomicRequest(http.MethodPut, "/api/v1/domains/4/hosting", "", userID)
	previous := hostingRuntimeState{DomainName: "app.example", ProjectType: "node", RuntimeVersion: sql.NullString{String: "20.19.2", Valid: true}}
	panel.auditHostingChange(request, 4, previous, hostingSettings{ProjectType: "node", RuntimeVersion: "22.14.0"})
	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "site.node_version:app.example:20.19.2->22.14.0" || !strings.Contains(details, `"from":"20.19.2"`) || !strings.Contains(details, `"support":"supported"`) {
		t.Fatalf("audit = %q %s", action, details)
	}
	// Nothing changed: nothing recorded.
	panel.auditHostingChange(request, 4, previous, hostingSettings{ProjectType: "node", RuntimeVersion: "20.19.2"})
	var count int
	database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'site.%'`).Scan(&count)
	if count != 1 {
		t.Fatalf("rows = %d", count)
	}
}
