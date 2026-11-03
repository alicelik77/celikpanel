//go:build linux

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// alpha.118 lab: logrotate is missing on minimal Debian 13 and Arch images.
// The traffic is still counted (the log is read from where it stopped, it
// only grows) and the state says missing_tool; once the tool exists the next
// hourly check writes the rotation file.
func TestTrafficCountsWithoutLogrotate(t *testing.T) {
	logs := trafficTestDirs(t)
	dir := filepath.Join(t.TempDir(), "logrotate.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	trafficLogrotatePath = filepath.Join(dir, "celikpanel-traffic")
	tool := filepath.Join(t.TempDir(), "logrotate")
	previousTools := trafficLogrotateTools
	trafficLogrotateTools = []string{tool}
	t.Cleanup(func() { trafficLogrotateTools = previousTools })

	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(logs, "9.log")
	appendTraffic(t, log, trafficLine(100), trafficLine(50))
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	appendTraffic(t, log, trafficLine(25))
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	resp := collectTraffic(t, "", 0)
	if out, requests := batchBytes(resp.Batches); out != 175 || requests != 3 {
		t.Fatalf("without logrotate: %d bytes, %d requests", out, requests)
	}
	if resp.Logrotate != "missing_tool" {
		t.Fatalf("logrotate state %q, want missing_tool", resp.Logrotate)
	}
	// The administrator installs logrotate: the next hourly check is fine.
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if state := reconcileTrafficLogrotate(trafficNow().Add(2 * time.Hour)); state == "missing_tool" || state == "failed" {
		t.Fatalf("with the tool: %q", state)
	}
	if _, err := os.Stat(trafficLogrotatePath); err != nil {
		t.Fatalf("rotation file: %v", err)
	}
}

func TestNginxInstallBringsLogrotateWhereMissing(t *testing.T) {
	catalogue := []string{"nginx"}
	got := withLogrotateCompanion("nginx", catalogue, false)
	if !reflect.DeepEqual(got, []string{"nginx", "logrotate"}) {
		t.Fatalf("nginx without logrotate: %v", got)
	}
	if !reflect.DeepEqual(catalogue, []string{"nginx"}) {
		t.Fatalf("the catalogue slice changed: %v", catalogue)
	}
	if got := withLogrotateCompanion("nginx", catalogue, true); !reflect.DeepEqual(got, catalogue) {
		t.Fatalf("logrotate present: %v", got)
	}
	if got := withLogrotateCompanion("postfix", []string{"postfix"}, false); !reflect.DeepEqual(got, []string{"postfix"}) {
		t.Fatalf("other components: %v", got)
	}
}
