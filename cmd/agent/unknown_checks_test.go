package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alicelik/celikpanel/internal/checkresult"
)

// alpha.129 audit: a check that could not finish is an unknown result, never
// "absent", "not ours" or "failed", so no rollback or recovery stops a
// running service on it.

func TestZoneSOAProbeWithoutAnAnswerInTimeIsUnknown(t *testing.T) {
	slow := func(ctx context.Context, _, _, _ string) (dnsSOAProbeResult, error) {
		<-ctx.Done()
		return dnsSOAProbeResult{}, ctx.Err()
	}
	err := verifyDNSZoneAuthoritiesAt(context.Background(), "127.0.0.1", []expectedDNSZoneAuthority{{Domain: "example.test", Serial: 7}}, slow)
	if err == nil || !checkresult.Is(err) {
		t.Fatalf("a probe without an answer in time must be unknown: %v", err)
	}
	wrong := func(context.Context, string, string, string) (dnsSOAProbeResult, error) {
		return dnsSOAProbeResult{Authoritative: true, RCode: dnsRCodeNoError, SOASerials: []uint32{6}}, nil
	}
	if err := verifyDNSZoneAuthoritiesAt(context.Background(), "127.0.0.1", []expectedDNSZoneAuthority{{Domain: "example.test", Serial: 7}}, wrong); err == nil || checkresult.Is(err) {
		t.Fatalf("a served wrong serial stays a verified failure: %v", err)
	}
}

func TestPowerDNSVendorProofCutByTheDeadlineIsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := verifyExactPDNSVendorPackageOwnership(ctx, func(lookupCtx context.Context) ([]byte, error) {
		cancel()
		return nil, lookupCtx.Err()
	})
	if err == nil || !checkresult.Is(err) {
		t.Fatalf("a dpkg-query cut by the deadline must be unknown: %v", err)
	}
	if err := verifyExactPDNSVendorPackageOwnership(context.Background(), func(context.Context) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}); err == nil || checkresult.Is(err) {
		t.Fatalf("a finished refusal stays a verified failure: %v", err)
	}
}

func TestServiceUnitProbeThatNeverAnswersIsUnknown(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("shell stand-in for systemctl")
	}
	dir := t.TempDir()
	slow := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := verifyServiceMutationUnitWithExecutable(context.Background(), slow, "example.service", false)
	if err == nil || !checkresult.Is(err) {
		t.Fatalf("systemctl is-enabled that never answers must be unknown: %v", err)
	}
	refuses := filepath.Join(dir, "systemctl-disabled")
	if err := os.WriteFile(refuses, []byte("#!/bin/sh\necho disabled\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyServiceMutationUnitWithExecutable(context.Background(), refuses, "example.service", false); err == nil || checkresult.Is(err) {
		t.Fatalf("an answered \"disabled\" stays a verified failure: %v", err)
	}
}
