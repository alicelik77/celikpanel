package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108, the owner's Boston findings (Ubuntu 24.04, phpPgAdmin
// 7.14.7+dfsg-1 on the distribution's PHP 8.3):
//   - the proof after install failed although the tool's page answered: the
//     installed Agent runs with UMask=0027, so CelikPanel's PHP check was
//     created 0750/0640 and PHP-FPM's pool user could not open it;
//   - the Manage card said "PHP-FPM 8.3 is not running" for a failed proof;
//   - "Use PHP 8.3" on 8.3 rewrote and reloaded for nothing.
//
// The page fixtures under testdata/phppgadmin-7.14.7 are what phpPgAdmin
// 7.14.7 renders, derived by reading (not running) the Ubuntu 24.04 package
// phppgadmin_7.14.7+dfsg-1_all.deb (SHA-256
// 937c059e06db74449c1c5f8c16fe9b45db56d90524fe134366cfae5d3afd2c71; Debian 13
// ships the same source version): index.php through Misc::printHeader and
// printFooter, and lib.inc.php's refusals from lang/english.php.

func a108ToolsUmask(t *testing.T, mask int) {
	t.Helper()
	previous := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(previous) })
}

func TestA108PHPCheckIsReadableByPHPFPMUnderTheAgentsUmask(t *testing.T) {
	saved := phpToolCheckScriptPath
	phpToolCheckScriptPath = filepath.Join(t.TempDir(), "celikpanel-php-check", "check.php")
	t.Cleanup(func() { phpToolCheckScriptPath = saved })
	// The unit's UMask=0027 (deploy/systemd/celikpanel-agent.service).
	a108ToolsUmask(t, 0o027)
	if err := ensurePHPToolCheckScript(); err != nil {
		t.Fatal(err)
	}
	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s = %v %v, want %v", path, info.Mode().Perm(), err, want)
		}
	}
	dir := filepath.Dir(phpToolCheckScriptPath)
	assertMode(dir, 0o755)
	assertMode(phpToolCheckScriptPath, 0o644)

	// Boston's state after alpha.105-107: same content, unreadable modes. The
	// next configuration, move or proof repairs it.
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(phpToolCheckScriptPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ensurePHPToolCheckScript(); err != nil {
		t.Fatal(err)
	}
	assertMode(dir, 0o755)
	assertMode(phpToolCheckScriptPath, 0o644)
	if data, _ := os.ReadFile(phpToolCheckScriptPath); string(data) != phpToolCheckScript {
		t.Fatalf("script = %q", data)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".check-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
}

func a108ToolsFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "phppgadmin-7.14.7", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestA108PHPPgAdminRealPageShapes(t *testing.T) {
	// The frameset both Ubuntu 24.04 and Debian 13 render is the tool.
	if err := checkPHPToolResponse(phpToolPHPPgAdmin, 200, a108ToolsFixture(t, "index.html")); err != nil {
		t.Fatalf("the real phpPgAdmin 7.14.7 page was refused: %v", err)
	}
	// Its refusals are not, also the one that names phpPgAdmin.
	for _, name := range []string{"libnotfound.html", "badconfig.html"} {
		body := a108ToolsFixture(t, name)
		err := checkPHPToolResponse(phpToolPHPPgAdmin, 200, body)
		var typed *phpToolProbeError
		if !errors.As(err, &typed) || typed.Kind != "tool_error" || !strings.Contains(err.Error(), strings.TrimSpace(string(body))[:40]) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	// Something else answering says what it was.
	err := checkPHPToolResponse(phpToolPHPPgAdmin, 200, []byte("<html><head><title>Welcome to nginx!</title></head><body>…</body></html>"))
	var typed *phpToolProbeError
	if !errors.As(err, &typed) || typed.Kind != "not_tool_page" || !strings.Contains(err.Error(), "it answered: Welcome to nginx!") {
		t.Fatalf("err = %v", err)
	}
}

func TestA108UnreadableCheckIsItsOwnCause(t *testing.T) {
	// PHP-FPM's answer when its pool user cannot open the script.
	err := checkPHPToolCheckResponse(phpToolPHPPgAdmin, "8.3", 404, []byte("File not found.\n"))
	var typed *phpToolProbeError
	if !errors.As(err, &typed) || typed.Kind != "check_unreadable" || typed.Status != 404 ||
		!strings.Contains(err.Error(), "PHP-FPM 8.3 could not open CelikPanel's PHP check for phpPgAdmin (HTTP 404: File not found.)") {
		t.Fatalf("err = %v (%+v)", err, typed)
	}
	if err := checkPHPToolCheckResponse(phpToolPHPPgAdmin, "8.3", 403, []byte("Access denied.\n")); !errors.As(err, &typed) || typed.Kind != "check_unreadable" {
		t.Fatalf("403: err = %v", err)
	}
	err = checkPHPToolCheckResponse(phpToolPHPPgAdmin, "8.3", 502, []byte("<html><head><title>502 Bad Gateway</title></head></html>"))
	if !errors.As(err, &typed) || typed.Kind != "no_answer" || !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("502: err = %v", err)
	}
	cause := phpToolCauseOf(err, "other", "8.3")
	if cause.Kind != "no_answer" || cause.Status != 502 {
		t.Fatalf("cause = %+v", cause)
	}
}

func TestA108PageExcerptIsShortAndPlain(t *testing.T) {
	if got := pageExcerpt([]byte("<p>a\x1b[31m\n  b</p>")); got != "a [31m b" {
		t.Fatalf("got %q", got)
	}
	if got := pageExcerpt(nil); got != "an empty page" {
		t.Fatalf("got %q", got)
	}
	if got := pageExcerpt([]byte(strings.Repeat("x", 500))); len([]rune(got)) != 161 {
		t.Fatalf("len = %d", len([]rune(got)))
	}
}

func TestA108ProofIsBoundedWhateverOneAttemptWaitsFor(t *testing.T) {
	calls := 0
	probe := func(ctx context.Context, _, _ string) error {
		calls++
		if calls == 1 {
			return &phpToolProbeError{Kind: "check_unreadable", Status: 404, Message: "File not found."}
		}
		<-ctx.Done() // an attempt that would hang
		return ctx.Err()
	}
	started := time.Now()
	err := retryPHPToolProbe(context.Background(), phpToolPHPPgAdmin, "8.3", probe, 80*time.Millisecond, 5*time.Millisecond)
	if took := time.Since(started); took > 600*time.Millisecond {
		t.Fatalf("the proof took %v", took)
	}
	var typed *phpToolProbeError
	if !errors.As(err, &typed) || typed.Kind != "check_unreadable" {
		t.Fatalf("an attempt cut by the bound replaced the cause read before: %v", err)
	}
}

func TestA108SameVersionBindOnlyProves(t *testing.T) {
	host := newFakeBindHost()
	host.previous = "/run/php/php8.3-fpm.sock"
	host.packages = map[string]bool{"php8.3-pgsql": true, "php8.3-mbstring": true, "php8.3-xml": true}
	ops := host.ops()
	// The configuration already says exactly what a move to 8.3 would write.
	host.conf["/etc/nginx/conf.d/celikpanel-dbtools.conf"] = []byte("new:/run/php/php8.3-fpm.sock")

	result, err := bindPHPToolWith(context.Background(), ops, phpToolPHPPgAdmin, "8.3")
	if err != nil || !result.ProofOnly || result.MutationApplied || result.Restored {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if want := []string{"cli:8.3", "probe"}; !reflect.DeepEqual(host.steps, want) {
		t.Fatalf("steps = %v, want %v", host.steps, want)
	}
	if host.recorded[phpToolPHPPgAdmin] != "8.3" {
		t.Fatalf("recorded = %v", host.recorded)
	}

	// A failing proof changes nothing and says the real cause.
	host.steps = nil
	host.probeErr = &phpToolProbeError{Kind: "check_unreadable", Status: 404, Message: "PHP-FPM 8.3 could not open CelikPanel's PHP check"}
	result, err = bindPHPToolWith(context.Background(), ops, phpToolPHPPgAdmin, "8.3")
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolProbeFailed || typed.Cause.Kind != "check_unreadable" || typed.Cause.Status != 404 {
		t.Fatalf("err = %v (%+v)", err, typed)
	}
	if !result.ProofOnly || result.MutationApplied || result.Restored || !strings.Contains(err.Error(), "was not changed") {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if want := []string{"cli:8.3", "probe"}; !reflect.DeepEqual(host.steps, want) {
		t.Fatalf("steps = %v, want %v", host.steps, want)
	}
}

func TestA108ProofPreparesTheCheckFirst(t *testing.T) {
	host := newFakeBindHost()
	recorded := map[string]*transport.PHPToolCause{}
	ops := proveFake(host, nil, nil, recorded)
	prepared := false
	ops.prepare = func() error { prepared = true; host.steps = append(host.steps, "prepare"); return nil }
	resp := provePHPToolWith(context.Background(), ops, phpToolPHPPgAdmin, false)
	if !prepared || !resp.Proved || !reflect.DeepEqual(host.steps, []string{"prepare", "probe:8.4"}) {
		t.Fatalf("resp = %+v steps = %v", resp, host.steps)
	}

	host.steps = nil
	ops.prepare = func() error { return errors.New("read-only file system") }
	resp = provePHPToolWith(context.Background(), ops, phpToolPHPPgAdmin, false)
	if resp.Proved || recorded[phpToolPHPPgAdmin] == nil || recorded[phpToolPHPPgAdmin].Kind != "config_prepare_failed" || len(host.steps) != 0 {
		t.Fatalf("resp = %+v recorded = %+v steps = %v", resp, recorded[phpToolPHPPgAdmin], host.steps)
	}
}

func TestA108BindingsSayTheUnitStateBesideAFailedProof(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.3", true)
	conf := renderDBToolsConfig([]string{phpToolPHPPgAdmin}, map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: filepath.Join(host.run, "php8.3-fpm.sock")})
	if err := os.WriteFile(host.dbToolsConf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	savedProofs := phpToolProofRecordPath
	phpToolProofRecordPath = filepath.Join(t.TempDir(), "php-tool-proofs.json")
	t.Cleanup(func() { phpToolProofRecordPath = savedProofs })
	if err := writePHPToolProof(phpToolPHPPgAdmin, &transport.PHPToolCause{Kind: "check_unreadable", Version: "8.3", Status: 404}, false); err != nil {
		t.Fatal(err)
	}
	reads := 0
	phpFPMUnitState = func(version string) string { reads++; return "active" }
	for _, binding := range phpToolBindingsView().Tools {
		if binding.Tool != phpToolPHPPgAdmin {
			continue
		}
		if !binding.SocketReady || binding.FPMState != "active" || !binding.ProofFailed || binding.ProofCause == nil || binding.ProofCause.Kind != "check_unreadable" {
			t.Fatalf("binding = %+v", binding)
		}
	}
	if reads != 1 {
		t.Fatalf("the unit was read %d times", reads)
	}
	for out, want := range map[string]string{"active\n": "active", "failed\n": "failed", "inactive\n": "inactive", "": "unknown", "garbage": "unknown"} {
		if got := phpFPMUnitStateFrom(out); got != want {
			t.Fatalf("%q = %q, want %q", out, got, want)
		}
	}
}
