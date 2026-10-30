//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostmutationlock"
	"github.com/alicelik/celikpanel/internal/mailrenewalkit"
	"github.com/alicelik/celikpanel/internal/servicemutationledger"
)

func TestMailEnrollmentAdmissionKeepsRecordedIntent(t *testing.T) {
	target := strings.Repeat("e", 64)
	id := enrollmentLedgerID()
	empty := servicemutationledger.Ledger{Version: 1, Jobs: map[string]*servicemutationledger.ServiceMutationJob{}}
	fresh, err := mailEnrollmentAdmission(&empty, id.RequestID, id.OwnerID, target, target)
	if err != nil || !fresh {
		t.Fatal(fresh, err)
	}
	if _, err = mailEnrollmentAdmission(&empty, id.RequestID, "", "", target); err == nil {
		t.Fatal("resume admitted absence")
	}
	for _, pair := range [][2]string{{"bad", target}, {id.OwnerID, strings.Repeat("f", 64)}, {"", target}, {id.OwnerID, ""}} {
		if _, err = mailEnrollmentAdmission(&empty, id.RequestID, pair[0], pair[1], target); err == nil {
			t.Fatal("bad owner/target admitted")
		}
	}
	ledger, err := servicemutationledger.AdmitMailEnrollment(&empty, id, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"forward", "rollback", "restored"} {
		if state != "forward" {
			ledger, err = servicemutationledger.AdvanceMailEnrollment(&ledger, id, state, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
		}
		before, _ := servicemutationledger.Encode(&ledger)
		for _, owner := range []string{"", id.OwnerID} {
			reviewed := ""
			if owner != "" {
				reviewed = target
			}
			fresh, err = mailEnrollmentAdmission(&ledger, id.RequestID, owner, reviewed, target)
			if err != nil || fresh {
				t.Fatal("recorded intent replaced", state, err)
			}
		}
		if _, err = mailEnrollmentAdmission(&ledger, id.RequestID, strings.Repeat("c", 32), target, target); err == nil {
			t.Fatal("owner replacement")
		}
		after, _ := servicemutationledger.Encode(&ledger)
		if string(before) != string(after) {
			t.Fatal("admission mutated observation")
		}
	}
	ledger, _ = servicemutationledger.AdmitMailEnrollment(&empty, id, time.Now().UTC())
	if _, err = mailEnrollmentAdmission(&ledger, strings.Repeat("f", 32), id.OwnerID, target, target); err == nil {
		t.Fatal("competing admission")
	}
	ledger.Jobs[id.RequestID].Kind = "service"
	if _, err = mailEnrollmentAdmission(&ledger, id.RequestID, id.OwnerID, target, target); err == nil {
		t.Fatal("reused malformed identity")
	}
}
func TestMailEnrollmentDetachedWorkerScope(t *testing.T) {
	target := strings.Repeat("e", 64)
	id := enrollmentLedgerID()
	helper := filepath.Join(mailrenewalkit.InstalledRoot, target, mailrenewalkit.BinaryName)
	for _, accepted := range [][]string{{id.RequestID}, {id.RequestID, id.OwnerID, target}} {
		args, err := mailEnrollmentWorkerUnitArgs(helper, accepted)
		if err != nil {
			t.Fatal(err)
		}
		raw := strings.Join(args, " ")
		for _, required := range []string{"--unit=celikpanel-mail-enrollment-" + id.RequestID + ".service", "KillMode=control-group", "RuntimeMaxSec=3min", "--no-block", "--collect", helper + " --enrollment-worker"} {
			if !strings.Contains(raw, required) {
				t.Fatal(raw)
			}
		}
		for _, denied := range []string{"self-update", "OnFailure=", "Restart=always", "/bin/sh"} {
			if strings.Contains(raw, denied) {
				t.Fatal(raw)
			}
		}
	}
	for _, args := range [][]string{nil, {"bad"}, {id.RequestID, "rollback"}, {id.RequestID, id.OwnerID, "../kit"}, {id.RequestID, id.OwnerID, target, "extra"}} {
		if _, err := mailEnrollmentWorkerUnitArgs(helper, args); err == nil {
			t.Fatal(args)
		}
	}
	for _, path := range []string{"/opt/celikpanel/bin/agent", "/tmp/renew", helper + "/../renew", strings.Replace(helper, target, strings.Repeat("d", 64), 1)} {
		if _, err := mailEnrollmentWorkerUnitArgs(path, []string{id.RequestID, id.OwnerID, target}); err == nil {
			t.Fatal(path)
		}
	}
}
func TestMailEnrollmentWorkerLockHandoff(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root fixture")
	}
	root, err := os.MkdirTemp("/run", "cp-mail-worker-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	release, host := filepath.Join(root, "transaction.lock"), filepath.Join(root, "mutation.lock")
	for _, path := range []string{release, host} {
		if err = os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"-test.run=^TestMailEnrollmentWorkerLockChild$", "--", root}
	if err = runMailEnrollmentWithLocks(context.Background(), release, host, hostmutationlock.Owner{}, args); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{release, host} {
		f, e := hostmutationlock.AcquireExisting(path, hostmutationlock.Owner{})
		if e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	withMailEnrollmentWorkerLockWait(t, 0)
	for _, busy := range []string{release, host} {
		held, e := hostmutationlock.AcquireExisting(busy, hostmutationlock.Owner{})
		if e != nil {
			t.Fatal(e)
		}
		e = runMailEnrollmentWithLocks(context.Background(), release, host, hostmutationlock.Owner{}, args)
		held.Close()
		if !errors.Is(e, hostmutationlock.ErrBusy) {
			t.Fatal("parallel worker admitted", e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err = runMailEnrollmentWithLocks(ctx, release, host, hostmutationlock.Owner{}, append(args, "wait")); err == nil {
		t.Fatal("unbounded child")
	}
	if _, err = os.Stat(filepath.Join(root, "started")); err != nil {
		t.Fatal("child never reached locked work")
	}
	for _, path := range []string{release, host} {
		f, e := hostmutationlock.AcquireExisting(path, hostmutationlock.Owner{})
		if e != nil {
			t.Fatal("deadline retained exclusion", e)
		}
		f.Close()
	}
	os.Remove(host)
	if err = runMailEnrollmentWithLocks(context.Background(), release, host, hostmutationlock.Owner{}, args); err == nil {
		t.Fatal("missing runtime recreated")
	}
	if _, err = os.Stat(host); !os.IsNotExist(err) {
		t.Fatal("missing lock created")
	}
}
func withMailEnrollmentWorkerLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	old := mailEnrollmentWorkerLockWait
	mailEnrollmentWorkerLockWait = wait
	t.Cleanup(func() { mailEnrollmentWorkerLockWait = old })
}

// 2026-10-30 (Arch run ap-e): a worker that met a lock held for a moment gave
// up at once and setup waited for the owner. It now waits, bounded, while a
// lock is busy, takes both locks once they are free and only then runs; past
// the bound it is refused as before, and nothing ran.
func TestMailEnrollmentWorkerWaitsBoundedForABriefHold(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root fixture")
	}
	root, err := os.MkdirTemp("/run", "cp-mail-worker-wait-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	release, host := filepath.Join(root, "transaction.lock"), filepath.Join(root, "mutation.lock")
	for _, path := range []string{release, host} {
		if err = os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"-test.run=^TestMailEnrollmentWorkerLockChild$", "--", root}
	for _, busy := range []string{release, host} {
		withMailEnrollmentWorkerLockWait(t, 5*time.Second)
		held, e := hostmutationlock.AcquireExisting(busy, hostmutationlock.Owner{})
		if e != nil {
			t.Fatal(e)
		}
		go func() { time.Sleep(600 * time.Millisecond); held.Close() }()
		began := time.Now()
		if e = runMailEnrollmentWithLocks(context.Background(), release, host, hostmutationlock.Owner{}, args); e != nil {
			t.Fatalf("a brief hold of %s was not outwaited: %v", filepath.Base(busy), e)
		}
		if time.Since(began) < 500*time.Millisecond {
			t.Fatal("the worker ran while the lock was held")
		}
	}
	// Past the bound: the same verified refusal, and the child never ran.
	withMailEnrollmentWorkerLockWait(t, 700*time.Millisecond)
	held, err := hostmutationlock.AcquireExisting(host, hostmutationlock.Owner{})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	began := time.Now()
	err = runMailEnrollmentWithLocks(context.Background(), release, host, hostmutationlock.Owner{}, append(args, "wait"))
	if !errors.Is(err, errMailEnrollmentWorkerBusy) || time.Since(began) < 600*time.Millisecond {
		t.Fatalf("a persistent hold: %v after %s", err, time.Since(began))
	}
	if _, err = os.Stat(filepath.Join(root, "started")); !os.IsNotExist(err) {
		t.Fatal("the worker ran without the host lock")
	}
	// The release lock is not kept while waiting for the host lock.
	if f, e := hostmutationlock.AcquireExisting(release, hostmutationlock.Owner{}); e != nil {
		t.Fatal("release lock retained after the refusal", e)
	} else {
		f.Close()
	}
}

func TestMailEnrollmentWorkerLockChild(t *testing.T) {
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Skip("descriptor child")
	}
	root := os.Args[index+1]
	for path, fd := range map[string]int{filepath.Join(root, "transaction.lock"): 9, filepath.Join(root, "mutation.lock"): 8} {
		if err := hostmutationlock.VerifyInherited(path, fd, hostmutationlock.Owner{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CELIKPANEL_") || strings.HasPrefix(entry, "LD_") {
			t.Fatal("inherited override")
		}
	}
	if len(os.Args) > index+2 {
		if err := os.WriteFile(filepath.Join(root, "started"), []byte("locked"), 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
	}
}

func TestMailEnrollmentRuntimePreparationHasReleaseExclusion(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root lock fixture")
	}
	root := t.TempDir()
	release := filepath.Join(root, "transaction.lock")
	host := filepath.Join(root, "mutation.lock")
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("recorded proof unavailable")
	called := false
	err := runMailEnrollmentWithPreparedLocks(context.Background(), release, host, hostmutationlock.Owner{}, nil, func(f *os.File) error {
		called = true
		if err := hostmutationlock.VerifyInherited(release, int(f.Fd()), hostmutationlock.Owner{}); err != nil {
			t.Fatal(err)
		}
		return sentinel
	})
	if err != sentinel || !called {
		t.Fatal("preparation boundary skipped", err)
	}
	if _, err = os.Stat(host); !os.IsNotExist(err) {
		t.Fatal("denied proof created host lock")
	}
	lock, err := hostmutationlock.AcquireExisting(release, hostmutationlock.Owner{})
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	withMailEnrollmentWorkerLockWait(t, 0)
	called = false
	err = runMailEnrollmentWithPreparedLocks(context.Background(), release, host, hostmutationlock.Owner{}, nil, func(*os.File) error { called = true; return nil })
	if !errors.Is(err, errMailEnrollmentWorkerBusy) || called {
		t.Fatal("prepare ran without release exclusion", err)
	}
}

func TestMailEnrollmentRuntimeRequiresRecordedIdentity(t *testing.T) {
	id := enrollmentLedgerID()
	generation := strings.Repeat("e", 64)
	empty := servicemutationledger.Ledger{Version: 1, Jobs: map[string]*servicemutationledger.ServiceMutationJob{}}
	raw, _ := servicemutationledger.Encode(&empty)
	if _, err := recordedMailEnrollmentRuntimeIdentity(raw, []string{id.RequestID, id.OwnerID, generation}, generation); err == nil {
		t.Fatal("new intent became recorded runtime authority")
	}
	ledger, err := servicemutationledger.AdmitMailEnrollment(&empty, id, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"forward", "rollback", "restored"} {
		if state != "forward" {
			ledger, err = servicemutationledger.AdvanceMailEnrollment(&ledger, id, state, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
		}
		raw, _ = servicemutationledger.Encode(&ledger)
		for _, accepted := range [][]string{{id.RequestID}, {id.RequestID, id.OwnerID, generation}} {
			got, err := recordedMailEnrollmentRuntimeIdentity(raw, accepted, generation)
			if err != nil || got != id {
				t.Fatal(state, got, err)
			}
		}
		for _, accepted := range [][]string{{strings.Repeat("d", 32)}, {id.RequestID, strings.Repeat("c", 32), generation}, {id.RequestID, id.OwnerID, strings.Repeat("f", 64)}, nil} {
			if _, err := recordedMailEnrollmentRuntimeIdentity(raw, accepted, generation); err == nil {
				t.Fatal("invalid runtime authority accepted", accepted)
			}
		}
	}
	other := id
	other.RequestID = strings.Repeat("f", 32)
	ledger, err = servicemutationledger.AdmitMailEnrollment(&ledger, other, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = servicemutationledger.Encode(&ledger)
	if _, err := recordedMailEnrollmentRuntimeIdentity(raw, []string{id.RequestID}, generation); err == nil {
		t.Fatal("competing active operation ignored")
	}
	if _, err := recordedMailEnrollmentRuntimeIdentity([]byte("{}"), []string{id.RequestID}, generation); err == nil {
		t.Fatal("missing canonical state accepted")
	}
}
