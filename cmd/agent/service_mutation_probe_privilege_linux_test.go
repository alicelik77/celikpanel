//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/hostmutationlock"
)

// serviceMutationProbeCanProveIdle reports whether this test process can read
// every process's descriptors (CAP_SYS_PTRACE), which the host lock probe
// needs before it may answer idle. The Agent runs as root; an unprivileged
// test run gets "could not be established" instead (alpha.136).
func serviceMutationProbeCanProveIdle(t *testing.T) bool {
	t.Helper()
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if value, ok := strings.CutPrefix(line, "CapEff:"); ok {
			set, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			return set&(1<<19) != 0
		}
	}
	t.Fatal("no CapEff line")
	return false
}

// requireServiceMutationIdleProof accepts an idle answer only where the probe
// can prove it; elsewhere the same state must be refused because the holder
// could not be established, never accepted as idle.
func requireServiceMutationIdleProof(t *testing.T, err error, what string) {
	t.Helper()
	if serviceMutationProbeCanProveIdle(t) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), hostmutationlock.ErrUnknown.Error()) {
		t.Fatalf("%s without CAP_SYS_PTRACE: want the probe's unknown answer, got %v", what, err)
	}
}
