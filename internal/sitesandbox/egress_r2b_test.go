package sitesandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestTheGuardIsTheNextReleasesByteForByte (S4 R2a third fix round; alpha.150):
// alpha.148 and alpha.149 wrote R2b's guard (SHA-256
// e19ce9461d7c4a274746843990ef3de43f552e3758b1b920d4c2ed8e6077877e, branch
// security/s4-r2b-site-tasks at 6534c01f1), so that their guard already puts
// sites' tasks (their own helper path) and what a finished task left running
// back under the SMTP block after an "nft flush ruleset". alpha.150 changes
// only its readings, which now fail towards keeping a cgroup blocked
// (egress_a150_test.go); its text is pinned here, and a return to alpha.149
// writes that release's guard back, which still takes every task.
func TestTheGuardIsTheNextReleasesByteForByte(t *testing.T) {
	text := EgressGuardScriptText()
	sum := sha256.Sum256([]byte(text))
	if got := hex.EncodeToString(sum[:]); got != "ad775de890b7312e6536da6cfc22e69530ede70466baadd8dd4668c1760129d3" {
		t.Errorf("the guard's text is not alpha.150's (sha256 %s)", got)
	}
	for _, want := range []string{
		`'celikcron-*.service'`,
		`celikcron-[0-9]*-[0-9]*.service) ;;`,
		`-e 'argv[]=` + TaskHelperPath + ` egress-add '`,
		`for dir in /sys/fs/cgroup/celiksite_*.slice/celikcron-*-*.service; do`,
		`if ! procs=$(cat "$dir/cgroup.procs" 2>/dev/null); then`,
		`procs=unreadable`,
		`if pre=$(systemctl show -P ExecStartPre "$unit"); then`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the guard lacks %q", want)
		}
	}
	if TaskHelperPath != "/usr/lib/celikpanel-sandbox/sandbox-task-helper" {
		t.Errorf("the tasks' helper path is %s", TaskHelperPath)
	}
}
