package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108 (D-038 §13): before a removal the Agent names what the system's
// own traces show using the component outside CelikPanel — web-server lines,
// dependent services, connected local programs — from a fake /etc and fake
// ss output.

func writeTraceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mountTraceTree(t *testing.T, ss map[string]string, ssErr error) string {
	t.Helper()
	root := t.TempDir()
	restoreGlobs, restoreUnits, restoreSS := removalTraceWebConfigGlobs, removalTraceUnitDir, removalTraceSS
	t.Cleanup(func() {
		removalTraceWebConfigGlobs, removalTraceUnitDir, removalTraceSS = restoreGlobs, restoreUnits, restoreSS
	})
	removalTraceWebConfigGlobs = []string{
		filepath.Join(root, "nginx/nginx.conf"), filepath.Join(root, "nginx/sites-enabled/*"), filepath.Join(root, "nginx/conf.d/*.conf"),
		filepath.Join(root, "apache2/sites-enabled/*"),
	}
	removalTraceUnitDir = filepath.Join(root, "systemd")
	if err := os.MkdirAll(removalTraceUnitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removalTraceSS = func(_ context.Context, args ...string) (string, error) {
		if ssErr != nil {
			return "", ssErr
		}
		return ss[strings.Join(args, " ")], nil
	}
	return root
}

func phpVersionScope(version string) removalTraceScope {
	svc := core.GetManagedServiceByID("php-fpm")
	return newRemovalTraceScope(svc, version, []string{"php" + version + "-fpm"})
}

func TestRemovalTracesNameTheOwnersNginxLineForAPHPVersionSocket(t *testing.T) {
	root := mountTraceTree(t, map[string]string{}, nil)
	writeTraceFile(t, filepath.Join(root, "nginx/sites-enabled/legacy.conf"), "server {\n    listen 80;\n    # fastcgi_pass unix:/run/php/php8.4-fpm.sock; (old)\n    location ~ \\.php$ {\n        fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n    }\n}\n")
	writeTraceFile(t, filepath.Join(root, "nginx/sites-enabled/other.conf"), "fastcgi_pass unix:/run/php/php8.3-fpm.sock;\n")
	// CelikPanel's own tool configuration is not a trace (the tools are named apart).
	writeTraceFile(t, filepath.Join(root, "nginx/conf.d/celikpanel-dbtools.conf"), "fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n")
	writeTraceFile(t, filepath.Join(root, "apache2/sites-enabled/app.conf"), "  SetHandler \"proxy:unix:/run/php/php8.4-fpm-app.sock|fcgi://localhost\"\n")

	traces, state, unread := collectRemovalTraces(context.Background(), phpVersionScope("8.4"))
	if state != "checked" || len(unread) != 0 {
		t.Fatalf("state = %s %v", state, unread)
	}
	var lines []string
	for _, trace := range traces {
		if trace.Kind == "web_config" {
			lines = append(lines, filepath.Base(trace.Path)+":"+strconv.Itoa(trace.Line)+":"+trace.Endpoint)
		}
	}
	want := "legacy.conf:5:/run/php/php8.4-fpm.sock,app.conf:1:/run/php/php8.4-fpm-app.sock"
	if got := strings.Join(lines, ","); got != want {
		t.Fatalf("web traces = %s", got)
	}
}

func TestRemovalTracesNameTheOwnersDependentUnitsButNotTargetsOrCelikPanelsOwn(t *testing.T) {
	root := mountTraceTree(t, map[string]string{}, nil)
	units := filepath.Join(root, "systemd")
	writeTraceFile(t, filepath.Join(units, "billing.service"), "[Unit]\nDescription=Billing\nRequires=mariadb.service\nAfter=network.target mariadb.service\n")
	writeTraceFile(t, filepath.Join(units, "report.timer.d/override.conf"), "[Unit]\nWants=mariadb\n")
	writeTraceFile(t, filepath.Join(units, "celikapp-12.service"), "[Unit]\nAfter=mariadb.service\n")
	writeTraceFile(t, filepath.Join(units, "worker.service.wants/mariadb.service"), "")
	writeTraceFile(t, filepath.Join(units, "multi-user.target.wants/mariadb.service"), "")
	writeTraceFile(t, filepath.Join(units, "mariadb.service.d/limits.conf"), "[Service]\nLimitNOFILE=1000\nAfter=billing.service\n")

	svc := core.GetManagedServiceByID("mariadb")
	traces, state, _ := collectRemovalTraces(context.Background(), newRemovalTraceScope(svc, "", []string{"mariadb"}))
	if state != "checked" {
		t.Fatalf("state = %s", state)
	}
	var got []string
	for _, trace := range traces {
		if trace.Kind == "unit" {
			got = append(got, trace.Unit+"/"+trace.Relation)
		}
	}
	if strings.Join(got, ",") != "billing.service/Requires,billing.service/After,report.timer/Wants,worker.service/Wants" {
		t.Fatalf("unit traces = %v", got)
	}
}

const ssUnixFixture = `u_str ESTAB 0 0 /run/php/php8.4-fpm.sock 5001 * 5002 users:(("php-fpm8.4",pid=700,fd=9))
u_str ESTAB 0 0 * 5002 * 5001 users:(("legacy-worker",pid=900,fd=4))
u_str ESTAB 0 0 /run/php/php8.3-fpm.sock 6001 * 6002 users:(("php-fpm8.3",pid=701,fd=9))
u_str ESTAB 0 0 * 6002 * 6001 users:(("nginx",pid=800,fd=11))
u_str ESTAB 0 0 * 7002 * 7001 users:(("celikpanel-agent",pid=10,fd=3))
`

const ssTCPFixture = `ESTAB 0 0 127.0.0.1:41000 127.0.0.1:3306 users:(("python3",pid=1200,fd=5))
ESTAB 0 0 127.0.0.1:3306 127.0.0.1:41000 users:(("mariadbd",pid=600,fd=30))
ESTAB 0 0 10.0.0.5:3306 203.0.113.9:52000 users:(("mariadbd",pid=600,fd=31))
ESTAB 0 0 127.0.0.1:41010 127.0.0.1:3306 users:(("celikpanel-agent",pid=10,fd=7))
LISTEN 0 80 127.0.0.1:3306 0.0.0.0:* users:(("mariadbd",pid=600,fd=20))
`

func TestRemovalTracesNameLocalProgramsConnectedNotTheComponentItself(t *testing.T) {
	mountTraceTree(t, map[string]string{"-Hxp": ssUnixFixture, "-Htnp": ssTCPFixture}, nil)
	traces, _, _ := collectRemovalTraces(context.Background(), phpVersionScope("8.4"))
	var got []string
	for _, trace := range traces {
		if trace.Kind == "process" {
			got = append(got, trace.Process+"@"+trace.Endpoint)
		}
	}
	if strings.Join(got, ",") != "legacy-worker@/run/php/php8.4-fpm.sock" {
		t.Fatalf("php 8.4 connections = %v", got)
	}

	svc := core.GetManagedServiceByID("mariadb")
	traces, _, _ = collectRemovalTraces(context.Background(), newRemovalTraceScope(svc, "", []string{"mariadb"}))
	got = nil
	for _, trace := range traces {
		if trace.Kind == "process" {
			got = append(got, trace.Process+"@"+trace.Endpoint)
		}
	}
	if strings.Join(got, ",") != "python3@127.0.0.1:3306" {
		t.Fatalf("mariadb connections = %v", got)
	}
}

func TestRemovalTracesSayPartialWhenASourceCannotBeReadAndNothingWhenClean(t *testing.T) {
	mountTraceTree(t, nil, errors.New("ss: not found"))
	traces, state, unread := collectRemovalTraces(context.Background(), phpVersionScope("8.4"))
	if state != "partial" || strings.Join(unread, ",") != "connections" || len(traces) != 0 {
		t.Fatalf("partial = %s %v %v", state, unread, traces)
	}
	mountTraceTree(t, map[string]string{}, nil)
	traces, state, _ = collectRemovalTraces(context.Background(), phpVersionScope("8.4"))
	if state != "checked" || len(traces) != 0 {
		t.Fatalf("clean = %s %v", state, traces)
	}
	// A web server's own configuration is not a trace of it.
	nginx := core.GetManagedServiceByID("nginx")
	if facts := core.RemovalTraceFactsFor(nginx, ""); facts.WebConfig {
		t.Fatal("nginx's own configuration looked at as a trace")
	}
	if got := removalTraceVersion(core.GetManagedServiceByID("php-fpm"), "php8.4-fpm"); got != "8.4" {
		t.Fatalf("version = %q", got)
	}
}

func TestRemovalPreviewCarriesTheTraces(t *testing.T) {
	restore := removalTraceReader
	t.Cleanup(func() { removalTraceReader = restore })
	var asked removalTraceScope
	removalTraceReader = func(_ context.Context, scope removalTraceScope) ([]transport.RemovalTrace, string, []string) {
		asked = scope
		return []transport.RemovalTrace{{Kind: "unit", Unit: "billing.service", Relation: "Requires"}}, "checked", nil
	}
	resp := emptyServiceRemovalPreview()
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(string) bool { return true },
		unitExists:          func(unit string) bool { return unit == "mariadb" },
		unitsMatching:       func(string) []string { return nil },
	}
	serviceRemovalPreviewWithOps(context.Background(), &transport.ServiceRemovalPreviewRequest{ID: "mariadb"}, &resp, ops,
		func(string) (packageRemovalExecutor, error) {
			return packageRemovalExecutor{}, errors.New("no executor in this test")
		})
	if len(resp.Traces) != 1 || resp.TracesState != "checked" || !asked.ownUnit("mariadb.service") {
		t.Fatalf("preview traces = %+v %s scope=%v", resp.Traces, resp.TracesState, asked.units)
	}
}
