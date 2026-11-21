package sitesandbox

import (
	"strings"
	"testing"
)

// TestEgressAllowsOnlyLoopbackSubmission (S4 R2b): a member of the SMTP
// block may reach this server's own submission services on loopback (465 and
// 587, which take only clients that log in with a mailbox's password); port
// 25 stays refused everywhere, loopback included, and 465 and 587 to any
// other address. The accept rules come before the reject rule, and they name
// only loopback destinations and only the submission ports.
func TestEgressAllowsOnlyLoopbackSubmission(t *testing.T) {
	text := EgressRuleset()
	accept4 := "\t\tip daddr 127.0.0.0/8 tcp dport { 465, 587 } accept\n"
	accept6 := "\t\tip6 daddr ::1 tcp dport { 465, 587 } accept\n"
	reject := "\t\tmeta l4proto tcp tcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n"
	a4, a6, r := strings.Index(text, accept4), strings.Index(text, accept6), strings.Index(text, reject)
	if a4 < 0 || a6 < 0 || r < 0 || a4 > r || a6 > r {
		t.Fatalf("rule order (accept4 %d, accept6 %d, reject %d):\n%s", a4, a6, r, text)
	}
	if strings.Count(text, " accept\n") != 2 {
		t.Fatalf("an accept rule beyond loopback submission:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasSuffix(line, " accept") && strings.Contains(line, "25") {
			t.Fatalf("port 25 is accepted somewhere: %q", line)
		}
	}
	if !ValidUnitName("celikcron-7-3.service") || ValidUnitName("celikcron-7.service") || ValidUnitName("celikcron-7-3.timer") {
		t.Fatal("the helper does not take exactly the task services")
	}
	if !strings.Contains(EgressGuardScriptText(), "'celikcron-*.service'") || !strings.Contains(EgressGuardScriptText(), "celikcron-[0-9]*-[0-9]*.service)") {
		t.Fatal("the guard does not re-add task services")
	}
}
