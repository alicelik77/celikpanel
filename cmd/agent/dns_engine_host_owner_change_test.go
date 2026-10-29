package main

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/alicelik/celikpanel/internal/binddns"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Only a verified change of the running engine's published tree is the
// owner's; a missing tree or a read error is not (Beta-1 fix).
func TestBINDOwnerChangeIsOnlyAVerifiedChangeOfARunningEngine(t *testing.T) {
	running := transport.DNSBackendRuntimeState{Engine: transport.DNSEngineBIND, Installed: true, Running: true}
	changed := fmt.Errorf("load: %w", binddns.ErrGenerationChanged)
	if !bindOwnerChangedForBackendReadiness(running, changed) {
		t.Fatal("a changed tree of the running engine")
	}
	for name, err := range map[string]error{"absent": fs.ErrNotExist, "read": errors.New("input/output error"), "none": nil} {
		if bindOwnerChangedForBackendReadiness(running, err) {
			t.Fatalf("%s read as an owner change", name)
		}
	}
	stopped := running
	stopped.Running = false
	if bindOwnerChangedForBackendReadiness(stopped, changed) {
		t.Fatal("a stopped engine serves nothing of the owner's")
	}
}
