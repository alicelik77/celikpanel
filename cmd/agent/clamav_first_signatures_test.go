package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type clamAVStubCall struct {
	unit  string
	start bool
}

type clamAVStub struct {
	calls     []clamAVStubCall
	systemctl [][]string
	unitDir   string
}

func stubClamAV(t *testing.T, present func() bool, socket bool) *clamAVStub {
	t.Helper()
	stub := &clamAVStub{unitDir: t.TempDir()}
	oldEnable, oldExists, oldFile, oldPresent := clamAVEnableUnit, clamAVUnitExists, clamAVUnitFileFound, clamAVSignaturesPresent
	oldWait, oldPoll, oldDir, oldSystemctl, oldWrite := clamAVSignatureWait, clamAVSignaturePoll, clamAVUnitDir, clamAVSystemctl, clamAVWriteUnit
	t.Cleanup(func() {
		clamAVEnableUnit, clamAVUnitExists, clamAVUnitFileFound, clamAVSignaturesPresent = oldEnable, oldExists, oldFile, oldPresent
		clamAVSignatureWait, clamAVSignaturePoll, clamAVUnitDir, clamAVSystemctl, clamAVWriteUnit = oldWait, oldPoll, oldDir, oldSystemctl, oldWrite
	})
	clamAVEnableUnit = func(_ context.Context, _ string, unit string, start bool) error {
		stub.calls = append(stub.calls, clamAVStubCall{unit, start})
		return nil
	}
	clamAVUnitExists = func(_, name string) bool { return name == "clamav-freshclam" }
	clamAVUnitFileFound = func(_, name string) bool { return socket && name == clamAVDaemonSocket }
	clamAVSignaturesPresent = present
	clamAVSignatureWait, clamAVSignaturePoll = 50*time.Millisecond, 5*time.Millisecond
	clamAVUnitDir = stub.unitDir
	clamAVSystemctl = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		stub.systemctl = append(stub.systemctl, append([]string{}, args...))
		return nil, nil
	}
	clamAVWriteUnit = func(path string, content []byte) error { return os.WriteFile(path, content, 0o644) }
	return stub
}

// Signatures that arrive in the wait let the install start the daemon.
func TestClamAVSignaturesArriveDuringTheWait(t *testing.T) {
	checks := 0
	stub := stubClamAV(t, func() bool { checks++; return checks > 3 }, true)
	ready, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"})
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if want := []clamAVStubCall{{"clamav-freshclam", true}}; !reflect.DeepEqual(stub.calls, want) {
		t.Fatalf("calls %v, want %v", stub.calls, want)
	}
}

// Signatures already present: nothing is started early, nothing waits.
func TestClamAVSignaturesAlreadyPresent(t *testing.T) {
	stub := stubClamAV(t, func() bool { return true }, true)
	if ready, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("unexpected calls %v", stub.calls)
	}
}

