package sitesandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestTheGuardIsTheNextReleasesByteForByte (S4 R2a third fix round): this
// release is where a server comes back to from the next one (R2b, sites'
// scheduled tasks as celikcron-<site>-<n> units in the site's slice). Its
// egress guard renders the same text as that release's (branch
// security/s4-r2b-site-tasks at 6534c01f1: SHA-256 below, measured by
// rendering EgressGuardScriptText from both trees), so the return rewrites
// nothing, and after an "nft flush ruleset" the guard still puts running
// tasks (their own helper path) and what a finished task left running back
// under the SMTP block.
func TestTheGuardIsTheNextReleasesByteForByte(t *testing.T) {
	text := EgressGuardScriptText()
	sum := sha256.Sum256([]byte(text))
	if got := hex.EncodeToString(sum[:]); got != "e19ce9461d7c4a274746843990ef3de43f552e3758b1b920d4c2ed8e6077877e" {
		t.Errorf("the guard's text is not the next release's (sha256 %s)", got)
	}
	for _, want := range []string{
		`'celikcron-*.service'`,
		`celikcron-[0-9]*-[0-9]*.service) ;;`,
		`-e 'argv[]=` + TaskHelperPath + ` egress-add '`,
		`for dir in /sys/fs/cgroup/celiksite_*.slice/celikcron-*-*.service; do`,
		`read -r first 2>/dev/null < "$dir/cgroup.procs" || continue`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the guard lacks %q", want)
		}
	}
	if TaskHelperPath != "/usr/lib/celikpanel-sandbox/sandbox-task-helper" {
		t.Errorf("the tasks' helper path is %s", TaskHelperPath)
	}
}
