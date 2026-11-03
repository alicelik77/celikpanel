package services

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Website traffic measurement (alpha.118, D-044): the site's traffic log
// setting is written with the vhost and validated by the same nginx test; a
// CelikPanel page is not counted; a file the owner changed is kept.

func trafficTestGenerator(t *testing.T) (*NginxGenerator, *int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows test users cannot create production-style enabled symlinks")
	}
	previousDir := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = previousDir })
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	reloads := 0
	ng.validateNginx = func() error { return nil }
	ng.reloadNginx = func() error { reloads++; return nil }
	ng.nginxActive = func() string { return "true" }
	return ng, &reloads
}

func readTrafficFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestTrafficLogSettingIsWrittenWithTheVhostAndFollowsThePage(t *testing.T) {
	ng, reloads := trafficTestGenerator(t)
	item := ManagedVhostItem{
		Domain: "traffic.example", Body: "server { include " + PanelManagedDir("traffic.example") + "/*.conf; }\n",
		Trigger: transport.SiteFileTriggerCreate, TrafficLog: &TrafficLogItem{DomainID: 41, Counted: true},
	}
	results, err := ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].TrafficLog != transport.SiteFileChallengeWritten {
		t.Fatalf("traffic log = %q", results[0].TrafficLog)
	}
	include := readTrafficFile(t, TrafficIncludePath("traffic.example"))
	if !strings.Contains(include, "access_log "+TrafficLogPath(41)+" "+TrafficLogFormat+" buffer=64k flush=1m if=$celikpanel_traffic_counted;") {
		t.Fatalf("include:\n%s", include)
	}
	format := readTrafficFile(t, TrafficHTTPConfPath())
	if !strings.Contains(format, "log_format celikpanel_traffic '$msec $bytes_sent $request_length';") || !strings.Contains(format, "acme-challenge/ 0;") {
		t.Fatalf("format file:\n%s", format)
	}
	if info, err := os.Stat(TrafficLogDir()); err != nil || !info.IsDir() {
		t.Fatalf("log directory: %v", err)
	}
	if TrafficIncludeState("traffic.example", 41) != "counting" || TrafficHTTPConfState() != transport.SiteFileChallengeUnchanged {
		t.Fatal("states do not read back")
	}

	// The site shows a CelikPanel page: the setting keeps no log line.
	item.Trigger = transport.SiteFileTriggerChange
	item.TrafficLog = &TrafficLogItem{DomainID: 41, Counted: false}
	results, err = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if err != nil || results[0].TrafficLog != transport.SiteFileChallengeWritten {
		t.Fatalf("not counted: %+v %v", results[0], err)
	}
	if strings.Contains(readTrafficFile(t, TrafficIncludePath("traffic.example")), "access_log") || TrafficIncludeState("traffic.example", 41) != "not_counting" {
		t.Fatal("a CelikPanel page is counted")
	}
	before := *reloads
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].TrafficLog != transport.SiteFileChallengeUnchanged || *reloads != before {
		t.Fatalf("an unchanged setting was written again: %+v reloads %d->%d", results[0], before, *reloads)
	}
}

func TestTrafficLogSettingKeepsTheOwnersFiles(t *testing.T) {
	ng, _ := trafficTestGenerator(t)
	// The owner replaced CelikPanel's log format file.
	if err := os.MkdirAll(filepath.Dir(TrafficHTTPConfPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(TrafficHTTPConfPath(), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := ManagedVhostItem{
		Domain: "owner.example", Body: "server { }\n",
		Trigger: transport.SiteFileTriggerCreate, TrafficLog: &TrafficLogItem{DomainID: 42, Counted: true},
	}
	results, err := ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].TrafficLog != TrafficLogFormatKept {
		t.Fatalf("traffic log = %q", results[0].TrafficLog)
	}
	if readTrafficFile(t, TrafficHTTPConfPath()) != "# mine\n" {
		t.Fatal("the owner's format file was replaced")
	}
	// Without the format no site may name it (nginx would refuse).
	if strings.Contains(readTrafficFile(t, TrafficIncludePath("owner.example")), "access_log") {
		t.Fatal("a site names a log format that is not defined")
	}
	// The owner's own setting in CelikPanel's directory is kept as well.
	if err := os.WriteFile(TrafficIncludePath("owner.example"), []byte("access_log off;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	item.Trigger = transport.SiteFileTriggerChange
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].TrafficLog != transport.SiteFileChallengeKept || readTrafficFile(t, TrafficIncludePath("owner.example")) != "access_log off;\n" {
		t.Fatalf("the owner's setting: %+v", results[0])
	}
}

func TestTrafficLogSettingIsPutBackWhenNginxRefuses(t *testing.T) {
	ng, _ := trafficTestGenerator(t)
	ng.validateNginx = func() error { return errors.New("nginx: [emerg] refused") }
	item := ManagedVhostItem{
		Domain: "refused.example", Body: "server { }\n",
		Trigger: transport.SiteFileTriggerCreate, TrafficLog: &TrafficLogItem{DomainID: 43, Counted: true},
	}
	results, _ := ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].TrafficLog != transport.SiteFileChallengeFailed {
		t.Fatalf("traffic log = %q", results[0].TrafficLog)
	}
	if _, err := os.Stat(TrafficIncludePath("refused.example")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused setting stayed: %v", err)
	}
	if _, err := os.Stat(TrafficHTTPConfPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused format file stayed: %v", err)
	}
}

func TestTrafficLimitPageServesOnlyThePageAndValidation(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	data := VhostData{SiteID: 1, Domain: "limit.example", DocumentRoot: "/var/www/limit", ProjectType: "php",
		PHPSocket: "/run/php/php8.3-fpm.sock", ACMEChallengeRoot: testACMEChallengeRoot, TrafficLimited: true}
	out, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"celikpanel-traffic-limit", "This site has used its monthly traffic", "return 503", "include " + PanelManagedDir("limit.example")} {
		if !strings.Contains(out, want) {
			t.Fatalf("lacks %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"fastcgi_pass", "root /var/www", "include " + OwnerIncludeDir("limit.example")} {
		if strings.Contains(out, banned) {
			t.Fatalf("still has %q", banned)
		}
	}
	// Suspended wins over the traffic limit.
	data.Suspended = true
	out, _ = ng.Render(data)
	if !strings.Contains(out, "celikpanel-suspended") || strings.Contains(out, "celikpanel-traffic-limit") {
		t.Fatal("suspended does not win")
	}
	// A site without the flag renders exactly as before (no traffic page).
	data.Suspended, data.TrafficLimited = false, false
	out, _ = ng.Render(data)
	if strings.Contains(out, "celikpanel-traffic-limit") {
		t.Fatal("a normal site shows the traffic page")
	}
}
