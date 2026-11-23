package sitesandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEgressSubmissionExceptionOnlyWhereWritten (S4 R2b fix round, review
// finding 5): without the exception every mail port is refused for members,
// loopback included; with it (written only while 465/587 are CelikPanel's
// own submission entries, cmd/agent/site_tasks_mail.go) a member may reach
// ports 465 and 587 at this server's own addresses - traffic to any local
// address leaves through "lo", so the server's host name reaches the name
// its certificate carries - and port 25 stays refused everywhere.
func TestEgressSubmissionExceptionOnlyWhereWritten(t *testing.T) {
	reject := "\t\tmeta l4proto tcp tcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n"
	closed := EgressRulesetWith(false)
	if EgressRuleset() != closed {
		t.Fatal("the plain ruleset is not the closed one")
	}
	if strings.Contains(closed, " accept\n") || !strings.Contains(closed, reject) {
		t.Fatalf("closed ruleset:\n%s", closed)
	}
	if !strings.Contains(closed, "cannot send mail through this server") {
		t.Fatalf("the closed file does not say sites cannot send mail through this server:\n%s", closed)
	}
	open := EgressRulesetWith(true)
	accept := "\t\toifname \"lo\" tcp dport { 465, 587 } accept\n"
	a, r := strings.Index(open, accept), strings.Index(open, reject)
	if a < 0 || r < 0 || a > r {
		t.Fatalf("rule order (accept %d, reject %d):\n%s", a, r, open)
	}
	if strings.Count(open, " accept\n") != 1 {
		t.Fatalf("an accept rule beyond submission to this server:\n%s", open)
	}
	for _, line := range strings.Split(open, "\n") {
		if strings.HasSuffix(line, " accept") && strings.Contains(line, "25") {
			t.Fatalf("port 25 is accepted somewhere: %q", line)
		}
	}
	if strings.Contains(open, "127.0.0.0/8") || strings.Contains(open, "ip6 daddr ::1") {
		t.Fatalf("the exception is still loopback-only:\n%s", open)
	}
	if !ValidUnitName("celikcron-7-3.service") || ValidUnitName("celikcron-7.service") || ValidUnitName("celikcron-7-3.timer") {
		t.Fatal("the helper does not take exactly the task services")
	}
}

// TestEgressGuardKeepsWhatATaskLeftRunning (S4 R2b fix round, review finding
// 3): a task's run that ended can leave processes in its cgroup
// (KillMode=process); the guard adds every task cgroup in a site's slice that
// still holds a process, so after a flush they are blocked again too.
func TestEgressGuardKeepsWhatATaskLeftRunning(t *testing.T) {
	guard := EgressGuardScriptText()
	for _, want := range []string{
		"'celikcron-*.service'", "celikcron-[0-9]*-[0-9]*.service)",
		"for dir in /sys/fs/cgroup/celiksite_*.slice/celikcron-*-*.service; do",
		`read -r first 2>/dev/null < "$dir/cgroup.procs" || continue`,
		`celikcron-"$site"-[0-9]*.service) ;;`,
		`"{ \"$slice/$unit\" }"`,
	} {
		if !strings.Contains(guard, want) {
			t.Fatalf("guard lacks %q:\n%s", want, guard)
		}
	}
}

// TestEgressGuardKeepsALeftoverWhoseTaskWasRemoved (S4 R2b second fix round,
// review N12): after a task's unit file is removed and systemd reloaded,
// `systemctl show -P ExecStartPre` is empty while a process the task left
// still runs in its cgroup (measured). The guard's second loop accepts such a
// cgroup by its path alone, so after a flush the leftover is blocked again;
// a cgroup whose name does not belong to its slice's site is not. On
// 6db8cd368 the loop asked systemd for ExecStartPre and skipped it.
func TestEgressGuardKeepsALeftoverWhoseTaskWasRemoved(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runs the guard with /bin/sh")
	}
	dir := t.TempDir()
	cgroups := filepath.Join(dir, "cgroup")
	for _, d := range []string{"celiksite_7.slice/celikcron-7-3.service", "celiksite_7.slice/celikcron-8-1.service", "celiksite_7.slice/celikcron-7-4.service"} {
		if err := os.MkdirAll(filepath.Join(cgroups, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	procs := map[string]string{"celiksite_7.slice/celikcron-7-3.service": "4242\n", "celiksite_7.slice/celikcron-8-1.service": "4343\n", "celiksite_7.slice/celikcron-7-4.service": ""}
	for d, body := range procs {
		if err := os.WriteFile(filepath.Join(cgroups, d, "cgroup.procs"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(dir, "bin")
	log := filepath.Join(dir, "nft.log")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// systemd knows no unit any more (its file was removed): nothing listed,
	// nothing shown. nft has the table and logs what is added.
	fakes := map[string]string{
		"systemctl": "#!/bin/sh\nexit 0\n",
		"nft":       "#!/bin/sh\nif [ \"$1\" = list ]; then echo '@smtp_blocked'; exit 0; fi\necho \"$*\" >> " + log + "\n",
	}
	for name, body := range fakes {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := strings.ReplaceAll(EgressGuardScriptText(), "/sys/fs/cgroup", cgroups)
	script = strings.Replace(script, "nft="+NFT, "nft="+filepath.Join(bin, "nft"), 1)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("guard: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(log)
	added := string(raw)
	if !strings.Contains(added, `add element inet celikpanel_egress smtp_blocked { "celiksite_7.slice/celikcron-7-3.service" }`) {
		t.Fatalf("the leftover of a removed task was not blocked again:\n%s", added)
	}
	if strings.Contains(added, "celikcron-8-1") || strings.Contains(added, "celikcron-7-4") {
		t.Fatalf("a cgroup of another site (or an empty one) was added:\n%s", added)
	}
}
