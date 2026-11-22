package sitesandbox

import (
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