// alpha.118 lab round 2: daily landed first, the install started the daemon,
// and systemd skipped the daemon and the socket on main. The wait now ends
// only when main AND daily are there (the units' two conditions).
func TestClamAVWaitRequiresMainAndDaily(t *testing.T) {
	dir := t.TempDir()
	old := clamAVDatabaseDir
	clamAVDatabaseDir = dir
	t.Cleanup(func() { clamAVDatabaseDir = old })
	stubClamAV(t, func() bool { return clamAVSignaturesReady(clamAVDatabaseDir) }, true)
	if err := os.WriteFile(filepath.Join(dir, "daily.cvd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ready, _ := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); ready {
		t.Fatal("daily alone must not count as ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "main.cvd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ready, _ := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); !ready {
		t.Fatal("main and daily must count as ready")
	}
}

// With the signatures complete, the socket starts before the daemon: an
// explicit start re-evaluates the conditions a skipped start left unmet.
func TestClamAVStartsSocketThenDaemon(t *testing.T) {
	stub := stubClamAV(t, func() bool { return true }, true)
	if _, err := clamAVStartWithSignatures(context.Background(), "/usr/bin/systemctl", "clamav-daemon"); err != nil {
		t.Fatal(err)
	}
	want := []clamAVStubCall{{clamAVDaemonSocket, true}, {"clamav-daemon", true}}
	if !reflect.DeepEqual(stub.calls, want) {
		t.Fatalf("calls %v, want %v", stub.calls, want)
	}
}

// A download that outlasts the wait is "waiting", not a failure, and not
// stuck: the daemon and socket are enabled without a start (systemd would
// skip them) and the standard path unit is written and turned on.
func TestClamAVWaitingArmsThePathUnit(t *testing.T) {
	stub := stubClamAV(t, func() bool { return false }, true)
	waiting, note, err := clamAVArmFirstStart(context.Background(), "/usr/bin/systemctl", "clamav-daemon")
	if err != nil || !waiting || note != "" {
		t.Fatalf("waiting=%v note=%q err=%v", waiting, note, err)
	}
	want := []clamAVStubCall{{"clamav-daemon", false}, {clamAVDaemonSocket, false}}
	if !reflect.DeepEqual(stub.calls, want) {
		t.Fatalf("calls %v, want %v", stub.calls, want)
	}
	wantSystemctl := [][]string{{"daemon-reload"}, {"enable", "--now", clamAVFirstStartPathUnit}}
	if !reflect.DeepEqual(stub.systemctl, wantSystemctl) {
		t.Fatalf("systemctl %v, want %v", stub.systemctl, wantSystemctl)
	}
	service, err := os.ReadFile(filepath.Join(stub.unitDir, clamAVFirstStartServiceUnit))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(service), "\nCondition") {
		t.Errorf("a [Unit] condition leaves the path unit without a state change:\n%s", service)
	}
	for _, line := range []string{
		"ExecCondition=/bin/sh -c 'for set in main daily; do [ -f /var/lib/clamav/$${set}.cvd ] || [ -f /var/lib/clamav/$${set}.cld ] || [ -d /var/lib/clamav/$${set}.inc ] || exit 1; done'",
		"ExecCondition=/usr/bin/systemctl -q is-enabled clamav-daemon.service",
		"ExecStart=/usr/bin/systemctl --no-block start clamav-daemon.socket clamav-daemon.service",
		"ExecStartPost=/usr/bin/systemctl --no-block disable --now " + clamAVFirstStartPathUnit,
		"Type=oneshot",
	} {
		if !strings.Contains(string(service), "\n"+line+"\n") {
			t.Errorf("service unit lacks %q:\n%s", line, service)
		}
	}
	path, err := os.ReadFile(filepath.Join(stub.unitDir, clamAVFirstStartPathUnit))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"PathChanged=/var/lib/clamav", "Unit=" + clamAVFirstStartServiceUnit, "WantedBy=paths.target"} {
		if !strings.Contains(string(path), "\n"+line+"\n") {
			t.Errorf("path unit lacks %q:\n%s", line, path)
		}
	}
	// Idempotent: the same files are not rewritten, systemd is not reloaded.
	stub.systemctl = nil
	if _, _, err := clamAVArmFirstStart(context.Background(), "/usr/bin/systemctl", "clamav-daemon"); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"enable", "--now", clamAVFirstStartPathUnit}}; !reflect.DeepEqual(stub.systemctl, want) {
		t.Fatalf("second arm: systemctl %v, want %v", stub.systemctl, want)
	}
}

// The service's signature check, run as systemd runs it ("$$" is "$"),
// accepts exactly what the daemon's ConditionPathExistsGlob= lines accept.
func TestClamAVFirstStartSignatureCheckMatchesTheUnits(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	dir := t.TempDir()
	old := clamAVDatabaseDir
	clamAVDatabaseDir = dir
	t.Cleanup(func() { clamAVDatabaseDir = old })
	script := ""
	for _, line := range strings.Split(renderClamAVFirstStartService("/usr/bin/systemctl", "clamav-daemon", true), "\n") {
		if rest, ok := strings.CutPrefix(line, "ExecCondition=/bin/sh -c '"); ok {
			script = strings.ReplaceAll(strings.TrimSuffix(rest, "'"), "$$", "$")
		}
	}
	if script == "" {
		t.Fatal("no signature check")
	}
	check := func() bool { return exec.Command("/bin/sh", "-c", script).Run() == nil }
	steps := []struct {
		create func() error
		want   bool
	}{
		{func() error { return nil }, false},
		{func() error { return os.WriteFile(filepath.Join(dir, "daily.cvd"), []byte("x"), 0o644) }, false},
		{func() error { return os.Mkdir(filepath.Join(dir, "main.cvd"), 0o755) }, false},
		{func() error { return os.Mkdir(filepath.Join(dir, "main.inc"), 0o755) }, true},
	}
	for i, step := range steps {
		if err := step.create(); err != nil {
			t.Fatal(err)
		}
		if got := check(); got != step.want || got != clamAVSignaturesReady(dir) {
			t.Fatalf("step %d: shell check %v, Go check %v, want %v", i, got, clamAVSignaturesReady(dir), step.want)
		}
	}
}

