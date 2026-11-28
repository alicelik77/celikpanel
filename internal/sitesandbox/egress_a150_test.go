package sitesandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// alpha.150 (S4 R2b, after the merge with alpha.149): every reading of the
// egress guard that decides "not in use, leave it out" fails towards "in
// use, keep it blocked".

// runGuard runs the guard with a fake systemctl (its body) and a fake nft
// that has the table and logs what is added; cgroups is the fake
// /sys/fs/cgroup. It answers the guard's exit error, its output and nft's
// log.
func runGuard(t *testing.T, systemctl string, cgroups string) (error, string, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("runs the guard with /bin/sh")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	log := filepath.Join(dir, "nft.log")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakes := map[string]string{
		"systemctl": "#!/bin/sh\n" + systemctl,
		"nft":       "#!/bin/sh\nif [ \"$1\" = list ]; then echo '@smtp_blocked'; exit 0; fi\necho \"$*\" >> " + log + "\n",
	}
	for name, body := range fakes {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if cgroups == "" {
		cgroups = filepath.Join(dir, "no-cgroups")
	}
	script := strings.ReplaceAll(EgressGuardScriptText(), "/sys/fs/cgroup", cgroups)
	script = strings.Replace(script, "nft="+NFT, "nft="+filepath.Join(bin, "nft"), 1)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	raw, _ := os.ReadFile(log)
	return err, string(out), string(raw)
}

// TestEgressGuardKeepsATaskCgroupWhoseProcessesCannotBeRead: a task's cgroup
// whose cgroup.procs cannot be read is added (a process may run there); an
// empty one is not. Before alpha.150 the unreadable one was left out.
func TestEgressGuardKeepsATaskCgroupWhoseProcessesCannotBeRead(t *testing.T) {
	cgroups := filepath.Join(t.TempDir(), "cgroup")
	for _, d := range []string{"celiksite_7.slice/celikcron-7-3.service", "celiksite_7.slice/celikcron-7-4.service"} {
		if err := os.MkdirAll(filepath.Join(cgroups, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A cgroup.procs that cannot be read (a directory: unreadable for root too).
	if err := os.MkdirAll(filepath.Join(cgroups, "celiksite_7.slice/celikcron-7-3.service/cgroup.procs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroups, "celiksite_7.slice/celikcron-7-4.service/cgroup.procs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err, out, added := runGuard(t, "exit 0\n", cgroups)
	if err != nil {
		t.Fatalf("guard: %v\n%s", err, out)
	}
	if !strings.Contains(added, `add element inet celikpanel_egress smtp_blocked { "celiksite_7.slice/celikcron-7-3.service" }`) {
		t.Fatalf("a task cgroup whose processes could not be read was left out:\n%s", added)
	}
	if strings.Contains(added, "celikcron-7-4") {
		t.Fatalf("an empty task cgroup was added:\n%s", added)
	}
}

// TestEgressGuardAddsAUnitWhoseHelperLineCannotBeRead: a running unit whose
// ExecStartPre cannot be read is added as if its helper loaded the block; one
// whose line reads without the helper (an earlier release's) is not; one
// whose place cannot be read is said and fails the run.
func TestEgressGuardAddsAUnitWhoseHelperLineCannotBeRead(t *testing.T) {
	systemctl := `case "$1" in
list-units) printf '%s\n' 'celikrt-5.service loaded active running x' 'celikrt-6.service loaded active running x' 'celikrt-7.service loaded active running x'; exit 0 ;;
show)
	case "$3:$4" in
		ExecStartPre:celikrt-5.service) exit 1 ;;
		ExecStartPre:celikrt-6.service) echo '{ path=/usr/bin/true ; argv[]=/usr/bin/true ; }'; exit 0 ;;
		ExecStartPre:celikrt-7.service) echo '{ argv[]=` + HelperPath + ` egress-add celikrt-7.service ; }'; exit 0 ;;
		ControlGroup:celikrt-7.service) exit 1 ;;
		ControlGroup:*) echo "/system.slice/$4"; exit 0 ;;
	esac ;;
esac
exit 0
`
	err, out, added := runGuard(t, systemctl, "")
	if !strings.Contains(added, `{ "system.slice/celikrt-5.service" }`) {
		t.Fatalf("a unit whose helper line could not be read was left out:\n%s\n%s", added, out)
	}
	if strings.Contains(added, "celikrt-6.service") {
		t.Fatalf("a unit without the helper was added:\n%s", added)
	}
	if err == nil || !strings.Contains(out, "where celikrt-7.service runs could not be read") {
		t.Fatalf("a place that could not be read passed silently: %v\n%s", err, out)
	}
}

// TestEgressGuardSaysWhenItCannotListTheUnits: a list of units that cannot be
// read fails the run with its reason; nothing is taken out of the set.
func TestEgressGuardSaysWhenItCannotListTheUnits(t *testing.T) {
	err, out, added := runGuard(t, "[ \"$1\" = list-units ] && exit 1\nexit 0\n", "")
	if err == nil || !strings.Contains(out, "could not be listed") {
		t.Fatalf("an unreadable list passed silently: %v\n%s", err, out)
	}
	if strings.Contains(added, "delete") {
		t.Fatalf("the guard took something out of the set:\n%s", added)
	}
}
