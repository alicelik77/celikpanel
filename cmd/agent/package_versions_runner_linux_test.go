//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The real runner, with a real process. A child started in the background
// writes its pid to a file, so the test can see that the whole process group
// is gone after the runner returns.

func childGone(t *testing.T, pidFile string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the child never wrote its pid: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("pid file %q", raw)
		}
		status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		// Gone, or a zombie waiting for init: either way not running.
		if err != nil || strings.Contains(string(status), ") Z ") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %d of the query's process group is still running", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPackageVersionRunnerStopsAtTheOutputCeiling(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := `sleep 60 & echo $! > "$1"; head -c 200000 /dev/zero | tr '\0' 'x'; sleep 60`
	started := time.Now()
	out, err := runPackageVersionCommand(context.Background(), 4096, "/bin/sh", "-c", script, "sh", pidFile)
	if !errors.Is(err, errPackageVersionOutputTooLarge) {
		t.Fatalf("over the ceiling: err = %v, want the too-large error", err)
	}
	if out != nil {
		t.Fatalf("a partial answer was returned: %d bytes", len(out))
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("the runner waited %s; the process should have been killed at the ceiling", elapsed)
	}
	childGone(t, pidFile)
}

func TestPackageVersionRunnerIsBoundedInTime(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := `sleep 60 & echo $! > "$1"; wait`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runPackageVersionCommand(ctx, 4096, "/bin/sh", "-c", script, "sh", pidFile)
	if err == nil {
		t.Fatal("a query past its timeout returned no error")
	}
	if elapsed := time.Since(started); elapsed > 7*time.Second {
		t.Fatalf("the runner waited %s past a 300 ms timeout", elapsed)
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("ctx.Err() = %v", ctx.Err())
	}
	childGone(t, pidFile)
}

func TestPackageVersionRunnerUsesAFixedEnvironment(t *testing.T) {
	t.Setenv("DPKG_ADMINDIR", "/tmp/elsewhere")
	t.Setenv("LANG", "tr_TR.UTF-8")
	out, err := runPackageVersionCommand(context.Background(), 4096, "/usr/bin/env")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(out))
	want := []string{"LC_ALL=C", "LANG=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	if strings.Join(lines, " ") != strings.Join(want, " ") {
		t.Fatalf("environment = %q, want exactly %q", lines, want)
	}
}

func TestPackageVersionRunnerReturnsTheWholeAnswerUnderTheCeiling(t *testing.T) {
	out, err := runPackageVersionCommand(context.Background(), 4096, "/bin/sh", "-c", "printf 'nginx\\tii \\t1.26.3-3\\n'")
	if err != nil || string(out) != "nginx\tii \t1.26.3-3\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}