// Signatures completing while the path unit was being set up: a path unit
// sees only later changes, so the install starts the daemon itself and
// removes the path unit again.
func TestClamAVSignaturesCompletingDuringArmStartTheDaemon(t *testing.T) {
	checks := 0
	stub := stubClamAV(t, func() bool { checks++; return checks > 0 }, true)
	waiting, _, err := clamAVArmFirstStart(context.Background(), "/usr/bin/systemctl", "clamav-daemon")
	if err != nil || waiting {
		t.Fatalf("waiting=%v err=%v", waiting, err)
	}
	want := []clamAVStubCall{{"clamav-daemon", false}, {clamAVDaemonSocket, false}, {clamAVDaemonSocket, true}, {"clamav-daemon", true}}
	if !reflect.DeepEqual(stub.calls, want) {
		t.Fatalf("calls %v, want %v", stub.calls, want)
	}
	for _, name := range []string{clamAVFirstStartPathUnit, clamAVFirstStartServiceUnit} {
		if _, err := os.Stat(filepath.Join(stub.unitDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s must be removed once the daemon started: %v", name, err)
		}
	}
}

// A unit file the owner edited is never rewritten or removed; it is said.
func TestClamAVFirstStartKeepsOwnerEdits(t *testing.T) {
	stub := stubClamAV(t, func() bool { return false }, false)
	owned := filepath.Join(stub.unitDir, clamAVFirstStartPathUnit)
	if err := os.WriteFile(owned, []byte("[Path]\nPathChanged=/srv/mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, note, err := clamAVArmFirstStart(context.Background(), "/usr/bin/systemctl", "clamav-daemon")
	if err != nil || !strings.Contains(note, "changed outside CelikPanel") {
		t.Fatalf("note=%q err=%v", note, err)
	}
	if got, _ := os.ReadFile(owned); string(got) != "[Path]\nPathChanged=/srv/mine\n" {
		t.Fatalf("owner file rewritten: %q", got)
	}
	// Without a socket unit the service starts the daemon alone.
	service, _ := os.ReadFile(filepath.Join(stub.unitDir, clamAVFirstStartServiceUnit))
	if !strings.Contains(string(service), "--no-block start clamav-daemon.service\n") {
		t.Fatalf("service without socket:\n%s", service)
	}
	if note := clamAVRetireFirstStart(context.Background(), "/usr/bin/systemctl"); !strings.Contains(note, "changed outside CelikPanel") {
		t.Fatalf("retire note %q", note)
	}
	if _, err := os.Stat(owned); err != nil {
		t.Fatal("owner file removed")
	}
}

// A failing updater is an error the install reports.
func TestClamAVFailingUpdater(t *testing.T) {
	stubClamAV(t, func() bool { return false }, false)
	clamAVEnableUnit = func(context.Context, string, string, bool) error { return errors.New("freshclam failed") }
	if _, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); err == nil {
		t.Fatal("a failing updater must be reported")
	}
}

// Status reads never start ClamAV: the start helpers are reached from the
// install (and the uninstall's cleanup) only.
func TestClamAVStartOnlyFromInstall(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"clamav_first_signatures.go": true, "install_rpc.go": true}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || allowed[file] {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"clamAVStartWithSignatures(", "clamAVArmFirstStart(", "clamAVAwaitFirstSignatures("} {
			if strings.Contains(string(body), name) {
				t.Errorf("%s calls %s", file, name)
			}
		}
	}
}
