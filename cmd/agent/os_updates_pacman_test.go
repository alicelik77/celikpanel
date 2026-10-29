package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestParseCheckupdatesAndArchAudit(t *testing.T) {
	got := parseCheckupdates("openssl 3.5.1-1 -> 3.5.2-1\nlinux 6.16.1.arch1-1 -> 6.16.2.arch1-1\n==> ERROR: noise\n\nlib32-glibc 2.42-1 -> 2.42-2\n")
	if want := []string{"openssl", "linux", "lib32-glibc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("checkupdates=%v", got)
	}
	if got := parseArchAuditNames("openssl\nPackage x is affected\nlibxml2\n"); !reflect.DeepEqual(got, []string{"openssl", "libxml2"}) {
		t.Fatalf("arch-audit=%v", got)
	}
}

type fakeExitCode int

func (f fakeExitCode) Error() string { return "exit" }
func (f fakeExitCode) ExitCode() int { return int(f) }

func TestOSUpdatesPacmanCountsWithoutTouchingTheSystemDatabase(t *testing.T) {
	dir := t.TempDir()
	check := filepath.Join(dir, "checkupdates")
	audit := filepath.Join(dir, "arch-audit")
	oldCheck, oldAudit, oldCommand := checkupdatesPath, archAuditPath, osUpdatesCommand
	t.Cleanup(func() { checkupdatesPath, archAuditPath, osUpdatesCommand = oldCheck, oldAudit, oldCommand })
	checkupdatesPath, archAuditPath = check, audit

	resp := &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if resp.Pending.State != "unknown" || resp.Pending.Reason != "checkupdates_missing" {
		t.Fatalf("missing checkupdates: %+v", resp.Pending)
	}
	for _, path := range []string{check, audit} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var calls [][]string
	checkResult := func() ([]byte, error) { return []byte("openssl 1 -> 2\nnginx 1 -> 2\n"), nil }
	osUpdatesCommand = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if name == check {
			if !slices.Contains(env, "CHECKUPDATES_DB=/var/lib/celikpanel-agent-private/checkupdates-db") {
				t.Fatalf("checkupdates runs on the default database copy: %v", env)
			}
			return checkResult()
		}
		return []byte("openssl\nlibfoo\n"), nil
	}
	resp = &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if p := resp.Pending; p.State != "known" || p.Total != 2 || p.Security != 1 || p.SecurityState != "known" || !reflect.DeepEqual(p.SecurityNames, []string{"openssl"}) || p.ListsAt == "" {
		t.Fatalf("pending=%+v", p)
	}
	for _, call := range calls {
		for _, arg := range call {
			if arg == "-Sy" || arg == "-Syu" || arg == "-S" {
				t.Fatalf("a status read ran a package transaction: %v", call)
			}
		}
	}
	// Exit 2 is "nothing waiting"; exit 1 is unknown, never "nothing".
	checkResult = func() ([]byte, error) { return nil, fakeExitCode(2) }
	resp = &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if resp.Pending.State != "known" || resp.Pending.Total != 0 {
		t.Fatalf("none waiting: %+v", resp.Pending)
	}
	checkResult = func() ([]byte, error) { return []byte("==> ERROR: Cannot fetch updates"), fakeExitCode(1) }
	resp = &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if resp.Pending.State != "unknown" || resp.Pending.Reason != "checkupdates_failed" {
		t.Fatalf("failed checkupdates: %+v", resp.Pending)
	}
	checkResult = func() ([]byte, error) { return nil, errors.New("not an exit code") }
	resp = &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if resp.Pending.State != "unknown" {
		t.Fatalf("unknown error: %+v", resp.Pending)
	}
	// Without arch-audit the count stands and the security count says so.
	checkResult = func() ([]byte, error) { return []byte("openssl 1 -> 2\n"), nil }
	_ = os.Remove(audit)
	resp = &transport.OSUpdatesStatusResponse{}
	osUpdatesPacman(context.Background(), resp)
	if resp.Pending.State != "known" || resp.Pending.SecurityState != "arch_audit_missing" || resp.Pending.Security != 0 {
		t.Fatalf("no arch-audit: %+v", resp.Pending)
	}
}
