//go:build linux && realsystem

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for alpha.108 slice d (D-038 §13), run only inside a
// disposable Debian 13 virtual machine booted from the base image (nginx
// installed; build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A108=1).
// Through the Agent's own removal preview: PHP-FPM 8.4 is installed; the
// "owner" then adds by hand an nginx server block handing PHP to its socket, a
// service of their own that Requires its unit, and a program connected to its
// socket (this test process). The preview of removing PHP 8.4 must name all
// three — file and line, unit and relation, process and pid — and, once they
// are gone, say nothing. Nothing is removed. Never run it on an installed
// server.
//
// alpha.108 gerçek sistem kanıtı (kaldırma izleri); yalnız atılabilir bir
// Debian 13 sanal makinesinde koşar.

func requireRealSystemA108(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A108") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.108 VM")
	}
}

func a108dPreview(t *testing.T, a *Agent, pkg string) transport.ServiceRemovalPreviewResponse {
	t.Helper()
	var preview transport.ServiceRemovalPreviewResponse
	if err := a.ServiceRemovalPreviewV1(&transport.ServiceRemovalPreviewRequest{ID: "php-fpm", Package: pkg}, &preview); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal preview "+pkg, preview)
	return preview
}

func TestRealSystemA108RemovalTraces(t *testing.T) {
	requireRealSystemA108(t)
	a := &Agent{}
	if a107Version("php8.4-fpm") == "" {
		if resp := a107Install(t, a, "php-fpm", ""); resp.Error != "" {
			t.Fatalf("install PHP-FPM: %s", resp.Error)
		}
	}
	socket := "/run/php/php8.4-fpm.sock"
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	// The owner's own server block, by hand.
	site := "/etc/nginx/sites-enabled/a108-owner.conf"
	if err := os.WriteFile(site, []byte("server {\n    listen 127.0.0.1:8108;\n    root /var/www/html;\n    location ~ \\.php$ {\n        include snippets/fastcgi-php.conf;\n        fastcgi_pass unix:"+socket+";\n    }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The owner's own service, which requires PHP 8.4's unit.
	unit := "/etc/systemd/system/a108-owner.service"
	if err := os.WriteFile(unit, []byte("[Unit]\nDescription=alpha.108 owner workload\nRequires=php8.4-fpm.service\nAfter=php8.4-fpm.service\n\n[Service]\nExecStart=/bin/sleep infinity\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a107Out(t, "systemctl", "stop", "a108-owner.service")
		os.Remove(site)
		os.Remove(unit)
		a107Out(t, "systemctl", "daemon-reload")
		a107Out(t, "systemctl", "reload", "nginx")
	})
	a107Out(t, "nginx", "-t")
	a107Out(t, "systemctl", "reload", "nginx")
	a107Out(t, "systemctl", "daemon-reload")
	a107Out(t, "systemctl", "start", "a108-owner.service")
	// A program connected to the socket: this test process.
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("connect to %s: %v", socket, err)
	}
	a107Out(t, "ss", "-Hxp")

	preview := a108dPreview(t, a, "php8.4-fpm")
	if preview.TracesState != "checked" {
		t.Fatalf("traces state = %q unread=%v", preview.TracesState, preview.TracesUnread)
	}
	var webLine, dependent, connected bool
	for _, trace := range preview.Traces {
		switch {
		case trace.Kind == "web_config" && trace.Path == site && trace.Line == 6 && trace.Endpoint == socket:
			webLine = true
		case trace.Kind == "unit" && trace.Unit == "a108-owner.service" && trace.Relation == "Requires":
			dependent = true
		case trace.Kind == "process" && trace.PID == os.Getpid() && trace.Endpoint == socket:
			connected = true
		}
	}
	if !webLine || !dependent || !connected {
		t.Fatalf("traces web=%v unit=%v process=%v: %+v", webLine, dependent, connected, preview.Traces)
	}
	if a107Version("php8.4-fpm") == "" {
		t.Fatal("the preview removed something")
	}
	// The whole component's preview names them too.
	whole := a108dPreview(t, a, "")
	if len(whole.Traces) == 0 {
		t.Fatalf("whole-component preview names nothing: %+v", whole)
	}

	// Gone: the same preview says nothing CelikPanel knows of uses it.
	conn.Close()
	a107Out(t, "systemctl", "stop", "a108-owner.service")
	os.Remove(site)
	os.Remove(unit)
	a107Out(t, "systemctl", "daemon-reload")
	a107Out(t, "systemctl", "reload", "nginx")
	clean := a108dPreview(t, a, "php8.4-fpm")
	for _, trace := range clean.Traces {
		if strings.HasPrefix(filepath.Base(trace.Path), "a108-owner") || trace.Unit == "a108-owner.service" || trace.PID == os.Getpid() {
			t.Fatalf("a removed trace is still named: %+v", trace)
		}
	}
	t.Logf("after the owner's traces are gone: state=%s traces=%d", clean.TracesState, len(clean.Traces))
}
