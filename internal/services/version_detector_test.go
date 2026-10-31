package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpstreamPackageVersion(t *testing.T) {
	for input, want := range map[string]string{
		"1:10.11.13-0+deb13u1": "10.11.13",
		"3.0pl1-197":           "3.0",
		"5.1.5-1":              "5.1.5",
		"1.24.0-2ubuntu7.5":    "1.24.0",
		"2.4.1+dfsg1-6":        "2.4.1",
		"9.20.29":              "9.20.29",
		"1:2.3.21+dfsg1-3":     "2.3.21",
		"":                     "",
		"abc-1":                "",
	} {
		if got := UpstreamPackageVersion(input); got != want {
			t.Errorf("UpstreamPackageVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

// A service's own program is never started to learn its version: Debian's
// cron, given "version", started a second daemon that died on the pid lock
// every time the service list was read.
func TestDetectVersionNeverRunsTheServiceProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts on PATH need a Unix host")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	for _, name := range []string{"cron", "fakedaemon"} {
		script := "#!/bin/sh\necho started >> " + marker + "\necho 'version 1.2.3'\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	for _, name := range []string{"cron", "fakedaemon"} {
		if got := NewVersionDetector().DetectVersion(name); got != "unknown" {
			t.Errorf("DetectVersion(%q) = %q, want unknown", name, got)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("DetectVersion started a service program")
	}
}
